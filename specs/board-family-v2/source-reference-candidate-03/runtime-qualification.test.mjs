import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {assessQualification,verifyQualification,qualificationVersion,requiredCommands} from './runtime-qualification.mjs';
import {selectedDependencies,preparationEnvironment,prepareRuntime} from './prepare-runtime.mjs';

// Pure synthetic receipts only; no live approval, build or provider call here.
function fixture() {
  const sha='a'.repeat(64),m={evaluation_id:'test-qualification',binary:path.resolve('test-only-binary'),binary_sha256:sha,node_sha256:sha,kicad_cli_sha256:sha,evaluation_plan_sha256:sha,runtime_files_sha256:{'source.go':sha,'test-only-binary':sha}};
  const q={version:qualificationVersion,status:'passed-offline-live-not-authorized',evaluation_id:m.evaluation_id,live_requests:0,binary:m.binary,binary_sha256:sha,node_sha256:sha,kicad_cli_sha256:sha,plan_sha256:sha,
    compiler:{version:'go1.26.8',sha256:'2ebc27dd4e38e9b86a9f41df0307785f4f7e2997e4be761a7b4af04b41a0de57'},source_sha256:{'source.go':sha},commands:requiredCommands.map(id=>({id,executable:'synthetic-test-only',args:[],result:{child_terminal_observed:true,exit_code:0,signal:null,timed_out:false,spawn_error:null,log_overflow:false,storage_error:false,wall_seconds:1}})),
    native:[{id:'bmp280-standard',comparison:{files_compared:Object.fromEntries(Array.from({length:39},(_,i)=>[i,sha]))}},{id:'sht31-standard',comparison:{files_compared:Object.fromEntries(Array.from({length:42},(_,i)=>[i,sha]))}}]};
  return {m,q};
}
test('qualification receipt contract requires complete recorded offline work',()=>{const {m,q}=fixture();assert.doesNotThrow(()=>assessQualification(m,q));});
for(const [name,change] of [
  ['running qualification',q=>{q.status='running-offline';}],['wrong evaluation',q=>{q.evaluation_id='other';}],['changed compiler',q=>{q.compiler.sha256='b'.repeat(64);}],['changed version',q=>{q.compiler.version='go1.27.1';}],
  ['missing source closure',q=>{q.source_sha256={};}],['unbound source',q=>{q.source_sha256['new.go']='a'.repeat(64);}],['different binary',q=>{q.binary_sha256='b'.repeat(64);}],
  ['missing command',q=>{q.commands.pop();}],['unobserved child',q=>{q.commands[0].result.child_terminal_observed=false;}],['failed command',q=>{q.commands[0].result.exit_code=1;}],['unknown exit',q=>{q.commands[0].result.exit_code=null;}],
  ['timeout',q=>{q.commands[0].result.timed_out=true;}],['lost log',q=>{q.commands[0].result.storage_error=true;}],['overflow',q=>{q.commands[0].result.log_overflow=true;}],['partial native comparison',q=>{q.native[0].comparison.files_compared={};}],['live usage',q=>{q.live_requests=1;}],
])test(`reject ${name}`,()=>{const {m,q}=fixture();change(q);assert.throws(()=>assessQualification(m,q));});
test('filesystem verifier never accepts a missing qualification binding',()=>assert.throws(()=>verifyQualification({}),/qualification is required/));
test('compiler closure includes selected source, assembly, embed and test paths exactly once',()=>{
  const root=process.cwd(),dir=path.join(root,'internal/example');
  assert.deepEqual(selectedDependencies(`${dir}|main.go,helper.go|entry.s|fixture.json|main_test.go\n${dir}|main.go`,root),['internal/example/entry.s','internal/example/fixture.json','internal/example/helper.go','internal/example/main.go','internal/example/main_test.go']);
});
for(const value of ['', 'relative|main.go','/outside-workspace|main.go'])test(`invalid compiler inventory fails: ${value||'empty'}`,()=>assert.throws(()=>selectedDependencies(value)));
test('preparation environment strips credentials, live fixtures and auto-download settings',()=>{
  const values={OPENAI_API_KEY:'test-only',ANTHROPIC_API_KEY:'test-only',GEMINI_API_KEY:'test-only',GOOGLE_API_KEY:'test-only',KICADAI_LIVE_PROVIDER_TESTS:'1',KICADAI_INDEXED_CORPUS_FAILURE:'invalid-extraction',KICADAI_INDEXED_TEST_BINARY:'not-used',KICADAI_OFFLINE_NATIVE_CLI:'not-used'};
  const old=Object.fromEntries(Object.keys(values).map(k=>[k,process.env[k]]));Object.assign(process.env,values);
  try {const env=preparationEnvironment();for(const k of Object.keys(values))assert.equal(env[k],undefined);assert.equal(env.GOPROXY,'off');assert.equal(env.GOTOOLCHAIN,'local');assert.equal(env.GOWORK,'off');}
  finally {for(const [k,v] of Object.entries(old))if(v===undefined)delete process.env[k];else process.env[k]=v;}
});
test('unsafe or historical output paths reject before preparation',async()=>{
  for(const p of ['','/tmp/runtime','.cache/board-family-v2/typed-runtime-02-01','.cache/board-family-v2/indexed-runtime-03-../x'])await assert.rejects(prepareRuntime(p),/usage/);
});
