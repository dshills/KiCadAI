// Negative final-entry tests reuse the unchanged pure scorer for positive logic.
// Synthetic or forged live flags can never enter the final file-backed route.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {loadFinalInputs,prepareFinalReview,checkFinalReview,recordFinalReview} from './final-scoring.mjs';
import {fixture,simulatedReview} from './scoring.test.mjs';
import {scoreReview} from './scoring.mjs';
import {hash,durableJSON} from '../evaluation/acceptance-lib.mjs';
function temporary(t) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'owned-final-scoring-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));return root;
}
test('full synthetic criteria and forged live mode remain insufficient for final acceptance',t=>{
  const root=temporary(t),input=fixture();input.state.mode='live';
  const result=scoreReview({...input,review:simulatedReview(input)});
  assert.equal(result.criteria_met,true);assert.equal(result.acceptance,'not-established-by-scoring-alone');
  const file=path.join(root,'fake-manifest.json');
  durableJSON(file,{kind:'offline-test',...result});
  assert.throws(()=>loadFinalInputs(file,path.join(root,'batch')),/offline evidence cannot be promoted/);
  const output=path.join(root,'review');
  assert.throws(()=>prepareFinalReview(file,path.join(root,'batch'),output));assert.equal(fs.existsSync(output),false);
});
test('fake live manifest or extension cannot bypass source, runtime and journal authentication',t=>{
  const root=temporary(t),file=path.join(root,'fake-manifest.json'),extension=path.join(root,'extension');
  durableJSON(file,{kind:'live-candidate',version:'owned-final-runtime-1',status:'approved',mode:'live'});
  assert.throws(()=>loadFinalInputs(file,path.join(root,'batch')));
  fs.mkdirSync(extension);durableJSON(path.join(extension,'freeze.json'),{version:'owned-final-review-extension-1',status:'frozen-semantic-review-pending',mode:'live',node_sha256:'0'.repeat(64)});
  const out=path.join(root,'results.json');assert.throws(()=>recordFinalReview(extension,file,out));assert.equal(fs.existsSync(out),false);
  assert.throws(()=>checkFinalReview(extension,file));
});
test('old retained evidence and scorer remain immutable and cannot serve as final live input',{skip:!process.env.KICADAI_OWNED_REHEARSAL_ROOT},t=>{
  const root=process.env.KICADAI_OWNED_REHEARSAL_ROOT,manifest=path.join(root,'manifest.json'),result=path.join(root,'batch/result.json');
  const before=[manifest,result].map(hash),output=path.join(temporary(t),'review');
  assert.throws(()=>prepareFinalReview(manifest,path.join(root,'batch'),output),/offline evidence cannot be promoted/);
  assert.deepEqual([manifest,result].map(hash),before);assert.equal(fs.existsSync(output),false);
});
