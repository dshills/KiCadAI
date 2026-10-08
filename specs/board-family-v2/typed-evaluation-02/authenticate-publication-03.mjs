// Versioned CI-publication correction only. All live evidence and original checks stay immutable.
import assert from 'node:assert/strict';
import {hash,read,authenticateFiles} from '../evaluation/acceptance-lib.mjs';
import {E,evaluationID,checkLedger} from './acceptance.mjs';
import {verifyTerminalEvidence} from './authenticate-final.mjs';

for(const key of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS'])assert.ok(!process.env[key],'run without provider credentials');
assert.deepEqual(process.argv.slice(2),['--check']);
const original=read(`${E}/FINAL-PUBLICATION-02.json`),successor=read(`${E}/PUBLICATION-CI-03.json`);
assert.equal(original.evaluation_id,evaluationID);assert.equal(original.status,'complete-failed-evaluation-published');
assert.equal(successor.evaluation_id,evaluationID);assert.equal(successor.status,'ci-fixture-correction-only');
assert.equal(hash(`${E}/FINAL-PUBLICATION-02.json`),successor.original_publication_sha256);
authenticateFiles(E,original.files_sha256);authenticateFiles('.',successor.files_sha256);
for(const [file,sha] of Object.entries(original.prerequisites_sha256)) {
  const preserved=file==='.github/workflows/typed-intent-final-evidence.yml'?`${E}/checkpoints/publication-02-workflow.yml`:file;
  assert.equal(hash(preserved),sha,'original prerequisite changed: '+file);
}
const result=verifyTerminalEvidence();assert.deepEqual(result,original.result);
const final=read(`${E}/evidence-02/state.json`),ledger=read(`${E}/evidence-02/ledger.json`);
for(let n=1;n<=6;n++) {
  const tag=String(n).padStart(2,'0'),state=read(`${E}/checkpoints/stop-${tag}-state.json`),l=read(`${E}/checkpoints/stop-${tag}-ledger.json`);
  assert.equal(state.status,'stopped');assert.deepEqual(state.bindings,final.bindings);
  assert.deepEqual(state.records,final.records.slice(0,state.records.length));
  assert.deepEqual(checkLedger(l),ledger.entries.slice(0,state.records.length));
  assert.equal(hash(`${E}/checkpoints/stop-${tag}-ledger.json`),state.ledger_sha256);
}
console.log(JSON.stringify({...result,published_files:Object.keys(original.files_sha256).length,ci_addendum_files:Object.keys(successor.files_sha256).length,original_publication:'byte-identical; original workflow authenticated from archived copy'},null,2));
