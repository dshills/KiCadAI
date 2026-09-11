import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,writeFileSync,symlinkSync,rmSync} from 'node:fs';
import {join} from 'node:path';
import {tmpdir} from 'node:os';
import {pointer,safePath,sha,streamTerminal,walk} from './authenticate.mjs';

test('inventory seals exact regular bytes and rejects symlinks',()=>{
  const root=mkdtempSync(join(tmpdir(),'interface-auth-test-'));
  try{
    writeFileSync(join(root,'a.json'),'{}');
    assert.deepEqual(walk(root),[{path:'a.json',bytes:2,sha256:sha(Buffer.from('{}'))}]);
    symlinkSync('a.json',join(root,'link'));assert.throws(()=>walk(root));
  }finally{rmSync(root,{recursive:true,force:true});}
});
test('paths and audit pointers reject escapes or missing evidence',()=>{
  for(const p of ['/etc/passwd','../escape','a/../b','a\\b',''])assert.throws(()=>safePath(p));
  assert.equal(safePath('I01/initial/attempt-1.intent.json'),'I01/initial/attempt-1.intent.json');
  assert.equal(pointer({'a/b':{'~':3}},'/a~1b/~0'),3);
  assert.throws(()=>pointer({},'/missing'));
});
test('terminal stream verification is independent and fail closed',()=>{
  const event=(type,sequence_number,extra={})=>'event: '+type+'\ndata: '+JSON.stringify({type,sequence_number,...extra})+'\n\n';
  const response={status:'completed',output:[{type:'message',content:[{type:'output_text',text:'hello'}]}]};
  const valid=event('response.output_text.delta',0,{delta:'hello'})+event('response.completed',1,{response});
  assert.deepEqual(streamTerminal(valid),response);
  assert.deepEqual(streamTerminal(valid.replaceAll('\n','\r\n')),response);
  for(const bad of [valid+event('response.completed',2,{response}),valid.replace('"sequence_number":1','"sequence_number":3'),valid.replace('"delta":"hello"','"delta":"different"'),event('response.created',0),event('response.failed',0,{response})])assert.throws(()=>streamTerminal(bad));
});
