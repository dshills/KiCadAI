// Offline successor qualification. No provider credentials or live runner.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {compareBundle, authenticateExamples} from '../evaluation/acceptance-lib.mjs';

const base = 'specs/board-family-v2/typed-intent-02';
const out = process.argv[2];
assert.ok(process.argv.length === 3 && /^\.cache\/board-family-v2\/typed-intent-02-run-[0-9]+$/.test(out || ''), 'provide a new scoped cache directory');
assert.ok(!fs.existsSync(out), 'never overwrite verification evidence');
fs.mkdirSync(out, {recursive: false});
const root = process.cwd();
const hash = f => crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const read = f => JSON.parse(fs.readFileSync(f, 'utf8'));
const toolchain = path.join(root, '.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64');
const env = {...process.env, PATH: `${toolchain}/bin:${process.env.PATH}`, GOROOT: toolchain,
  GOTOOLCHAIN: 'local', GOENV: 'off', GOWORK: 'off', GOFLAGS: '', GOEXPERIMENT: '', GOMAXPROCS: '4',
  GOCACHE: `${root}/.cache/go/build`, GOMODCACHE: `${root}/.cache/go/mod`, GOPROXY: 'off', GOSUMDB: 'off',
  GOLANGCI_LINT_CACHE: `${root}/.cache/golangci-lint`};
for (const key of ['OPENAI_API_KEY', 'ANTHROPIC_API_KEY', 'GEMINI_API_KEY', 'GOOGLE_API_KEY', 'KICADAI_LIVE_PROVIDER_TESTS']) delete env[key];
const sourceFiles = ['go.mod', 'go.sum', '.github/workflows/ci.yml', `${base}/README.md`, `${base}/verify.mjs`,
  ...['internal/boardfamily', 'internal/aiprovider', 'cmd/kicadai-board-family'].flatMap(dir =>
    fs.readdirSync(dir).filter(f => f.endsWith('.go')).map(f => `${dir}/${f}`))].sort();
const sources = Object.fromEntries(sourceFiles.map(f => [f, hash(f)]));
const historicFiles = ['specs/board-family-v2/evaluation/runtime-01.json', 'specs/board-family-v2/evaluation/freeze-01.json',
  'specs/board-family-v2/evidence/final-01/manifest.json', 'specs/board-family-v2/offline-admission-01/publication.json',
  '.cache/board-family-v1/live-ledger.json', '.cache/board-family-v2/evaluation-final-01-ledger.json',
  '.cache/board-family-v2/evaluation-final-01/state.json', '.cache/board-family-v2/runtime-01/kicadai-board-family',
  `${base}/usability-01/results.json`];
const history = Object.fromEntries(historicFiles.map(f => [f, hash(f)]));
const commands = [];
async function run(name, command, args, timeoutMS = 180000) {
  const log = `${out}/${name}.log`, fd = fs.openSync(log, 'wx'), started = Date.now();
  console.log(`start ${name}`);
  const result = await new Promise(resolve => {
    const child = spawn(command, args, {env, stdio: ['ignore', fd, fd], detached: true});
    let timedOut = false, spawnError = null;
    const timer = setTimeout(() => {timedOut = true; try {process.kill(-child.pid, 'SIGKILL');} catch {}}, timeoutMS);
    child.on('error', e => {spawnError = e.message;});
    child.on('close', (exitCode, signal) => {clearTimeout(timer); resolve({exitCode, signal, timedOut, spawnError});});
  });
  fs.closeSync(fd);
  const item = {name, command, args, timeout_ms: timeoutMS, ...result,
    seconds: (Date.now() - started) / 1000, log: path.basename(log), log_sha256: hash(log)};
  commands.push(item);
  console.log(JSON.stringify(item));
  assert.ok(result.exitCode === 0 && !result.signal && !result.timedOut && !result.spawnError, `failed ${name}; see ${log}`);
}
const startedUTC = new Date().toISOString(), go = `${toolchain}/bin/go`;
await run('historical-before', process.execPath, ['specs/board-family-v2/evaluation/audit-final.mjs', '--check']);
await run('prior-correction', process.execPath, ['specs/board-family-v2/offline-admission-01/check.mjs']);
await run('intent-tests', go, ['test', '-short', '-count=1', '-v', '-coverprofile', `${out}/intent.cover`, './internal/boardfamily', './cmd/kicadai-board-family']);
await run('intent-race', go, ['test', '-short', '-race', '-count=1', './internal/boardfamily', './cmd/kicadai-board-family']);
await run('intent-fuzz', go, ['test', './internal/boardfamily', '-run', '^$', '-fuzz', '^FuzzDecodeIntentNeverExposesInvalidConfig$', '-fuzztime=15s', '-parallel=2']);
await run('full-short-regression', go, ['test', '-short', '-p=1', '-timeout', '20m', './...'], 1260000);
await run('full-lint', '/Users/dshills/Development/Go/bin/golangci-lint', ['run', '--timeout=5m', './...'], 330000);
await run('node-safeguards', process.execPath, ['--test', 'specs/board-family-v2/evaluation/acceptance.test.mjs', 'specs/board-family-v2/development/replay-normalization.test.mjs']);
const binary = `${out}/kicadai-board-family`;
await run('build', go, ['build', '-o', binary, './cmd/kicadai-board-family']);
await run('export-contract', `${root}/${binary}`, ['--export-live-contract', `${out}/contract.json`]);
const contract = read(`${out}/contract.json`);
assert.equal(contract.admission_version, 'typed-requirements-02');
assert.equal(contract.schema_name, 'board_family_requirements_v2');
assert.equal(contract.model, 'gpt-4.1-mini-2025-04-14');
assert.equal(contract.max_output_tokens, 1600);
assert.notDeepEqual(contract, read('specs/board-family-v2/evaluation/LIVE_CONTRACT-01.json'));
const {receipt: examples} = authenticateExamples(), smoke = [];
for (const id of ['bmp280-standard', 'sht31-standard']) {
  const example = examples.cases.find(c => c.id === id);
  assert.ok(example);
  const directory = `${out}/${id}`;
  await run(`native-${id}`, `${root}/${binary}`, ['--config', `${example.destination}/configuration.json`, '--output', directory, '--kicad-cli', '/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli']);
  const comparison = compareBundle(directory, example), validation = read(`${directory}/validation.json`);
  smoke.push({id, directory, checks_passed: validation.checks.length, validation, comparison});
}
await run('historical-after', process.execPath, ['specs/board-family-v2/evaluation/audit-final.mjs', '--check']);
assert.deepEqual(Object.fromEntries(sourceFiles.map(f => [f, hash(f)])), sources, 'source changed during qualification');
assert.deepEqual(Object.fromEntries(historicFiles.map(f => [f, hash(f)])), history, 'historical evidence changed');
const verbose = fs.readFileSync(`${out}/intent-tests.log`, 'utf8');
const wordingPasses = (verbose.match(/--- PASS: TestIntentOrdinaryWordingMetamorphic\/[^/]+\/[^/]+\/[0-9]+ /g) || []).length;
const syntheticPasses = (verbose.match(/--- PASS: TestIntentFrozenPromptsSyntheticFacts\//g) || []).length;
const legacyPasses = (verbose.match(/--- PASS: TestRecordedV2OfflineAdmission\//g) || []).length;
assert.equal(wordingPasses, 25); assert.equal(syntheticPasses, 14); assert.equal(legacyPasses, 14);
const receipt = {status: 'offline-typed-intent-pass-not-live-acceptance', admission_version: 'typed-requirements-02',
  started_utc: startedUTC, finished_utc: new Date().toISOString(), new_api_requests: 0, new_api_spend_usd: 0,
  provider_credentials_removed: true, source_sha256: sources, historical_bytes_unchanged_sha256: history,
  binary_path: binary, binary_sha256: hash(binary), successor_contract_sha256: hash(`${out}/contract.json`),
  synthetic_wording_passes: wordingPasses, synthetic_frozen_prompt_passes: syntheticPasses,
  legacy_seen_response_passes: legacyPasses, verbose_pass_records_including_parents: (verbose.match(/--- PASS:/g) || []).length,
  coverage_sha256: hash(`${out}/intent.cover`), commands, native_smoke: smoke,
  limitations: ['Implementing-agent synthetic extractions, not unseen provider responses or a measured reliability/speed improvement.',
    'The model still interprets semantics; quotes and numeric validation do not prove complete requirement understanding.',
    'Native smokes use explicit configuration, not live AI; unchanged five-profile qualification is reused.',
    'Hashes authenticate local recorded bytes, not a provider signature or an independent reviewer.',
    'Historical raw 4/14 and application 8/14 live scores remain unchanged; both old budgets remain exhausted.']};
fs.writeFileSync(`${out}/receipt.json`, JSON.stringify(receipt, null, 2) + '\n', {flag: 'wx'});
console.log(JSON.stringify({receipt: `${out}/receipt.json`, status: receipt.status, wording_passes: wordingPasses,
  native_cases: smoke.length, native_comparisons: smoke.reduce((n, c) => n + Object.keys(c.comparison.files_compared).length, 0)}));
