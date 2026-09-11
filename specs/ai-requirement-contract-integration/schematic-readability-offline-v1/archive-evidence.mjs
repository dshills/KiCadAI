// Retain all development runs, including failures, and the final native run.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {createReadStream,readdirSync,existsSync,mkdirSync,statSync} from 'node:fs';
import {join,resolve} from 'node:path';
const repo=process.cwd(),prefix='kicadai-schematic-readability-offline-v1-',names=[...Array.from({length:9},(_,i)=>prefix+'dev'+(i+1)),prefix+'final'];
const archive=resolve(repo,'.cache/schematic-readability-offline-v1-all-runs.tar.gz'),extracted=resolve(repo,'.cache/schematic-readability-offline-v1-archive-verification');
assert(!existsSync(archive)&&!existsSync(extracted),'Never overwrite a sealed archive');
const sha=b=>createHash('sha256').update(b).digest('hex');
const walk=(dir,prefix='')=>readdirSync(join(dir,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{assert(!e.isSymbolicLink());const p=join(prefix,e.name);return e.isDirectory()?walk(dir,p):[p];});
const secret=process.env.OPENAI_API_KEY;assert(secret,'Existing key required for local exact-secret scan; never printed or transmitted');
async function fileDigest(path,scan){let bytes=0,carry=Buffer.alloc(0);const h=createHash('sha256'),needle=Buffer.from(secret);for await(const chunk of createReadStream(path)){h.update(chunk);bytes+=chunk.length;if(scan){const b=Buffer.concat([carry,chunk]);assert(!b.includes(needle),'Exact-secret scan failed');carry=b.subarray(Math.max(0,b.length-needle.length+1));}}return {bytes,sha256:h.digest('hex')};}
async function inventory(dir,scan){const files=[];for(const path of walk(dir))files.push({path,...await fileDigest(join(dir,path),scan)});return {file_count:files.length,total_bytes:files.reduce((s,f)=>s+f.bytes,0),sha256:sha(JSON.stringify(files))};}
const inventories=[];for(const name of names)inventories.push({name,...await inventory(join('/tmp',name),true)});
const env={...process.env,COPYFILE_DISABLE:'1'};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const commands=[];const run=args=>{const r=spawnSync('/usr/bin/tar',args,{env,encoding:'utf8',timeout:600000,maxBuffer:1024*1024});commands.push({command:'/usr/bin/tar',args,status:r.status,stderr:r.stderr});assert.equal(r.status,0);};
run(['-czf',archive,'-C','/tmp',...names]);mkdirSync(extracted);run(['-xzf',archive,'-C',extracted]);
for(const old of inventories){const {name,...expected}=old;assert.deepEqual(await inventory(join(extracted,name),false),expected,'Fresh extraction mismatch');assert.deepEqual(await inventory(join('/tmp',name),false),expected,'Original changed while archiving');}
console.log(JSON.stringify({verified_utc:new Date().toISOString(),archive,archive_bytes:statSync(archive).size,archive_sha256:(await fileDigest(archive,false)).sha256,extracted,all_development_and_final_runs_retained:true,fresh_extraction_verified:true,exact_existing_secret_absent:true,inventories,commands,provider_calls:0},null,2));
