// Authenticate prior sources at their commits, not against this implementation.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createReadStream,readFileSync,readdirSync,writeFileSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..');
const sha=b=>createHash('sha256').update(b).digest('hex'),json=p=>JSON.parse(readFileSync(p));
const walk=(root,prefix='')=>readdirSync(join(root,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{assert(!e.isSymbolicLink());const p=join(prefix,e.name);return e.isDirectory()?walk(root,p):[p];});
async function digest(path){const h=createHash('sha256');let bytes=0;for await(const b of createReadStream(path)){h.update(b);bytes+=b.length;}return {bytes,sha256:h.digest('hex')};}
const previous=[];
for(const [name,commit] of [['pin-aware-readability','7647dda9ba4d94d57213fe8f7ecaf5d474c667fe'],['ownership-locality','f8b29a9904dd02c38583cb4c15d72f55b5ee6b75'],['functional-readability','c2ec8c88da11f8cf4aac498775e216ae2baaa34d'],['pcb-seed-rotated-fields','9ac563194a4a3b89d52d54b8ee38ce04a157ff58'],['native-readability-replay','446e19c9b934ad8026b4e9ec2996b47d9ef413f9']]){
 const dir=join(phase,'..',name+(name==='ownership-locality'?'-offline-v2':'-offline-v1')),old=json(join(dir,'verification.json'));
 assert.equal(execFileSync('git',['diff',commit,'--name-only','--',dir],{cwd:repo,encoding:'utf8'}),'');
 for(const f of old.source)assert.equal(sha(execFileSync('git',['show',commit+':'+f.path],{cwd:repo,maxBuffer:8*1024*1024})),f.sha256);
 const files=[];for(const path of walk(old.root))files.push({path,...await digest(join(old.root,path))});
 const inventory={file_count:files.length,total_bytes:files.reduce((n,f)=>n+f.bytes,0),sha256:sha(JSON.stringify(files))};assert.deepEqual(inventory,old.inventory);
 const archive=json(join(dir,name==='native-readability-replay'?'archive-verification.json':'archive.json')),actual=await digest(archive.archive);assert.equal(actual.bytes,archive.bytes??archive.archive_bytes);assert.equal(actual.sha256,archive.sha256??archive.archive_sha256);
 previous.push({name,commit,source_files:old.source.length,inventory,archive:actual,technical_gate_passed:old.technical_gate_passed??old.phase_gate_passed,complete_practical_benchmark_cases:0});
}
const run=path=>JSON.parse(execFileSync(process.execPath,[path],{cwd:repo,encoding:'utf8',maxBuffer:32*1024*1024}));
const preceding=run(join(phase,'../native-readability-replay-offline-v1/audit-history.mjs'));
const campaign=run(join(phase,'../offline-repair-v1/verify-preservation.mjs'));
const result={schema:'kicadai.joint-annotation-placement-history.v1',verified_utc:new Date().toISOString(),previous,preceding,campaign,provider_calls:0};
writeFileSync(join(phase,'history-authentication.json'),JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({verified:true,previous,provider_calls:0}));
