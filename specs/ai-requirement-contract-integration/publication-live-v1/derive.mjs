// Reproducible post-run derivation. No network access; raw evidence is read-only.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {resolve,dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {sha,pointer,walk} from '../live-v1/authenticate.mjs';
const publication=dirname(fileURLToPath(import.meta.url));
const read=p=>JSON.parse(readFileSync(p,'utf8'));
export function derive(root='/tmp/kicadai-ai-requirement-interface-v1') {
 const corpusPath=join(publication,'../live-v1/corpus.json');
 const corpus=read(corpusPath),manual=read(join(publication,'manual-review.json'));
 const auth=read(join(publication,'authentication.json')),replay=read(join(publication,'supplementary-replay.json'));
 const end=read(join(root,'campaign-end.json'));
 const inventory=read(join(root,'inventory.json'));
 assert.deepEqual(walk(root).filter(f=>f.path!=='inventory.json'),inventory);
 assert.equal(sha(readFileSync(join(root,'inventory.json'))),auth.raw_inventory_sha256);
 assert.equal(replay.provider_calls,0);assert.equal(replay.production_compiler_modified,false);
 assert.equal(replay.sealed_replay_passed,false);assert.equal(replay.replayed_attempts.length,15);
 assert.equal(replay.tampered_answer_controls.length,8);
 const files=new Map(inventory.map(x=>[x.path,x]));
 const evidence=(path,p)=>{
  const f=files.get(path);assert(f,'Uninventoried evidence '+path);
  const value=pointer(read(join(root,path)),p);
  return {path,sha256:f.sha256,pointer:p,value_sha256:sha(Buffer.from(JSON.stringify(value)))};
 };
 const cases=corpus.cases.map((c,i)=>{
  const review=manual[c.id],outcome=end.outcomes[i];assert(review);assert.equal(c.id,outcome.case_id);assert.equal(review.clauses.length,c.clauses.length);
  const selected=read(join(root,review.selected));
  const clauses=review.clauses.map((a,index)=>{
   assert(['pass','fail','not_run'].includes(a.status));assert(a.reason);
   const ev=[...(a.pointers??[]).map(p=>evidence(review.selected,p)),...(a.evidence??[]).map(x=>evidence(x.path,x.pointer))];
   assert(ev.length>0,'Clause lacks exact evidence');
   const result={number:index+1,text:c.clauses[index],status:a.status,reason:a.reason,evidence:ev};
   if(a.requires_replay){
    const controls=replay.tampered_answer_controls.filter(x=>x.case_id===c.id);
    assert.equal(controls.length,4);assert(controls.every(x=>x.rejected===true));
    result.supplementary_replay={path:'supplementary-replay.json',sha256:sha(readFileSync(join(publication,'supplementary-replay.json'))),case_id:c.id,controls_rejected:4,sealed_replay_passed:false};
   }
   return result;
  });
  const correctStatus=c.kind==='ready'?outcome.initial_status==='ready':
   c.kind==='refusal'?outcome.initial_status==='unsupported':
   outcome.initial_status==='needs_clarification'&&outcome.follow_up_status==='ready';
  const faithful=correctStatus&&clauses.every(x=>x.status==='pass');
  const cost=auth.requests.filter(r=>r.case_id===c.id).reduce((n,r)=>n+r.estimated_or_reserved_microusd,0);
  const replayed=replay.replayed_attempts.filter(r=>r.case_id===c.id);
  assert.equal(replayed.length,outcome.initial_attempts+outcome.follow_up_attempts);
  return {case_id:c.id,title:c.title,kind:c.kind,source_sha256:sha(Buffer.from(c.prompt.trim())),
   corpus_sha256:sha(readFileSync(corpusPath)),selected:review.selected,selected_sha256:files.get(review.selected).sha256,
   first_attempt_status:outcome.first_attempt_status,initial_final_status:outcome.initial_status,follow_up_final_status:outcome.follow_up_status,
   initial_attempts:outcome.initial_attempts,follow_up_attempts:outcome.follow_up_attempts,requests:replayed.length,
   estimated_or_reserved_microusd:cost,estimated_or_reserved_usd:cost/1e6,
   clause_passes:clauses.filter(x=>x.status==='pass').length,clause_count:clauses.length,
   observed_faithful_workflow:faithful,first_attempt_each_leg_faithful:faithful&&outcome.initial_attempts===1&&(c.kind!=='clarification'||outcome.follow_up_attempts===1),
   compiler_status_appropriate:correctStatus,
   final_issues:selected.compilation.issues??[],
   evidence_authentication_passed:true,supplementary_offline_compilation_matched:true,
   sealed_replay_passed:false,board_pass:false,clauses,note:review.note};
 });
 const groups=Object.fromEntries(['ready','refusal','clarification'].map(kind=>{
  const group=cases.filter(c=>c.kind===kind);
  return [kind,{observed_faithful:group.filter(c=>c.observed_faithful_workflow).length,denominator:group.length,first_attempt_each_leg_faithful:group.filter(c=>c.first_attempt_each_leg_faithful).length}];
 }));
 assert.deepEqual(groups,{ready:{observed_faithful:1,denominator:4,first_attempt_each_leg_faithful:0},refusal:{observed_faithful:2,denominator:2,first_attempt_each_leg_faithful:2},clarification:{observed_faithful:1,denominator:2,first_attempt_each_leg_faithful:1}});
 const summary={schema:'kicadai.interface-results.v1',evaluation:'public-frozen interface v1',started_utc:end.started_utc,ended_utc:end.ended_utc,
  source_commit:auth.source_commit,binary_source_commit:auth.binary_source_commit,freeze_sha256:auth.freeze_sha256,raw_inventory_sha256:auth.raw_inventory_sha256,
  groups,gate_passed:false,gate_reasons:['Ready gate 1/4 versus required 4/4','Clarification completion 1/2 versus required 2/2','I04 compiler-accepted semantic counterexample','Sealed offline replay failed; supplementary post-run audit is not a sealed replay pass'],
  requests:end.generation_requests,estimated_or_reserved_usd:end.estimated_or_reserved_usd,actual_billed_usd:null,
  clauses_total:cases.reduce((n,c)=>n+c.clause_count,0),clauses_passed:cases.reduce((n,c)=>n+c.clause_passes,0),
  exclusions:0,not_run:0,raw_files:auth.file_count,raw_bytes:auth.total_bytes,frozen_files:auth.frozen_files_verified,
  authentication_passed:true,sealed_replay_passed:false,supplementary_replayed_attempts:15,tampered_bindings_rejected:8,
  resources_within_sampled_limits:auth.resources_within_limits,resources:end.resources,wall_seconds:end.wall_seconds,
  full_board_passes:0,practical_board_goal_achieved:false,historical_results_modified:false,
  cases:cases.map(({clauses,final_issues,...c})=>c)};
 return {audits:{schema:'kicadai.interface-clause-audits.v1',reviewer:'Codex local source-bound semantic review',independent_reviewer:false,case_count:8,clause_count:57,raw_inventory_sha256:auth.raw_inventory_sha256,cases},summary};
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url))console.log(JSON.stringify(derive(process.argv[2]),null,2));
