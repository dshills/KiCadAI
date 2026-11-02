// A single fixed batch. --check is read-only/offline; --live needs a separately
// recorded user approval and an authenticated offline runtime qualification.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
import { evaluationDirectory as E, evaluationID, model, policy, oldLedger, hash, read, durableJSON, offlineEnvironment, inventory, authenticateFiles, authenticateFrozen, authenticateExamples, expectedConfig, exampleID, authenticateNativeBundle, compareBundle, assessDecision, checkLedger, checkApproval, batchMetrics } from './acceptance-lib.mjs';

const batch = '.cache/board-family-v2/evaluation-final-01';
const ledgerFile = '.cache/board-family-v2/evaluation-final-01-ledger.json';
const runtimeFile = `${E}/runtime-01.json`;
const approvalFile = `${E}/APPROVAL-01.json`;
const stateFile = `${batch}/state.json`;
const cli = '/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';

export function processIsAlive(pid) {
  assert.ok(Number.isSafeInteger(pid) && pid > 0, 'missing prior process identity');
  try { process.kill(pid, 0); return true; }
  catch (error) { if (error.code === 'ESRCH') return false; throw error; }
}
export function verifyResume(state, spec, entries, alive = processIsAlive) {
  assert.equal(state.evaluation_id, evaluationID);
  assert.equal(state.status, 'stopped');
  assert.equal(alive(state.owner_pid), false, 'previous runner is still live; do not restart');
  assert.ok(state.records.length > 0 && state.records.length < spec.cases.length);
  assert.deepEqual(state.records.map(r => r.id), spec.cases.slice(0, state.records.length).map(c => c.id));
  for (const r of state.records) {
    assert.equal(r.state, 'finished', 'unknown prior outcome needs separate terminal-process evidence; never retry');
    assert.equal(r.child_terminal_observed, true);
  }
  assert.deepEqual(entries, state.ledger_entries, 'ledger changed since observed terminal child');
  return spec.cases.slice(state.records.length);
}
export async function executeCase(command, args, env, stem, beforeStart = () => {}, timeoutMS = 600_000) {
  const stdout = fs.openSync(`${stem}.stdout.log`, 'wx', 0o600);
  let stderr;
  try { stderr = fs.openSync(`${stem}.stderr.log`, 'wx', 0o600); }
  catch (error) { fs.closeSync(stdout); throw error; }
  const start = performance.now();
  try {
    beforeStart(); // Durable attempt identity is required before child creation.
    return await new Promise(resolve => {
      const child = spawn(command, args, { env, stdio: ['ignore', stdout, stderr], detached: true });
      let spawnError = null, timedOut = false;
      // Kill the whole process group, including native descendants, on timeout.
      const timer = setTimeout(() => {
        timedOut = true;
        if (child.pid) { try { process.kill(-child.pid, 'SIGKILL'); } catch (e) { if (e.code !== 'ESRCH') spawnError = e; } }
      }, timeoutMS);
      child.on('error', error => { spawnError = error; });
      child.on('close', (code, signal) => {
        clearTimeout(timer);
        resolve({ exit_code: code, signal, child_pid: child.pid ?? null, child_terminal_observed: true, timed_out: timedOut, spawn_error: spawnError?.code ?? null, wall_seconds: (performance.now() - start) / 1000 });
      });
    });
  } finally {
    fs.fsyncSync(stdout); fs.fsyncSync(stderr);
    fs.closeSync(stdout); fs.closeSync(stderr);
  }
}
export async function main(args) {
  assert.ok(args.length === 1 && ['--check', '--live', '--resume'].includes(args[0]), 'usage: node run-language.mjs --check | --live | --resume');
  const { freeze, spec } = authenticateFrozen();
  const { receipt } = authenticateExamples();
  const runtime = read(runtimeFile);
  assert.equal(runtime.status, 'offline-qualified-live-not-authorized');
  assert.equal(runtime.evaluation_id, evaluationID);
  const binary = runtime.binary;
  assert.match(binary, /^\.cache\/board-family-v2\/runtime-[0-9]+\/kicadai-board-family$/);
  assert.equal(runtime.kicad_cli, cli);
  assert.equal(runtime.freeze_sha256, hash(`${E}/freeze-01.json`));
  authenticateFiles('.', runtime.files_sha256);
  assert.equal(runtime.files_sha256[binary], hash(binary));
  assert.equal(runtime.files_sha256[cli], undefined, 'external executable identity must be explicit');
  assert.equal(runtime.kicad_cli_sha256, hash(cli));
  assert.equal(runtime.node_sha256, hash(process.execPath));
  assert.deepEqual(runtime.commands.map(c => c.id), ['build', 'go-tests', 'lint', 'node-tests', 'payload', ...spec.cases.filter(c => c.expected_disposition === 'supported').map(c => c.id)]);
  for (const result of runtime.commands) assert.equal(result.exit_code, 0, 'offline qualification failed');
  const supported = spec.cases.filter(c => c.expected_disposition === 'supported');
  assert.deepEqual(runtime.cases.map(c => c.id), supported.map(c => c.id));
  for (const [i, c] of runtime.cases.entries()) {
    assert.equal(c.example_id, exampleID(supported[i]));
    assert.deepEqual(read(`${c.directory}/configuration.json`), expectedConfig(spec, supported[i]));
    assert.deepEqual(compareBundle(c.directory, receipt.cases.find(e => e.id === c.example_id)), c.comparison);
  }
  if (args[0] === '--check') {
    console.log(JSON.stringify({ status: 'offline-preflight-pass', live_authorization: 'not-checked-or-granted', cases: 14, old_ledger_unchanged: true }));
    return;
  }
  // Nothing below this point runs during offline preparation/tests. This record
  // must quote an actual new user approval; a policy JSON is not authorization.
  const approval = read(approvalFile);
  checkApproval(approval, hash(runtimeFile), hash(`${E}/freeze-01.json`));
  assert.ok(process.env.OPENAI_API_KEY, 'approved existing key unavailable');
  const env = { ...offlineEnvironment(), OPENAI_API_KEY: process.env.OPENAI_API_KEY };
  const bindings = { approval_sha256: hash(approvalFile), runtime_sha256: hash(runtimeFile), freeze_sha256: hash(`${E}/freeze-01.json`) };
  const lock = `${batch}.lock`;
  fs.mkdirSync(lock, { mode: 0o700 }); // Existing lock always blocks; never auto-delete it.
  try {
    durableJSON(`${lock}/owner.json`, { pid: process.pid, started_utc: new Date().toISOString(), evaluation_id: evaluationID }, { exclusive: true });
    let state, pending;
    if (args[0] === '--live') {
      assert.equal(fs.existsSync(batch), false, 'batch already exists; do not reset attempts');
      assert.equal(fs.existsSync(ledgerFile), false, 'ledger already exists; never initialize another history');
      fs.mkdirSync(batch, { mode: 0o700 });
      state = { evaluation_id: evaluationID, bindings, planned_case_ids: spec.cases.map(c => c.id), owner_pid: process.pid, status: 'running', started_utc: new Date().toISOString(), records: [], ledger_entries: [] };
      pending = spec.cases;
      durableJSON(stateFile, state, { exclusive: true });
    } else {
      state = read(stateFile);
      assert.deepEqual(state.bindings, bindings, 'approval/runtime changed since batch began');
      const entries = fs.existsSync(ledgerFile) ? checkLedger(read(ledgerFile)) : [];
      pending = verifyResume(state, spec, entries);
      for (const r of state.records) authenticateFiles(batch, r.files_sha256);
      state.owner_pid = process.pid;
      state.status = 'running';
      delete state.stop_reason;
      durableJSON(stateFile, state);
    }
    for (const c of pending) {
      assert.equal(hash(oldLedger), freeze.old_ledger_sha256);
      assert.equal(hash(binary), runtime.files_sha256[binary]);
      authenticateFiles('.', runtime.files_sha256);
      const previousEntries = fs.existsSync(ledgerFile) ? checkLedger(read(ledgerFile), state.ledger_entries) : [];
      assert.deepEqual(previousEntries, state.ledger_entries);
      assert.ok(previousEntries.length < policy.max_requests);
      const prompt = `${batch}/${c.id}.txt`, directory = `${batch}/${c.id}`;
      fs.writeFileSync(prompt, c.prompt, { flag: 'wx', mode: 0o600 });
      const record = { id: c.id, state: 'attempt-recorded-before-launch', expected_disposition: c.expected_disposition, prompt_sha256: hash(prompt), started_utc: new Date().toISOString() };
      const commandArgs = ['--prompt-file', prompt, '--ledger', ledgerFile, '--live-budget', `${E}/budget-01.json`, '--output', directory, '--kicad-cli', cli];
      const execution = await executeCase(path.resolve(binary), commandArgs, env, `${batch}/${c.id}`, () => {
        state.records.push(record);
        durableJSON(stateFile, state);
      });
      Object.assign(record, execution, { state: 'finished', output_checks_pass: false });
      try {
        const entries = fs.existsSync(ledgerFile) ? checkLedger(read(ledgerFile), previousEntries) : [];
        state.ledger_entries = entries;
        assert.equal(execution.exit_code, 0, 'execution failed; no automatic retry');
        assert.equal(execution.signal, null);
        assert.equal(execution.timed_out, false);
        assert.equal(entries.length, previousEntries.length + 1, 'expected exactly one physical reservation');
        const entry = entries.at(-1), selection = read(`${directory}/selection.json`);
        assert.equal(entry.status, 'completed');
        assert.equal(selection.model, model);
        assert.equal(selection.ledger_index, entry.index);
        assert.ok(selection.response_id);
        assert.equal(selection.response_id, entry.response_id);
        assert.ok(Number.isInteger(selection.usage?.input_tokens) && selection.usage.input_tokens > 0);
        assert.ok(Number.isInteger(selection.usage?.output_tokens) && selection.usage.output_tokens > 0);
        assert.equal(selection.usage.input_tokens, entry.input_tokens);
        assert.equal(selection.usage.output_tokens, entry.output_tokens);
        assert.ok(selection.raw_decision && selection.decision, 'missing raw/admitted response evidence');
        record.response_id = selection.response_id;
        record.ledger_index = entry.index;
        record.raw = assessDecision(spec, c, selection.raw_decision);
        record.admitted = assessDecision(spec, c, selection.decision);
        record.raw_and_admitted_equal = isDeepStrictEqual(selection.raw_decision, selection.decision);
        const stdout = read(`${batch}/${c.id}.stdout.log`);
        if (selection.decision.disposition === 'supported') {
          assert.equal(stdout.passed, true, 'native/export validation did not pass');
          const cfg = selection.decision.configuration;
          const example = receipt.cases.find(e => e.id === exampleID(cfg));
          assert.ok(example, 'unexpected generated family/profile');
          authenticateNativeBundle(directory);
          record.output_checks_pass = c.expected_disposition === 'supported' && record.admitted.configuration_matches;
          if (record.output_checks_pass) {
            record.bundle = compareBundle(directory, example);
            assert.deepEqual(read(`${directory}/configuration.json`), expectedConfig(spec, c));
          } else {
            record.bundle_comparison = 'not-an-accepted-configuration; semantic miss retained, native gates authenticated';
          }
        } else {
          assert.ok(['clarify', 'unsupported'].includes(selection.decision.disposition));
          assert.deepEqual(inventory(directory), ['selection.json'], 'non-design response produced board artifacts');
          assert.equal(selection.decision.configuration, null);
          record.output_checks_pass = c.expected_disposition !== 'supported';
        }
      } catch (error) {
        record.execution_or_evidence_failure = String(error.message).slice(0, 2000);
        state.status = 'stopped';
        state.stop_reason = 'transport/accounting/protocol/native evidence failure; retain all planned cases, inspect before resuming unseen cases';
      }
      const files = [c.id + '.txt', c.id + '.stdout.log', c.id + '.stderr.log'];
      if (fs.existsSync(directory)) files.push(...inventory(directory).map(f => `${c.id}/${f}`));
      record.files_sha256 = Object.fromEntries(files.map(f => [f, hash(`${batch}/${f}`)]));
      state.metrics = batchMetrics(spec, state.records);
      state.ledger_sha256 = fs.existsSync(ledgerFile) ? hash(ledgerFile) : null;
      durableJSON(stateFile, state);
      console.log(JSON.stringify({ id: c.id, raw_automatic_checks_pass: record.raw?.automatic_checks_pass ?? false, admitted_automatic_checks_pass: record.admitted?.automatic_checks_pass ?? false, semantic_review: 'pending', seconds: record.wall_seconds, stopped: state.status === 'stopped' }));
      if (state.status === 'stopped') break;
    }
    if (state.status === 'running') state.status = 'collection-complete-semantic-review-pending';
    state.finished_utc = new Date().toISOString();
    state.metrics = batchMetrics(spec, state.records);
    assert.equal(hash(oldLedger), freeze.old_ledger_sha256);
    durableJSON(stateFile, state);
    console.log(JSON.stringify({ status: state.status, ...state.metrics }));
    if (state.status === 'stopped') process.exitCode = 1;
  } finally {
    fs.unlinkSync(`${lock}/owner.json`);
    fs.rmdirSync(lock);
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
}
