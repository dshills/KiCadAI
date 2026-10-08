// Offline evidence checks shared by preparation, the one live batch and tests.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import { isDeepStrictEqual } from 'node:util';
import { normalizeReplay } from '../development/replay-normalization.mjs';

export const evaluationDirectory = 'specs/board-family-v2/evaluation';
export const evaluationID = 'board-family-v2-final-01';
export const model = 'gpt-4.1-mini-2025-04-14';
export const policy = { goal: evaluationID, max_requests: 14, max_micro_usd: 1_000_000 };
export const oldLedger = '.cache/board-family-v1/live-ledger.json';
export const checks = ['electrical_contract', 'reference_and_bom_integrity', 'pcb_writer_and_connectivity', 'schematic_writer', 'native_parse_render', 'kicad_version', 'kicad_erc', 'kicad_strict_drc_and_parity', 'kicad_roundtrip_kicad_sch', 'kicad_roundtrip_kicad_pcb', 'schematic_preview', 'pcb_preview', 'manufacturing_exports', 'input_immutability'];
export const hashBytes = b => crypto.createHash('sha256').update(b).digest('hex');
export const hash = file => hashBytes(fs.readFileSync(file));
export const read = file => JSON.parse(fs.readFileSync(file, 'utf8'));
export function offlineEnvironment() {
  const env = { ...process.env };
  for (const key of ['OPENAI_API_KEY', 'ANTHROPIC_API_KEY', 'GEMINI_API_KEY', 'GOOGLE_API_KEY', 'KICADAI_LIVE_PROVIDER_TESTS']) delete env[key];
  return env;
}
export function inventory(directory, prefix = '') {
  const files = [];
  assert.ok(fs.lstatSync(path.join(directory, prefix)).isDirectory(), 'expected real directory');
  for (const item of fs.readdirSync(path.join(directory, prefix), { withFileTypes: true })) {
    const file = path.join(prefix, item.name);
    assert.equal(item.isSymbolicLink(), false, `symlink: ${file}`);
    if (item.isDirectory()) files.push(...inventory(directory, file));
    else { assert.ok(item.isFile(), `non-file: ${file}`); files.push(file); }
  }
  return files.sort();
}
export function authenticateFiles(directory, files) {
  for (const [file, expected] of Object.entries(files)) {
    assert.ok(!path.isAbsolute(file) && !file.split(/[\\/]/).includes('..'), 'unsafe evidence path');
    const full = path.join(directory, file);
    assert.ok(fs.lstatSync(full).isFile(), `not a regular file: ${full}`);
    assert.equal(hash(full), expected, `evidence changed: ${full}`);
  }
}
export function durableJSON(file, value, { exclusive = false } = {}) {
  const bytes = JSON.stringify(value, null, 2) + '\n';
  if (exclusive) {
    const fd = fs.openSync(file, 'wx', 0o600);
    try { fs.writeFileSync(fd, bytes); fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
  } else {
    const temporary = `${file}.pending-${process.pid}`;
    const fd = fs.openSync(temporary, 'wx', 0o600);
    try { fs.writeFileSync(fd, bytes); fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
    fs.renameSync(temporary, file);
  }
  const parent = fs.openSync(path.dirname(file), 'r');
  try { fs.fsyncSync(parent); } finally { fs.closeSync(parent); }
}
export function authenticateFrozen() {
  const frozenPath = `${evaluationDirectory}/freeze-01.json`;
  assert.equal(hash(frozenPath), '80d0d774a49f15240bbc159cd53da765eb9ee9e376b6e352509be3e76372c074', 'freeze changed');
  const freeze = read(frozenPath);
  authenticateFiles('.', freeze.files_sha256);
  assert.equal(hash(oldLedger), freeze.old_ledger_sha256, 'exhausted v1 ledger changed');
  assert.equal(read(oldLedger).entries.length, 41);
  assert.deepEqual(read(`${evaluationDirectory}/budget-01.json`), policy);
  const spec = read(`${evaluationDirectory}/cases-01.json`);
  assert.equal(spec.evaluation_id, evaluationID);
  assert.equal(spec.cases.length, 14);
  assert.equal(new Set(spec.cases.map(c => c.id)).size, 14);
  return { freeze, spec };
}
export function authenticateExamples() {
  const base = 'specs/board-family-v2/evidence';
  const receipt = read(`${base}/examples-01.json`);
  assert.equal(hash(`${base}/integration-checkpoint-05.json`), receipt.checkpoint_sha256);
  assert.equal(hash(`${base}/deterministic-replay-02.json`), receipt.replay_sha256);
  assert.equal(hash(`${base}/visual-review-02.json`), receipt.visual_review_sha256);
  const visual = read(`${base}/visual-review-02.json`);
  assert.equal(hash(visual.review_notes_path), visual.review_notes_sha256);
  assert.equal(visual.replay_evidence_sha256, receipt.replay_sha256);
  for (const c of receipt.cases) {
    authenticateFiles(c.destination, c.files_sha256);
    assert.deepEqual(inventory(c.destination), Object.keys(c.files_sha256).sort());
  }
  const historical = new Map();
  for (const group of ['acceptance', 'guardrails', 'integration', 'live', 'offline']) {
    const files = read(`specs/board-family-v1/evidence/${group}/manifest.json`).files;
    authenticateFiles('.', files);
    for (const [file, sha] of Object.entries(files)) {
      if (historical.has(file)) assert.equal(historical.get(file), sha);
      historical.set(file, sha);
    }
  }
  assert.equal(historical.size, 677);
  return { receipt, historical_files_verified: historical.size };
}
export function expectedConfig(spec, c) {
  return { ...spec.supported_configuration_defaults, family: c.family, profile: c.profile, total_bus_capacitance_pf: c.total_bus_capacitance_pf };
}
export function exampleID(c) {
  assert.ok(['esp32_bmp280_v1', 'esp32_sht31_v1'].includes(c.family), 'unexpected family');
  return `${c.family === 'esp32_bmp280_v1' ? 'bmp280' : 'sht31'}-${c.profile}`;
}
export function authenticateNativeBundle(directory) {
  const validation = read(`${directory}/validation.json`);
  assert.equal(validation.passed, true);
  assert.equal(validation.kicad_version, '10.0.3');
  assert.deepEqual(validation.checks.map(c => c.name), checks);
  assert.ok(validation.checks.every(c => c.passed === true));
  assert.deepEqual(Object.keys(validation.native_sha256).sort(), ['board.kicad_pcb', 'board.kicad_pro', 'board.kicad_sch']);
  authenticateFiles(directory, validation.native_sha256);
  const manufacturing = read(`${directory}/manufacturing/manifest.json`);
  assert.equal(manufacturing.source_pcb_sha256, validation.native_sha256['board.kicad_pcb']);
  assert.equal(Object.keys(manufacturing.files_sha256).length, 19);
  authenticateFiles(`${directory}/manufacturing`, manufacturing.files_sha256);
  const actual = inventory(directory).filter(f => f !== 'selection.json' && !f.endsWith('.kicad_prl'));
  return { validation, actual };
}
export function compareBundle(directory, example) {
  const { actual } = authenticateNativeBundle(directory);
  assert.deepEqual(actual, Object.keys(example.files_sha256).sort(), 'incomplete or extra bundle files');
  const replayFiles = actual.filter(f => /^(?:lib|footprints)\//.test(f) || /^(?:board\.kicad_(?:pcb|sch|pro)|sym-lib-table|fp-lib-table|bom\.(?:json|csv)|configuration\.json|electrical\.json)$/.test(f) || /^preview\/(?:board|pcb)\.svg$/.test(f) || /^manufacturing\//.test(f) && f !== 'manufacturing/manifest.json' && !f.endsWith('.log'));
  assert.equal(replayFiles.filter(f => f.startsWith('manufacturing/')).length, 16);
  const compared = {};
  for (const file of replayFiles) {
    const original = fs.readFileSync(`${example.destination}/${file}`), current = fs.readFileSync(`${directory}/${file}`);
    const a = normalizeReplay(file, original), b = normalizeReplay(file, current);
    assert.deepEqual(a, b, `bundle differs from reviewed example: ${file}`);
    compared[file] = { current_raw_sha256: hashBytes(current), example_raw_sha256: hashBytes(original), normalized_fields: b.normalized_fields, compared_sha256: hashBytes(b.content) };
  }
  return { validation_sha256: hash(`${directory}/validation.json`), files_compared: compared, all_files_sha256: Object.fromEntries(actual.map(f => [f, hash(`${directory}/${f}`)])) };
}
export function assessDecision(spec, c, decision) {
  const dispositions = ['supported', 'clarify', 'unsupported'];
  const clauses = Array.isArray(decision?.clauses) ? decision.clauses : [];
  const completeClauses = clauses.length > 0 && clauses.length <= 32 && clauses.every(x => x && isDeepStrictEqual(Object.keys(x).sort(), ['disposition', 'reason', 'text']) && typeof x.text === 'string' && dispositions.includes(x.disposition) && typeof x.reason === 'string' && x.reason.trim()) && clauses.map(x => x.text).join('') === c.prompt;
  const derived = clauses.some(x => x?.disposition === 'unsupported') ? 'unsupported' : clauses.some(x => x?.disposition === 'clarify') ? 'clarify' : 'supported';
  const configurationMatches = c.expected_disposition === 'supported' ? isDeepStrictEqual(decision?.configuration, expectedConfig(spec, c)) : decision?.configuration === null;
  const structural = decision !== null && typeof decision === 'object' && isDeepStrictEqual(Object.keys(decision).sort(), ['clauses', 'configuration', 'disposition', 'message']) && typeof decision.message === 'string' && decision.message.trim().length > 0;
  return { disposition_matches: decision?.disposition === c.expected_disposition, configuration_matches: configurationMatches, complete_original_clauses: Boolean(completeClauses), clause_dispositions_consistent: derived === decision?.disposition, automatic_checks_pass: Boolean(structural && completeClauses && configurationMatches && derived === decision?.disposition && decision.disposition === c.expected_disposition), semantic_review: 'pending-explicit-source-bound-review' };
}
export function checkLedger(ledger, completedEntries = []) {
  assert.equal(ledger.version, 2);
  assert.equal(ledger.goal, policy.goal);
  assert.equal(ledger.max_requests, policy.max_requests);
  assert.equal(ledger.max_micro_usd, policy.max_micro_usd);
  assert.ok(!ledger.halt_reason, 'ledger halted');
  assert.ok(Array.isArray(ledger.entries) && ledger.entries.length <= policy.max_requests);
  assert.ok(ledger.entries.length * 50_000 <= policy.max_micro_usd);
  for (const [i, entry] of ledger.entries.entries()) {
    assert.equal(entry.index, i + 1);
    assert.equal(entry.model, model);
    assert.equal(entry.reserve_micro_usd, 50_000);
    assert.ok(['reserved_unknown_outcome', 'completed', 'failed_or_unknown'].includes(entry.status));
  }
  assert.ok(ledger.entries.length >= completedEntries.length, 'ledger history shortened');
  assert.deepEqual(ledger.entries.slice(0, completedEntries.length), completedEntries, 'prior reservations changed');
  return ledger.entries;
}
export function checkApproval(approval, runtimeHash, freezeHash) {
  assert.equal(approval.status, 'explicit-user-approved');
  assert.equal(approval.evaluation_id, evaluationID);
  assert.equal(approval.max_physical_requests, 14);
  assert.equal(approval.max_micro_usd, 1_000_000);
  assert.equal(approval.existing_key_only, true);
  assert.equal(approval.runtime_sha256, runtimeHash);
  assert.equal(approval.freeze_sha256, freezeHash);
  assert.ok(typeof approval.user_message === 'string' && approval.user_message.trim(), 'actual user approval quote required');
  assert.ok(typeof approval.recorded_utc === 'string' && Number.isFinite(Date.parse(approval.recorded_utc)));
}
export function batchMetrics(spec, records) {
  const finished = records.filter(r => r.state === 'finished');
  const times = records.filter(r => spec.cases.find(c => c.id === r.id)?.expected_disposition === 'supported').map(r => r.wall_seconds);
  const allTimesKnown = times.length === 5 && times.every(t => Number.isFinite(t) && t >= 0);
  const sorted = allTimesKnown ? [...times].sort((a, b) => a - b) : [];
  return { planned_cases: 14, attempted_cases: records.length, recorded_outcomes: finished.length, raw_automatic_passes: finished.filter(r => r.raw?.automatic_checks_pass).length, admitted_automatic_passes: finished.filter(r => r.admitted?.automatic_checks_pass && r.output_checks_pass).length, supported_attempts: times.length, median_all_supported_seconds: allTimesKnown ? sorted[2] : null, maximum_all_supported_seconds: allTimesKnown ? sorted[4] : null, timing_target_met: allTimesKnown && sorted[2] < 60 && sorted[4] < 120, acceptance: 'not-established-until-semantic-review-and-complete-evidence' };
}
