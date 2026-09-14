// No sockets, real keys or provider calls. These test evaluator mechanics only.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import test from 'node:test';
import {spawnSync} from 'node:child_process';
import {read,hashBytes,expectedConfig,offlineEnvironment,durableJSON} from '../evaluation/acceptance-lib.mjs';
import {executeCase,processIsAlive} from '../evaluation/run-language.mjs';
import {E,evaluationID,policy,model,admissionVersion,batch,ledgerFile,validateCases,conforms,assessRaw,assessSelection,checkLedger,checkApproval,verifyResume,metrics,checkSemanticReview} from './acceptance.mjs';

const spec=validateCases(read(`${E}/cases-02.json`)), schema=read(`${E}/LIVE_CONTRACT-02.json`).schema;
function synthetic(c) {
  // Deliberately broad source quotes are synthetic structural fixtures, not
  // examples of good model extraction or evidence of real language performance.
  const facts=c.requirements.map(r=>({...r.any_of[0],quote:c.prompt}));
  return {model,admission_version:admissionVersion,original_request:c.prompt,request_clauses:[{id:0,text:c.prompt}],
    raw_intent:{version:'2',clauses:[{id:0,facts:facts.length?facts:[{kind:'none'}]}]},
    decision:{disposition:c.expected_disposition,message:'Synthetic test only; meaning review is still required.',clauses:[{text:c.prompt,disposition:c.expected_disposition,reason:'Synthetic test only.'}],configuration:c.expected_disposition==='supported'?expectedConfig(spec,c):null}};
}
function temp(t) {const d=fs.mkdtempSync(path.join(os.tmpdir(),'typed-eval-02-test-'));t.after(()=>fs.rmSync(d,{recursive:true,force:true}));return d;}
const completedEntry={index:1,model,status:'completed',reserve_micro_usd:50000,response_id:'synthetic-response',input_tokens:100,output_tokens:200,estimated_micro_usd:360};

test('14 explicit cases cover useful profiles, choices and refusals without executing them',()=>{
  assert.deepEqual(new Set(spec.cases.slice(0,5).map(c=>`${c.family}/${c.profile}`)),new Set(['esp32_bmp280_v1/standard','esp32_bmp280_v1/fast','esp32_bmp280_v1/low_current','esp32_sht31_v1/standard','esp32_sht31_v1/fast']));
  assert.throws(()=>validateCases({...spec,cases:spec.cases.slice(1)}));
  assert.throws(()=>validateCases({...spec,cases:[spec.cases[1],...spec.cases.slice(1)]}));
});
test('synthetic matches leave raw and final meaning review pending',()=>{
  for(const c of spec.cases) {
    const r=assessSelection(spec,c,synthetic(c),schema);
    assert.equal(r.raw.automatic_checks_pass,true,c.id); assert.equal(r.admitted.automatic_checks_pass,true,c.id);
    assert.equal(r.raw.semantic_review,'pending-source-bound-review');
  }
});
test('every required typed fact matters, even if the application answer is correct',()=>{
  for(const c of spec.cases) for(let i=0;i<c.requirements.length;i++) {
    const s=synthetic(c);s.raw_intent.clauses[0].facts.splice(i,1);
    if(!s.raw_intent.clauses[0].facts.length)s.raw_intent.clauses[0].facts=[{kind:'none'}];
    const r=assessSelection(spec,c,s,schema);
    assert.equal(r.raw.automatic_checks_pass,false,`${c.id}/${i}`);
    assert.equal(r.admitted.automatic_checks_pass,true,'local correctness must not promote raw correctness');
  }
});
test('wrong polarity, value and quantity cannot satisfy frozen fact requirements',()=>{
  for(const id of ['useful-02','useful-03','useful-05','choice-02','choice-03','refuse-04']) {
    const c=spec.cases.find(c=>c.id===id),s=synthetic(c),f=s.raw_intent.clauses[0].facts.at(-1);
    if(f.state)f.state=f.state==='required'?'not_required':'required';else f.number++;
    assert.equal(assessRaw(c,s,schema).automatic_checks_pass,false,id);
  }
});
test('invalid schema branch, original request, clause coverage and invented quotes fail',()=>{
  const c=spec.cases[0];
  const mutations=[
    s=>s.original_request='different',s=>s.admission_version='bounded-requirements-01',s=>s.raw_decision={},
    s=>s.request_clauses[0].id=1,s=>s.request_clauses[0].text='partial',s=>s.request_clauses=[],
    s=>s.raw_intent.version='1',s=>s.raw_intent.clauses[0].id=-1,s=>s.raw_intent.clauses[0].id=2,
    s=>s.raw_intent.clauses.push(s.raw_intent.clauses[0]),s=>s.raw_intent.clauses[0].facts=[],
    s=>s.raw_intent.clauses[0].facts[0].quote='not in source',s=>s.raw_intent.clauses[0].facts[0].state='safe',
    s=>s.raw_intent.clauses[0].facts[0].number=123,s=>s.raw_intent.extra=true,
    s=>s.raw_intent.clauses[0].facts[0].quote='',s=>s.raw_intent=null,
  ];
  for(const mutate of mutations) {const s=synthetic(c);mutate(s);assert.equal(assessRaw(c,s,schema).automatic_checks_pass,false);}
  assert.equal(conforms(Infinity,{type:'number'}),false);assert.equal(conforms(0.5,{type:'integer'}),false);
  assert.equal(conforms('x',{type:'unsupported'}),false);
});
test('correct raw facts do not excuse a wrong final configuration or unnecessary question',()=>{
  const c=spec.cases[0];
  for(const mutate of [s=>s.decision.configuration.total_bus_capacitance_pf=100,s=>s.decision.disposition='clarify',s=>s.decision.clauses[0].text='truncated']) {
    const s=synthetic(c);mutate(s);const r=assessSelection(spec,c,s,schema);
    assert.equal(r.raw.automatic_checks_pass,true);assert.equal(r.admitted.automatic_checks_pass,false);
  }
});
test('new budget approval is exact, not inherited from the previous evaluation',()=>{
  const a={status:'explicit-user-approved',evaluation_id:evaluationID,max_physical_requests:14,max_micro_usd:1000000,existing_key_only:true,runtime_sha256:'runtime',freeze_sha256:'freeze',user_message:'Synthetic authorization fixture; never installed as authority.',recorded_utc:'2026-09-14T00:00:00Z'};
  checkApproval(a,'runtime','freeze');assert.throws(()=>checkApproval(a,'runtime','freeze',true));
  checkApproval({...a,allow_unattempted_resume:true},'runtime','freeze',true);
  for(const key of Object.keys(a)) {const b={...a};delete b[key];assert.throws(()=>checkApproval(b,'runtime','freeze'),key);}
  for(const b of [{...a,evaluation_id:'board-family-v2-final-01'},{...a,max_physical_requests:15},{...a,max_micro_usd:10000000},{...a,runtime_sha256:'changed'},{...a,status:'proposed'}])assert.throws(()=>checkApproval(b,'runtime','freeze'));
});
test('ledger guards prevent reset, mutation, excess requests and inaccurate usage',()=>{
  const l={version:2,...policy,entries:[completedEntry]};checkLedger(l,[completedEntry]);
  for(const bad of [{...l,goal:'board-family-v2-final-01'},{...l,entries:[]},{...l,max_requests:41},{...l,halt_reason:'usage anomaly'},
    {...l,entries:[{...completedEntry,estimated_micro_usd:0}]},{...l,entries:[{...completedEntry,output_tokens:1601}]},
    {...l,entries:Array.from({length:15},(_,i)=>({...completedEntry,index:i+1}))}])assert.throws(()=>checkLedger(bad,[completedEntry]));
  checkLedger({...l,entries:[{index:1,model,status:'failed_or_unknown',reserve_micro_usd:50000}]});
  assert.throws(()=>checkLedger({...l,entries:[completedEntry,{...completedEntry,index:2}]}));
});
test('resume requires terminal evidence, unchanged prefix and only untouched cases',()=>{
  const s={evaluation_id:evaluationID,status:'stopped',owner_pid:1234,records:[{id:spec.cases[0].id,state:'finished',child_terminal_observed:true}],ledger_entries:[completedEntry]};
  assert.deepEqual(verifyResume(s,spec,[completedEntry],()=>false),spec.cases.slice(1));
  assert.throws(()=>verifyResume(s,spec,[completedEntry],()=>true));assert.throws(()=>verifyResume(s,spec,[],()=>false));
  for(const bad of [{...s,status:'running'},{...s,owner_pid:null},{...s,records:[{...s.records[0],child_terminal_observed:false}]},{...s,records:[{...s.records[0],id:spec.cases[1].id}]}])assert.throws(()=>verifyResume(bad,spec,[completedEntry],()=>false));
});
test('all five useful attempts count in timing, including failures and missing values',()=>{
  const records=spec.cases.slice(0,5).map((c,i)=>({id:c.id,state:'finished',wall_seconds:i===0?150:4}));
  const m=metrics(spec,records);assert.equal(m.planned_cases,14);assert.equal(m.maximum_all_useful_seconds,150);assert.equal(m.timing_target_met,false);
  delete records[0].wall_seconds;assert.equal(metrics(spec,records).median_all_useful_seconds,null);
  assert.equal(metrics(spec,records).acceptance,'not-established-by-automatic-scores');
});
test('an execution or evidence error defeats an optimistic artifact flag',()=>{
  const records=[{id:spec.cases[0].id,state:'finished',wall_seconds:4,admitted:{automatic_checks_pass:true},output_checks_pass:true,execution_or_evidence_failure:'native comparison failed'}];
  assert.equal(metrics(spec,records).application_automatic_passes,0);
});
test('source-bound meaning review cannot pass a missing requirement, changed byte binding or false judgment',()=>{
  const bindings={state_sha256:'state',freeze_sha256:'freeze',runtime_sha256:'runtime',prompt_hashes:Object.fromEntries(spec.cases.map(c=>[c.id,hashBytes(Buffer.from(c.prompt))]))};
  const selections=Object.fromEntries(spec.cases.map(c=>[c.id,{selection:synthetic(c),sha256:`sha-${c.id}`}]))
  const state={records:spec.cases.map(c=>({id:c.id,...assessSelection(spec,c,selections[c.id].selection,schema),output_checks_pass:true}))};
  const review={status:'source-bound-semantic-review',evaluation_id:evaluationID,state_sha256:'state',freeze_sha256:'freeze',runtime_sha256:'runtime',reviewer:{kind:'implementing-agent',name:'Synthetic test only'},cases:spec.cases.map(c=>({id:c.id,prompt_sha256:bindings.prompt_hashes[c.id],selection_sha256:`sha-${c.id}`,raw_complete:true,raw_correct:true,decision_correct:true,notes:'Artificial fixture, not an actual review.',requirements:c.requirements.map((r,i)=>({id:r.id,passed:true,evidence:[{clause_id:0,fact_index:i}],note:'Synthetic fact correspondence.'}))}))};
  assert.ok(checkSemanticReview(spec,state,review,bindings,selections).every(r=>r.complete_pass));
  assert.throws(()=>checkSemanticReview(spec,state,{...review,state_sha256:'changed'},bindings,selections));
  const bad=structuredClone(review);bad.cases[0].requirements[0].evidence=[];assert.throws(()=>checkSemanticReview(spec,state,bad,bindings,selections));
  const falseJudgment=structuredClone(review);falseJudgment.cases[0].raw_correct=false;
  assert.equal(checkSemanticReview(spec,state,falseJudgment,bindings,selections)[0].complete_pass,false);
  const wrong=structuredClone(review);wrong.cases[0].requirements[0].evidence=[{clause_id:0,fact_index:1}];assert.throws(()=>checkSemanticReview(spec,state,wrong,bindings,selections));
});
test('failed local child has a durable identity and is never retried',async t=>{
  const d=temp(t),identity=`${d}/attempt.json`;let calls=0;
  const r=await executeCase(process.execPath,['-e','const fs=require("fs");if(!JSON.parse(fs.readFileSync(process.argv[1])).attempt)process.exit(99);process.exit(17)',identity],offlineEnvironment(),`${d}/child`,()=>{calls++;durableJSON(identity,{attempt:1},{exclusive:true});});
  assert.equal(r.exit_code,17);assert.equal(r.child_terminal_observed,true);assert.equal(calls,1);assert.equal(processIsAlive(r.child_pid),false);
});
test('runner without frozen runtime or actual approval cannot launch an evaluation',t=>{
  // Run from an empty sandbox directory, never the real evaluation's cwd.
  // This remains safe even after a genuine approval is later committed.
  const directory=temp(t),script=path.resolve(`${E}/run.mjs`);
  const env={...offlineEnvironment(),OPENAI_API_KEY:'synthetic-offline-value'};
  for(const mode of ['--live','--resume','--bogus']) {
    const r=spawnSync(process.execPath,[script,mode],{cwd:directory,env,encoding:'utf8',timeout:30000});
    assert.notEqual(r.status,0);assert.ok(!r.signal);assert.ok(!r.error);
    assert.equal(fs.existsSync(path.join(directory,batch)),false);assert.equal(fs.existsSync(path.join(directory,ledgerFile)),false);assert.equal(fs.existsSync(path.join(directory,`${batch}.lock`)),false);
  }
});
