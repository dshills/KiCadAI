// Synthetic gate records only. No approval file or live authorization is created.
import test from 'node:test';
import assert from 'node:assert/strict';
import {validateGates,validateApproval,requiredWorkflows,version,evaluationID,model,endpoint} from './final-runtime.mjs';
export function gatesFixture() {
  const m={version,kind:'live-candidate',evaluation_id:evaluationID,source_commit:'c'.repeat(40),batch_directory:'/synthetic-only/fixed-batch',
    policy:{goal:evaluationID,max_requests:14,max_micro_usd:1000000}};
  const manifestSHA='a'.repeat(64),gatesSHA='b'.repeat(64),at='2026-09-15T12:00:00Z';
  const gates={version:'owned-live-release-gates-1',status:'passed',manifest_sha256:manifestSHA,source_commit:m.source_commit,
    review:{kind:'implementing-agent',name:'SYNTHETIC TEST ONLY',scope:'Synthetic test record, not actual source review',findings_open:0,reviewed_utc:'2026-09-15T10:00:00Z'},
    ci:{repository:'dshills/KiCadAI',head_sha:m.source_commit,checked_utc:'2026-09-15T10:00:00Z',
      workflows:requiredWorkflows.map((name,i)=>({name,id:i+1,head_sha:m.source_commit,status:'completed',conclusion:'success',url:'https://github.com/dshills/KiCadAI/actions/runs/'+(i+1)}))}};
  const approval={version:'owned-live-approval-1',approved:true,source:'explicit-user-message',user_message:'SYNTHETIC TEST ONLY, not actual user approval',
    evaluation_id:evaluationID,manifest_sha256:manifestSHA,release_gates_sha256:gatesSHA,batch_directory:m.batch_directory,
    model,endpoint,max_requests:14,max_micro_usd:1000000,reuse_existing_key:true,retries:false,recovery:false,
    approved_utc:'2026-09-15T11:00:00Z',expires_utc:'2026-09-16T11:00:00Z'};
  return {m,manifestSHA,gatesSHA,gates,approval,at};
}
test('matching synthetic gate/approval records exercise validation but grant no real authority',()=>{
  const x=gatesFixture();validateGates(x.m,x.manifestSHA,x.gates);validateApproval(x.m,x.manifestSHA,x.gatesSHA,x.approval,x.at);
});
for(const [name,change] of [
  ['wrong manifest',g=>g.manifest_sha256='d'.repeat(64)],
  ['wrong source commit',g=>g.source_commit='d'.repeat(40)],
  ['open review findings',g=>g.review.findings_open=1],
  ['missing reviewer',g=>g.review.name=''],
  ['incomplete review scope',g=>g.review.scope=''],
  ['wrong repository',g=>g.ci.repository='other/project'],
  ['wrong CI commit',g=>g.ci.head_sha='d'.repeat(40)],
  ['missing workflow',g=>g.ci.workflows.pop()],
  ['duplicate workflow',g=>g.ci.workflows[0]=g.ci.workflows[1]],
  ['reused run identity',g=>g.ci.workflows[0].id=g.ci.workflows[1].id],
  ['wrong workflow commit',g=>g.ci.workflows[0].head_sha='d'.repeat(40)],
  ['pending CI',g=>g.ci.workflows[0].status='in_progress'],
  ['failed CI',g=>g.ci.workflows[0].conclusion='failure'],
  ['unrelated run URL',g=>g.ci.workflows[0].url='https://example.com'],
])test('release gates reject '+name,()=>{const x=gatesFixture();change(x.gates);assert.throws(()=>validateGates(x.m,x.manifestSHA,x.gates));});
for(const [name,change] of [
  ['old approval version',a=>a.version='indexed-live-approval-1'],
  ['not approved',a=>a.approved=false],
  ['non-user authority',a=>a.source='manifest'],
  ['missing user message',a=>a.user_message=''],
  ['different evaluation',a=>a.evaluation_id='old-evaluation'],
  ['different manifest',a=>a.manifest_sha256='d'.repeat(64)],
  ['different gates',a=>a.release_gates_sha256='d'.repeat(64)],
  ['different batch directory',a=>a.batch_directory='/new-batch'],
  ['different model',a=>a.model='unapproved'],
  ['different endpoint',a=>a.endpoint='https://example.com'],
  ['larger request budget',a=>a.max_requests=15],
  ['smaller mismatched budget',a=>a.max_requests=13],
  ['larger cost budget',a=>a.max_micro_usd=2000000],
  ['new-key authority',a=>a.reuse_existing_key=false],
  ['retry authority',a=>a.retries=true],
  ['recovery authority',a=>a.recovery=true],
  ['future approval',a=>a.approved_utc='2026-09-15T13:00:00Z'],
  ['expired approval',a=>a.expires_utc='2026-09-15T11:30:00Z'],
  ['overlong approval',a=>a.expires_utc='2027-01-01T00:00:00Z'],
  ['malformed timestamp',a=>a.approved_utc='not-a-date'],
])test('approval rejects '+name,()=>{const x=gatesFixture();change(x.approval);assert.throws(()=>validateApproval(x.m,x.manifestSHA,x.gatesSHA,x.approval,x.at));});
test('test transport cannot be granted live approval',()=>{const x=gatesFixture();x.m.kind='offline-test';assert.throws(()=>validateApproval(x.m,x.manifestSHA,x.gatesSHA,x.approval,x.at));});
