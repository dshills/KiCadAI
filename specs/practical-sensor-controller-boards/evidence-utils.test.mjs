import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {treeRSSFromPS,evidenceBytes,checkEnvironment,checkBuild,providerStopReason,policy} from './evidence-utils.mjs';

test('RSS includes descendants, excludes unrelated processes, and rejects malformed readings',()=>{
  assert.equal(treeRSSFromPS('10 1 20\n12 11 30\n11 10 40\n20 1 99\n',10),90*1024);
  assert.equal(treeRSSFromPS('',10),0);
  for(const text of ['10 1 NaN','10 1 -1','10 1 1 1','10 1 1\n10 1 2']) assert.throws(()=>treeRSSFromPS(text,10));
  assert.throws(()=>treeRSSFromPS('',undefined));
});
test('environment and build identities fail closed',()=>{
  const env=Object.fromEntries(['node','node_binary_sha256','platform','arch','go_version','kicad_cli','kicad_version','kicad_binary_sha256','symbols_root','footprints_root'].map(k=>[k,'synthetic']));
  env.resource_policy=policy;
  checkEnvironment(env,{...env});
  assert.throws(()=>checkEnvironment(env,{...env,kicad_version:'changed'}));
  assert.throws(()=>checkEnvironment({...env,resource_policy:{}},env));
  const metadata='worker: go1.26.8\n\tbuild\tvcs.revision=abc\n\tbuild\tvcs.modified=false\n';
  checkBuild(metadata,'abc','go1.26.8');
  assert.throws(()=>checkBuild(metadata,'def','go1.26.8'));
  assert.throws(()=>checkBuild(metadata.replace('false','true'),'abc','go1.26.8'));
});
test('retention sampling and nested provider failures',t=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'practical-supervisor-synthetic-'));
  t.after(()=>fs.rmSync(root,{recursive:true}));
  fs.writeFileSync(path.join(root,'payload'),'abc');
  assert.equal(evidenceBytes(root),3);
  fs.mkdirSync(path.join(root,'follow-up'));
  fs.writeFileSync(path.join(root,'follow-up','attempt-1.error.json'),JSON.stringify({code:'ai_provider_authentication'}));
  assert.equal(providerStopReason(root),'ai_provider_authentication');
  fs.symlinkSync(path.join(root,'payload'),path.join(root,'link'));
  assert.throws(()=>evidenceBytes(root));
});
