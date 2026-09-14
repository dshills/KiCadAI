// Read-only reauthentication and scoring. No approval creation or live execution.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {hash,hashBytes,read,inventory,authenticateFiles,expectedConfig,exampleID,compareBundle,authenticateNativeBundle} from '../evaluation/acceptance-lib.mjs';
import {processIsAlive} from '../evaluation/run-language.mjs';
import {batch,ledgerFile,runtimeFile,freezeFile,approvalFile,evaluationID,model,checkApproval,checkLedger,assessSelection,metrics,checkSemanticReview} from './acceptance.mjs';
import {authenticateRuntime} from './runtime.mjs';

const args=process.argv.slice(2);
assert.ok(args[0]==='--check' && args.length<=2,'usage: audit.mjs --check [SOURCE_BOUND_REVIEW_JSON]');
const {spec,contract,receipt}=authenticateRuntime(), stateFile=`${batch}/state.json`, state=read(stateFile);
assert.equal(state.evaluation_id,evaluationID);
assert.equal(state.bindings.runtime_sha256,hash(runtimeFile));assert.equal(state.bindings.freeze_sha256,hash(freezeFile));
assert.equal(state.bindings.approval_sha256,hash(approvalFile));
checkApproval(read(approvalFile),hash(runtimeFile),hash(freezeFile));
assert.ok(['stopped','collection-complete-semantic-review-pending'].includes(state.status),'runner is not terminal');
assert.equal(processIsAlive(state.owner_pid),false,'runner still exists; do not treat observation as completion');
assert.deepEqual(state.planned_case_ids,spec.cases.map(c=>c.id));
assert.ok(state.records.length<=spec.cases.length);
assert.deepEqual(state.records.map(r=>r.id),spec.cases.slice(0,state.records.length).map(c=>c.id));
const ledger=fs.existsSync(ledgerFile)?read(ledgerFile):null,entries=ledger?checkLedger(ledger):[];
assert.deepEqual(entries,state.ledger_entries);assert.equal(state.ledger_sha256,ledger?hash(ledgerFile):null);
const selections={}, records=[];
let ledgerPrefix=[];
for(const [i,record] of state.records.entries()) {
  const c=spec.cases[i],dir=`${batch}/${c.id}`,selectionFile=`${dir}/selection.json`;
  assert.equal(record.state,'finished');assert.equal(record.child_terminal_observed,true);
  authenticateFiles(batch,record.files_sha256);assert.equal(hash(`${batch}/${c.id}.txt`),hashBytes(Buffer.from(c.prompt)));
  assert.equal(record.prompt_sha256,hashBytes(Buffer.from(c.prompt)));
  const files=[`${c.id}.txt`,`${c.id}.stdout.log`,`${c.id}.stderr.log`];
  if(fs.existsSync(`${batch}/${c.id}.ledger.json`)) {
    const current=checkLedger(read(`${batch}/${c.id}.ledger.json`),ledgerPrefix);
    assert.ok(current.length-ledgerPrefix.length<=1);ledgerPrefix=current;files.push(`${c.id}.ledger.json`);
  }
  if(fs.existsSync(dir))files.push(...inventory(dir).map(f=>`${c.id}/${f}`));
  assert.deepEqual(Object.keys(record.files_sha256).sort(),files.sort());
  const r={...record};
  if(fs.existsSync(selectionFile)) {
    const selection=read(selectionFile);selections[c.id]={selection,sha256:hash(selectionFile)};
    Object.assign(r,assessSelection(spec,c,selection,contract.schema));
    if(!record.execution_or_evidence_failure) {
      assert.equal(record.exit_code,0);assert.equal(record.signal,null);assert.equal(record.timed_out,false);assert.equal(record.spawn_error,null);
      assert.equal(selection.model,model);
      const e=entries.find(e=>e.index===selection.ledger_index);
      assert.ok(e);assert.equal(e.status,'completed');assert.equal(e.response_id,selection.response_id);
      assert.equal(selection.usage.input_tokens,e.input_tokens);assert.equal(selection.usage.output_tokens,e.output_tokens);
      assert.deepEqual(record.raw,r.raw);assert.deepEqual(record.admitted,r.admitted);
      if(selection.decision.disposition==='supported') {
        authenticateNativeBundle(dir);
        r.output_checks_pass=c.expected_disposition==='supported' && r.admitted.configuration_matches;
        if(r.output_checks_pass) {
          assert.deepEqual(read(`${dir}/configuration.json`),expectedConfig(spec,c));
          assert.deepEqual(compareBundle(dir,receipt.cases.find(e=>e.id===exampleID(c))),record.bundle);
        }
      } else {
        assert.deepEqual(inventory(dir),['selection.json']);assert.equal(selection.decision.configuration,null);
        r.output_checks_pass=c.expected_disposition!=='supported';
      }
      assert.equal(record.output_checks_pass,r.output_checks_pass);
    } else r.output_checks_pass=false;
  } else {r.raw=undefined;r.admitted=undefined;r.output_checks_pass=false;}
  records.push(r);
}
assert.deepEqual(ledgerPrefix,entries,'missing per-case ledger snapshots');
const computed=metrics(spec,records);
assert.deepEqual(state.metrics,metrics(spec,state.records),'recorded summary changed');
let semantic=null;
if(args[1]) semantic=checkSemanticReview(spec,{...state,records},read(args[1]),{
  state_sha256:hash(stateFile),runtime_sha256:hash(runtimeFile),freeze_sha256:hash(freezeFile),
  prompt_hashes:Object.fromEntries(spec.cases.map(c=>[c.id,hashBytes(Buffer.from(c.prompt))]))},selections);
const complete=records.length===spec.cases.length;
const passed=Boolean(complete && computed.timing_target_met && semantic?.every(c=>c.complete_pass));
console.log(JSON.stringify({status:passed?'complete-targeted-acceptance-pass':complete?'complete-acceptance-not-met':'incomplete-acceptance-not-met',
  evaluation_id:evaluationID,...computed,semantic_review:semantic?'checked':'pending',semantic_cases:semantic,
  reserved_micro_usd:entries.length*50000,estimated_micro_usd:entries.reduce((n,e)=>n+(e.estimated_micro_usd??0),0),
  usage_accounting_complete:entries.every(e=>e.status==='completed'),unknown_or_failed_outcome_entries:entries.filter(e=>e.status!=='completed').length,
  input_tokens:entries.reduce((n,e)=>n+(e.input_tokens??0),0),output_tokens:entries.reduce((n,e)=>n+(e.output_tokens??0),0),
  original_batch_scores:'unchanged',evidence_notice:'Local source/byte integrity, not provider signatures, independent review or physical certification.'},null,2));
