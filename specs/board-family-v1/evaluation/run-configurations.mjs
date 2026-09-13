// Execute the predeclared offline acceptance cases. Never repairs generated files.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import os from 'node:os';
import {spawnSync} from 'node:child_process';
const root=process.cwd();const out=path.resolve(process.argv[2]);if(fs.existsSync(out))throw Error('output must be new');fs.mkdirSync(out,{recursive:true});
const source=path.join(root,'specs/board-family-v1/evaluation/configurations.json');const spec=JSON.parse(fs.readFileSync(source));
const sha=b=>crypto.createHash('sha256').update(b).digest('hex');
const bin=path.join(root,'.cache/board-family-v1/kicadai-board-family');const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const summary={started_utc:new Date().toISOString(),spec_sha256:sha(fs.readFileSync(source)),binary_sha256:sha(fs.readFileSync(bin)),machine:{os:os.platform(),release:os.release(),architecture:os.arch(),cpu:os.cpus()[0]?.model,cpu_count:os.cpus().length},cases:[],replays:[],api_requests:0};
summary.runner_sha256=sha(fs.readFileSync(new URL(import.meta.url)));
summary.base_commit=spawnSync('git',['rev-parse','HEAD'],{env,encoding:'utf8'}).stdout.trim();
summary.source_sha256={};
function snapshot(dir){for(const entry of fs.readdirSync(path.join(root,dir),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name))){const p=path.join(dir,entry.name);if(entry.isDirectory())snapshot(p);else if(entry.isFile())summary.source_sha256[p]=sha(fs.readFileSync(path.join(root,p)));else throw Error('nonregular source entry')}}
snapshot('internal/boardfamily');snapshot('cmd/kicadai-board-family');
summary.source_context='Hashes cover the new production packages and embedded assets; unchanged shared packages are identified by base_commit. This is local integrity/provenance evidence, not a third-party signature.';
const artifacts=dir=>{
 const files={};const visit=(p,rel='')=>{for(const n of fs.readdirSync(p).sort()){const full=path.join(p,n),r=path.join(rel,n);if(fs.statSync(full).isDirectory()){if(['lib','footprints'].includes(r)||r.startsWith('lib'+path.sep)||r.startsWith('footprints'+path.sep))visit(full,r)}else if(/\.kicad_(sch|pcb|pro|sym|mod)$/.test(n)||['sym-lib-table','fp-lib-table','configuration.json','electrical.json','bom.json','bom.csv'].includes(n))files[r]=sha(fs.readFileSync(full))}};visit(dir);return files;
};
function run(c,id){const input=path.join(out,id+'.json');const {id:ignored,...options}=c;fs.writeFileSync(input,JSON.stringify({...spec.defaults,...options},null,2)+'\n');const dir=path.join(out,id);const args=['--config',input,'--output',dir,'--kicad-cli','/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli'];const start=performance.now();const r=spawnSync(bin,args,{env,encoding:'utf8',timeout:600000,maxBuffer:2000000});fs.writeFileSync(path.join(out,id+'.stdout.log'),r.stdout||'');fs.writeFileSync(path.join(out,id+'.stderr.log'),r.stderr||'');let result;try{result=JSON.parse(r.stdout)}catch{};const record={id,exit_code:r.status,signal:r.signal,wall_seconds:(performance.now()-start)/1000,result,artifacts:r.status===0?artifacts(dir):null};console.log(JSON.stringify({id,passed:result?.passed===true,seconds:record.wall_seconds}));return record}
for(const c of spec.cases)summary.cases.push(run(c,c.id));
for(const id of spec.replay_cases){const c=spec.cases.find(c=>c.id===id);const r=run(c,id+'-replay');r.matches_original=JSON.stringify(r.artifacts)===JSON.stringify(summary.cases.find(c=>c.id===id).artifacts)&&r.artifacts!==null;summary.replays.push(r)}
const times=summary.cases.map(c=>c.wall_seconds).sort((a,b)=>a-b);summary.metrics={supported_passes:summary.cases.filter(c=>c.exit_code===0&&c.result?.passed).length,supported_total:summary.cases.length,median_seconds:(times[4]+times[5])/2,max_seconds:Math.max(...times),deterministic_replays:summary.replays.filter(r=>r.matches_original).length};summary.passed=summary.metrics.supported_passes===10&&summary.metrics.deterministic_replays===3&&summary.metrics.median_seconds<300&&summary.metrics.max_seconds<600;summary.finished_utc=new Date().toISOString();fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify(summary,null,2)+'\n');console.log(JSON.stringify(summary.metrics));process.exitCode=summary.passed?0:1;
