// Mechanical capture of an openly engineered, KiCad-saved development reference.
// Does not repair or alter any native content. Invoke from the repository root.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';

const source = '.cache/board-family-v2/sht31-canonical-01';
const destination = 'internal/boardfamily/reference-sht31';
const manifestPath = 'specs/board-family-v2/development/reference-import.json';
const pinned = {
  'board.kicad_pcb': '35c1a93888c9411c7bd564af9471738fbd0955742a0782b8ff38bc0e9697eb15',
  'board.kicad_sch': '22d23f5aeb63f88b08e2f7a385c299e7e85061e13d1b0602d283f6164708e867',
  'board.kicad_pro': 'bd3a8d8fa0bde57fe7b5562661c145881257ae596cb9ec6b8aba295f2779505e',
};
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
if (fs.existsSync(destination) || fs.existsSync(manifestPath)) throw new Error('capture destination already exists');
for (const [file, expected] of Object.entries(pinned)) {
  if (sha(fs.readFileSync(path.join(source, file))) !== expected) throw new Error(`source identity changed: ${file}`);
}
const erc = JSON.parse(fs.readFileSync(path.join(source, 'erc.json')));
const drc = JSON.parse(fs.readFileSync(path.join(source, 'drc.json')));
if (!erc.sheets?.length || erc.sheets.some(s => !Array.isArray(s.violations) || s.violations.length)) throw new Error('ERC incomplete or failing');
for (const key of ['violations', 'unconnected_items', 'schematic_parity']) {
  if (!Array.isArray(drc[key]) || drc[key].length) throw new Error(`DRC ${key} incomplete or failing`);
}
const files = [...Object.keys(pinned), 'sym-lib-table', 'fp-lib-table'];
function collect(dir) {
  for (const entry of fs.readdirSync(path.join(source, dir), { withFileTypes: true })) {
    const rel = `${dir}/${entry.name}`;
    if (entry.isSymbolicLink()) throw new Error(`symlink: ${rel}`);
    if (entry.isDirectory()) collect(rel);
    else if (entry.isFile()) files.push(rel);
  }
}
collect('lib'); collect('footprints'); files.sort();
const identities = {};
let patch = '*** Begin Patch\n';
function add(file, bytes) {
  const text = bytes.toString('utf8');
  if (!text.endsWith('\n') || !Buffer.from(text).equals(bytes)) throw new Error(`not newline-terminated UTF-8: ${file}`);
  patch += `*** Add File: ${file}\n${text.slice(0, -1).split('\n').map(line => '+' + line).join('\n')}\n`;
}
for (const file of files) {
  const bytes = fs.readFileSync(path.join(source, file));
  identities[file] = sha(bytes);
  add(`${destination}/${file}`, bytes);
}
add(manifestPath, Buffer.from(JSON.stringify({ source, destination, status: 'native-development-reference-not-final-acceptance', engineer_commit: '741271c0', files_sha256: identities }, null, 2) + '\n'));
patch += '*** End Patch\n';
const result = spawnSync('apply_patch', [], { input: patch, encoding: 'utf8', maxBuffer: 1024 * 1024 });
if (result.status !== 0) throw new Error(result.stderr || result.stdout || 'apply_patch failed');
for (const [file, expected] of Object.entries(identities)) {
  if (sha(fs.readFileSync(path.join(destination, file))) !== expected) throw new Error(`capture differs: ${file}`);
}
console.log(JSON.stringify({ copied_files: files.length, byte_identical: true, destination }));
