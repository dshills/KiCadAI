// Append-only evaluator sidecar. Reuses authenticated capture/native evidence;
// it never rebuilds the application, invokes KiCad, creates approval or calls AI.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {hash,read,durableJSON,authenticateFiles} from '../evaluation/acceptance-lib.mjs';
import {loadInputs,reviewTemplate,scoreReview,scoringDependencies} from './scoring.mjs';

const self=fileURLToPath(import.meta.url);
export function prepareReview(manifestFile,batchRoot,output) {
  const inputs=loadInputs(manifestFile,batchRoot);
  assert.equal(inputs.state.mode,'offline','this extension is for the retained rehearsal only');
  fs.mkdirSync(output,{mode:0o700}); // Exclusive; earlier review attempts survive.
  const template=path.join(output,'review-template.json');
  durableJSON(template,reviewTemplate(inputs),{exclusive:true});
  const receipt={version:'owned-evaluator-extension-1',status:'frozen-evaluator-review-pending',mode:'offline',
    manifest_path:path.resolve(manifestFile),batch_path:path.resolve(batchRoot),bindings:inputs.bindings,
    node_sha256:hash(process.execPath),template_sha256:hash(template),live_requests:0,
    evidence_reuse:'Existing owned-v4 journals, source contracts and complete native bundles; no regeneration or repair.'};
  durableJSON(path.join(output,'freeze.json'),receipt,{exclusive:true});
  return {status:receipt.status,freeze_sha256:hash(path.join(output,'freeze.json')),cases:inputs.spec.cases.length,
    raw_facts:Object.values(inputs.selections).reduce((n,s)=>n+(s.raw_intent?.facts?.length??0),0),live_requests:0};
}
export function authenticateExtension(output) {
  const f=read(path.join(output,'freeze.json'));
  assert.equal(f.version,'owned-evaluator-extension-1');assert.equal(f.status,'frozen-evaluator-review-pending');assert.equal(f.mode,'offline');assert.equal(f.live_requests,0);
  assert.equal(f.node_sha256,hash(process.execPath));
  assert.equal(f.template_sha256,hash(path.join(output,'review-template.json')));
  assert.deepEqual(Object.keys(f.bindings.scoring_files_sha256).sort(),scoringDependencies.map(x=>path.relative(process.cwd(),x)).sort());
  authenticateFiles('.',f.bindings.scoring_files_sha256);
  const inputs=loadInputs(f.manifest_path,f.batch_path);
  assert.deepEqual(inputs.bindings,f.bindings,'evaluator/source/evidence bindings changed');
  assert.equal(inputs.state.mode,'offline');
  assert.deepEqual(read(path.join(output,'review-template.json')),reviewTemplate(inputs),'frozen review context changed');
  return inputs;
}
export function checkReview(output,reviewFile) {
  return scoreReview({...authenticateExtension(output),review:read(reviewFile)});
}
export function recordReview(output,reviewFile,destination) {
  const result=checkReview(output,reviewFile);
  durableJSON(destination,{...result,evidence:{freeze_sha256:hash(path.join(output,'freeze.json')),review_sha256:hash(reviewFile)}},{exclusive:true});
  return result;
}
if(process.argv[1]&&path.resolve(process.argv[1])===self) {
  try {
    const [mode,...args]=process.argv.slice(2);let result;
    if(mode==='--prepare'&&args.length===3)result=prepareReview(...args);
    else if(mode==='--check'&&args.length===2)result=checkReview(...args);
    else if(mode==='--record'&&args.length===3)result=recordReview(...args);
    else throw new Error('usage: review-runner.mjs --prepare MANIFEST BATCH NEW_DIRECTORY | --check EXTENSION REVIEW | --record EXTENSION REVIEW NEW_RESULTS');
    console.log(JSON.stringify(result,null,2));
  } catch(error) {console.error(error.message);process.exitCode=1;}
}
