// Reproducible, read-only visual-review preparation. Never edits the CAD/export
// inputs; copied renderer inputs and all derived images live in a new cache dir.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';

const [source, output] = process.argv.slice(2);
if (!/^\.cache\/board-family-v2\/integration-[0-9]+-(?:sht31|bmp280)-(?:standard|fast|low_current)$/.test(source || '') || !/^\.cache\/board-family-v2\/visual-review-[a-z0-9-]+$/.test(output || '')) throw new Error('usage: render-manufacturing.mjs .cache/board-family-v2/integration-NN-FAMILY-PROFILE .cache/board-family-v2/visual-review-NAME');
if (fs.existsSync(output)) throw new Error('review directory must be new');
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const read = file => JSON.parse(fs.readFileSync(file, 'utf8'));
const manifest = read(`${source}/manufacturing/manifest.json`);
const native = read(`${source}/validation.json`);
if (!native.passed || native.checks.length !== 14 || native.checks.some(c => !c.passed)) throw new Error('source did not pass all native/export gates');
const bound = { ...Object.fromEntries(Object.entries(manifest.files_sha256).map(([f, h]) => [`manufacturing/${f}`, h])), ...native.native_sha256 };
for (const file of ['manufacturing/manifest.json', 'validation.json', 'preview/board.svg', 'preview/pcb.svg']) bound[file] = hash(`${source}/${file}`);
function authenticate() {
  for (const [file, expected] of Object.entries(bound)) {
    if (file.includes('..') || path.isAbsolute(file) || hash(`${source}/${file}`) !== expected) throw new Error(`source changed: ${file}`);
  }
  if (manifest.source_pcb_sha256 !== bound['board.kicad_pcb']) throw new Error('manifest source differs');
}
authenticate();
fs.mkdirSync(output);
fs.mkdirSync(`${output}/input`);
const env = { ...process.env };
for (const key of ['OPENAI_API_KEY', 'ANTHROPIC_API_KEY', 'GEMINI_API_KEY', 'GOOGLE_API_KEY', 'KICADAI_LIVE_PROVIDER_TESTS']) delete env[key];
const gerbonara = path.resolve('.cache/board-family-v2/gerber-review-venv/bin/gerbonara');
const rsvg = '/opt/homebrew/bin/rsvg-convert';
const commands = [];
function run(executable, args, name) {
  const result = spawnSync(executable, args, { env, encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024 });
  fs.writeFileSync(`${output}/${name}.stdout`, result.stdout || '', { flag: 'wx' });
  fs.writeFileSync(`${output}/${name}.stderr`, result.stderr || '', { flag: 'wx' });
  commands.push({ executable, args, exit_status: result.status, stderr: `${name}.stderr` });
  if (result.error || result.status !== 0) throw new Error(`${name} failed: ${result.error || result.stderr}`);
  return result.stdout;
}
const version = run(gerbonara, ['--version'], 'gerbonara-version').trim();
if (!version.endsWith('1.6.3')) throw new Error('review requires Gerbonara 1.6.3');
const rasterVersion = run(rsvg, ['--version'], 'rsvg-version').trim();
run(path.resolve('.cache/board-family-v2/gerber-review-venv/bin/pip'), ['freeze'], 'review-dependencies');
for (const file of Object.keys(bound).filter(f => /\.(?:gbr|drl)$/.test(f))) fs.copyFileSync(`${source}/${file}`, `${output}/input/${path.basename(file)}`, fs.constants.COPYFILE_EXCL);
const meta = JSON.parse(run(gerbonara, ['meta', '--warnings', 'default', `${output}/input`], 'layer-metadata'));
// Gerbonara's metadata omits empty drill layers. Parse those independently and
// record the omission explicitly; neither family currently has any NPTH hits.
const emptyDrills = [];
const populatedDrills = [];
for (const name of ['board-PTH.drl', 'board-NPTH.drl']) {
  const input = `${output}/input/${name}`;
  if (/^X/m.test(fs.readFileSync(input, 'utf8'))) populatedDrills.push(name);
  else {
    emptyDrills.push(name);
    const parsed = JSON.parse(run(path.resolve('.cache/board-family-v2/gerber-review-venv/bin/python'), ['-c', 'from gerbonara.excellon import ExcellonFile; import sys,json; f=ExcellonFile.open(sys.argv[1]); print(json.dumps({"objects":len(f.objects)}))', input], `${name}-empty-parse`));
    if (parsed.objects !== 0) throw new Error('unexpected objects in empty drill file');
  }
}
if (Object.values(meta.graphical_layers).reduce((n, group) => n + Object.keys(group).length, 0) !== 9 || JSON.stringify(meta.drill_layers.map(d => path.basename(d.path)).sort()) !== JSON.stringify(populatedDrills.sort())) throw new Error('renderer omitted graphical or populated drill layers');
const inputMap = {
  'board-F_Cu\\.gbr': 'top copper', 'board-B_Cu\\.gbr': 'bottom copper',
  'board-F_Mask\\.gbr': 'top mask', 'board-B_Mask\\.gbr': 'bottom mask',
  'board-F_Paste\\.gbr': 'top paste', 'board-B_Paste\\.gbr': 'bottom paste',
  'board-F_Silkscreen\\.gbr': 'top silk', 'board-B_Silkscreen\\.gbr': 'bottom silk',
  'board-Edge_Cuts\\.gbr': 'mechanical outline',
  'board-PTH\\.drl': 'drill plated', 'board-NPTH\\.drl': 'drill nonplated',
};
fs.writeFileSync(`${output}/input-map.json`, JSON.stringify(inputMap, null, 2) + '\n', { flag: 'wx' });
// Unlike the pretty-render CLI documentation, 1.6.3's raw stack renderer needs
// "side use" color keys. Missing keys omit layers entirely, even with defaults.
const colors = { 'top copper': '#dd8d43', 'bottom copper': '#dd8d43', 'drill pth': '#ffffff', 'drill npth': '#ffffff', 'drill unknown': '#ffffff', 'mechanical outline': '#9fa8b3' };
fs.writeFileSync(`${output}/copper-colors.json`, JSON.stringify(colors, null, 2) + '\n', { flag: 'wx' });
const region = source.includes('sht31') ? '96,-21,108,-10' : '73,-30,90,-16';
const renders = [
  ['top', '--pretty', '--top', '-1,-81,121,1'],
  ['bottom', '--pretty', '--bottom', '-1,-81,121,1'],
  ['top-copper-drills', '--no-filters', '--top', '-1,-81,121,1'],
  ['bottom-copper-drills', '--no-filters', '--bottom', '-1,-81,121,1'],
  ['sensor-copper-drills', '--no-filters', '--top', region],
];
for (const [name, mode, side, bounds] of renders) {
  const args = ['render', '--warnings', 'default', '--no-builtin-name-rules', '--input-map', `${output}/input-map.json`, mode, side, '--force-bounds', bounds];
  if (mode === '--no-filters') args.push('--colorscheme', `${output}/copper-colors.json`);
  args.push(`${output}/input`, `${output}/${name}.svg`);
  run(gerbonara, args, name);
}
const layerRenders = [['sensor-paste', 'top paste', region], ['sensor-mask', 'top mask', region], ['top-paste', 'top paste', '-1,-81,121,1'], ['top-mask', 'top mask', '-1,-81,121,1'], ['bottom-mask', 'bottom mask', '-1,-81,121,1'], ['outline', 'mechanical outline', '-1,-81,121,1']];
for (const [name, layer, bounds] of layerRenders) {
  fs.writeFileSync(`${output}/${name}-colors.json`, JSON.stringify({ [layer]: '#dd8d43' }) + '\n', { flag: 'wx' });
  run(gerbonara, ['render', '--warnings', 'default', '--no-builtin-name-rules', '--input-map', `${output}/input-map.json`, '--no-filters', '--colorscheme', `${output}/${name}-colors.json`, '--force-bounds', bounds, `${output}/input`, `${output}/${name}.svg`], name);
}
for (const name of [...renders, ...layerRenders].map(r => r[0])) {
  if (!/<(?:path|circle|rect|polygon)\b/.test(fs.readFileSync(`${output}/${name}.svg`, 'utf8'))) throw new Error(`empty renderer output: ${name}`);
}
for (const name of [...renders, ...layerRenders].map(r => r[0])) run(rsvg, ['--background-color', '#111827', '--width', '2400', '--output', `${output}/${name}.png`, `${output}/${name}.svg`], `${name}-png`);
for (const [name, input] of [['native-schematic', 'preview/board.svg'], ['native-pcb', 'preview/pcb.svg'], ['drill-map-pth', 'manufacturing/drill/board-PTH-drl_map.svg'], ['drill-map-npth', 'manufacturing/drill/board-NPTH-drl_map.svg']]) {
  run(rsvg, ['--background-color', '#ffffff', '--width', name === 'native-schematic' ? '4000' : '2400', '--output', `${output}/${name}.png`, `${source}/${input}`], name);
}
authenticate();
const files = {};
function collect(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) collect(full);
    else files[path.relative(output, full)] = hash(full);
  }
}
collect(output);
const receipt = { status: 'rendered-for-review-not-review-signoff', source, created_utc: new Date().toISOString(), recipe_sha256: hash(process.argv[1]), renderer: version, rasterizer: rasterVersion, empty_drill_files_parsed_separately: emptyDrills, source_sha256: bound, files_sha256: files, commands, limitations: ['Derived review images only; original native and manufacturing files are unchanged.', 'Metadata includes all nine Gerber and all populated drill layers; empty drills are parsed separately. Rendering uses an explicit eleven-file layer map. Warnings remain in stderr logs.', 'Gerbonara warns about KiCad G90 after the header and its automatic metadata labels drill plating unknown; the explicit renderer map and native-bound production drill checks supply the intended plating.', 'Visual inspection and assembly/electrical signoff are separate from rendering success.'] };
fs.writeFileSync(`${output}/receipt.json`, JSON.stringify(receipt, null, 2) + '\n', { flag: 'wx' });
console.log(JSON.stringify({ output, rendered: true, source_unchanged: true, warnings: commands.filter(c => fs.statSync(`${output}/${c.stderr}`).size).map(c => c.stderr) }));
