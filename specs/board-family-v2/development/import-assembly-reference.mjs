// Import only a new, already checked development reference. Historical canonical
// inputs, import receipts and evaluated outputs are never overwritten.
import fs from 'node:fs';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';
import assert from 'node:assert/strict';

const revision = process.argv[2] || '03';
assert.ok(['03', '04'].includes(revision));
const source = `.cache/board-family-v2/sht31-assembly-${revision}`;
const destination = 'internal/boardfamily/reference-sht31';
const output = revision === '03' ? 'specs/board-family-v2/development/reference-assembly-import.json' : 'specs/board-family-v2/development/reference-assembly-canonical-import.json';
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const read = file => JSON.parse(fs.readFileSync(file));
assert.equal(fs.existsSync(output), false, 'import receipt already exists');
assert.equal(hash(`${source}/engineering.json`), { '03': '605596d2ad2652fe842599f57ee2ed00600fc33ab2f829c74a94a425cfae6854', '04': 'aa60f1eeedb89941abc5763d1eb3e8917ff24d5c246a6b1ebd0d4df8abaeb199' }[revision]);
const engineering = read(`${source}/engineering.json`);
const previous = read('specs/board-family-v2/development/reference-import.json');
const priorAssembly = revision === '04' ? read('specs/board-family-v2/development/reference-assembly-import.json') : {files_sha256:{}};
const failed = read('specs/board-family-v2/development/reference-assembly-import-attempt-01.json');
assert.equal(failed.status, 'failed-import-byte-verification-not-accepted');
assert.equal(hash('specs/board-family-v2/development/reference-import.json'), engineering.source_manifest_sha256);
assert.deepEqual(previous.files_sha256, engineering.source_files_sha256);
for (const [file, expected] of Object.entries(previous.files_sha256)) {
  assert.equal(hash(`${previous.source}/${file}`), expected, `historical canonical input changed: ${file}`);
  // The recorded failed import left already-correct single-newline bytes. Admit
  // only the original or the exact freshly qualified successor, no other edits.
  assert.ok([expected, engineering.files_sha256[file], priorAssembly.files_sha256[file]].includes(hash(`${destination}/${file}`)), `current reference has unaccounted changes: ${file}`);
}
const erc = read(`${source}/erc.json`), drc = read(`${source}/drc.json`);
assert.ok(erc.sheets.length);
for (const sheet of erc.sheets) assert.deepEqual(sheet.violations, []);
for (const key of ['violations', 'unconnected_items', 'schematic_parity']) assert.deepEqual(drc[key], []);
let patch = '*** Begin Patch\n';
let changed = 0;
for (const [file, expected] of Object.entries(engineering.files_sha256)) {
  assert.ok(!file.includes('..') && !file.startsWith('/'));
  const src = `${source}/${file}`, dst = `${destination}/${file}`;
  assert.equal(hash(src), expected);
  if (fs.existsSync(dst) && !(file in previous.files_sha256)) assert.ok([expected, failed.files_sha256[file], priorAssembly.files_sha256[file]].includes(hash(dst)), `unexpected existing new asset: ${file}`);
  if (fs.existsSync(dst) && hash(dst) === expected) continue;
  changed++;
  if (!fs.existsSync(dst)) {
    const text = fs.readFileSync(src, 'utf8'); assert.ok(text.endsWith('\n'));
    patch += `*** Add File: ${dst}\n${text.slice(0,-1).split('\n').map(l=>'+'+l).join('\n')}\n`;
  } else {
    const diff = spawnSync('git', ['diff', '--no-index', '--no-ext-diff', '--', dst, src], {encoding:'utf8', maxBuffer:4*1024*1024});
    assert.equal(diff.status, 1, diff.stderr);
    const hunks = diff.stdout.slice(diff.stdout.indexOf('@@')).replace(/^@@[^\n]*@@[^\n]*$/gm, '@@');
    assert.ok(hunks.startsWith('@@\n') && !hunks.includes('No newline at end of file'));
    patch += `*** Update File: ${dst}\n${hunks}`;
  }
}
const record = {source, destination, status:'native-development-assembly-reference-not-final-acceptance', previous_import_sha256:engineering.source_manifest_sha256, engineering_sha256:hash(`${source}/engineering.json`), recipe_sha256:hash('specs/board-family-v2/development/engineer-assembly/main.go'), tests_sha256:hash('specs/board-family-v2/development/engineer-assembly/main_test.go'), native_checks_sha256:{erc:hash(`${source}/erc.json`),drc:hash(`${source}/drc.json`)}, files_sha256:engineering.files_sha256, notes:['Custom U2 footprint identity distinguishes the manufacturer-directed mask/paste revision from the original KiCad footprint.','Original footprint remains byte-identical as provenance and the development recipe test input.','No generated evaluation output was edited. Software review does not qualify the physical assembly process.']};
patch += `*** Add File: ${output}\n${(JSON.stringify(record,null,2)+'\n').slice(0,-1).split('\n').map(l=>'+'+l).join('\n')}\n*** End Patch\n`;
const result = spawnSync('apply_patch',[],{input:patch,encoding:'utf8',maxBuffer:1024*1024});
assert.equal(result.status,0,result.stderr||result.stdout);
for (const [file,expected] of Object.entries(engineering.files_sha256)) assert.equal(hash(`${destination}/${file}`),expected);
console.log(JSON.stringify({output,changed_reference_files:changed,verified_reference_files:Object.keys(engineering.files_sha256).length}));
