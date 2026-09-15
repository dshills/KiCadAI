// Real CLI subprocesses, in-memory provider transport only. Never the live binary.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {hash,read,durableJSON} from '../evaluation/acceptance-lib.mjs';
import {freezeContracts,offlineEnvironment} from './contracts.mjs';
import {collect} from './final-collector.mjs';
import {authenticateBatch} from './final-authenticate.mjs';
import {version,finalDependencies,checkManifest,verifyReuse,evaluationID} from './final-runtime.mjs';
import {loadCorpus} from './scoring.mjs';
const binary=process.env.KICADAI_OWNED_TEST_BINARY;
const fixture=id=>({id,prompt:id==='clarify'?'I have not chosen a sensor.':id==='unsupported'?'Please use SHT31. Require its heater.':'Please use BMP280 with standard profile.'});
function setup(t,ids,changes={}) {
  const root=fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(),'owned-final-process-')));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const cases=ids.map(fixture);
  const m={version,status:'frozen-not-live-authorized',kind:'offline-test',evaluation_id:'offline-owned-final',
    workspace_root:fs.realpathSync(process.cwd()),batch_directory:path.join(root,'batch'),source_commit:null,
    policy:{goal:'offline-owned-final',max_requests:cases.length,max_micro_usd:cases.length*50000},cases,
    binary:path.resolve(binary),binary_sha256:hash(binary),node_sha256:hash(process.execPath),prefix_args:['-test.run=^TestOwnedProcessHelper$','--'],
    qualification:'non-design-test-fixtures',kicad_cli:'/not-used-offline',reuse:null,case_timeout_ms:60000,total_timeout_ms:180000,
    runtime_files_sha256:Object.fromEntries(finalDependencies.map(f=>[path.relative(process.cwd(),f),hash(f)])),...changes};
  m.contracts=freezeContracts(m,path.join(root,'contracts'));
  const manifestPath=path.join(root,'manifest.json');durableJSON(manifestPath,m,{exclusive:true});
  return {root,m,manifestPath,mode:'offline'};
}
test('final collector retains completed model failures, owns a single batch and replays all journals',{skip:!binary},async t=>{
  const o=setup(t,['invalid-extraction','malformed-output','refusal','clarify','unsupported']),r=await collect(o);
  assert.equal(r.status,'collection-complete-semantic-review-pending');assert.equal(r.recorded_outcomes,5);
  assert.equal(r.recorded_model_failures,3);assert.equal(r.launched_cases,5);
  assert.deepEqual(r.records.map(x=>x.execution.exit_code),[1,1,1,0,0]);
  assert.equal(authenticateBatch(o.manifestPath,o.m.batch_directory).acceptance,'offline-only-cannot-establish-live-acceptance');
  const before=hash(path.join(o.m.batch_directory,'ledger.json'));
  await assert.rejects(collect(o));assert.equal(hash(path.join(o.m.batch_directory,'ledger.json')),before);
  await assert.rejects(collect({...o,output:path.join(o.root,'another-batch')}),/fixed by the manifest/);
  assert.equal(fs.existsSync(path.join(o.root,'another-batch')),false);
});
for(const failure of ['transport-error','missing-model','crash-after-request','generation-conflict','validation-failure','timeout','log-overflow']) {
  test('final collector stops after '+failure+' without dropping or reattempting cases',{skip:!binary},async t=>{
    const o=setup(t,['clarify',failure,'unsupported'],['timeout','log-overflow'].includes(failure)?{case_timeout_ms:500}:{});
    const r=await collect(o);assert.equal(r.status,'stopped-no-retry');assert.equal(r.prepared_cases,2);assert.equal(r.recorded_outcomes,1);
    assert.deepEqual(r.unattempted_case_ids,['unsupported']);assert.equal(r.records[1].collection_class,'unsafe-to-continue');
    assert.equal(fs.existsSync(path.join(o.m.batch_directory,'unsupported')),false);
    authenticateBatch(o.manifestPath,o.m.batch_directory);
    if(failure==='timeout')assert.equal(r.records[1].execution.timed_out,true);
    if(failure==='log-overflow')assert.equal(r.records[1].execution.log_overflow,true);
    if(failure==='generation-conflict')assert.equal(fs.readFileSync(path.join(o.m.batch_directory,failure,'board/user-owned.txt'),'utf8'),'do not replace');
  });
}
test('test manifests, extra output destinations and approval files cannot enable live execution',{skip:!binary},async t=>{
  const o=setup(t,['clarify']);
  await assert.rejects(collect({...o,mode:'live'}),/test helpers can never/);
  await assert.rejects(collect({...o,approvalPath:'not-approved'}));
  await assert.rejects(collect({...o,gatesPath:'not-release-gates'}));
  assert.equal(fs.existsSync(o.m.batch_directory),false);
});
test('production-shaped manifest still requires separate authority before any child or batch',{skip:!binary},async t=>{
  const o=setup(t,['clarify']),m=o.m;
  // Test-only counterfeit candidate: it can pass shape checks but cannot pass
  // reused production provenance. No key is present and no child is launched.
  m.kind='live-candidate';m.evaluation_id=evaluationID;m.policy={goal:evaluationID,max_requests:14,max_micro_usd:1000000};
  m.cases=loadCorpus().cases.map(({id,prompt})=>({id,prompt}));
  m.contracts=freezeContracts(m,path.join(o.root,'production-shaped-contracts'));
  m.prefix_args=[];m.source_commit=spawnSync('git',['rev-parse','HEAD'],{encoding:'utf8',env:offlineEnvironment()}).stdout.trim();
  m.qualification='reviewed-two-family-examples';m.qualification_receipt_sha256=hash('specs/board-family-v2/evidence/examples-01.json');
  m.kicad_cli=m.binary;m.kicad_cli_sha256=m.binary_sha256;m.case_timeout_ms=120000;m.total_timeout_ms=900000;
  m.reuse={rehearsal_root:path.join(o.root,'nonexistent-proof'),review_root:path.join(o.root,'nonexistent-review')};
  m.reuse_files_sha256={...m.runtime_files_sha256};durableJSON(o.manifestPath,m);
  assert.doesNotThrow(()=>checkManifest(o.manifestPath,'live'));
  await assert.rejects(collect({...o,mode:'live'}),/fresh approval and release gates required/);
  assert.equal(fs.existsSync(m.batch_directory),false);
  assert.throws(()=>verifyReuse(m),'shape checks cannot substitute for qualified production evidence');
});
test('changed contract, manifest or evidence fails closed and batch cannot be relocated',{skip:!binary},async t=>{
  const o=setup(t,['clarify']);await collect(o);
  const root=o.m.batch_directory;
  for(const [name,change] of [
    ['clarify/journal/response.bin',b=>Buffer.concat([b,Buffer.from('tampered')])],
    ['clarify/command.process.json',b=>{const v=JSON.parse(b);v.child_terminal_observed=false;return Buffer.from(JSON.stringify(v));}],
    ['result.json',b=>{const v=JSON.parse(b);v.mode='live';return Buffer.from(JSON.stringify(v));}],
    ['ledger.json',b=>{const v=JSON.parse(b);v.entries[0].response_id='changed';return Buffer.from(JSON.stringify(v));}],
  ]) {
    const file=path.join(root,name),bytes=fs.readFileSync(file);
    fs.writeFileSync(file,change(bytes));assert.throws(()=>authenticateBatch(o.manifestPath,root));fs.writeFileSync(file,bytes);
  }
  const extra=path.join(root,'extra.txt');fs.writeFileSync(extra,'unrecorded');assert.throws(()=>authenticateBatch(o.manifestPath,root));fs.unlinkSync(extra);
  assert.throws(()=>authenticateBatch(o.manifestPath,path.join(o.root,'another-batch')),/substitute batch/);
  authenticateBatch(o.manifestPath,root);
  const contract=o.m.contracts[0],original=fs.readFileSync(contract.path),changed=JSON.parse(original);
  changed.capability_context+=' changed';durableJSON(contract.path,changed);
  assert.throws(()=>checkManifest(o.manifestPath,'offline'));fs.writeFileSync(contract.path,original);
});
test('final collector CLI rejects missing mode and extra arguments without execution',{skip:!binary},t=>{
  const o=setup(t,['clarify']),file='specs/board-family-v2/owned-evidence-05/final-collector.mjs';
  for(const args of [[],['--live',o.manifestPath],['--offline',o.manifestPath,'extra']]) {
    const r=spawnSync(process.execPath,[file,...args],{env:offlineEnvironment(),encoding:'utf8',timeout:10000});
    assert.equal(r.status,1);assert.equal(r.signal,null);assert.equal(fs.existsSync(o.m.batch_directory),false);
  }
});
