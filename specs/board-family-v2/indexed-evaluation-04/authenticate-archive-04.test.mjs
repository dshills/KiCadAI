import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import test from 'node:test';
import {fileURLToPath} from 'node:url';
import {authenticateArchive} from './authenticate-archive-04.mjs';

test('complete failed-acceptance archive verifies without a native runtime or key',()=>{
  const r=authenticateArchive();assert.equal(r.runtime_verified,false);assert.equal(r.physical_requests,14);assert.equal(r.complete_passes,9);assert.equal(r.estimated_micro_usd,20037);
});
test('credentials are rejected before archive access',()=>{
  const prior=process.env.OPENAI_API_KEY;
  try{process.env.OPENAI_API_KEY='offline-test-fixture-not-a-real-key';assert.throws(()=>authenticateArchive({root:'/does-not-exist'}),/credentials/);}
  finally{if(prior===undefined)delete process.env.OPENAI_API_KEY;else process.env.OPENAI_API_KEY=prior;}
});
for(const [name,file]of [
  ['review','review.json'],['results','results.json'],['approval','approval.json'],['qualification','runtime-qualification.json'],
  ['ledger','batch/ledger.json'],['raw response','batch/useful-01/journal/response.bin'],['native output','batch/useful-02/board/configuration.json'],
])test('reject changed '+name,()=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'indexed04-tamper-'));
  try{
    fs.cpSync(path.dirname(fileURLToPath(import.meta.url)),root,{recursive:true});
    assert.equal(authenticateArchive({root}).complete_passes,9,'unchanged private copy must first verify');
    assert.ok(fs.existsSync(path.join(root,file)));fs.appendFileSync(path.join(root,file),' ');
    assert.throws(()=>authenticateArchive({root}));
  }finally{fs.rmSync(root,{recursive:true});}
});
for(const action of ['missing','extra','symlink'])test('reject '+action+' batch evidence',()=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'indexed04-inventory-'));
  try{
    fs.cpSync(path.dirname(fileURLToPath(import.meta.url)),root,{recursive:true});
    assert.equal(authenticateArchive({root}).complete_passes,9);
    if(action==='missing')fs.unlinkSync(path.join(root,'batch/useful-01/audit.stdout.log'));
    if(action==='extra')fs.writeFileSync(path.join(root,'batch/injected.json'),'{}');
    if(action==='symlink')fs.symlinkSync(path.join(root,'results.json'),path.join(root,'batch/injected-link'));
    assert.throws(()=>authenticateArchive({root}));
  }finally{fs.rmSync(root,{recursive:true});}
});
