// Final handoff QA: bind command results, snapshots, report links and claims.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync,readdirSync,existsSync,writeFileSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..');
const hash=b=>createHash('sha256').update(b).digest('hex'),json=p=>JSON.parse(readFileSync(p,'utf8'));
const secret=process.env.OPENAI_API_KEY;assert(secret,'Existing key used only for local exact-secret scan');
const final=json(join(repo,'.cache/pcb-seed-rotated-fields-v1-sources/native-final.json'));
for(const f of final.files)assert.equal(hash(readFileSync(join(repo,f.path))),f.sha256);
const modes=['native-dev1','native-dev2','native-final','focused','focused-dev2','focused-final','lint','lint-final','vet','race','full'];
const runs=[];
for(const mode of modes){const r=json(join(phase,mode+'.execution.json')),s=json(r.source_snapshot);assert.equal(hash(readFileSync(r.source_snapshot)),r.source_snapshot_sha256);assert.equal(hash(readFileSync(join(phase,mode+'.log'))),r.log_sha256);assert.equal(r.source_unchanged,true);assert.equal(r.code,mode==='focused-dev2'?1:0);for(const f of s.files){assert.equal(Buffer.byteLength(f.source),f.bytes);assert.equal(hash(Buffer.from(f.source)),f.sha256);}if(['native-final','focused-final','lint-final','vet','race','full'].includes(mode))assert.deepEqual(s.files,final.files,'Final checks used different source');runs.push({mode,code:r.code,started_utc:r.started_utc,finished_utc:r.finished_utc});}
const full=readFileSync(join(phase,'full.log'),'utf8'),pass=[...full.matchAll(/^ok\s+(\S+)\s+([\d.]+)s$/gm)],noTests=[...full.matchAll(/^\?\s+(\S+)\s+\[no test files\]$/gm)];
assert.equal(pass.length,151);assert.equal(noTests.length,14);
assert(pass.some(m=>m[1]==='kicadai/internal/compositionlowering'),'Thermal preservation test package absent');
const longest=pass.map(m=>({package:m[1],seconds:Number(m[2])})).sort((a,b)=>b.seconds-a.seconds)[0];assert(longest.seconds<=720);
const verification=json(join(phase,'verification.json'));assert.equal(verification.technical_gate_passed,true);assert.equal(verification.complete_practical_benchmark_cases,0);assert.equal(verification.cases.length,2);
let files=0,links=0;
for(const name of readdirSync(phase)){const path=join(phase,name),b=readFileSync(path);assert(!b.includes(Buffer.from(secret)),'Secret present in phase artifact');files++;if(name.endsWith('.json'))json(path);if(name.endsWith('.md'))for(const m of b.toString('utf8').matchAll(/\]\(([^)]+)\)/g)){const p=m[1].split('#')[0];if(!p||/^[a-z]+:/i.test(p))continue;const target=resolve(phase,p);assert(existsSync(target)||target===join(phase,'qa.json'),'Broken report link: '+p);links++;}}
assert(!readFileSync(join(phase,'README.md'),'utf8').includes('still pending'),'Unfinished status in report');
const result={schema:'kicadai.pcb-seed-rotated-fields-handoff-qa.v1',runs,source_files:final.files.length,full_suite:{passed_packages:pass.length,no_test_packages:noTests.length,longest},phase_files_scanned:files,local_links_checked:links,exact_secret_scan:'absent',native_targeted_examples_passed:2,practical_benchmark_passes_added:0,provider_calls:0,assessment:'Ready to share scoped technical fixes with explicit readability and practical-milestone caveats'};
const path=join(phase,'qa.json');if(existsSync(path)){const prior=json(path);delete prior.phase_files_scanned;const current={...result};delete current.phase_files_scanned;assert.deepEqual(current,prior);}else writeFileSync(path,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify(result));
