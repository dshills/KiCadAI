// Authenticate two complete offline checkpoints and compare native/BOM/config
// bytes plus timestamp-normalized preview/manufacturing bytes, in memory only.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import { normalizeReplay } from './replay-normalization.mjs';

const [leftPath, rightPath, output] = process.argv.slice(2);
for (const file of [leftPath, rightPath, output]) if (!/^specs\/board-family-v2\/evidence\/[a-z0-9-]+\.json$/.test(file || '')) throw new Error('expected three v2 evidence JSON paths');
if (leftPath === rightPath || fs.existsSync(output)) throw new Error('distinct checkpoints and a new output are required');
const hashBytes = data => crypto.createHash('sha256').update(data).digest('hex');
const hash = file => hashBytes(fs.readFileSync(file));
const read = file => JSON.parse(fs.readFileSync(file, 'utf8'));
const left = read(leftPath), right = read(rightPath);
assert.deepEqual(left.source_sha256, right.source_sha256, 'checkpoints must use the same production/test sources');
assert.equal(left.binary_sha256, right.binary_sha256, 'checkpoints must use the same binary');
assert.equal(left.prior_live_ledger_sha256, right.prior_live_ledger_sha256);
for (const [file, sha] of Object.entries(right.source_sha256)) assert.equal(hash(file), sha, `current source differs: ${file}`);
assert.equal(hash('.cache/board-family-v1/live-ledger.json'), right.prior_live_ledger_sha256);
const ids = ['sht31-standard', 'sht31-fast', 'bmp280-standard', 'bmp280-fast', 'bmp280-low_current'];
assert.deepEqual(left.cases.map(c => c.id), ids);
assert.deepEqual(right.cases.map(c => c.id), ids);
function inventory(dir, prefix = '') {
  const found = [];
  for (const entry of fs.readdirSync(path.join(dir, prefix), { withFileTypes: true })) {
    const file = path.join(prefix, entry.name);
    if (entry.isSymbolicLink()) throw new Error(`unexpected symlink: ${file}`);
    if (entry.isDirectory()) found.push(...inventory(dir, file));
    else found.push(file);
  }
  return found.sort();
}
function authenticate(c) {
  if (!/^\.cache\/board-family-v2\/integration-[0-9]+-(?:sht31|bmp280)-(?:standard|fast|low_current)$/.test(c.directory)) throw new Error('invalid checkpoint directory');
  assert.deepEqual(read(`${c.directory}/validation.json`), c.validation);
  assert.equal(c.validation.passed, true);
  assert.equal(c.validation.checks.length, 14);
  assert.ok(c.validation.checks.every(x => x.passed));
  assert.equal(hash(`${c.directory}/manufacturing/manifest.json`), c.manufacturing_manifest_sha256);
  const manifest = read(`${c.directory}/manufacturing/manifest.json`);
  for (const [file, sha] of Object.entries({ ...c.validation.native_sha256, ...Object.fromEntries(Object.entries(manifest.files_sha256).map(([f, h]) => [`manufacturing/${f}`, h])) })) {
    if (file.includes('..') || path.isAbsolute(file)) throw new Error('invalid artifact path');
    assert.equal(hash(`${c.directory}/${file}`), sha, `changed artifact: ${c.id}/${file}`);
  }
  assert.equal(manifest.source_pcb_sha256, c.validation.native_sha256['board.kicad_pcb']);
  const files = inventory(c.directory).filter(f => /^(?:lib|footprints)\//.test(f) || /^(?:board\.kicad_(?:pcb|sch|pro)|sym-lib-table|fp-lib-table|bom\.(?:json|csv)|configuration\.json|electrical\.json)$/.test(f) || /^preview\/(?:board|pcb)\.svg$/.test(f) || /^manufacturing\//.test(f) && f !== 'manufacturing/manifest.json' && !f.endsWith('.log'));
  assert.equal(files.filter(f => f.startsWith('manufacturing/')).length, 16, 'expected all manufacturing deliverables');
  return files;
}
const cases = [];
for (const [i, id] of ids.entries()) {
  const a = left.cases[i], b = right.cases[i];
  assert.notEqual(a.directory, b.directory);
  const files = authenticate(a);
  assert.deepEqual(files, authenticate(b));
  const compared = {};
  for (const file of files) {
    const rawA = fs.readFileSync(`${a.directory}/${file}`), rawB = fs.readFileSync(`${b.directory}/${file}`);
    const normA = normalizeReplay(file, rawA), normB = normalizeReplay(file, rawB);
    assert.deepEqual(normA, normB, `replay differs: ${id}/${file}`);
    compared[file] = { left_raw_sha256: hashBytes(rawA), right_raw_sha256: hashBytes(rawB), raw_equal: rawA.equals(rawB), compared_sha256: hashBytes(normA.content), normalized_fields: normA.normalized_fields };
  }
  cases.push({ id, left_directory: a.directory, right_directory: b.directory, files: compared });
}
const result = { status: 'offline-deterministic-replay-pass-not-final-acceptance', created_utc: new Date().toISOString(), checkpoints_sha256: { [leftPath]: hash(leftPath), [rightPath]: hash(rightPath) }, source_sha256: right.source_sha256, binary_sha256: right.binary_sha256, recipes_sha256: Object.fromEntries(['compare-replay.mjs', 'replay-normalization.mjs', 'replay-normalization.test.mjs'].map(n => [`specs/board-family-v2/development/${n}`, hash(`specs/board-family-v2/development/${n}`)])), cases, exclusions: ['KiCad per-user .kicad_prl state is not a design deliverable.', 'Run logs, ERC/DRC reports and validation/manifest envelopes are authenticated or independently passed per run, not compared byte-for-byte; they contain per-run paths, times or raw timestamp-bound hashes.'], normalization_policy: 'Only the explicit native timestamp fields listed per file are normalized in memory. Geometry, all other metadata, coordinates, units, layer identity and BOM values remain significant. Original files are not written.' };
fs.writeFileSync(output, JSON.stringify(result, null, 2) + '\n', { flag: 'wx' });
console.log(JSON.stringify({ output, cases: cases.length, compared_files: cases.reduce((n, c) => n + Object.keys(c.files).length, 0) }));
