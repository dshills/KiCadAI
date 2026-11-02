// Real CLI/journal/auditor and collector. Only HTTP is replaced, in a Go test
// binary that cannot qualify for live mode. No approval is created or consumed.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {collect,version,executeRecorded} from './collector.mjs';
import {loadPlan,scoringDependencies,runtimeContract,authenticateBatch,reviewTemplate,scoreReview,assessIndexedRaw} from './scoring.mjs';
import {hash,read,offlineEnvironment,inventory,assessDecision} from '../evaluation/acceptance-lib.mjs';
const dir='specs/board-family-v2/source-reference-candidate-03',planFile=`${dir}/evaluation-plan.json`;
const binary=process.env.KICADAI_INDEXED_TEST_BINARY,cli=process.env.KICADAI_OFFLINE_NATIVE_CLI;
const write=(file,value)=>fs.writeFileSync(file,JSON.stringify(value,null,2)+'\n',{mode:0o600});
function setup(t,native=false) {
  fs.mkdirSync('.cache',{recursive:true});
  const root=fs.mkdtempSync(path.resolve('.cache/indexed-scoring-test-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const {plan,spec}=loadPlan(planFile),contractFile=path.relative(process.cwd(),path.join(root,'contract.json'));
  const m={version,evaluation_id:plan.evaluation_id,policy:plan.proposed_policy,cases:spec.cases.map(({id,prompt})=>({id,prompt})),
    binary:path.resolve(binary),binary_sha256:hash(binary),prefix_args:['-test.run=^TestIndexedCommandProcessHelper$','--'],node_sha256:hash(process.execPath),collector_sha256:hash(`${dir}/collector.mjs`),
    qualification:native?'reviewed-two-family-examples':'non-design-test-fixtures',kicad_cli:native?cli:'/not-used-offline',case_timeout_ms:60000,total_timeout_ms:300000,
    evaluation_plan_sha256:hash(planFile),scoring_contract_path:contractFile,
    ...(native?{qualification_receipt_sha256:hash('specs/board-family-v2/evidence/examples-01.json'),kicad_cli_sha256:hash(cli)}:{})};
  write(contractFile,runtimeContract(m));
  m.runtime_files_sha256=Object.fromEntries([...new Set([...scoringDependencies,planFile,plan.cases_source.path,contractFile,
    'cmd/kicadai-board-family/main.go','cmd/kicadai-board-family/indexed_process_test.go','cmd/kicadai-board-family/indexed_corpus_test.go','internal/boardfamily/intent_reference_audit.go'])].map(f=>[f,hash(f)]));
  const manifestPath=path.join(root,'manifest.json');write(manifestPath,m);
  return {root,manifestPath,output:path.join(root,'batch'),mode:'offline'};
}
function check(o) {return authenticateBatch(planFile,o.manifestPath,o.output);}
function changedRecord(o,index,mutate) {
  const resultFile=path.join(o.output,'result.json'),state=read(resultFile),r=state.records[index],caseRoot=path.join(o.output,r.id);
  mutate(r,caseRoot);
  r.files_sha256=Object.fromEntries(inventory(caseRoot).filter(f=>f!=='outcome.json').map(f=>[f,hash(path.join(caseRoot,f))]));
  write(path.join(caseRoot,'outcome.json'),r);write(resultFile,state);
}
async function tamper(t,o,name,mutate) {
  await t.test(name,()=>{
    const files=inventory(o.output),saved=new Map(files.map(f=>[f,fs.readFileSync(path.join(o.output,f))]));
    try {mutate();assert.throws(()=>check(o));}
    finally {
      for(const f of inventory(o.output))if(!saved.has(f))fs.unlinkSync(path.join(o.output,f));
      for(const [f,bytes] of saved)fs.writeFileSync(path.join(o.output,f),bytes);
    }
  });
}
test('all 14 completed invalid answers are authenticated, retained and scored zero',{skip:!binary},async t=>{
  const o=setup(t),prior=process.env.KICADAI_INDEXED_CORPUS_FAILURE;
  process.env.KICADAI_INDEXED_CORPUS_FAILURE='invalid-extraction';
  try {const state=await collect(o);assert.equal(state.recorded_outcomes,14);assert.equal(state.recorded_model_failures,14);}
  finally {if(prior===undefined)delete process.env.KICADAI_INDEXED_CORPUS_FAILURE;else process.env.KICADAI_INDEXED_CORPUS_FAILURE=prior;}
  const inputs=check(o),review=reviewTemplate(inputs);
  review.status='source-bound-semantic-review';review.reviewer={kind:'implementing-agent',name:'TEST FIXTURE ONLY — not a production review'};review.reviewed_utc=new Date().toISOString();
  for(const c of review.cases) {c.notes='Synthetic invalid extraction, not an AI accuracy observation.';for(const r of c.requirements)r.notes='No valid fact supports this requirement.';c.decision.notes='Invalid extraction cannot pass.';}
  const result=scoreReview({...inputs,review});assert.equal(result.planned_cases,14);assert.equal(result.complete_passes,0);assert.equal(result.raw_passes,0);assert.equal(result.application_passes,0);
  const template=path.join(o.root,'review-template.json'),processStem=path.join(o.root,'template-process');
  const p=await executeRecorded(process.execPath,[`${dir}/scoring.mjs`,'--template',planFile,o.manifestPath,o.output,template],offlineEnvironment(),processStem,30000);
  assert.equal(p.exit_code,0);assert.deepEqual(read(template),reviewTemplate(inputs));
  const q=await executeRecorded(process.execPath,[`${dir}/scoring.mjs`,'--check',planFile,o.manifestPath,o.output,template],offlineEnvironment(),path.join(o.root,'pending-review-process'),30000);
  assert.equal(q.exit_code,1,'pending review must not score as reviewed');
  await tamper(t,o,'coherently rehashed classification is recomputed',()=>changedRecord(o,0,r=>{r.collection_class='recorded-decision-semantic-review-pending';r.model_correct=null;}));
  await tamper(t,o,'coherently rehashed child exit cannot hide model failure',()=>changedRecord(o,0,(r,root)=>{r.execution.exit_code=0;write(path.join(root,'command.process.json'),r.execution);}));
  await tamper(t,o,'invented native comparison cannot be appended',()=>changedRecord(o,0,r=>{r.bundle={files_compared:{'board.kicad_pcb':'invented'}};}));
  await tamper(t,o,'changed audit exit cannot be accepted',()=>changedRecord(o,0,(r,root)=>{r.audit_execution.exit_code=1;write(path.join(root,'audit.process.json'),r.audit_execution);}));
  await tamper(t,o,'final ledger cannot rewrite already settled tokens',()=>{const file=path.join(o.output,'ledger.json'),l=read(file);l.entries[0].input_tokens++;write(file,l);});
  await tamper(t,o,'model-failure counter is independently derived',()=>{const file=path.join(o.output,'result.json'),s=read(file);s.recorded_model_failures=0;write(file,s);});
  await tamper(t,o,'extra batch files cannot be silently omitted',()=>fs.writeFileSync(path.join(o.output,'unrecorded.txt'),'extra'));
  await t.test('repinned invented schema still differs from actual executable',()=>{
    const m=read(o.manifestPath),contract=m.scoring_contract_path,originalContract=fs.readFileSync(contract),originalManifest=fs.readFileSync(o.manifestPath),startFile=path.join(o.output,'start.json'),originalStart=fs.readFileSync(startFile);
    try {const c=read(contract);c.schema={type:'object'};write(contract,c);m.runtime_files_sha256[contract]=hash(contract);write(o.manifestPath,m);const s=read(startFile);s.manifest_sha256=hash(o.manifestPath);write(startFile,s);assert.throws(()=>check(o),/scoring contract differs/);}
    finally {fs.writeFileSync(contract,originalContract);fs.writeFileSync(o.manifestPath,originalManifest);fs.writeFileSync(startFile,originalStart);}
  });
  assert.equal(check(o).state.recorded_outcomes,14);
});
test('all 14 hand-authored responses traverse real command, with all five real native bundles',{skip:!binary||!cli},async t=>{
  const o=setup(t,true),state=await collect(o);
  assert.equal(state.status,'collection-complete-semantic-review-pending');assert.equal(state.recorded_outcomes,14);assert.equal(state.recorded_model_failures,0);
  const inputs=check(o),review=reviewTemplate(inputs);
  for(const c of inputs.spec.cases) {
    assert.equal(assessDecision(inputs.spec,c,inputs.selections[c.id].decision).automatic_checks_pass,true,c.id);
    assert.equal(assessIndexedRaw(c,inputs.selections[c.id],inputs.schema).automatic_checks_pass,true,c.id);
  }
  assert.deepEqual(state.records.slice(0,5).map(r=>Object.keys(r.bundle.files_compared).length),[39,39,39,42,42]);
  assert.equal(review.status,'pending-source-bound-review','integration does not auto-approve semantic accuracy');
  await tamper(t,o,'coherently rehashed fabrication output is compared again to reviewed example',()=>changedRecord(o,0,(r,root)=>{
    const f=Object.keys(r.bundle.all_files_sha256).find(f=>f.startsWith('manufacturing/')&&f.endsWith('.gbr'));
    assert.ok(f);fs.appendFileSync(path.join(root,'board',f),'\nG04 synthetic corruption*\n');
    const manifestFile=path.join(root,'board/manufacturing/manifest.json'),m=read(manifestFile);m.files_sha256[path.basename(f)]=hash(path.join(root,'board',f));write(manifestFile,m);
  }));
  t.diagnostic(`14/14 synthetic outcomes, all 5 configurations with 201 compared deliverables total (39 each BMP280, 42 each SHT31); ${state.total_wall_seconds.toFixed(3)}s collector wall; NO live model accuracy claim`);
});
