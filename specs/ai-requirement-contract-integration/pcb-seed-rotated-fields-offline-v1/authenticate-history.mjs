// Read-only historical checks; only this phase's new receipt is written.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createReadStream,readFileSync,readdirSync,writeFileSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..'),base='446e19c9b934ad8026b4e9ec2996b47d9ef413f9';
const hash=b=>createHash('sha256').update(b).digest('hex');
const json=p=>JSON.parse(readFileSync(p,'utf8'));
const walk=(root,prefix='')=>readdirSync(join(root,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{assert(!e.isSymbolicLink());const p=join(prefix,e.name);return e.isDirectory()?walk(root,p):[p];});
async function digest(path){const h=createHash('sha256');let bytes=0;for await(const b of createReadStream(path)){h.update(b);bytes+=b.length;}return {bytes,sha256:h.digest('hex')};}
const prior=join(repo,'specs/ai-requirement-contract-integration/native-readability-replay-offline-v1');
assert.equal(execFileSync('git',['diff',base,'--name-only','--',prior],{cwd:repo,encoding:'utf8'}).trim(),'');
const source=json(join(repo,'.cache/native-readability-replay-v1-sources/final.json'));
assert.equal(source.files.length,23);
for(const f of source.files){assert.equal(hash(Buffer.from(f.source)),f.sha256);assert.equal(hash(execFileSync('git',['show',base+':'+f.path],{cwd:repo,maxBuffer:8*1024*1024})),f.sha256);}
const root='/tmp/kicadai-native-readability-replay-offline-v1-final',files=[];
for(const path of walk(root))files.push({path,...await digest(join(root,path))});
const inventory={file_count:files.length,total_bytes:files.reduce((n,f)=>n+f.bytes,0),sha256:hash(JSON.stringify(files))};
assert.deepEqual(inventory,json(join(prior,'verification.json')).inventory);
assert.equal(json(join(prior,'verification.json')).phase_gate_passed,false);
const archive=await digest(join(repo,'.cache/native-readability-replay-offline-v1-all-runs.tar.gz'));
assert.deepEqual(archive,{bytes:828583049,sha256:'7e0207678095c5fd7326456122f269e3de02ae23f4947692f2abcb8245d72704'});
const run=path=>JSON.parse(execFileSync(process.execPath,[path],{cwd:repo,encoding:'utf8',maxBuffer:32*1024*1024}));
const preceding=run(join(prior,'audit-history.mjs'));
const campaign=run(join(repo,'specs/ai-requirement-contract-integration/offline-repair-v1/verify-preservation.mjs'));
const result={schema:'kicadai.pcb-seed-rotated-fields-history.v1',verified_utc:new Date().toISOString(),base,prior_source_files:source.files.length,prior_source_exact_at_commit:true,prior_raw_inventory:inventory,prior_archive:archive,prior_negative_gate_preserved:true,preceding,campaign,provider_calls:0};
writeFileSync(join(phase,'history-authentication.json'),JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({verified:true,prior_source_files:source.files.length,prior_raw_files:inventory.file_count,prior_negative_gate_preserved:true,provider_calls:0}));
