// Verify the existing archive without recreating it. macOS bsdtar adds AppleDouble
// metadata members; validate and inventory those separately from the raw files.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createReadStream,readFileSync,writeFileSync,existsSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..'),archive=join(repo,'.cache/pin-aware-readability-offline-v1-all-runs.tar.gz');
const manifest=join(phase,'archive-manifest.json'),entries=JSON.parse(readFileSync(manifest));
const hash=b=>createHash('sha256').update(b).digest('hex');
const secret=process.env.OPENAI_API_KEY;assert(secret,'Existing key used for local exact-secret scan only');
async function digest(path){const h=createHash('sha256');let bytes=0;for await(const b of createReadStream(path)){h.update(b);bytes+=b.length;}return {bytes,sha256:h.digest('hex')};}
for(const e of entries){const first=e.path.split('/')[0],root=first==='pin-aware-readability-v1-sources'?join(repo,'.cache'): '/tmp';assert.deepEqual(await digest(join(root,e.path)),{bytes:e.bytes,sha256:e.sha256},'Original evidence changed');}
const python=`import sys,json,tarfile,hashlib,struct
p=json.load(sys.stdin); expected={e['path']:e for e in p['entries']}; key=p['secret'].encode()
seen=set(); metadata=[]; total=0; meta_seen=set()
directories={n[:i] for n in expected for i,c in enumerate(n) if c=='/'}
with tarfile.open(sys.argv[1], 'r|gz') as archive:
 for m in archive:
  if m.isdir():
   assert m.name.rstrip('/') in directories, 'Unexpected directory'
   continue
  assert m.isfile(), 'Non-regular archive member'
  if m.name not in expected:
   parts=m.name.split('/'); assert parts[-1].startswith('._'), 'Unexpected metadata name'
   parts[-1]=parts[-1][2:]; counterpart='/'.join(parts)
   assert counterpart in expected or counterpart in directories, 'Orphan metadata'
   assert m.name not in meta_seen and m.size<1024*1024, 'Invalid metadata extent'
   b=archive.extractfile(m).read(); assert key not in b and len(b)>=26, 'Invalid metadata payload'
   magic,version=struct.unpack('>II',b[:8]); assert magic==0x00051607 and version==0x00020000, 'Not AppleDouble v2'
   count=struct.unpack('>H',b[24:26])[0]; assert 26+12*count<=len(b)
   for i in range(count):
    ident,offset,length=struct.unpack('>III',b[26+12*i:38+12*i]); assert offset>=26+12*count and offset+length<=len(b), 'Invalid AppleDouble entry'
   metadata.append({'path':m.name,'bytes':len(b),'sha256':hashlib.sha256(b).hexdigest()}); meta_seen.add(m.name)
   continue
  assert m.name not in seen, 'Duplicate raw member'
  e=expected[m.name]; h=hashlib.sha256(); size=0; tail=b''
  with archive.extractfile(m) as f:
   while True:
    b=f.read(1024*1024)
    if not b: break
    data=tail+b; assert key not in data, 'Secret found in raw member'; tail=data[-max(0,len(key)-1):]
    h.update(b); size+=len(b)
  assert size==e['bytes'] and h.hexdigest()==e['sha256'], 'Raw member mismatch'
  seen.add(m.name); total+=size
assert seen==set(expected), 'Missing raw member'
print(json.dumps({'files_verified':len(seen),'bytes_verified':total,'appledouble_metadata':metadata,'full_extraction_created':False,'exact_secret_scan':'absent'}))
`;
const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const streamed=JSON.parse(execFileSync('/usr/bin/python3',['-c',python,archive],{env,input:JSON.stringify({entries,secret}),encoding:'utf8',timeout:120000,maxBuffer:8*1024*1024}));
const result={schema:'kicadai.pin-aware-readability-archive.v1',archive,...await digest(archive),manifest_sha256:hash(readFileSync(manifest)),streamed,all_original_files_unchanged:true,archive_policy:'Created once with COPYFILE_DISABLE=1. All regular data/source files are exact; any supported AppleDouble metadata is separately validated. No full extraction created.',provider_calls:0};
const path=join(phase,'archive.json');if(existsSync(path))assert.deepEqual(JSON.parse(readFileSync(path)),result);else writeFileSync(path,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({archive,bytes:result.bytes,sha256:result.sha256,files_verified:streamed.files_verified,metadata_files:streamed.appledouble_metadata.length,exact_secret_scan:'absent'}));
