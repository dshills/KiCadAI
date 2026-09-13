// Read-only recheck of the archive and its fresh extraction created by
// archive-evidence.mjs. This script never replaces or modifies evidence.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {createReadStream,readdirSync,readFileSync,statSync} from 'node:fs';
import {join,resolve} from 'node:path';
const repo=process.cwd(),archive=resolve('.cache/native-readability-replay-offline-v1-all-runs.tar.gz'),extracted=resolve('.cache/native-readability-replay-offline-v1-archive-verification');
const hash=b=>createHash('sha256').update(b).digest('hex');
const walk=(root,prefix='')=>readdirSync(join(root,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{assert(!e.isSymbolicLink());const p=join(prefix,e.name);return e.isDirectory()?walk(root,p):[p];});
async function digest(path){const h=createHash('sha256');let bytes=0;for await(const b of createReadStream(path)){h.update(b);bytes+=b.length;}return {bytes,sha256:h.digest('hex')};}
async function inventory(root){const files=[];for(const path of walk(root))files.push({path,...await digest(join(root,path))});return {file_count:files.length,total_bytes:files.reduce((n,f)=>n+f.bytes,0),sha256:hash(JSON.stringify(files))};}
const names=readdirSync(extracted).filter(n=>n.startsWith('kicadai-native-readability-replay-offline-v1-')).sort();
assert.equal(names.length,11);
const inventories=[];for(const name of names){const original=await inventory(join('/tmp',name));assert.deepEqual(await inventory(join(extracted,name)),original);inventories.push({name,...original});}
const sources='native-readability-replay-v1-sources',sourceInventory=await inventory(join(repo,'.cache',sources));assert.equal(sourceInventory.file_count,12);assert.deepEqual(await inventory(join(extracted,sources)),sourceInventory);
const archiveDigest=await digest(archive);assert.equal(archiveDigest.sha256,'7e0207678095c5fd7326456122f269e3de02ae23f4947692f2abcb8245d72704');assert.equal(statSync(archive).size,828583049);
for(const path of walk(join(repo,'.cache',sources))){const s=JSON.parse(readFileSync(join(repo,'.cache',sources,path)));for(const f of s.files){const b=Buffer.from(f.source);assert.equal(b.length,f.bytes);assert.equal(hash(b),f.sha256);}}
console.log(JSON.stringify({verified_utc:new Date().toISOString(),archive,archive_bytes:archiveDigest.bytes,archive_sha256:archiveDigest.sha256,extracted,archive_creation_and_fresh_extraction:'archive-evidence.mjs completed with exit 0; all extracted content independently rechecked here',all_development_and_final_runs_retained:true,compile_only_attempts_without_raw_directory:['dev7'],source_snapshots:sourceInventory,source_content_hashes_verified:true,inventories,provider_calls:0},null,2));
