// Post-freeze read-only audit of terminal recovery 2. No provider calls/writes.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
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
const auth = read(join(dir, 'recovery-2-authorization.json'));
const root = auth.recovery_root;
const freeze = read(join(dir, 'freeze.json'));
assert.equal(sha(readFileSync(join(dir, 'freeze.json'))), auth.original_freeze_sha256);
verify(repo, freeze.files);
verify(auth.execution_source.checkout, freeze.files);
const git = (...args) => execFileSync('git', args, { cwd: auth.execution_source.checkout, encoding: 'utf8' }).trim();
assert.equal(git('rev-parse', 'HEAD'), auth.execution_source.commit);
assert.equal(git('status', '--porcelain'), '');
assert.equal(git('diff', '--name-only', freeze.baseline_commit, '--', '.',
  ':(exclude)internal/practicalboardeval/**', ':(exclude)cmd/practical-board-eval/**',
  ':(exclude)specs/practical-sensor-controller-boards/**'), '');
for (const [path, hash, priorRoot] of [
  [auth.original_evidence_inventory, auth.original_evidence_inventory_sha256, auth.original_root],
  [auth.prior_recovery_evidence_inventory, auth.prior_recovery_evidence_inventory_sha256, auth.prior_recovery_root],
]) {
  const bytes = readFileSync(join(dir, path));
  assert.equal(sha(bytes), hash);
  assert.deepEqual(walk(priorRoot), JSON.parse(bytes).files);
}
assert.deepEqual(readFileSync(join(root, 'recovery-authorization.json')), readFileSync(join(dir, 'recovery-2-authorization.json')));
for (const receipt of auth.journal_policy.copied_prior_entries) {
  assert.deepEqual(readFileSync(join(root, 'request-journal', receipt)), readFileSync(join(auth.prior_recovery_root, 'request-journal', receipt)));
}
const journalFiles = ['001', '002', '003'].flatMap(n => [`${n}.reservation.json`, `${n}.usage.json`]);
assert.deepEqual(readdirSync(join(root, 'request-journal')).sort(), journalFiles);
const files = walk(root);
const caseFiles = read(join(root, 'baseline/P01/inventory.json'));
assert.deepEqual(walk(join(root, 'baseline/P01')).filter(f => f.path !== 'inventory.json'), caseFiles);
const start = read(join(root, 'baseline/campaign-start.json'));
const end = read(join(root, 'baseline/campaign-end.json'));
assert.equal(start.source_commit, auth.execution_source.commit);
assert.equal(start.freeze_sha256, auth.original_freeze_sha256);
assert.equal(start.binary_sha256, auth.execution_source.binary_sha256);
assert.equal(sha(readFileSync(auth.execution_source.binary)), start.binary_sha256);
assert(start.binary_build_metadata.includes('vcs.modified=false'));
assert.equal(end.stop_reason, 'ai_provider_timeout');
assert.equal(end.outcomes[0].id, 'P01');
assert.equal(end.outcomes[0].exit_code, 1);
assert(end.outcomes.slice(1).every(x => x.status === 'not_run' && x.reason === end.stop_reason));
const result = read(join(root, 'baseline/P01/result.json'));
assert.equal(result.provider_attempts, 1);
assert.equal(result.follow_up_attempts, 0);
assert.equal(result.replays_completed, 0);
const request = readFileSync(join(root, 'baseline/P01/http-003.request.json'));
const receipt = read(join(root, 'request-journal/003.reservation.json'));
assert.equal(receipt.number, 3);
assert.equal(receipt.request_bytes, request.length);
assert.equal(receipt.request_sha256, sha(request));
assert.deepEqual(request, readFileSync(join(auth.original_root, 'baseline/P01/http-001.request.json')));
const settings = JSON.parse(request);
assert.equal(settings.model, 'gpt-5.6-sol');
assert.equal(settings.max_output_tokens, 16384);
assert.equal(settings.stream, true);
assert.equal(settings.store, false);
assert.equal(settings.background, false);
assert.equal(settings.text.format.type, 'json_schema');
assert.equal(settings.text.format.strict, true);
assert(!settings.tools || settings.tools.length === 0);
const responseMetadata = read(join(root, 'baseline/P01/http-003.response-metadata.json'));
const response = readFileSync(join(root, 'baseline/P01/http-003.response.txt'));
assert.equal(responseMetadata.status, 200);
assert.equal(responseMetadata.retained_bytes, response.length);
const events = response.toString('utf8').replace(/\r\n/g, '\n').split('\n\n')
  .map(block => block.split('\n').filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n'))
  .filter(Boolean).map(data => JSON.parse(data));
assert(events.every((event, index) => event.sequence_number === index));
const eventCounts = {};
events.forEach(event => { eventCounts[event.type] = (eventCounts[event.type] || 0) + 1; });
assert(!events.some(event => ['response.completed', 'response.failed', 'response.incomplete', 'error'].includes(event.type)));
const envelopes = events.filter(event => event.response);
assert.equal(envelopes.length, 2);
assert(envelopes.every(event => event.response.model === settings.model && event.response.status === 'in_progress' && event.response.usage === null));
const outputText = events.filter(event => event.type === 'response.output_text.delta').map(event => event.delta).join('');
assert.throws(() => JSON.parse(outputText), SyntaxError);
const key = process.env.OPENAI_API_KEY;
assert(key, 'Existing credential required only for local exact-secret scan');
for (const file of files) {
  const bytes = readFileSync(join(root, file.path));
  assert(!bytes.includes(key), `Credential found in ${file.path}`);
  if (file.path.endsWith('.gz')) assert(!gunzipSync(bytes, { maxOutputLength: 2 ** 31 - 1 }).includes(key), `Credential found in decompressed ${file.path}`);
}
const now = new Date().toISOString();
const inventory = {
  schema: 'kicadai.practical-board-evidence-inventory.v1', evidence_root: root,
  source_commit: start.source_commit, freeze_sha256: start.freeze_sha256,
  phase: 'baseline', cohort: 'approved_recovery_2', status: 'interrupted_during_provider_stream',
  authenticated_utc: now,
  exact_environment_credential_scan: 'absent_from_all_retained_files_including_decompressed_library_index',
  file_count: files.length, total_bytes: files.reduce((sum, file) => sum + file.bytes, 0), files,
};
const audit = read(join(dir, 'recovery-1-audits.json'));
audit.review_utc = now;
audit.cohort = 'approved_recovery_2';
audit.prior_recovery_inventory = 'recovery-1-evidence-inventory.json';
audit.evidence_inventory = 'recovery-2-evidence-inventory.json';
audit.authorization = 'recovery-2-authorization.json';
audit.baseline_disposition = 'incomplete_provider_stream_request_timeout';
audit.authentication.case_retained_file_count = caseFiles.length;
audit.authentication.whole_retained_tree_sha256_inventory = files.length;
audit.authentication.prior_recovery_evidence_files_reverified = 28;
audit.authentication.stream_sequence_contiguous = true;
audit.authentication.http_response_metadata_and_partial_body_bound = true;
audit.manual_assistance.separate_recovery_approval_count = 2;
audit.provider.connection_attempts = 1;
audit.provider.cumulative_connection_attempts = 3;
audit.provider.api_response_received = true;
audit.provider.key_authentication = 'accepted_for_this_recorded_request';
audit.provider.model_access = 'gpt-5.6-sol_accepted_for_this_recorded_request';
const usage = ['001', '002', '003'].map(n => read(join(root, 'request-journal', `${n}.usage.json`)));
assert(usage.every(u => u.usage_available === false));
audit.provider.estimated_or_reserved_usd = usage[2].estimated_or_reserved_usd;
audit.provider.cumulative_estimated_or_reserved_usd = usage.reduce((sum, item) => sum + item.estimated_or_reserved_usd, 0);
audit.provider.remaining_estimated_or_reserved_usd = 50 - audit.provider.cumulative_estimated_or_reserved_usd;
audit.provider.remaining_baseline_requests = 33;
audit.provider.remaining_total_requests = 69;
audit.provider.http_status = responseMetadata.status;
audit.provider.complete_response_received = false;
audit.provider.complete_structured_requirement_received = false;
audit.provider.stream = {
  response_bytes: response.length, event_count: events.length, event_counts: eventCounts,
  first_sequence: events[0].sequence_number, last_sequence: events.at(-1).sequence_number,
  output_text_bytes: Buffer.byteLength(outputText), output_text_complete_json: false,
  terminal_event_received: false, final_usage_received: false,
};
const byPath = new Map(files.map(file => [file.path, file]));
const rebind = value => {
  if (!value || typeof value !== 'object') return;
  if (typeof value.file === 'string' && typeof value.sha256 === 'string') {
    assert(byPath.has(value.file), `Missing audit source: ${value.file}`);
    value.sha256 = byPath.get(value.file).sha256;
  }
  for (const child of Object.values(value)) rebind(child);
};
rebind(audit.cases);
const corpus = read(join(dir, 'corpus.json'));
const inputs = [...corpus.cases, ...corpus.paraphrases];
assert.deepEqual(audit.cases.map(x => x.case_id), inputs.map(x => x.id));
assert.deepEqual(end.outcomes.map(x => x.id), inputs.map(x => x.id));
audit.cases[0].first_failed_gate = 'provider_completion';
audit.cases[0].reason = 'HTTP 200 and partial output received; frozen two-minute client deadline canceled the stream before a terminal event or complete JSON requirement. Not an electrical or physical result.';
audit.cases[0].source_binding['http-003.response-metadata.json'] = {
  file: 'baseline/P01/http-003.response-metadata.json',
  sha256: byPath.get('baseline/P01/http-003.response-metadata.json').sha256,
};
audit.cases[0].source_binding['http-003.response.txt'] = {
  file: 'baseline/P01/http-003.response.txt', sha256: sha(response),
};
audit.cases.forEach((item, index) => {
  const input = inputs[index];
  const base = input.of ? corpus.cases.find(x => x.id === input.of) : input;
  const clauses = [...base.acceptance, ...(base.kind === 'positive' ? corpus.positive_common_acceptance : [])];
  assert.deepEqual(item.clauses.map(x => x.text), clauses);
  assert(item.clauses.every(x => x.disposition === 'not_run'));
  assert(item.gates.every(x => x.disposition === 'not_run'));
  assert.equal(item.disposition, index === 0 ? 'failed' : 'not_run');
});
assert.equal(audit.cases.reduce((n, x) => n + x.clauses.length, 0), 108);
assert.equal(audit.baseline_complete_passes, null);
assert.equal(audit.observed_complete_candidates, 0);
console.log(JSON.stringify({ inventory, audit }, null, 2));
