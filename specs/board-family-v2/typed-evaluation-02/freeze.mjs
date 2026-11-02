// Freeze only local definitions. Never creates runtime spending authority.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {hash,read,durableJSON,authenticateExamples} from '../evaluation/acceptance-lib.mjs';
import {E,evaluationID,policy,freezeFile,validateCases} from './acceptance.mjs';
import {authenticatePlan} from './runtime.mjs';
assert.ok(process.argv.length===3 && ['--freeze','--check'].includes(process.argv[2]),'usage: freeze.mjs --freeze | --check');
if(process.argv[2]==='--freeze') {
  assert.equal(fs.existsSync(freezeFile),false,'never overwrite a frozen evaluation');
  validateCases(read(`${E}/cases-02.json`));assert.deepEqual(read(`${E}/budget-02.json`),policy);
  assert.equal(hash(`${E}/LIVE_CONTRACT-02.json`),hash('specs/board-family-v2/typed-intent-02/verification/contract.json'),'qualified contract changed');
  authenticateExamples();
  const files=fs.readdirSync(E).filter(f=>/\.(mjs|json|md)$/.test(f) && !['freeze-02.json','runtime-02.json','APPROVAL-02.json'].includes(f)).map(f=>`${E}/${f}`);
  files.push('.github/workflows/typed-intent-evaluation.yml',
    'specs/board-family-v2/evaluation/acceptance-lib.mjs','specs/board-family-v2/evaluation/run-language.mjs',
    'specs/board-family-v2/development/replay-normalization.mjs',
    'specs/board-family-v2/typed-intent-02/publication.json','specs/board-family-v2/typed-intent-02/verification/receipt.json',
    'specs/board-family-v2/evaluation/freeze-01.json','specs/board-family-v2/evaluation/runtime-01.json',
    'specs/board-family-v2/evidence/final-01/manifest.json');
  durableJSON(freezeFile,{status:'frozen-not-spending-authority',evaluation_id:evaluationID,created_utc:new Date().toISOString(),
    live_requests:0,files_sha256:Object.fromEntries(files.sort().map(f=>[f,hash(f)]))},{exclusive:true});
}
const {freeze,spec}=authenticatePlan();
console.log(JSON.stringify({status:freeze.status,cases:spec.cases.length,files:Object.keys(freeze.files_sha256).length,freeze_sha256:hash(freezeFile),live_requests:0}));
