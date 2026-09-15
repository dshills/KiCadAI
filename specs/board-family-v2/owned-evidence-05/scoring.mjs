// Evaluator-only owned-v4 raw-fact scoring. No provider or native-generation calls.
// Raw facts stay v4. Resolved references are review annotations, never model output.
import assert from 'node:assert/strict';
import {isDeepStrictEqual as equal} from 'node:util';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {hash,hashBytes,read,assessDecision} from '../evaluation/acceptance-lib.mjs';
import {validateCases} from '../typed-evaluation-02/acceptance.mjs';
import {conforms} from '../source-reference-candidate-03/scoring.mjs';
import {authenticateBatch} from './authenticate.mjs';
import {checkManifest,collectorDependencies} from './collector.mjs';
import {admission,assertContract} from './contracts.mjs';
export {admission};

export const corpusFile='specs/board-family-v2/typed-evaluation-02/cases-02.json';
export const corpusSHA='90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867';
const ownFiles=['./scoring.mjs','./review-runner.mjs','./scoring.test.mjs','./review-runner.test.mjs','../../../.github/workflows/owned-semantic-scoring.yml','../source-reference-candidate-03/scoring.mjs','../typed-evaluation-02/acceptance.mjs'].map(f=>fileURLToPath(new URL(f,import.meta.url)));
export const scoringDependencies=[...new Set([...ownFiles,...collectorDependencies, path.resolve(corpusFile)])].sort();
const rawFacts=s=>Array.isArray(s?.raw_intent?.facts)?s.raw_intent.facts:[];
const uniqueSorted=xs=>[...new Set(xs)].sort((a,b)=>a-b);
const ids=(xs,max)=>Array.isArray(xs)&&xs.every(x=>Number.isSafeInteger(x)&&x>=0&&x<max)&&new Set(xs).size===xs.length;
const nonempty=x=>typeof x==='string'&&x.trim().length>0;
const caseContext=(c,s)=>({original_prompt:c.prompt,raw_intent:s?.raw_intent??null,extraction_outcome:s?.extraction_outcome??null,decision_context:s?.decision??null,review_rubric:c.review,required_reason:c.required_reason??null,question_must_resolve:c.question_must_resolve??null});

export function loadCorpus() {
  assert.equal(hash(corpusFile),corpusSHA,'known regression corpus changed');
  return validateCases(read(corpusFile));
}
export function loadInputs(manifestFile,batchRoot) {
  const original=loadCorpus(),m=checkManifest(manifestFile),verified=authenticateBatch(manifestFile,batchRoot);
  assert.deepEqual(m.cases,original.cases.map(({id,prompt})=>({id,prompt})));
  const contracts=Object.fromEntries(m.contracts.map(c=>[c.id,read(c.path)]));
  return {...verified,spec:{...original,evaluation_id:m.evaluation_id},contracts,bindings:{...verified.bindings,corpus_sha256:corpusSHA,
    contract_sha256:Object.fromEntries(m.contracts.map(c=>[c.id,c.sha256])),
    scoring_files_sha256:Object.fromEntries(scoringDependencies.map(f=>[path.relative(process.cwd(),f),hash(f)]))}};
}

// A schema-valid fact's atomic qN/field reference resolves to application-owned
// numeric magnitude, field and owner. Alias sets preserve the original raw fact.
export function factContext(fact,contract) {
  const source=contract.source;
  const clauses=new Set(),quantities=new Set();let numericChoice=null;
  const quantity=id=>{
    const q=source.quantities[id];assert.ok(q&&q.id===id&&source.clauses[q.clause_id]);
    quantities.add(id);clauses.add(q.clause_id);return q;
  };
  const refs=fact.kind==='number'?fact.context:fact.evidence;
  assert.ok(Array.isArray(refs));
  if(fact.kind==='number') {
    const match=/^q(0|[1-9][0-9]*)\/([a-z][a-z0-9_]*)$/.exec(fact.choice);assert.ok(match,'unknown atomic numeric choice');
    const id=Number(match[1]),field=match[2],q=quantity(id);
    assert.ok(Object.hasOwn(q.fields,field)&&Number.isFinite(q.fields[field]),'dimensionally ineligible choice');
    numericChoice={id:fact.choice,quantity_id:id,clause_id:q.clause_id,field,value:q.fields[field]};
  }
  for(const ref of refs) {
    const match=/^([cq])(0|[1-9][0-9]*)$/.exec(ref);assert.ok(match,'unknown evidence alias');
    const id=Number(match[2]);
    if(match[1]==='q') {assert.ok(['feature','other','unclear'].includes(fact.kind));quantity(id);}
    else {assert.ok(source.clauses[id]&&source.clauses[id].id===id);clauses.add(id);}
  }
  return {fact:structuredClone(fact),source_context:uniqueSorted([...clauses]).map(id=>structuredClone(source.clauses[id])),
    quantity_context:uniqueSorted([...quantities]).map(id=>structuredClone(source.quantities[id])),numeric_choice:numericChoice,resolution_error:null};
}
function reviewFactContext(f,s,contract) {
  try {return factContext(f,contract);}
  catch {return {fact:structuredClone(f),source_context:[],quantity_context:[],numeric_choice:null,resolution_error:'invalid owned evidence references; inspect raw fact and original prompt'};}
}
export function matchingFacts(requirement,contexts) {
  const found=[];
  contexts.forEach((context,i)=>{
    const f=context.fact;
    if(requirement.any_of.some(m=>f.kind===m.kind&&f.state===(m.state??'required')&&
      (m.kind==='number'?context.numeric_choice?.field===m.value&&context.numeric_choice.value===m.number:f.value===m.value)))found.push(i);
  });
  return found;
}

export function assessOwnedRaw(c,s,contract) {
  let structure=false,contexts=[];
  try {
    assertContract(contract,c.prompt);
    assert.equal(s.admission_version,admission);assert.equal(s.original_request,c.prompt);
    assert.deepEqual(s.request_clauses,contract.source.clauses);assert.deepEqual(s.source_quantities,contract.source.quantities);
    assert.equal(s.raw_intent.version,admission);assert.ok(conforms(s.raw_intent,contract.schema));
    assert.equal(contract.source.clauses.map(c=>c.text).join(''),c.prompt);
    contexts=s.raw_intent.facts.map(f=>factContext(f,contract));structure=true;
  } catch {contexts=[];}
  const covered=new Set(contexts.flatMap(c=>c.quantity_context.map(q=>q.id)));
  const missing=(contract?.source?.quantities??[]).filter(q=>!covered.has(q.id)).map(q=>q.id);
  const requirements=c.requirements.map(r=>({id:r.id,matching_fact_indices:structure?matchingFacts(r,contexts):[]}));
  const complete=requirements.every(r=>r.matching_fact_indices.length>0);
  return {structure_pass:structure,required_facts_present:complete,all_numeric_occurrences_referenced:structure&&missing.length===0,
    unreferenced_quantity_ids:missing,requirements,automatic_checks_pass:structure&&complete&&missing.length===0,
    semantic_review:'pending-explicit-fact-meaning-scope-and-completeness-review'};
}

export function reviewTemplate({spec,selections,contracts,bindings}) {
  return {version:'owned-source-review-1',status:'pending-source-bound-review',reviewer:{kind:'implementing-agent',name:''},reviewed_utc:null,
    bindings:structuredClone(bindings),cases:spec.cases.map(c=>{
      const s=selections[c.id],facts=rawFacts(s);
      return {id:c.id,...structuredClone(caseContext(c,s)),prompt_sha256:hashBytes(c.prompt),selection_sha256:bindings.selection_sha256[c.id]??null,
        contract_sha256:bindings.contract_sha256[c.id],raw_complete:false,omitted_requirements:[],notes:'',
        facts:facts.map((f,i)=>({fact_index:i,...reviewFactContext(f,s,contracts[c.id]),meaning_correct:false,source_scope_correct:false,notes:''})),
        requirements:c.requirements.map(r=>({id:r.id,requirement:structuredClone(r),passed:false,fact_indices:[],notes:''})),
        decision:{correct:false,targeted_or_truthful:false,notes:''}};
    })};
}

export function scoreReview({spec,state,selections,contracts,review,bindings}) {
  assert.equal(review.version,'owned-source-review-1');assert.equal(review.status,'source-bound-semantic-review');assert.deepEqual(review.bindings,bindings);
  assert.ok(['implementing-agent','independent-human','independent-agent'].includes(review.reviewer?.kind));assert.ok(nonempty(review.reviewer.name));
  assert.ok(typeof review.reviewed_utc==='string'&&Number.isFinite(Date.parse(review.reviewed_utc)));
  assert.ok(['offline','live'].includes(state.mode));
  assert.equal(spec.cases.length,14);assert.equal(state.planned_cases,14);
  assert.deepEqual(review.cases.map(c=>c.id),spec.cases.map(c=>c.id));
  assert.equal(new Set(state.records.map(r=>r.id)).size,state.records.length);
  assert.deepEqual(state.records.map(r=>r.id),spec.cases.slice(0,state.records.length).map(c=>c.id));
  const results=spec.cases.map((c,i)=>{
    const r=review.cases[i],s=selections[c.id],contract=contracts[c.id],facts=rawFacts(s),record=state.records.find(x=>x.id===c.id);
    const raw=assessOwnedRaw(c,s,contract),decision=assessDecision(spec,c,s?.decision);
    for(const [key,value] of Object.entries(caseContext(c,s)))assert.deepEqual(r[key],value,'review context differs from original evidence');
    assert.equal(r.prompt_sha256,hashBytes(c.prompt));assert.equal(r.selection_sha256,bindings.selection_sha256[c.id]??null);
    assert.equal(r.contract_sha256,bindings.contract_sha256[c.id]);
    assert.equal(typeof r.raw_complete,'boolean');assert.ok(nonempty(r.notes));
    assert.ok(Array.isArray(r.omitted_requirements)&&r.omitted_requirements.every(nonempty));
    assert.deepEqual(r.facts.map(f=>f.fact_index),facts.map((_,j)=>j));
    for(const f of r.facts) {
      for(const [key,value] of Object.entries(reviewFactContext(facts[f.fact_index],s,contract)))assert.deepEqual(f[key],value,'review fact or source resolution differs');
      assert.equal(typeof f.meaning_correct,'boolean');assert.equal(typeof f.source_scope_correct,'boolean');assert.ok(nonempty(f.notes));
      if(f.resolution_error)assert.equal(f.source_scope_correct,false,'invalid references cannot be approved');
    }
    assert.deepEqual(r.requirements.map(rr=>rr.id),c.requirements.map(rr=>rr.id));
    for(const [j,rr] of r.requirements.entries()) {
      assert.deepEqual(rr.requirement,c.requirements[j]);assert.equal(typeof rr.passed,'boolean');assert.ok(nonempty(rr.notes));assert.ok(ids(rr.fact_indices,facts.length));
      if(rr.passed)assert.ok(rr.fact_indices.some(k=>raw.requirements[j].matching_fact_indices.includes(k)&&r.facts[k].meaning_correct&&r.facts[k].source_scope_correct),'requirement lacks a correct source-bound supporting fact');
    }
    assert.equal(typeof r.decision.correct,'boolean');assert.equal(typeof r.decision.targeted_or_truthful,'boolean');assert.ok(nonempty(r.decision.notes));
    const recorded=record?.state==='recorded-outcome'&&record.advance===true;
    const rawPass=Boolean(recorded&&record.collection_class!=='recorded-model-failure'&&s?.extraction_outcome==='decision'&&raw.automatic_checks_pass&&r.raw_complete&&r.omitted_requirements.length===0&&r.facts.every(f=>f.meaning_correct&&f.source_scope_correct)&&r.requirements.every(rr=>rr.passed));
    const artifactPass=Boolean(recorded&&(c.expected_disposition!=='supported'||Object.keys(record.bundle?.files_compared??{}).length>0));
    const appPass=Boolean(recorded&&s?.extraction_outcome==='decision'&&decision.automatic_checks_pass&&r.decision.correct&&r.decision.targeted_or_truthful&&artifactPass);
    return {id:c.id,raw_automatic:raw,application_automatic:decision,raw_pass:rawPass,application_pass:appPass,complete_pass:rawPass&&appPass};
  });
  const useful=spec.cases.filter(c=>c.expected_disposition==='supported');
  const times=useful.map(c=>state.records.find(r=>r.id===c.id)?.execution?.wall_seconds);
  const known=times.length===5&&times.every(t=>Number.isFinite(t)&&t>=0),sorted=known?[...times].sort((a,b)=>a-b):[];
  const timing=known&&sorted[2]<60&&sorted[4]<120;
  const criteria=state.status==='collection-complete-semantic-review-pending'&&state.records.length===14&&state.recorded_outcomes===14&&results.every(r=>r.complete_pass)&&timing;
  return {version:'owned-semantic-results-1',evaluation_id:spec.evaluation_id,planned_cases:14,recorded_cases:state.recorded_outcomes,
    raw_passes:results.filter(r=>r.raw_pass).length,application_passes:results.filter(r=>r.application_pass).length,complete_passes:results.filter(r=>r.complete_pass).length,
    median_all_useful_seconds:known?sorted[2]:null,maximum_all_useful_seconds:known?sorted[4]:null,timing_target_met:timing,criteria_met:criteria,
    acceptance:state.mode==='offline'?'offline-only-cannot-establish-live-acceptance':'not-established-by-scoring-alone',
    corpus_status:'known regression cases; not independent or statistical evidence',reviewer:review.reviewer,cases:results};
}
