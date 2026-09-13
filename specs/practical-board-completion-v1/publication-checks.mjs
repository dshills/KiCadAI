import assert from 'node:assert/strict';

export const gateOrder=['requirement_interpretation','component_model_qualification','electrical_analyses','schematic_readability','placement','routing_connectivity','native_validation_writer','deterministic_replay'];

// This checks the closed negative publication, not eligibility for future success.
export function verifyNegativePublication(result,audit,original,acceptance){
 assert.equal(result.publication_outcome,'negative_readiness_closed');
 assert.equal(result.milestone_achieved,false);assert.equal(result.evaluation_complete,false);
 assert.equal(result.live_final_status,'not_run');
 for(const value of Object.values(result.fresh_final))assert.equal(value,null);
 assert.deepEqual(result.original_denominators,{positive:8,refusal:4,clarification:2,paraphrase:2});
 assert.deepEqual(result.cases.map(c=>[c.case_id,c.kind]),original.cases.map(c=>[c.case_id,c.kind]));
 for(const [i,c] of result.cases.entries()){
  assert.equal(c.fresh_final_status,'not_run');assert.equal(c.fresh_final_pass,null);
  assert.equal(c.original_status,original.cases[i].disposition);
  assert.equal(c.original_first_failed_gate,original.cases[i].first_failed_gate);
 }
 for(const [published,historical] of [['complete_positive_passes','baseline_passes'],['correct_refusals','baseline_correct_refusals'],['complete_clarifications','baseline_complete_clarification_workflows'],['complete_paraphrases','baseline_complete_paraphrases']])assert.equal(result.original_baseline[published],original[historical]);
 assert.equal(result.provider.new_requests,0);assert.equal(result.provider.new_estimated_or_reserved_usd,0);
 assert.equal(result.provider.historical_cumulative_requests,original.provider.cumulative_requests);
 assert.equal(result.provider.historical_cumulative_estimated_or_reserved_usd,original.provider.cumulative_estimated_or_reserved_usd);
 assert.equal(result.provider.actual_billed_usd,null);
 assert.equal(result.development.batches_used,1);assert.equal(result.development.batches_unused,2);
 assert.equal(result.development.candidate_search_rejections,6);assert.equal(result.development.baseline_search_rejections,5);assert.equal(result.development.baseline_protocol_incompatibilities,1);
 for(const key of ['electrical_syntheses','native_executions','deterministic_replays','demonstrated_new_complete_boards','manual_output_repairs'])assert.equal(result.development[key],0);
 assert.equal(result.review.independent_review,false);assert.equal(audit.independent_review,false);
 assert.deepEqual(audit.cases.map(c=>c.case_id),['P01','P02','P03','P04','P05','P06']);
 for(const c of audit.cases){
  assert.equal(c.accepted_ai_requirement,false);assert.equal(c.complete_board_pass,false);
  assert.equal(c.first_unqualified_gate,gateOrder[0]);
  assert.deepEqual(c.clauses.map(x=>x.clause),acceptance[c.case_id]);
  assert.equal(c.clauses.length,10);
  c.clauses.forEach((x,i)=>{assert.equal(x.clause_number,i+1);assert.equal(x.actual,null);assert.equal(x.measurement_status,'unavailable');assert.equal(x.qualification,'not_demonstrated');assert(x.evidence.length>0);});
  assert.deepEqual(c.gates.map(g=>g.gate),gateOrder);
  c.gates.forEach((g,i)=>{assert.equal(g.pass,false);assert.equal(g.status,i===0?'unqualified':'not_run');});
 }
 assert.equal(result.development.clause_audits,audit.cases.reduce((n,c)=>n+c.clauses.length,0));
 return {cases:result.cases.length,development_clause_audits:60,milestone_achieved:false};
}
