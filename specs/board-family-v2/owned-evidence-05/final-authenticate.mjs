// Read-only authentication of final owned-v4 evidence. No language score is
// inferred from successful collection, contract replay or deterministic files.
// Hashes establish local byte consistency, not independent provider attestation.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {hash,hashBytes,read,inventory,authenticateFiles,authenticateExamples} from '../evaluation/acceptance-lib.mjs';
import {cleanTerminal,assessAttempt} from './collector.mjs';
import {version as collectorVersion} from './final-collector.mjs';
import {boundedJSON,checkManifest,checkAuthority,verifyReuse} from './final-runtime.mjs';
import {offlineEnvironment} from './contracts.mjs';

export function authenticateBatch(manifestFile,root) {
  const raw=boundedJSON(manifestFile),m=checkManifest(manifestFile,raw.kind==='live-candidate'?'live':'offline');
  assert.equal(path.resolve(root),m.batch_directory,'cannot authenticate a substitute batch');
  const spec={evaluation_id:m.evaluation_id,cases:m.cases},start=read(path.join(root,'start.json')),state=read(path.join(root,'result.json'));
  assert.equal(start.manifest_sha256,hash(manifestFile));assert.equal(start.evaluation_id,m.evaluation_id);
  assert.equal(start.version,collectorVersion);assert.equal(start.status,'started-not-complete');
  assert.deepEqual(start.planned_case_ids,m.cases.map(c=>c.id));assert.deepEqual(read(path.join(root,'budget.json')),m.policy);
  assert.equal(state.version,collectorVersion);assert.equal(state.evaluation_id,m.evaluation_id);assert.equal(state.planned_cases,m.cases.length);
  const mode=m.kind==='live-candidate'?'live':'offline';
  assert.equal(state.mode,mode);assert.equal(start.mode,mode);
  if(mode==='live') {
    const binding=checkAuthority(m,manifestFile,path.join(root,'approval.json'),path.join(root,'release-gates.json'),start.started_utc);
    assert.equal(start.approval_sha256,binding.approval_sha256);assert.equal(start.release_gates_sha256,binding.release_gates_sha256);
  } else {assert.equal(start.approval_sha256,null);assert.equal(start.release_gates_sha256,null);}
  assert.equal(state.acceptance,'not-established-by-collection');
  verifyReuse(m);
  assert.ok(state.records.length<=spec.cases.length);
  assert.deepEqual(state.records.map(r=>r.id),spec.cases.slice(0,state.records.length).map(c=>c.id));
  const examples=m.qualification==='reviewed-two-family-examples'?authenticateExamples().receipt.cases:[];
  const selections={},selectionHashes={};let prefix=[],stopped=false;
  for(const record of state.records) {
    assert.equal(stopped,false,'case followed an unsafe attempt');
    const caseRoot=path.join(root,record.id),outcome=read(path.join(caseRoot,'outcome.json'));
    assert.deepEqual(outcome,record);authenticateFiles(caseRoot,record.files_sha256);
    assert.deepEqual(inventory(caseRoot).filter(f=>f!=='outcome.json'),Object.keys(record.files_sha256).sort());
    const prompt=spec.cases.find(c=>c.id===record.id).prompt;
    assert.equal(hash(path.join(caseRoot,'prompt.txt')),hashBytes(prompt));assert.equal(record.prompt_sha256,hashBytes(prompt));
    assert.deepEqual(read(path.join(caseRoot,'attempt.json')),{id:record.id,prompt_sha256:record.prompt_sha256,state:'attempt-recorded-before-launch',started_utc:record.started_utc});
    if(record.execution)assert.deepEqual(read(path.join(caseRoot,'command.process.json')),record.execution);
    if(record.audit_execution)assert.deepEqual(read(path.join(caseRoot,'audit.process.json')),record.audit_execution);
    const file=path.join(caseRoot,'journal/selection/selection.json');
    if(fs.existsSync(file)) {selections[record.id]=read(file);selectionHashes[record.id]=hash(file);}
    if(record.state==='recorded-outcome') {
      cleanTerminal(record.audit_execution);assert.equal(record.audit_execution.exit_code,0);
      const probe=spawnSync(m.binary,[...m.prefix_args,'--inspect-owned-journal',path.join(caseRoot,'journal')],{env:offlineEnvironment(),encoding:'utf8',timeout:10000,maxBuffer:1024*1024});
      assert.equal(probe.error,undefined);assert.equal(probe.signal,null);assert.equal(probe.status,0,'journal no longer verifies');
      const audit=JSON.parse(probe.stdout);assert.deepEqual(audit,read(path.join(caseRoot,'audit.stdout.log')));
      const ledger=read(path.join(caseRoot,'journal/selection/ledger.json'));
      const expected=assessAttempt({execution:record.execution,audit,ledger,policy:m.policy,prefix,prompt,caseRoot,examples});
      for(const key of ['collection_class','advance','model_correct','response_id','ledger_index','bundle']) assert.deepEqual(record[key],expected[key],`recorded ${key} differs from replay`);
      assert.equal(record.failure,undefined);prefix=ledger.entries;
    } else {
      assert.ok(['attempt-recorded-before-launch','terminal-observed'].includes(record.state));
      assert.equal(record.advance,false);assert.equal(record.collection_class,'unsafe-to-continue');assert.ok(typeof record.failure==='string'&&record.failure.trim());
      stopped=true;
    }
  }
  const ledgerFile=path.join(root,'ledger.json');
  if(fs.existsSync(ledgerFile)) {
    const ledger=read(ledgerFile);assert.equal(ledger.version,2);assert.equal(ledger.goal,m.policy.goal);
    assert.equal(ledger.max_requests,m.policy.max_requests);assert.equal(ledger.max_micro_usd,m.policy.max_micro_usd);
    assert.deepEqual(ledger.entries.slice(0,prefix.length),prefix,'final ledger changed earlier attempts');
    if(!stopped) {assert.deepEqual(ledger.entries,prefix);assert.ok(!ledger.halt_reason);}
    else assert.ok(ledger.entries.length>=prefix.length&&ledger.entries.length<=prefix.length+1);
  } else assert.equal(prefix.length,0,'completed accounting is missing');
  assert.equal(state.prepared_cases,state.records.length);
  assert.equal(state.launched_cases,state.records.filter(r=>r.execution?.child_pid!=null).length);
  assert.equal(state.recorded_outcomes,state.records.filter(r=>r.state==='recorded-outcome').length);
  assert.equal(state.recorded_model_failures,state.records.filter(r=>r.collection_class==='recorded-model-failure').length);
  assert.deepEqual(state.unattempted_case_ids,spec.cases.slice(state.records.length).map(c=>c.id));
  assert.equal(state.status,!stopped&&state.records.length===m.cases.length?'collection-complete-semantic-review-pending':'stopped-no-retry');
  assert.ok(Number.isFinite(state.total_wall_seconds)&&state.total_wall_seconds>=0);
  const expectedInventory=['start.json','budget.json','result.json',...(mode==='live'?['approval.json','release-gates.json']:[]),...(fs.existsSync(ledgerFile)?['ledger.json']:[]),...state.records.flatMap(r=>['outcome.json',...Object.keys(r.files_sha256)].map(f=>`${r.id}/${f}`))].sort();
  assert.deepEqual(inventory(root),expectedInventory,'unrecorded or missing batch evidence');
  const bindings={manifest_sha256:hash(manifestFile),result_sha256:hash(path.join(root,'result.json')),approval_sha256:start.approval_sha256,release_gates_sha256:start.release_gates_sha256,selection_sha256:selectionHashes};
  return {spec,state,selections,bindings,acceptance:mode==='live'?'authenticated-live-evidence-semantic-review-pending':'offline-only-cannot-establish-live-acceptance'};
}

if(process.argv[1] && path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  try {
    const [manifest,root,...extra]=process.argv.slice(2);assert.ok(manifest && root && extra.length===0,'usage: final-authenticate.mjs MANIFEST FIXED_BATCH');
    const {state,acceptance}=authenticateBatch(manifest,root);
    console.log(JSON.stringify({status:'evidence-authenticated',recorded_outcomes:state.recorded_outcomes,acceptance}));
  } catch(error) {console.error(error.message);process.exitCode=1;}
}
