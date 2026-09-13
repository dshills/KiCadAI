import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import fs from 'node:fs';
import {join,relative,isAbsolute,sep} from 'node:path';

export const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
export function inventory(root,secret=''){
 const result=[];
 const walk=dir=>{for(const name of fs.readdirSync(dir).sort()){
  const path=join(dir,name),stat=fs.lstatSync(path);assert(!stat.isSymbolicLink(),'symbolic link in evidence');
  if(stat.isDirectory()){walk(path);continue;}assert(stat.isFile(),'nonregular evidence');
  const hash=createHash('sha256'),fd=fs.openSync(path,'r'),buffer=Buffer.alloc(1024*1024);let count=0,tail=Buffer.alloc(0);
  try{for(;;){const n=fs.readSync(fd,buffer,0,buffer.length,null);if(!n)break;const b=buffer.subarray(0,n);hash.update(b);count+=n;
   if(secret){const chunk=Buffer.concat([tail,b]),needle=Buffer.from(secret);assert(!chunk.includes(needle),'credential found; evidence must not be published');tail=Buffer.from(chunk.subarray(Math.max(0,chunk.length-needle.length+1)));}
  }}finally{fs.closeSync(fd);}
  result.push({path:relative(root,path).split(sep).join('/'),bytes:count,sha256:hash.digest('hex')});
 }};walk(root);return result;
}
export function verifyInventory(root,records,{exclude=[],secret=''}={}){
 const seen=new Set();for(const f of records){
  assert(f.path&&!isAbsolute(f.path)&&!f.path.split('/').some(x=>x==='..'||x==='.'||x==='')&&!f.path.includes('\\'),'unsafe inventory path');
  assert(!seen.has(f.path),'duplicate inventory path');seen.add(f.path);
  assert(Number.isSafeInteger(f.bytes)&&f.bytes>=0&&/^[a-f0-9]{64}$/.test(f.sha256),'invalid inventory record');
 }
 const actual=inventory(root,secret).filter(f=>!exclude.includes(f.path));
 assert.deepEqual(actual,records,'missing, added, or changed evidence');return actual;
}
export function summarizeReadiness(outcomes){
 assert.equal(outcomes.length,12,'all six cases and both engines are required');
 const seen=new Set();for(const row of outcomes){
  assert(/^P0[1-6]$/.test(row.id)&&['baseline-1','candidate-1'].includes(row.run));
  assert(!seen.has(row.id+row.run));seen.add(row.id+row.run);
  assert.equal(row.provider_calls,0);assert.equal(row.complete_board_pass,false);
 }
 return {
  authored_probes:6,engine_invocations:12,
  candidate_native_candidates:outcomes.filter(r=>r.run==='candidate-1'&&r.result?.status==='native_candidate_requires_replay_and_audit').length,
  baseline_protocol_incompatibilities:outcomes.filter(r=>r.run==='baseline-1'&&r.result?.first_failed_gate==='requirement_decode_or_validation').length,
  resource_monitor_failures:outcomes.filter(r=>r.monitor_error||r.stop_reason||r.exit_code!==0||r.signal||!(r.peak_sampled_process_tree_rss_bytes>0)).length,
  complete_positive_passes:null,live_final_run:false,qualifying_uplifts:null,
  admission:false,reason:'No independent clause, complete native, visual and deterministic-replay admission certificate; development statuses cannot substitute.',provider_calls:0,
 };
}
