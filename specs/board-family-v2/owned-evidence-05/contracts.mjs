// Pure contract export/replay, never a provider call. Source-specific schemas
// must be exported from the pinned executable; a static v3 blueprint is invalid.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {hash,read,durableJSON,offlineEnvironment as baseEnvironment} from '../evaluation/acceptance-lib.mjs';

export const admission='4-owned-evidence-experimental';
export function offlineEnvironment() {
  const env=baseEnvironment();
  for(const key of ['KICADAI_OWNED_PROCESS_MODE','KICADAI_OWNED_CORPUS_FAILURE','KICADAI_OWNED_TEST_BINARY','KICADAI_INDEXED_PROCESS_MODE','KICADAI_INDEXED_CORPUS_FAILURE','KICADAI_INDEXED_TEST_BINARY','KICADAI_OFFLINE_NATIVE_CLI']) delete env[key];
  return env;
}

export function assertContract(contract,prompt) {
  assert.equal(contract.admission_version,admission);
  assert.equal(contract.request_revision,'owned-request-05');
  assert.equal(contract.schema_name,'board_family_owned_requirements_v4');
  assert.equal(contract.model,'gpt-4.1-mini-2025-04-14');
  assert.equal(contract.destination,'https://api.openai.com/v1/responses');
  assert.equal(contract.experimental,true);
  assert.equal(contract.max_output_tokens,1600);assert.equal(contract.max_request_bytes,65536);
  assert.equal(contract.source.request,prompt);
  assert.ok(contract.schema && typeof contract.schema==='object');
  assert.ok(typeof contract.capability_context==='string' && contract.capability_context.length>0);
}

export function runtimeContract(m,prompt) {
  assert.equal(hash(m.binary),m.binary_sha256,'contract executable changed');
  const temporary=fs.mkdtempSync(path.join(os.tmpdir(),'owned-contract-audit-'));
  try {
    const file=path.join(temporary,'contract.json');
    const result=spawnSync(m.binary,[...m.prefix_args,'--intent-protocol','owned-v4','--prompt',prompt,'--export-live-contract',file],{env:offlineEnvironment(),encoding:'utf8',timeout:10000,maxBuffer:1024*1024});
    assert.equal(result.error,undefined);assert.equal(result.signal,null);assert.equal(result.status,0,'offline per-request contract export failed');
    const contract=read(file);assertContract(contract,prompt);return contract;
  } finally {fs.rmSync(temporary,{recursive:true,force:true});}
}

export function freezeContracts(m,output) {
  fs.mkdirSync(output,{mode:0o700}); // Exclusive; no previous package is replaced.
  return m.cases.map(c=>{
    assert.match(c.id,/^[a-zA-Z0-9][a-zA-Z0-9-]{0,39}$/);
    const file=path.resolve(output,`${c.id}.json`);
    durableJSON(file,runtimeContract(m,c.prompt),{exclusive:true});
    return {id:c.id,path:file,sha256:hash(file)};
  });
}

export function checkContracts(m) {
  assert.deepEqual(m.contracts?.map(c=>c.id),m.cases.map(c=>c.id),'one ordered source-specific contract is required per case');
  assert.equal(new Set(m.contracts.map(c=>c.path)).size,m.cases.length);
  for(const [i,c] of m.contracts.entries()) {
    assert.ok(path.isAbsolute(c.path));assert.match(c.sha256,/^[0-9a-f]{64}$/);
    const stat=fs.lstatSync(c.path);assert.ok(stat.isFile() && stat.size<=1024*1024);
    assert.equal(hash(c.path),c.sha256,'frozen owned contract changed');
    assertContract(read(c.path),m.cases[i].prompt);
  }
}

export function verifyRuntimeContracts(m) {
  checkContracts(m);
  for(const [i,c] of m.contracts.entries()) assert.deepEqual(read(c.path),runtimeContract(m,m.cases[i].prompt),'frozen contract differs from executable');
}
