import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import os from 'node:os';
import {join,dirname,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {inventory,verifyInventory,summarizeReadiness,sha} from './evidence.mjs';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../..');
test('inventories fail closed on altered, extra, missing, linked and unsafe records',t=>{
 const root=fs.mkdtempSync(join(os.tmpdir(),'completion-inventory-test-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 fs.writeFileSync(join(root,'a'),'original');const expected=inventory(root);verifyInventory(root,expected);
 fs.writeFileSync(join(root,'a'),'changed');assert.throws(()=>verifyInventory(root,expected));
 fs.writeFileSync(join(root,'a'),'original');fs.writeFileSync(join(root,'b'),'extra');assert.throws(()=>verifyInventory(root,expected));
 fs.unlinkSync(join(root,'b'));fs.unlinkSync(join(root,'a'));assert.throws(()=>verifyInventory(root,expected));
 fs.writeFileSync(join(root,'a'),'original');assert.throws(()=>verifyInventory(root,[...expected,...expected]));
 assert.throws(()=>verifyInventory(root,[{...expected[0],path:'../a'}]));
 fs.symlinkSync(join(root,'a'),join(root,'link'));assert.throws(()=>inventory(root));
});
test('exact-secret scan covers chunk boundaries',t=>{
 const root=fs.mkdtempSync(join(os.tmpdir(),'completion-secret-test-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const secret='synthetic-boundary-secret';fs.writeFileSync(join(root,'a'),Buffer.concat([Buffer.alloc(1024*1024-5,65),Buffer.from(secret)]));
 assert.throws(()=>inventory(root,secret));
});
test('full denominator is mandatory and development cannot become final success',()=>{
 const rows=Array.from({length:6},(_,i)=>['baseline-1','candidate-1'].map(run=>({id:'P0'+(i+1),run,provider_calls:0,complete_board_pass:false,exit_code:0,signal:null,peak_sampled_process_tree_rss_bytes:1,result:{status:'native_candidate_requires_replay_and_audit'}}))).flat();
 const summary=summarizeReadiness(rows);assert.equal(summary.candidate_native_candidates,6);assert.equal(summary.admission,false);assert.equal(summary.complete_positive_passes,null);assert.equal(summary.live_final_run,false);
 assert.throws(()=>summarizeReadiness(rows.slice(1)));assert.throws(()=>summarizeReadiness([...rows.slice(1),rows[1]]));
 assert.throws(()=>summarizeReadiness(rows.map((r,i)=>i===0?{...r,provider_calls:1}:r)));
});
test('frozen probes bind exact nonreserved original briefs and acceptance clauses',()=>{
 const manifest=JSON.parse(fs.readFileSync(join(phase,'development-batch-1.json'))),corpus=JSON.parse(fs.readFileSync(join(repo,'specs/practical-sensor-controller-boards/corpus.json')));
 assert.deepEqual(manifest.cases.map(c=>c.id),['P01','P02','P03','P04','P05','P06']);
 for(const c of manifest.cases){
  const original=corpus.cases.find(x=>x.id===c.id);assert.equal(original.reserved,false);
  assert.equal(fs.readFileSync(join(phase,'probes',c.id+'.txt'),'utf8'),corpus.positive_common_prompt+'\n\n'+original.prompt+'\n');
  assert.deepEqual(JSON.parse(fs.readFileSync(join(phase,'probes',c.id+'.acceptance.json'))),[...corpus.positive_common_acceptance,...original.acceptance]);
  for(const f of c.files){const b=fs.readFileSync(join(repo,f.path));assert.equal(b.length,f.bytes);assert.equal(sha(b),f.sha256);}
 }
});
