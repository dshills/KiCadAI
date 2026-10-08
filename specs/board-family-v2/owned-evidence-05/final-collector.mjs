// Final collector: one fixed batch, no retry/resume/recovery and no output repair.
// Only an explicit live-candidate manifest plus separate matching release gates
// and recorded user approval can restore a key to the extraction child.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {hash,durableJSON,inventory,authenticateExamples} from '../evaluation/acceptance-lib.mjs';
import {executeRecorded,cleanTerminal,assessAttempt} from './collector.mjs';
import {offlineEnvironment} from './contracts.mjs';
import {boundedJSON as json,pin,validateManifest,verifyReuse,checkAuthority} from './final-runtime.mjs';
export const version='owned-final-collector-1';

function writeNew(file,bytes) {
  const fd=fs.openSync(file,'wx',0o600);
  try {fs.writeFileSync(fd,bytes);fs.fsyncSync(fd);} finally {fs.closeSync(fd);}
  const parent=fs.openSync(path.dirname(file),'r');
  try {fs.fsyncSync(parent);} finally {fs.closeSync(parent);}
}
export async function collect({manifestPath,approvalPath,gatesPath,mode,output}) {
  assert.equal(output,undefined,'batch destination is fixed by the manifest');
  const started=performance.now(),manifestSHA=hash(manifestPath),m=json(manifestPath);
  validateManifest(m,mode);
  if(mode==='offline') {assert.equal(approvalPath,undefined);assert.equal(gatesPath,undefined);}
  const binding=mode==='live'?checkAuthority(m,manifestPath,approvalPath,gatesPath):{approval_sha256:null,release_gates_sha256:null};
  verifyReuse(m); // Contract re-export and retained evidence replay, never a provider call.
  const examples=m.qualification==='reviewed-two-family-examples'?authenticateExamples().receipt.cases:[];
  const root=m.batch_directory;
  // An existing directory, even an incomplete one, consumes this batch identity.
  fs.mkdirSync(root,{mode:0o700});
  const owner=fs.lstatSync(root);
  const own=()=>{const now=fs.lstatSync(root);assert.ok(now.isDirectory()&&now.dev===owner.dev&&now.ino===owner.ino&&(now.mode&0o077)===0,'batch ownership changed');};
  const budget=path.join(root,'budget.json'),ledgerPath=path.join(root,'ledger.json');
  durableJSON(budget,m.policy,{exclusive:true});
  if(mode==='live') {
    writeNew(path.join(root,'approval.json'),fs.readFileSync(approvalPath));
    writeNew(path.join(root,'release-gates.json'),fs.readFileSync(gatesPath));
    pin(path.join(root,'approval.json'),binding.approval_sha256);
    pin(path.join(root,'release-gates.json'),binding.release_gates_sha256);
  }
  durableJSON(path.join(root,'start.json'),{version,evaluation_id:m.evaluation_id,mode,manifest_sha256:manifestSHA,...binding,
    planned_case_ids:m.cases.map(c=>c.id),owner_pid:process.pid,started_utc:new Date().toISOString(),status:'started-not-complete'},{exclusive:true});
  const records=[];let prefix=[],stopped=false;
  const remaining=cap=>{const ms=Math.floor(m.total_timeout_ms-(performance.now()-started));assert.ok(ms>0,'total collection deadline elapsed');return Math.min(cap,ms);};
  const recheck=()=>{
    own();pin(manifestPath,manifestSHA);validateManifest(m,mode);
    if(mode==='live') {
      assert.deepEqual(checkAuthority(m,manifestPath,approvalPath,gatesPath),binding);
      pin(path.join(root,'approval.json'),binding.approval_sha256);pin(path.join(root,'release-gates.json'),binding.release_gates_sha256);
    }
  };
  for(const c of m.cases) {
    recheck();
    if(performance.now()-started>=m.total_timeout_ms) {stopped=true;break;}
    if(prefix.length)assert.deepEqual(json(ledgerPath).entries,prefix,'ledger changed between children');
    else assert.equal(fs.existsSync(ledgerPath),false,'unexpected prior ledger');
    const caseRoot=path.join(root,c.id);fs.mkdirSync(caseRoot,{mode:0o700});
    const prompt=path.join(caseRoot,'prompt.txt');writeNew(prompt,c.prompt);
    const record={id:c.id,prompt_sha256:hash(prompt),state:'attempt-recorded-before-launch',started_utc:new Date().toISOString()};
    durableJSON(path.join(caseRoot,'attempt.json'),record,{exclusive:true});records.push(record);
    try {
      const args=[...m.prefix_args,'--intent-protocol','owned-v4','--evidence-journal',path.join(caseRoot,'journal'),'--prompt-file',prompt,
        '--ledger',ledgerPath,'--live-budget',budget,'--output',path.join(caseRoot,'board'),'--kicad-cli',m.kicad_cli];
      const env=offlineEnvironment();
      if(mode==='live') {
        // Never persist, log or forward the key to audits/native tools.
        assert.ok(process.env.OPENAI_API_KEY?.trim(),'existing OPENAI_API_KEY is required');
        env.OPENAI_API_KEY=process.env.OPENAI_API_KEY;
      }
      record.execution=await executeRecorded(m.binary,args,env,path.join(caseRoot,'command'),remaining(m.case_timeout_ms));
      delete env.OPENAI_API_KEY;
      record.state='terminal-observed';recheck();cleanTerminal(record.execution);
      record.audit_execution=await executeRecorded(m.binary,[...m.prefix_args,'--inspect-owned-journal',path.join(caseRoot,'journal')],
        offlineEnvironment(),path.join(caseRoot,'audit'),remaining(10000));
      cleanTerminal(record.audit_execution);assert.equal(record.audit_execution.exit_code,0,'journal verification failed');recheck();
      const audit=json(path.join(caseRoot,'audit.stdout.log'),1024*1024),ledger=json(ledgerPath,1024*1024);
      Object.assign(record,assessAttempt({execution:record.execution,audit,ledger,policy:m.policy,prefix,prompt:c.prompt,caseRoot,examples}));
      prefix=ledger.entries;record.state='recorded-outcome';
    } catch(error) {
      record.advance=false;record.collection_class='unsafe-to-continue';record.failure=String(error.message).slice(0,2000);stopped=true;
    }
    own();
    record.files_sha256=Object.fromEntries(inventory(caseRoot).map(f=>[f,hash(path.join(caseRoot,f))]));
    durableJSON(path.join(caseRoot,'outcome.json'),record,{exclusive:true});
    if(stopped)break;
  }
  recheck();
  const result={version,evaluation_id:m.evaluation_id,mode,status:stopped?'stopped-no-retry':'collection-complete-semantic-review-pending',
    planned_cases:m.cases.length,prepared_cases:records.length,launched_cases:records.filter(r=>r.execution?.child_pid!=null).length,
    recorded_outcomes:records.filter(r=>r.state==='recorded-outcome').length,recorded_model_failures:records.filter(r=>r.collection_class==='recorded-model-failure').length,
    unattempted_case_ids:m.cases.slice(records.length).map(c=>c.id),total_wall_seconds:(performance.now()-started)/1000,records,acceptance:'not-established-by-collection'};
  durableJSON(path.join(root,'result.json'),result,{exclusive:true});return result;
}
export async function main(args) {
  const [flag,manifestPath,approvalPath,gatesPath,...extra]=args;
  assert.equal(extra.length,0);
  assert.ok((flag==='--offline'&&args.length===2)||(flag==='--live'&&args.length===4),
    'usage: final-collector.mjs --offline TEST_MANIFEST | --live MANIFEST APPROVAL RELEASE_GATES');
  const result=await collect({mode:flag.slice(2),manifestPath,approvalPath,gatesPath});
  console.log(JSON.stringify({status:result.status,mode:result.mode,planned_cases:result.planned_cases,recorded_outcomes:result.recorded_outcomes,
    recorded_model_failures:result.recorded_model_failures,total_wall_seconds:result.total_wall_seconds,acceptance:result.acceptance}));
  if(result.status==='stopped-no-retry')process.exitCode=1;
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url))
  main(process.argv.slice(2)).catch(error=>{console.error(error.message);process.exitCode=1;});
