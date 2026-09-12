import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import {verifyNegativePublication} from './publication-checks.mjs';

const read=name=>JSON.parse(fs.readFileSync(new URL(name,import.meta.url)));
const result=read('results.json'),audit=read('clause-audits.json');
const original=read('../practical-sensor-controller-boards/protocol-v2-results.json');
const acceptance=Object.fromEntries(audit.cases.map(c=>[c.case_id,read(`probes/${c.case_id}.acceptance.json`)]));
test('published negative result retains all cases and all sixty clauses',()=>{
 assert.deepEqual(verifyNegativePublication(result,audit,original,acceptance),{cases:16,development_clause_audits:60,milestone_achieved:false});
});
test('unrun final cannot become a scored zero or an inferred success',()=>{
 for(const value of [0,6]){const changed=structuredClone(result);changed.fresh_final.complete_positive_passes=value;assert.throws(()=>verifyNegativePublication(changed,audit,original,acceptance));}
 const changed=structuredClone(result);changed.milestone_achieved=true;assert.throws(()=>verifyNegativePublication(changed,audit,original,acceptance));
});
test('missing or weakened clause and missing gate fail closed',()=>{
 for(const alter of [a=>a.cases[0].clauses.pop(),a=>a.cases[0].clauses[0].clause='controller',a=>a.cases[0].gates.pop(),a=>a.cases[0].gates[0].pass=true]){
  const changed=structuredClone(audit);alter(changed);assert.throws(()=>verifyNegativePublication(result,changed,original,acceptance));
 }
});
test('human authored probes and unknown measurements cannot be promoted',()=>{
 for(const alter of [a=>a.cases[0].accepted_ai_requirement=true,a=>a.cases[0].clauses[0].actual=0,a=>a.cases[0].clauses[0].qualification='pass']){
  const changed=structuredClone(audit);alter(changed);assert.throws(()=>verifyNegativePublication(result,changed,original,acceptance));
 }
 const changed=structuredClone(result);changed.cases.pop();assert.throws(()=>verifyNegativePublication(changed,audit,original,acceptance));
});
test('historical scores and request ledger cannot be rewritten',()=>{
 for(const alter of [r=>r.original_baseline.complete_positive_passes=1,r=>r.cases[0].original_status='pass',r=>r.provider.historical_cumulative_requests=0,r=>r.provider.actual_billed_usd=0]){
  const changed=structuredClone(result);alter(changed);assert.throws(()=>verifyNegativePublication(changed,audit,original,acceptance));
 }
});
