// Live runner: authored offline. Do not execute without concrete-payload approval.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {spawnSync} from 'node:child_process';
if(process.argv[2]!=='--live'||![4,6].includes(process.argv.length)||(process.argv.length===6&&process.argv[4]!=='--prior-run'))throw Error('usage AFTER APPROVAL ONLY: node run-language.mjs --live NEW_OUTPUT [--prior-run PREVIOUS_OUTPUT]');
const root=process.cwd(),out=path.resolve(process.argv[3]);
const ledger=path.join(root,'.cache/board-family-v1/live-ledger.json');
const file=path.join(root,'specs/board-family-v1/evaluation/language.json');
const spec=JSON.parse(fs.readFileSync(file));
const binary=path.join(root,'.cache/board-family-v1/kicadai-board-family');
const hash=f=>crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const priorSummaryPath=process.argv[5]?path.join(path.resolve(process.argv[5]),'summary.json'):null;
const firstAttempts=new Map(),latestAttempts=new Map(),visited=new Set();
function readHistory(summaryPath,expectedHash){
 if(visited.has(summaryPath))throw Error('cyclic prior-run history');visited.add(summaryPath);
 if(expectedHash&&hash(summaryPath)!==expectedHash)throw Error('prior-run evidence changed');
 const s=JSON.parse(fs.readFileSync(summaryPath));
 if(s.spec_sha256!==hash(file))throw Error('prior run used different cases; do not reset first-shot history');
 if(s.prior_run)readHistory(s.prior_run.summary_path,s.prior_run.sha256);
 for(const c of s.cases){if(!firstAttempts.has(c.id))firstAttempts.set(c.id,c);latestAttempts.set(c.id,c)}
}
if(priorSummaryPath)readHistory(priorSummaryPath);
// Continue only cases never attempted. Recovery needs an explicit separate run;
// it cannot replace the earliest observation in the first-shot denominator.
const pending=spec.cases.filter(c=>!firstAttempts.has(c.id));
if(fs.existsSync(out))throw Error('output must be new');
const prior=fs.existsSync(ledger)?JSON.parse(fs.readFileSync(ledger)):null;
if(!pending.length)throw Error('no unseen cases remain; do not silently repeat evaluation');
if(prior?.halt_reason||(prior?.entries?.length??0)+pending.length>20)throw Error('insufficient remaining request allowance or halted ledger');
if(!process.env.OPENAI_API_KEY)throw Error('existing key unavailable; do not provision a new key automatically');
fs.mkdirSync(out,{recursive:true});
const env={...process.env};for(const k of ['ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const summary={started_utc:new Date().toISOString(),spec_sha256:hash(file),binary_sha256:hash(binary),runner_sha256:hash(new URL(import.meta.url)),ledger_path:ledger,prior_request_count:prior?.entries?.length??0,planned_case_ids:pending.map(c=>c.id),independence:spec.independence,cases:[]};
if(priorSummaryPath)summary.prior_run={summary_path:priorSummaryPath,sha256:hash(priorSummaryPath),notice:'Earlier first-shot outcomes are retained. Repeated cases in this run are explicit recovery attempts, not new first-shot successes.'};
const save=()=>fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify(summary,null,2)+'\n');save();
for(const c of pending){
 const prompt=path.join(out,c.id+'.txt'),dir=path.join(out,c.id);fs.writeFileSync(prompt,c.prompt,{flag:'wx'});
 const start=performance.now();const r=spawnSync(binary,['--prompt-file',prompt,'--ledger',ledger,'--output',dir,'--kicad-cli','/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli'],{env,encoding:'utf8',timeout:600000,maxBuffer:2000000});
 fs.writeFileSync(path.join(out,c.id+'.stdout.log'),r.stdout||'');fs.writeFileSync(path.join(out,c.id+'.stderr.log'),r.stderr||'');
 let result,selection;try{result=JSON.parse(r.stdout);selection=JSON.parse(fs.readFileSync(path.join(dir,'selection.json')))}catch{}
 const disposition=selection?.decision?.disposition;
 const record={id:c.id,expected_disposition:c.expected_disposition,disposition,exit_code:r.status,signal:r.signal,wall_seconds:(performance.now()-start)/1000,result,selection_sha256:fs.existsSync(path.join(dir,'selection.json'))?hash(path.join(dir,'selection.json')):null,passed:false};
 record.first_attempt=!firstAttempts.has(c.id);
 if(c.expected_disposition==='supported'){
  const expected={...spec.configuration_expectations,profile:c.expected_profile,total_bus_capacitance_pf:spec.capacitance_overrides_pf[c.id]??(c.expected_profile==='standard'?200:100)};
  const got=selection?.decision?.configuration;
  record.configuration_matches=!!got&&Object.entries(expected).every(([k,v])=>got[k]===v);
  let validation;try{validation=JSON.parse(fs.readFileSync(path.join(dir,'validation.json')))}catch{}
  record.native_validation_passed=validation?.passed===true&&validation?.checks?.every(c=>c.passed===true);
  record.passed=r.status===0&&disposition==='supported'&&result?.passed===true&&record.configuration_matches&&record.native_validation_passed&&record.wall_seconds<600;
 }else{
  record.no_native_design=['board.kicad_sch','board.kicad_pcb','board.kicad_pro'].every(f=>!fs.existsSync(path.join(dir,f)));
  record.passed=r.status===0&&disposition===c.expected_disposition&&selection.decision.configuration===null&&record.no_native_design;
 }
 summary.cases.push(record);save();console.log(JSON.stringify({id:c.id,passed:record.passed,disposition,seconds:record.wall_seconds}));
 if(!firstAttempts.has(c.id))firstAttempts.set(c.id,record);
 latestAttempts.set(c.id,record);
 // A semantic first-shot miss stays in the denominator; no repair or retry.
 // Transport/protocol/native failures stop this run with partial evidence.
 if(r.status!==0){summary.stopped_reason='execution failure; inspect retained evidence before any further paid work';break}
}
const group=(map,name)=>[...map.values()].filter(c=>c.expected_disposition===name);
const supported=group(latestAttempts,'supported'),times=supported.filter(c=>c.passed).map(c=>c.wall_seconds).sort((a,b)=>a-b);
const median=times.length?(times[Math.floor((times.length-1)/2)]+times[Math.floor(times.length/2)])/2:null;
summary.metrics={supported_first_shot_passes:group(firstAttempts,'supported').filter(c=>c.passed).length,supported_first_selection_matches:group(firstAttempts,'supported').filter(c=>c.disposition==='supported'&&c.configuration_matches).length,supported_latest_passes:supported.filter(c=>c.passed).length,supported_expected_total:10,unsupported_passes:group(firstAttempts,'unsupported').filter(c=>c.passed).length,unsupported_expected_total:4,clarification_passes:group(firstAttempts,'clarify').filter(c=>c.passed).length,clarification_expected_total:2,completed_supported_timing_count:times.length,median_completed_supported_seconds:median,max_completed_supported_seconds:times.length?Math.max(...times):null};
summary.passed=firstAttempts.size===16&&summary.metrics.supported_first_shot_passes>=9&&summary.metrics.unsupported_passes===4&&summary.metrics.clarification_passes===2&&times.length===10&&median<300&&Math.max(...times)<600&&supported.every(c=>c.native_validation_passed);
summary.finished_utc=new Date().toISOString();if(fs.existsSync(ledger)){summary.ledger_sha256=hash(ledger);summary.request_count=JSON.parse(fs.readFileSync(ledger)).entries.length}
save();console.log(JSON.stringify(summary.metrics));process.exitCode=summary.passed?0:1;
