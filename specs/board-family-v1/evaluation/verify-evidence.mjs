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

// The final packet is additional evidence, never a replacement live score.
{
 const dir=base+'/evidence/acceptance',a=read(dir+'/assessment.json'),l=read(dir+'/ledger.json'),freshSpec=read(base+'/evaluation/language-holdout-proposed.json');
 for(const [f,sha] of Object.entries(read(dir+'/manifest.json').files)){assert.equal(hash(f),sha,f);checked++}
 assert.deepEqual(l.entries.slice(0,17),ledger.entries);assert.equal(l.entries.length,35);assert(!l.halt_reason);
 assert.equal(a.contract_sha256,hash(base+'/evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json'));assert.equal(a.holdout_sha256,hash(base+'/evaluation/language-holdout-proposed.json'));
 const rows=[],seen=new Set();
 for(const run of a.runs){
  const s=read(dir+'/'+run.run+'/summary.json'),fresh=run.run!=='recovery-checks-01',sp=fresh?freshSpec:spec;
  for(const k of ['source_commit','binary_sha256','runner_sha256'])assert.equal(s[k],run[k]);
  assert.equal(s.passed,false);if(!fresh)assert.equal(s.development_checks_passed,true);
  if(s.prior_run){const prior=s.prior_run.summary_path.split('/').slice(-2,-1)[0];const prefix=prior.startsWith('acceptance-language')?live:dir;assert.equal(hash(prefix+'/'+prior+'/summary.json'),s.prior_run.sha256)}
  for(const c of s.cases){
   const p=dir+'/'+run.run+'/'+c.id,d=read(p+'/selection.json'),o=read(p+'.stdout.log'),e=l.entries[d.ledger_index-1],w=sp.cases.find(x=>x.id===c.id);assert(w);
   assert.equal(hash(p+'/selection.json'),c.selection_sha256);assert.equal(fs.readFileSync(p+'.txt','utf8'),w.prompt);assert.deepEqual(o,c.result);
   assert(!seen.has(d.ledger_index));seen.add(d.ledger_index);assert.equal(c.first_attempt,fresh);
   assert.equal(d.response_id,e.response_id);assert.equal(d.model,e.model);assert.equal(d.usage.input_tokens,e.input_tokens);assert.equal(d.usage.output_tokens,e.output_tokens);
   let pass;
   if(w.expected_disposition==='supported'){
    const want={...sp.configuration_expectations,profile:w.expected_profile,total_bus_capacitance_pf:sp.capacitance_overrides_pf[c.id]??(w.expected_profile==='standard'?200:100)};
    assert.equal(d.decision.disposition,'supported');assert.deepEqual(d.decision.configuration,want);assert.deepEqual(read(p+'/configuration.json'),want);
    const v=read(p+'/validation.json');pass=c.exit_code===0&&o.passed===true&&v.passed===true&&v.checks.length===13&&v.checks.every(x=>x.passed)&&c.wall_seconds<600;
    for(const [f,sha] of Object.entries(v.native_sha256))assert.equal(hash('examples/board-family-v1/'+want.profile+'/'+f),sha);
   }else pass=c.exit_code===0&&d.decision.disposition===w.expected_disposition&&d.decision.configuration===null&&o.passed===false&&c.no_native_design===true;
   assert.equal(c.passed,pass,run.run+'/'+c.id);
   if(fresh)rows.push({id:c.id,kind:w.expected_disposition,pass,c,o,selection:d,run:run.run});
  }
 }
 assert.deepEqual([...seen].sort((x,y)=>x-y),Array.from({length:18},(_,i)=>i+18));assert.equal(rows.length,16);assert.equal(new Set(rows.map(x=>x.id)).size,16);
 for(const [kind,key] of [['supported','supported_first_completed_boards'],['unsupported','unsupported_first_passes'],['clarify','clarification_first_passes']])assert.equal(rows.filter(x=>x.kind===kind&&x.pass).length,a[key]);
 assert.equal(rows.filter(x=>x.pass).length,a.strict_live_passes);assert.equal(a.strict_live_acceptance_passed,false);assert.equal(a.goal_complete,false);
 const median=xs=>{const x=[...xs].sort((u,v)=>u-v);return (x[4]+x[5])/2},supported=rows.filter(x=>x.kind==='supported');assert.equal(supported.length,10);
 assert.equal(median(supported.map(x=>x.c.wall_seconds)),a.median_end_to_end_seconds);assert.equal(Math.max(...supported.map(x=>x.c.wall_seconds)),a.max_end_to_end_seconds);
 assert.equal(median(supported.map(x=>x.o.generation_seconds)),a.median_generation_seconds);assert.equal(median(supported.map(x=>x.o.validation_seconds)),a.median_validation_seconds);
 const replay=read(dir+'/decision-replay-01/summary.json');assert.equal(replay.passed,true);assert.equal(replay.api_requests,0);assert.equal(replay.case_count,16);assert.equal(new Set(replay.cases.map(x=>x.id)).size,16);
 for(const r of replay.cases){
  const original=rows.find(x=>x.id===r.id);assert(original);assert.equal(r.source_response_id,original.selection.response_id);assert.equal(r.ledger_index,original.selection.ledger_index);
  assert.equal(r.source_selection_sha256,hash(dir+'/'+original.run+'/'+r.id+'/selection.json'));assert.equal(r.original_live_exit_code,original.c.exit_code);assert.equal(r.original_live_passed,original.pass);
  assert.equal(r.current_decision.disposition,original.kind);assert.equal(r.api_requests,0);assert.equal(r.native_generation_attempted,false);assert.equal(r.passed,true);
  if(original.kind==='supported')assert.deepEqual(r.current_decision.configuration,original.selection.decision.configuration);else assert.equal(r.current_decision.configuration,null);
  assert.equal(r.current_decision.clauses.map(x=>x.text).join(''),freshSpec.cases.find(x=>x.id===r.id).prompt);
  assert.deepEqual(read(dir+'/decision-replay-01/'+r.id+'.json'),r);
 }
 let k=0,u=0,it=0,ot=0;for(const [i,e] of l.entries.entries()){assert.equal(e.index,i+1);if(e.status==='completed'){const n=Math.ceil((2*e.input_tokens+8*e.output_tokens)/5);assert.equal(n,e.estimated_micro_usd);k+=n;it+=e.input_tokens;ot+=e.output_tokens}else{assert.equal(e.status,'failed_or_unknown');u+=e.reserve_micro_usd}}
 assert.equal(k,a.known_estimated_micro_usd);assert.equal(u,a.unknown_reserved_micro_usd);assert.equal(k+u,a.conservative_total_micro_usd);assert.equal(it,a.known_input_tokens);assert.equal(ot,a.known_output_tokens);
 for(const [f,sha] of Object.entries({...a.reused_unchanged_qualification_source_sha256,...a.corrected_source_sha256}))assert.equal(hash(f),sha,f);
 assert.equal(read(dir+'/bounded-regression.json').exit_code,0);
 console.log(JSON.stringify({verified_file_hashes:checked,goal_physical_requests:l.entries.length,strict_live_passes:a.strict_live_passes,strict_live_total:16,corrected_offline_replays:16,conservative_micro_usd:k+u,goal_complete:false}));
}
