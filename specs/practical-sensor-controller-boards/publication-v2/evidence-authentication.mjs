// Read-only checkpoint authentication. This never grades a design or calls a provider.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {existsSync,lstatSync,readFileSync,readdirSync,realpathSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {gunzipSync} from 'node:zlib';
import {inspectStream} from './stream-audit.mjs';
export const read=p=>JSON.parse(readFileSync(p,'utf8'));
export const sha=b=>createHash('sha256').update(b).digest('hex');
export function hasKeyShapedString(bytes) {
  // The native library index can exceed V8's maximum single-string length.
  // A 64-byte overlap covers the minimum ASCII credential-pattern length.
  const step=1024*1024;
  for(let offset=0;offset<bytes.length;offset+=step)if(/sk-(?:proj-)?[A-Za-z0-9_-]{20,}/.test(bytes.subarray(offset,Math.min(bytes.length,offset+step+64)).toString('utf8')))return true;
  return false;
}
export const seal=(base,path)=>{const b=readFileSync(join(base,path));return {path,bytes:b.length,sha256:sha(b)};};
export function walk(base,rel='') {return readdirSync(join(base,rel)).sort().flatMap(n=>{const p=rel?`${rel}/${n}`:n;const s=lstatSync(join(base,p));assert(!s.isSymbolicLink(),`Symlink: ${p}`);assert(s.isFile()||s.isDirectory(),`Special file: ${p}`);return s.isDirectory()?walk(base,p):[seal(base,p)];});}
export function authenticateCompletedCases(repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..'),root='/tmp/kicadai-practical-sensor-controller-public-1-protocol-v2',binary='/tmp/kicadai-practical-board-eval-protocol-v2') {
const freeze=read(join(repo,'specs/practical-sensor-controller-boards/freeze-v2.json'));
for(const f of freeze.files)assert.deepEqual(seal(repo,f.path),f);
const start=read(join(root,'baseline/campaign-start.json'));
assert.equal(start.protocol,'v2');assert.equal(start.provider_request_timeout_seconds,300);
assert.equal(start.freeze_sha256,sha(readFileSync(join(repo,'specs/practical-sensor-controller-boards/freeze-v2.json'))));
assert(start.binary_build_metadata.includes('vcs.modified=false'));
assert.equal(start.binary_sha256,sha(readFileSync(binary)));
const key=process.env.OPENAI_API_KEY;assert(key,'Existing key needed only for local exact-secret scan');
const corpus=read(join(repo,'specs/practical-sensor-controller-boards/corpus.json'));
const inputs=[...corpus.cases,...corpus.paraphrases];
const completed=[];
for(const input of inputs){
  const caseRoot=join(root,'baseline',input.id);
  if(!existsSync(join(root,'baseline',`${input.id}.resources.json`)))continue;
  const auditStarted=performance.now();
  const inventory=read(join(caseRoot,'inventory.json'));
  const files=walk(caseRoot);assert.deepEqual(files.filter(f=>f.path!=='inventory.json'),inventory);
  let redactionMarkers=0;
  for(const f of files){let b=readFileSync(join(caseRoot,f.path));assert(!b.includes(key),`Credential present: ${input.id}/${f.path}`);if(f.path.endsWith('.gz'))b=gunzipSync(b);assert(!b.includes(key),`Credential present in decoded: ${input.id}/${f.path}`);assert(!/sk-(?:proj-)?[A-Za-z0-9_-]{20,}/.test(b.toString('utf8')),`Key-shaped string: ${input.id}/${f.path}`);redactionMarkers+=(b.toString('utf8').match(/<redacted>/g)||[]).length;}
  const c=read(join(caseRoot,'case.json'));assert.equal(c.id,input.id);
  assert.equal(readFileSync(join(caseRoot,'prompt.txt'),'utf8'),c.prompt);
  const result=read(join(caseRoot,'result.json'));
  const resources=read(join(root,'baseline',`${input.id}.resources.json`));
  assert.equal(result.case_id,input.id);assert.equal(result.campaign,'baseline');
  assert.equal(result.manual_implementation_repairs,0);
  assert(result.provider_attempts<=2);assert(result.follow_up_attempts<=2);
  const requests=[];
  for(const f of files.filter(f=>/http-\d+\.request\.json$/.test(f.path))){
    const number=Number(f.path.match(/http-(\d+)/)[1]);const prefix=f.path.replace('.request.json','');
    const receipt=read(join(root,'request-journal',`${String(number).padStart(3,'0')}.reservation.json`));
    assert.equal(receipt.request_sha256,f.sha256);assert.equal(receipt.request_bytes,f.bytes);
    assert.equal(receipt.campaign,'baseline');assert([input.id,`${input.id}-answer`].includes(receipt.case_id));
    assert(f.bytes<=131072);
    const request=read(join(caseRoot,f.path));assert.equal(request.model,'gpt-5.6-sol');
    assert.equal(request.max_output_tokens,16384);assert.equal(request.stream,true);assert.equal(request.store,false);assert.equal(request.background,false);
    assert.equal(request.text.format.type,'json_schema');assert.equal(request.text.format.strict,true);assert(!request.tools||request.tools.length===0);
    const usage=read(join(root,'request-journal',`${String(number).padStart(3,'0')}.usage.json`));
    let stream=null;
    if(existsSync(join(caseRoot,`${prefix}.response.txt`))){
      const b=readFileSync(join(caseRoot,`${prefix}.response.txt`));
      const meta=read(join(caseRoot,`${prefix}.response-metadata.json`));
      if(meta.status===200){
        const inspected=inspectStream(b,meta);
        const terminalResponse=inspected.response;
        if(terminalResponse?.status==='completed'){
          assert.equal(terminalResponse.model,'gpt-5.6-sol');
          assert(terminalResponse.usage.output_tokens<=16384);
          const relativeDir=f.path.includes('/')?f.path.slice(0,f.path.lastIndexOf('/')+1):'';
          const attempt=requests.filter(r=>r.case_id===receipt.case_id).length+1;
          if(usage.usage_available){
            for(const k of ['input_tokens','output_tokens','total_tokens'])assert.equal(terminalResponse.usage[k],usage.usage[k]);
            assert(Math.abs(usage.estimated_or_reserved_usd-(usage.usage.input_tokens*4+usage.usage.output_tokens*20)/1e6)<1e-10);
            assert.deepEqual(inspected.envelope.intent,read(join(caseRoot,`${relativeDir}attempt-${attempt}.intent.txt`)));
            const attemptMeta=read(join(caseRoot,`${relativeDir}attempt-${attempt}.metadata.json`));
            assert.equal(attemptMeta.response_id,terminalResponse.id);
            assert.equal(attemptMeta.model,terminalResponse.model);
            assert.deepEqual(attemptMeta.usage,usage.usage);
          }else{
            // The recorder retains up to 8 MiB before the production client's 2 MiB decoder cap.
            // Do not repair the frozen receipt or turn its rejected output into an accepted input.
            const error=read(join(caseRoot,`${relativeDir}attempt-${attempt}.error.json`));
            assert.equal(error.code,'ai_output_json_invalid');
            assert.equal(error.error,'OpenAI response exceeds 2097152-byte limit');
            assert(meta.retained_bytes>2097152);
            assert.equal(usage.estimated_or_reserved_usd,receipt.reserved_usd);
            assert(!existsSync(join(caseRoot,`${relativeDir}attempt-${attempt}.intent.txt`)));
          }
        }
        stream={...inspected.summary,usage_available_in_frozen_journal:usage.usage_available,raw_stream_usage:terminalResponse?.usage??null,client_rejected_complete_response:terminalResponse?.status==='completed'&&!usage.usage_available};
      }else stream={http_status:meta.status,bytes:b.length};
    }
    requests.push({number,case_id:receipt.case_id,request_sha256:f.sha256,estimated_or_reserved_usd:usage.estimated_or_reserved_usd,stream});
  }
  const timings=files.filter(f=>/attempt-\d+\.timing\.json$/.test(f.path)).map(f=>read(join(caseRoot,f.path)));
  assert.equal(timings.length,result.provider_attempts+result.follow_up_attempts);
  assert.equal(requests.length,timings.filter(t=>t.reservation_created).length);
  let issues=null;if(existsSync(join(caseRoot,'compilation.json'))){const compilation=read(join(caseRoot,'compilation.json'));issues={};for(const x of compilation.issues??[])issues[x.code]=(issues[x.code]??0)+1;}
  completed.push({case_id:input.id,result,resources,retained_files:files.length,retained_bytes:files.reduce((n,f)=>n+f.bytes,0),inventory_sha256:sha(readFileSync(join(caseRoot,'inventory.json'))),redaction_markers:redactionMarkers,requests,issue_counts:issues,automated_authentication_elapsed_seconds:(performance.now()-auditStarted)/1000});
}
return {observed_utc:new Date().toISOString(),checkpoint_only:true,semantic_audit_not_performed_by_this_script:true,completed};
}
if(process.argv[1]&&realpathSync(fileURLToPath(import.meta.url))===realpathSync(resolve(process.argv[1])))console.log(JSON.stringify(authenticateCompletedCases(),null,2));
