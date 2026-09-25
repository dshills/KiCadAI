// Pure evaluator tests. All judgments below are synthetic test inputs, never
// actual review records or claims about model quality/provider authenticity.
import test from 'node:test';
import assert from 'node:assert/strict';
import {admission,loadPlan,conforms,matchingFacts,assessIndexedRaw,reviewTemplate,scoreReview} from './scoring.mjs';
import {hashBytes} from '../evaluation/acceptance-lib.mjs';

const object=properties=>({type:'object',additionalProperties:false,required:Object.keys(properties),properties});
const schema=object({version:{type:'string',enum:[admission]},facts:{type:'array',maxItems:64,items:object({
  kind:{type:'string'},value:{type:'string'},state:{type:'string',enum:['required','not_required','forbidden','uncertain']},
  sources:{type:'array',minItems:1,maxItems:32,items:{type:'integer',minimum:0,maximum:31}},
  quantities:{type:'array',maxItems:128,items:{type:'integer',minimum:0,maximum:127}},
})}});
const copy=x=>structuredClone(x);
function fixture() {
  const prompt='Please use BMP280 with 100 pF total capacitance.';
  const facts=[{kind:'sensor',value:'BMP280',state:'required',sources:[0],quantities:[]},{kind:'number',value:'total_bus_capacitance_pf',state:'required',sources:[0],quantities:[0]}];
  const spec={evaluation_id:'test-only-synthetic-score',supported_configuration_defaults:{version:'1'},cases:Array.from({length:14},(_,i)=>({
    id:`case-${i}`,prompt,expected_disposition:i<5?'supported':i<8?'clarify':'unsupported',family:'esp32_bmp280_v1',profile:'standard',total_bus_capacitance_pf:100,review:'Synthetic grading fixture.',
    requirements:[{id:'sensor',description:'sensor test',any_of:[{kind:'sensor',value:'BMP280',state:'required'}]},{id:'cap',description:'quantity test',any_of:[{kind:'number',value:'total_bus_capacitance_pf',number:100}]}],
  }))};
  const selections=Object.fromEntries(spec.cases.map(c=>[c.id,{admission_version:admission,original_request:prompt,request_clauses:[{id:0,text:prompt}],
    source_quantities:[{id:0,clause_id:0,start:prompt.indexOf('100'),end:prompt.indexOf('100')+6,text:'100 pF',fields:{total_bus_capacitance_pf:100}}],
    raw_intent:{version:admission,facts:copy(facts)},extraction_outcome:'decision',decision:{disposition:c.expected_disposition,message:'Synthetic decision.',configuration:c.expected_disposition==='supported'?{version:'1',family:c.family,profile:c.profile,total_bus_capacitance_pf:100}:null,clauses:[{text:prompt,disposition:c.expected_disposition,reason:'Synthetic reason.'}]},
  }]));
  const records=spec.cases.map(c=>({id:c.id,state:'recorded-outcome',advance:true,collection_class:'recorded-decision-semantic-review-pending',execution:{wall_seconds:1},...(c.expected_disposition==='supported'?{bundle:{files_compared:{'fixture':'synthetic-only'}}}:{})}));
  const state={mode:'offline',status:'collection-complete-semantic-review-pending',records,recorded_outcomes:14};
  const bindings={selection_sha256:Object.fromEntries(Object.entries(selections).map(([k,s])=>[k,hashBytes(JSON.stringify(s))]))};
  return {spec,state,selections,schema,bindings};
}
function simulatedReview(inputs) {
  const review=reviewTemplate(inputs);
  review.status='source-bound-semantic-review';review.reviewer={kind:'implementing-agent',name:'SYNTHETIC TEST FIXTURE — not a real reviewer'};review.reviewed_utc='2026-09-14T00:00:00Z';
  for(const r of review.cases) {
    r.raw_complete=true;r.notes='Simulated judgment for scorer test only.';
    for(const f of r.facts){f.meaning_correct=true;f.source_scope_correct=true;f.notes='Simulated fact judgment.';}
    const c=inputs.spec.cases.find(c=>c.id===r.id),s=inputs.selections[r.id];
    for(const [i,q] of r.requirements.entries()) {q.fact_indices=s?matchingFacts(c.requirements[i],s.raw_intent.facts,s.source_quantities):[];q.passed=q.fact_indices.length>0;q.notes='Simulated requirement judgment.';}
    r.decision={correct:true,targeted_or_truthful:true,notes:'Simulated decision judgment.'};
  }
  return review;
}
const score=(inputs,review=simulatedReview(inputs))=>scoreReview({...inputs,review});

test('plan retains exactly the original 14 prompts, strict thresholds and no live approval',()=>{
  const {plan,spec}=loadPlan('specs/board-family-v2/source-reference-candidate-03/evaluation-plan.json');
  assert.equal(spec.cases.length,14);assert.equal(spec.cases.filter(c=>c.expected_disposition==='supported').length,5);
  assert.equal(plan.status,'preparation-no-live-approval');assert.equal(plan.proposed_policy.max_requests,14);
});
test('complete synthetic offline score cannot establish live acceptance',()=>{
  const result=score(fixture());assert.equal(result.complete_passes,14);assert.equal(result.acceptance,'offline-only-cannot-establish-live-acceptance');
});
test('pure scoring logic requires all 14, all native bundles and timing for live acceptance',()=>{
  const x=fixture();x.state.mode='live';assert.equal(score(x).acceptance,'all-14-first-attempt-acceptance-met');
  delete x.state.records[0].bundle;assert.equal(score(x).complete_passes,13);assert.equal(score(x).acceptance,'complete-acceptance-not-met');
});
test('template contains full source, quantities, decision and gold but no approved judgments',()=>{
  const x=fixture(),r=reviewTemplate(x);assert.equal(r.status,'pending-source-bound-review');
  assert.equal(r.cases[0].original_prompt,x.spec.cases[0].prompt);assert.deepEqual(r.cases[0].facts[1].quantity_context,[x.selections['case-0'].source_quantities[0]]);
  assert.equal(r.cases[0].facts[1].meaning_correct,false);assert.throws(()=>score(x,r));
});
for(const state of ['forbidden','not_required','uncertain'])test(`numeric ${state} cannot satisfy a positive gold magnitude`,()=>{
  const x=fixture();x.selections['case-0'].raw_intent.facts[1].state=state;
  assert.equal(assessIndexedRaw(x.spec.cases[0],x.selections['case-0'],schema).required_facts_present,false);assert.equal(score(x).raw_passes,13);
});
for(const [name,change] of [
  ['invented magnitude',s=>{s.raw_intent.facts[1].number=100;}],
  ['wrong conversion',s=>{s.source_quantities[0].fields.total_bus_capacitance_pf=1000;}],
  ['wrong role',s=>{s.raw_intent.facts[1].value='clock_hz';}],
  ['out-of-range source',s=>{s.raw_intent.facts[1].sources=[1];}],
  ['duplicate source',s=>{s.raw_intent.facts[1].sources=[0,0];}],
  ['out-of-range quantity',s=>{s.raw_intent.facts[1].quantities=[1];}],
  ['wrong quantity span',s=>{s.source_quantities[0].start=0;}],
  ['historical version',s=>{s.raw_intent.version='2';}],
])test(`${name} fails automatic raw checks`,()=>{
  const x=fixture();change(x.selections['case-0']);assert.equal(assessIndexedRaw(x.spec.cases[0],x.selections['case-0'],schema).automatic_checks_pass,false);
});
test('valid-looking invented feature remains semantic failure despite matching all gold',()=>{
  const x=fixture();x.selections['case-0'].raw_intent.facts.push({kind:'feature',value:'custom_geometry',state:'required',sources:[0],quantities:[]});
  assert.equal(assessIndexedRaw(x.spec.cases[0],x.selections['case-0'],schema).automatic_checks_pass,true);
  const r=simulatedReview(x);r.cases[0].facts[2].meaning_correct=false;r.cases[0].facts[2].notes='No geometry requirement in original prompt.';
  assert.equal(score(x,r).raw_passes,13);
});
for(const key of ['meaning_correct','source_scope_correct'])test(`review cannot pass a requirement supported only by a fact with ${key}=false`,()=>{
  const x=fixture(),r=simulatedReview(x);r.cases[0].facts[0][key]=false;assert.throws(()=>score(x,r),/supporting fact/);
  r.cases[0].requirements[0].passed=false;assert.equal(score(x,r).raw_passes,13);
});
test('omitted constraints and untruthful refusal cannot be hidden by automatic matches',()=>{
  const x=fixture(),r=simulatedReview(x);r.cases[0].omitted_requirements=['An omitted additional requirement'];r.cases[8].decision.targeted_or_truthful=false;
  assert.equal(score(x,r).complete_passes,12);
});
test('unattempted and completed invalid answers remain in the full denominator',()=>{
  const x=fixture();x.state.records=x.state.records.slice(0,3);x.state.recorded_outcomes=3;x.state.status='stopped-no-retry';
  x.state.records[0].collection_class='recorded-model-failure';x.selections['case-0'].extraction_outcome='invalid_extraction';
  const result=score(x);assert.equal(result.planned_cases,14);assert.equal(result.complete_passes,2);assert.equal(result.median_all_useful_seconds,null);assert.equal(result.timing_target_met,false);
});
for(const [name,mutate] of [
  ['changed binding',r=>{r.bindings={selection_sha256:{}};}],
  ['edited prompt context',r=>{r.cases[0].original_prompt='fake';}],
  ['edited quantity context',r=>{r.cases[0].facts[1].quantity_context=[];}],
  ['edited requirement',r=>{r.cases[0].requirements[0].requirement={};}],
  ['dropped case',r=>{r.cases.pop();}],
  ['dropped fact',r=>{r.cases[0].facts.pop();}],
  ['missing rationale',r=>{r.cases[0].facts[0].notes='';}],
  ['missing reviewer',r=>{r.reviewer.name='';}],
  ['invalid timestamp',r=>{r.reviewed_utc=null;}],
])test(`${name} invalidates review`,()=>{const x=fixture(),r=copy(simulatedReview(x));mutate(r);assert.throws(()=>score(x,r));});
for(const times of [[1,2,60,70,80],[1,2,3,4,120],[1,2,3,4,null]])test(`useful timing is strict, complete and includes slow failures: ${times}`,()=>{
  const x=fixture();times.forEach((n,i)=>{x.state.records[i].execution.wall_seconds=n;});assert.equal(score(x).timing_target_met,false);
});
test('closed schema rejects non-finite values and extra properties',()=>{
  assert.equal(conforms(Infinity,{type:'number'}),false);assert.equal(conforms({x:1,y:2},object({x:{type:'integer'}})),false);
});
