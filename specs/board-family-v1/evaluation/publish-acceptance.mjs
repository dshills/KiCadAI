// Publish retained evidence once. Failed live outcomes are never rewritten.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const base='specs/board-family-v1', cache='.cache/board-family-v1', dest=base+'/evidence/acceptance';
const read=p=>JSON.parse(fs.readFileSync(p)), hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const median=xs=>{const x=[...xs].sort((a,b)=>a-b);return (x[Math.floor((x.length-1)/2)]+x[Math.floor(x.length/2)])/2};
assert(!fs.existsSync(dest),'publication must be new');
const copies=[],files={},records=[],native=[],runs=[],indices=new Set();
const queue=(src,dst)=>{assert(fs.existsSync(src),src);assert(!fs.existsSync(dst),dst);copies.push([src,dst,hash(src)])};
const ledger=read(cache+'/live-ledger.json');
assert.equal(ledger.entries.length,35);assert(!ledger.halt_reason);
assert.deepEqual(ledger.entries.slice(0,17),read(base+'/evidence/live/ledger.json').entries);
const contract=base+'/evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json',holdout=base+'/evaluation/language-holdout-proposed.json';
assert.equal(hash(contract),'76cf6b0a3965c18b81504ac67a1e5dbf3b9eec86b10e4a8ba8581da2e7229a9c');
assert.equal(hash(holdout),'1b2daa0c91e8a76ce1c6cb2524d247696af3ef7f0033da9a5bf4b403588f97e4');
const checks=['electrical_contract','reference_and_bom_integrity','pcb_writer_and_connectivity','schematic_writer','native_parse_render','kicad_version','kicad_erc','kicad_strict_drc_and_parity','kicad_roundtrip_kicad_sch','kicad_roundtrip_kicad_pcb','schematic_preview','pcb_preview','input_immutability'].sort();
for(const run of ['recovery-checks-01','acceptance-holdout-01','acceptance-holdout-02','acceptance-holdout-03']){
 const src=cache+'/'+run,s=read(src+'/summary.json'),fresh=run!=='recovery-checks-01';
 const specFile=fresh?holdout:base+'/evaluation/language.json',spec=read(specFile);
 assert.equal(s.spec_sha256,hash(specFile));assert.equal(s.contract_sha256,hash(contract));
 assert.equal(s.runner_sha256,hash(base+'/evaluation/run-language.mjs'));
 if(s.prior_run)assert.equal(hash(s.prior_run.summary_path),s.prior_run.sha256);
 assert.equal(s.passed,false);if(!fresh)assert.equal(s.development_checks_passed,true);
 runs.push({run,source_commit:s.source_commit,binary_sha256:s.binary_sha256,runner_sha256:s.runner_sha256});
 for(const f of ['summary.json','executed-contract.json'])queue(src+'/'+f,dest+'/'+run+'/'+f);
 for(const c of s.cases){
  const wanted=spec.cases.find(x=>x.id===c.id);assert(wanted);
  const p=src+'/'+c.id,selection=read(p+'/selection.json'),d=selection.decision,output=read(p+'.stdout.log');
  assert.equal(fs.readFileSync(p+'.txt','utf8'),wanted.prompt);assert.equal(c.first_attempt,fresh);
  assert.equal(hash(p+'/selection.json'),c.selection_sha256);assert.deepEqual(output,c.result);
  assert(!indices.has(selection.ledger_index));indices.add(selection.ledger_index);
  const e=ledger.entries[selection.ledger_index-1];
  for(const k of ['response_id','model'])assert.equal(e[k],selection[k]);
  for(const k of ['input_tokens','output_tokens'])assert.equal(e[k],selection.usage[k]);
  assert.equal(e.status,'completed');
  let nativePass=false;const nativePresent=fs.existsSync(p+'/board.kicad_pcb');
  if(nativePresent){
   const cfg=read(p+'/configuration.json'),v=read(p+'/validation.json');
   assert.equal(v.passed,true);assert.equal(v.kicad_version,'10.0.3');
   assert.deepEqual(v.checks.map(x=>x.name).sort(),checks);assert(v.checks.every(x=>x.passed));
   const erc=read(p+'/erc.json'),drc=read(p+'/drc.json');
   assert(erc.sheets.length>0&&erc.sheets.every(x=>Array.isArray(x.violations)&&x.violations.length===0));
   for(const k of ['violations','unconnected_items','schematic_parity'])assert.deepEqual(drc[k],[]);
   for(const [f,sha] of Object.entries(v.native_sha256)){assert.equal(hash(p+'/'+f),sha);assert.equal(hash('examples/board-family-v1/'+cfg.profile+'/'+f),sha)}
   for(const f of ['bom.json','bom.csv','fp-lib-table','sym-lib-table'])assert.equal(hash(p+'/'+f),hash('examples/board-family-v1/'+cfg.profile+'/'+f));
   nativePass=true;
   native.push({run,id:c.id,profile:cfg.profile,native_sha256:v.native_sha256,matches_reviewed_example:true,request_accepted:c.passed,deliverable:c.passed&&wanted.expected_disposition==='supported'});
  }else for(const f of ['board.kicad_pcb','board.kicad_sch','board.kicad_pro'])assert(!fs.existsSync(p+'/'+f));
  let selectionMatch=false,passed;
  if(wanted.expected_disposition==='supported'){
   const expected={...spec.configuration_expectations,profile:wanted.expected_profile,total_bus_capacitance_pf:spec.capacitance_overrides_pf[c.id]??(wanted.expected_profile==='standard'?200:100)};
   assert.equal(d.disposition,'supported');assert.deepEqual(d.configuration,expected);selectionMatch=true;
   assert.deepEqual(read(p+'/configuration.json'),expected);
   passed=c.exit_code===0&&output.passed===true&&nativePass&&c.wall_seconds<600;
  }else passed=c.exit_code===0&&d.disposition===wanted.expected_disposition&&d.configuration===null&&output.passed===false&&!nativePresent;
  assert.equal(c.passed,passed,run+'/'+c.id);
  records.push({run,id:c.id,expected:wanted.expected_disposition,first_attempt:fresh,selection_match:selectionMatch,passed,exit_code:c.exit_code,ledger_index:selection.ledger_index,wall_seconds:c.wall_seconds,generation_seconds:output.generation_seconds??null,validation_seconds:output.validation_seconds??null});
  for(const ext of ['txt','stdout.log','stderr.log'])queue(p+'.'+ext,dest+'/'+run+'/'+c.id+'.'+ext);
  for(const f of ['selection.json','configuration.json','electrical.json','validation.json','erc.json','erc.log','drc.json','drc.log','schematic-preview.log','pcb-preview.log'])if(fs.existsSync(p+'/'+f))queue(p+'/'+f,dest+'/'+run+'/'+c.id+'/'+f);
 }
}
assert.deepEqual([...indices].sort((a,b)=>a-b),Array.from({length:18},(_,i)=>i+18));
const fresh=records.filter(x=>x.first_attempt),supported=fresh.filter(x=>x.expected==='supported');
assert.equal(new Set(fresh.map(x=>x.id)).size,16);assert.equal(fresh.length,16);
const count=kind=>fresh.filter(x=>x.expected===kind&&x.passed).length;
assert.equal(count('supported'),10);assert.equal(count('unsupported'),2);assert.equal(count('clarify'),1);
assert.deepEqual(fresh.filter(x=>!x.passed).map(x=>x.id),['fresh-unsupported-02','fresh-unsupported-04','fresh-ambiguous-01']);
const replay=read(cache+'/decision-replay-01/summary.json');
assert.equal(replay.api_requests,0);assert.equal(replay.case_count,16);assert.equal(replay.passed,true);
assert.equal(replay.spec_sha256,hash(holdout));assert.equal(new Set(replay.cases.map(x=>x.id)).size,16);
for(const r of replay.cases){
 const c=fresh.find(x=>x.id===r.id),p=cache+'/'+r.run+'/'+r.id,selected=read(p+'/selection.json');assert(c);
 assert.equal(r.source_selection_sha256,hash(p+'/selection.json'));assert.equal(r.source_response_id,selected.response_id);
 assert.equal(r.ledger_index,c.ledger_index);assert.equal(r.original_live_exit_code,c.exit_code);assert.equal(r.original_live_passed,c.passed);
 assert.equal(r.current_decision.disposition,c.expected);assert.equal(r.passed,true);assert.equal(r.api_requests,0);assert.equal(r.native_generation_attempted,false);
 if(c.expected==='supported')assert.deepEqual(r.current_decision.configuration,selected.decision.configuration);else assert.equal(r.current_decision.configuration,null);
 assert.equal(r.current_decision.clauses.map(x=>x.text).join(''),fs.readFileSync(p+'.txt','utf8'));
 assert.deepEqual(read(cache+'/decision-replay-01/'+r.id+'.json'),r);
}
for(const f of fs.readdirSync(cache+'/decision-replay-01'))queue(cache+'/decision-replay-01/'+f,dest+'/decision-replay-01/'+f);
let known=0,unknown=0,input=0,output=0;
for(const [i,e] of ledger.entries.entries()){
 assert.equal(e.index,i+1);
 if(e.status==='completed'){
  assert(Number.isSafeInteger(e.input_tokens)&&e.input_tokens>0&&Number.isSafeInteger(e.output_tokens)&&e.output_tokens>0);
  const cost=Math.ceil((e.input_tokens*2+e.output_tokens*8)/5);assert.equal(e.estimated_micro_usd,cost);
  known+=cost;input+=e.input_tokens;output+=e.output_tokens;
 }else{assert.equal(e.status,'failed_or_unknown');unknown+=e.reserve_micro_usd}
}
const unchanged={};
for(const [f,sha] of Object.entries(read(base+'/evidence/offline/summary.json').source_sha256)){
 if(['internal/boardfamily/config.go','internal/boardfamily/native.go','internal/boardfamily/validate.go'].includes(f)||(f.startsWith('internal/boardfamily/reference/')&&fs.existsSync(f)&&!f.includes('/.kicadai/')&&!f.endsWith('.lck')&&!f.endsWith('.kicad_prl'))){assert.equal(hash(f),sha,'reused source changed: '+f);unchanged[f]=sha}
}
const finalRun=read(cache+'/acceptance-holdout-03/summary.json');
assert.equal(finalRun.ledger_sha256,hash(cache+'/live-ledger.json'));assert.equal(finalRun.binary_sha256,hash(cache+'/kicadai-board-family'));
const regression=read(cache+'/regression-10/execution.json');assert.equal(regression.exit_code,0);assert.equal(regression.provider_keys_removed,true);
const assessment={
 as_of_utc:new Date().toISOString(),strict_live_acceptance_passed:false,original_acceptance_passed:false,goal_complete:false,
 contract_sha256:hash(contract),holdout_sha256:hash(holdout),runs,development_checks_passed:2,
 supported_first_selection_matches:10,supported_first_completed_boards:10,supported_denominator:10,
 unsupported_first_passes:2,unsupported_denominator:4,clarification_first_passes:1,clarification_denominator:2,strict_live_passes:13,strict_live_denominator:16,
 corrected_offline_decision_replays_passed:16,offline_replay_api_requests:0,corrected_source_commit:finalRun.source_commit,corrected_binary_sha256:finalRun.binary_sha256,
 corrected_source_sha256:Object.fromEntries(['internal/boardfamily/interpret.go','internal/boardfamily/interpret_test.go','specs/board-family-v1/development/replay-decisions/main.go'].map(p=>[p,hash(p)])),reused_unchanged_qualification_source_sha256:unchanged,
 timing_count:10,median_end_to_end_seconds:median(supported.map(x=>x.wall_seconds)),max_end_to_end_seconds:Math.max(...supported.map(x=>x.wall_seconds)),median_generation_seconds:median(supported.map(x=>x.generation_seconds)),median_validation_seconds:median(supported.map(x=>x.validation_seconds)),
 goal_total_requests:35,authorized_request_ceiling:35,authorized_micro_usd_ceiling:10_000_000,known_input_tokens:input,known_output_tokens:output,known_estimated_micro_usd:known,unknown_reserved_micro_usd:unknown,conservative_total_micro_usd:known+unknown,actual_invoice_checked:false,native_projects:native,cases:records,
 limitations:['Implementing-agent authorship/self-review, not an independent benchmark or engineering review.','Live evaluation spans three source/binary versions; all ten supported cases used the initial holdout version.','Offline replay proves corrected handling of retained responses, not fresh model accuracy or a changed live score.','One wrongly accepted resize generated an unchanged board; retained as rejected evidence, not a successful deliverable.','Local hashes establish integrity/provenance, not independent authentication, firmware execution or hardware performance.','Native default-ignored categories and readability/assembly limitations remain disclosed in FAMILY.md.']
};
assert(assessment.median_end_to_end_seconds<300&&assessment.max_end_to_end_seconds<600);assert(known+unknown<=10_000_000);
queue(cache+'/live-ledger.json',dest+'/ledger.json');
for(const [profile,id] of [['standard','fresh-01'],['fast','fresh-02'],['low_current','fresh-03']])for(const f of ['selection.json','validation.json','erc.json','drc.json'])queue(cache+'/acceptance-holdout-01/'+id+'/'+f,'examples/board-family-v1/'+profile+'/holdout-'+f);
queue(cache+'/regression-10/execution.json',dest+'/bounded-regression.json');queue(cache+'/regression-10/tests.log',dest+'/bounded-regression.log');
// Validate the complete packet before copying anything; never overwrite evidence.
for(const [src,dst,sha] of copies){fs.mkdirSync(path.dirname(dst),{recursive:true});fs.copyFileSync(src,dst,fs.constants.COPYFILE_EXCL);assert.equal(hash(dst),sha);files[dst]=sha}
fs.writeFileSync(dest+'/assessment.json',JSON.stringify(assessment,null,2)+'\n',{flag:'wx'});files[dest+'/assessment.json']=hash(dest+'/assessment.json');
fs.writeFileSync(dest+'/manifest.json',JSON.stringify({created_utc:new Date().toISOString(),notice:'Byte-identical run copies; historical publications unchanged. Native hashes match reviewed examples; request acceptance is recorded separately.',files},null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({copied_hashes:Object.keys(files).length,strict_live_passes:13,strict_live_total:16,offline_replay_passes:16,requests:35,conservative_micro_usd:known+unknown,median_seconds:assessment.median_end_to_end_seconds,max_seconds:assessment.max_end_to_end_seconds,median_generation_seconds:assessment.median_generation_seconds,median_validation_seconds:assessment.median_validation_seconds}));
