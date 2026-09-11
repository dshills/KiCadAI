import {fileURLToPath} from 'node:url';
// Publication authentication, read-only and live-disabled. Requires a terminal campaign.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {existsSync,readFileSync,readdirSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {gunzipSync} from 'node:zlib';
import {authenticateCompletedCases,read,sha,seal,walk,hasKeyShapedString} from './evidence-authentication.mjs';
const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..');
const spec=join(repo,'specs/practical-sensor-controller-boards');
const root='/tmp/kicadai-practical-sensor-controller-public-1-protocol-v2';
assert(existsSync(join(root,'baseline/campaign-end.json')),'Campaign is not terminal; do not snapshot a live tree');
assert(!existsSync(join(root,'active-campaign.lock')),'Campaign lock remains active');
const started=performance.now();
const auth=read(join(spec,'protocol-v2-authorization.json'));
const start=read(join(root,'baseline/campaign-start.json'));
const end=read(join(root,'baseline/campaign-end.json'));
const corpus=read(join(spec,'corpus.json'));
const inputs=[...corpus.cases,...corpus.paraphrases];
assert.deepEqual(end.outcomes.map(x=>x.id),inputs.map(x=>x.id));
assert.equal(start.source_commit,'91cce8324a5eaa4d79008e4580dc91c92c0a2d37');
assert.deepEqual(readFileSync(join(root,'protocol-authorization.json')),readFileSync(join(spec,'protocol-v2-authorization.json')));
assert.deepEqual(readFileSync(join(root,'freeze-v2.json')),readFileSync(join(spec,'freeze-v2.json')));
const checkpoint=authenticateCompletedCases(repo,root);
assert.deepEqual(checkpoint.completed.map(x=>x.case_id),inputs.map(x=>x.id),'Every terminal case needs authenticated resource and result records');
for(const item of checkpoint.completed)assert.deepEqual(item.resources,end.outcomes.find(x=>x.id===item.case_id));
const files=walk(root);
const publishedInventory=join(spec,'protocol-v2-evidence-inventory.json');
if(existsSync(publishedInventory)){
  const published=read(publishedInventory);
  assert.deepEqual(files,published.files,'Raw tree differs from the published outer inventory');
  assert.equal(published.source_commit,start.source_commit);assert.equal(published.freeze_sha256,start.freeze_sha256);
}
const key=process.env.OPENAI_API_KEY;assert(key,'Existing key needed only for local exact-secret scan');
for(const file of files){
  let bytes=readFileSync(join(root,file.path));assert(!bytes.includes(key),`Credential present: ${file.path}`);
  if(file.path.endsWith('.gz'))bytes=gunzipSync(bytes,{maxOutputLength:2**31-1});
  assert(!bytes.includes(key),`Credential present in decoded: ${file.path}`);
  assert(!hasKeyShapedString(bytes),`Key-shaped string: ${file.path}`);
}
for(const name of auth.journal_policy.copy_exactly_once)assert.deepEqual(readFileSync(join(root,'request-journal',name)),readFileSync(join(auth.prior_canonical_journal,name)));
const journalFiles=readdirSync(join(root,'request-journal')).sort();
const numbers=journalFiles.filter(n=>n.endsWith('.reservation.json')).map(n=>Number(n.split('.')[0]));
assert.deepEqual(numbers,Array.from({length:numbers.length},(_,i)=>i+1));
assert.deepEqual(journalFiles,numbers.flatMap(n=>{const p=String(n).padStart(3,'0');return [`${p}.reservation.json`,`${p}.usage.json`];}));
const entries=numbers.map(n=>{const p=String(n).padStart(3,'0');const reservation=read(join(root,'request-journal',`${p}.reservation.json`));const usage=read(join(root,'request-journal',`${p}.usage.json`));assert.equal(reservation.number,n);assert.equal(reservation.campaign,'baseline');return {reservation,usage};});
const v2Requests=checkpoint.completed.flatMap(c=>c.requests).sort((a,b)=>a.number-b.number);
assert.deepEqual(v2Requests.map(x=>x.number),numbers.filter(n=>n>=4));
const cumulativeCost=entries.reduce((n,e)=>n+e.usage.estimated_or_reserved_usd,0);
const v2Cost=entries.slice(3).reduce((n,e)=>n+e.usage.estimated_or_reserved_usd,0);
assert(numbers.length<=36);assert(cumulativeCost<=50);
const freeze=read(join(spec,'freeze-v2.json'));
const git=(...args)=>execFileSync('git',args,{cwd:repo,encoding:'utf8'}).trim();
assert.equal(git('diff','--name-only',freeze.baseline_commit,start.source_commit,'--','.',':(exclude)internal/practicalboardeval/**',':(exclude)cmd/practical-board-eval/**',':(exclude)specs/practical-sensor-controller-boards/**'),'');
const diff=execFileSync('git',['diff',freeze.baseline_commit,start.source_commit,'--','.'],{cwd:repo});
assert.equal(sha(diff),start.source_diff_sha256);
const environment=read(join(spec,'environment.json'));
const snapshot=read(join(root,'baseline/environment-snapshot/snapshot.json'));
for(const [k,v]of Object.entries(environment.baseline_snapshot))assert.equal(snapshot[k],v);
assert.equal(sha(readFileSync(join(root,'baseline/environment-snapshot/closed-loop-policy.json'))),environment.closed_loop_policy_sha256);
assert.equal(sha(readFileSync(join(root,'baseline/environment-snapshot/installed-capabilities.json'))),snapshot.capabilities_sha256);
const prior=[];
for(const name of ['baseline-evidence-inventory.json','recovery-1-evidence-inventory.json','recovery-2-evidence-inventory.json']){
  const inventory=read(join(spec,name));assert.deepEqual(walk(inventory.evidence_root),inventory.files);prior.push({inventory:name,sha256:sha(readFileSync(join(spec,name))),file_count:inventory.files.length});
}
const caseBudgetCompliance=checkpoint.completed.map(item=>({case_id:item.case_id,worker_exit_successful:item.resources.exit_code===0,within_sampled_supervisor_limits:item.resources.signal===null&&item.resources.stop_reason===null&&item.resources.sampling_errors===0&&Number.isFinite(item.resources.peak_sampled_process_tree_rss_bytes)&&item.resources.peak_sampled_process_tree_rss_bytes>0&&item.resources.peak_sampled_process_tree_rss_bytes<=16*1024**3&&item.resources.wall_seconds<=1200}));
const totalBytes=files.reduce((n,f)=>n+f.bytes,0);
const complete=end.stop_reason===null&&end.outcomes.every(x=>x.status!=='not_run');
const inventory={schema:'kicadai.practical-board-evidence-inventory.v1',evidence_root:root,source_commit:start.source_commit,freeze_sha256:start.freeze_sha256,phase:'baseline',cohort:'approved_protocol_v2_baseline',status:complete?'completed':'interrupted',authenticated_utc:new Date().toISOString(),exact_environment_credential_scan:'absent_from_all_retained_files_including_decompressed_library_index',file_count:files.length,total_bytes:totalBytes,files};
console.log(JSON.stringify({inventory,authentication:{prior_inventories_reverified:prior,campaign_complete:complete,manual_semantic_audits_required:true,checkpoint,provider:{carried_requests:3,new_requests:v2Requests.length,cumulative_requests:numbers.length,new_estimated_or_reserved_usd:v2Cost,cumulative_estimated_or_reserved_usd:cumulativeCost,remaining_baseline_requests:36-numbers.length,remaining_total_requests:72-numbers.length,remaining_estimated_or_reserved_usd:50-cumulativeCost,actual_billed_usd:null,unknown_usage_requests:entries.filter(e=>!e.usage.usage_available).map(e=>e.reservation.number)},resources:{phase_wall_seconds:end.wall_seconds,case_budget_compliance:caseBudgetCompliance,phase_wall_within_cap:end.wall_seconds<=7200,evidence_within_cap:totalBytes<=10*1024**3,raw_evidence_bytes:totalBytes,sampled_rss_is_lower_bound_on_instantaneous_peak:true,build_preflight_review_costs_separate:true},automated_authentication_elapsed_seconds:(performance.now()-started)/1000}},null,2));
