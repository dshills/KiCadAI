// One compact publication of the approved six-call follow-up. No API use.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const base='specs/board-family-v1',src='.cache/board-family-v1/acceptance-guardrails-01',dest=base+'/evidence/guardrails';
const read=p=>JSON.parse(fs.readFileSync(p)),hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const s=read(src+'/summary.json'),spec=read(base+'/evaluation/guardrail-followup-proposed.json'),before=read(src+'/ledger-before.json'),after=read(src+'/ledger-after.json');
assert(!fs.existsSync(dest));assert.equal(s.passed,true);assert.equal(s.cases.length,6);assert.equal(new Set(s.cases.map(c=>c.id)).size,6);
assert.equal(s.spec_sha256,hash(base+'/evaluation/guardrail-followup-proposed.json'));assert.equal(s.contract_sha256,hash(src+'/executed-contract.json'));
assert.equal(s.contract_sha256,hash(base+'/evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json'));
assert.equal(before.entries.length,35);assert.equal(after.entries.length,41);assert(!after.halt_reason);
assert.deepEqual(before,read(base+'/evidence/acceptance/ledger.json'));assert.deepEqual(after.entries.slice(0,35),before.entries);
assert.equal(hash(src+'/ledger-after.json'),hash('.cache/board-family-v1/live-ledger.json'));assert.equal(hash(src+'/ledger-after.json'),s.ledger_sha256);
const copies=['summary.json','executed-contract.json','ledger-before.json','ledger-after.json'].map(f=>[src+'/'+f,dest+'/'+f]);
for(const [i,c] of s.cases.entries()){
 const w=spec.cases.find(x=>x.id===c.id),p=src+'/'+c.id,selection=read(p+'/selection.json'),d=selection.decision,e=after.entries[i+35];assert(w);
 assert.equal(c.passed,true);assert.equal(c.first_attempt,true);assert.equal(c.exit_code,0);assert.equal(c.no_native_design,true);assert.equal(c.original_prompt_preserved,true);
 assert.deepEqual(fs.readdirSync(p),['selection.json']);assert.equal(fs.readFileSync(p+'.txt','utf8'),w.prompt);assert.equal(hash(p+'/selection.json'),c.selection_sha256);
 assert.deepEqual(read(p+'.stdout.log'),c.result);assert.equal(c.result.passed,false);assert.equal(c.result.disposition,w.expected_disposition);
 assert.equal(d.disposition,w.expected_disposition);assert.equal(d.configuration,null);assert(d.message.trim());assert.equal(d.clauses.map(x=>x.text).join(''),w.prompt);
 assert.equal(selection.ledger_index,i+36);assert.equal(e.index,selection.ledger_index);assert.equal(e.status,'completed');assert.equal(e.response_id,selection.response_id);assert.equal(e.model,selection.model);
 assert.equal(e.input_tokens,selection.usage.input_tokens);assert.equal(e.output_tokens,selection.usage.output_tokens);
 for(const f of ['.txt','.stdout.log','.stderr.log','/selection.json'])copies.push([p+f,dest+'/'+c.id+f]);
}
assert.deepEqual(s.metrics,{unsupported_passes:4,unsupported_total:4,clarification_passes:2,clarification_total:2});
let known=0,unknown=0,input=0,output=0;
for(const [i,e] of after.entries.entries()){
 assert.equal(e.index,i+1);
 if(e.status==='completed'){assert(Number.isInteger(e.input_tokens)&&e.input_tokens>0&&Number.isInteger(e.output_tokens)&&e.output_tokens>0);const cost=Math.ceil((e.input_tokens*2+e.output_tokens*8)/5);assert.equal(e.estimated_micro_usd,cost);known+=cost;input+=e.input_tokens;output+=e.output_tokens}
 else{assert.equal(e.status,'failed_or_unknown');unknown+=e.reserve_micro_usd}
}
assert(known+unknown<=10_000_000);
const regression=read('.cache/board-family-v1/regression-12/execution.json');assert.equal(regression.exit_code,0);assert.equal(regression.provider_keys_removed,true);
for(const [f,name] of [['execution.json','bounded-regression.json'],['tests.log','bounded-regression.log']])copies.push(['.cache/board-family-v1/regression-12/'+f,dest+'/'+name]);
const assessment={as_of_utc:new Date().toISOString(),targeted_guardrail_followup_passed:true,unsupported_passes:4,unsupported_total:4,clarification_passes:2,clarification_total:2,source_commit:s.source_commit,production_base_commit:s.production_base_commit,cap_only_lineage:s.cap_only_lineage,binary_sha256:s.binary_sha256,spec_sha256:s.spec_sha256,contract_sha256:s.contract_sha256,goal_total_requests:41,known_responses:40,known_input_tokens:input,known_output_tokens:output,known_estimated_micro_usd:known,unknown_reserved_micro_usd:unknown,conservative_total_micro_usd:known+unknown,actual_invoice_checked:false,original_live_acceptance_passed:false,earlier_holdout_strict_passed:false,earlier_holdout_strict_score:'13/16',evidence_combination:'Ten correct first-attempt supported language boards from the earlier holdout, unchanged current-decoder replay of those requests, ten numerically configured boards plus three native-identical post-integration replays, and this separately approved six-case live refusal/clarification follow-up. Not a newly executed single-binary 16/16 trial.',semantic_review:{reviewer:'implementing Codex agent',checked:'Four refusals explain Bluetooth, 12 V direct supply, relay output and 90x50 mm outline restrictions. Both clarifications ask for missing priorities/loads/measurement interval without assigning a configuration. All six preserve original requests and produce no native files.'},bounded_regression_seconds:regression.seconds,limitations:['Original failed trials and their denominators remain unchanged.','Small implementing-agent-authored evaluation sets, not an independent benchmark or universal semantic guarantee.','Local hashes establish integrity/provenance, not third-party signatures or measured hardware performance.','Inherited repository CI lint failure and skipped dependent quality gate remain separately disclosed.']};
const files={};for(const [from,to] of copies){fs.mkdirSync(path.dirname(to),{recursive:true});fs.copyFileSync(from,to,fs.constants.COPYFILE_EXCL);assert.equal(hash(from),hash(to));files[to]=hash(to)}
fs.writeFileSync(dest+'/assessment.json',JSON.stringify(assessment,null,2)+'\n',{flag:'wx'});files[dest+'/assessment.json']=hash(dest+'/assessment.json');
fs.writeFileSync(dest+'/manifest.json',JSON.stringify({notice:'Byte-identical six-case evidence. Earlier publications and the first 35 ledger entries are unchanged.',files},null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({verified_new_hashes:Object.keys(files).length,unsupported:4,clarify:2,requests:41,conservative_micro_usd:known+unknown}));
