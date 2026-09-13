// Authenticate the predecessor at its own source commit, not this worktree.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {createReadStream,readFileSync,readdirSync} from 'node:fs';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..'),base='b9d48213aa0431d5cf19824287c6403a4aea5529';
const sha=b=>createHash('sha256').update(b).digest('hex');
const old=JSON.parse(readFileSync(join(phase,'../explicit-reference-promotion-offline-v1/verification.json')));
for(const source of old.source)assert.equal(sha(execFileSync('git',['show',base+':'+source.path],{cwd:repo})),source.sha256);
assert.equal(execFileSync('git',['diff',base,'--name-only','--','specs/ai-requirement-contract-integration/explicit-reference-promotion-offline-v1'],{cwd:repo,encoding:'utf8'}),'');
const walk=(dir,prefix='')=>readdirSync(join(dir,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{assert(!e.isSymbolicLink());const p=join(prefix,e.name);return e.isDirectory()?walk(dir,p):[p];});
const files=[];for(const path of walk(old.root)){const h=createHash('sha256');let bytes=0;for await(const chunk of createReadStream(join(old.root,path))){bytes+=chunk.length;h.update(chunk);}files.push({path,bytes,sha256:h.digest('hex')});}
const inventory={file_count:files.length,total_bytes:files.reduce((s,f)=>s+f.bytes,0),sha256:sha(JSON.stringify(files))};assert.deepEqual(inventory,old.inventory);
console.log(JSON.stringify({verified_utc:new Date().toISOString(),previous_source_commit:base,previous_source_files_authenticated:old.source.length,previous_reports_unchanged:true,previous_raw_inventory:inventory,provider_calls:0},null,2));
