import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {collect,executeRecorded,version,checkLiveApproval} from './collector.mjs';
import {hash,offlineEnvironment} from '../evaluation/acceptance-lib.mjs';
const dir=path.dirname(fileURLToPath(import.meta.url)),collector=path.join(dir,'collector.mjs'),fixture=path.join(dir,'fixtures/collector-child.mjs');
function setup(t,modes,changes={}) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'indexed-collector-test-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const manifest={version,evaluation_id:'offline-collector-test',policy:{goal:'offline-collector-test',max_requests:modes.length,max_micro_usd:modes.length*50000},
    cases:modes.map((prompt,i)=>({id:`case-${i+1}`,prompt})),binary:process.execPath,binary_sha256:hash(process.execPath),node_sha256:hash(process.execPath),collector_sha256:hash(collector),
    prefix_args:[fixture],qualification:'non-design-test-fixtures',kicad_cli:'/not-used-offline',case_timeout_ms:5000,total_timeout_ms:30000,
    runtime_files_sha256:Object.fromEntries([fixture,collector,path.join(dir,'runtime-qualification.mjs'),path.resolve(dir,'../evaluation/acceptance-lib.mjs'),path.resolve(dir,'../development/replay-normalization.mjs')].map(f=>[path.relative(process.cwd(),f),hash(f)])),...changes};
  const manifestPath=path.join(root,'manifest.json'); fs.writeFileSync(manifestPath,JSON.stringify(manifest),{mode:0o600});
  return {root,manifestPath,output:path.join(root,'batch'),mode:'offline'};
}
test('records completed bad answers and continues without retry',async t=>{
  const options=setup(t,['invalid','refusal','clarify','unsupported']);
  const result=await collect(options);
  assert.equal(result.status,'collection-complete-semantic-review-pending');
  assert.equal(result.planned_cases,4);assert.equal(result.launched_cases,4);assert.equal(result.recorded_outcomes,4);assert.equal(result.recorded_model_failures,2);
  assert.deepEqual(result.unattempted_case_ids,[]);
  assert.deepEqual(result.records.map(r=>r.execution.exit_code),[1,1,0,0]);
  assert.deepEqual(result.records.map(r=>r.model_correct),[false,false,null,null]);
  assert.ok(result.total_wall_seconds>result.records.reduce((n,r)=>n+r.execution.wall_seconds,0));
  const before=hash(path.join(options.output,'ledger.json'));
  await assert.rejects(collect(options));
  assert.equal(hash(path.join(options.output,'ledger.json')),before,'repeat changed budget history');
});
for(const mode of ['transport','crash','timeout','log-overflow','bad-audit','duplicate-id','tamper-prefix','native-failure']) test(`stops on ${mode}, keeps failed case and denominator`,async t=>{
  const options=setup(t,['clarify',mode,'unsupported'],mode==='timeout'||mode==='log-overflow'?{case_timeout_ms:500}:{});
  const result=await collect(options);
  assert.equal(result.status,'stopped-no-retry');assert.equal(result.planned_cases,3);assert.equal(result.prepared_cases,2);assert.equal(result.recorded_outcomes,1);
  assert.deepEqual(result.unattempted_case_ids,['case-3']);assert.equal(result.records[1].advance,false);assert.equal(result.records[1].collection_class,'unsafe-to-continue');
  assert.ok(fs.existsSync(path.join(options.output,'case-2/outcome.json')));
  assert.equal(fs.existsSync(path.join(options.output,'case-3')),false);
  if(mode==='timeout') assert.equal(result.records[1].execution.timed_out,true);
  if(mode==='log-overflow') assert.equal(result.records[1].execution.log_overflow,true);
});
test('rejects changed executable before creating a batch',async t=>{
  const options=setup(t,['clarify'],{binary_sha256:'0'.repeat(64)});
  await assert.rejects(collect(options));assert.equal(fs.existsSync(options.output),false);
});
test('live refuses synthetic qualification and never consumes a key',async t=>{
  const options=setup(t,['clarify']);
  await assert.rejects(collect({...options,mode:'live',approvalPath:path.join(options.root,'absent-approval.json')}));
  assert.equal(fs.existsSync(options.output),false);
});
test('spawn failure has actual terminal close and cannot appear as exit zero',async t=>{
  const options=setup(t,['clarify']);
  const result=await executeRecorded(path.join(options.root,'missing-command'),[],offlineEnvironment(),path.join(options.root,'spawn'),1000);
  assert.equal(result.child_terminal_observed,true);assert.equal(result.child_pid,null);assert.equal(result.spawn_error,'ENOENT');assert.notEqual(result.exit_code,0);
});
test('existing log prevents a launch and is not replaced',async t=>{
  const options=setup(t,['clarify']),stem=path.join(options.root,'existing');
  fs.writeFileSync(stem+'.stdout.log','owned-before-test');
  await assert.rejects(executeRecorded(process.execPath,['-e','process.exit(0)'],offlineEnvironment(),stem,1000));
  assert.equal(fs.readFileSync(stem+'.stdout.log','utf8'),'owned-before-test');
  assert.equal(fs.existsSync(stem+'.process.json'),false);
});
test('standalone collector CLI records its own final exit and complete elapsed time',async t=>{
  const options=setup(t,['invalid','clarify']);
  const stem=path.join(options.root,'collector-process');
  const result=await executeRecorded(process.execPath,[collector,'--offline',options.manifestPath,options.output],offlineEnvironment(),stem,10000);
  assert.equal(result.exit_code,0);assert.equal(result.child_terminal_observed,true);
  const summary=JSON.parse(fs.readFileSync(stem+'.stdout.log','utf8'));
  assert.equal(summary.recorded_outcomes,2);assert.equal(summary.recorded_model_failures,1);assert.equal(summary.acceptance,'not-established-by-collection');
  assert.ok(summary.total_wall_seconds>0 && result.wall_seconds>=summary.total_wall_seconds);
});
test('manifest preflight does not create output or spend authority',async t=>{
  const options=setup(t,['clarify']),stem=path.join(options.root,'check');
  const p=await executeRecorded(process.execPath,[collector,'--check',options.manifestPath],offlineEnvironment(),stem,10000);
  assert.equal(p.exit_code,0);assert.equal(fs.existsSync(options.output),false);
  assert.equal(JSON.parse(fs.readFileSync(stem+'.stdout.log','utf8')).live_authorization,'not-checked-or-granted');
});
test('approval validation is pure and binds exact runtime, identity, request and dollar caps',()=>{
  const m={evaluation_id:'test-only-approval',policy:{max_requests:14,max_micro_usd:1000000}},sha='a'.repeat(64);
  const a={status:'explicit-user-approved',evaluation_id:m.evaluation_id,manifest_sha256:sha,max_physical_requests:14,max_micro_usd:1000000,existing_key_only:true,user_message:'Synthetic test fixture; NOT an actual user approval.',recorded_utc:'2026-09-14T00:00:00Z'};
  assert.doesNotThrow(()=>checkLiveApproval(a,m,sha));
  for(const [key,value] of Object.entries({status:'pending',evaluation_id:'other',manifest_sha256:'b'.repeat(64),max_physical_requests:15,max_micro_usd:2000000,existing_key_only:false,user_message:'',recorded_utc:'invalid'}))assert.throws(()=>checkLiveApproval({...a,[key]:value},m,sha),key);
});
