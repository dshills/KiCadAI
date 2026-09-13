// Publish one compact, immutable copy of the original evaluation, including failures.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const root=process.cwd(),dest=path.join(root,'specs/board-family-v1/evidence/live');
const hash=f=>crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const read=f=>JSON.parse(fs.readFileSync(f));
assert(!fs.existsSync(dest),'publication must be new; preserve previous evidence');
const specPath=path.join(root,'specs/board-family-v1/evaluation/language.json'),spec=read(specPath);
const files={},first=new Map(),latest=new Map(),native=[],attempts=[];
function copy(src,relative){
 const dst=path.join(root,relative);fs.mkdirSync(path.dirname(dst),{recursive:true});
 fs.copyFileSync(src,dst,fs.constants.COPYFILE_EXCL);assert.equal(hash(src),hash(dst));files[relative]=hash(dst);
}
function write(relative,data){
 const dst=path.join(root,relative);fs.mkdirSync(path.dirname(dst),{recursive:true});
 fs.writeFileSync(dst,JSON.stringify(data,null,2)+'\n',{flag:'wx'});files[relative]=hash(dst);
}
const prefix='specs/board-family-v1/evidence/live';
for(const suffix of ['01','02','03']){
 const run='acceptance-language-'+suffix,src=path.join(root,'.cache/board-family-v1',run),summary=read(path.join(src,'summary.json'));
 assert.equal(summary.spec_sha256,hash(specPath));
 copy(path.join(src,'summary.json'),`${prefix}/${run}/summary.json`);
 for(const c of summary.cases){
  const expected=spec.cases.find(s=>s.id===c.id);assert(expected);
  assert.equal(fs.readFileSync(path.join(src,c.id+'.txt'),'utf8'),expected.prompt);
  const row={...c,run};attempts.push(row);if(!first.has(c.id))first.set(c.id,row);latest.set(c.id,row);
  for(const ext of ['txt','stdout.log','stderr.log'])copy(path.join(src,c.id+'.'+ext),`${prefix}/${run}/${c.id}.${ext}`);
  const dir=path.join(src,c.id);
  for(const f of ['selection.json','configuration.json','electrical.json','validation.json','erc.json','erc.log','drc.json','drc.log']){
   if(fs.existsSync(path.join(dir,f)))copy(path.join(dir,f),`${prefix}/${run}/${c.id}/${f}`);
  }
  if(c.selection_sha256)assert.equal(hash(path.join(dir,'selection.json')),c.selection_sha256);
  if(fs.existsSync(path.join(dir,'board.kicad_pcb'))){
   const v=read(path.join(dir,'validation.json')),cfg=read(path.join(dir,'configuration.json'));
   assert.equal(v.passed,true);assert.equal(v.checks.length,13);assert(v.checks.every(x=>x.passed));
   const erc=read(path.join(dir,'erc.json')),drc=read(path.join(dir,'drc.json'));
   assert(erc.sheets.length>0&&erc.sheets.every(s=>Array.isArray(s.violations)&&s.violations.length===0));
   for(const k of ['violations','unconnected_items','schematic_parity'])assert.deepEqual(drc[k],[]);
   for(const [f,sha] of Object.entries(v.native_sha256)){
    assert.equal(hash(path.join(dir,f)),sha);
    assert.equal(hash(path.join(root,'examples/board-family-v1',cfg.profile,f)),sha);
   }
   native.push({id:c.id,run,accepted_request:c.passed===true&&expected.expected_disposition==='supported',profile:cfg.profile,native_sha256:v.native_sha256,matches_reviewed_example:true,erc_ignored_checks:erc.ignored_checks,drc_ignored_checks:drc.ignored_checks});
  }
 }
}
assert.equal(first.size,16);assert.equal(attempts.length,17);
const ledgerPath=path.join(root,'.cache/board-family-v1/live-ledger.json'),ledger=read(ledgerPath);
assert.equal(ledger.entries.length,attempts.length);assert(!ledger.halt_reason);
const indices=attempts.map(c=>read(path.join(root,'.cache/board-family-v1',c.run,c.id,'selection.json')).ledger_index);
assert.deepEqual([...indices].sort((a,b)=>a-b),Array.from({length:17},(_,i)=>i+1));
let known=0,unknown=0;
for(const e of ledger.entries){
 if(e.input_tokens&&e.output_tokens){const micro=Math.ceil((e.input_tokens*2+e.output_tokens*8)/5);assert.equal(e.estimated_micro_usd,micro);known+=micro}
 else {assert.equal(e.status,'failed_or_unknown');unknown+=e.reserve_micro_usd}
}
copy(ledgerPath,`${prefix}/ledger.json`);
for(const [profile,id] of [['standard','nl-07'],['fast','nl-08'],['low_current','nl-06']]){
 const c=latest.get(id);assert(c.passed);
 for(const f of ['selection.json','validation.json','erc.json','drc.json'])copy(path.join(root,'.cache/board-family-v1',c.run,id,f),`examples/board-family-v1/${profile}/language-${f}`);
}
const supported=[...latest.values()].filter(c=>c.expected_disposition==='supported'&&c.passed);
const median=values=>{const x=[...values].sort((a,b)=>a-b);return (x[Math.floor((x.length-1)/2)]+x[Math.floor(x.length/2)])/2};
const count=(kind,predicate)=>[...first.values()].filter(c=>c.expected_disposition===kind&&predicate(c)).length;
const assessment={as_of_utc:new Date().toISOString(),overall_passed:false,request_count:17,known_estimated_micro_usd:known,unknown_reserved_micro_usd:unknown,conservative_total_micro_usd:known+unknown,actual_provider_invoice_checked:false,supported_first_selection_matches:count('supported',c=>c.disposition==='supported'&&c.configuration_matches),supported_first_completed_boards:count('supported',c=>c.passed),supported_denominator:10,supported_latest_completed_boards:supported.length,unsupported_first_passes:count('unsupported',c=>c.passed),unsupported_denominator:4,clarification_first_passes:count('clarify',c=>c.passed),clarification_denominator:2,timing_population:'Eight completed supported cases, including the explicit nl-01 recovery; failures and the incorrectly accepted ambiguous board are excluded from these conditional completion timings, not from reliability denominators.',timing_count:supported.length,median_completed_wall_seconds:median(supported.map(c=>c.wall_seconds)),max_completed_wall_seconds:Math.max(...supported.map(c=>c.wall_seconds)),median_generation_seconds:median(supported.map(c=>c.result.generation_seconds)),median_validation_seconds:median(supported.map(c=>c.result.validation_seconds)),native_runs:native,first_attempts:[...first.values()],latest_attempts:[...latest.values()],limitations:['The initial nl-01 response could not be decoded; its selection and actual usage are unknown. Retain its full $0.05 reserve.','nl-03 incorrectly refused an explicit exclusion; nl-05 selected correctly but failed text-boundary decoding; ambiguous-02 wrongly generated a board. None is erased by a later fix.','Binary hashes identify each recorded run. Later decoder/prompt/safety-gate changes are not represented as live-tested results.','Local hashes and response IDs establish retained provenance, not independent authentication or hardware proof.']};
write(`${prefix}/assessment.json`,assessment);
fs.writeFileSync(path.join(dest,'manifest.json'),JSON.stringify({created_utc:new Date().toISOString(),notice:'Unchanged original evidence copies. No raw accepted/rejected record has been rewritten. Native geometry equals the separately reviewed examples; rejected ambiguous output is retained only in the original local run, not offered as a successful example.',files},null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({copied_files:Object.keys(files).length,requests:assessment.request_count,conservative_micro_usd:assessment.conservative_total_micro_usd,overall_passed:assessment.overall_passed}));
