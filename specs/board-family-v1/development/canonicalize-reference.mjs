// Development-time native save: preserves its input and writes a new reference.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {spawnSync} from 'node:child_process';
const [src,dst]=process.argv.slice(2).map(x=>path.resolve(x));
if(!src||!dst||fs.existsSync(dst))throw Error('require source and NEW output');
const drc=JSON.parse(fs.readFileSync(path.join(src,'drc.json')));
if(['violations','unconnected_items'].some(k=>!Array.isArray(drc[k])||drc[k].length))throw Error('source must have complete non-shorted routing');
fs.mkdirSync(dst,{recursive:true});
for(const n of ['board.kicad_sch','board.kicad_pcb','board.kicad_pro','sym-lib-table','fp-lib-table','lib','footprints'])fs.cpSync(path.join(src,n),path.join(dst,n),{recursive:true,errorOnExist:true,force:false});
const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const receipt={source:src,created_at:new Date().toISOString(),qualification:'unqualified development copy: revalidate after native save',input_schematic_parity:drc.schematic_parity,files:{}};
for(const [kind,ext] of [['sch','kicad_sch'],['pcb','kicad_pcb']]){
 const file=path.join(dst,'board.'+ext);
 const hash=()=>crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
 const before=hash();const r=spawnSync('/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli',[kind,'upgrade','--force',file],{env,encoding:'utf8',timeout:120000});
 fs.writeFileSync(path.join(dst,kind+'-upgrade.log'),(r.stdout||'')+'\n'+(r.stderr||''));
 if(r.status!==0)throw Error(kind+' upgrade failed');
 receipt.files[ext]={before,after:hash()};
}
fs.writeFileSync(path.join(dst,'canonicalization.json'),JSON.stringify(receipt,null,2)+'\n');
console.log('Native reference saved in '+dst);
