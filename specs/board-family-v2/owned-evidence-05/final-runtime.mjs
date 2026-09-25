// Final owned-v4 runtime gates. All preparation and verification are offline.
// A manifest or CI receipt is not spending authority. The user must approve the
// exact manifest, fixed batch destination, caps and separate release-gates hash.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {hash,read,durableJSON,inventory,authenticateFiles} from '../evaluation/acceptance-lib.mjs';
import {offlineEnvironment,checkContracts,verifyRuntimeContracts} from './contracts.mjs';
import {sourceFiles,verifyPackage} from './prepare-rehearsal.mjs';
import {checkReview} from './review-runner.mjs';
import {scoringDependencies,loadCorpus} from './scoring.mjs';
import {executeRecorded,cleanTerminal} from './collector.mjs';

export const version='owned-final-runtime-1';
export const evaluationID='board-family-v2-owned-final-05';
export const model='gpt-4.1-mini-2025-04-14';
export const endpoint='https://api.openai.com/v1/responses';
export const requiredWorkflows=[
  'CI','Typed intent final evidence','Typed intent evaluation safeguards',
  'Indexed request 04 final evidence','Indexed intent evaluation safeguards',
  'Owned intent offline rehearsal safeguards','Owned intent semantic scoring safeguards',
  'Owned intent final collection safeguards',
];
const own=['final-runtime.mjs','final-collector.mjs','final-authenticate.mjs','final-scoring.mjs',
  'final-runtime.test.mjs','final-collector.test.mjs','final-scoring.test.mjs',
  '../../../.github/workflows/owned-final-collection.yml','prepare-rehearsal.mjs'];
export const finalDependencies=[...new Set([...scoringDependencies,...own.map(f=>fileURLToPath(new URL(f,import.meta.url)))])].sort();
const sha=x=>assert.match(x,/^[a-f0-9]{64}$/);
const text=x=>assert.ok(typeof x==='string'&&x.trim().length>0);
const date=x=>{assert.ok(typeof x==='string'&&Number.isFinite(Date.parse(x)));return Date.parse(x);};
export function boundedJSON(file,limit=4*1024*1024) {
  const stat=fs.lstatSync(file);assert.ok(stat.isFile()&&stat.size<=limit,'expected bounded regular JSON file');
  return JSON.parse(fs.readFileSync(file,'utf8'));
}
export function pin(file,digest) {
  sha(digest);assert.ok(fs.lstatSync(file).isFile(),'frozen input must be regular');
  assert.equal(hash(file),digest,'frozen input changed');
}
function git(args) {
  const r=spawnSync('git',args,{env:offlineEnvironment(),encoding:'utf8',timeout:10000,maxBuffer:2*1024*1024});
  assert.equal(r.error,undefined);assert.equal(r.signal,null);assert.equal(r.status,0,'git source check failed');return r.stdout.trim();
}
export function validateManifest(m,mode) {
  assert.ok(['offline','live'].includes(mode));
  assert.equal(m.version,version);assert.equal(m.status,'frozen-not-live-authorized');
  assert.ok(['offline-test','live-candidate'].includes(m.kind));
  assert.match(m.evaluation_id,/^[a-z0-9][a-z0-9-]{2,79}$/);
  assert.equal(m.policy.goal,m.evaluation_id);
  assert.ok(Array.isArray(m.cases)&&m.cases.length>0&&m.cases.length<=14);
  assert.equal(new Set(m.cases.map(c=>c.id)).size,m.cases.length);
  for(const c of m.cases) {
    assert.match(c.id,/^[a-zA-Z0-9][a-zA-Z0-9-]{0,39}$/);
    assert.ok(typeof c.prompt==='string'&&Buffer.byteLength(c.prompt)>0&&Buffer.byteLength(c.prompt)<=2000);
  }
  assert.equal(m.policy.max_requests,m.cases.length);
  assert.ok(Number.isSafeInteger(m.policy.max_micro_usd)&&m.policy.max_micro_usd>=m.cases.length*50000&&m.policy.max_micro_usd<=1000000);
  assert.ok(path.isAbsolute(m.batch_directory));
  assert.equal(path.join(fs.realpathSync(path.dirname(m.batch_directory)),path.basename(m.batch_directory)),m.batch_directory,'batch path must be canonical');
  assert.equal(m.workspace_root,fs.realpathSync(process.cwd()));
  assert.ok(path.isAbsolute(m.binary));pin(m.binary,m.binary_sha256);pin(process.execPath,m.node_sha256);
  assert.ok(path.isAbsolute(m.kicad_cli));
  assert.ok(Number.isSafeInteger(m.case_timeout_ms)&&m.case_timeout_ms>=100&&m.case_timeout_ms<=120000);
  assert.ok(Number.isSafeInteger(m.total_timeout_ms)&&m.total_timeout_ms>=m.case_timeout_ms&&m.total_timeout_ms<=900000);
  if(m.kind==='live-candidate') {
    assert.equal(mode,'live','live candidate is not an offline fixture');
    assert.equal(m.evaluation_id,evaluationID);
    assert.deepEqual(m.cases,loadCorpus().cases.map(({id,prompt})=>({id,prompt})));
    assert.deepEqual(m.policy,{goal:evaluationID,max_requests:14,max_micro_usd:1000000});
    assert.deepEqual(m.prefix_args,[],'production executable takes no test prefix');
    assert.equal(m.case_timeout_ms,120000);assert.equal(m.total_timeout_ms,900000);
    assert.equal(m.qualification,'reviewed-two-family-examples');
    assert.match(m.source_commit,/^[a-f0-9]{40}$/);
    assert.equal(git(['rev-parse','HEAD']),m.source_commit,'run from the frozen source commit');
    assert.ok(m.reuse&&path.isAbsolute(m.reuse.rehearsal_root)&&path.isAbsolute(m.reuse.review_root));
    pin(m.kicad_cli,m.kicad_cli_sha256);
    pin('specs/board-family-v2/evidence/examples-01.json',m.qualification_receipt_sha256);
    assert.ok(Object.keys(m.reuse_files_sha256??{}).length>0);
    authenticateFiles('.',m.reuse_files_sha256);
  } else {
    assert.equal(mode,'offline','test helpers can never run in live mode');
    assert.deepEqual(m.prefix_args,['-test.run=^TestOwnedProcessHelper$','--']);
    assert.equal(m.qualification,'non-design-test-fixtures');
    assert.equal(m.reuse,null);
  }
  assert.ok(Object.keys(m.runtime_files_sha256??{}).length>0);
  for(const f of finalDependencies)assert.equal(m.runtime_files_sha256[path.relative(process.cwd(),f)],hash(f),'final dependency missing from freeze');
  authenticateFiles('.',m.runtime_files_sha256);checkContracts(m);
  return m;
}
export function checkManifest(file,mode) {
  return validateManifest(boundedJSON(file),mode);
}
export function verifyReuse(m) {
  verifyRuntimeContracts(m);
  if(m.kind==='offline-test')return {status:'offline-fixture-only',live_requests:0};
  const r=m.reuse,q=boundedJSON(path.join(r.rehearsal_root,'preparation.json'));
  pin(path.join(r.rehearsal_root,'manifest.json'),r.manifest_sha256);
  pin(path.join(r.rehearsal_root,'preparation.json'),r.preparation_sha256);
  pin(path.join(r.review_root,'freeze.json'),r.review_freeze_sha256);
  pin(path.join(r.review_root,'review.json'),r.review_sha256);
  assert.equal(m.binary,q.production_binary);assert.equal(m.binary_sha256,q.production_binary_sha256);
  const old=boundedJSON(path.join(r.rehearsal_root,'manifest.json'));
  pin(path.join(r.safeguards_root,'verification.json'),r.safeguards_sha256);
  verifyFinalChecks(r.safeguards_root,old);
  assert.deepEqual(m.contracts,old.contracts,'reuse exact production-exported contracts');
  for(const [f,d] of Object.entries(q.source_sha256))assert.equal(m.runtime_files_sha256[f],d,'qualified application source changed');
  const rehearsal=verifyPackage(r.rehearsal_root),review=checkReview(r.review_root,path.join(r.review_root,'review.json'));
  assert.equal(review.criteria_met,true);assert.equal(review.acceptance,'offline-only-cannot-establish-live-acceptance');
  return {status:'unchanged-offline-qualification-authenticated',rehearsal,synthetic_complete:review.complete_passes,live_requests:0};
}
export function validateGates(m,manifestSHA,gates) {
  sha(manifestSHA);
  assert.equal(gates.version,'owned-live-release-gates-1');assert.equal(gates.status,'passed');
  assert.equal(gates.manifest_sha256,manifestSHA);assert.equal(gates.source_commit,m.source_commit);
  assert.ok(['implementing-agent','independent-human','independent-agent'].includes(gates.review?.kind));
  text(gates.review.name);text(gates.review.scope);assert.equal(gates.review.findings_open,0);date(gates.review.reviewed_utc);
  assert.equal(gates.ci?.repository,'dshills/KiCadAI');assert.equal(gates.ci.head_sha,m.source_commit);date(gates.ci.checked_utc);
  assert.deepEqual(gates.ci.workflows.map(r=>r.name).sort(),[...requiredWorkflows].sort(),'all exact-source CI workflows required');
  assert.equal(new Set(gates.ci.workflows.map(r=>r.id)).size,requiredWorkflows.length);
  for(const r of gates.ci.workflows) {
    assert.ok(Number.isSafeInteger(r.id)&&r.id>0);assert.equal(r.head_sha,m.source_commit);
    assert.equal(r.status,'completed');assert.equal(r.conclusion,'success');
    assert.equal(r.url,'https://github.com/dshills/KiCadAI/actions/runs/'+r.id);
  }
}
export function validateApproval(m,manifestSHA,gatesSHA,approval,at=new Date().toISOString()) {
  sha(manifestSHA);sha(gatesSHA);
  assert.equal(m.kind,'live-candidate');
  assert.equal(approval.version,'owned-live-approval-1');assert.equal(approval.approved,true);
  assert.equal(approval.source,'explicit-user-message');text(approval.user_message);
  assert.equal(approval.evaluation_id,m.evaluation_id);assert.equal(approval.manifest_sha256,manifestSHA);
  assert.equal(approval.release_gates_sha256,gatesSHA);assert.equal(approval.batch_directory,m.batch_directory);
  assert.equal(approval.model,model);assert.equal(approval.endpoint,endpoint);
  assert.equal(approval.max_requests,m.policy.max_requests);assert.equal(approval.max_micro_usd,m.policy.max_micro_usd);
  assert.equal(approval.reuse_existing_key,true);assert.equal(approval.retries,false);assert.equal(approval.recovery,false);
  const approved=date(approval.approved_utc),expires=date(approval.expires_utc),now=date(at);
  assert.ok(expires>approved&&expires-approved<=7*24*3600*1000,'approval must expire within seven days');
  assert.ok(now>=approved&&now<expires,'approval is not currently valid');
}
export function checkAuthority(m,manifestFile,approvalFile,gatesFile,at) {
  assert.ok(approvalFile&&gatesFile,'separate fresh approval and release gates required');
  const gates=boundedJSON(gatesFile),approval=boundedJSON(approvalFile),manifestSHA=hash(manifestFile),gatesSHA=hash(gatesFile);
  validateGates(m,manifestSHA,gates);validateApproval(m,manifestSHA,gatesSHA,approval,at);
  assert.ok(date(approval.approved_utc)>=Math.max(date(gates.review.reviewed_utc),date(gates.ci.checked_utc)),'approval must follow completed release gates');
  return {approval_sha256:hash(approvalFile),release_gates_sha256:gatesSHA};
}
const testArgs=['--test','specs/board-family-v2/owned-evidence-05/final-runtime.test.mjs',
  'specs/board-family-v2/owned-evidence-05/final-collector.test.mjs','specs/board-family-v2/owned-evidence-05/final-scoring.test.mjs'];
export function verifyFinalChecks(root,rehearsalManifest) {
  const q=boundedJSON(path.join(root,'verification.json'));
  assert.equal(q.version,'owned-final-safeguards-1');assert.equal(q.status,'passed-offline');assert.equal(q.live_requests,0);
  assert.deepEqual(q.command.args,testArgs);assert.equal(q.command.executable,process.execPath);
  assert.equal(q.node_sha256,hash(process.execPath));assert.equal(q.test_binary,rehearsalManifest.binary);
  assert.equal(q.test_binary_sha256,rehearsalManifest.binary_sha256);pin(q.test_binary,q.test_binary_sha256);
  assert.deepEqual(Object.keys(q.files_sha256).sort(),finalDependencies.map(f=>path.relative(process.cwd(),f)).sort());
  authenticateFiles('.',q.files_sha256);
  cleanTerminal(q.command.result);assert.equal(q.command.result.exit_code,0);
  assert.deepEqual(read(path.join(root,'tests.process.json')),q.command.result);
  for(const name of ['tests.process.json','tests.stdout.log','tests.stderr.log'])pin(path.join(root,name),q.logs_sha256[name]);
  return {status:q.status,live_requests:0,wall_seconds:q.command.result.wall_seconds};
}
export async function runFinalChecks(rehearsalRoot,output) {
  verifyPackage(rehearsalRoot);
  const m=boundedJSON(path.join(rehearsalRoot,'manifest.json'));
  const files=Object.fromEntries(finalDependencies.map(f=>[path.relative(process.cwd(),f),hash(f)]));
  fs.mkdirSync(output,{mode:0o700});
  const env={...offlineEnvironment(),KICADAI_OWNED_TEST_BINARY:m.binary,KICADAI_OWNED_REHEARSAL_ROOT:path.resolve(rehearsalRoot)};
  const result=await executeRecorded(process.execPath,testArgs,env,path.join(output,'tests'),120000);
  authenticateFiles('.',files);cleanTerminal(result);assert.equal(result.exit_code,0,'final safeguard process failed');
  const q={version:'owned-final-safeguards-1',status:'passed-offline',live_requests:0,node_sha256:hash(process.execPath),
    test_binary:m.binary,test_binary_sha256:m.binary_sha256,files_sha256:files,command:{executable:process.execPath,args:testArgs,result},
    logs_sha256:Object.fromEntries(['tests.process.json','tests.stdout.log','tests.stderr.log'].map(f=>[f,hash(path.join(output,f))]))};
  durableJSON(path.join(output,'verification.json'),q,{exclusive:true});return verifyFinalChecks(output,m);
}
export function freezeFinal(rehearsalRoot,reviewRoot,checksRoot,output) {
  assert.equal(git(['status','--porcelain']), '', 'commit preparation before freezing its source');
  const root=path.resolve(output);assert.equal(fs.existsSync(root),false,'never replace an earlier runtime');
  const rehearsal=path.resolve(rehearsalRoot),review=path.resolve(reviewRoot),checks=path.resolve(checksRoot);
  verifyPackage(rehearsal);
  const scored=checkReview(review,path.join(review,'review.json'));
  assert.equal(scored.criteria_met,true);assert.equal(scored.acceptance,'offline-only-cannot-establish-live-acceptance');
  const q=boundedJSON(path.join(rehearsal,'preparation.json')),old=boundedJSON(path.join(rehearsal,'manifest.json'));
  verifyFinalChecks(checks,old);
  const files=[...new Set([...sourceFiles(),...finalDependencies.map(f=>path.relative(process.cwd(),f))])].sort();
  const reuseFiles=[...inventory(rehearsal).map(f=>path.join(rehearsal,f)),...inventory(review).map(f=>path.join(review,f)),...inventory(checks).map(f=>path.join(checks,f))];
  fs.mkdirSync(root,{mode:0o700});
  const m={version,status:'frozen-not-live-authorized',kind:'live-candidate',evaluation_id:evaluationID,
    workspace_root:fs.realpathSync(process.cwd()),source_commit:git(['rev-parse','HEAD']),batch_directory:path.join(fs.realpathSync(root),'batch'),
    policy:{goal:evaluationID,max_requests:14,max_micro_usd:1000000},cases:loadCorpus().cases.map(({id,prompt})=>({id,prompt})),contracts:old.contracts,
    binary:q.production_binary,binary_sha256:q.production_binary_sha256,prefix_args:[],
    node_sha256:hash(process.execPath),kicad_cli:old.kicad_cli,kicad_cli_sha256:old.kicad_cli_sha256,
    qualification:'reviewed-two-family-examples',qualification_receipt_sha256:old.qualification_receipt_sha256,
    case_timeout_ms:120000,total_timeout_ms:900000,
    runtime_files_sha256:Object.fromEntries(files.map(f=>[f,hash(f)])),
    reuse_files_sha256:Object.fromEntries(reuseFiles.map(f=>[path.relative(process.cwd(),f),hash(f)])),
    reuse:{rehearsal_root:rehearsal,review_root:review,safeguards_root:checks,safeguards_sha256:hash(path.join(checks,'verification.json')),manifest_sha256:hash(path.join(rehearsal,'manifest.json')),
      preparation_sha256:hash(path.join(rehearsal,'preparation.json')),review_freeze_sha256:hash(path.join(review,'freeze.json')),review_sha256:hash(path.join(review,'review.json'))}};
  const file=path.join(root,'manifest.json');durableJSON(file,m,{exclusive:true});
  validateManifest(m,'live');verifyReuse(m);
  return {status:m.status,manifest_path:file,manifest_sha256:hash(file),source_commit:m.source_commit,batch_directory:m.batch_directory,live_requests:0};
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  try {
    const [mode,...args]=process.argv.slice(2);let result;
    if(mode==='--freeze'&&args.length===4)result=freezeFinal(...args);
    else if(mode==='--test'&&args.length===2)result=await runFinalChecks(...args);
    else if(mode==='--check'&&args.length===1) {
      const m=boundedJSON(args[0]);validateManifest(m,m.kind==='live-candidate'?'live':'offline');
      result={...verifyReuse(m),authorization:'not-checked-or-granted'};
    } else throw new Error('usage: final-runtime.mjs --test REHEARSAL NEW_CHECKS_DIRECTORY | --freeze REHEARSAL REVIEW CHECKS NEW_DIRECTORY | --check MANIFEST');
    console.log(JSON.stringify(result));
  } catch(error) {console.error(error.message);process.exitCode=1;}
}
