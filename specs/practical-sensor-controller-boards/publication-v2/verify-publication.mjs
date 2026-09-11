// Cross-check published summaries without a provider call or credential.
// This supplements, not replaces, raw-tree authentication and manual review.
import assert from 'node:assert/strict';
import {existsSync,readFileSync} from 'node:fs';
import {dirname,join,resolve,sep} from 'node:path';
import {fileURLToPath} from 'node:url';
import {read,sha} from './evidence-authentication.mjs';

const spec=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const corpus=read(join(spec,'corpus.json'));
const inputs=[...corpus.cases,...corpus.paraphrases];
const results=read(join(spec,'protocol-v2-results.json'));
const auth=read(join(spec,'protocol-v2-authentication.json'));
const inventory=read(join(spec,'protocol-v2-evidence-inventory.json'));
const archive=read(join(spec,'protocol-v2-archive.json'));
const report=readFileSync(join(spec,'PROTOCOL-V2-RESULTS.md'),'utf8');
const rows=report.split('\n').filter(line=>/^\| \[(P|N|C|W)\d\d\]/.test(line));
assert.equal(rows.length,inputs.length);
assert.deepEqual(results.cases.map(c=>c.case_id),inputs.map(c=>c.id));
assert.deepEqual(auth.checkpoint.completed.map(c=>c.case_id),inputs.map(c=>c.id));

const audits=[];
for(const [index,input] of inputs.entries()) {
  const row=results.cases[index];
  const a=read(join(spec,row.audit));
  const c=auth.checkpoint.completed[index];
  audits.push(a);
  assert.equal(row.audit,`protocol-v2-audits/${input.id}.audit.json`);
  assert.equal(row.kind,input.of?'paraphrase':input.kind);
  for(const field of ['case_id','kind','disposition','first_failed_gate'])assert.equal(row[field],a[field]);
  for(const field of ['provider_attempts','follow_up_attempts','replays_completed'])assert.equal(row[field],c.result[field]);
  assert.deepEqual(a.resource_observation,c.resources);
  assert.equal(row.worker_wall_seconds,c.resources.wall_seconds);
  assert.equal(row.peak_sampled_process_tree_rss_bytes,c.resources.peak_sampled_process_tree_rss_bytes);
  assert.equal(row.retained_case_bytes,c.retained_bytes);
  assert.equal(row.retained_case_bytes,c.resources.evidence_bytes);
  assert.equal(row.replays_completed,0);
  assert.equal(c.result.manual_implementation_repairs,0);
  assert.notEqual(c.result.compilation_status,'ready');
  assert(!inventory.files.some(f=>f.path===`baseline/${input.id}/compiled-requirement.json`));
  const cells=rows[index].split('|').slice(1,-1).map(s=>s.trim());
  assert.equal(cells[0],`[${input.id}](${row.audit})`);
  assert(cells[1].startsWith(a.disposition==='passed'?'Pass —':'Fail —'));
  assert.deepEqual(cells.slice(2),[String(row.provider_attempts),row.worker_wall_seconds.toFixed(3),(row.peak_sampled_process_tree_rss_bytes/1024**2).toFixed(2),(row.retained_case_bytes/1024**2).toFixed(2)]);
  const pin=inventory.files.find(f=>f.path===a.source_binding['result.json'].file);
  assert.equal(pin.sha256,a.source_binding['result.json'].sha256);
}
for(const [kind,denominator,metric] of [
  ['positive','primary_denominator','baseline_passes'],
  ['refusal','refusal_denominator','baseline_correct_refusals'],
  ['clarification','clarification_denominator','baseline_complete_clarification_workflows'],
  ['paraphrase','paraphrase_denominator','baseline_complete_paraphrases'],
]) {
  const group=results.cases.filter(c=>c.kind===kind);
  assert.equal(results[denominator],group.length);
  assert.equal(results[metric],group.filter(c=>c.disposition==='passed').length);
}
assert.equal(results.first_attempt_complete_primary_successes,0);
assert.equal(results.corrected_complete_primary_successes,0);
assert.equal(results.first_attempt_correct_refusals,0);
assert.equal(results.corrected_correct_refusals,results.cases.filter(c=>c.kind==='refusal'&&c.disposition==='passed'&&c.provider_attempts===2).length);
assert.equal(results.fixed_clarification_answer_turns,results.cases.reduce((n,c)=>n+c.follow_up_attempts,0));
for(const field of ['manual_implementation_repairs','accepted_compiler_ready_requirements','generated_native_projects','completed_downstream_replays'])assert.equal(results[field],0);
for(const field of ['final_passes','improved','preserved','reserved_final_passes','human_active_minutes'])assert.equal(results[field],null);
assert.equal(results.baseline_campaign_complete,true);assert.equal(auth.campaign_complete,true);
assert.equal(results.evaluation_complete,false);assert.equal(results.milestone_achieved,false);
assert.equal(results.scope_decision,'requires_new_scope_before_production_changes');
assert.deepEqual(results.provider,auth.provider);
assert.equal(results.provider.new_requests,auth.checkpoint.completed.reduce((n,c)=>n+c.requests.length,0));
assert.equal(results.phase_wall_seconds,auth.resources.phase_wall_seconds);
assert.equal(results.peak_case_sampled_process_tree_rss_bytes,Math.max(...results.cases.map(c=>c.peak_sampled_process_tree_rss_bytes)));
assert.equal(results.retained_files,inventory.files.length);assert.equal(inventory.file_count,inventory.files.length);
assert.equal(inventory.total_bytes,inventory.files.reduce((n,f)=>n+f.bytes,0));
assert.equal(results.retained_evidence_bytes,inventory.total_bytes);
assert.equal(results.case_audit_count,audits.length);
assert.equal(results.case_acceptance_clause_count,audits.reduce((n,a)=>n+a.clauses.length,0));
assert.equal(results.freeze_sha256,sha(readFileSync(join(spec,'freeze-v2.json'))));
assert.equal(inventory.freeze_sha256,results.freeze_sha256);
assert.equal(inventory.source_commit,results.evaluated_source_commit);
assert.equal(archive.retained_payloads,inventory.file_count);assert.equal(archive.retained_payload_bytes,inventory.total_bytes);
assert.equal(archive.remote_publication,false);
const verifyArchive=process.argv.includes('--verify-local-archive');
if(verifyArchive) {
  const bytes=readFileSync(archive.path);
  assert.equal(bytes.length,archive.bytes);assert.equal(sha(bytes),archive.sha256);
}
let links=0;
for(const name of ['README.md','PROTOCOL-V2-RESULTS.md','PROTOCOL-V2-REVIEW.md','SCOPE-DECISION.md']) {
  const document=readFileSync(join(spec,name),'utf8');
  for(const match of document.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
    if(/^[a-z]+:|^#/.test(match[1]))continue;
    const target=resolve(spec,match[1].split('#')[0]);
    assert(target.startsWith(spec+sep));assert(existsSync(target),`Broken link in ${name}: ${match[1]}`);links++;
  }
}
console.log(JSON.stringify({verified_utc:new Date().toISOString(),cases:audits.length,clauses:results.case_acceptance_clause_count,table_rows:rows.length,local_links:links,local_archive_sha256_verified:verifyArchive,raw_evidence_authenticated_by_this_script:false,provider_calls:0},null,2));
