import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import test from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {relativeFile,parseBatch,authenticate,publication} from './authenticate-indexed-03.mjs';

test('snapshot paths cannot escape or inject Git requests',()=>{
  for(const p of ['../secret','/etc/passwd','a/../b','a\nb','a\0b','a\\b','','.'])assert.throws(()=>relativeFile(p));
  assert.equal(relativeFile('.cache/go/mod/a@v1/file.go'),'.cache/go/mod/a@v1/file.go');
});
const frame=b=>Buffer.concat([Buffer.from('a'.repeat(40)+' blob '+b.length+'\n'),b,Buffer.from('\n')]);
test('binary Git objects are hashed by exact size, not newlines',()=>{
  const bytes=Buffer.from([0,10,255,10]);
  assert.deepEqual(parseBatch(frame(bytes),['a']),{a:createHash('sha256').update(bytes).digest('hex')});
});
for(const [name,data,names]of [
  ['missing',Buffer.from('a missing\n'),['a']],
  ['wrong type',Buffer.from('a'.repeat(40)+' tree 0\n\n'),['a']],
  ['truncated',frame(Buffer.from('abc')).subarray(0,-2),['a']],
  ['trailing',Buffer.concat([frame(Buffer.from('a')),Buffer.from('x')]),['a']],
  ['wrong terminator',Buffer.concat([frame(Buffer.from('a')).subarray(0,-1),Buffer.from('x')]),['a']],
  ['duplicate',Buffer.concat([frame(Buffer.from('a')),frame(Buffer.from('b'))]),['a','a']],
  ['unbounded',Buffer.from('a'.repeat(40)+' blob 99999999999\n'),['a']],
])test('reject '+name,()=>assert.throws(()=>parseBatch(data,names)));

test('archive verifies independently of changed development source',()=>{
  const r=authenticate();assert.equal(r.git_source_files,1155);
  assert.equal(r.runtime_verified,false);assert.equal(r.complete_passes,0);
});
test('historical checks reject provider credentials before doing work',()=>{
  const previous=process.env.OPENAI_API_KEY;
  try {process.env.OPENAI_API_KEY='offline-test-only';assert.throws(()=>authenticate(),/without provider credentials/);}
  finally {if(previous===undefined)delete process.env.OPENAI_API_KEY;else process.env.OPENAI_API_KEY=previous;}
});
for(const mode of ['changed-review','changed-body','missing-file','extra-file'])test('publication rejects '+mode,()=>{
  const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../..');
  const cache=path.join(repo,'.cache');fs.mkdirSync(cache,{recursive:true});
  const scratch=fs.mkdtempSync(path.join(cache,'history-tamper-'));
  try {
    const archive=path.join(scratch,publication);fs.mkdirSync(path.dirname(archive),{recursive:true});
    fs.cpSync(path.join(repo,publication),archive,{recursive:true,errorOnExist:true,force:false});
    assert.equal(authenticate({repo:scratch}).complete_passes,0,'unchanged scratch copy must first verify');
    if(mode==='changed-review')fs.appendFileSync(path.join(archive,'review.json'),' ');
    if(mode==='changed-body')fs.appendFileSync(path.join(archive,'batch/useful-01/journal/response.bin'),' ');
    if(mode==='missing-file')fs.unlinkSync(path.join(archive,'batch/useful-01/audit.stdout.log'));
    if(mode==='extra-file')fs.writeFileSync(path.join(archive,'injected.json'),'{}');
    assert.throws(()=>authenticate({repo:scratch}));
  } finally {fs.rmSync(scratch,{recursive:true});}
});
