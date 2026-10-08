// Versioned owned-v4 rehearsal collector. It cannot make live requests.
// Historical indexed collectors and evidence are never modified or reinterpreted.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {hash,durableJSON,inventory,authenticateFiles,authenticateExamples,compareBundle,exampleID} from '../evaluation/acceptance-lib.mjs';
import {executeRecorded,cleanTerminal,validateLedger} from '../source-reference-candidate-03/collector.mjs';
import {offlineEnvironment,checkContracts,verifyRuntimeContracts} from './contracts.mjs';
export {executeRecorded,cleanTerminal,validateLedger};

export const version='owned-offline-collector-1';
const model='gpt-4.1-mini-2025-04-14';
const self=fileURLToPath(import.meta.url);
export const collectorDependencies=['./collector.mjs','./contracts.mjs','./authenticate.mjs','../source-reference-candidate-03/collector.mjs','../source-reference-candidate-03/runtime-qualification.mjs','../evaluation/acceptance-lib.mjs','../development/replay-normalization.mjs'].map(f=>fileURLToPath(new URL(f,import.meta.url)));
const journalFiles=['request/body.bin','request/receipt.json','response.bin','response/receipt.json','selection/ledger.json','selection/receipt.json','selection/selection.json','start.json'];

function boundedRead(file,limit=4*1024*1024) {
  const info=fs.lstatSync(file);
  assert.ok(info.isFile() && info.size<=limit,'expected bounded regular file');
  return fs.readFileSync(file);
}
function json(file,limit) { return JSON.parse(boundedRead(file,limit).toString('utf8')); }
function pin(file,sha) { assert.match(sha,/^[a-f0-9]{64}$/); assert.equal(hash(file),sha,'frozen runtime changed'); }
function writeNew(file,bytes) {
  const fd=fs.openSync(file,'wx',0o600);
  try { fs.writeFileSync(fd,bytes); fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
  const parent=fs.openSync(path.dirname(file),'r');
  try { fs.fsyncSync(parent); } finally { fs.closeSync(parent); }
}

// This does not score language accuracy. A wrong but format-valid decision is
// retained for semantic review; a completed invalid extraction remains failure.
export function assessAttempt({execution,audit,ledger,policy,prefix,prompt,caseRoot,examples}) {
  cleanTerminal(execution);
  assert.equal(audit.version,'owned-journal-audit-1');
  assert.deepEqual(audit.policy,policy);
  const entries=validateLedger(ledger,policy,prefix);
  assert.equal(entries.length,prefix.length+1);
  assert.deepEqual(audit.ledger,ledger);
  const s=audit.selection,e=entries.at(-1);
  assert.equal(s.original_request,prompt); assert.equal(s.admission_version,'4-owned-evidence-experimental');
  assert.equal(s.ledger_index,e.index); assert.equal(s.response_id,e.response_id); assert.equal(s.model,model);
  assert.equal(s.usage.input_tokens,e.input_tokens??0); assert.equal(s.usage.output_tokens,e.output_tokens??0);
  assert.equal(s.usage.total_tokens,(e.input_tokens??0)+(e.output_tokens??0));
  const journal=path.join(caseRoot,'journal'),output=path.join(caseRoot,'board');
  assert.deepEqual(Object.keys(audit.files_sha256).sort(),journalFiles);
  assert.deepEqual(inventory(journal),journalFiles);
  authenticateFiles(journal,audit.files_sha256);
  const summary=json(path.join(caseRoot,'command.stdout.log'),1024*1024);
  const sameSelection=()=>{
    assert.deepEqual(inventory(output),['selection.json']);
    assert.equal(hash(path.join(output,'selection.json')),audit.files_sha256['selection/selection.json']);
    assert.equal(s.decision.configuration,null);
  };
  if(['invalid_extraction','provider_refusal'].includes(audit.outcome)) {
    assert.equal(execution.exit_code,1); assert.equal(summary.passed,false); assert.equal(summary.disposition,'failed');
    sameSelection();
    return {collection_class:'recorded-model-failure',advance:true,model_correct:false,response_id:e.response_id,ledger_index:e.index};
  }
  assert.equal(audit.outcome,'decision'); assert.equal(execution.exit_code,0,'native/execution failure stops collection');
  const disposition=s.decision.disposition;
  assert.ok(['supported','clarify','unsupported'].includes(disposition));
  let bundle;
  if(disposition==='supported') {
    assert.equal(summary.passed,true); assert.ok(s.decision.configuration);
    assert.equal(hash(path.join(output,'selection.json')),audit.files_sha256['selection/selection.json']);
    const example=examples.find(x=>x.id===exampleID(s.decision.configuration));
    assert.ok(example,'supported result has no reviewed example');
    authenticateFiles(example.destination,example.files_sha256);
    bundle=compareBundle(output,example);
  } else {
    assert.equal(summary.passed,false); assert.equal(summary.disposition,disposition); sameSelection();
  }
  return {collection_class:'recorded-decision-semantic-review-pending',advance:true,model_correct:null,response_id:e.response_id,ledger_index:e.index,...(bundle?{bundle}: {})};
}

function validateManifest(m,mode) {
  assert.equal(mode,'offline','owned-v4 live collection is not enabled by this rehearsal package');
  assert.equal(m.status,'offline-rehearsal-only');
  assert.equal(m.version,version); assert.match(m.evaluation_id,/^[a-z0-9][a-z0-9-]{2,79}$/);
  assert.equal(m.policy.goal,m.evaluation_id);
  assert.ok(Array.isArray(m.cases) && m.cases.length>0 && m.cases.length<=20);
  assert.equal(m.policy.max_requests,m.cases.length);
  assert.ok(Number.isSafeInteger(m.policy.max_micro_usd) && m.policy.max_micro_usd>=m.cases.length*50_000 && m.policy.max_micro_usd<=1_000_000);
  assert.equal(new Set(m.cases.map(c=>c.id)).size,m.cases.length);
  for(const c of m.cases) { assert.match(c.id,/^[a-zA-Z0-9][a-zA-Z0-9-]{0,39}$/); assert.ok(typeof c.prompt==='string' && Buffer.byteLength(c.prompt)>0 && Buffer.byteLength(c.prompt)<=2000); }
  assert.ok(path.isAbsolute(m.binary)); pin(m.binary,m.binary_sha256);
  pin(process.execPath,m.node_sha256); pin(self,m.collector_sha256);
  assert.ok(Number.isSafeInteger(m.case_timeout_ms) && m.case_timeout_ms>=100 && m.case_timeout_ms<=600_000);
  assert.ok(Number.isSafeInteger(m.total_timeout_ms) && m.total_timeout_ms>=m.case_timeout_ms && m.total_timeout_ms<=1_200_000);
  assert.deepEqual(m.prefix_args,['-test.run=^TestOwnedProcessHelper$','--'],'offline collection requires the test-only in-memory transport');
  assert.ok(path.isAbsolute(m.kicad_cli));
  assert.ok(['reviewed-two-family-examples','non-design-test-fixtures'].includes(m.qualification));
  if(m.qualification==='reviewed-two-family-examples') { pin('specs/board-family-v2/evidence/examples-01.json',m.qualification_receipt_sha256); pin(m.kicad_cli,m.kicad_cli_sha256); }
  assert.ok(m.runtime_files_sha256 && Object.keys(m.runtime_files_sha256).length>0);
  for(const file of collectorDependencies) assert.equal(m.runtime_files_sha256[path.relative(process.cwd(),file)],hash(file),'collector dependency missing from freeze');
  authenticateFiles('.',m.runtime_files_sha256);
  checkContracts(m);
}

export function checkManifest(manifestPath,mode='offline') {
  const m=json(manifestPath,4*1024*1024); validateManifest(m,mode); return m;
}

// One fresh batch; no automatic resume or reattempt. A partial root is preserved
// on every error and blocks a repeated invocation. All provider keys and test
// override variables are stripped. The only transport is the Go test fixture.
export async function collect({manifestPath,approvalPath,output,mode='offline'}) {
  const started=performance.now(),manifestSHA=hash(manifestPath),m=json(manifestPath,4*1024*1024);
  validateManifest(m,mode);
  const approvalSHA=null,env=offlineEnvironment();
  assert.equal(approvalPath,undefined,'offline rehearsal does not consume any approval');
  verifyRuntimeContracts(m);
  const examples=m.qualification==='reviewed-two-family-examples'?authenticateExamples().receipt.cases:[];
  const absolute=path.resolve(output),parent=fs.realpathSync(path.dirname(absolute));
  const root=path.join(parent,path.basename(absolute));
  fs.mkdirSync(root,{mode:0o700}); // Exclusive: never remove/reuse an old batch.
  const owner=fs.lstatSync(root);
  const own=()=>{const now=fs.lstatSync(root);assert.ok(now.isDirectory() && now.dev===owner.dev && now.ino===owner.ino && (now.mode&0o077)===0,'batch ownership changed');};
  const budget=path.join(root,'budget.json'),ledgerPath=path.join(root,'ledger.json');
  durableJSON(budget,m.policy,{exclusive:true});
  durableJSON(path.join(root,'start.json'),{version,evaluation_id:m.evaluation_id,mode,manifest_sha256:manifestSHA,approval_sha256:approvalSHA,planned_case_ids:m.cases.map(c=>c.id),owner_pid:process.pid,started_utc:new Date().toISOString(),status:'started-not-complete'},{exclusive:true});
  const records=[]; let prefix=[],stopped=false;
  const remaining=cap=>{const ms=Math.floor(m.total_timeout_ms-(performance.now()-started));assert.ok(ms>0,'total collection deadline elapsed');return Math.min(cap,ms);};
  const recheck=()=>{own();pin(manifestPath,manifestSHA);validateManifest(m,mode);if(approvalSHA)pin(approvalPath,approvalSHA);};
  for(const c of m.cases) {
    recheck();
    if(performance.now()-started>=m.total_timeout_ms) {stopped=true;break;}
    if(prefix.length) assert.deepEqual(json(ledgerPath).entries,prefix,'ledger changed between children');
    else assert.equal(fs.existsSync(ledgerPath),false,'unexpected prior ledger');
    const caseRoot=path.join(root,c.id); fs.mkdirSync(caseRoot,{mode:0o700});
    const prompt=path.join(caseRoot,'prompt.txt'); writeNew(prompt,c.prompt);
    const record={id:c.id,prompt_sha256:hash(prompt),state:'attempt-recorded-before-launch',started_utc:new Date().toISOString()};
    durableJSON(path.join(caseRoot,'attempt.json'),record,{exclusive:true});
    records.push(record);
    try {
      const args=[...m.prefix_args,'--intent-protocol','owned-v4','--evidence-journal',path.join(caseRoot,'journal'),'--prompt-file',prompt,'--ledger',ledgerPath,'--live-budget',budget,'--output',path.join(caseRoot,'board'),'--kicad-cli',m.kicad_cli];
      record.execution=await executeRecorded(m.binary,args,env,path.join(caseRoot,'command'),remaining(m.case_timeout_ms));
      record.state='terminal-observed';
      recheck(); cleanTerminal(record.execution);
      record.audit_execution=await executeRecorded(m.binary,[...m.prefix_args,'--inspect-owned-journal',path.join(caseRoot,'journal')],offlineEnvironment(),path.join(caseRoot,'audit'),remaining(10000));
      cleanTerminal(record.audit_execution);assert.equal(record.audit_execution.exit_code,0,'journal verification failed');
      recheck();
      const audit=json(path.join(caseRoot,'audit.stdout.log'),1024*1024),ledger=json(ledgerPath,1024*1024);
      Object.assign(record,assessAttempt({execution:record.execution,audit,ledger,policy:m.policy,prefix,prompt:c.prompt,caseRoot,examples}));
      prefix=ledger.entries;
      record.state='recorded-outcome';
    } catch(error) {
      record.advance=false;record.collection_class='unsafe-to-continue';record.failure=String(error.message).slice(0,2000);stopped=true;
    }
    own();
    // Preserve all generated/partial files. Nothing here repairs or deletes them.
    record.files_sha256=Object.fromEntries(inventory(caseRoot).map(f=>[f,hash(path.join(caseRoot,f))]));
    durableJSON(path.join(caseRoot,'outcome.json'),record,{exclusive:true});
    if(stopped) break;
  }
  recheck();
  const result={version,evaluation_id:m.evaluation_id,mode,status:stopped?'stopped-no-retry':'collection-complete-semantic-review-pending',planned_cases:m.cases.length,prepared_cases:records.length,launched_cases:records.filter(r=>r.execution?.child_pid!=null).length,recorded_outcomes:records.filter(r=>r.state==='recorded-outcome').length,recorded_model_failures:records.filter(r=>r.collection_class==='recorded-model-failure').length,unattempted_case_ids:m.cases.slice(records.length).map(c=>c.id),total_wall_seconds:(performance.now()-started)/1000,records,acceptance:'not-established-by-collection'};
  durableJSON(path.join(root,'result.json'),result,{exclusive:true});
  return result;
}

export async function main(args) {
  const started=performance.now();
  if(args[0]==='--check' && args.length===2) {
    const m=checkManifest(args[1]); verifyRuntimeContracts(m);
    console.log(JSON.stringify({status:'offline-manifest-preflight-pass',live_authorization:'not-checked-or-granted'}));
    return;
  }
  assert.ok(args[0]==='--offline' && args.length===3,'usage: collector.mjs --check MANIFEST | --offline MANIFEST NEW_DIRECTORY; live execution is unavailable');
  const mode=args[0].slice(2),result=await collect({manifestPath:args[1],output:args.at(-1),mode});
  console.log(JSON.stringify({status:result.status,mode,planned_cases:result.planned_cases,recorded_outcomes:result.recorded_outcomes,recorded_model_failures:result.recorded_model_failures,total_wall_seconds:(performance.now()-started)/1000,acceptance:result.acceptance}));
  if(result.status==='stopped-no-retry') process.exitCode=1;
}
if(process.argv[1] && path.resolve(process.argv[1])===self) main(process.argv.slice(2)).catch(error=>{console.error(error.message);process.exitCode=1;});
