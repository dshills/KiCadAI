// Regenerate a prototype package from source; never flash, order or repair outputs.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { verifyPackage } from './verify-package.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const [cliArg, confirmationArg, kicadArg, pioArg, outputArg, gerbonaraArg, rsvgArg] = process.argv.slice(2);
if (process.argv.length !== 9) throw new Error('usage: node package.mjs CLI CONFIRMATION KICAD_CLI PLATFORMIO NEW_OUTPUT GERBONARA RSVG');
const [cli, confirmation, kicad, pio, output] = [cliArg, confirmationArg, kicadArg, pioArg, outputArg].map(p=>path.resolve(p));
const hash = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const json = p => JSON.parse(fs.readFileSync(p, 'utf8'));
const writeJSON = (p,v) => fs.writeFileSync(p, JSON.stringify(v,null,2)+'\n',{flag:'wx'});
const config = json(confirmation).specification?.configuration;
if (JSON.stringify(config) !== JSON.stringify(json(path.join(root,'examples/board-family-v2/sht31-standard/configuration.json'))))
  throw new Error('prototype package requires exact reviewed SHT31 standard configuration');
if (fs.existsSync(output)) throw new Error('output must be new');
// Strip provider credentials and unrelated process environment from child tools.
const env = Object.fromEntries(Object.entries(process.env).filter(([key])=>
  ['HOME','PATH','TMPDIR','GOTOOLCHAIN','GOPROXY','GOSUMDB','GOMODCACHE','GOCACHE'].includes(key) || key.startsWith('PLATFORMIO_')));
fs.mkdirSync(output,{recursive:true});
fs.mkdirSync(path.join(output,'verification'));
function run(program,args,name,timeout=180000) {
  const r=spawnSync(program,args,{cwd:root,env,encoding:'utf8',timeout,maxBuffer:16*1024*1024});
  fs.writeFileSync(path.join(output,'verification',name+'.log'),(r.stdout||'')+(r.stderr||''),{flag:'wx'});
  if(r.error || r.status!==0) throw new Error(`${name} failed; diagnostics retained, no release manifest`);
  return r.stdout.trim();
}
function copyFile(from,to) {
  if (!fs.lstatSync(from).isFile()) throw new Error('regular file required: '+from);
  fs.mkdirSync(path.dirname(to),{recursive:true});fs.copyFileSync(from,to,fs.constants.COPYFILE_EXCL);
}
function collect(dir,base=dir,result={}) {
  for(const name of fs.readdirSync(dir).sort()) {
    const p=path.join(dir,name),s=fs.lstatSync(p);
    if(s.isDirectory()) collect(p,base,result);
    else if(s.isFile()) result[path.relative(base,p)]=hash(p);
    else throw new Error('symlink/nonregular file forbidden: '+p);
  }
  return result;
}
const sourceDirs=['firmware/sht31-bringup/include','firmware/sht31-bringup/src','firmware/sht31-bringup/tests',
  'docs/prototypes/sht31','scripts/sht31-prototype'];
const sourceFiles=['firmware/sht31-bringup/platformio.ini','firmware/sht31-bringup/partitions.csv',
  'firmware/sht31-bringup/README.md',...sourceDirs.flatMap(d=>Object.keys(collect(path.join(root,d))).map(f=>`${d}/${f}`))];
const before=Object.fromEntries(sourceFiles.map(f=>[f,hash(path.join(root,f))]));
const pioVersion=run(pio,['--version'],'platformio-version');
if(pioVersion !== 'PlatformIO Core, version 6.1.19') throw new Error('review requires PlatformIO 6.1.19');
run('c++',['-std=c++11','-Wall','-Wextra','-Werror','-fsanitize=address,undefined',
  '-I','firmware/sht31-bringup/include','firmware/sht31-bringup/tests/test_sht31.cpp',
  '-o',path.join(output,'verification','test-sht31')],'host-build');
run(path.join(output,'verification','test-sht31'),[],'host-tests');
run(process.execPath,['--test','scripts/sht31-prototype/verify-run.test.mjs','scripts/sht31-prototype/verify-package.test.mjs'],'verifier-tests');
// Force recompilation so cached object files cannot hide a source change.
run(pio,['run','-d','firmware/sht31-bringup','-e','sht31-standard','-t','clean'],'firmware-clean');
run(pio,['run','-d','firmware/sht31-bringup','-e','sht31-standard'],'firmware-build',600000);
run(cli,['spec','build','--input',confirmation,'--output',path.join(output,'board'),'--kicad-cli',kicad],'native-build',600000);
const board=path.join(output,'board'),validation=json(path.join(board,'validation.json'));
if(!validation.passed || validation.checks.length!==14 || validation.checks.some(c=>!c.passed)) throw new Error('native gates failed');
const bm=json(path.join(board,'bundle-manifest.json'));
for(const [f,h] of Object.entries(bm.files_sha256)) if(hash(path.join(board,f))!==h) throw new Error('board hash mismatch');
for(const f of ['board.kicad_pro','board.kicad_sch','board.kicad_pcb','bom.json','configuration.json'])
  if(hash(path.join(board,f))!==hash(path.join(root,'examples/board-family-v2/sht31-standard',f))) throw new Error('reviewed reference differs: '+f);
run(process.execPath,['scripts/sht31-prototype/render.mjs',board,path.join(output,'inspection'),path.resolve(gerbonaraArg),path.resolve(rsvgArg),kicad],'inspection-render',180000);
for(const f of ['firmware.bin','firmware.elf','firmware.map','bootloader.bin','partitions.bin'])
  copyFile(path.join(root,'firmware/sht31-bringup/.pio/build/sht31-standard',f),path.join(output,'firmware',f));
for(const f of sourceFiles) {
  if(hash(path.join(root,f))!==before[f]) throw new Error('source changed during packaging: '+f);
  copyFile(path.join(root,f),path.join(output,'source',f));
}
copyFile(confirmation,path.join(output,'confirmation.json'));
copyFile(path.join(root,'docs/prototypes/sht31/PACKAGE.md'),path.join(output,'README.md'));
writeJSON(path.join(output,'firmware','flash-layout.json'),{chip:'esp32',flash_mode:'dio',flash_freq:'40m',flash_size:'4MB',
  files:{'0x1000':'bootloader.bin','0x8000':'partitions.bin','0x10000':'firmware.bin'},
  automatic_reset:false,hardware_flashing_performed:false});
writeJSON(path.join(output,'package-manifest.json'),{
  version:'sht31-prototype-package-1',status:'software-verified-awaiting-human-release',release_authorized:false,
  physical_validation:'not_performed',manual_artifact_repair:false,
  reviewed_configuration:config,source_files_sha256:before,
  cli_sha256:hash(cli),kicad_cli_sha256:hash(kicad),platformio_version:pioVersion,
  native_gates:14,files_sha256:collect(output),
  release_holds_document:'source/docs/prototypes/sht31/REVIEW.md',
  bench_procedure:'source/docs/prototypes/sht31/BENCH.md'});
try { verifyPackage(output); }
catch (error) { fs.unlinkSync(path.join(output,'package-manifest.json')); throw error; }
console.log(JSON.stringify({output,status:'software-verified-awaiting-human-release',physical_validation:'not_performed'}));
