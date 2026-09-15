// Evaluator-only. Gold requirements/review notes never enter the model request.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import assert from 'node:assert/strict';
import {isDeepStrictEqual as equal} from 'node:util';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {hash,hashBytes,read,durableJSON,offlineEnvironment,inventory,authenticateFiles,authenticateExamples,assessDecision} from '../evaluation/acceptance-lib.mjs';
import {validateCases} from '../typed-evaluation-02/acceptance.mjs';
import {checkManifest,checkLiveApproval,cleanTerminal,assessAttempt,version as collectorVersion} from './collector.mjs';

export const admission='3-indexed-quantities-experimental';
export const scoringDependencies=[
  './scoring.mjs','./collector.mjs','./runtime-qualification.mjs','../evaluation/acceptance-lib.mjs',
  '../typed-evaluation-02/acceptance.mjs','../development/replay-normalization.mjs',
].map(f=>path.relative(process.cwd(),fileURLToPath(new URL(f,import.meta.url))));
const states=['required','not_required','forbidden','uncertain'];
const sameKeys=(v,keys)=>v && typeof v==='object' && !Array.isArray(v) && equal(Object.keys(v).sort(),[...keys].sort());
const ids=(xs,max,min=0)=>Array.isArray(xs)&&xs.length>=min&&xs.every(x=>Number.isSafeInteger(x)&&x>=0&&x<max)&&new Set(xs).size===xs.length;
const factContext=(f,s)=>({fact:f,source_context:f.sources?.map(id=>s.request_clauses?.[id]),quantity_context:f.quantities?.map(id=>s.source_quantities?.[id])});
const caseContext=(c,s)=>({original_prompt:c.prompt,decision_context:s?.decision??null,review_rubric:c.review??'',required_reason:c.required_reason??null,question_must_resolve:c.question_must_resolve??null});

export function loadPlan(file) {
  const p=read(file);assert.equal(p.version,'indexed-evaluation-plan-1');assert.equal(p.status,'preparation-no-live-approval');
  assert.deepEqual(p.cases_source,{path:'specs/board-family-v2/typed-evaluation-02/cases-02.json',sha256:'90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867'});
  assert.equal(p.evaluation_id,'board-family-v2-indexed-final-03');assert.equal(hash(p.cases_source.path),p.cases_source.sha256);
  const original=validateCases(read(p.cases_source.path));
  assert.deepEqual(p.proposed_policy,{goal:p.evaluation_id,max_requests:14,max_micro_usd:1000000});
  assert.deepEqual(p.acceptance,{planned_cases:14,useful_cases:5,all_raw_extractions_correct:true,all_application_decisions_correct:true,all_required_native_bundles_verified:true,source_bound_review_required:true,median_all_useful_seconds_below:60,maximum_all_useful_seconds_below:120});
  return {plan:p,spec:{...original,evaluation_id:p.evaluation_id}};
}

// Independent structural check of the provider's closed-object/anyOf subset.
// The Go auditor also regenerates source quantities and replays the exact bytes.
export function conforms(v,s) {
  if(s.anyOf)return s.anyOf.some(x=>conforms(v,x));
  if(s.enum&&!s.enum.some(x=>equal(x,v)))return false;
  if(s.type==='object')return sameKeys(v,Object.keys(s.properties??{}))&&s.additionalProperties===false&&equal([...(s.required??[])].sort(),Object.keys(s.properties??{}).sort())&&Object.entries(s.properties).every(([k,c])=>conforms(v[k],c));
  if(s.type==='array')return Array.isArray(v)&&v.length>=(s.minItems??0)&&v.length<=(s.maxItems??Infinity)&&v.every(x=>conforms(x,s.items));
  if(s.type==='string')return typeof v==='string';
  if(s.type==='integer')return Number.isSafeInteger(v)&&v>=(s.minimum??-Infinity)&&v<=(s.maximum??Infinity);
  if(s.type==='number')return typeof v==='number'&&Number.isFinite(v);
  return false;
}

export function matchingFacts(requirement,facts,quantities) {
  const found=[];
  facts.forEach((f,i)=>{
    if(requirement.any_of.some(m=>{
      if(f.kind!==m.kind||f.value!==m.value)return false;
      if(m.kind!=='number')return f.state===m.state;
      const state=m.state??'required';
      return f.state===state&&f.quantities?.length===1&&quantities[f.quantities[0]]?.fields?.[f.value]===m.number;
    }))found.push(i);
  });
  return found;
}

export function assessIndexedRaw(c,s,schema) {
  const raw=s?.raw_intent,clauses=s?.request_clauses,qs=s?.source_quantities;
  let shape=s?.admission_version===admission&&s.original_request===c.prompt&&raw?.version===admission&&conforms(raw,schema);
  shape=Boolean(shape&&Array.isArray(clauses)&&clauses.length>0&&clauses.length<=32&&clauses.every((x,i)=>sameKeys(x,['id','text'])&&x.id===i&&typeof x.text==='string')&&clauses.map(x=>x.text).join('')===c.prompt);
  shape=Boolean(shape&&Array.isArray(qs)&&qs.length<=128&&qs.every((q,i)=>sameKeys(q,['id','clause_id','start','end','text','fields'])&&q.id===i&&Number.isSafeInteger(q.clause_id)&&q.clause_id>=0&&q.clause_id<clauses.length&&Number.isSafeInteger(q.start)&&Number.isSafeInteger(q.end)&&q.start>=0&&q.end>q.start&&q.end<=Buffer.byteLength(c.prompt)&&Buffer.from(c.prompt).subarray(q.start,q.end).toString('utf8')===q.text&&q.fields&&typeof q.fields==='object'&&!Array.isArray(q.fields)&&Object.values(q.fields).every(Number.isFinite)));
  shape=Boolean(shape&&Array.isArray(raw.facts));
  const facts=shape?raw.facts:[];
  if(shape)shape=facts.every(f=>states.includes(f.state)&&ids(f.sources,clauses.length,1)&&ids(f.quantities,qs.length)&&f.quantities.every(id=>f.sources.includes(qs[id].clause_id))&&(f.kind!=='number'||(f.quantities.length===1&&Number.isFinite(qs[f.quantities[0]].fields[f.value]))));
  const requirements=c.requirements.map(r=>({id:r.id,matching_fact_indices:shape?matchingFacts(r,facts,qs):[]}));
  return {structure_pass:shape,required_facts_present:requirements.every(r=>r.matching_fact_indices.length>0),requirements,automatic_checks_pass:shape&&requirements.every(r=>r.matching_fact_indices.length>0),semantic_review:'pending-explicit-fact-and-source-review'};
}

// Evidence has already been authenticated by authenticateBatch below. This
// function deliberately cannot turn an offline result into a live acceptance.
export function scoreReview({spec,state,selections,schema,review,bindings}) {
  assert.equal(review.status,'source-bound-semantic-review');assert.deepEqual(review.bindings,bindings);
  assert.ok(['implementing-agent','independent-human','independent-agent'].includes(review.reviewer?.kind));assert.ok(typeof review.reviewer.name==='string'&&review.reviewer.name.trim());assert.ok(typeof review.reviewed_utc==='string'&&Number.isFinite(Date.parse(review.reviewed_utc)));
  assert.deepEqual(review.cases.map(c=>c.id),spec.cases.map(c=>c.id));
  const results=spec.cases.map((c,i)=>{
    const r=review.cases[i],s=selections[c.id],record=state.records.find(x=>x.id===c.id),raw=assessIndexedRaw(c,s,schema),decision=assessDecision(spec,c,s?.decision);
    const facts=s?.raw_intent?.version===admission&&Array.isArray(s.raw_intent.facts)?s.raw_intent.facts:[];
    for(const [key,value] of Object.entries(caseContext(c,s)))assert.deepEqual(r[key],value,'review context differs from evidence');
    assert.equal(r.prompt_sha256,hashBytes(c.prompt));assert.equal(r.selection_sha256,bindings.selection_sha256[c.id]??null);
    assert.equal(typeof r.raw_complete,'boolean');assert.ok(r.notes?.trim());assert.ok(Array.isArray(r.omitted_requirements)&&r.omitted_requirements.every(x=>typeof x==='string'&&x.trim()));
    assert.deepEqual(r.facts.map(x=>x.fact_index),facts.map((_,j)=>j));
    for(const f of r.facts) {
      for(const [key,value] of Object.entries(factContext(facts[f.fact_index],s)))assert.deepEqual(f[key],value,'review fact context differs from evidence');
      assert.equal(typeof f.meaning_correct,'boolean');assert.equal(typeof f.source_scope_correct,'boolean');assert.ok(f.notes?.trim());
    }
    assert.deepEqual(r.requirements.map(x=>x.id),c.requirements.map(x=>x.id));
    for(const [j,rr] of r.requirements.entries()) {
      assert.deepEqual(rr.requirement,c.requirements[j],'review requirement differs from gold');
      assert.equal(typeof rr.passed,'boolean');assert.ok(rr.notes?.trim());assert.ok(ids(rr.fact_indices,facts.length));
      if(rr.passed) {
        assert.ok(rr.fact_indices.length>0);
        assert.ok(rr.fact_indices.some(k=>raw.requirements[j].matching_fact_indices.includes(k)&&r.facts[k].meaning_correct&&r.facts[k].source_scope_correct),'requirement has no correct source-bound supporting fact');
      }
    }
    assert.equal(typeof r.decision.correct,'boolean');assert.equal(typeof r.decision.targeted_or_truthful,'boolean');assert.ok(r.decision.notes?.trim());
    const recorded=record?.state==='recorded-outcome'&&record?.advance===true;
    const rawPass=Boolean(recorded&&record.collection_class!=='recorded-model-failure'&&s?.extraction_outcome==='decision'&&raw.automatic_checks_pass&&r.raw_complete&&r.omitted_requirements.length===0&&r.facts.every(f=>f.meaning_correct&&f.source_scope_correct)&&r.requirements.every(x=>x.passed));
    const artifactPass=Boolean(recorded&&(c.expected_disposition!=='supported'||record.bundle?.files_compared&&Object.keys(record.bundle.files_compared).length>0));
    const appPass=Boolean(recorded&&s?.extraction_outcome==='decision'&&decision.automatic_checks_pass&&r.decision.correct&&r.decision.targeted_or_truthful&&artifactPass);
    return {id:c.id,raw_automatic:raw,application_automatic:decision,raw_pass:rawPass,application_pass:appPass,complete_pass:rawPass&&appPass};
  });
  const useful=spec.cases.filter(c=>c.expected_disposition==='supported');
  const times=useful.map(c=>state.records.find(r=>r.id===c.id)?.execution?.wall_seconds);
  const known=times.length===5&&times.every(x=>Number.isFinite(x)&&x>=0),sorted=known?[...times].sort((a,b)=>a-b):[];
  const timing=known&&sorted[2]<60&&sorted[4]<120;
  const complete=spec.cases.length===14&&state.status==='collection-complete-semantic-review-pending'&&state.records.length===14&&state.recorded_outcomes===14&&results.every(r=>r.complete_pass)&&timing;
  return {version:'indexed-semantic-results-1',evaluation_id:spec.evaluation_id,planned_cases:14,recorded_cases:state.recorded_outcomes,raw_passes:results.filter(r=>r.raw_pass).length,application_passes:results.filter(r=>r.application_pass).length,complete_passes:results.filter(r=>r.complete_pass).length,median_all_useful_seconds:known?sorted[2]:null,maximum_all_useful_seconds:known?sorted[4]:null,timing_target_met:timing,corpus_status:'known regression cases; not independent or statistical evidence',reviewer:review.reviewer,acceptance:state.mode==='live'&&complete?'all-14-first-attempt-acceptance-met':state.mode==='offline'?'offline-only-cannot-establish-live-acceptance':'complete-acceptance-not-met',cases:results};
}

// Re-export from the pinned executable with no credential or provider request.
// A pinned but invented schema file must not redefine what the runtime accepts.
export function runtimeContract(m) {
  const temporary=fs.mkdtempSync(path.join(os.tmpdir(),'indexed-contract-audit-'));
  try {
    const file=path.join(temporary,'contract.json');
    const probe=spawnSync(m.binary,[...m.prefix_args,'--intent-protocol','indexed-v3','--export-live-contract',file],{env:offlineEnvironment(),encoding:'utf8',timeout:10000,maxBuffer:1024*1024});
    assert.equal(probe.error,undefined);assert.equal(probe.signal,null);assert.equal(probe.status,0,'offline contract export failed');
    return read(file);
  } finally {fs.rmSync(temporary,{recursive:true,force:true});}
}

// Local source/byte consistency, not provider attestation or proof against a
// malicious evidence author. Actual process observations are made by collector.
// Recompute admission, accounting, classification AND all reviewed native files.
export function authenticateBatch(planFile,manifestFile,root,approvalFile) {
  const {plan,spec}=loadPlan(planFile),m=checkManifest(manifestFile),start=read(path.join(root,'start.json')),state=read(path.join(root,'result.json'));
  assert.equal(m.evaluation_id,plan.evaluation_id);assert.deepEqual(m.policy,plan.proposed_policy);
  assert.deepEqual(m.cases,spec.cases.map(({id,prompt})=>({id,prompt})));
  assert.equal(m.evaluation_plan_sha256,hash(planFile));assert.equal(start.manifest_sha256,hash(manifestFile));assert.equal(start.evaluation_id,spec.evaluation_id);
  assert.equal(start.version,collectorVersion);assert.equal(start.status,'started-not-complete');
  assert.deepEqual(start.planned_case_ids,spec.cases.map(c=>c.id));assert.deepEqual(read(path.join(root,'budget.json')),m.policy);
  assert.equal(state.version,collectorVersion);assert.equal(state.evaluation_id,spec.evaluation_id);assert.equal(state.planned_cases,14);assert.equal(state.mode,start.mode);assert.ok(['live','offline'].includes(state.mode));
  assert.equal(state.acceptance,'not-established-by-collection');
  if(state.mode==='live') {checkManifest(manifestFile,'live');checkLiveApproval(read(approvalFile),m,hash(manifestFile));assert.equal(hash(approvalFile),start.approval_sha256);}
  else {assert.equal(approvalFile,undefined);assert.equal(start.approval_sha256,null);}
  const contractFile=m.scoring_contract_path;assert.equal(m.runtime_files_sha256[contractFile],hash(contractFile));
  for(const file of [...scoringDependencies,planFile,plan.cases_source.path]) {
    const relative=path.relative(process.cwd(),path.resolve(file));
    assert.equal(m.runtime_files_sha256[relative],hash(file),'scoring dependency missing from freeze');
  }
  const contract=read(contractFile);assert.equal(contract.admission_version,admission);assert.equal(contract.schema_name,'board_family_indexed_requirements_v3');
  assert.deepEqual(contract,runtimeContract(m),'scoring contract differs from executable');
  assert.ok(state.records.length<=spec.cases.length);
  assert.deepEqual(state.records.map(r=>r.id),spec.cases.slice(0,state.records.length).map(c=>c.id));
  const examples=m.qualification==='reviewed-two-family-examples'?authenticateExamples().receipt.cases:[];
  const selections={},selectionHashes={};let prefix=[],stopped=false;
  for(const record of state.records) {
    assert.equal(stopped,false,'case followed an unsafe attempt');
    const caseRoot=path.join(root,record.id),outcome=read(path.join(caseRoot,'outcome.json'));
    assert.deepEqual(outcome,record);authenticateFiles(caseRoot,record.files_sha256);
    assert.deepEqual(inventory(caseRoot).filter(f=>f!=='outcome.json'),Object.keys(record.files_sha256).sort());
    const prompt=spec.cases.find(c=>c.id===record.id).prompt;
    assert.equal(hash(path.join(caseRoot,'prompt.txt')),hashBytes(prompt));assert.equal(record.prompt_sha256,hashBytes(prompt));
    assert.deepEqual(read(path.join(caseRoot,'attempt.json')),{id:record.id,prompt_sha256:record.prompt_sha256,state:'attempt-recorded-before-launch',started_utc:record.started_utc});
    if(record.execution)assert.deepEqual(read(path.join(caseRoot,'command.process.json')),record.execution);
    if(record.audit_execution)assert.deepEqual(read(path.join(caseRoot,'audit.process.json')),record.audit_execution);
    const file=path.join(caseRoot,'journal/selection/selection.json');
    if(fs.existsSync(file)) {selections[record.id]=read(file);selectionHashes[record.id]=hash(file);}
    if(record.state==='recorded-outcome') {
      cleanTerminal(record.audit_execution);assert.equal(record.audit_execution.exit_code,0);
      const probe=spawnSync(m.binary,[...m.prefix_args,'--inspect-indexed-journal',path.join(caseRoot,'journal')],{env:offlineEnvironment(),encoding:'utf8',timeout:10000,maxBuffer:1024*1024});
      assert.equal(probe.error,undefined);assert.equal(probe.signal,null);assert.equal(probe.status,0,'journal no longer verifies');
      const audit=JSON.parse(probe.stdout);assert.deepEqual(audit,read(path.join(caseRoot,'audit.stdout.log')));
      const ledger=read(path.join(caseRoot,'journal/selection/ledger.json'));
      const expected=assessAttempt({execution:record.execution,audit,ledger,policy:m.policy,prefix,prompt,caseRoot,examples});
      for(const key of ['collection_class','advance','model_correct','response_id','ledger_index','bundle']) assert.deepEqual(record[key],expected[key],`recorded ${key} differs from replay`);
      assert.equal(record.failure,undefined);prefix=ledger.entries;
    } else {
      assert.ok(['attempt-recorded-before-launch','terminal-observed'].includes(record.state));
      assert.equal(record.advance,false);assert.equal(record.collection_class,'unsafe-to-continue');assert.ok(typeof record.failure==='string'&&record.failure.trim());
      stopped=true;
    }
  }
  const ledgerFile=path.join(root,'ledger.json');
  if(fs.existsSync(ledgerFile)) {
    const ledger=read(ledgerFile);assert.equal(ledger.version,2);assert.equal(ledger.goal,m.policy.goal);
    assert.equal(ledger.max_requests,m.policy.max_requests);assert.equal(ledger.max_micro_usd,m.policy.max_micro_usd);
    assert.deepEqual(ledger.entries.slice(0,prefix.length),prefix,'final ledger changed earlier attempts');
    if(!stopped) {assert.deepEqual(ledger.entries,prefix);assert.ok(!ledger.halt_reason);}
    else assert.ok(ledger.entries.length>=prefix.length&&ledger.entries.length<=prefix.length+1);
  } else assert.equal(prefix.length,0,'completed accounting is missing');
  assert.equal(state.prepared_cases,state.records.length);
  assert.equal(state.launched_cases,state.records.filter(r=>r.execution?.child_pid!=null).length);
  assert.equal(state.recorded_outcomes,state.records.filter(r=>r.state==='recorded-outcome').length);
  assert.equal(state.recorded_model_failures,state.records.filter(r=>r.collection_class==='recorded-model-failure').length);
  assert.deepEqual(state.unattempted_case_ids,spec.cases.slice(state.records.length).map(c=>c.id));
  assert.equal(state.status,!stopped&&state.records.length===14?'collection-complete-semantic-review-pending':'stopped-no-retry');
  assert.ok(Number.isFinite(state.total_wall_seconds)&&state.total_wall_seconds>=0);
  const expectedInventory=['start.json','budget.json','result.json',...(fs.existsSync(ledgerFile)?['ledger.json']:[]),...state.records.flatMap(r=>['outcome.json',...Object.keys(r.files_sha256)].map(f=>`${r.id}/${f}`))].sort();
  assert.deepEqual(inventory(root),expectedInventory,'unrecorded or missing batch evidence');
  const bindings={plan_sha256:hash(planFile),manifest_sha256:hash(manifestFile),result_sha256:hash(path.join(root,'result.json')),cases_sha256:plan.cases_source.sha256,selection_sha256:selectionHashes};
  return {spec,state,selections,schema:contract.schema,bindings};
}

export function reviewTemplate({spec,selections,bindings}) {
  return {status:'pending-source-bound-review',reviewer:{kind:'implementing-agent',name:''},reviewed_utc:null,bindings,cases:spec.cases.map(c=>{
    const s=selections[c.id],facts=s?.raw_intent?.version===admission&&Array.isArray(s.raw_intent.facts)?s.raw_intent.facts:[];
    return {id:c.id,...caseContext(c,s),prompt_sha256:hashBytes(c.prompt),selection_sha256:bindings.selection_sha256[c.id]??null,raw_complete:false,omitted_requirements:[],notes:'',facts:facts.map((f,i)=>({fact_index:i,...factContext(f,s),meaning_correct:false,source_scope_correct:false,notes:''})),requirements:c.requirements.map(r=>({id:r.id,requirement:r,passed:false,fact_indices:[],notes:''})),decision:{correct:false,targeted_or_truthful:false,notes:''}};
  })};
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  try {
    const [mode,plan,manifest,root,file,approval]=process.argv.slice(2);
    assert.ok(['--template','--check'].includes(mode)&&plan&&manifest&&root&&file,'usage: scoring.mjs --template|--check PLAN MANIFEST BATCH REVIEW [APPROVAL]');
    const inputs=authenticateBatch(plan,manifest,root,approval);
    if(mode==='--template')durableJSON(file,reviewTemplate(inputs),{exclusive:true});
    else console.log(JSON.stringify(scoreReview({...inputs,review:read(file)}),null,2));
  } catch(error){console.error(error.message);process.exitCode=1;}
}
