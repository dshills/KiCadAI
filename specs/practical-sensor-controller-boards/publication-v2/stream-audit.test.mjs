import assert from 'node:assert/strict';
import test from 'node:test';
import {inspectStream} from './stream-audit.mjs';
const text=JSON.stringify({schema:'synthetic',intent:{value:1}});
const fixture=()=>[
  {type:'response.created',sequence_number:0,response:{status:'in_progress'}},
  {type:'response.output_text.delta',sequence_number:1,delta:text},
  {type:'response.completed',sequence_number:2,response:{status:'completed',output:[{type:'message',content:[{type:'output_text',text}]}]}}
];
const encode=events=>Buffer.from(events.map(event=>`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join(''));
function inspect(events,extraBytes=0){const bytes=encode(events);return inspectStream(bytes,{status:200,retained_bytes:bytes.length+extraBytes});}
test('complete stream authenticates exact final text and envelope',()=>{const result=inspect(fixture());assert.deepEqual(result.envelope,{schema:'synthetic',intent:{value:1}});assert.equal(result.summary.terminal,'response.completed');});
test('sequence holes and post-terminal events fail closed',()=>{const gap=fixture();gap[1].sequence_number=9;assert.throws(()=>inspect(gap),/Noncontiguous/);const after=fixture();after.push({type:'response.output_text.delta',sequence_number:3,delta:'x'});assert.throws(()=>inspect(after),/after terminal/);});
test('unexplained byte-count mismatch fails closed',()=>assert.throws(()=>inspect(fixture(),12),/length mismatch/));
test('opaque encrypted-content redaction is disclosed without changing semantic text',()=>{const events=fixture();events[2].response.output.unshift({type:'reasoning',encrypted_content:'opaque<redacted>opaque'});const result=inspect(events,44);assert.equal(result.summary.redacted_paths.length,1);assert.deepEqual(result.envelope,{schema:'synthetic',intent:{value:1}});});
test('semantic redaction fails closed',()=>{const events=fixture();events[1].delta='<redacted>';assert.throws(()=>inspect(events),/Semantic redaction/);});
test('missing terminal remains incomplete, never a complete JSON claim',()=>{const result=inspect(fixture().slice(0,2));assert.equal(result.summary.terminal,null);assert.equal(result.envelope,null);});
test('mismatched final text fails closed',()=>{const events=fixture();events[2].response.output[0].content[0].text='{}';assert.throws(()=>inspect(events),/differs/);});
