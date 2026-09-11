// Offline verification only. Authenticator subprocesses use the existing key
// solely for exact-secret scans; no provider client or network transport exists.
// Historical publication verification extracts archives into a fresh temp dir.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const historicalRepo = resolve(process.argv[2] || join(repo, '.cache/interface-v1-source-49953'));
const base = '49953eaa599612aaf8e60f31ab3e786f32ddc722';
const hash = b => createHash('sha256').update(b).digest('hex');
const git = (cwd, ...args) => execFileSync('git', args, {cwd, encoding: 'utf8'}).trim();
const run = (cwd, path, ...args) => JSON.parse(execFileSync(process.execPath, [path, ...args], {cwd, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024}));
assert.equal(git(historicalRepo, 'rev-parse', 'HEAD'), base, 'Wrong historical source');
assert.equal(git(historicalRepo, 'status', '--porcelain', '--untracked-files=no'), '', 'Historical source modified');
const protectedPaths = ['specs/ai-requirement-contract-integration/live-v1', 'specs/ai-requirement-contract-integration/publication-live-v1', 'specs/practical-sensor-controller-boards'];
assert.equal(git(repo, 'diff', base, '--name-only', '--', ...protectedPaths), '', 'Historical specification or publication changed');

const interfaceCheck = run(historicalRepo, 'specs/ai-requirement-contract-integration/publication-live-v1/verify-publication.mjs');
const practical = run(repo, 'specs/practical-sensor-controller-boards/publication-v2/authenticate.mjs');
const practicalPublication = run(repo, 'specs/practical-sensor-controller-boards/publication-v2/verify-publication.mjs', '--verify-local-archive');
const practicalAudits = run(repo, 'specs/practical-sensor-controller-boards/publication-v2/verify-audits.mjs');

const fixturePath = 'internal/behavioralintent/testdata/interface_v1_i04_counterexample.json';
const fixture = readFileSync(join(repo, fixturePath));
const original = readFileSync('/tmp/kicadai-ai-requirement-interface-v1/I04/initial/attempt-2.intent.json');
assert.deepEqual(fixture, Buffer.concat([original, Buffer.from('\n')]), 'Counterexample changed beyond its final LF');
assert.equal(hash(original), 'ea3ff80eaed18fec6316a56a61a47046285576d0b56b29de48ec445a87c94b02');

const binaries = [
  ['/tmp/kicadai-ai-requirement-eval-v1', '6d75691600d14771ae860480a08c77b441552debfa0ec608657ce986ae6a402f'],
  ['/tmp/kicadai-practical-board-eval-protocol-v2', '2628c1017a32837b69c230cf3690c8f4ba8f0089e1899967bde525cb2f370a94'],
];
for (const [path, expected] of binaries) assert.equal(hash(readFileSync(path)), expected, 'Sealed binary changed');

const secret = process.env.OPENAI_API_KEY;
assert(secret, 'Existing key is required only for a local exact-secret scan');
const trackedChanges = git(repo, 'diff', base, '--name-only', '--diff-filter=ACMR').split('\n').filter(Boolean);
const untracked = git(repo, 'ls-files', '--others', '--exclude-standard').split('\n').filter(Boolean);
const changed = [...new Set([...trackedChanges, ...untracked])].sort();
for (const path of changed) {
  const bytes = readFileSync(join(repo, path));
  assert(!bytes.includes(secret), 'Credential in changed file');
  assert(!/\bsk-[A-Za-z0-9_-]{20,}/.test(bytes.toString('utf8')), 'Key-shaped content in changed file');
}
console.log(JSON.stringify({
  schema: 'kicadai.offline-repair-preservation.v1', verified_utc: new Date().toISOString(),
  historical_source: base, historical_source_checkout: historicalRepo, historical_tracked_paths_unchanged: true,
  interface: interfaceCheck,
  practical: {raw_files: practical.inventory.file_count, raw_bytes: practical.inventory.total_bytes,
    exact_secret_scan: practical.inventory.exact_environment_credential_scan,
    campaign_complete: practical.authentication.campaign_complete,
    prior_inventories_reverified: practical.authentication.prior_inventories_reverified,
    publication: practicalPublication, audit_count: practicalAudits.audits.length, clauses: practicalAudits.total_clauses},
  fixture: {path: fixturePath, original_sha256: hash(original), repository_sha256: hash(fixture), change: 'one final LF only'},
  sealed_binaries_unchanged: binaries.map(([path, sha256]) => ({path, sha256})),
  changed_files_secret_scan: 'absent', changed_files_scanned: changed.length, provider_calls: 0,
  historical_gate_passed: false, new_live_evaluation_performed: false,
}, null, 2));
