// Read-only artifact authentication plus a new, never-overwritten checkpoint.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

const prefix = process.argv[2];
const output = process.argv[3];
if (!/^\.cache\/board-family-v2\/integration-[0-9]+$/.test(prefix || '') || !/^specs\/board-family-v2\/evidence\/[a-z0-9-]+\.json$/.test(output || '')) throw new Error('usage: record-integration.mjs .cache/board-family-v2/integration-NN specs/board-family-v2/evidence/NAME.json');
if (fs.existsSync(output)) throw new Error('checkpoint already exists');
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const read = file => JSON.parse(fs.readFileSync(file, 'utf8'));
const expectedChecks = ['electrical_contract', 'reference_and_bom_integrity', 'pcb_writer_and_connectivity', 'schematic_writer', 'native_parse_render', 'kicad_version', 'kicad_erc', 'kicad_strict_drc_and_parity', 'kicad_roundtrip_kicad_sch', 'kicad_roundtrip_kicad_pcb', 'schematic_preview', 'pcb_preview', 'manufacturing_exports', 'input_immutability'];
const cases = [];
for (const id of ['sht31-standard', 'sht31-fast', 'bmp280-standard', 'bmp280-fast', 'bmp280-low_current']) {
  const dir = `${prefix}-${id}`;
  const validation = read(`${dir}/validation.json`);
  if (!validation.passed || validation.kicad_version !== '10.0.3' || JSON.stringify(validation.checks.map(c => c.name)) !== JSON.stringify(expectedChecks) || validation.checks.some(c => !c.passed)) throw new Error(`incomplete/failing validation: ${id}`);
  for (const [file, expected] of Object.entries(validation.native_sha256)) if (hash(`${dir}/${file}`) !== expected) throw new Error(`native changed: ${id}/${file}`);
  const manufacturing = read(`${dir}/manufacturing/manifest.json`);
  if (manufacturing.source_pcb_sha256 !== validation.native_sha256['board.kicad_pcb']) throw new Error(`export source mismatch: ${id}`);
  for (const [file, expected] of Object.entries(manufacturing.files_sha256)) {
    if (file.includes('..') || path.isAbsolute(file) || hash(`${dir}/manufacturing/${file}`) !== expected) throw new Error(`export changed: ${id}/${file}`);
  }
  cases.push({ id, directory: dir, configuration: read(`${dir}/configuration.json`), validation, manufacturing_manifest_sha256: hash(`${dir}/manufacturing/manifest.json`), export_file_count: Object.keys(manufacturing.files_sha256).length });
}
const historical = new Map();
for (const group of ['acceptance', 'guardrails', 'integration', 'live', 'offline']) {
  const manifest = read(`specs/board-family-v1/evidence/${group}/manifest.json`);
  for (const [file, expected] of Object.entries(manifest.files)) {
    if (historical.has(file) && historical.get(file) !== expected) throw new Error(`conflicting historical identity: ${file}`);
    if (hash(file) !== expected) throw new Error(`historical evidence changed: ${file}`);
    historical.set(file, expected);
  }
}
const source = {};
for (const directory of ['internal/boardfamily', 'cmd/kicadai-board-family']) {
  for (const entry of fs.readdirSync(directory)) if (entry.endsWith('.go')) source[`${directory}/${entry}`] = hash(`${directory}/${entry}`);
}
const importPath = 'specs/board-family-v2/development/reference-assembly-canonical-import.json';
const imported = read(importPath);
for (const [file, expected] of Object.entries(imported.files_sha256)) if (hash(`${imported.destination}/${file}`) !== expected) throw new Error(`SHT31 reference changed: ${file}`);
fs.mkdirSync(path.dirname(output), { recursive: true });
const result = {
  status: 'offline-integration-checkpoint-not-final-acceptance',
  created_utc: new Date().toISOString(),
  notices: ['All cases used explicit --config mode; no live model evaluation is represented.', 'Historical publication bytes remain unchanged; historical current-production-source identity assertions are version-bound and do not qualify successor code.', 'Export checks do not replace visual/manufacturing review or physical bring-up.'],
  source_sha256: source,
  binary_sha256: hash('.cache/board-family-v2/kicadai-board-family'),
  reference_import_path: importPath,
  reference_import_sha256: hash(importPath),
  prior_live_ledger_sha256: hash('.cache/board-family-v1/live-ledger.json'),
  historical_publication_files_verified: historical.size,
  cases,
};
fs.writeFileSync(output, JSON.stringify(result, null, 2) + '\n', { flag: 'wx' });
console.log(JSON.stringify({ output, boards_passed: cases.length, checks_per_board: expectedChecks.length, historical_files_verified: historical.size }));
