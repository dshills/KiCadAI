// Read-only terminal evidence authentication; never imports a network client.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {readFileSync,readdirSync,lstatSync,existsSync} from 'node:fs';
import {resolve,dirname,join,posix} from 'node:path';
import {fileURLToPath} from 'node:url';

const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..');
const spec='specs/ai-requirement-contract-integration/live-v1';
export const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const read=path=>JSON.parse(readFileSync(path,'utf8'));
export function safePath(path) {
  assert.equal(typeof path,'string');assert(path&&!path.startsWith('/')&&posix.normalize(path)===path&&!path.startsWith('..')&&!path.includes('\\'),'Unsafe evidence path');
  return path;
}
export function walk(root,prefix='') {
  const files=[];
  for(const name of readdirSync(join(root,prefix)).sort()) {
    const path=prefix?prefix+'/'+name:name;safePath(path);
    const st=lstatSync(join(root,path));assert(!st.isSymbolicLink(),'Symlink evidence');
    if(st.isDirectory())files.push(...walk(root,path));
    else {assert(st.isFile(),'Nonregular evidence');const bytes=readFileSync(join(root,path));files.push({path,bytes:bytes.length,sha256:sha(bytes)});}
  }
  return files;
}
export function pointer(value,path) {
  if(path==='')return value;
  assert(path.startsWith('/'),'Expected JSON pointer');
  for(const raw of path.slice(1).split('/')) {
    const key=raw.replaceAll('~1','/').replaceAll('~0','~');
    assert(value!==null&&typeof value==='object'&&Object.hasOwn(value,key),'Missing audit pointer');
    value=value[key];
  }
  return value;
}
export function streamTerminal(text) {
  const events=[];
  let terminal=null,done=false,output='',sawText=false;
  for(const frame of text.replaceAll('\r\n','\n').split('\n\n')) {
    const lines=frame.split('\n');
    const data=lines.filter(x=>x.startsWith('data:')).map(x=>x.slice(5).replace(/^ /,'')).join('\n');
    if(!data)continue;
    if(data==='[DONE]'){assert(terminal&&!done,'Unexpected DONE');done=true;continue;}
    assert(!terminal&&!done,'Post-terminal data');
    const event=JSON.parse(data);
    const declared=lines.filter(x=>x.startsWith('event:')).at(-1)?.slice(6).trim();
    if(declared&&event.type)assert.equal(declared,event.type);
    const kind=event.type||declared;assert(kind);
    const sequenced=events.length?events[0].sequence_number!==undefined:event.sequence_number!==undefined;
    assert.equal(event.sequence_number!==undefined,sequenced,'Mixed sequence numbering');
    if(sequenced)assert.equal(event.sequence_number,events.length,'Noncontiguous sequence');
    events.push(event);
    if(kind==='response.output_text.delta'){assert.equal(typeof event.delta,'string');output+=event.delta;sawText=true;}
    if(['response.completed','response.incomplete','response.failed'].includes(kind)){
      terminal=event.response;assert.equal(terminal?.status,kind.slice(9));
      if(terminal.status==='completed'&&sawText){
        const full=(terminal.output??[]).filter(x=>x.type==='message').flatMap(x=>x.content??[]).filter(x=>x.type==='output_text').map(x=>x.text).join('');
        assert.equal(output,full,'Changed terminal text');
      }
    }
  }
  assert(terminal,'No terminal stream response');
  return terminal;
}

export function authenticate(root,secret) {
  assert(secret,'Existing key needed only for a local exact-secret scan');
  const files=walk(root),inventory=read(join(root,'inventory.json'));
  assert.deepEqual(files.filter(x=>x.path!=='inventory.json'),inventory,'Outer inventory mismatch');
  const byPath=new Map(files.map(x=>[x.path,x]));
  for(const file of files){
    const bytes=readFileSync(join(root,file.path));
    assert(!bytes.includes(secret),'Credential found in '+file.path);
    assert(!/\bsk-[A-Za-z0-9_-]{20,}/.test(bytes.toString('utf8')),'Key-shaped content found in '+file.path);
  }
  const start=read(join(root,'campaign-start.json')),end=read(join(root,'campaign-end.json'));
  const freeze=read(join(root,'freeze.json')),freezeBytes=readFileSync(join(root,'freeze.json'));
  assert.deepEqual(freezeBytes,readFileSync(join(repo,spec,'freeze.json')));
  assert.equal(start.freeze_sha256,sha(freezeBytes));assert.equal(start.source_commit,freeze.source_commit);
  assert.equal(freeze.evidence_root,root);assert.equal(freeze.binary_path,'/tmp/kicadai-ai-requirement-eval-v1');
  assert.equal(start.binary_sha256,sha(readFileSync(freeze.binary_path)));
  assert(start.binary_build_info.includes('vcs.modified=false'));assert(start.binary_build_info.includes(start.binary_source_commit));
  for(const commit of [start.source_commit,start.binary_source_commit])assert(/^[0-9a-f]{40}$/.test(commit));
  const changed=execFileSync('git',['diff','--name-only',start.source_commit,start.binary_source_commit,'--','.',':(exclude)'+spec+'/freeze.json'],{cwd:repo,encoding:'utf8'}).trim();
  assert.equal(changed,'','Evaluated source differs from reviewed source');
  for(const file of freeze.files){
    safePath(file.path);
    const current=readFileSync(join(repo,file.path)),retained=readFileSync(join(root,'frozen-inputs',file.path));
    assert.deepEqual(current,retained);assert.equal(current.length,file.bytes);assert.equal(sha(current),file.sha256);
  }
  const corpus=read(join(repo,spec,'corpus.json')),preflight=read(join(repo,spec,'snapshot/preflight.json'));
  const capabilities=readFileSync(join(repo,spec,'snapshot/installed-capabilities.json'));
  const providerSchema=read(join(repo,spec,'snapshot/provider-schema.json'));
  assert.equal(sha(capabilities),preflight.capability_sha256);
  assert.equal(sha(readFileSync(join(repo,spec,'corpus.json'))),preflight.corpus_sha256);
  assert.equal(start.model,'gpt-5.6-sol');assert.equal(start.max_output_tokens,16384);
  assert.equal(start.max_requests,20);assert.equal(start.max_estimated_or_reserved_usd,25);
  assert.equal(start.provider_calls_before_start,0);assert.equal(preflight.live_requests,0);
  const ids=['I01','I02','I03','I04','R01','R02','C01','C02'];
  assert.deepEqual(corpus.cases.map(x=>x.id),ids);assert.deepEqual(end.outcomes.map(x=>x.case_id),ids);
  const receiptPaths=files.filter(x=>/^journal\/\d{3}\.reservation\.json$/.test(x.path));
  const receipts=[],legCounts=new Map();
  let spent=0,previousCase=-1;
  for(const [index,file]of receiptPaths.entries()){
    const receipt=read(join(root,file.path)),number=String(index+1).padStart(3,'0');
    assert.equal(receipt.number,index+1);assert.equal(file.path,'journal/'+number+'.reservation.json');
    const item=corpus.cases.find(x=>x.id===receipt.case_id);assert(item);
    const caseIndex=ids.indexOf(item.id);assert(caseIndex>=previousCase,'Case order changed');previousCase=caseIndex;
    assert(['initial','follow-up'].includes(receipt.leg));if(receipt.leg==='follow-up')assert.equal(item.kind,'clarification');
    const key=item.id+'/'+receipt.leg,attempt=(legCounts.get(key)??0)+1;legCounts.set(key,attempt);
    assert.equal(receipt.attempt,attempt);assert(attempt<=2);
    const prefix=key+'/http-'+number,attemptPrefix=key+'/attempt-'+attempt;
    const requestBytes=readFileSync(join(root,prefix+'.request.json')),request=JSON.parse(requestBytes);
    assert.equal(requestBytes.length,receipt.request_bytes);assert(requestBytes.length>0&&requestBytes.length<=131072);
    assert.equal(sha(requestBytes),receipt.request_sha256);assert.equal(receipt.reserved_microusd,(requestBytes.length+4096)*5+16384*20);
    assert.equal(request.model,'gpt-5.6-sol');assert.equal(request.max_output_tokens,16384);
    assert.equal(request.store,false);assert.equal(request.stream,true);assert.equal(request.background,false);
    assert.deepEqual(Object.keys(request).sort(),['background','input','instructions','max_output_tokens','model','store','stream','text'].sort());
    assert.equal(sha(Buffer.from(request.instructions)),preflight.instruction_sha256);
    assert.equal(request.text.format.type,'json_schema');assert.equal(request.text.format.strict,true);
    assert.equal(request.text.format.name,'kicadai_behavioral_intent_v1');assert.deepEqual(request.text.format.schema,providerSchema);
    const input=JSON.parse(request.input),contextBytes=readFileSync(join(root,key,'provider-context.json'));
    assert.equal(input.prompt,item.prompt);assert.equal(input.attempt,attempt);assert.equal(input.capability_context,contextBytes.toString('utf8'));
    assert.deepEqual(input.diagnostics,read(join(root,attemptPrefix+'.diagnostics.json')));
    assert((input.diagnostics??[]).length<=8);
    const context=JSON.parse(contextBytes);
    assert.equal(context.source.sha256,sha(Buffer.from(item.prompt.trim())));assert.equal(context.source.byte_length,Buffer.byteLength(item.prompt.trim()));
    assert.equal(context.capability_sha256,sha(capabilities));assert.deepEqual(context.capabilities,JSON.parse(capabilities));
    for(const statement of context.source.statements)assert.equal(Buffer.from(item.prompt.trim()).subarray(statement.start_byte,statement.end_byte).toString('utf8'),statement.text);
    if(receipt.leg==='initial')assert(!context.follow_up);
    else {
      const bound=read(join(root,item.id,'bound-answer.json')),selectedBytes=readFileSync(join(root,item.id,'initial/selected.json')),selected=JSON.parse(selectedBytes),review=read(join(root,item.id,'answer-review.json'));
      assert.equal(review.case_id,item.id);assert.equal(review.selected_sha256,sha(selectedBytes));assert.equal(review.approve_fixed_answer,true);assert(review.reason&&review.reviewer);
      assert.deepEqual(context.follow_up.input,bound);assert.deepEqual(context.follow_up.prior_proposal,selected.proposal);assert.deepEqual(context.follow_up.prior_compilation,selected.compilation);
      assert(bound.answers.length>0);for(const answer of bound.answers)assert.equal(answer.answer,item.answer);
    }
    if(attempt===2){const prior=read(join(root,key,'attempt-1.compilation.json'));assert.equal(prior.status,'invalid');assert(!prior.requirement);assert((prior.issues??[]).some(x=>x.severity==='error'));}
    const usage=read(join(root,'journal/'+number+'.usage.json'));
    assert.equal(usage.accounting_violation,false);
    let terminal=null,supplemental=null,transport='failed_or_incomplete';
    if(byPath.has(prefix+'.response.txt')){
      const payload=readFileSync(join(root,prefix+'.response.txt')),meta=read(join(root,prefix+'.response-metadata.json'));
      assert.equal(meta.retained_bytes,payload.length);assert.equal(meta.retained_sha256,sha(payload));assert.equal(meta.authorization_headers_retained,false);
      if(!meta.redacted){assert.equal(meta.captured_bytes,payload.length);assert.equal(meta.captured_sha256,sha(payload));}
      if(meta.capture_complete){
        try{terminal=meta.content_type?.includes('text/event-stream')?streamTerminal(payload.toString('utf8')):JSON.parse(payload);supplemental=terminal?.usage??null;transport='complete_capture';}catch{transport='complete_capture_invalid_payload';}
      }
    }else assert(byPath.has(prefix+'.transport-error.json'),'Missing response/transport evidence');
    let cost=receipt.reserved_microusd;
    if(usage.usage_available){
      const u={input_tokens:0,output_tokens:0,total_tokens:0,...usage.usage};assert(Number.isInteger(u.input_tokens)&&u.input_tokens>=0&&u.input_tokens<=requestBytes.length+4096);
      assert(Number.isInteger(u.output_tokens)&&u.output_tokens>=0&&u.output_tokens<=16384);
      assert.equal(u.total_tokens,u.input_tokens+u.output_tokens);assert(u.total_tokens>0);
      assert(terminal&&terminal.status==='completed');assert.equal(terminal.model,'gpt-5.6-sol');
      assert.equal(u.input_tokens,terminal.usage.input_tokens);assert.equal(u.output_tokens,terminal.usage.output_tokens);assert.equal(u.total_tokens,terminal.usage.total_tokens);
      cost=u.input_tokens*5+u.output_tokens*20;
    }
    assert.equal(usage.estimated_or_reserved_microusd,cost);spent+=cost;
    const errorExists=byPath.has(attemptPrefix+'.error.json');
    if(!errorExists){
      assert(terminal&&terminal.status==='completed','Accepted output lacks complete terminal evidence');
      const output=(terminal.output??[]).filter(x=>x.type==='message').flatMap(x=>x.content??[]).filter(x=>x.type==='output_text').map(x=>x.text);
      assert.equal(output.length,1);const envelope=JSON.parse(output[0]);assert.equal(envelope.schema,'kicadai.ai.intent.v1');
      assert.deepEqual(envelope.intent,read(join(root,attemptPrefix+'.intent.json')));
      assert(byPath.has(attemptPrefix+'.compilation.json'));
    }else assert.equal(usage.usage_available,false,'Rejected output released reservation');
    receipts.push({number:index+1,case_id:item.id,leg:receipt.leg,attempt,transport,estimated_or_reserved_microusd:cost,usage_available:usage.usage_available,supplemental_unaccepted_usage:usage.usage_available?null:supplemental});
  }
  assert(receipts.length<=20&&spent<=25_000_000);
  assert.equal(end.generation_requests,receipts.length);assert.equal(end.estimated_or_reserved_microusd,spent);assert.equal(end.estimated_or_reserved_usd,spent/1e6);
  assert.equal(end.full_board_passes,0);assert.equal(end.semantic_audits_complete,false);
  for(const [i,item]of corpus.cases.entries()){
    const outcome=end.outcomes[i];assert.equal(outcome.kind,item.kind);assert.equal(outcome.board_pass,false);
    if(outcome.status==='not_run'){assert(!existsSync(join(root,item.id)));continue;}
    assert.deepEqual(outcome,read(join(root,item.id,'result.json')));
    for(const [leg,attempts]of [['initial',outcome.initial_attempts],['follow-up',outcome.follow_up_attempts]]){
      assert(attempts>=0&&attempts<=2);
      for(let n=1;n<=attempts;n++){
        const prefix=item.id+'/'+leg+'/attempt-'+n,timing=read(join(root,prefix+'.timing.json'));
        const matching=receipts.filter(r=>r.case_id===item.id&&r.leg===leg&&r.attempt===n);
        assert.equal(matching.length,timing.reservation_created?1:0);
        if(!timing.reservation_created)assert(byPath.has(prefix+'.error.json'));
      }
    }
  }
  const resourcesWithinLimits=end.wall_seconds<=5400&&end.resources.sampling_errors===0&&end.resources.samples>0&&end.resources.peak_sampled_process_tree_rss_bytes>0&&end.resources.peak_sampled_process_tree_rss_bytes<=16*1024**3&&end.resources.peak_sampled_evidence_bytes<=1024**3&&end.outcomes.every(x=>x.wall_seconds<=1200);
  return {schema:'kicadai.interface-authentication.v1',verified_utc:new Date().toISOString(),raw_root:root,source_commit:start.source_commit,binary_source_commit:start.binary_source_commit,binary_sha256:start.binary_sha256,freeze_sha256:start.freeze_sha256,raw_inventory_sha256:sha(readFileSync(join(root,'inventory.json'))),file_count:files.length,total_bytes:files.reduce((n,x)=>n+x.bytes,0),exact_environment_credential_scan:'absent',frozen_files_verified:freeze.files.length,case_count:corpus.cases.length,clause_count:corpus.cases.reduce((n,c)=>n+c.clauses.length,0),resources_within_limits:resourcesWithinLimits,campaign_stop_reason:end.stop_reason,requests:receipts,estimated_or_reserved_usd:spent/1e6,actual_billed_usd:null,provider_calls_by_authenticator:0,semantic_faithfulness_audited_by_this_script:false};
}

if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const root=process.argv[2]??'/tmp/kicadai-ai-requirement-interface-v1';
  console.log(JSON.stringify(authenticate(root,process.env.OPENAI_API_KEY),null,2));
}
