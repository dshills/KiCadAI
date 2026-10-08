// Offline publication/review of one stopped batch. Never invokes a provider,
// changes a ledger, repairs evidence, or authorizes another attempt.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {authenticateBatch, reviewTemplate, scoreReview} from '../source-reference-candidate-03/scoring.mjs';

const destination='specs/board-family-v2/indexed-evaluation-03';
const batch='.cache/board-family-v2/indexed-final-03';
const manifest='.cache/board-family-v2/indexed-runtime-03-01/manifest.json';
const approval='.cache/board-family-v2/indexed-final-03-approval.json';
const plan='specs/board-family-v2/source-reference-candidate-03/evaluation-plan.json';
const mode=process.argv[2];
assert.ok(['--record','--check'].includes(mode));
for(const key of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS']) assert.ok(!process.env[key],`${key} must be unset`);
const sha=b=>createHash('sha256').update(b).digest('hex');
const read=p=>JSON.parse(fs.readFileSync(p,'utf8'));
const write=(p,v)=>fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n',{flag:'wx',mode:0o600});
function files(root,prefix='') {
  return fs.readdirSync(path.join(root,prefix),{withFileTypes:true}).flatMap(e=>{
    assert.ok(!e.isSymbolicLink());const p=path.join(prefix,e.name);
    return e.isDirectory()?files(root,p):[p];
  }).sort();
}
const inputs=authenticateBatch(plan,manifest,batch,approval);
assert.equal(inputs.state.status,'stopped-no-retry');
assert.equal(inputs.state.launched_cases,1);
assert.equal(inputs.state.recorded_outcomes,0);
assert.equal(inputs.state.unattempted_case_ids.length,13);
const j=path.join(batch,'useful-01/journal');
const request=fs.readFileSync(path.join(j,'request/body.bin'));
const response=fs.readFileSync(path.join(j,'response.bin'));
const rr=read(path.join(j,'response/receipt.json'));
const s=inputs.selections['useful-01'];
assert.equal(sha(request),read(path.join(j,'request/receipt.json')).sha256);
assert.equal(sha(response),rr.sha256);
assert.deepEqual(request,Buffer.from(s.provider_evidence.request_body_base64,'base64'));
assert.deepEqual(response,Buffer.from(s.provider_evidence.response_body_base64,'base64'));
assert.equal(rr.http_status,200);assert.equal(rr.eof_observed,true);
for(const k of ['truncated','transport_error','read_error','close_error'])assert.equal(rr[k],false);
assert.equal(s.extraction_outcome,'invalid_response_evidence');
assert.equal(s.decision.configuration,null);
const ledger=read(path.join(batch,'ledger.json'));
assert.equal(ledger.entries.length,1);
assert.equal(ledger.entries[0].status,'failed_or_unknown');
assert.equal(ledger.entries[0].reserve_micro_usd,50000);
assert.deepEqual(ledger,read(path.join(j,'selection/ledger.json')));
const frames=response.toString('utf8').split(/\r?\n\r?\n/).flatMap(block=>{
  const text=block.split(/\r?\n/).filter(l=>l.startsWith('data:')).map(l=>l.slice(5).replace(/^ /,'')).join('\n');
  return text&&text!=='[DONE]'?[JSON.parse(text)]:[];
});
// Diagnostic only: JSON.parse is not a replacement for the frozen Go auditor.
function depth(value,n=0,p='$') {
  let best={depth:n,path:p};
  if(value!==null&&typeof value==='object')for(const [key,child]of Object.entries(value)){
    const found=depth(child,n+1,p+'.'+key);if(found.depth>best.depth)best=found;
  }
  return best;
}
const deep=frames.map((e,i)=>({event:i,type:e.type,...depth(e)})).filter(e=>e.depth>12);
assert.deepEqual(deep.map(e=>e.depth),[13,13,13]);
assert.deepEqual(deep.map(e=>e.type),['response.created','response.in_progress','response.completed']);
const terminal=frames.filter(e=>e.type==='response.completed');assert.equal(terminal.length,1);
const terminalText=terminal[0].response.output.flatMap(o=>o.content??[]).filter(c=>c.type==='output_text').map(c=>c.text).join('');
assert.deepEqual(JSON.parse(terminalText),s.raw_intent);
assert.deepEqual(s.raw_intent.facts,[
  {kind:'sensor',quantities:[],sources:[1],state:'required',value:'BMP280'},
  {kind:'measurement',quantities:[],sources:[1],state:'required',value:'pressure'},
  {kind:'profile',quantities:[],sources:[2],state:'required',value:'standard'},
  {kind:'feature',quantities:[],sources:[1],state:'required',value:'wireless_operation'},
]);

const review=reviewTemplate(inputs);
review.status='source-bound-semantic-review';
review.reviewer={kind:'implementing-agent',name:'Codex implementing agent; not independent review'};
review.reviewed_utc='2026-09-15T09:48:22Z';
for(const c of review.cases) {
  c.notes='Not attempted: collection stopped after useful-01 failed evidence validation. No model output, decision, or native bundle exists for this case; no correctness credit is assigned.';
  c.omitted_requirements=[];
  for(const r of c.requirements)r.notes='Not assessed: no response exists for this unattempted case.';
  c.decision.notes='Not attempted; no application outcome exists to review.';
}
const first=review.cases[0];
first.notes='The local evidence validator rejected this HTTP-200 stream, so this is diagnostic review of retained bytes, not an accepted response. Both minimum gold requirements appear, but two extra facts are unfaithful. The original stopped result and unknown-usage ledger remain unchanged.';
first.facts[0].notes='Clause 1 requests pressure sensing but never names BMP280. The extraction contract forbids inferring a named sensor fact; family inference belongs to the application.';
first.facts[1].meaning_correct=true;first.facts[1].source_scope_correct=true;
first.facts[1].notes='Clause 1 explicitly requests a pressure monitor, supporting required pressure measurement.';
first.facts[2].meaning_correct=true;first.facts[2].source_scope_correct=true;
first.facts[2].notes='Clause 2 explicitly accepts the standard profile, supporting required standard profile.';
first.facts[3].notes='Clause 1 explicitly asks for a wired monitor. It does not request wireless operation; the emitted affirmative wireless requirement is invented and contradicts the wired scope.';
first.requirements[0].passed=true;first.requirements[0].fact_indices=[1];first.requirements[0].notes='The required pressure measurement is supported by fact 1 and source clause 1.';
first.requirements[1].passed=true;first.requirements[1].fact_indices=[2];first.requirements[1].notes='The standard-profile requirement is supported by fact 2 and source clause 2.';
first.decision.notes='The application correctly withheld an unverified configuration, but its generic failure clarification did not satisfy this supported board request. No native output exists; safety behavior is not task success.';
const result=scoreReview({...inputs,review});
assert.equal(result.acceptance,'complete-acceptance-not-met');
assert.equal(result.complete_passes,0);assert.equal(result.raw_passes,0);assert.equal(result.application_passes,0);
const diagnostic={status:'offline-diagnosis-not-a-repaired-result',frames:frames.length,response_bytes:response.length,response_sha256:sha(response),request_sha256:sha(request),depth_limit_in_frozen_intent_validator:12,events_exceeding_limit:deep,raw_terminal_claim:{id:terminal[0].response.id,model:terminal[0].response.model,status:terminal[0].response.status,usage:terminal[0].response.usage},usage_notice:'Raw terminal fields are diagnostic only. Frozen evidence validation failed; no cost is settled or refunded and the ledger retains a USD 0.05 reservation.',known_semantic_defects:['Invented named BMP280 sensor fact','Invented affirmative wireless requirement for a wired monitor'],planned_cases:14,physical_requests:1,unattempted_cases:13,verified_native_bundles:0};
const archive=path.join(destination,'batch');
if(mode==='--record') {
  assert.ok(!fs.existsSync(archive));fs.mkdirSync(archive,{mode:0o700});
  for(const p of files(batch)) {
    const to=path.join(archive,p);fs.mkdirSync(path.dirname(to),{recursive:true,mode:0o700});fs.copyFileSync(path.join(batch,p),to,fs.constants.COPYFILE_EXCL);
  }
  for(const [from,name]of [[approval,'approval.json'],[manifest,'runtime-manifest.json'],['.cache/board-family-v2/indexed-runtime-03-01/qualification.json','runtime-qualification.json']])fs.copyFileSync(from,path.join(destination,name),fs.constants.COPYFILE_EXCL);
  write(path.join(destination,'review.json'),review);write(path.join(destination,'results.json'),result);write(path.join(destination,'diagnosis.json'),diagnostic);
} else {
  assert.deepEqual(read(path.join(destination,'review.json')),review);
  assert.deepEqual(read(path.join(destination,'results.json')),result);
  assert.deepEqual(read(path.join(destination,'diagnosis.json')),diagnostic);
}
assert.deepEqual(files(archive),files(batch));
for(const p of files(batch))assert.equal(sha(fs.readFileSync(path.join(archive,p))),sha(fs.readFileSync(path.join(batch,p))));
assert.equal(sha(fs.readFileSync(path.join(destination,'approval.json'))),sha(fs.readFileSync(approval)));
assert.equal(sha(fs.readFileSync(path.join(destination,'runtime-manifest.json'))),sha(fs.readFileSync(manifest)));
assert.deepEqual(authenticateBatch(plan,manifest,archive,path.join(destination,'approval.json')).bindings,inputs.bindings);
console.log(JSON.stringify({status:'stopped-batch-record-and-review-verified',batch_files:files(batch).length,planned:14,physical_requests:1,unattempted:13,complete_passes:0,verified_native_bundles:0,acceptance:result.acceptance,reserved_micro_usd:50000,live_requests_in_this_check:0}));
