// Offline-only build, tests, native replay and source/binary qualification.
// Failed preparation directories are retained. A new invocation needs a new
// directory; this recipe cannot overwrite an existing qualified runtime record.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { evaluationDirectory as E, evaluationID, hash, read, durableJSON, offlineEnvironment, inventory, authenticateFiles, authenticateFrozen, authenticateExamples, expectedConfig, exampleID, compareBundle } from './acceptance-lib.mjs';
import { executeCase } from './run-language.mjs';

const [out] = process.argv.slice(2);
assert.ok(process.argv.length === 3 && /^\.cache\/board-family-v2\/runtime-[0-9]+$/.test(out || ''), 'usage: prepare-runtime.mjs .cache/board-family-v2/runtime-NN');
const destination = `${E}/runtime-01.json`;
assert.equal(fs.existsSync(out), false, 'preparation output must be new');
assert.equal(fs.existsSync(destination), false, 'qualified runtime already exists; do not rewrite it');
const root = process.cwd();
const goroot = '.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64';
const go = path.resolve(goroot, 'bin/go');
const cli = '/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';
const binary = `${out}/kicadai-board-family`;
const env = { ...offlineEnvironment(), PATH: `${path.dirname(go)}:${process.env.PATH}`, GOROOT: path.resolve(goroot), GOTOOLCHAIN: 'local', GOENV: 'off', GOWORK: 'off', GOFLAGS: '', GOEXPERIMENT: '', GOMAXPROCS: '4', GOCACHE: path.resolve('.cache/go/build'), GOMODCACHE: path.resolve('.cache/go/mod'), GOPROXY: 'off', GOSUMDB: 'off', GOLANGCI_LINT_CACHE: path.resolve('.cache/golangci-lint') };
assert.ok(!env.OPENAI_API_KEY && !env.GEMINI_API_KEY && !env.KICADAI_LIVE_PROVIDER_TESTS);
const { spec } = authenticateFrozen();
const { receipt, historical_files_verified } = authenticateExamples();
// Ask the compiler for selected non-standard source/assembly/embed dependencies,
// including their test source. No inferred or hand-maintained source file list.
const fields = ['GoFiles', 'CgoFiles', 'CFiles', 'CXXFiles', 'MFiles', 'HFiles', 'FFiles', 'SFiles', 'SwigFiles', 'SwigCXXFiles', 'SysoFiles', 'EmbedFiles', 'TestGoFiles', 'XTestGoFiles', 'TestEmbedFiles', 'XTestEmbedFiles'];
const template = `{{if not .Standard}}{{.Dir}}${fields.map(f => `|{{join .${f} ","}}`).join('')}{{end}}`;
const listed = spawnSync(go, ['list', '-deps', '-f', template, './cmd/kicadai-board-family'], { env, encoding: 'utf8', timeout: 30_000, maxBuffer: 4_000_000 });
assert.equal(listed.status, 0, listed.stderr);
const files = new Set(['go.mod', 'go.sum', `${goroot}/bin/go`]);
for (const line of listed.stdout.trim().split('\n').filter(Boolean)) {
  const [directory, ...groups] = line.split('|');
  for (const name of groups.flatMap(g => g.split(',')).filter(Boolean)) {
    const relative = path.relative(root, path.join(directory, name));
    assert.ok(relative && !relative.startsWith('../') && !path.isAbsolute(relative), 'dependency outside bound workspace');
    files.add(relative);
  }
}
for (const name of ['acceptance-lib.mjs', 'acceptance.test.mjs', 'run-language.mjs', 'prepare-runtime.mjs', 'budget-01.json', 'freeze-01.json', 'cases-01.json', 'PROTOCOL-01.md', 'LIVE_CONTRACT-01.json']) files.add(`${E}/${name}`);
for (const name of ['replay-normalization.mjs', 'replay-normalization.test.mjs']) files.add(`specs/board-family-v2/development/${name}`);
const identities = Object.fromEntries([...files].sort().map(file => [file, hash(file)]));
fs.mkdirSync(out, { mode: 0o700 });
const commands = [], cases = [];
const preparation = { status: 'running-offline', started_utc: new Date().toISOString(), source_sha256: identities, commands, cases };
durableJSON(`${out}/preparation.json`, preparation, { exclusive: true });
async function command(id, executable, args) {
  const result = await executeCase(executable, args, env, `${out}/${id}`);
  commands.push({ id, command: executable, args, ...result });
  durableJSON(`${out}/preparation.json`, preparation);
  console.log(JSON.stringify({ id, exit_code: result.exit_code, seconds: result.wall_seconds, provider_keys_removed: true }));
  assert.equal(result.exit_code, 0, `offline ${id} failed; inspect retained logs`);
  return result;
}
try {
  await command('build', go, ['build', '-o', binary, './cmd/kicadai-board-family']);
  await command('go-tests', go, ['test', '-short', '-p=1', '-timeout', '20m', './...']);
  await command('lint', '/Users/dshills/Development/Go/bin/golangci-lint', ['run', '--timeout=5m']);
  await command('node-tests', process.execPath, ['--test', `${E}/acceptance.test.mjs`, 'specs/board-family-v2/development/replay-normalization.test.mjs']);
  const exported = `${out}/executed-contract.json`;
  await command('payload', path.resolve(binary), ['--export-live-contract', exported]);
  assert.equal(hash(exported), hash(`${E}/LIVE_CONTRACT-01.json`), 'runtime changed the frozen payload');
  for (const c of spec.cases.filter(c => c.expected_disposition === 'supported')) {
    const configuration = `${out}/${c.id}-configuration.json`, directory = `${out}/${c.id}`;
    durableJSON(configuration, expectedConfig(spec, c), { exclusive: true });
    await command(c.id, path.resolve(binary), ['--config', configuration, '--output', directory, '--kicad-cli', cli]);
    const example = receipt.cases.find(e => e.id === exampleID(c));
    const comparison = compareBundle(directory, example);
    cases.push({ id: c.id, example_id: example.id, directory, comparison });
  }
  authenticateFiles('.', identities);
  authenticateFrozen(); authenticateExamples();
  preparation.status = 'passed-offline-no-live-requests';
  preparation.finished_utc = new Date().toISOString();
  durableJSON(`${out}/preparation.json`, preparation);
  for (const file of inventory(out)) identities[`${out}/${file}`] = hash(`${out}/${file}`);
  const commit = spawnSync('git', ['rev-parse', 'HEAD'], { env, encoding: 'utf8', timeout: 10_000 });
  assert.equal(commit.status, 0);
  const result = { status: 'offline-qualified-live-not-authorized', evaluation_id: evaluationID, created_utc: new Date().toISOString(), source_commit: commit.stdout.trim(), source_notice: 'The working-source and embedded-file hashes below, not this pre-commit HEAD alone, identify the tested runtime. Standard library identity is represented by the pinned Go toolchain binary/version. Full dependency selection is retained in this recipe.', binary, kicad_cli: cli, kicad_cli_sha256: hash(cli), node_sha256: hash(process.execPath), freeze_sha256: hash(`${E}/freeze-01.json`), commands, cases, historical_files_verified, files_sha256: identities, live_requests: 0, semantic_acceptance: 'not-tested' };
  const contents = JSON.stringify(result, null, 2) + '\n';
  const patch = `*** Begin Patch\n*** Add File: ${destination}\n${contents.trimEnd().split('\n').map(l => '+' + l).join('\n')}\n*** End Patch\n`;
  const written = spawnSync('apply_patch', [], { env, input: patch, encoding: 'utf8', maxBuffer: 2_000_000 });
  assert.equal(written.status, 0, written.stderr);
  assert.deepEqual(read(destination), result);
  console.log(JSON.stringify({ status: result.status, output: destination, cases: cases.length, compared_files: cases.reduce((n, c) => n + Object.keys(c.comparison.files_compared).length, 0), bound_files: Object.keys(identities).length, live_requests: 0 }));
} catch (error) {
  preparation.status = 'failed-offline';
  preparation.error = String(error.message).slice(0, 2000);
  preparation.finished_utc = new Date().toISOString();
  durableJSON(`${out}/preparation.json`, preparation);
  throw error;
}
