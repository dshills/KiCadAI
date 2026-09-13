// Read-only QA of retained bytes, original dispositions, denominators and billing estimates.
import fs from 'node:fs';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const read=p=>JSON.parse(fs.readFileSync(p)),hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const base='specs/board-family-v1',live=base+'/evidence/live';
let checked=0;
for(const name of ['offline','live']){
 const m=read(`${base}/evidence/${name}/manifest.json`);
 for(const [f,sha] of Object.entries(m.files)){assert.equal(hash(f),sha,f);checked++}
}
const spec=read(base+'/evaluation/language.json'),ledger=read(live+'/ledger.json'),assessment=read(live+'/assessment.json');
const first=new Map(),latest=new Map(),indices=new Set();
let physical=0;
for(const run of ['acceptance-language-01','acceptance-language-02','acceptance-language-03']){
 const summary=read(`${live}/${run}/summary.json`);assert.equal(summary.spec_sha256,hash(base+'/evaluation/language.json'));
 for(const c of summary.cases){
  const p=`${live}/${run}/${c.id}`,selected=read(p+'/selection.json'),output=read(p+'.stdout.log'),expected=spec.cases.find(x=>x.id===c.id);
  assert.deepEqual(c.result,output);assert.equal(fs.readFileSync(p+'.txt','utf8'),expected.prompt);
  assert(!indices.has(selected.ledger_index));indices.add(selected.ledger_index);physical++;
  const entry=ledger.entries[selected.ledger_index-1];assert(entry);assert.equal(entry.index,selected.ledger_index);
  assert.equal(selected.response_id||'',entry.response_id||'');
  assert.equal(selected.usage?.input_tokens||0,entry.input_tokens||0);assert.equal(selected.usage?.output_tokens||0,entry.output_tokens||0);
  const d=selected.decision;
  let selectionMatch=false,passed=false;
  if(expected.expected_disposition==='supported'){
   const want={...spec.configuration_expectations,profile:expected.expected_profile,total_bus_capacitance_pf:spec.capacitance_overrides_pf[c.id]??(expected.expected_profile==='standard'?200:100)};
   selectionMatch=d?.disposition==='supported'&&!!d.configuration&&Object.entries(want).every(([k,v])=>d.configuration[k]===v);
   let native=false;if(fs.existsSync(p+'/validation.json')){const v=read(p+'/validation.json');native=v.passed===true&&v.checks.length===13&&v.checks.every(x=>x.passed===true)}
   passed=selectionMatch&&c.exit_code===0&&output.passed===true&&native&&c.wall_seconds<600;
  }else{
   passed=c.exit_code===0&&d?.disposition===expected.expected_disposition&&d.configuration===null&&output.passed===false&&c.no_native_design===true;
  }
  assert.equal(c.passed,passed,`${run}/${c.id} acceptance`);
  const row={id:c.id,kind:expected.expected_disposition,selectionMatch,passed,seconds:c.wall_seconds,result:output};
  if(!first.has(c.id))first.set(c.id,row);latest.set(c.id,row);
 }
}
assert.equal(physical,ledger.entries.length);assert.equal(first.size,16);
const count=(kind,field)=>[...first.values()].filter(x=>x.kind===kind&&x[field]).length;
assert.equal(count('supported','selectionMatch'),assessment.supported_first_selection_matches);
assert.equal(count('supported','passed'),assessment.supported_first_completed_boards);
assert.equal(count('unsupported','passed'),assessment.unsupported_first_passes);
assert.equal(count('clarify','passed'),assessment.clarification_first_passes);
const accepted=[...latest.values()].filter(x=>x.kind==='supported'&&x.passed),times=accepted.map(x=>x.seconds).sort((a,b)=>a-b);
assert.equal(times.length,assessment.timing_count);assert.equal((times[3]+times[4])/2,assessment.median_completed_wall_seconds);
assert.equal(Math.max(...times),assessment.max_completed_wall_seconds);
let known=0,unknown=0;
for(const e of ledger.entries){if(e.input_tokens&&e.output_tokens){const n=Math.ceil((2*e.input_tokens+8*e.output_tokens)/5);assert.equal(n,e.estimated_micro_usd);known+=n}else{assert.equal(e.status,'failed_or_unknown');unknown+=e.reserve_micro_usd}}
assert.equal(known,assessment.known_estimated_micro_usd);assert.equal(unknown,assessment.unknown_reserved_micro_usd);assert.equal(known+unknown,assessment.conservative_total_micro_usd);
assert.equal(assessment.overall_passed,false);
console.log(JSON.stringify({verified_file_hashes:checked,physical_requests:physical,first_selections:count('supported','selectionMatch'),first_complete_boards:count('supported','passed'),unsupported:count('unsupported','passed'),clarify:count('clarify','passed'),conservative_micro_usd:known+unknown}));
