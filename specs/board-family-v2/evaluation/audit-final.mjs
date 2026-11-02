// Offline post-run authentication/publication. Never calls a provider or edits a
// frozen input, execution record, ledger, or generated output.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { isDeepStrictEqual } from 'node:util';
import { fileURLToPath } from 'node:url';
import { evaluationDirectory as E, model, read, hash, inventory, authenticateFiles, authenticateFrozen, authenticateExamples, expectedConfig, exampleID, compareBundle, assessDecision, checkLedger, checkApproval, batchMetrics } from './acceptance-lib.mjs';
import { processIsAlive } from './run-language.mjs';

const source = '.cache/board-family-v2/evaluation-final-01';
const checkpoints = `${source}-checkpoints`;
const destination = 'specs/board-family-v2/evidence/final-01';
const receiptPath = `${destination}/manifest.json`;
const mode = process.argv[2];
assert.ok(process.argv.length === 3 && ['--audit', '--publish', '--check'].includes(mode));
for (const key of ['OPENAI_API_KEY', 'ANTHROPIC_API_KEY', 'GEMINI_API_KEY', 'GOOGLE_API_KEY', 'KICADAI_LIVE_PROVIDER_TESTS']) assert.ok(!process.env[key], 'run audit without provider credentials');

const { freeze, spec } = authenticateFrozen();
const { receipt: examples, historical_files_verified } = authenticateExamples();
const base = mode === '--check' ? `${destination}/batch` : source;
const ledgerPath = mode === '--check' ? `${destination}/ledger.json` : `${source}-ledger.json`;
const checkpointBase = mode === '--check' ? `${destination}/checkpoints` : checkpoints;
const state = read(`${base}/state.json`), ledger = read(ledgerPath);
const reviewPath = `${E}/SEMANTIC-REVIEW-01.json`, review = read(reviewPath);
assert.equal(hash(`${base}/state.json`), review.final_state_sha256);
assert.equal(state.status, 'stopped'); // Final case failed; never relabel raw state.
assert.equal(state.records.length, 14);
assert.deepEqual(state.planned_case_ids, spec.cases.map(c => c.id));
assert.deepEqual(state.records.map(r => r.id), state.planned_case_ids);
assert.deepEqual(review.cases.map(r => r.id), state.planned_case_ids);
assert.equal(review.evaluation_id, spec.evaluation_id);
assert.deepEqual(state.bindings, {
  approval_sha256: hash(`${E}/APPROVAL-01.json`),
  runtime_sha256: hash(`${E}/runtime-01.json`),
  freeze_sha256: hash(`${E}/freeze-01.json`),
});
checkApproval(read(`${E}/APPROVAL-01.json`), state.bindings.runtime_sha256, state.bindings.freeze_sha256);
const entries = checkLedger(ledger);
assert.deepEqual(entries, state.ledger_entries);
assert.equal(entries.length, 14);
assert.equal(hash(ledgerPath), state.ledger_sha256);
assert.equal(new Set(entries.map(e => e.response_id)).size, 14);
assert.equal(state.bindings.runtime_sha256, '6cc472ae95b6f802d4c3ff4d8e2d2a0b82baa52e3879eb298207e3695b6b31a2');
if (mode !== '--check') {
  const runtime = read(`${E}/runtime-01.json`);
  authenticateFiles('.', runtime.files_sha256);
  assert.equal(hash(runtime.kicad_cli), runtime.kicad_cli_sha256);
  assert.equal(hash(process.execPath), runtime.node_sha256);
  assert.equal(processIsAlive(state.owner_pid), false);
  assert.equal(fs.existsSync(`${source}.lock`), false);
  assert.equal(fs.existsSync(`${source}-ledger.json.lock`), false);
}

const cases = [], expectedFiles = ['state.json'];
for (const [i, c] of spec.cases.entries()) {
  const r = state.records[i], entry = entries[i], manual = review.cases[i];
  assert.equal(r.state, 'finished');
  assert.equal(r.child_terminal_observed, true);
  assert.equal(r.signal, null);
  assert.equal(r.timed_out, false);
  assert.equal(r.spawn_error, null);
  assert.ok([0, 1].includes(r.exit_code));
  if (mode !== '--check') assert.equal(processIsAlive(r.child_pid), false);
  assert.ok(Number.isFinite(r.wall_seconds) && r.wall_seconds >= 0);
  assert.equal(fs.readFileSync(`${base}/${c.id}.txt`, 'utf8'), c.prompt);
  assert.equal(hash(`${base}/${c.id}.txt`), r.prompt_sha256);
  authenticateFiles(base, r.files_sha256);
  expectedFiles.push(...Object.keys(r.files_sha256));
  const selectionPath = `${base}/${c.id}/selection.json`, selection = read(selectionPath);
  assert.equal(hash(selectionPath), manual.selection_sha256);
  assert.ok(typeof manual.review === 'string' && manual.review.length > 50);
  for (const field of ['raw_meaning_consistent', 'local_candidate_meaning_consistent']) assert.equal(typeof manual[field], 'boolean');
  assert.equal(entry.index, i + 1);
  assert.equal(entry.status, 'completed');
  assert.equal(selection.model, model);
  assert.equal(selection.response_id, entry.response_id);
  assert.equal(selection.ledger_index, i + 1);
  assert.equal(selection.usage.input_tokens, entry.input_tokens);
  assert.equal(selection.usage.output_tokens, entry.output_tokens);
  assert.equal(selection.usage.total_tokens, entry.input_tokens + entry.output_tokens);
  for (const n of [entry.input_tokens, entry.output_tokens]) assert.ok(Number.isSafeInteger(n) && n > 0);
  assert.equal(entry.estimated_micro_usd, Math.ceil((4 * entry.input_tokens + 16 * entry.output_tokens) / 10));
  const raw = assessDecision(spec, c, selection.raw_decision);
  const candidate = assessDecision(spec, c, selection.decision);
  if (r.raw) assert.deepEqual(raw, r.raw);
  if (r.admitted) assert.deepEqual(candidate, r.admitted);
  const stdout = read(`${base}/${c.id}.stdout.log`);
  assert.equal(stdout.output, `${source}/${c.id}`); // Original execution path is retained after copying.
  let bundle = null;
  const hasNative = fs.existsSync(`${base}/${c.id}/validation.json`);
  if (hasNative) {
    assert.equal(r.exit_code, 0);
    assert.equal(stdout.passed, true);
    assert.equal(selection.decision.disposition, 'supported');
    assert.deepEqual(read(`${base}/${c.id}/configuration.json`), selection.decision.configuration);
    const example = examples.cases.find(e => e.id === exampleID(selection.decision.configuration));
    assert.ok(example);
    bundle = compareBundle(`${base}/${c.id}`, example);
    if (r.bundle) assert.deepEqual(bundle, r.bundle);
    if (c.expected_disposition === 'supported') assert.deepEqual(selection.decision.configuration, expectedConfig(spec, c));
  } else {
    assert.deepEqual(inventory(`${base}/${c.id}`), ['selection.json']);
    if (r.exit_code === 0) {
      assert.ok(['clarify', 'unsupported'].includes(selection.decision.disposition));
      assert.equal(selection.decision.configuration, null);
      assert.equal(stdout.disposition, selection.decision.disposition);
      assert.equal(stdout.message, selection.decision.message);
      assert.equal(stdout.passed, false); // No native validation was run for a non-design response.
    } else {
      assert.equal(stdout.ledger_index, i + 1);
      assert.equal(stdout.disposition, 'failed');
      assert.equal(stdout.passed, false);
      assert.ok(fs.readFileSync(`${base}/${c.id}.stderr.log`, 'utf8').trim());
    }
  }
  const outputOK = c.expected_disposition === 'supported' ? hasNative && candidate.configuration_matches : !hasNative;
  cases.push({ id: c.id, selection_sha256: hash(selectionPath), raw_checks: raw, local_candidate_checks: candidate,
    raw_contract_pass: raw.automatic_checks_pass && manual.raw_meaning_consistent,
    application_pass: r.exit_code === 0 && candidate.automatic_checks_pass && manual.local_candidate_meaning_consistent && outputOK,
    application_rejected: r.exit_code !== 0, native_bundle_present: hasNative,
    raw_and_local_candidate_equal: isDeepStrictEqual(selection.raw_decision, selection.decision),
    wall_seconds: r.wall_seconds, bundle_comparison: bundle });
}
assert.equal(new Set(expectedFiles).size, expectedFiles.length);
assert.deepEqual(inventory(base), expectedFiles.sort());

for (const [n, length] of [2, 6, 10, 13].entries()) {
  const prefix = `stop-0${n + 1}`;
  const s = read(`${checkpointBase}/${prefix}-state.json`);
  const l = read(`${checkpointBase}/${prefix}-ledger.json`);
  const resume = read(`${E}/RESUME-0${n + 1}.json`);
  assert.equal(hash(`${checkpointBase}/${prefix}-state.json`), resume.prior_state_sha256);
  assert.equal(hash(`${checkpointBase}/${prefix}-ledger.json`), resume.prior_ledger_sha256);
  assert.equal(s.status, 'stopped');
  assert.deepEqual(s.bindings, state.bindings);
  assert.deepEqual(s.records, state.records.slice(0, length));
  assert.deepEqual(l.entries, entries.slice(0, length));
  assert.deepEqual(s.ledger_entries, l.entries);
  assert.deepEqual(resume.pending_case_ids, spec.cases.slice(length).map(c => c.id));
}
assert.equal(inventory(checkpointBase).length, 8);
assert.deepEqual(state.metrics, batchMetrics(spec, state.records));
const supportedTimes = cases.slice(0, 5).map(c => c.wall_seconds).sort((a, b) => a - b);
const metrics = {
  planned_cases: 14, attempted_cases: 14, completed_provider_responses: 14,
  raw_contract_passes: cases.filter(c => c.raw_contract_pass).length,
  application_passes: cases.filter(c => c.application_pass).length,
  supported_board_successes: cases.slice(0, 5).filter(c => c.application_pass).length,
  clarify_successes: cases.slice(5, 8).filter(c => c.application_pass).length,
  unsupported_successes: cases.slice(8).filter(c => c.application_pass).length,
  application_rejections: cases.filter(c => c.application_rejected).length,
  unsupported_requests_producing_board: cases.slice(8).filter(c => c.native_bundle_present).map(c => c.id),
  native_bundles: cases.filter(c => c.native_bundle_present).length,
  native_checks_per_bundle: 14,
  all_generated_deliverables_compared: cases.reduce((n, c) => n + Object.keys(c.bundle_comparison?.files_compared ?? {}).length, 0),
  supported_generated_deliverables_compared: cases.slice(0, 5).reduce((n, c) => n + Object.keys(c.bundle_comparison?.files_compared ?? {}).length, 0),
  median_all_five_supported_seconds: supportedTimes[2], maximum_all_five_supported_seconds: supportedTimes[4],
  timing_target_met: supportedTimes[2] < 60 && supportedTimes[4] < 120,
  summed_case_wall_seconds: cases.reduce((n, c) => n + c.wall_seconds, 0),
  collection_elapsed_seconds_including_stop_reviews: (Date.parse(state.finished_utc) - Date.parse(state.started_utc)) / 1000,
  input_tokens: entries.reduce((n, e) => n + e.input_tokens, 0), output_tokens: entries.reduce((n, e) => n + e.output_tokens, 0),
  estimated_micro_usd_per_request_rounded_sum: entries.reduce((n, e) => n + e.estimated_micro_usd, 0),
  permanent_reserved_micro_usd: entries.reduce((n, e) => n + e.reserve_micro_usd, 0),
  request_slots_remaining: 0, historical_files_verified,
};
assert.equal(metrics.raw_contract_passes, 4);
assert.equal(metrics.application_passes, 8);
assert.deepEqual(metrics.unsupported_requests_producing_board, ['unsupported-04']);
const boundPaths = [`${E}/APPROVAL-01.json`, `${E}/FIREWALL-APPROVAL-01.json`, ...[1,2,3,4].map(n => `${E}/RESUME-0${n}.json`), reviewPath,
  `${E}/runtime-01.json`, `${E}/freeze-01.json`, ...Object.keys(freeze.files_sha256), `${E}/budget-01.json`,
  'README.md', 'specs/board-family-v2/RESULTS.md', 'specs/board-family-v2/REVIEW.md', 'specs/board-family-v2/README.md', 'specs/board-family-v2/WORK.md',
  'specs/board-family-v2/evidence/examples-01.json', 'specs/board-family-v2/evidence/visual-review-02.json',
  path.relative('.', fileURLToPath(import.meta.url))];
const bindings = Object.fromEntries([...new Set(boundPaths)].map(f => [f, hash(f)]));
const assessment = { status: 'complete-evidence-acceptance-failed', evaluation_id: spec.evaluation_id, started_utc: state.started_utc,
  finished_utc: state.finished_utc, raw_state_status: state.status, metrics, cases, bindings_sha256: bindings,
  limitations: ['Agent-authored targeted cases and implementing-agent review; not independent holdout or statistical reliability.',
    'SHA-256 binds local recorded bytes; provider response IDs/usage are not cryptographic provider attestations or billing reconciliation.',
    'selection.json retains structured adapter output, not raw HTTP bytes or the full provider envelope.',
    'Local candidates in rejected cases are not admitted results. Native validity does not prove requirements satisfaction.',
    'No board output was manually repaired. One semantically unsupported generated board is retained as failed evidence, not a deployable example.',
    'No fabrication, assembly, firmware or bench qualification. Both request and dollar caps were enforced; low spend does not restore request slots.'] };

function scan(file) {
  assert.ok(!/(?:sk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{20,}|Bearer\s+[A-Za-z0-9_.-]{20,})/.test(fs.readFileSync(file, 'utf8')), `possible credential in scoped evidence file: ${file}`);
}
for (const rel of inventory(base)) scan(`${base}/${rel}`);
for (const rel of inventory(checkpointBase)) scan(`${checkpointBase}/${rel}`);
scan(ledgerPath);
for (const file of boundPaths) scan(file);
if (mode === '--publish') {
  assert.equal(fs.existsSync(destination), false, 'publication already exists; never overwrite');
  const published = {};
  function copy(src, rel) {
    const dst = `${destination}/${rel}`;
    fs.mkdirSync(path.dirname(dst), { recursive: true });
    fs.copyFileSync(src, dst, fs.constants.COPYFILE_EXCL); // Mechanical byte copy, no rewriting.
    assert.equal(hash(src), hash(dst));
    published[rel] = hash(dst);
  }
  for (const file of inventory(base)) copy(`${base}/${file}`, `batch/${file}`);
  for (const file of inventory(checkpointBase)) copy(`${checkpointBase}/${file}`, `checkpoints/${file}`);
  copy(ledgerPath, 'ledger.json');
  const result = { ...assessment, published_files_sha256: published, copied_files: Object.keys(published).length };
  const content = JSON.stringify(result, null, 2) + '\n';
  const patch = `*** Begin Patch\n*** Add File: ${receiptPath}\n` + content.trimEnd().split('\n').map(l => '+' + l).join('\n') + '\n*** End Patch\n';
  const done = spawnSync('apply_patch', [], { input: patch, encoding: 'utf8', maxBuffer: 1024 * 1024 });
  assert.equal(done.status, 0, done.stderr || done.stdout);
  authenticateFiles(destination, published);
} else if (mode === '--check') {
  const { published_files_sha256, copied_files, ...publishedAssessment } = read(receiptPath);
  assert.deepEqual(assessment, publishedAssessment);
  authenticateFiles(destination, published_files_sha256);
  assert.equal(copied_files, Object.keys(published_files_sha256).length);
  assert.deepEqual(inventory(destination), [...Object.keys(published_files_sha256), 'manifest.json'].sort());
}
console.log(JSON.stringify({ status: assessment.status, mode, metrics, files_authenticated: expectedFiles.length, receipt: mode === '--audit' ? null : receiptPath }));
