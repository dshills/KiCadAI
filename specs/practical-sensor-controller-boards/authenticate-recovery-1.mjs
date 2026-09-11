// Post-freeze, read-only authentication of the terminal recovery-1 evidence.
// This does not invoke the provider, change evaluation criteria, or write files.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { lstatSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gunzipSync } from 'node:zlib';

const dir = dirname(fileURLToPath(import.meta.url));
const repo = resolve(dir, '../..');
const read = path => JSON.parse(readFileSync(path, 'utf8'));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const seal = (root, path) => {
  const absolute = join(root, path);
  assert(lstatSync(absolute).isFile(), `Not a regular file: ${path}`);
  const bytes = readFileSync(absolute);
  return { path, bytes: bytes.length, sha256: sha(bytes) };
};
const walk = (root, relative = '') => readdirSync(join(root, relative)).sort().flatMap(name => {
  const path = relative ? `${relative}/${name}` : name;
  const stat = lstatSync(join(root, path));
  assert(!stat.isSymbolicLink(), `Symlink: ${path}`);
  return stat.isDirectory() ? walk(root, path) : [seal(root, path)];
});
const verify = (root, files) => files.forEach(file => assert.deepEqual(seal(root, file.path), file));
const authorization = read(join(dir, 'recovery-1-authorization.json'));
const root = authorization.recovery_root;
const oldRoot = authorization.original_root;
const freeze = read(join(dir, 'freeze.json'));
assert.equal(sha(readFileSync(join(dir, 'freeze.json'))), authorization.original_freeze_sha256);
verify(repo, freeze.files);
const oldInventoryBytes = readFileSync(join(dir, 'baseline-evidence-inventory.json'));
assert.equal(sha(oldInventoryBytes), authorization.original_evidence_inventory_sha256);
const oldInventory = JSON.parse(oldInventoryBytes);
assert.deepEqual(walk(oldRoot), oldInventory.files);
assert.deepEqual(readFileSync(join(root, 'recovery-authorization.json')), readFileSync(join(dir, 'recovery-1-authorization.json')));
for (const receipt of authorization.journal_policy.copied_original_entries) {
  assert.deepEqual(readFileSync(join(root, 'request-journal', receipt)), readFileSync(join(oldRoot, 'request-journal', receipt)));
}
assert.deepEqual(readdirSync(join(root, 'request-journal')).sort(), [
  '001.reservation.json', '001.usage.json', '002.reservation.json', '002.usage.json',
]);
const files = walk(root);
const caseFiles = read(join(root, 'baseline/P01/inventory.json'));
assert.deepEqual(walk(join(root, 'baseline/P01')).filter(f => f.path !== 'inventory.json'), caseFiles);
const start = read(join(root, 'baseline/campaign-start.json'));
const end = read(join(root, 'baseline/campaign-end.json'));
assert.equal(start.source_commit, '4341df799c25c811cd05e80f5d860c009ea50585');
assert.equal(start.freeze_sha256, authorization.original_freeze_sha256);
assert.equal(start.binary_sha256, seal('/tmp', 'kicadai-practical-board-eval-recovery-1').sha256);
assert(start.binary_build_metadata.includes('vcs.modified=false'));
assert.equal(end.stop_reason, 'ai_provider_timeout');
assert.equal(end.outcomes[0].id, 'P01');
assert.equal(end.outcomes[0].exit_code, 1);
assert(end.outcomes.slice(1).every(x => x.status === 'not_run' && x.reason === end.stop_reason));
assert.equal(read(join(root, 'baseline/P01/result.json')).replays_completed, 0);
const request = readFileSync(join(root, 'baseline/P01/http-002.request.json'));
const receipt = read(join(root, 'request-journal/002.reservation.json'));
assert.equal(receipt.number, 2);
assert.equal(receipt.request_bytes, request.length);
assert.equal(receipt.request_sha256, sha(request));
assert.deepEqual(request, readFileSync(join(oldRoot, 'baseline/P01/http-001.request.json')));
const settings = JSON.parse(request);
assert.equal(settings.model, 'gpt-5.6-sol');
assert.equal(settings.max_output_tokens, 16384);
assert.equal(settings.stream, true);
assert.equal(settings.store, false);
assert.equal(settings.background, false);
assert.equal(settings.text.format.type, 'json_schema');
assert.equal(settings.text.format.strict, true);
assert(!settings.tools || settings.tools.length === 0);
const key = process.env.OPENAI_API_KEY;
assert(key, 'Existing credential required only for local exact-secret scan');
for (const file of files) {
  const bytes = readFileSync(join(root, file.path));
  assert(!bytes.includes(key), `Credential found in ${file.path}`);
  if (file.path.endsWith('.gz')) assert(!gunzipSync(bytes, { maxOutputLength: 2 ** 31 - 1 }).includes(key), `Credential found in decompressed ${file.path}`);
}
const now = new Date().toISOString();
const inventory = {
  schema: 'kicadai.practical-board-evidence-inventory.v1',
  evidence_root: root, source_commit: start.source_commit,
  freeze_sha256: start.freeze_sha256, phase: 'baseline', cohort: 'approved_recovery_1',
  status: 'interrupted_before_provider_response', authenticated_utc: now,
  exact_environment_credential_scan: 'absent_from_all_retained_files_including_decompressed_library_index',
  file_count: files.length, total_bytes: files.reduce((sum, file) => sum + file.bytes, 0), files,
};
const audit = read(join(dir, 'baseline-audits.json'));
audit.review_utc = now;
audit.cohort = 'approved_recovery_1';
audit.original_baseline_inventory = 'baseline-evidence-inventory.json';
audit.evidence_inventory = 'recovery-1-evidence-inventory.json';
audit.authorization = 'recovery-1-authorization.json';
audit.original_audit_limitation = 'Original interrupted audit used six nonpositive category labels and collapsed each two-clause clarification into one label (106 rows). This recovery audit expands all frozen acceptance text (108 rows); original evidence and criteria remain unchanged.';
audit.authentication.whole_retained_tree_sha256_inventory = files.length;
audit.authentication.original_evidence_files_reverified = oldInventory.file_count;
audit.authentication.original_receipts_copied_byte_for_byte = true;
audit.authentication.frozen_files_reverified = freeze.files.length;
audit.manual_assistance.separate_recovery_approval_count = 1;
audit.provider.connection_attempts = 1;
audit.provider.cumulative_connection_attempts = 2;
audit.provider.carried_reservation_is_not_new_request = true;
const usage1 = read(join(root, 'request-journal/001.usage.json'));
const usage2 = read(join(root, 'request-journal/002.usage.json'));
assert.equal(usage1.usage_available, false);
assert.equal(usage2.usage_available, false);
audit.provider.estimated_or_reserved_usd = usage2.estimated_or_reserved_usd;
audit.provider.cumulative_estimated_or_reserved_usd = usage1.estimated_or_reserved_usd + usage2.estimated_or_reserved_usd;
audit.provider.remaining_estimated_or_reserved_usd = 50 - audit.provider.cumulative_estimated_or_reserved_usd;
audit.provider.remaining_baseline_requests = 34;
audit.provider.remaining_total_requests = 70;
const byPath = new Map(files.map(file => [file.path, file]));
function rebind(value) {
  if (!value || typeof value !== 'object') return;
  if (typeof value.file === 'string' && typeof value.sha256 === 'string') {
    assert(byPath.has(value.file), `Missing audit source: ${value.file}`);
    value.sha256 = byPath.get(value.file).sha256;
  }
  for (const child of Object.values(value)) rebind(child);
}
rebind(audit.cases);
const corpus = read(join(dir, 'corpus.json'));
const inputs = [...corpus.cases, ...corpus.paraphrases];
assert.deepEqual(audit.cases.map(x => x.case_id), inputs.map(x => x.id));
assert.deepEqual(end.outcomes.map(x => x.id), inputs.map(x => x.id));
audit.cases.forEach((item, index) => {
  const input = inputs[index];
  const base = input.of ? corpus.cases.find(x => x.id === input.of) : input;
  const clauses = [...base.acceptance, ...(base.kind === 'positive' ? corpus.positive_common_acceptance : [])];
  // The first interrupted audit used category labels for its six nonpositive
  // clauses. Keep that audit immutable; bind this audit to the full frozen text.
  if (base.kind !== 'positive') {
    assert.equal(item.clauses.length, 1);
    const original = item.clauses[0];
    item.clauses = clauses.map((text, i) => ({
      ...structuredClone(original), number: i + 1, text,
      original_audit_clause_label: original.text,
    }));
  }
  assert.deepEqual(item.clauses.map(x => x.text), clauses);
  assert(item.clauses.every(x => x.disposition === 'not_run'));
  assert(item.gates.every(x => x.disposition === 'not_run'));
  assert.equal(item.disposition, index === 0 ? 'failed' : 'not_run');
});
assert.equal(audit.cases.reduce((n, x) => n + x.clauses.length, 0), 108);
assert.equal(audit.baseline_complete_passes, null);
assert.equal(audit.observed_complete_candidates, 0);
console.log(JSON.stringify({ inventory, audit }, null, 2));
