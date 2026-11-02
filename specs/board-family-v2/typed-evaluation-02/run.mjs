// One bounded successor batch. Importing this module never invokes a provider.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
import {hash,read,durableJSON,offlineEnvironment,inventory,authenticateFiles,expectedConfig,exampleID,authenticateNativeBundle,compareBundle} from '../evaluation/acceptance-lib.mjs';
import {executeCase,processIsAlive} from '../evaluation/run-language.mjs';
import {E,evaluationID,policy,model,batch,ledgerFile,runtimeFile,freezeFile,approvalFile,cli,checkLedger,checkApproval,verifyResume,assessSelection,metrics} from './acceptance.mjs';
import {authenticateRuntime} from './runtime.mjs';

export async function main(args) {
  assert.ok(args.length===1 && ['--check','--live','--resume'].includes(args[0]),'usage: run.mjs --check | --live | --resume');
  const {spec,contract,runtime,receipt}=authenticateRuntime();
  if(args[0]==='--check') {
    console.log(JSON.stringify({status:'offline-preflight-pass',evaluation_id:evaluationID,cases:14,live_authorization:'not-checked-or-granted'}));
    return;
  }
  checkApproval(read(approvalFile),hash(runtimeFile),hash(freezeFile),args[0]==='--resume');
  assert.ok(process.env.OPENAI_API_KEY,'approved existing key unavailable');
  const env={...offlineEnvironment(),OPENAI_API_KEY:process.env.OPENAI_API_KEY};
  const bindings={approval_sha256:hash(approvalFile),runtime_sha256:hash(runtimeFile),freeze_sha256:hash(freezeFile)};
  const stateFile=`${batch}/state.json`, lock=`${batch}.lock`;
  // A leftover lock is evidence, never permission to restart or delete it.
  fs.mkdirSync(lock,{mode:0o700});
  try {
    durableJSON(`${lock}/owner.json`,{pid:process.pid,evaluation_id:evaluationID,started_utc:new Date().toISOString()},{exclusive:true});
    let state,pending;
    if(args[0]==='--live') {
      assert.equal(fs.existsSync(batch),false,'batch already exists');
      assert.equal(fs.existsSync(ledgerFile),false,'ledger already exists; do not reset or relabel');
      fs.mkdirSync(batch,{mode:0o700});
      state={evaluation_id:evaluationID,bindings,planned_case_ids:spec.cases.map(c=>c.id),owner_pid:process.pid,status:'running',started_utc:new Date().toISOString(),records:[],ledger_entries:[]};
      pending=spec.cases;
      durableJSON(stateFile,state,{exclusive:true});
    } else {
      state=read(stateFile);
      assert.deepEqual(state.bindings,bindings,'approval/runtime/freeze changed');
      const entries=fs.existsSync(ledgerFile)?checkLedger(read(ledgerFile)):[];
      pending=verifyResume(state,spec,entries,processIsAlive);
      for(const record of state.records) authenticateFiles(batch,record.files_sha256);
      state.owner_pid=process.pid; state.status='running'; delete state.stop_reason;
      durableJSON(stateFile,state);
    }
    for(const c of pending) {
      authenticateRuntime(); // Recheck source, tooling, examples and old ledgers.
      assert.equal(hash(approvalFile),bindings.approval_sha256);
      const before=fs.existsSync(ledgerFile)?checkLedger(read(ledgerFile),state.ledger_entries):[];
      assert.deepEqual(before,state.ledger_entries);
      assert.ok(before.length<policy.max_requests);
      const prompt=`${batch}/${c.id}.txt`, directory=`${batch}/${c.id}`;
      fs.writeFileSync(prompt,c.prompt,{flag:'wx',mode:0o600});
      const record={id:c.id,state:'attempt-recorded-before-launch',expected_disposition:c.expected_disposition,prompt_sha256:hash(prompt),started_utc:new Date().toISOString()};
      const execution=await executeCase(path.resolve(runtime.binary),['--prompt-file',prompt,'--ledger',ledgerFile,'--live-budget',`${E}/budget-02.json`,'--output',directory,'--kicad-cli',cli],env,`${batch}/${c.id}`,()=>{
        state.records.push(record); durableJSON(stateFile,state);
      });
      Object.assign(record,execution,{state:'finished',output_checks_pass:false});
      try {
        const ledger=fs.existsSync(ledgerFile)?read(ledgerFile):null;
        const entries=ledger?checkLedger(ledger,before):[];
        state.ledger_entries=entries;
        if(ledger) durableJSON(`${batch}/${c.id}.ledger.json`,ledger,{exclusive:true});
        assert.equal(execution.exit_code,0,'execution failed; never retry this case');
        assert.equal(execution.signal,null); assert.equal(execution.timed_out,false); assert.equal(execution.spawn_error,null);
        assert.equal(entries.length,before.length+1,'expected one physical reservation');
        const entry=entries.at(-1),selection=read(`${directory}/selection.json`);
        assert.equal(entry.status,'completed'); assert.equal(selection.model,model);
        assert.equal(selection.ledger_index,entry.index); assert.equal(selection.response_id,entry.response_id);
        assert.equal(selection.usage?.input_tokens,entry.input_tokens); assert.equal(selection.usage?.output_tokens,entry.output_tokens);
        assert.equal(selection.usage?.total_tokens,entry.input_tokens+entry.output_tokens);
        record.response_id=selection.response_id; record.ledger_index=entry.index;
        Object.assign(record,assessSelection(spec,c,selection,contract.schema));
        const stdout=read(`${batch}/${c.id}.stdout.log`);
        if(selection.decision?.disposition==='supported') {
          assert.equal(stdout.passed,true,'native validation failed');
          authenticateNativeBundle(directory);
          const cfg=selection.decision.configuration,example=receipt.cases.find(e=>e.id===exampleID(cfg));
          assert.ok(example,'unreviewed family/profile');
          record.output_checks_pass=c.expected_disposition==='supported' && record.admitted.configuration_matches;
          if(record.output_checks_pass) {
            assert.deepEqual(read(`${directory}/configuration.json`),expectedConfig(spec,c));
            record.bundle=compareBundle(directory,example);
          } else record.bundle_comparison='wrong-acceptance-output-preserved-as-failed-evidence';
        } else {
          assert.ok(['clarify','unsupported'].includes(selection.decision?.disposition));
          assert.deepEqual(inventory(directory),['selection.json'],'non-design generated native artifacts');
          assert.equal(selection.decision.configuration,null);
          record.output_checks_pass=c.expected_disposition!=='supported';
        }
      } catch(error) {
        record.output_checks_pass=false;
        record.execution_or_evidence_failure=String(error.message).slice(0,2000);
        state.status='stopped'; state.stop_reason='transport/accounting/protocol/native evidence failure; no retry';
      }
      const files=[`${c.id}.txt`,`${c.id}.stdout.log`,`${c.id}.stderr.log`];
      if(fs.existsSync(`${batch}/${c.id}.ledger.json`)) files.push(`${c.id}.ledger.json`);
      if(fs.existsSync(directory)) files.push(...inventory(directory).map(f=>`${c.id}/${f}`));
      record.files_sha256=Object.fromEntries(files.map(f=>[f,hash(`${batch}/${f}`)]));
      state.metrics=metrics(spec,state.records); state.ledger_sha256=fs.existsSync(ledgerFile)?hash(ledgerFile):null;
      durableJSON(stateFile,state);
      console.log(JSON.stringify({id:c.id,raw_automatic_pass:record.raw?.automatic_checks_pass??false,application_automatic_pass:Boolean(record.admitted?.automatic_checks_pass&&record.output_checks_pass),semantic_review:'pending',seconds:record.wall_seconds,stopped:state.status==='stopped'}));
      if(state.status==='stopped') break;
    }
    if(state.status==='running') state.status='collection-complete-semantic-review-pending';
    state.finished_utc=new Date().toISOString(); state.metrics=metrics(spec,state.records);
    authenticateRuntime(); durableJSON(stateFile,state);
    console.log(JSON.stringify({status:state.status,...state.metrics}));
    if(state.status==='stopped') process.exitCode=1;
  } finally {
    fs.unlinkSync(`${lock}/owner.json`); fs.rmdirSync(lock);
  }
}
if(process.argv[1] && import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href) {
  main(process.argv.slice(2)).catch(error=>{console.error(error.message);process.exitCode=1;});
}
