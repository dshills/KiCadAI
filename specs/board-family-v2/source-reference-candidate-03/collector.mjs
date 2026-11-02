// Importing this module never starts a process or reads a credential. Live use
// requires a separately frozen manifest and a matching actual user approval.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {hash,durableJSON,offlineEnvironment,inventory,authenticateFiles,authenticateExamples,compareBundle,exampleID} from '../evaluation/acceptance-lib.mjs';
import {verifyQualification} from './runtime-qualification.mjs';

export const version='indexed-collector-1';
const model='gpt-4.1-mini-2025-04-14';
const self=fileURLToPath(import.meta.url);
const dependencies=[self,fileURLToPath(new URL('./runtime-qualification.mjs',import.meta.url)),fileURLToPath(new URL('../evaluation/acceptance-lib.mjs',import.meta.url)),fileURLToPath(new URL('../development/replay-normalization.mjs',import.meta.url))];
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

// The promise resolves only on close, not exit, so redirected pipes have closed.
// A deadline or log overflow kills this owned process group and then still waits
// for its actual terminal event. No PID/file inference, retry or resume exists.
export async function executeRecorded(command,args,env,stem,timeoutMS) {
  assert.notEqual(process.platform,'win32','collector requires POSIX process-group termination');
  assert.ok(Number.isSafeInteger(timeoutMS) && timeoutMS>0 && timeoutMS<=600_000);
  const fds=[];
  try {
    for(const stream of ['stdout','stderr']) fds.push(fs.openSync(`${stem}.${stream}.log`,'wx',0o600));
  } catch(error) { for(const fd of fds) fs.closeSync(fd); throw error; }
  const start=performance.now();
  let result;
  const errors=[];
  try {
    result=await new Promise((resolve,reject)=>{
      let child;
      try { child=spawn(command,args,{env,stdio:['ignore','pipe','pipe'],detached:true}); }
      catch(error) { reject(error); return; }
      let spawnError=null,timedOut=false,logOverflow=false,storageError=false;
      const sizes=[0,0];
      const kill=()=>{ if(child.pid) { try { process.kill(-child.pid,'SIGKILL'); } catch(error) { if(error.code!=='ESRCH') spawnError=error.code??'kill-failed'; } } };
      const timer=setTimeout(()=>{timedOut=true;kill();},timeoutMS);
      for(const [i,stream] of [child.stdout,child.stderr].entries()) stream.on('data',chunk=>{
        const keep=chunk.subarray(0,Math.max(0,1024*1024-sizes[i]));
        sizes[i]+=keep.length;
        try { if(keep.length) fs.writeFileSync(fds[i],keep); }
        catch { storageError=true;kill(); }
        if(keep.length<chunk.length) {logOverflow=true;kill();}
      });
      child.on('error',error=>{spawnError=error.code??'spawn-failed';});
      child.on('close',(code,signal)=>{
        clearTimeout(timer);
        resolve({exit_code:code,signal,child_pid:child.pid??null,child_terminal_observed:true,timed_out:timedOut,spawn_error:spawnError,log_overflow:logOverflow,storage_error:storageError,wall_seconds:(performance.now()-start)/1000});
      });
    });
  } catch(error) { errors.push(error); }
  for(const fd of fds) {
    try {fs.fsyncSync(fd);} catch(error){errors.push(error);}
    try {fs.closeSync(fd);} catch(error){errors.push(error);}
  }
  if(errors.length || result?.storage_error) throw new AggregateError(errors,'process log persistence failed; no continuation');
  durableJSON(`${stem}.process.json`,result,{exclusive:true});
  return result;
}

export function cleanTerminal(p) {
  assert.equal(p.child_terminal_observed,true);
  assert.equal(p.signal,null); assert.equal(p.timed_out,false); assert.equal(p.spawn_error,null);
  assert.equal(p.log_overflow,false); assert.equal(p.storage_error,false);
  assert.ok(Number.isSafeInteger(p.exit_code) && [0,1].includes(p.exit_code));
}

export function validateLedger(l,policy,prefix=[]) {
  assert.equal(l.version,2); assert.equal(l.goal,policy.goal);
  assert.equal(l.max_requests,policy.max_requests); assert.equal(l.max_micro_usd,policy.max_micro_usd);
  assert.ok(!l.halt_reason); assert.ok(Array.isArray(l.entries));
  assert.ok(l.entries.length<=policy.max_requests && l.entries.length*50_000<=policy.max_micro_usd);
  assert.deepEqual(l.entries.slice(0,prefix.length),prefix,'previous attempts changed');
  const ids=new Set();
  for(const [i,e] of l.entries.entries()) {
    assert.equal(e.index,i+1); assert.equal(e.model,model); assert.equal(e.status,'completed');
    assert.equal(e.reserve_micro_usd,50_000);
    assert.ok(typeof e.response_id==='string' && e.response_id.length>0 && !ids.has(e.response_id)); ids.add(e.response_id);
    assert.ok(Number.isSafeInteger(e.input_tokens??0) && (e.input_tokens??0)>=0 && (e.input_tokens??0)<=1_000_000);
    assert.ok(Number.isSafeInteger(e.output_tokens??0) && (e.output_tokens??0)>=0 && (e.output_tokens??0)<=1600);
    assert.equal(e.estimated_micro_usd,Math.ceil(((e.input_tokens??0)*4+(e.output_tokens??0)*16)/10));
    assert.ok(e.estimated_micro_usd<=50_000);
  }
  return l.entries;
}

// This does not score language accuracy. A wrong but format-valid decision is
// retained for semantic review; a completed invalid extraction remains failure.
export function assessAttempt({execution,audit,ledger,policy,prefix,prompt,caseRoot,examples}) {
  cleanTerminal(execution);
  assert.equal(audit.version,'indexed-journal-audit-1');
  assert.deepEqual(audit.policy,policy);
  const entries=validateLedger(ledger,policy,prefix);
  assert.equal(entries.length,prefix.length+1);
  assert.deepEqual(audit.ledger,ledger);
  const s=audit.selection,e=entries.at(-1);
  assert.equal(s.original_request,prompt); assert.equal(s.admission_version,'3-indexed-quantities-experimental');
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
  assert.ok(Array.isArray(m.prefix_args) && m.prefix_args.every(x=>typeof x==='string'));
  assert.ok(path.isAbsolute(m.kicad_cli));
  if(mode==='live') { assert.deepEqual(m.prefix_args,[]); assert.equal(m.qualification,'reviewed-two-family-examples'); }
  else { assert.equal(mode,'offline'); assert.ok(['reviewed-two-family-examples','non-design-test-fixtures'].includes(m.qualification)); }
  if(m.qualification==='reviewed-two-family-examples') { pin('specs/board-family-v2/evidence/examples-01.json',m.qualification_receipt_sha256); pin(m.kicad_cli,m.kicad_cli_sha256); }
  assert.ok(m.runtime_files_sha256 && Object.keys(m.runtime_files_sha256).length>0);
  for(const file of dependencies) assert.equal(m.runtime_files_sha256[path.relative(process.cwd(),file)],hash(file),'collector dependency missing from freeze');
  authenticateFiles('.',m.runtime_files_sha256);
  if(mode==='live'||m.production_qualification)verifyQualification(m);
}

export function checkManifest(manifestPath,mode='offline') {
  const m=json(manifestPath,1024*1024); validateManifest(m,mode); return m;
}

export function checkLiveApproval(a,m,manifestSHA) {
  assert.equal(a.status,'explicit-user-approved'); assert.equal(a.evaluation_id,m.evaluation_id);
  assert.equal(a.manifest_sha256,manifestSHA); assert.equal(a.max_physical_requests,m.policy.max_requests); assert.equal(a.max_micro_usd,m.policy.max_micro_usd);
  assert.equal(a.existing_key_only,true); assert.ok(typeof a.user_message==='string' && a.user_message.trim());
  assert.ok(typeof a.recorded_utc==='string' && Number.isFinite(Date.parse(a.recorded_utc)));
}

// One fresh batch; no automatic resume or reattempt. A partial root is preserved
// on every error and blocks a repeated invocation. mode=offline strips all keys.
export async function collect({manifestPath,approvalPath,output,mode='offline'}) {
  const started=performance.now(),manifestSHA=hash(manifestPath),m=json(manifestPath,1024*1024);
  validateManifest(m,mode);
  let approvalSHA=null;
  const env=offlineEnvironment();
  if(mode==='live') {
    const a=json(approvalPath,65536); approvalSHA=hash(approvalPath);
    checkLiveApproval(a,m,manifestSHA);
    assert.ok(process.env.OPENAI_API_KEY,'approved existing key unavailable'); env.OPENAI_API_KEY=process.env.OPENAI_API_KEY;
  } else assert.equal(approvalPath,undefined,'offline mode does not consume approval');
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
      const args=[...m.prefix_args,'--intent-protocol','indexed-v3','--evidence-journal',path.join(caseRoot,'journal'),'--prompt-file',prompt,'--ledger',ledgerPath,'--live-budget',budget,'--output',path.join(caseRoot,'board'),'--kicad-cli',m.kicad_cli];
      record.execution=await executeRecorded(m.binary,args,env,path.join(caseRoot,'command'),remaining(m.case_timeout_ms));
      record.state='terminal-observed';
      recheck(); cleanTerminal(record.execution);
      record.audit_execution=await executeRecorded(m.binary,[...m.prefix_args,'--inspect-indexed-journal',path.join(caseRoot,'journal')],offlineEnvironment(),path.join(caseRoot,'audit'),remaining(10000));
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
    validateManifest(json(args[1],1024*1024),'offline');
    console.log(JSON.stringify({status:'offline-manifest-preflight-pass',live_authorization:'not-checked-or-granted'}));
    return;
  }
  assert.ok((args[0]==='--offline' && args.length===3)||(args[0]==='--live' && args.length===4),'usage: collector.mjs --check MANIFEST | --offline MANIFEST NEW_DIRECTORY | --live MANIFEST APPROVAL NEW_DIRECTORY');
  const mode=args[0].slice(2),result=await collect({manifestPath:args[1],approvalPath:mode==='live'?args[2]:undefined,output:args.at(-1),mode});
  console.log(JSON.stringify({status:result.status,mode,planned_cases:result.planned_cases,recorded_outcomes:result.recorded_outcomes,recorded_model_failures:result.recorded_model_failures,total_wall_seconds:(performance.now()-started)/1000,acceptance:result.acceptance}));
  if(result.status==='stopped-no-retry') process.exitCode=1;
}
if(process.argv[1] && path.resolve(process.argv[1])===self) main(process.argv.slice(2)).catch(error=>{console.error(error.message);process.exitCode=1;});
