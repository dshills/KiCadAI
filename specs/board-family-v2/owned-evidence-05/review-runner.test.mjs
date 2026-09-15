// Optional integration with the retained rehearsal. No transport, build or
// KiCad calls. All positive judgments here are synthetic scorer-test fixtures.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {prepareReview,authenticateExtension,checkReview,recordReview} from './review-runner.mjs';
import {assessOwnedRaw,reviewTemplate} from './scoring.mjs';
import {hash,read,durableJSON} from '../evaluation/acceptance-lib.mjs';
const root=process.env.KICADAI_OWNED_REHEARSAL_ROOT;
test('evaluator sidecar reuses exact journals/contracts/native files and refuses stale or incomplete review',{skip:!root},t=>{
  const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'owned-review-sidecar-'));t.after(()=>fs.rmSync(tmp,{recursive:true,force:true}));
  const extension=path.join(tmp,'extension'),manifest=path.join(root,'manifest.json'),batch=path.join(root,'batch');
  const before=[manifest,path.join(batch,'result.json'),path.join(batch,'useful-01/board/board.kicad_pcb')].map(hash);
  const prepared=prepareReview(manifest,batch,extension);
  assert.equal(prepared.live_requests,0);assert.equal(prepared.cases,14);assert.equal(prepared.raw_facts,48);
  const freeze=read(path.join(extension,'freeze.json'));
  assert.throws(()=>prepareReview(manifest,batch,extension));
  assert.equal(hash(path.join(extension,'freeze.json')),prepared.freeze_sha256);
  const inputs=authenticateExtension(extension);
  assert.throws(()=>checkReview(extension,path.join(extension,'review-template.json')),/source-bound-semantic-review/);
  const review=reviewTemplate(inputs);review.status='source-bound-semantic-review';review.reviewed_utc='2026-09-15T00:00:00Z';
  review.reviewer={kind:'implementing-agent',name:'SYNTHETIC TEST ONLY — no actual review claim'};
  for(const r of review.cases) {
    r.raw_complete=true;r.notes='Synthetic control for sidecar plumbing only.';
    for(const f of r.facts){f.meaning_correct=true;f.source_scope_correct=true;f.notes='Synthetic control fact.';}
    const c=inputs.spec.cases.find(c=>c.id===r.id),automatic=assessOwnedRaw(c,inputs.selections[r.id],inputs.contracts[r.id]);
    for(const [i,q] of r.requirements.entries()){q.passed=true;q.fact_indices=automatic.requirements[i].matching_fact_indices;q.notes='Synthetic control requirement.';}
    r.decision={correct:true,targeted_or_truthful:true,notes:'Synthetic control decision.'};
  }
  const file=path.join(tmp,'synthetic-review.json'),resultFile=path.join(tmp,'result.json');durableJSON(file,review,{exclusive:true});
  assert.equal(checkReview(extension,file).complete_passes,14);
  assert.equal(recordReview(extension,file,resultFile).acceptance,'offline-only-cannot-establish-live-acceptance');
  const resultSHA=hash(resultFile);assert.throws(()=>recordReview(extension,file,resultFile));assert.equal(hash(resultFile),resultSHA);
  const change=copy=>{copy.cases[0].facts[0].fact.value='humidity';};
  const changed=structuredClone(review);change(changed);durableJSON(file,changed);assert.throws(()=>checkReview(extension,file));durableJSON(file,review);
  for(const changedFreeze of [{...freeze,status:'approved-live'},{...freeze,bindings:{...freeze.bindings,scoring_files_sha256:{}}},{...freeze,template_sha256:'0'.repeat(64)}]) {
    durableJSON(path.join(extension,'freeze.json'),changedFreeze);assert.throws(()=>authenticateExtension(extension));
  }
  durableJSON(path.join(extension,'freeze.json'),freeze);
  assert.deepEqual([manifest,path.join(batch,'result.json'),path.join(batch,'useful-01/board/board.kicad_pcb')].map(hash),before);
  authenticateExtension(extension);
  t.diagnostic('Retained synthetic evidence replayed; all provider and native generation work reused, not rerun.');
});
