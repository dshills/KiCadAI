// Approved six-case follow-up only. No retry or replacement ledger.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';

assert(process.argv.length===4&&process.argv[2]==='--live','usage AFTER APPROVAL: node run-guardrails.mjs --live NEW_OUTPUT');
const root=process.cwd(),out=path.resolve(process.argv[3]),base='specs/board-family-v1';
const ledger=path.resolve('.cache/board-family-v1/live-ledger.json'),binary=path.resolve('.cache/board-family-v1/kicadai-board-family');
const specFile=base+'/evaluation/guardrail-followup-proposed.json',contract=base+'/evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json';
const read=p=>JSON.parse(fs.readFileSync(p)),digest=b=>crypto.createHash('sha256').update(b).digest('hex'),hash=p=>digest(fs.readFileSync(p));
assert.equal(hash(specFile),'ccd2806424b811d24f13b7e2cb91b4ed92c8673d691096ca3dce64ba338a208a');
assert.equal(hash(contract),'76cf6b0a3965c18b81504ac67a1e5dbf3b9eec86b10e4a8ba8581da2e7229a9c');
assert.equal(hash(ledger),'3a90e509d6a6ee37fc0a657f51d2f086004cb62ce9807632d08c02f0cc18b3d6','must start at the retained 35-call ledger; never replay a partially executed run');
const before=read(ledger),spec=read(specFile);assert.equal(before.entries.length,35);assert(!before.halt_reason);
assert.equal(spec.cases.length,6);assert.equal(spec.cases.filter(c=>c.expected_disposition==='unsupported').length,4);assert.equal(spec.cases.filter(c=>c.expected_disposition==='clarify').length,2);
assert(!fs.existsSync(out));assert(process.env.OPENAI_API_KEY,'approved existing key is unavailable');
const env={...process.env};for(const k of ['ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const offlineEnv={...env};delete offlineEnv.OPENAI_API_KEY;
const git=(...args)=>{const r=spawnSync('git',args,{env:offlineEnv,encoding:'utf8'});assert.equal(r.status,0,r.stderr);return r.stdout};
const productionBase='faca271b6198d399e3e2e791f40f35df2ac57232';
const replacements={
 'internal/boardfamily/ledger.go':['const MaxLiveRequests = 35','const MaxLiveRequests = 41'],
 'cmd/kicadai-board-family/main.go':["goal's 35-request / $10 limit","goal's 41-request / $10 limit"]
};
assert.equal(git('status','--porcelain','--','cmd','internal'),'','production source must be committed');
assert.deepEqual(git('diff','--name-only',productionBase,'--','cmd','internal').trim().split('\n').sort(),Object.keys(replacements).sort(),'decision/generation/validation code must remain frozen');
const lineage={};
for(const [f,[from,to]] of Object.entries(replacements)){
 const old=git('show',productionBase+':'+f),current=fs.readFileSync(f,'utf8');assert(old.includes(from));assert.equal(current,old.replace(from,to));
 lineage[f]={before_sha256:digest(old),after_sha256:hash(f),replacement:[from,to]};
}
fs.mkdirSync(out,{recursive:true});
const save=(name,data)=>fs.writeFileSync(path.join(out,name),JSON.stringify(data,null,2)+'\n');
fs.copyFileSync(ledger,path.join(out,'ledger-before.json'),fs.constants.COPYFILE_EXCL);
const exported=path.join(out,'executed-contract.json'),e=spawnSync(binary,['--export-live-contract',exported],{env:offlineEnv,encoding:'utf8',timeout:10000});
assert.equal(e.status,0,e.stderr);assert.equal(hash(exported),hash(contract),'binary request contract changed; no call sent');
const summary={started_utc:new Date().toISOString(),run_kind:'targeted_guardrail_followup',source_commit:git('rev-parse','HEAD').trim(),production_base_commit:productionBase,cap_only_lineage:lineage,binary_sha256:hash(binary),runner_sha256:hash(new URL(import.meta.url)),contract_sha256:hash(contract),spec_sha256:hash(specFile),ledger_path:ledger,prior_request_count:35,authorized_request_ceiling:41,authorized_usd_ceiling:10,planned_case_ids:spec.cases.map(c=>c.id),policy:spec.policy,independence:'Six literal prompts declared and approved before execution; unseen by the selector, authored by the implementing agent, not an independent benchmark. Only the literal prompt and unchanged contract/schema are sent. Historical live scores remain unchanged.',cases:[],passed:false};
save('summary.json',summary);
for(const c of spec.cases){
 const prior=read(ledger);assert.deepEqual(prior.entries.slice(0,35),before.entries);assert.equal(prior.entries.length,35+summary.cases.length);assert(!prior.halt_reason);
 const dir=path.join(out,c.id),prompt=path.join(out,c.id+'.txt');fs.writeFileSync(prompt,c.prompt,{flag:'wx'});
 const start=performance.now(),r=spawnSync(binary,['--prompt-file',prompt,'--ledger',ledger,'--output',dir,'--kicad-cli','/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli'],{env,encoding:'utf8',timeout:600000,maxBuffer:2000000});
 fs.writeFileSync(path.join(out,c.id+'.stdout.log'),r.stdout||'');fs.writeFileSync(path.join(out,c.id+'.stderr.log'),r.stderr||'');
 let selection=null,result=null;try{selection=read(path.join(dir,'selection.json'))}catch{}try{result=JSON.parse(r.stdout)}catch{}
 const after=read(ledger),entry=after.entries.at(-1),noNative=['board.kicad_sch','board.kicad_pcb','board.kicad_pro'].every(f=>!fs.existsSync(path.join(dir,f)));
 const d=selection?.decision,coverage=d?.clauses?.map(x=>x.text).join('')===c.prompt;
 const record={id:c.id,expected_disposition:c.expected_disposition,exit_code:r.status,signal:r.signal,wall_seconds:(performance.now()-start)/1000,first_attempt:true,result,selection_sha256:selection?hash(path.join(dir,'selection.json')):null,ledger_index:selection?.ledger_index??null,no_native_design:noNative,original_prompt_preserved:coverage,passed:r.status===0&&d?.disposition===c.expected_disposition&&d.configuration===null&&!!d.message?.trim()&&coverage&&noNative&&result?.passed===false&&result?.disposition===c.expected_disposition};
 summary.cases.push(record);save('summary.json',summary);console.log(JSON.stringify({id:c.id,passed:record.passed,disposition:d?.disposition,seconds:record.wall_seconds}));
 if(after.halt_reason||after.entries.length!==prior.entries.length+1||entry.status!=='completed'||selection?.ledger_index!==entry.index){summary.stopped_reason='transport/accounting result requires inspection; no automatic retry';break}
 assert.deepEqual(after.entries.slice(0,-1),prior.entries);
 assert.equal(entry.response_id,selection.response_id);assert.equal(entry.input_tokens,selection.usage.input_tokens);assert.equal(entry.output_tokens,selection.usage.output_tokens);
}
const finalLedger=read(ledger);assert.deepEqual(finalLedger.entries.slice(0,35),before.entries);
summary.metrics={unsupported_passes:summary.cases.filter(c=>c.expected_disposition==='unsupported'&&c.passed).length,unsupported_total:4,clarification_passes:summary.cases.filter(c=>c.expected_disposition==='clarify'&&c.passed).length,clarification_total:2};
summary.passed=summary.cases.length===6&&summary.cases.every(c=>c.passed)&&!summary.stopped_reason;
summary.finished_utc=new Date().toISOString();summary.request_count=finalLedger.entries.length;summary.ledger_sha256=hash(ledger);
fs.copyFileSync(ledger,path.join(out,'ledger-after.json'),fs.constants.COPYFILE_EXCL);save('summary.json',summary);
console.log(JSON.stringify({passed:summary.passed,...summary.metrics,requests:summary.request_count}));process.exitCode=summary.passed?0:1;
