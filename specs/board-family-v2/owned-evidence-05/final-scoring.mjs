// Final live semantic review. The frozen raw-v4 scorer is reused unchanged.
// Only this file-backed route, after complete live evidence authentication,
// can issue final acceptance. An object with mode="live" is never sufficient.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {hash,read,durableJSON,authenticateFiles} from '../evaluation/acceptance-lib.mjs';
import {loadCorpus,corpusSHA,reviewTemplate,scoreReview} from './scoring.mjs';
import {boundedJSON,checkManifest,finalDependencies} from './final-runtime.mjs';
import {authenticateBatch} from './final-authenticate.mjs';

export function loadFinalInputs(manifestFile,batchRoot) {
  const candidate=boundedJSON(manifestFile);
  assert.equal(candidate.kind,'live-candidate','offline evidence cannot be promoted into final live results');
  const m=checkManifest(manifestFile,'live'),verified=authenticateBatch(manifestFile,batchRoot);
  assert.equal(verified.state.mode,'live');
  const original=loadCorpus();
  assert.deepEqual(m.cases,original.cases.map(({id,prompt})=>({id,prompt})));
  return {...verified,spec:{...original,evaluation_id:m.evaluation_id},contracts:Object.fromEntries(m.contracts.map(c=>[c.id,read(c.path)])),
    bindings:{...verified.bindings,corpus_sha256:corpusSHA,source_commit:m.source_commit,
      contract_sha256:Object.fromEntries(m.contracts.map(c=>[c.id,c.sha256])),
      final_files_sha256:Object.fromEntries(finalDependencies.map(f=>[path.relative(process.cwd(),f),hash(f)]))}};
}
export function prepareFinalReview(manifestFile,batchRoot,output) {
  const inputs=loadFinalInputs(manifestFile,batchRoot);
  fs.mkdirSync(output,{mode:0o700});
  const template=path.join(output,'review-template.json');
  durableJSON(template,reviewTemplate(inputs),{exclusive:true});
  const freeze={version:'owned-final-review-extension-1',status:'frozen-semantic-review-pending',mode:'live',
    manifest_path:path.resolve(manifestFile),batch_path:path.resolve(batchRoot),bindings:inputs.bindings,
    node_sha256:hash(process.execPath),template_sha256:hash(template)};
  durableJSON(path.join(output,'freeze.json'),freeze,{exclusive:true});
  return {status:freeze.status,freeze_sha256:hash(path.join(output,'freeze.json')),cases:inputs.spec.cases.length,live_requests:0};
}
export function checkFinalReview(output,reviewFile) {
  const f=boundedJSON(path.join(output,'freeze.json'));
  assert.equal(f.version,'owned-final-review-extension-1');assert.equal(f.status,'frozen-semantic-review-pending');assert.equal(f.mode,'live');
  assert.equal(f.node_sha256,hash(process.execPath));assert.equal(f.template_sha256,hash(path.join(output,'review-template.json')));
  assert.deepEqual(Object.keys(f.bindings.final_files_sha256).sort(),finalDependencies.map(x=>path.relative(process.cwd(),x)).sort());
  authenticateFiles('.',f.bindings.final_files_sha256);
  const inputs=loadFinalInputs(f.manifest_path,f.batch_path);
  assert.deepEqual(inputs.bindings,f.bindings,'frozen final evidence changed');
  assert.deepEqual(read(path.join(output,'review-template.json')),reviewTemplate(inputs));
  const score=scoreReview({...inputs,review:boundedJSON(reviewFile)});
  assert.equal(score.acceptance,'not-established-by-scoring-alone');
  return {...score,version:'owned-final-semantic-results-1',
    acceptance:score.criteria_met?'all-14-first-attempt-acceptance-met':'live-acceptance-not-met',
    evidence:{freeze_sha256:hash(path.join(output,'freeze.json')),review_sha256:hash(reviewFile),...inputs.bindings},
    limitations:'Known regression corpus, not an independent holdout or statistical estimate. Local evidence authentication, not independent provider attestation. No fabrication or bench proof.'};
}
export function recordFinalReview(output,reviewFile,destination) {
  const result=checkFinalReview(output,reviewFile);
  durableJSON(destination,result,{exclusive:true});return result;
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  try {
    const [mode,...args]=process.argv.slice(2);let result;
    if(mode==='--prepare'&&args.length===3)result=prepareFinalReview(...args);
    else if(mode==='--check'&&args.length===2)result=checkFinalReview(...args);
    else if(mode==='--record'&&args.length===3)result=recordFinalReview(...args);
    else throw new Error('usage: final-scoring.mjs --prepare MANIFEST FIXED_BATCH NEW_DIRECTORY | --check EXTENSION REVIEW | --record EXTENSION REVIEW NEW_RESULTS');
    console.log(JSON.stringify(result,null,2));
  } catch(error) {console.error(error.message);process.exitCode=1;}
}
