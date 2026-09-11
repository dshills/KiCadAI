// Read-only interpretation of retained SSE evidence; no provider or filesystem access.
import assert from 'node:assert/strict';
export function inspectStream(bytes, metadata) {
  assert(Buffer.isBuffer(bytes));
  assert.equal(metadata.status,200);
  const events=bytes.toString('utf8').replace(/\r\n/g,'\n').split('\n\n')
    .map(block=>block.split('\n').filter(line=>line.startsWith('data:')).map(line=>line.slice(5).trimStart()).join('\n'))
    .filter(Boolean).map(data=>JSON.parse(data));
  assert(events.length>0,'Empty stream');
  assert(events.every((event,index)=>event.sequence_number===index),'Noncontiguous event sequence');
  const redactedPaths=[];
  const visit=(value,path)=>{
    if(typeof value==='string'&&value.includes('<redacted>'))redactedPaths.push(path);
    else if(value&&typeof value==='object')for(const [key,child] of Object.entries(value))visit(child,`${path}/${key}`);
  };
  events.forEach((event,index)=>visit(event,`/events/${index}`));
  if(redactedPaths.length===0)assert.equal(metadata.retained_bytes,bytes.length,'Unexplained retained length mismatch');
  else assert(redactedPaths.every(path=>path.endsWith('/encrypted_content')),'Semantic redaction requires separate manual adjudication');
  const terminals=events.filter(event=>['response.completed','response.failed','response.incomplete','error'].includes(event.type));
  assert(terminals.length<=1,'Multiple terminal events');
  if(terminals.length)assert.equal(terminals[0],events.at(-1),'Events after terminal event');
  const response=terminals[0]?.response;
  const text=events.filter(event=>event.type==='response.output_text.delta').map(event=>event.delta).join('');
  let envelope=null;
  if(response?.status==='completed') {
    assert.equal(terminals[0].type,'response.completed');
    const finalText=response.output.filter(item=>item.type==='message').flatMap(item=>item.content).filter(part=>part.type==='output_text').map(part=>part.text).join('');
    assert.equal(finalText,text,'Final text differs from streamed deltas');
    envelope=JSON.parse(text);
  }
  return {events,text,envelope,response,summary:{http_status:200,bytes:bytes.length,metadata_pre_redaction_bytes:metadata.retained_bytes,redacted_paths:redactedPaths,events:events.length,terminal:terminals[0]?.type??null,final_status:response?.status??null}};
}
