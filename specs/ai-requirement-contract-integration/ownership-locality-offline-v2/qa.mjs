// Final report/evidence QA. A failed bounded test is retained, never hidden.
import assert from 'node:assert/strict';
import {readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..');
const json=p=>JSON.parse(readFileSync(p)),sha=b=>createHash('sha256').update(b).digest('hex');
const key=process.env.OPENAI_API_KEY;assert(key,'Existing key used only for exact local secret scanning');
const modes=['focused','focused-dev2','focused-final','native-dev1','native-final','lint','lint-final','race-ownership','race-layout','vet','full'];
const frozen=json(join(repo,'.cache/ownership-locality-v2-sources/native-final.json'));
assert.equal(frozen.files.length,11);
for(const f of frozen.files){assert.equal(Buffer.byteLength(f.source),f.bytes);assert.equal(sha(Buffer.from(f.source)),f.sha256);assert.equal(sha(readFileSync(join(repo,f.path))),f.sha256);}
const executions=[];
for(const mode of modes){
 const receipt=json(join(phase,mode+'.execution.json')),snapshot=json(receipt.source_snapshot);
 assert.equal(receipt.source_snapshot_sha256,sha(readFileSync(receipt.source_snapshot)));assert.equal(receipt.log_sha256,sha(readFileSync(join(phase,mode+'.log'))));assert.equal(receipt.source_unchanged,true);
 assert(receipt.removed_provider_variables.includes('OPENAI_API_KEY'));assert.equal(receipt.dependency_network,'disabled');
 for(const f of snapshot.files){assert.equal(sha(Buffer.from(f.source)),f.sha256);assert(!f.source.includes(key));}
 if(['focused-final','native-final','lint-final','race-ownership','race-layout','vet','full'].includes(mode))assert.deepEqual(snapshot.files,frozen.files);
 assert.equal(receipt.signal,null);assert([0,1].includes(receipt.code));
 executions.push({mode,exit_code:receipt.code,started_utc:receipt.started_utc,finished_utc:receipt.finished_utc,source_unchanged:true});
}
const full=readFileSync(join(phase,'full.log'),'utf8'),passes=[...full.matchAll(/^ok\s+(\S+)\s+([0-9.]+)s$/gm)].map(m=>({package:m[1],seconds:Number(m[2])})),noTests=[...full.matchAll(/^\?\s+\S+\s+\[no test files\]$/gm)].length;
assert(passes.some(p=>p.package==='kicadai/internal/compositionlowering'),'Thermal rejection coverage absent');
assert(passes.every(p=>p.seconds<=720));
const verification=json(join(phase,'verification.json'));assert.equal(verification.technical_gate_passed,true);assert.equal(verification.complete_practical_benchmark_cases,0);
const development=json(join(phase,'native-dev1-verification.json'));
assert.deepEqual(json(join(development.root,'sealed-projects.json')).projects,json(join(verification.root,'sealed-projects.json')).projects,'Development-to-final primary projects changed');
for(const name of ['standalone_regulator','controller_adc_100ma'])assert.deepEqual(json(join(development.root,name,'workflow_request.json')),json(join(verification.root,name,'workflow_request.json')));
const nativeModes=modes.filter(m=>m.startsWith('native-'));assert.equal(nativeModes.length,2);assert(!existsSync(join(phase,'native-dev2.execution.json')));
for(const c of json(join(phase,'readability-metrics.json')).cases){assert.equal(c.decoupling_non_regression,true);assert(c.mean_decoupling_distance_mm.current<c.mean_decoupling_distance_mm.previous);assert(c.max_decoupling_distance_mm.current<c.max_decoupling_distance_mm.previous);}
assert.equal(json(join(phase,'readability-metrics.json')).readability_passes,0);
const archive=json(join(phase,'archive.json'));assert.equal(archive.streamed.exact_secret_scan,'absent');assert.equal(archive.all_original_files_unchanged,true);
const phaseFiles=readdirSync(phase).sort(),links=[];
for(const file of phaseFiles){const b=readFileSync(join(phase,file));assert(!b.includes(Buffer.from(key)),'Secret in report artifact');
 if(file.endsWith('.json'))JSON.parse(b);
 if(file.endsWith('.md'))for(const m of b.toString().matchAll(/\]\(([^)]+)\)/g)){const target=m[1];if(/^https?:/.test(target))continue;const path=resolve(phase,target.replace(/:\d+$/,''));assert(existsSync(path)||path===join(phase,'qa.json'),'Broken artifact link '+target);links.push({file,target});}
}
assert(!/being finalized|still pending/.test(readFileSync(join(phase,'README.md'),'utf8')));
assert.equal(execFileSync('git',['diff','--check'],{cwd:repo,encoding:'utf8'}),'');
assert.equal(execFileSync('git',['diff','--cached','--check'],{cwd:repo,encoding:'utf8'}),'');
const result={schema:'kicadai.ownership-locality-qa.v2',source_files:frozen.files.length,executions,full_suite:{passed_packages:passes.length,no_test_packages:noTests,longest:passes.toSorted((a,b)=>b.seconds-a.seconds)[0],exit_code:executions.find(e=>e.mode==='full').exit_code},links_checked:links.length,exact_secret_scan:'absent',provider_calls:0,technical_examples_passed:2,complete_readability_examples:0,benchmark_passes_added:0};
const path=join(phase,'qa.json');if(existsSync(path))assert.deepEqual(json(path),result);else writeFileSync(path,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
for(const link of links)assert(existsSync(resolve(phase,link.target.replace(/:\d+$/,''))),'Unresolved artifact link after QA publication');
console.log(JSON.stringify(result,null,2));
