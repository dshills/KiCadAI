// Final evidence/report QA retains failed bounded evaluations explicitly.
import assert from 'node:assert/strict';
import {readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..');
const json=p=>JSON.parse(readFileSync(p)),sha=b=>createHash('sha256').update(b).digest('hex');
const key=process.env.OPENAI_API_KEY;assert(key,'Existing key is used only for a local exact-secret scan');
const modes=['focused','focused-dev2','focused-dev3','focused-final','native-dev1','native-dev2','native-dev3','native-final','lint','lint-dev2','lint-dev3','lint-final','race-ownership','race-layout','race-designapi','vet','full'];
assert.deepEqual(readdirSync(phase).filter(p=>p.endsWith('.execution.json')).map(p=>p.replace('.execution.json','')).sort(),[...modes].sort());
const frozen=json(join(repo,'.cache/pin-aware-readability-v1-sources/native-final.json'));assert.equal(frozen.files.length,21);
for(const f of frozen.files){assert.equal(Buffer.byteLength(f.source),f.bytes);assert.equal(sha(Buffer.from(f.source)),f.sha256);assert.equal(sha(readFileSync(join(repo,f.path))),f.sha256);}
const executions=[];
for(const mode of modes){const receipt=json(join(phase,mode+'.execution.json')),snapshot=json(receipt.source_snapshot);
 assert.equal(receipt.source_snapshot_sha256,sha(readFileSync(receipt.source_snapshot)));assert.equal(receipt.log_sha256,sha(readFileSync(join(phase,mode+'.log'))));assert.equal(receipt.source_unchanged,true);
 assert(receipt.removed_provider_variables.includes('OPENAI_API_KEY'));assert.equal(receipt.dependency_network,'disabled');assert.equal(receipt.signal,null);assert([0,1].includes(receipt.code));
 for(const f of snapshot.files){assert.equal(sha(Buffer.from(f.source)),f.sha256);assert(!f.source.includes(key));}
 if(['focused-final','native-final','lint-final','race-ownership','race-layout','race-designapi','vet','full'].includes(mode))assert.deepEqual(snapshot.files,frozen.files);
 if(mode.startsWith('native-'))assert.equal(receipt.code,1,'Recorded negative native attempt must stay negative');
 executions.push({mode,exit_code:receipt.code,started_utc:receipt.started_utc,finished_utc:receipt.finished_utc,source_unchanged:true});
}
const full=readFileSync(join(phase,'full.log'),'utf8'),passes=[...full.matchAll(/^ok\s+(\S+)\s+([0-9.]+)s$/gm)].map(m=>({package:m[1],seconds:Number(m[2])})),noTests=[...full.matchAll(/^\?\s+\S+\s+\[no test files\]$/gm)].length;
const fullCode=executions.find(e=>e.mode==='full').exit_code;
if(fullCode===0){assert.equal(passes.length,151);assert.equal(noTests,14);assert(passes.every(p=>p.seconds<=720));assert(passes.some(p=>p.package==='kicadai/internal/compositionlowering'));}
const verification=json(join(phase,'verification.json'));assert.equal(verification.technical_gate_passed,false);assert.equal(verification.technical_examples_passed,1);assert.equal(verification.complete_practical_benchmark_cases,0);
const attempts=json(join(phase,'attempts.json'));assert.equal(attempts.development_attempts,3);assert.equal(attempts.frozen_final_attempts,1);assert.equal(attempts.final_matches_last_development_inputs_and_available_primary_projects,true);
const pins=json(join(phase,'pin-aware-audit.json'));assert.equal(pins.cases[0].eligible_attachments,4);assert.equal(pins.cases[0].side_passes,4);assert.equal(pins.cases[0].blocks.length,2);assert.equal(pins.cases[1].status,'not_evaluable');
const metrics=json(join(phase,'readability-metrics.json'));assert(metrics.cases.every(c=>!c.decoupling_non_regression));assert.equal(metrics.practical_benchmark_passes_added,0);
const archive=json(join(phase,'archive.json'));assert.equal(archive.streamed.exact_secret_scan,'absent');assert.equal(archive.all_original_files_unchanged,true);
assert.equal(json(join(phase,'history-authentication.json')).previous[0].commit,'f8b29a9904dd02c38583cb4c15d72f55b5ee6b75');
const files=readdirSync(phase).sort(),links=[],artifacts=[];
for(const file of files){const b=readFileSync(join(phase,file));assert(!b.includes(Buffer.from(key)),'Secret in phase artifact');if(file.endsWith('.json'))JSON.parse(b);
 if(file!=='qa.json')artifacts.push({path:file,bytes:b.length,sha256:sha(b)});
 if(file.endsWith('.md'))for(const m of b.toString().matchAll(/\]\(([^)]+)\)/g)){const target=m[1];if(/^https?:/.test(target))continue;const path=resolve(phase,target.replace(/:\d+$/,''));assert(existsSync(path)||path===join(phase,'qa.json'),'Broken artifact link '+target);links.push({file,target});}
}
assert(!/TESTS_TO_BE_RECORDED|being finalized|still pending/.test(readFileSync(join(phase,'README.md'),'utf8')));
assert.equal(execFileSync('git',['diff','--check'],{cwd:repo,encoding:'utf8'}),'');assert.equal(execFileSync('git',['diff','--cached','--check'],{cwd:repo,encoding:'utf8'}),'');
const result={schema:'kicadai.pin-aware-readability-qa.v1',source_files:frozen.files.length,executions,full_suite:{passed_packages:passes.length,no_test_packages:noTests,longest:passes.toSorted((a,b)=>b.seconds-a.seconds)[0],exit_code:fullCode},technical_examples_passed:1,pin_aware_examples_certified:1,local_panel_examples_certified:1,complete_readability_examples:0,planned_examples:2,benchmark_passes_added:0,links_checked:links.length,artifacts,exact_secret_scan:'absent',provider_calls:0};
const path=join(phase,'qa.json');if(existsSync(path))assert.deepEqual(json(path),result);else writeFileSync(path,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
for(const link of links)assert(existsSync(resolve(phase,link.target.replace(/:\d+$/,''))));
console.log(JSON.stringify({...result,artifacts:artifacts.length},null,2));
