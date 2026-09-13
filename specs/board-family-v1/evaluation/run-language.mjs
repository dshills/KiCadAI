// Live runner: authored offline. Do not execute without concrete-payload approval.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {spawnSync} from 'node:child_process';
if(process.argv[2]!=='--live'||process.argv.length!==4)throw Error('usage AFTER APPROVAL ONLY: node run-language.mjs --live NEW_OUTPUT');
const root=process.cwd(),out=path.resolve(process.argv[3]);
const ledger=path.join(root,'.cache/board-family-v1/live-ledger.json');
const file=path.join(root,'specs/board-family-v1/evaluation/language.json');
const spec=JSON.parse(fs.readFileSync(file));
const binary=path.join(root,'.cache/board-family-v1/kicadai-board-family');
const hash=f=>crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
if(fs.existsSync(out))throw Error('output must be new');
const prior=fs.existsSync(ledger)?JSON.parse(fs.readFileSync(ledger)):null;
if(prior?.halt_reason||(prior?.entries?.length??0)+spec.cases.length>20)throw Error('insufficient remaining request allowance or halted ledger');
if(!process.env.OPENAI_API_KEY)throw Error('existing key unavailable; do not provision a new key automatically');
fs.mkdirSync(out,{recursive:true});
const env={...process.env};for(const k of ['ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const summary={started_utc:new Date().toISOString(),spec_sha256:hash(file),binary_sha256:hash(binary),runner_sha256:hash(new URL(import.meta.url)),ledger_path:ledger,prior_request_count:prior?.entries?.length??0,independence:spec.independence,cases:[]};
const save=()=>fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify(summary,null,2)+'\n');save();
for(const c of spec.cases){
 const prompt=path.join(out,c.id+'.txt'),dir=path.join(out,c.id);fs.writeFileSync(prompt,c.prompt,{flag:'wx'});
 const start=performance.now();const r=spawnSync(binary,['--prompt-file',prompt,'--ledger',ledger,'--output',dir,'--kicad-cli','/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli'],{env,encoding:'utf8',timeout:600000,maxBuffer:2000000});
 fs.writeFileSync(path.join(out,c.id+'.stdout.log'),r.stdout||'');fs.writeFileSync(path.join(out,c.id+'.stderr.log'),r.stderr||'');
 let result,selection;try{result=JSON.parse(r.stdout);selection=JSON.parse(fs.readFileSync(path.join(dir,'selection.json')))}catch{}
 const disposition=selection?.decision?.disposition;
 const record={id:c.id,expected_disposition:c.expected_disposition,disposition,exit_code:r.status,signal:r.signal,wall_seconds:(performance.now()-start)/1000,result,selection_sha256:fs.existsSync(path.join(dir,'selection.json'))?hash(path.join(dir,'selection.json')):null,passed:false};
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
 // A semantic first-shot miss stays in the denominator; no repair or retry.
 // Transport/protocol/native failures stop this run with partial evidence.
 if(r.status!==0){summary.stopped_reason='execution failure; inspect retained evidence before any further paid work';break}
}
const group=name=>summary.cases.filter(c=>c.expected_disposition===name);
const supported=group('supported'),times=supported.map(c=>c.wall_seconds).sort((a,b)=>a-b);
summary.metrics={supported_first_shot_passes:supported.filter(c=>c.passed).length,supported_expected_total:10,unsupported_passes:group('unsupported').filter(c=>c.passed).length,unsupported_expected_total:4,clarification_passes:group('clarify').filter(c=>c.passed).length,clarification_expected_total:2,median_supported_seconds:times.length===10?(times[4]+times[5])/2:null,max_supported_seconds:times.length?Math.max(...times):null};
summary.passed=summary.cases.length===16&&summary.metrics.supported_first_shot_passes>=9&&summary.metrics.unsupported_passes===4&&summary.metrics.clarification_passes===2&&summary.metrics.median_supported_seconds<300&&summary.metrics.max_supported_seconds<600&&supported.every(c=>c.native_validation_passed);
summary.finished_utc=new Date().toISOString();if(fs.existsSync(ledger)){summary.ledger_sha256=hash(ledger);summary.request_count=JSON.parse(fs.readFileSync(ledger)).entries.length}
save();console.log(JSON.stringify(summary.metrics));process.exitCode=summary.passed?0:1;
