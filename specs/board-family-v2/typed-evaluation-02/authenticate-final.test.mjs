import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import test from 'node:test';
import {verifyTerminalEvidence} from './authenticate-final.mjs';

const source='specs/board-family-v2/typed-evaluation-02/evidence-02';
test('complete failed batch authenticates without changing its conclusion',()=>{
  const r=verifyTerminalEvidence();
  assert.equal(r.acceptance,'complete-acceptance-not-met');assert.equal(r.raw_passes,5);assert.equal(r.application_passes,7);
  assert.equal(r.native_bundles,2);assert.equal(r.compared_deliverables,81);assert.equal(r.estimated_micro_usd,17016);
});
const mutations=[
  ['changed prompt',d=>fs.appendFileSync(`${d}/useful-01.txt`,' changed')],
  ['changed selection',d=>{const p=`${d}/choice-01/selection.json`,s=JSON.parse(fs.readFileSync(p));s.decision.disposition='clarify';fs.writeFileSync(p,JSON.stringify(s));}],
  ['removed failed record',d=>{const p=`${d}/state.json`,s=JSON.parse(fs.readFileSync(p));s.records.splice(1,1);fs.writeFileSync(p,JSON.stringify(s));}],
  ['ledger reset',d=>{const p=`${d}/ledger.json`,s=JSON.parse(fs.readFileSync(p));s.entries=[];fs.writeFileSync(p,JSON.stringify(s));}],
  ['unobserved terminal child',d=>{const p=`${d}/state.json`,s=JSON.parse(fs.readFileSync(p));s.records[0].child_terminal_observed=false;fs.writeFileSync(p,JSON.stringify(s));}],
  ['changed native artifact',d=>fs.appendFileSync(`${d}/useful-01/board.kicad_pcb`,'\nchanged')],
  ['extra hidden outcome',d=>fs.writeFileSync(`${d}/extra-retry.json`,'{}')],
];
for(const [name,mutate] of mutations)test(name+' is rejected',()=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'bf2-final-evidence-')),d=path.join(root,'evidence');
  try {fs.cpSync(source,d,{recursive:true,errorOnExist:true});mutate(d);assert.throws(()=>verifyTerminalEvidence(d));}
  finally {fs.rmSync(root,{recursive:true,force:true});}
});
