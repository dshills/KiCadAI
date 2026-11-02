// Opt-in integration against a compiled Go test binary: real main, guarded
// request builder, raw journal, read-only Go replay audit, ledger and collector.
// Only the HTTP transport is synthetic. Optional native cases use real KiCad.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {collect,version} from './collector.mjs';
import {hash} from '../evaluation/acceptance-lib.mjs';
const binary=process.env.KICADAI_INDEXED_TEST_BINARY,cli=process.env.KICADAI_OFFLINE_NATIVE_CLI;
const dir=path.dirname(fileURLToPath(import.meta.url));
function setup(t,modes,native=false) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'indexed-collector-go-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const m={version,evaluation_id:'offline-go-collector',policy:{goal:'offline-go-collector',max_requests:modes.length,max_micro_usd:modes.length*50000},
    cases:modes.map(id=>({id,prompt:id==='clarify'?'I have not chosen a sensor.':id==='unsupported'?'Please use SHT31. I require heater operation.':id==='native-SHT31'?'Please use SHT31 with standard profile.':'Please use BMP280 with standard profile.'})),
    binary:path.resolve(binary),binary_sha256:hash(binary),node_sha256:hash(process.execPath),collector_sha256:hash(path.join(dir,'collector.mjs')),
    prefix_args:['-test.run=^TestIndexedCommandProcessHelper$','--'],qualification:native?'reviewed-two-family-examples':'non-design-test-fixtures',kicad_cli:native?cli:'/not-used-offline',
    case_timeout_ms:60000,total_timeout_ms:180000,runtime_files_sha256:Object.fromEntries(['cmd/kicadai-board-family/main.go','cmd/kicadai-board-family/indexed_process_test.go','internal/boardfamily/intent_reference_audit.go',path.relative(process.cwd(),path.join(dir,'collector.mjs')),path.relative(process.cwd(),path.join(dir,'runtime-qualification.mjs')),'specs/board-family-v2/evaluation/acceptance-lib.mjs','specs/board-family-v2/development/replay-normalization.mjs'].map(f=>[f,hash(f)])),
    ...(native?{qualification_receipt_sha256:hash('specs/board-family-v2/evidence/examples-01.json'),kicad_cli_sha256:hash(cli)}:{})};
  const manifestPath=path.join(root,'manifest.json');fs.writeFileSync(manifestPath,JSON.stringify(m),{mode:0o600});
  return {manifestPath,output:path.join(root,'batch'),mode:'offline'};
}
test('real Go journal replay lets recorded invalid answers finish one bounded batch',{skip:!binary},async t=>{
  const result=await collect(setup(t,['invalid-extraction','malformed-output','refusal','clarify','unsupported']));
  assert.equal(result.status,'collection-complete-semantic-review-pending');assert.equal(result.recorded_outcomes,5);assert.equal(result.recorded_model_failures,3);
  assert.equal(result.records.filter(r=>r.model_correct===false).length,3);assert.equal(result.launched_cases,5);
  for(const r of result.records) {assert.equal(r.execution.child_terminal_observed,true);assert.equal(r.audit_execution.exit_code,0);}
});
for(const mode of ['transport-error','missing-model','crash-after-request','generation-conflict','validation-failure']) test(`real Go ${mode} stops before the next case`,{skip:!binary},async t=>{
  const result=await collect(setup(t,['clarify',mode,'unsupported']));
  assert.equal(result.status,'stopped-no-retry');assert.equal(result.recorded_outcomes,1);assert.equal(result.prepared_cases,2);assert.deepEqual(result.unattempted_case_ids,['unsupported']);
  assert.equal(result.records[1].collection_class,'unsafe-to-continue');
});
test('complete seven-case collection includes both real reviewed native bundles',{skip:!binary||!cli},async t=>{
  const result=await collect(setup(t,['invalid-extraction','malformed-output','refusal','clarify','unsupported','native-BMP280','native-SHT31'],true));
  assert.equal(result.status,'collection-complete-semantic-review-pending');assert.equal(result.recorded_outcomes,7);assert.equal(result.recorded_model_failures,3);
  for(const r of result.records.slice(-2)) {assert.equal(r.execution.exit_code,0);assert.ok(Object.keys(r.bundle.files_compared).length>0);assert.equal(r.model_correct,null);}
  t.diagnostic(`7/7 recorded; 3 measured model failures; 2 reviewed native bundles; total wall=${result.total_wall_seconds.toFixed(3)}s; not an AI accuracy result`);
});
