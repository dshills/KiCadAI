// Opt-in tests run the actual Go main, default pipeline and owned journal.
// HTTP is synthetic; native cases use installed KiCad and reviewed examples.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {collect,version,collectorDependencies,checkManifest,executeRecorded} from './collector.mjs';
import {freezeContracts,offlineEnvironment,verifyRuntimeContracts} from './contracts.mjs';
import {authenticateBatch} from './authenticate.mjs';
import {hash,read,durableJSON,assessDecision} from '../evaluation/acceptance-lib.mjs';

const binary=process.env.KICADAI_OWNED_TEST_BINARY,cli=process.env.KICADAI_OFFLINE_NATIVE_CLI;
const dir=path.dirname(fileURLToPath(import.meta.url));
const source='specs/board-family-v2/typed-evaluation-02/cases-02.json';
function fixture(id) {
  return {id,prompt:id==='clarify'?'I have not chosen a sensor.':id==='unsupported'?'Please use SHT31. Require its heater.':'Please use BMP280 with standard profile.'};
}
function setup(t,cases,native=false,changes={}) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'owned-collector-go-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const m={version,status:'offline-rehearsal-only',evaluation_id:'offline-owned-collector',
    policy:{goal:'offline-owned-collector',max_requests:cases.length,max_micro_usd:cases.length*50000},cases,
    binary:path.resolve(binary),binary_sha256:hash(binary),node_sha256:hash(process.execPath),collector_sha256:hash(path.join(dir,'collector.mjs')),
    prefix_args:['-test.run=^TestOwnedProcessHelper$','--'],qualification:native?'reviewed-two-family-examples':'non-design-test-fixtures',kicad_cli:native?cli:'/not-used-offline',
    case_timeout_ms:60000,total_timeout_ms:180000,
    runtime_files_sha256:Object.fromEntries([...collectorDependencies.map(f=>path.relative(process.cwd(),f)),source,'cmd/kicadai-board-family/owned_process_test.go','cmd/kicadai-board-family/owned_corpus_test.go'].map(f=>[f,hash(f)])),
    ...(native?{qualification_receipt_sha256:hash('specs/board-family-v2/evidence/examples-01.json'),kicad_cli_sha256:hash(cli)}:{}),...changes};
  m.contracts=freezeContracts(m,path.join(root,'contracts'));
  const manifestPath=path.join(root,'manifest.json');durableJSON(manifestPath,m,{exclusive:true});
  return {root,m,manifestPath,output:path.join(root,'batch'),mode:'offline'};
}
test('owned response outcomes retain all physical attempts and authenticate without credentials',{skip:!binary},async t=>{
  const options=setup(t,['invalid-extraction','malformed-output','refusal','clarify','unsupported'].map(fixture));
  const result=await collect(options);
  assert.equal(result.status,'collection-complete-semantic-review-pending');
  assert.equal(result.recorded_outcomes,5);assert.equal(result.recorded_model_failures,3);assert.equal(result.launched_cases,5);
  assert.deepEqual(result.records.map(r=>r.execution.exit_code),[1,1,1,0,0]);
  assert.deepEqual(result.records.map(r=>r.model_correct),[false,false,false,null,null]);
  assert.equal(authenticateBatch(options.manifestPath,options.output).acceptance,'offline-only-cannot-establish-live-acceptance');
  const before=hash(path.join(options.output,'ledger.json'));
  await assert.rejects(collect(options));
  assert.equal(hash(path.join(options.output,'ledger.json')),before,'repeat changed completed accounting');
});
for(const failure of ['transport-error','missing-model','crash-after-request','generation-conflict','validation-failure','timeout','log-overflow']) {
  test(`owned collector stops after ${failure} and preserves the remaining denominator`,{skip:!binary},async t=>{
    const options=setup(t,['clarify',failure,'unsupported'].map(fixture),false,['timeout','log-overflow'].includes(failure)?{case_timeout_ms:500}:{});
    const result=await collect(options);
    assert.equal(result.status,'stopped-no-retry');assert.equal(result.recorded_outcomes,1);assert.equal(result.prepared_cases,2);
    assert.deepEqual(result.unattempted_case_ids,['unsupported']);
    assert.equal(result.records[1].collection_class,'unsafe-to-continue');
    assert.equal(fs.existsSync(path.join(options.output,'unsupported')),false);
    if(failure==='timeout')assert.equal(result.records[1].execution.timed_out,true);
    if(failure==='log-overflow')assert.equal(result.records[1].execution.log_overflow,true);
    authenticateBatch(options.manifestPath,options.output);
    if(failure==='generation-conflict')assert.equal(fs.readFileSync(path.join(options.output,failure,'board/user-owned.txt'),'utf8'),'do not replace');
  });
}
test('exact source-specific contract must match the executable even after hashes are rewritten',{skip:!binary},async t=>{
  const options=setup(t,[fixture('clarify')]),binding=options.m.contracts[0];
  const contract=read(binding.path);contract.capability_context+=' invented instruction';
  durableJSON(binding.path,contract);binding.sha256=hash(binding.path);durableJSON(options.manifestPath,options.m);
  assert.doesNotThrow(()=>checkManifest(options.manifestPath)); // Hash consistency is not behavioral authenticity.
  assert.throws(()=>verifyRuntimeContracts(options.m),/differs from executable/);
  await assert.rejects(collect(options),/differs from executable/);
  assert.equal(fs.existsSync(options.output),false);
});
test('legacy versions, changed binaries, unsafe prefixes and live mode fail before creating a batch',{skip:!binary},async t=>{
  const options=setup(t,[fixture('clarify')]);
  for(const changes of [{version:'indexed-collector-1'},{binary_sha256:'0'.repeat(64)},{prefix_args:[]},{status:'live-approved'}]) {
    durableJSON(options.manifestPath,{...options.m,...changes});
    await assert.rejects(collect(options));assert.equal(fs.existsSync(options.output),false);
  }
  durableJSON(options.manifestPath,options.m);
  await assert.rejects(collect({...options,mode:'live'}),/live collection is not enabled/);
  await assert.rejects(collect({...options,approvalPath:'no-approval-accepted.json'}),/does not consume any approval/);
  assert.equal(fs.existsSync(options.output),false);
});
test('replay rejects altered response, ledger, contract, process, outcome and unrecorded files',{skip:!binary},async t=>{
  const options=setup(t,[fixture('clarify')]);await collect(options);
  const files=[
    ['clarify/journal/response.bin',b=>Buffer.concat([b,Buffer.from('tampered')])],
    ['ledger.json',b=>{const v=JSON.parse(b);v.entries[0].response_id='changed';return Buffer.from(JSON.stringify(v));}],
    ['clarify/command.process.json',b=>{const v=JSON.parse(b);v.child_terminal_observed=false;return Buffer.from(JSON.stringify(v));}],
    ['clarify/outcome.json',b=>{const v=JSON.parse(b);v.model_correct=true;return Buffer.from(JSON.stringify(v));}],
    ['result.json',b=>{const v=JSON.parse(b);v.recorded_outcomes=0;return Buffer.from(JSON.stringify(v));}],
  ];
  for(const [name,change] of files) {
    const file=path.join(options.output,name),original=fs.readFileSync(file);
    fs.writeFileSync(file,change(original));assert.throws(()=>authenticateBatch(options.manifestPath,options.output),name);fs.writeFileSync(file,original);
  }
  const extra=path.join(options.output,'unrecorded.txt');fs.writeFileSync(extra,'unrecorded');
  assert.throws(()=>authenticateBatch(options.manifestPath,options.output),/unrecorded or missing/);fs.unlinkSync(extra);
  authenticateBatch(options.manifestPath,options.output);
});
test('all 14 synthetic corpus cases traverse real command processes; all five native bundles match reviewed files',{skip:!binary||!cli},async t=>{
  assert.equal(hash(source),'90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867');
  const spec=read(source),options=setup(t,spec.cases.map(({id,prompt})=>({id,prompt})),true);
  const result=await collect(options);
  assert.equal(result.status,'collection-complete-semantic-review-pending',JSON.stringify(result.records.filter(r=>r.failure)));
  assert.equal(result.recorded_outcomes,14);assert.equal(result.recorded_model_failures,0);
  const verified=authenticateBatch(options.manifestPath,options.output);
  for(const c of spec.cases)assert.equal(assessDecision(spec,c,verified.selections[c.id].decision).automatic_checks_pass,true,c.id);
  assert.equal(result.records.filter(r=>r.bundle).length,5);
  const compared=result.records.reduce((n,r)=>n+Object.keys(r.bundle?.files_compared??{}).length,0);
  assert.equal(compared,201);
  assert.equal(verified.acceptance,'offline-only-cannot-establish-live-acceptance');
  t.diagnostic(`14 synthetic responses authenticated; 5 real native bundles; ${compared} compared files; ${result.total_wall_seconds.toFixed(3)}s; not live AI accuracy`);
});
test('standalone preflight and collector CLI finish with observed terminal state',{skip:!binary},async t=>{
  const options=setup(t,[fixture('clarify')]),collector=path.join(dir,'collector.mjs');
  const preflight=await executeRecorded(process.execPath,[collector,'--check',options.manifestPath],offlineEnvironment(),path.join(options.root,'preflight'),10000);
  assert.equal(preflight.exit_code,0);assert.equal(fs.existsSync(options.output),false);
  const actual=await executeRecorded(process.execPath,[collector,'--offline',options.manifestPath,options.output],offlineEnvironment(),path.join(options.root,'collector'),10000);
  assert.equal(actual.exit_code,0);assert.equal(actual.child_terminal_observed,true);
  assert.equal(read(path.join(options.root,'collector.stdout.log')).recorded_outcomes,1);
  authenticateBatch(options.manifestPath,options.output);
});

