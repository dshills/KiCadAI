// Read-only receipt verification. --run-focused reruns local tests with provider
// credentials removed and module downloads disabled; it never makes API calls.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {readFileSync} from 'node:fs';
import {dirname, isAbsolute, join, relative, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const directory = dirname(fileURLToPath(import.meta.url));
const repo = resolve(directory, '../../..');
const receipt = JSON.parse(readFileSync(join(directory, 'verification.json'), 'utf8'));
assert.equal(receipt.schema, 'kicadai.endpoint-regulator-offline-verification.v1');
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
for (const entry of [...receipt.source_files, ...receipt.artifact_files]) {
  const path = resolve(repo, entry.path);
  const inside = relative(repo, path);
  assert(!isAbsolute(inside) && inside !== '..' && !inside.startsWith('../'), 'Receipt path escapes repository');
  const bytes = readFileSync(path);
  assert.equal(bytes.length, entry.bytes, `Size changed: ${entry.path}`);
  assert.equal(digest(bytes), entry.sha256, `Hash changed: ${entry.path}`);
}
const protectedPaths = [
  'specs/ai-requirement-contract-integration/live-v1',
  'specs/ai-requirement-contract-integration/publication-live-v1',
  'specs/ai-requirement-contract-integration/offline-repair-v1',
  'specs/practical-sensor-controller-boards',
  'internal/behavioralintent/testdata/interface_v1_i04_counterexample.json',
];
assert.equal(execFileSync('git', ['diff', receipt.starting_revision, '--name-only', '--', ...protectedPaths], {cwd: repo, encoding: 'utf8'}).trim(), '', 'Historical artifacts changed');

let focusedRerun = false;
if (process.argv.includes('--run-focused')) {
  const env = {...process.env};
  for (const name of receipt.environment.provider_keys_and_live_switch_unset) delete env[name];
  const goRoot = join(repo, receipt.environment.go_root_relative);
  Object.assign(env, {
    PATH: `${goRoot}/bin:${env.PATH || ''}`, GOROOT: goRoot, GOTOOLCHAIN: 'local',
    GOENV: 'off', GOWORK: 'off', GOFLAGS: '', GOEXPERIMENT: '', CGO_ENABLED: '1',
    GOMAXPROCS: '4', GOPROXY: 'off', GOSUMDB: 'off',
    GOCACHE: join(repo, '.cache/go/build'), GOMODCACHE: join(repo, '.cache/go/mod'),
  });
  execFileSync(join(goRoot, 'bin/go'), receipt.focused_go_args, {cwd: repo, env, stdio: 'inherit'});
  focusedRerun = true;
}
console.log(JSON.stringify({
  schema: 'kicadai.endpoint-regulator-receipt-check.v1', verified_utc: new Date().toISOString(),
  source_files: receipt.source_files.length, artifact_files: receipt.artifact_files.length,
  hashes_match: true, historical_tracked_paths_unchanged: true, focused_rerun: focusedRerun,
  provider_calls: 0, new_live_evaluation_performed: false, full_board_goal_achieved: false,
}, null, 2));
