// Frozen-input, offline, serial development. There are no provider calls or retries.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createHash} from 'node:crypto';
import {execFileSync,spawn} from 'node:child_process';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {evidenceBytes,treeRSSFromPS} from '../practical-sensor-controller-boards/evidence-utils.mjs';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../..');
const [manifestArgument]=process.argv.slice(2);assert.equal(process.argv.length,3);
const sha=b=>createHash('sha256').update(b).digest('hex');
const manifestPath=resolve(manifestArgument),manifestBytes=fs.readFileSync(manifestPath),manifest=JSON.parse(manifestBytes);
assert.equal(manifest.schema,'kicadai.completion-development.v1');assert([1,2,3].includes(manifest.batch));
assert.deepEqual(manifest.cases.map(c=>c.id),['P01','P02','P03','P04','P05','P06']);
assert.equal(manifest.provider_calls,0);assert.equal(manifest.case_wall_ms,1200000);assert.equal(manifest.batch_wall_ms,7200000);
assert.equal(manifest.rss_bytes,16*1024**3);assert.equal(manifest.evidence_bytes,10*1024**3);assert.equal(manifest.runs_per_case.length,2);
assert.equal(manifest.runner_sha256,sha(fs.readFileSync(fileURLToPath(import.meta.url))));
const removed=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_OPENAI_LIVE_TEST'];
const env={...process.env,GOMAXPROCS:'4'};for(const k of removed)delete env[k];
for(const k of Object.keys(env))if(k.startsWith('KICADAI_'))delete env[k];
Object.assign(env,{KICADAI_KICAD_CLI:'/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli',KICADAI_SYMBOLS_ROOT:'/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols',KICADAI_FOOTPRINTS_ROOT:'/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints'});
const git=(cwd,...a)=>execFileSync('git',a,{cwd,env,encoding:'utf8'}).trim();
const fileRecord=p=>{const b=fs.readFileSync(p);return {bytes:b.length,sha256:sha(b)};};
const verify=()=>{
 assert.equal(sha(fs.readFileSync(manifestPath)),sha(manifestBytes));
 assert.equal(sha(fs.readFileSync(fileURLToPath(import.meta.url))),manifest.runner_sha256);
 for(const c of manifest.cases)for(const f of c.files)assert.deepEqual(fileRecord(join(repo,f.path)),{bytes:f.bytes,sha256:f.sha256});
 for(const e of Object.values(manifest.engines)){
  const receipt=JSON.parse(fs.readFileSync(join(repo,e.build_receipt)));
  assert.equal(receipt.exit_code,0);assert.equal(receipt.source_unchanged,true);assert.equal(receipt.source_revision,e.source);
  assert.equal(sha(fs.readFileSync(join(repo,e.binary))),receipt.binary_sha256);assert.equal(receipt.adapter_sha256,manifest.adapter_sha256);
  assert.equal(git(e.cwd,'rev-parse','HEAD'),e.source);assert.equal(git(e.cwd,'status','--porcelain','--untracked-files=no'),'');
 }
};
verify();
const free=()=>{const s=fs.statfsSync(repo);return s.bavail*s.bsize;};assert(free()>=12*1024**3,'12 GiB preflight reserve required');
const parent=join(repo,'.cache/practical-board-completion-v1/development');fs.mkdirSync(parent,{recursive:true});
const root=join(parent,'batch-'+manifest.batch);fs.mkdirSync(root,{mode:0o700});
const write=(p,v)=>fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n',{flag:'wx',mode:0o600});
fs.writeFileSync(join(root,'manifest.json'),manifestBytes,{flag:'wx',mode:0o600});
const lock=join(parent,'active.lock');fs.writeFileSync(lock,JSON.stringify({pid:process.pid,batch:manifest.batch})+'\n',{flag:'wx',mode:0o600});
let active=null;const kill=()=>{if(active)try{process.kill(-active.pid,'SIGKILL');}catch(e){if(e.code!=='ESRCH')throw e;}};
process.once('exit',()=>{kill();try{fs.unlinkSync(lock);}catch{}});
const inventory=dir=>{
 const records=[];const walk=d=>{for(const name of fs.readdirSync(d).sort()){
  const p=join(d,name),s=fs.lstatSync(p);assert(!s.isSymbolicLink()&&(s.isDirectory()||s.isFile()));
  if(s.isDirectory()){walk(p);continue;}
  const h=createHash('sha256'),fd=fs.openSync(p,'r'),buffer=Buffer.alloc(1024*1024);let size=0;
  try{for(;;){const n=fs.readSync(fd,buffer,0,buffer.length,null);if(!n)break;h.update(buffer.subarray(0,n));size+=n;}}finally{fs.closeSync(fd);}
  records.push({path:p.slice(dir.length+1),bytes:size,sha256:h.digest('hex')});
 }};walk(dir);return records;
};
const began=Date.now(),outcomes=[];let batchStop=null;
write(join(root,'start.json'),{started_utc:new Date(began).toISOString(),manifest_sha256:sha(manifestBytes),node:process.version,node_binary_sha256:sha(fs.readFileSync(process.execPath)),kicad_version:execFileSync(env.KICADAI_KICAD_CLI,['--version'],{env,encoding:'utf8'}).trim(),kicad_binary_sha256:sha(fs.readFileSync(env.KICADAI_KICAD_CLI)),removed_provider_variables:removed,provider_calls:0,manual_implementation_repairs:0,probe_authorship:'implementing_agent_human_structured_assistance',human_active_minutes:null});
for(const c of manifest.cases){
 const caseStart=Date.now(),caseRoot=join(root,c.id);fs.mkdirSync(caseRoot,{mode:0o700});
 for(const run of manifest.runs_per_case){
  const runID=run.variant+'-'+run.replay;
  if(batchStop||Date.now()-began>=manifest.batch_wall_ms||Date.now()-caseStart>=manifest.case_wall_ms){
   const row={id:c.id,run:runID,status:'not_run',reason:batchStop??'wall_cap'};outcomes.push(row);write(join(caseRoot,runID+'.not-run.json'),row);continue;
  }
  verify();
  const e=manifest.engines[run.variant],binary=join(repo,e.binary),out=join(caseRoot,runID),input=join(repo,c.input);
  const logPath=join(caseRoot,runID+'.log'),fd=fs.openSync(logPath,'wx',0o600);
  const remaining=Math.min(manifest.case_wall_ms-(Date.now()-caseStart),manifest.batch_wall_ms-(Date.now()-began));
  const args=['--mode','native','--input',input,'--output',out,'--timeout',remaining+'ms'];
  const started=Date.now();let peak=null,samples=0,monitorError=null,stop=null,spawnError=null;
  active=spawn(binary,args,{cwd:e.cwd,env,detached:true,stdio:['ignore',fd,fd]});
  const child=active;
  const sample=()=>{try{
   const rss=treeRSSFromPS(execFileSync('ps',['-axo','pid=,ppid=,rss='],{env,encoding:'utf8',timeout:5000}),child.pid);
   peak=Math.max(peak??0,rss);samples++;
   if(rss>manifest.rss_bytes)stop='rss_cap';
   else if(Date.now()-caseStart>=manifest.case_wall_ms)stop='case_wall_cap';
   else if(Date.now()-began>=manifest.batch_wall_ms)stop=batchStop='batch_wall_cap';
   else if(evidenceBytes(root)>manifest.evidence_bytes)stop=batchStop='evidence_cap';
   else if(free()<10*1024**3)stop=batchStop='free_disk_reserve';
  }catch(error){monitorError=error.message;stop=batchStop='resource_monitor_failed';}
  if(stop)kill();};
  const interval=setInterval(sample,1000),timer=setTimeout(()=>{stop='wall_deadline';kill();},remaining);
  child.on('error',e=>{spawnError=e.message;});sample();
  const terminal=await new Promise(resolve=>child.on('close',(exit_code,signal)=>resolve({exit_code,signal})));
  clearInterval(interval);clearTimeout(timer);active=null;fs.closeSync(fd);
  const result=fs.existsSync(join(out,'result.json'))?JSON.parse(fs.readFileSync(join(out,'result.json'))):null;
  const row={id:c.id,run:runID,...terminal,spawn_error:spawnError,started_utc:new Date(started).toISOString(),wall_seconds:(Date.now()-started)/1000,peak_sampled_process_tree_rss_bytes:peak,samples,sample_interval_ms:1000,monitor_error:monitorError,stop_reason:stop,input_sha256:sha(fs.readFileSync(input)),log_sha256:sha(fs.readFileSync(logPath)),result,status:result?.status??'runtime_failure',complete_board_pass:false,provider_calls:0};
  if(result?.first_failed_gate==='requirement_decode_or_validation')row.failure_class='protocol_incompatibility_not_engine_uplift';
  else if(result?.first_failed_gate)row.failure_class='engine_gate_failure_for_this_authored_probe';
  if(fs.existsSync(out))write(join(caseRoot,runID+'.inventory.json'),inventory(out));
  write(join(caseRoot,runID+'.resources.json'),row);outcomes.push(row);console.log(JSON.stringify(row));
  verify();
 }
}
write(join(root,'end.json'),{finished_utc:new Date().toISOString(),wall_seconds:(Date.now()-began)/1000,stop_reason:batchStop,outcomes,complete_board_passes:null,qualification:'development only; no live, clause, visual, or deterministic-replay pass is inferred',provider_calls:0});
write(join(root,'inventory.json'),inventory(root));
