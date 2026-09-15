// Pure tests: these synthetic judgments are NOT published semantic reviews.
import test from 'node:test';
import assert from 'node:assert/strict';
import {admission,factContext,assessOwnedRaw,reviewTemplate,scoreReview,matchingFacts,loadCorpus} from './scoring.mjs';
const copy=x=>structuredClone(x);
const object=properties=>({type:'object',additionalProperties:false,required:Object.keys(properties),properties});
const string=(...values)=>({type:'string',...(values.length?{enum:values}:{})});
const refs=(values,min)=>({type:'array',minItems:min,maxItems:values.length,items:string(...values)});
const states=string('required','not_required','forbidden','uncertain');

export function fixture() {
  const prompt='Please use BMP280. With 100 pF total capacitance.';
  const clauses=[{id:0,text:'Please use BMP280.'},{id:1,text:' With 100 pF total capacitance.'}];
  const quantities=[{id:0,clause_id:1,start:prompt.indexOf('100'),end:prompt.indexOf('100')+6,text:'100 pF',fields:{total_bus_capacitance_pf:100}}];
  const schema=object({version:string(admission),facts:{type:'array',maxItems:64,items:{anyOf:[
    object({kind:string('sensor'),value:string('BMP280'),state:states,evidence:refs(['c0','c1'],1)}),
    object({kind:string('number'),choice:string('q0/total_bus_capacitance_pf'),state:states,context:refs(['c0','c1'],0)}),
    object({kind:string('feature'),value:string('wireless_operation','accuracy_guarantee'),state:states,evidence:refs(['c0','c1','q0'],1)}),
    object({kind:string('other'),detail:string(),state:states,evidence:refs(['c0','c1','q0'],1)}),
  ]}}});
  const contract={admission_version:admission,request_revision:'owned-request-05',schema_name:'board_family_owned_requirements_v4',model:'gpt-4.1-mini-2025-04-14',destination:'https://api.openai.com/v1/responses',experimental:true,max_output_tokens:1600,max_request_bytes:65536,capability_context:'Synthetic scorer test only',source:{request:prompt,clauses,quantities},schema};
  const spec={evaluation_id:'synthetic-scorer-test',supported_configuration_defaults:{version:'1'},cases:Array.from({length:14},(_,i)=>({
    id:'case-'+i,prompt,expected_disposition:i<5?'supported':i<8?'clarify':'unsupported',family:'esp32_bmp280_v1',profile:'standard',total_bus_capacitance_pf:100,review:'Synthetic reviewer rubric, not actual gold.',
    requirements:[{id:'sensor',description:'Test sensor',any_of:[{kind:'sensor',value:'BMP280',state:'required'}]},{id:'cap',description:'Test capacitance',any_of:[{kind:'number',value:'total_bus_capacitance_pf',number:100}]}],
  }))};
  const selections=Object.fromEntries(spec.cases.map(c=>[c.id,{admission_version:admission,original_request:prompt,request_clauses:copy(clauses),source_quantities:copy(quantities),extraction_outcome:'decision',
    raw_intent:{version:admission,facts:[{kind:'sensor',value:'BMP280',state:'required',evidence:['c0']},{kind:'number',choice:'q0/total_bus_capacitance_pf',state:'required',context:['c0']}]},
    decision:{disposition:c.expected_disposition,message:'Synthetic test decision',configuration:c.expected_disposition==='supported'?{version:'1',family:c.family,profile:c.profile,total_bus_capacitance_pf:100}:null,clauses:[{text:prompt,disposition:c.expected_disposition,reason:'Synthetic test reason'}]}}]));
  const contracts=Object.fromEntries(spec.cases.map(c=>[c.id,copy(contract)]));
  const state={mode:'offline',planned_cases:14,recorded_outcomes:14,status:'collection-complete-semantic-review-pending',records:spec.cases.map(c=>({
    id:c.id,state:'recorded-outcome',advance:true,collection_class:'recorded-decision-semantic-review-pending',execution:{wall_seconds:1},
    ...(c.expected_disposition==='supported'?{bundle:{files_compared:{fixture:'test-only; not a native bundle'}}}:{})}))};
  const bindings={selection_sha256:Object.fromEntries(spec.cases.map(c=>[c.id,'synthetic-selection-binding'])),contract_sha256:Object.fromEntries(spec.cases.map(c=>[c.id,'synthetic-contract-binding']))};
  return {spec,state,selections,contracts,bindings};
}
export function simulatedReview(x) {
  const r=reviewTemplate(x);r.status='source-bound-semantic-review';r.reviewer={kind:'implementing-agent',name:'SYNTHETIC TEST INPUT — not a reviewer'};r.reviewed_utc='2026-09-15T00:00:00Z';
  for(const c of r.cases) {
    c.raw_complete=true;c.notes='Synthetic scorer control.';
    for(const f of c.facts) {f.meaning_correct=true;f.source_scope_correct=!f.resolution_error;f.notes='Synthetic fact judgment.';}
    const raw=assessOwnedRaw(x.spec.cases.find(k=>k.id===c.id),x.selections[c.id],x.contracts[c.id]);
    for(const [j,q] of c.requirements.entries()) {q.fact_indices=raw.requirements[j].matching_fact_indices;q.passed=q.fact_indices.length>0;q.notes='Synthetic matching judgment.';}
    c.decision={correct:true,targeted_or_truthful:true,notes:'Synthetic decision judgment.'};
  }
  return r;
}
const score=(x,r=simulatedReview(x))=>scoreReview({...x,review:r});
const raw=x=>assessOwnedRaw(x.spec.cases[0],x.selections['case-0'],x.contracts['case-0']);

test('known corpus and thresholds remain unchanged',()=>{const s=loadCorpus();assert.equal(s.cases.length,14);assert.equal(s.cases.filter(c=>c.expected_disposition==='supported').length,5);});
test('full synthetic success is offline-only; forged live mode never grants acceptance',()=>{
  const x=fixture();assert.equal(score(x).complete_passes,14);assert.equal(score(x).criteria_met,true);assert.equal(score(x).acceptance,'offline-only-cannot-establish-live-acceptance');
  x.state.mode='live';assert.equal(score(x).acceptance,'not-established-by-scoring-alone');
});
test('template preserves raw v4 numeric choice, adds owner/context annotations, and approves nothing',()=>{
  const x=fixture(),before=copy(x),r=reviewTemplate(x),f=r.cases[0].facts[1];
  assert.equal(r.status,'pending-source-bound-review');assert.equal(f.fact.choice,'q0/total_bus_capacitance_pf');assert.equal(f.fact.number,undefined);
  assert.deepEqual(f.source_context,x.contracts['case-0'].source.clauses);assert.equal(f.numeric_choice.value,100);assert.equal(f.numeric_choice.clause_id,1);
  assert.deepEqual(f.quantity_context,x.selections['case-0'].source_quantities);assert.equal(f.meaning_correct,false);assert.deepEqual(x,before);assert.throws(()=>score(x,r));
});
test('duplicate alias sets preserve raw arrays while resolving one owner and one occurrence',()=>{
  const x=fixture(),f=x.selections['case-0'].raw_intent.facts[1];f.context=['c0','c0'];assert.equal(raw(x).structure_pass,true);
  const context=factContext(f,x.contracts['case-0']);assert.deepEqual(context.fact.context,['c0','c0']);assert.deepEqual(context.source_context.map(c=>c.id),[0,1]);
  const feature={kind:'feature',value:'accuracy_guarantee',state:'required',evidence:['q0','q0','c1']};
  assert.deepEqual(factContext(feature,x.contracts['case-0']).quantity_context.map(q=>q.id),[0]);
});
test('editing a template cannot mutate its authenticated input evidence',()=>{
  const x=fixture(),before=copy(x),r=reviewTemplate(x);
  r.cases[0].raw_intent.facts[0].value='changed';r.cases[0].decision_context.message='changed';
  assert.deepEqual(x,before);
});
for(const state of ['forbidden','not_required','uncertain'])test('numeric '+state+' cannot satisfy a positive requirement',()=>{
  const x=fixture();x.selections['case-0'].raw_intent.facts[1].state=state;assert.equal(raw(x).structure_pass,true);assert.equal(raw(x).automatic_checks_pass,false);assert.equal(score(x).raw_passes,13);
});
for(const [name,change] of [
  ['invented magnitude',x=>{x.selections['case-0'].raw_intent.facts[1].number=100;}],
  ['wrong wire version',x=>{x.selections['case-0'].raw_intent.version='3-indexed-quantities-experimental';}],
  ['wrong numeric choice',x=>{x.selections['case-0'].raw_intent.facts[1].choice='q0/clock_hz';}],
  ['quantity ownership tamper',x=>{x.selections['case-0'].source_quantities[0].clause_id=0;}],
  ['quantity conversion tamper',x=>{x.selections['case-0'].source_quantities[0].fields.total_bus_capacitance_pf=1000;}],
  ['changed original prompt',x=>{x.selections['case-0'].original_request='changed';}],
  ['invalid source alias',x=>{x.selections['case-0'].raw_intent.facts[0].evidence=['c2'];}],
  ['noncanonical alias',x=>{x.selections['case-0'].raw_intent.facts[0].evidence=['c00'];}],
  ['identity quantity reference',x=>{x.selections['case-0'].raw_intent.facts[0].evidence=['q0'];}],
  ['missing reference array',x=>{delete x.selections['case-0'].raw_intent.facts[1].context;}],
  ['null reference array',x=>{x.selections['case-0'].raw_intent.facts[1].context=null;}],
  ['too many aliases',x=>{x.selections['case-0'].raw_intent.facts[1].context=['c0','c0','c0'];}],
])test(name+' fails raw structure',()=>{const x=fixture();change(x);assert.equal(raw(x).structure_pass,false);assert.equal(score(x).raw_passes,13);});
test('all quantity occurrences must be retained, even if gold magnitude already matches',()=>{
  const x=fixture(),c=x.contracts['case-0'],s=x.selections['case-0'];
  const extra={...copy(c.source.quantities[0]),id:1};c.source.quantities.push(extra);s.source_quantities.push(copy(extra));
  assert.equal(raw(x).structure_pass,true);assert.equal(raw(x).required_facts_present,true);assert.deepEqual(raw(x).unreferenced_quantity_ids,[1]);assert.equal(raw(x).automatic_checks_pass,false);
});
test('valid invented feature is semantic failure despite automatic gold matches',()=>{
  const x=fixture();x.selections['case-0'].raw_intent.facts.push({kind:'feature',value:'wireless_operation',state:'required',evidence:['c0']});
  assert.equal(raw(x).automatic_checks_pass,true);
  const r=simulatedReview(x);r.cases[0].facts[2].meaning_correct=false;r.cases[0].facts[2].notes='No radio requirement in source.';
  assert.equal(score(x,r).raw_passes,13);assert.equal(score(x,r).application_passes,14);
});
for(const key of ['meaning_correct','source_scope_correct'])test('a requirement cannot use a fact rejected for '+key,()=>{
  const x=fixture(),r=simulatedReview(x);r.cases[0].facts[0][key]=false;assert.throws(()=>score(x,r),/supporting fact/);
  r.cases[0].requirements[0].passed=false;assert.equal(score(x,r).raw_passes,13);
});
test('missing constraints and misleading decisions independently lower complete success',()=>{
  const x=fixture(),r=simulatedReview(x);r.cases[0].omitted_requirements=['Unrepresented constraint'];r.cases[8].decision.targeted_or_truthful=false;
  assert.equal(score(x,r).complete_passes,12);assert.equal(score(x,r).raw_passes,13);assert.equal(score(x,r).application_passes,13);
});
test('invalid extraction and unattempted cases never disappear from denominator or timing',()=>{
  const x=fixture();x.state.records=x.state.records.slice(0,3);x.state.recorded_outcomes=3;x.state.status='stopped-no-retry';
  x.state.records[0].collection_class='recorded-model-failure';x.selections['case-0'].extraction_outcome='invalid_extraction';
  const r=score(x);assert.equal(r.planned_cases,14);assert.equal(r.complete_passes,2);assert.equal(r.median_all_useful_seconds,null);assert.equal(r.criteria_met,false);
});
test('missing native evidence and slow failed useful cases cannot be omitted',()=>{
  const x=fixture();delete x.state.records[0].bundle;assert.equal(score(x).application_passes,13);
  x.state.records[0].execution.wall_seconds=120;assert.equal(score(x).timing_target_met,false);assert.equal(score(x).maximum_all_useful_seconds,120);
});
for(const [name,mutate] of [
  ['binding',r=>{r.bindings.selection_sha256={};}],
  ['prompt',r=>{r.cases[0].original_prompt='changed';}],
  ['raw fact',r=>{r.cases[0].facts[1].fact.choice='q1/total_bus_capacitance_pf';}],
  ['resolved owner',r=>{r.cases[0].facts[1].numeric_choice.clause_id=0;}],
  ['source context',r=>{r.cases[0].facts[1].source_context=[];}],
  ['contract hash',r=>{r.cases[0].contract_sha256='changed';}],
  ['gold requirement',r=>{r.cases[0].requirements[0].requirement.description='changed';}],
  ['fact omission',r=>{r.cases[0].facts.pop();}],
  ['blank judgment',r=>{r.cases[0].notes='';}],
])test('changed review '+name+' is rejected',()=>{const x=fixture(),r=simulatedReview(x);mutate(r);assert.throws(()=>score(x,r));});
test('numeric gold matches field/value/state through annotations, never model magnitudes',()=>{
  const x=fixture(),c=x.spec.cases[0],s=x.selections[c.id],contexts=s.raw_intent.facts.map(f=>factContext(f,x.contracts[c.id]));
  assert.deepEqual(matchingFacts(c.requirements[1],contexts),[1]);
  contexts[1].numeric_choice.value=70;assert.deepEqual(matchingFacts(c.requirements[1],contexts),[]);
});
