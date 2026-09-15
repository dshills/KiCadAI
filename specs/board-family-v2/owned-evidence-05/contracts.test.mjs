import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {offlineEnvironment,assertContract,admission,checkContracts} from './contracts.mjs';
import {compilerFiles} from './prepare-rehearsal.mjs';
import {hash,durableJSON} from '../evaluation/acceptance-lib.mjs';

function contract(prompt) {
  return {admission_version:admission,request_revision:'owned-request-05',schema_name:'board_family_owned_requirements_v4',model:'gpt-4.1-mini-2025-04-14',destination:'https://api.openai.com/v1/responses',experimental:true,max_output_tokens:1600,max_request_bytes:65536,source:{request:prompt},schema:{type:'object'},capability_context:'test-only'};
}
test('offline environment removes provider keys and all old/new process overrides without mutating the parent',()=>{
  const keys=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS','KICADAI_OWNED_PROCESS_MODE','KICADAI_OWNED_CORPUS_FAILURE','KICADAI_OWNED_TEST_BINARY','KICADAI_INDEXED_PROCESS_MODE','KICADAI_INDEXED_CORPUS_FAILURE','KICADAI_INDEXED_TEST_BINARY','KICADAI_OFFLINE_NATIVE_CLI'];
  const before=Object.fromEntries(keys.map(k=>[k,process.env[k]]));
  try {
    for(const key of keys)process.env[key]='synthetic-placeholder';
    const env=offlineEnvironment();
    for(const key of keys) {assert.equal(env[key],undefined);assert.equal(process.env[key],'synthetic-placeholder');}
  } finally {for(const key of keys) {if(before[key]===undefined)delete process.env[key];else process.env[key]=before[key];}}
});
test('owned contract retains pinned model, request revision, exact prompt and bounds',()=>{
  const prompt='Please use BMP280.',c=contract(prompt);assert.doesNotThrow(()=>assertContract(c,prompt));
  for(const [key,value] of Object.entries({admission_version:'3-indexed-quantities-experimental',request_revision:'old',schema_name:'other',model:'gpt-4.1-mini',destination:'https://example.com',experimental:false,max_output_tokens:1601,max_request_bytes:65537,source:{request:'different'},schema:null,capability_context:''}))assert.throws(()=>assertContract({...c,[key]:value},prompt),key);
});
test('one ordered regular-file contract per case is mandatory, with unchanged hashes',t=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'owned-contract-test-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const file=path.join(root,'one.json');durableJSON(file,contract('prompt'),{exclusive:true});
  const m={cases:[{id:'one',prompt:'prompt'}],contracts:[{id:'one',path:file,sha256:hash(file)}]};
  assert.doesNotThrow(()=>checkContracts(m));
  for(const changes of [{contracts:[]},{contracts:[{...m.contracts[0],id:'two'}]},{contracts:[{...m.contracts[0],sha256:'0'.repeat(64)}]},{cases:[{id:'one',prompt:'different'}]}])assert.throws(()=>checkContracts({...m,...changes}));
  const link=path.join(root,'link.json');fs.symlinkSync(file,link);
  assert.throws(()=>checkContracts({...m,contracts:[{...m.contracts[0],path:link}]}));
});
test('compiler inventory supports explicitly approved cached roots but rejects traversal and absent files',t=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'owned-source-test-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const workspace=path.join(root,'work'),cache=path.join(root,'cache');fs.mkdirSync(workspace);fs.mkdirSync(cache);
  fs.writeFileSync(path.join(workspace,'main.go'),'package main');fs.writeFileSync(path.join(cache,'runtime.go'),'package runtime');
  assert.deepEqual(compilerFiles(workspace+'|main.go\n'+cache+'|runtime.go',workspace,cache),[path.join(cache,'runtime.go'),path.join(workspace,'main.go')].sort());
  assert.throws(()=>compilerFiles(root+'|main.go',workspace,cache),/outside/);
  assert.throws(()=>compilerFiles(workspace+'|missing.go',workspace,cache));
  assert.throws(()=>compilerFiles('',workspace,cache));
});

