// Entirely offline; response objects are synthetic and cannot establish model accuracy.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import test from 'node:test';
import { authenticateFrozen, authenticateExamples, expectedConfig, exampleID, compareBundle, assessDecision, checkApproval, checkLedger, batchMetrics, durableJSON, hash, read, policy, model, offlineEnvironment } from './acceptance-lib.mjs';
import { executeCase, verifyResume, processIsAlive } from './run-language.mjs';

const { spec } = authenticateFrozen();
const syntheticDecision = c => ({ disposition: c.expected_disposition, message: 'Synthetic offline explanation, not a real semantic judgment.', clauses: [{ text: c.prompt, disposition: c.expected_disposition, reason: 'Synthetic coverage only.' }], configuration: c.expected_disposition === 'supported' ? expectedConfig(spec, c) : null });
function temporary(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'board-family-v2-runner-test-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}
test('frozen cases and all five published bundles authenticate without an API call', () => {
  const { receipt, historical_files_verified } = authenticateExamples();
  assert.equal(historical_files_verified, 677);
  let compared = 0;
  for (const c of spec.cases.filter(c => c.expected_disposition === 'supported')) {
    const example = receipt.cases.find(e => e.id === exampleID(c));
    assert.deepEqual(read(`${example.destination}/configuration.json`), expectedConfig(spec, c));
    compared += Object.keys(compareBundle(example.destination, example).files_compared).length;
  }
  assert.equal(compared, 201);
});
test('synthetic automatic matches always leave semantic review pending', () => {
  for (const c of spec.cases) {
    const result = assessDecision(spec, c, syntheticDecision(c));
    assert.equal(result.automatic_checks_pass, true);
    assert.equal(result.semantic_review, 'pending-explicit-source-bound-review');
  }
});
test('raw omissions and local corrections are separate, not model passes', () => {
  const c = spec.cases.find(c => c.id === 'unsupported-05');
  const admitted = syntheticDecision(c), raw = structuredClone(admitted);
  raw.clauses[0].text = 'Build a BMP280 pressure controller powered directly by 5 V USB';
  assert.equal(assessDecision(spec, c, raw).automatic_checks_pass, false);
  assert.equal(assessDecision(spec, c, admitted).automatic_checks_pass, true);
  assert.equal(assessDecision(spec, c, admitted).semantic_review, 'pending-explicit-source-bound-review');
});
test('wrong config, extra keys, inconsistent clauses and malformed responses fail', () => {
  const c = spec.cases[0], good = syntheticDecision(c);
  for (const bad of [null, {}, { ...good, configuration: { ...good.configuration, supply_max_v: 3.3 } }, { ...good, extra: true }, { ...good, clauses: [null] }, { ...good, clauses: [{ text: c.prompt, disposition: 'clarify', reason: 'x' }] }, { ...good, message: '' }, { ...good, clauses: [] }]) {
    assert.equal(assessDecision(spec, c, bad).automatic_checks_pass, false);
  }
});
test('native evidence cannot pass using only a green boolean or a shortened bundle', t => {
  const { receipt } = authenticateExamples(), example = receipt.cases[0];
  const copy = path.join(temporary(t), 'copied-fixture');
  fs.cpSync(example.destination, copy, { recursive: true });
  const file = `${copy}/validation.json`, original = fs.readFileSync(file), validation = read(file);
  validation.checks[1] = validation.checks[0];
  fs.writeFileSync(file, JSON.stringify(validation));
  assert.throws(() => compareBundle(copy, example));
  fs.writeFileSync(file, original);
  fs.unlinkSync(`${copy}/manufacturing/placement.csv`);
  assert.throws(() => compareBundle(copy, example));
});
test('approval must bind this exact goal, budget, freeze and qualified runtime', () => {
  const a = { status: 'explicit-user-approved', evaluation_id: policy.goal, max_physical_requests: 14, max_micro_usd: 1_000_000, existing_key_only: true, runtime_sha256: 'runtime', freeze_sha256: 'freeze', user_message: 'Synthetic test approval; never written as execution authority.', recorded_utc: '2026-09-13T00:00:00Z' };
  checkApproval(a, 'runtime', 'freeze');
  for (const key of Object.keys(a)) {
    const bad = { ...a }; delete bad[key];
    assert.throws(() => checkApproval(bad, 'runtime', 'freeze'), key);
  }
  for (const bad of [{ ...a, max_physical_requests: 15 }, { ...a, max_micro_usd: 10_000_000 }, { ...a, runtime_sha256: 'different' }, { ...a, status: 'proposed' }]) assert.throws(() => checkApproval(bad, 'runtime', 'freeze'));
});
test('ledger verification rejects truncation, mutation, mixed goals and excess requests', () => {
  const entry = { index: 1, model, status: 'failed_or_unknown', reserve_micro_usd: 50_000 };
  const ledger = { version: 2, ...policy, entries: [entry] };
  checkLedger(ledger, [entry]);
  for (const bad of [{ ...ledger, entries: [] }, { ...ledger, goal: 'board-family-v1-2026-09-13' }, { ...ledger, max_requests: 41 }, { ...ledger, halt_reason: 'usage anomaly' }, { ...ledger, entries: [{ ...entry, status: 'completed' }] }, { ...ledger, entries: Array.from({ length: 15 }, (_, i) => ({ ...entry, index: i + 1 })) }]) assert.throws(() => checkLedger(bad, [entry]));
});
test('resume requires a terminal runner and never returns already attempted cases', () => {
  const state = { evaluation_id: policy.goal, status: 'stopped', owner_pid: 12345, records: [{ id: spec.cases[0].id, state: 'finished', child_terminal_observed: true }], ledger_entries: [] };
  assert.deepEqual(verifyResume(state, spec, [], () => false).map(c => c.id), spec.cases.slice(1).map(c => c.id));
  assert.throws(() => verifyResume(state, spec, [], () => true));
  assert.throws(() => verifyResume({ ...state, records: [{ ...state.records[0], state: 'attempt-recorded-before-launch' }] }, spec, [], () => false));
  assert.throws(() => verifyResume({ ...state, records: [{ ...state.records[0], id: spec.cases[1].id }] }, spec, [], () => false));
  assert.throws(() => verifyResume({ ...state, status: 'collection-complete-semantic-review-pending' }, spec, [], () => false));
});
test('all supported attempts count in timing, including failures and unknown outcomes', () => {
  const records = spec.cases.slice(0, 5).map((c, i) => ({ id: c.id, state: 'finished', wall_seconds: i === 2 ? 150 : 4, raw: { automatic_checks_pass: i !== 2 }, admitted: { automatic_checks_pass: i !== 2 }, output_checks_pass: i !== 2 }));
  const metrics = batchMetrics(spec, records);
  assert.equal(metrics.planned_cases, 14);
  assert.equal(metrics.maximum_all_supported_seconds, 150);
  assert.equal(metrics.timing_target_met, false);
  delete records[0].wall_seconds;
  assert.equal(batchMetrics(spec, records).median_all_supported_seconds, null);
  assert.equal(batchMetrics(spec, records).timing_target_met, false);
});
test('durable pre-launch evidence exists in a failed local child; no retry occurs', async t => {
  const directory = temporary(t), record = `${directory}/attempt.json`;
  let attempts = 0;
  const result = await executeCase(process.execPath, ['-e', 'const f=require("fs");if(!JSON.parse(f.readFileSync(process.argv[1])).attempt)process.exit(99);process.exit(17)', record], offlineEnvironment(), `${directory}/case`, () => { attempts++; durableJSON(record, { attempt: 1 }, { exclusive: true }); });
  assert.equal(result.exit_code, 17);
  assert.equal(result.child_terminal_observed, true);
  assert.equal(attempts, 1);
  assert.equal(processIsAlive(result.child_pid), false);
  assert.throws(() => durableJSON(record, { attempt: 2 }, { exclusive: true }));
  assert.equal(read(record).attempt, 1);
  assert.ok(hash(record));
});
test('a timed-out local child is killed and observed terminal', async t => {
  const directory = temporary(t);
  const result = await executeCase(process.execPath, ['-e', 'setInterval(()=>{},1000)'], offlineEnvironment(), `${directory}/case`, () => {}, 100);
  assert.equal(result.timed_out, true);
  assert.equal(result.signal, 'SIGKILL');
  assert.equal(result.child_terminal_observed, true);
  assert.equal(processIsAlive(result.child_pid), false);
});
