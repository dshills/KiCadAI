// Archive all bounded native runs and source snapshots, then stream-authenticate
// every member. No second full extraction and no historical evidence deletion.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createReadStream,readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {join,resolve,dirname,basename} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..');
const hash=b=>createHash('sha256').update(b).digest('hex');
const secret=process.env.OPENAI_API_KEY;assert(secret,'Existing key used only for local exact-secret scan');
const walk=(root,prefix='')=>readdirSync(join(root,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{assert(!e.isSymbolicLink());const p=join(prefix,e.name);return e.isDirectory()?walk(root,p):[p];});
async function digest(path,scan=true){const h=createHash('sha256');let bytes=0,tail=Buffer.alloc(0);const key=Buffer.from(secret);for await(const b of createReadStream(path)){h.update(b);bytes+=b.length;if(scan){const data=Buffer.concat([tail,b]);assert(!data.includes(key),'Secret found in retained evidence');tail=data.subarray(Math.max(0,data.length-key.length+1));}}return {bytes,sha256:h.digest('hex')};}
const names=['dev1','dev2','final'].map(n=>'kicadai-pcb-seed-rotated-fields-offline-v1-'+n);
const sourceRoot=join(repo,'.cache/pcb-seed-rotated-fields-v1-sources');
const roots=[...names.map(n=>join('/tmp',n)),sourceRoot];
const entries=[],inventories=[];
for(const root of roots){const files=[];for(const path of walk(root)){const d=await digest(join(root,path));files.push({path,...d});entries.push({path:basename(root)+'/'+path,...d});}inventories.push({root,files:files.length,bytes:files.reduce((n,f)=>n+f.bytes,0),sha256:hash(JSON.stringify(files))});}
for(const path of walk(sourceRoot)){const s=JSON.parse(readFileSync(join(sourceRoot,path)));for(const f of s.files){assert.equal(Buffer.byteLength(f.source),f.bytes);assert.equal(hash(Buffer.from(f.source)),f.sha256);}}
const manifest=join(phase,'archive-manifest.json');writeFileSync(manifest,JSON.stringify(entries,null,2)+'\n',{flag:'wx'});
const archive=join(repo,'.cache/pcb-seed-rotated-fields-offline-v1-all-runs.tar.gz');assert(!existsSync(archive));
const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
execFileSync('/usr/bin/tar',['-czf',archive,'-C','/tmp',...names,'-C',join(repo,'.cache'),basename(sourceRoot)],{env,timeout:120000});
const python=`import sys, json, tarfile, hashlib
expected={e['path']:e for e in json.load(sys.stdin)}
seen=set(); total=0
with tarfile.open(sys.argv[1], 'r|gz') as archive:
 for member in archive:
  if member.isdir(): continue
  assert member.isfile() and member.name in expected and member.name not in seen, 'Unexpected archive member'
  e=expected[member.name]; h=hashlib.sha256(); size=0
  with archive.extractfile(member) as f:
   while True:
    b=f.read(1024*1024)
    if not b: break
    h.update(b); size+=len(b)
  assert size==e['bytes'] and h.hexdigest()==e['sha256'], 'Archive content mismatch'
  total+=size; seen.add(member.name)
assert seen==set(expected), 'Archive member missing'
print(json.dumps({'files_verified':len(seen),'bytes_verified':total,'full_extraction_created':False}))
`;
const streamed=JSON.parse(execFileSync('/usr/bin/python3',['-c',python,archive],{env,input:JSON.stringify(entries),encoding:'utf8',timeout:120000,maxBuffer:1024*1024}));
const result={schema:'kicadai.pcb-seed-rotated-fields-archive.v1',verified_utc:new Date().toISOString(),archive,...await digest(archive,false),manifest_sha256:hash(readFileSync(manifest)),inventories,streamed,source_content_verified:true,exact_secret_scan:'absent',provider_calls:0};
writeFileSync(join(phase,'archive.json'),JSON.stringify(result,null,2)+'\n',{flag:'wx'});console.log(JSON.stringify(result));
