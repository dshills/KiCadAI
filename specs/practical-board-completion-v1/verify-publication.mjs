import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {join,isAbsolute} from 'node:path';
import {inventory,verifyInventory,sha,summarizeReadiness} from './evidence.mjs';
import {verifyNegativePublication} from './publication-checks.mjs';

const repo=process.cwd(),phase=join(repo,'specs/practical-board-completion-v1');
const raw=join(repo,'.cache/practical-board-completion-v1'),pub=join(phase,'publication');
const secret=process.env.OPENAI_API_KEY;assert(secret,'existing approved key required only for exact local leak scanning');
const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_OPENAI_LIVE_TEST'])delete env[k];
const git=(...args)=>execFileSync('git',args,{cwd:repo,env,maxBuffer:32*1024*1024});
const json=p=>JSON.parse(fs.readFileSync(p));
const safe=p=>{assert(p&&!isAbsolute(p)&&!p.split('/').some(x=>['','..','.'].includes(x))&&!p.includes('\\'));return join(repo,p);};
const checkRecord=f=>{const b=fs.readFileSync(safe(f.path));if(f.bytes!==undefined)assert.equal(b.length,f.bytes);assert.equal(sha(b),f.sha256);assert(!b.includes(Buffer.from(secret)),'credential in publication evidence');};
const digest=async path=>{const hash=createHash('sha256');let bytes=0;assert(fs.lstatSync(path).isFile());for await(const b of fs.createReadStream(path)){hash.update(b);bytes+=b.length;}return {bytes,sha256:hash.digest('hex')};};

const original=json(join(repo,'specs/practical-sensor-controller-boards/protocol-v2-results.json'));
const result=json(join(phase,'results.json')),audits=json(join(phase,'clause-audits.json'));
const acceptance=Object.fromEntries(audits.cases.map(c=>[c.case_id,json(join(phase,`probes/${c.case_id}.acceptance.json`))]));
const report=verifyNegativePublication(result,audits,original,acceptance);
assert.equal(sha(fs.readFileSync(safe(result.original_baseline.source))),result.original_baseline.sha256);
for(const c of audits.cases){checkRecord(c.input);checkRecord(c.original_brief);for(const f of c.evidence)checkRecord(f);for(const clause of c.clauses)assert.deepEqual(clause.evidence,c.evidence);}
inventory(phase,secret);
const scope=json(join(pub,'branch-review-scope.json'));
for(const f of scope.files){const b=git('show',scope.source+':'+f.path);assert.equal(b.length,f.bytes);assert.equal(sha(b),f.sha256);}
assert.equal(git('diff',scope.source,'--name-only','--','cmd','internal','go.mod','go.sum','specs/practical-board-completion-v1/engine').toString().trim(),'','tested Go source changed');
assert.equal(git('diff','4d7e8c87a6bbd80de8e78b3271b1202ee13ee701','--name-only','--','specs/ai-requirement-contract-integration').toString().trim(),'','historical phase documents changed');
assert.equal(git('diff',scope.base,'--name-only','--','data').toString().trim(),'','catalog data changed');
for(const p of git('diff','--name-only',scope.base,'--').toString().trim().split('\n').filter(Boolean))assert(!fs.readFileSync(safe(p)).includes(Buffer.from(secret)),'credential in current diff');

const records=json(join(pub,'archive-manifest.json'));
for(const root of ['preparation','checks','development']){
 const subset=records.filter(f=>f.path.startsWith(root+'/')).map(f=>({...f,path:f.path.slice(root.length+1)}));
 verifyInventory(join(raw,root),subset,{secret});
}
assert(records.every(f=>/^(preparation|checks|development)\//.test(f.path)));
const archive=json(join(pub,'archive.json'));
assert.equal(sha(fs.readFileSync(join(pub,'archive-manifest.json'))),archive.manifest_sha256);
assert.deepEqual(await digest(archive.path),{bytes:archive.bytes,sha256:archive.sha256});
assert.equal(records.length,archive.streamed.files_verified);
assert.equal(records.reduce((n,f)=>n+f.bytes,0),archive.streamed.bytes_verified);
const checks=json(join(pub,'verification.json'));
for(const c of checks.checks){
 const actual=json(safe(c.raw_directory+'/execution.json'));
 const {raw_directory,...receipt}=c;assert.deepEqual(actual,receipt);
 assert.equal(sha(fs.readFileSync(safe(raw_directory+'/source.json'))),c.source_snapshot_sha256);
 assert.equal(sha(fs.readFileSync(safe(raw_directory+'/output.log'))),c.log_sha256);
 assert.equal(c.code,c.id==='reference-red'?1:0);assert.equal(c.source_unchanged,true);
}
const batch=join(raw,'development/batch-1'),manifest=json(join(phase,'development-batch-1.json'));
assert.equal(sha(fs.readFileSync(join(batch,'manifest.json'))),sha(fs.readFileSync(join(phase,'development-batch-1.json'))));
verifyInventory(batch,json(join(batch,'inventory.json')),{exclude:['inventory.json'],secret});
for(const c of manifest.cases){for(const f of c.files)checkRecord(f);for(const run of ['baseline-1','candidate-1'])assert.equal(sha(fs.readFileSync(join(batch,c.id,run,'input.json'))),sha(fs.readFileSync(safe(c.input))));}
const summary=summarizeReadiness(json(join(batch,'end.json')).outcomes),published=json(join(pub,'development-summary.json'));
for(const [k,v] of Object.entries(summary))assert.deepEqual(published[k],v);
const outcomes=json(join(batch,'end.json')).outcomes;
assert.equal(outcomes.filter(x=>x.run==='candidate-1'&&x.result?.first_failed_gate==='architecture_search').length,result.development.candidate_search_rejections);
assert.equal(outcomes.filter(x=>x.run==='baseline-1'&&x.result?.first_failed_gate==='architecture_search').length,result.development.baseline_search_rejections);
assert.equal(summary.baseline_protocol_incompatibilities,result.development.baseline_protocol_incompatibilities);

const freeze=json(join(repo,'specs/practical-sensor-controller-boards/freeze-v2.json'));
for(const f of freeze.files)checkRecord(f);
const oldInventoryPath=join(repo,'specs/practical-sensor-controller-boards/protocol-v2-evidence-inventory.json'),oldInventory=json(oldInventoryPath);
assert.equal(sha(fs.readFileSync(oldInventoryPath)),json(join(pub,'history.json')).original_inventory_sha256);
verifyInventory(oldInventory.evidence_root,oldInventory.files,{secret});
console.log(JSON.stringify({stage:'current_and_original',...report,raw_files:records.length,original_raw_files:oldInventory.files.length,provider_calls:0}));

// Preserve the historical locale-sorted depth-first serialization exactly.
const oldWalk=(dir,prefix='')=>fs.readdirSync(join(dir,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{const p=join(prefix,e.name);return e.isDirectory()?oldWalk(dir,p):[p];});
const historical=json(join(pub,'history-closed-phases.json'));
for(const h of historical.phases){
 const dir=join(repo,'specs/ai-requirement-contract-integration',h.phase),v=json(join(dir,'verification.json'));
 for(const f of v.source)assert.equal(sha(git('show',h.commit+':'+f.path)),f.sha256);
 const map=new Map(inventory(v.root,secret).map(f=>[f.path,f])),files=oldWalk(v.root).map(p=>map.get(p));
 const actual={file_count:files.length,total_bytes:files.reduce((n,f)=>n+f.bytes,0),sha256:sha(JSON.stringify(files))};
 assert.deepEqual(actual,v.inventory);assert.deepEqual(actual,h.raw);
 assert.deepEqual(await digest(h.archive.path),{bytes:h.archive.bytes,sha256:h.archive.sha256});
 console.log(JSON.stringify({stage:'historical_bytes_only',phase:h.phase,files:files.length,verified:true}));
}
console.log(JSON.stringify({verified:true,...report,raw_files:records.length,historical_phases:historical.phases.length,archive_sha256:archive.sha256,exact_secret_scan:'absent',provider_calls:0,native_executions:0}));
