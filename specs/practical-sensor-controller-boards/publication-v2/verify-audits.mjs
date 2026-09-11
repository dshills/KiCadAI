import {fileURLToPath} from 'node:url';
// Verify saved manual audit structure, evidence bindings and conservative grading.
import assert from 'node:assert/strict';
import {existsSync,readFileSync,readdirSync} from 'node:fs';
import {join,resolve,sep,dirname} from 'node:path';
import {read,sha} from './evidence-authentication.mjs';
const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..');
const root='/tmp/kicadai-practical-sensor-controller-public-1-protocol-v2';
const audits=process.argv[2]??join(repo,'specs/practical-sensor-controller-boards/protocol-v2-audits');
const corpus=read(join(repo,'specs/practical-sensor-controller-boards/corpus.json'));
const inputs=[...corpus.cases,...corpus.paraphrases];
assert.deepEqual(readdirSync(audits).filter(x=>x.endsWith('.audit.json')).sort(),inputs.map(x=>`${x.id}.audit.json`).sort(),'Expected exactly one audit for every frozen case, with no extras');
const names=['requirement interpretation','component/model qualification','electrical analyses','schematic/readability','placement','routing/connectivity','native validation/writer','deterministic replay'];
const all=[];
for(const input of inputs){
  const path=join(audits,`${input.id}.audit.json`);
  const a=read(path);const base=input.of?corpus.cases.find(x=>x.id===input.of):input;
  const clauses=[...base.acceptance,...(base.kind==='positive'?corpus.positive_common_acceptance:[])];
  assert.equal(a.case_id,input.id);assert.deepEqual(a.clauses.map(x=>x.text),clauses);
  assert.deepEqual(a.clauses.map(x=>x.number),clauses.map((_,i)=>i+1));
  assert(a.clauses.every(x=>['pass','fail','not_run'].includes(x.disposition)&&x.reason&&x.evidence.length));
  assert.deepEqual(a.gates.map(x=>x.name),base.kind==='positive'?names:[]);
  assert(a.gates.every(x=>['pass','fail','not_run'].includes(x.disposition)&&x.reason&&x.evidence.length));
  assert.equal(a.independence,'self_review; not external or blind');
  assert(a.review_elapsed_seconds>0);
  assert(Math.abs(a.review_elapsed_seconds-(Date.parse(a.review_utc)-Date.parse(a.review_started_utc))/1000)<0.001);
  assert.equal(a.manual_assistance.manual_implementation_repairs,0);
  for(const f of ['case.json','prompt.txt','result.json','inventory.json'])assert(a.source_binding[f]);
  assert(a.resource_binding);
  let bindingCount=0;
  const verifyBindings=v=>{
    if(!v||typeof v!=='object')return;
    if(typeof v.file==='string'&&typeof v.sha256==='string'){
      const p=resolve(root,v.file);assert(p.startsWith(resolve(root)+sep));assert.equal(sha(readFileSync(p)),v.sha256);bindingCount++;
      if(v.json_pointer!==undefined){
        assert(p.endsWith('.json'));let node=read(p);
        if(v.json_pointer!==''){assert(v.json_pointer.startsWith('/'));for(const part of v.json_pointer.slice(1).split('/').map(x=>x.replace(/~1/g,'/').replace(/~0/g,'~'))){assert(node&&Object.hasOwn(node,part),`Unresolved pointer ${v.file}${v.json_pointer}`);node=node[part];}}
      }
      if(v.line!==undefined)assert(v.line>=1&&v.line<=readFileSync(p,'utf8').split('\n').length);
    }
    for(const child of Object.values(v))verifyBindings(child);
  };verifyBindings(a);
  const r=read(join(root,'baseline',input.id,'result.json'));
  if(a.disposition==='passed'){
    assert(a.clauses.every(c=>c.disposition==='pass'));assert.equal(a.resource_observation.exit_code,0);assert.equal(a.resource_observation.stop_reason,null);assert.equal(a.resource_observation.sampling_errors,0);
    if(base.kind==='refusal'){assert.equal(r.status,'refusal_candidate');assert.equal(r.compilation_status,'unsupported');assert.equal(read(join(root,'baseline',input.id,'selected-intent.json')).requirement,null);assert(!existsSync(join(root,'baseline',input.id,'compiled-requirement.json')));}
    else throw Error('New positive/clarification pass requires explicit extended authentication, not this failure/refusal publication verifier');
  }else assert(['failed','not_run'].includes(a.disposition));
  all.push({id:input.id,disposition:a.disposition,clauses:a.clauses.length,bindings:bindingCount,review_seconds:a.review_elapsed_seconds});
}
assert.equal(readdirSync(audits).filter(x=>x.endsWith('.audit.json')).length,all.length);
console.log(JSON.stringify({verified_utc:new Date().toISOString(),audits:all,total_clauses:all.reduce((n,a)=>n+a.clauses,0),corpus_complete:all.length===inputs.length},null,2));
