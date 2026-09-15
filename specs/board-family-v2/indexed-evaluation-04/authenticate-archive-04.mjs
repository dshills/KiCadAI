// Portable, offline publication authentication. Does not run the macOS runtime,
// perform native validation, read credentials, or contact any network service.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {relativeFile,parseBatch} from '../history-verification-04/authenticate-indexed-03.mjs';
import {hash,hashBytes,read,inventory,authenticateFiles,authenticateExamples,offlineEnvironment} from '../evaluation/acceptance-lib.mjs';
import {loadPlan,scoreReview,scoringDependencies} from '../source-reference-candidate-03/scoring.mjs';
import {checkLiveApproval,cleanTerminal,validateLedger,assessAttempt} from '../source-reference-candidate-03/collector.mjs';

export const sourceCommit='7130f4db199d72468409d2d8d3d6a2f365fb9257';
export const publication='specs/board-family-v2/indexed-evaluation-04';
const pins={
  'approval.json':'ce871f2c555984a865053ba972452bb77dc6d36535c2f4572bb855c06e85f632',
  'runtime-manifest.json':'d3996a549ae1995f933179f8f73e93e21e23cb559465830ec5291b9ecc8322e4',
  'runtime-qualification.json':'75c8a748267ab780efc142a902b8dd83bcb55ce0606f3e6e4b455cc47943ef09',
  'runtime-contract.json':'4fbe0c4dbe31b70f634ce0dd9adcdea6777b7a9d3b39e39dba21c163788a9bf9',
  'review.json':'7414b1a45c7153ef1d12c7716ceeab2ca752d016e774c52c7bf0104857ca4b15',
  'results.json':'2dd0742d3919934f1b377a25439afed64a1717d05ac3f140e743d8e2c2ce5065',
  'metrics.json':'aef7c7d1dd715d4276032e325203a724bfaf4de1160843070100ae6a425e2dcd',
  'execution-observation.json':'be42ddf7bb2cb48ce8ae10f074f6955f256b45988e3026a91620660d4749d81a',
  'readiness-review.md':'6f995afb0398017068f5208e65129081dfee047b25bfe5bc2e5da938f77c6590',
  'batch/result.json':'3398d3004f3d996eda544e0ef370517fab79a0fd9375d4c3dc4e6af182ca6697',
  'batch/start.json':'2fe52e3912094f7cb248c240f39ff7c2042da9541f771279f9216391d542ad0a',
  'batch/budget.json':'d62d111031f5263f30cda0f0d84b97b65ef98f431ca620301f3db84b528ee24d',
  'batch/ledger.json':'75bfbb31a8ee87613ba5c0e20f9369d20740dee692362d6324ad358a0ee82ce7',
};
const keys=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS'];
function git(args,input){
  const r=spawnSync('git',['-c','core.fsmonitor=false',...args],{input,env:{...offlineEnvironment(),GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:'/dev/null'},timeout:30000,maxBuffer:256*1024*1024});
  assert.equal(r.error,undefined);assert.equal(r.signal,null);assert.equal(r.status,0,'recorded Git source unavailable; no network fetch is attempted');return r.stdout;
}
export function authenticateArchive({root=path.dirname(fileURLToPath(import.meta.url))}={}){
  for(const k of keys)assert.ok(!process.env[k],'archive checks require provider credentials to be unset');
  for(const [file,sha]of Object.entries(pins)){assert.ok(fs.lstatSync(path.join(root,file)).isFile());assert.equal(hash(path.join(root,file)),sha,'pinned publication changed: '+file);}
  const m=read(path.join(root,'runtime-manifest.json')),q=read(path.join(root,'runtime-qualification.json'));
  const contract=read(path.join(root,'runtime-contract.json')),approval=read(path.join(root,'approval.json'));
  assert.equal(m.production_qualification.sha256,pins['runtime-qualification.json']);
  assert.equal(m.runtime_files_sha256[m.scoring_contract_path],pins['runtime-contract.json']);
  assert.equal(contract.request_revision,'indexed-request-04');
  const tree=new Set(git(['ls-tree','--full-tree','-r','--name-only','-z',sourceCommit]).toString('utf8').split('\0').filter(Boolean).map(relativeFile));
  const frozen=Object.keys(m.runtime_files_sha256).map(relativeFile),tracked=frozen.filter(f=>tree.has(f)),cached=frozen.filter(f=>!tree.has(f));
  assert.equal(frozen.length,2536);assert.ok(cached.every(f=>f.startsWith('.cache/')));
  const sourceHashes=parseBatch(git(['cat-file','--batch'],tracked.map(f=>`${sourceCommit}:${f}\n`).join('')),tracked);
  for(const f of tracked)assert.equal(sourceHashes[f],m.runtime_files_sha256[f],'Git source differs from frozen bytes: '+f);
  for(const [f,sha]of Object.entries(q.source_sha256))assert.equal(m.runtime_files_sha256[relativeFile(f)],sha);
  assert.equal(q.commands.length,11);for(const c of q.commands){cleanTerminal(c.result);assert.equal(c.result.exit_code,0);}
  // Replay scoring with its frozen dependencies; do not silently use later code.
  const plan='specs/board-family-v2/source-reference-candidate-03/evaluation-plan.json';
  const {spec}=loadPlan(plan);
  for(const f of [...scoringDependencies,plan,'specs/board-family-v2/typed-evaluation-02/cases-02.json'])assert.equal(hash(f),m.runtime_files_sha256[f],'scoring input changed: '+f);
  checkLiveApproval(approval,m,pins['runtime-manifest.json']);
  const batch=path.join(root,'batch'),state=read(path.join(batch,'result.json')),start=read(path.join(batch,'start.json')),ledger=read(path.join(batch,'ledger.json'));
  assert.equal(start.manifest_sha256,pins['runtime-manifest.json']);assert.equal(start.approval_sha256,pins['approval.json']);
  assert.equal(start.mode,'live');assert.equal(state.mode,'live');assert.equal(start.evaluation_id,m.evaluation_id);assert.equal(state.evaluation_id,m.evaluation_id);
  assert.deepEqual(start.planned_case_ids,spec.cases.map(c=>c.id));assert.deepEqual(m.cases,spec.cases.map(({id,prompt})=>({id,prompt})));
  assert.deepEqual(read(path.join(batch,'budget.json')),m.policy);validateLedger(ledger,m.policy);assert.equal(ledger.entries.length,14);
  assert.equal(state.status,'collection-complete-semantic-review-pending');assert.equal(state.planned_cases,14);assert.equal(state.prepared_cases,14);assert.equal(state.launched_cases,14);assert.equal(state.recorded_outcomes,14);assert.equal(state.recorded_model_failures,2);assert.deepEqual(state.unattempted_case_ids,[]);
  assert.deepEqual(state.records.map(r=>r.id),spec.cases.map(c=>c.id));
  const examples=authenticateExamples().receipt.cases,selections={},selectionSHA={};let prefix=[];
  for(const [i,r]of state.records.entries()){
    const cr=path.join(batch,r.id),journal=path.join(cr,'journal');
    assert.deepEqual(read(path.join(cr,'outcome.json')),r);authenticateFiles(cr,r.files_sha256);
    assert.deepEqual(inventory(cr).filter(f=>f!=='outcome.json'),Object.keys(r.files_sha256).sort());
    for(const stem of ['command','audit']){const p=read(path.join(cr,stem+'.process.json'));assert.deepEqual(p,stem==='command'?r.execution:r.audit_execution);cleanTerminal(p);}
    assert.equal(r.audit_execution.exit_code,0);assert.equal(r.state,'recorded-outcome');
    const sf=path.join(journal,'selection/selection.json'),s=read(sf),request=read(path.join(journal,'request/receipt.json')),response=read(path.join(journal,'response/receipt.json'));
    assert.equal(request.sha256,hash(path.join(journal,'request/body.bin')));assert.equal(response.sha256,hash(path.join(journal,'response.bin')));
    assert.equal(response.http_status,200);assert.equal(response.eof_observed,true);for(const k of ['truncated','transport_error','read_error','close_error'])assert.equal(response[k],false);
    assert.equal(hash(path.join(cr,'prompt.txt')),hashBytes(spec.cases[i].prompt));
    assert.deepEqual(read(path.join(cr,'attempt.json')),{id:r.id,prompt_sha256:r.prompt_sha256,state:'attempt-recorded-before-launch',started_utc:r.started_utc});
    const snap=read(path.join(journal,'selection/ledger.json')),audit=read(path.join(cr,'audit.stdout.log'));
    const checked=assessAttempt({execution:r.execution,audit,ledger:snap,policy:m.policy,prefix,prompt:spec.cases[i].prompt,caseRoot:cr,examples});
    for(const k of ['collection_class','advance','model_correct','response_id','ledger_index','bundle'])assert.deepEqual(r[k],checked[k]);
    prefix=snap.entries;selections[r.id]=s;selectionSHA[r.id]=hash(sf);
  }
  assert.deepEqual(ledger.entries,prefix);
  const expected=['start.json','budget.json','result.json','ledger.json',...state.records.flatMap(r=>['outcome.json',...Object.keys(r.files_sha256)].map(f=>`${r.id}/${f}`))].sort();
  assert.deepEqual(inventory(batch),expected);assert.equal(expected.length,415);
  const bindings={plan_sha256:hash(plan),manifest_sha256:pins['runtime-manifest.json'],result_sha256:pins['batch/result.json'],cases_sha256:hash('specs/board-family-v2/typed-evaluation-02/cases-02.json'),selection_sha256:selectionSHA};
  const results=scoreReview({spec,state,selections,schema:contract.schema,review:read(path.join(root,'review.json')),bindings});
  assert.deepEqual(results,read(path.join(root,'results.json')));
  const metrics=read(path.join(root,'metrics.json'));
  for(const k of ['raw_passes','application_passes','complete_passes','acceptance','median_all_useful_seconds','maximum_all_useful_seconds','timing_target_met'])assert.equal(metrics[k],results[k]);
  const sum=k=>ledger.entries.reduce((n,e)=>n+(e[k]??0),0);
  for(const k of ['input_tokens','output_tokens','estimated_micro_usd'])assert.equal(metrics[k],sum(k));
  assert.equal(metrics.historical_reserved_micro_usd,sum('reserve_micro_usd'));
  assert.equal(metrics.verified_useful_native_bundles,state.records.filter(r=>r.bundle).length);
  assert.equal(metrics.native_files_compared,state.records.reduce((n,r)=>n+Object.keys(r.bundle?.files_compared??{}).length,0));
  return {status:'archive-source-accounting-and-scores-verified',publication_files:expected.length+9,git_source_files:tracked.length,frozen_cache_files_checked:0,runtime_verified:false,native_tools_rerun:false,raw_passes:results.raw_passes,application_passes:results.application_passes,complete_passes:results.complete_passes,physical_requests:14,unattempted_cases:0,estimated_micro_usd:metrics.estimated_micro_usd,acceptance:results.acceptance,live_requests_in_this_check:0,notice:'Portable preservation and arithmetic check; original runtime replay is a separate local check. No provider attestation or new execution authority.'};
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  try{assert.equal(process.argv.length,2);console.log(JSON.stringify(authenticateArchive(),null,2));}
  catch(e){console.error(e.message);process.exitCode=1;}
}
