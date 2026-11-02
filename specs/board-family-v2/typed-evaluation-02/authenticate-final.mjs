// Portable, read-only authentication of the completed failed batch; never calls a provider.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
import {hash,hashBytes,read,inventory,authenticateFiles,authenticateExamples,compareBundle,exampleID,expectedConfig} from '../evaluation/acceptance-lib.mjs';
import {E,evaluationID,model,checkApproval,checkLedger,assessSelection,metrics,checkSemanticReview} from './acceptance.mjs';
import {authenticatePlan} from './runtime.mjs';

export function verifyTerminalEvidence(directory=`${E}/evidence-02`) {
  const {spec,contract}=authenticatePlan(), {receipt}=authenticateExamples();
  const state=read(`${directory}/state.json`), ledger=read(`${directory}/ledger.json`), entries=checkLedger(ledger);
  assert.equal(state.evaluation_id,evaluationID);
  assert.equal(state.status,'collection-complete-semantic-review-pending');
  assert.equal(state.records.length,14); assert.equal(entries.length,14);
  assert.ok(entries.every(e=>e.status==='completed'));
  assert.deepEqual(state.planned_case_ids,spec.cases.map(c=>c.id));
  assert.deepEqual(state.records.map(r=>r.id),state.planned_case_ids);
  assert.deepEqual(state.ledger_entries,entries); assert.equal(state.ledger_sha256,hash(`${directory}/ledger.json`));
  const bindings={runtime_sha256:hash(`${E}/runtime-02.json`),freeze_sha256:hash(`${E}/freeze-02.json`),approval_sha256:hash(`${E}/APPROVAL-02.json`)};
  assert.deepEqual(state.bindings,bindings);
  checkApproval(read(`${E}/APPROVAL-02.json`),bindings.runtime_sha256,bindings.freeze_sha256);
  const selections={}, records=[], files=['state.json','ledger.json'];
  let prefix=[],nativeBundles=0,comparedFiles=0;
  for (const [i,r] of state.records.entries()) {
    const c=spec.cases[i], dir=`${directory}/${c.id}`, selection=read(`${dir}/selection.json`), e=entries[i];
    assert.equal(r.state,'finished'); assert.equal(r.child_terminal_observed,true);
    assert.equal(r.signal,null);assert.equal(r.timed_out,false);assert.equal(r.spawn_error,null);
    assert.ok(Number.isFinite(r.wall_seconds) && r.wall_seconds>=0);
    assert.equal(hash(`${directory}/${c.id}.txt`),hashBytes(Buffer.from(c.prompt)));
    assert.equal(r.prompt_sha256,hash(`${directory}/${c.id}.txt`));
    authenticateFiles(directory,r.files_sha256);
    const currentFiles=[`${c.id}.txt`,`${c.id}.stdout.log`,`${c.id}.stderr.log`,`${c.id}.ledger.json`,...inventory(dir).map(f=>`${c.id}/${f}`)].sort();
    assert.deepEqual(Object.keys(r.files_sha256).sort(),currentFiles);files.push(...currentFiles);
    const next=checkLedger(read(`${directory}/${c.id}.ledger.json`),prefix);
    assert.equal(next.length,prefix.length+1); prefix=next;
    assert.equal(selection.model,model);assert.equal(selection.ledger_index,e.index);assert.equal(selection.response_id,e.response_id);
    assert.equal(selection.usage.input_tokens,e.input_tokens);assert.equal(selection.usage.output_tokens,e.output_tokens);
    assert.equal(selection.usage.total_tokens,e.input_tokens+e.output_tokens);
    assert.equal(selection.original_request,c.prompt);
    selections[c.id]={selection,sha256:hash(`${dir}/selection.json`)};
    const recomputed={...r,...assessSelection(spec,c,selection,contract.schema)};
    if(r.execution_or_evidence_failure) {
      assert.equal(r.exit_code,1);assert.equal(r.output_checks_pass,false);
      assert.equal(selection.decision.disposition,'clarify');assert.equal(selection.decision.configuration,null);
      assert.deepEqual(inventory(dir),['selection.json']);recomputed.output_checks_pass=false;
    } else {
      assert.equal(r.exit_code,0);assert.deepEqual(recomputed.raw,r.raw);assert.deepEqual(recomputed.admitted,r.admitted);
      if(selection.decision.disposition==='supported') {
        assert.equal(c.expected_disposition,'supported');assert.deepEqual(selection.decision.configuration,expectedConfig(spec,c));
        assert.equal(r.output_checks_pass,true);
        assert.deepEqual(compareBundle(dir,receipt.cases.find(x=>x.id===exampleID(c))),r.bundle);
        nativeBundles++;comparedFiles+=Object.keys(r.bundle.files_compared).length;
      } else {
        assert.deepEqual(inventory(dir),['selection.json']);assert.equal(selection.decision.configuration,null);
        assert.equal(r.output_checks_pass,c.expected_disposition!=='supported');
      }
    }
    records.push(recomputed);
  }
  assert.deepEqual(prefix,entries);assert.deepEqual(inventory(directory),files.sort());
  assert.deepEqual(state.metrics,metrics(spec,state.records));
  const semantic=checkSemanticReview(spec,{...state,records},read(`${E}/SEMANTIC-REVIEW-02.json`),{
    state_sha256:hash(`${directory}/state.json`),runtime_sha256:bindings.runtime_sha256,freeze_sha256:bindings.freeze_sha256,
    prompt_hashes:Object.fromEntries(spec.cases.map(c=>[c.id,hashBytes(Buffer.from(c.prompt))]))},selections);
  const computed=metrics(spec,records), audit=read(`${E}/AUDIT-RESULT-02.json`);
  for(const [key,value] of Object.entries(computed))assert.deepEqual(audit[key],value);
  assert.deepEqual(audit.semantic_cases,semantic);assert.equal(audit.semantic_review,'checked');
  const reserve=entries.length*50000, cost=entries.reduce((n,e)=>n+e.estimated_micro_usd,0);
  assert.equal(audit.reserved_micro_usd,reserve);assert.equal(audit.estimated_micro_usd,cost);
  assert.equal(audit.input_tokens,entries.reduce((n,e)=>n+e.input_tokens,0));assert.equal(audit.output_tokens,entries.reduce((n,e)=>n+e.output_tokens,0));
  assert.equal(audit.usage_accounting_complete,true);assert.equal(audit.unknown_or_failed_outcome_entries,0);
  const passed=computed.timing_target_met && semantic.every(c=>c.complete_pass);
  assert.equal(audit.status,passed?'complete-targeted-acceptance-pass':'complete-acceptance-not-met');
  return {status:'published-evidence-authenticated',acceptance:audit.status,attempts:14,raw_passes:semantic.filter(c=>c.raw_pass).length,application_passes:semantic.filter(c=>c.application_pass).length,complete_passes:semantic.filter(c=>c.complete_pass).length,native_bundles:nativeBundles,compared_deliverables:comparedFiles,estimated_micro_usd:cost,reserved_micro_usd:reserve,evidence_files:files.length};
}

export function verifyPublication() {
  const manifest=read(`${E}/FINAL-PUBLICATION-02.json`);
  assert.equal(manifest.evaluation_id,evaluationID);assert.equal(manifest.status,'complete-failed-evaluation-published');
  authenticateFiles(E,manifest.files_sha256);
  for(const [file,sha] of Object.entries(manifest.prerequisites_sha256))assert.equal(hash(file),sha);
  const result=verifyTerminalEvidence();
  assert.deepEqual(result,manifest.result);
  const final=read(`${E}/evidence-02/state.json`), ledger=read(`${E}/evidence-02/ledger.json`);
  for(let n=1;n<=6;n++) {
    const tag=String(n).padStart(2,'0'), state=read(`${E}/checkpoints/stop-${tag}-state.json`), stoppedLedger=read(`${E}/checkpoints/stop-${tag}-ledger.json`);
    assert.equal(state.status,'stopped');assert.deepEqual(state.bindings,final.bindings);
    assert.deepEqual(state.records,final.records.slice(0,state.records.length));
    assert.deepEqual(checkLedger(stoppedLedger),ledger.entries.slice(0,state.records.length));
    assert.equal(hash(`${E}/checkpoints/stop-${tag}-ledger.json`),state.ledger_sha256);
  }
  return {...result,published_files:Object.keys(manifest.files_sha256).length};
}

if(process.argv[1] && import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href) {
  for(const key of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS'])assert.ok(!process.env[key],'run without provider credentials');
  assert.deepEqual(process.argv.slice(2),['--check']);
  console.log(JSON.stringify(verifyPublication(),null,2));
}
