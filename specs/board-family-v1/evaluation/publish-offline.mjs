// Compact, hash-verified local copies of existing evidence; never runs or repairs a board.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
const root=process.cwd();
const src=path.resolve(process.argv[2]);
const evidence=path.join(root,'specs/board-family-v1/evidence/offline');
const examples=path.join(root,'examples/board-family-v1');
if(fs.existsSync(evidence)||fs.existsSync(examples))throw Error('publication destinations must be new');
const hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const summary=JSON.parse(fs.readFileSync(path.join(src,'summary.json')));
if(summary.passed!==true||summary.cases.length!==10||summary.replays.length!==3)throw Error('acceptance incomplete');
for(const [file,digest] of Object.entries(summary.source_sha256))if(hash(path.join(root,file))!==digest)throw Error('source changed since acceptance: '+file);
for(const c of [...summary.cases,...summary.replays]){
 if(c.exit_code!==0||c.result?.passed!==true||!c.artifacts)throw Error('failed case: '+c.id);
 const dir=path.join(src,c.id);for(const [f,d]of Object.entries(c.artifacts))if(hash(path.join(dir,f))!==d)throw Error('artifact changed: '+c.id+'/'+f);
 const v=JSON.parse(fs.readFileSync(path.join(dir,'validation.json')));if(!v.passed||v.kicad_version!=='10.0.3'||v.checks.some(c=>!c.passed))throw Error('validation failed');
 const drc=JSON.parse(fs.readFileSync(path.join(dir,'drc.json')));for(const key of ['violations','unconnected_items','schematic_parity'])if(!Array.isArray(drc[key])||drc[key].length)throw Error('DRC incomplete');
 const erc=JSON.parse(fs.readFileSync(path.join(dir,'erc.json')));if(!erc.sheets?.length||erc.sheets.some(s=>!Array.isArray(s.violations)||s.violations.length))throw Error('ERC incomplete');
}
if(summary.replays.some(r=>r.matches_original!==true))throw Error('replay mismatch');
fs.mkdirSync(evidence,{recursive:true});fs.mkdirSync(examples,{recursive:true});
const copy=(a,b)=>{fs.mkdirSync(path.dirname(b),{recursive:true});fs.copyFileSync(a,b,fs.constants.COPYFILE_EXCL);if(hash(a)!==hash(b))throw Error('copy mismatch')};
copy(path.join(src,'summary.json'),path.join(evidence,'summary.json'));
for(const c of [...summary.cases,...summary.replays]){
 const dst=path.join(evidence,c.id);
 for(const f of ['configuration.json','electrical.json','validation.json','erc.json','drc.json','erc.log','drc.log','schematic-preview.log','pcb-preview.log'])if(fs.existsSync(path.join(src,c.id,f)))copy(path.join(src,c.id,f),path.join(dst,f));
 for(const f of ['stdout.log','stderr.log'])copy(path.join(src,c.id+'.'+f),path.join(dst,f));
}
for(const [profile,id]of [['standard','standard-boundaries'],['fast','fast-boundaries'],['low_current','low-current-boundaries']]){
 const record=summary.cases.find(c=>c.id===id);
 for(const f of Object.keys(record.artifacts))copy(path.join(src,id,f),path.join(examples,profile,f));
 for(const f of ['validation.json','erc.json','drc.json','erc.log','drc.log','schematic-preview.log','pcb-preview.log','preview/board.svg','preview/pcb.svg'])if(fs.existsSync(path.join(src,id,f)))copy(path.join(src,id,f),path.join(examples,profile,f));
}
const manifest={created_utc:new Date().toISOString(),source_run:src,source_summary_sha256:hash(path.join(src,'summary.json')),notice:'Local integrity evidence, not an independent signature. Reports preserve original execution paths. Example bytes are copied unchanged from three accepted cases.',files:{}};
function inventory(dir){for(const e of fs.readdirSync(dir,{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name))){const p=path.join(dir,e.name);if(e.isDirectory())inventory(p);else if(e.isFile())manifest.files[path.relative(root,p)]=hash(p);else throw Error('unexpected nonregular file')}}
inventory(evidence);inventory(examples);
fs.writeFileSync(path.join(evidence,'manifest.json'),JSON.stringify(manifest,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({evidence,examples,files:Object.keys(manifest.files).length,metrics:summary.metrics}));
