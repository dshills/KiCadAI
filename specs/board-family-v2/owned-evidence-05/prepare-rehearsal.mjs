// One append-only OFFLINE rehearsal package. This is not live qualification,
// a spending approval, a semantic score, or an independent review.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {hash,read,durableJSON,inventory,authenticateFiles,authenticateExamples,assessDecision} from '../evaluation/acceptance-lib.mjs';
import {collectorDependencies,executeRecorded,collect,checkManifest,version} from './collector.mjs';
import {freezeContracts,offlineEnvironment,verifyRuntimeContracts} from './contracts.mjs';
import {authenticateBatch} from './authenticate.mjs';

const dir='specs/board-family-v2/owned-evidence-05',source='specs/board-family-v2/typed-evaluation-02/cases-02.json';
const fields=['GoFiles','CgoFiles','CFiles','CXXFiles','MFiles','HFiles','FFiles','SFiles','SwigFiles','SwigCXXFiles','SysoFiles','EmbedFiles','TestGoFiles','XTestGoFiles','TestEmbedFiles','XTestEmbedFiles'];
const requiredCommands=['build-production','build-test-helper','go-short','go-race','lint','legacy-safeguards','owned-safeguards'];

function checked(command,args,env) {
  const r=spawnSync(command,args,{env,encoding:'utf8',timeout:30000,maxBuffer:8*1024*1024});
  assert.equal(r.error,undefined);assert.equal(r.signal,null);assert.equal(r.status,0,r.stderr);return r.stdout;
}
export function sourceFiles() {
  // A broad tracked snapshot includes runtime-read fixtures, historical bytes,
  // native references and scripts that Go's compiler dependency list cannot see.
  // New checkpoint prose is explicitly excluded, never an evaluation input.
  const names=checked('git',['ls-files','--cached','--others','--exclude-standard','-z'],offlineEnvironment()).split('\0').filter(Boolean)
    .filter(f=>!f.startsWith(dir+'/')||!f.endsWith('.md'));
  assert.ok(names.length>0);assert.equal(new Set(names).size,names.length);
  for(const f of names)assert.ok(fs.lstatSync(f).isFile(),'snapshot requires regular source files');
  return names.sort();
}
export function compilerFiles(stdout,workspace,cacheRoot) {
  const files=new Set();
  for(const line of stdout.split('\n').filter(Boolean)) {
    const [directory,...groups]=line.split('|');assert.ok(path.isAbsolute(directory)&&groups.length>0);
    for(const name of groups.flatMap(g=>g.split(',')).filter(Boolean)) {
      const file=path.resolve(directory,name);
      assert.ok([workspace,cacheRoot].some(root=>file.startsWith(path.resolve(root)+path.sep)),'compiler dependency outside explicit workspace/cache roots');
      assert.ok(fs.lstatSync(file).isFile(),'compiler input must be a regular file');files.add(file);
    }
  }
  assert.ok(files.size>0);return [...files].sort();
}

export function verifyPackage(root) {
  const q=read(path.join(root,'preparation.json'));
  assert.equal(q.version,'owned-offline-preparation-1');assert.equal(q.status,'passed-offline-safeguards');
  assert.equal(q.live_requests,0);assert.deepEqual(q.commands.map(c=>c.id),requiredCommands);
  authenticateFiles('.',q.source_sha256);
  for(const [file,sha] of Object.entries(q.external_sha256))assert.equal(hash(file),sha,'preparation dependency changed');
  for(const c of q.commands) {
    assert.equal(c.result.child_terminal_observed,true);assert.equal(c.result.exit_code,0);assert.equal(c.result.signal,null);
    assert.equal(c.result.timed_out,false);assert.equal(c.result.spawn_error,null);assert.equal(c.result.log_overflow,false);assert.equal(c.result.storage_error,false);
    assert.deepEqual(read(path.join(root,c.id+'.process.json')),c.result);
    for(const [file,sha] of Object.entries(c.files_sha256))assert.equal(hash(path.join(root,file)),sha);
  }
  const manifest=path.join(root,'manifest.json'),m=checkManifest(manifest);
  assert.equal(m.preparation_sha256,hash(path.join(root,'preparation.json')));
  assert.equal(m.binary_sha256,q.test_binary_sha256);
  assert.equal(hash(q.production_binary),q.production_binary_sha256);
  assert.deepEqual(m.cases,read(source).cases.map(({id,prompt})=>({id,prompt})));
  for(const [file,sha] of Object.entries(q.source_sha256))assert.equal(m.runtime_files_sha256[file],sha);
  verifyRuntimeContracts({...m,binary:q.production_binary,binary_sha256:q.production_binary_sha256,prefix_args:[]});
  const verified=authenticateBatch(manifest,path.join(root,'batch'));
  const spec=read(source);
  assert.equal(verified.state.recorded_outcomes,14);assert.equal(verified.state.recorded_model_failures,0);
  for(const c of spec.cases)assert.equal(assessDecision(spec,c,verified.selections[c.id].decision).automatic_checks_pass,true,c.id);
  const native=verified.state.records.filter(r=>r.bundle);
  assert.equal(native.length,5);assert.equal(native.reduce((n,r)=>n+Object.keys(r.bundle.files_compared).length,0),201);
  const receipt=read(path.join(root,'rehearsal.json'));
  assert.deepEqual(receipt,{version:'owned-offline-rehearsal-1',status:'passed-synthetic-rehearsal-not-live-acceptance',manifest_sha256:hash(manifest),result_sha256:hash(path.join(root,'batch/result.json')),recorded_cases:14,native_bundles:5,compared_files:201,live_requests:0});
  return receipt;
}

export async function prepareRehearsal(output,cacheRoot) {
  assert.match(output??'',/^\.cache\/board-family-v2\/owned-rehearsal-05-[0-9]+$/);
  assert.ok(path.isAbsolute(cacheRoot??''),'pass the existing absolute Go cache root');
  assert.equal(process.platform,'darwin');assert.equal(process.arch,'arm64');
  assert.equal(fs.existsSync(output),false,'retain previous attempts');
  const toolchain=path.join(cacheRoot,'mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64'),go=path.join(toolchain,'bin/go');
  const cli='/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli',lint='/Users/dshills/Development/Go/bin/golangci-lint';
  assert.equal(hash(go),'2ebc27dd4e38e9b86a9f41df0307785f4f7e2997e4be761a7b4af04b41a0de57');
  assert.equal(hash(source),'90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867');
  const env={...offlineEnvironment(),PATH:path.dirname(go)+':'+process.env.PATH,GOROOT:toolchain,GOCACHE:path.join(cacheRoot,'build'),GOMODCACHE:path.join(cacheRoot,'mod'),GOLANGCI_LINT_CACHE:path.join(cacheRoot,'golangci-lint'),GOTOOLCHAIN:'local',GOENV:'off',GOWORK:'off',GOFLAGS:'',GOEXPERIMENT:'',GOPROXY:'off',GOSUMDB:'off',GOMAXPROCS:'4'};
  const template='{{.Dir}}'+fields.map(f=>'|{{join .'+f+' ","}}').join('');
  const compiler=compilerFiles(checked(go,['list','-deps','-test','-f',template,'./cmd/kicadai-board-family'],env),process.cwd(),cacheRoot);
  const sources=sourceFiles(),external=[...compiler.filter(f=>!f.startsWith(process.cwd()+path.sep)),go,path.join(toolchain,'VERSION'),...['compile','asm','link','cgo'].map(f=>path.join(toolchain,'pkg/tool/darwin_arm64',f)),process.execPath,cli,lint];
  const examples=authenticateExamples().receipt;
  fs.mkdirSync(path.dirname(output),{recursive:true,mode:0o700});
  fs.mkdirSync(output,{mode:0o700});
  const binary=path.resolve(output,'kicadai-board-family'),testBinary=path.resolve(output,'board-family.test');
  const q={version:'owned-offline-preparation-1',status:'running-offline',source_commit:checked('git',['rev-parse','HEAD'],env).trim(),source_sha256:Object.fromEntries(sources.map(f=>[f,hash(f)])),external_sha256:Object.fromEntries([...new Set(external)].sort().map(f=>[f,hash(f)])),production_binary:binary,commands:[],started_utc:new Date().toISOString(),live_requests:0,reviewed_examples_sha256:hash('specs/board-family-v2/evidence/examples-01.json')};
  const receiptFile=path.join(output,'preparation.json');durableJSON(receiptFile,q,{exclusive:true});
  async function command(id,executable,args,extra={}) {
    assert.equal(id,requiredCommands[q.commands.length]);authenticateFiles('.',q.source_sha256);
    const result=await executeRecorded(executable,args,{...env,...extra},path.join(output,id),600000);
    const files_sha256=Object.fromEntries(['stdout.log','stderr.log','process.json'].map(s=>[id+'.'+s,hash(path.join(output,id+'.'+s))]));
    q.commands.push({id,executable,args,result,files_sha256});durableJSON(receiptFile,q);
    console.log(JSON.stringify({id,exit_code:result.exit_code,wall_seconds:result.wall_seconds}));
    assert.ok(result.child_terminal_observed&&result.exit_code===0&&result.signal===null&&!result.timed_out&&!result.spawn_error&&!result.log_overflow&&!result.storage_error,'offline safeguard failed: '+id);
  }
  try {
    await command('build-production',go,['build','-o',binary,'./cmd/kicadai-board-family']);q.production_binary_sha256=hash(binary);
    await command('build-test-helper',go,['test','-c','-o',testBinary,'./cmd/kicadai-board-family']);q.test_binary_sha256=hash(testBinary);
    const packages=['./internal/boardfamily','./cmd/kicadai-board-family','./internal/aiprovider','./internal/runtimebudget','./internal/fabrication'];
    await command('go-short',go,['test','-short','-p=1','-count=1',...packages]);
    await command('go-race',go,['test','-race','-short','-p=1','-count=1','./internal/boardfamily','./cmd/kicadai-board-family']);
    await command('lint',lint,['run','./...']);
    await command('legacy-safeguards',process.execPath,['--test','specs/board-family-v2/source-reference-candidate-03/collector.test.mjs','specs/board-family-v2/source-reference-candidate-03/runtime-qualification.test.mjs','specs/board-family-v2/source-reference-candidate-03/scoring.test.mjs']);
    await command('owned-safeguards',process.execPath,['--test',dir+'/contracts.test.mjs',dir+'/collector.integration.test.mjs'],{KICADAI_OWNED_TEST_BINARY:testBinary});
    const cases=read(source).cases.map(({id,prompt})=>({id,prompt}));
    const contracts=freezeContracts({binary,binary_sha256:q.production_binary_sha256,prefix_args:[],cases},path.join(output,'contracts'));
    authenticateFiles('.',q.source_sha256);authenticateExamples();
    for(const [file,sha] of Object.entries(q.external_sha256))assert.equal(hash(file),sha);
    q.status='passed-offline-safeguards';q.finished_utc=new Date().toISOString();durableJSON(receiptFile,q);
    const runtimeFiles={...q.source_sha256};for(const f of inventory(output))runtimeFiles[path.join(output,f)]=hash(path.join(output,f));
    for(const f of collectorDependencies)assert.equal(runtimeFiles[path.relative(process.cwd(),f)],hash(f));
    const m={version,status:'offline-rehearsal-only',evaluation_id:'board-family-v2-owned-rehearsal-05',policy:{goal:'board-family-v2-owned-rehearsal-05',max_requests:14,max_micro_usd:1000000},cases,contracts,
      binary:testBinary,binary_sha256:q.test_binary_sha256,prefix_args:['-test.run=^TestOwnedProcessHelper$','--'],node_sha256:hash(process.execPath),collector_sha256:hash(dir+'/collector.mjs'),kicad_cli:cli,kicad_cli_sha256:hash(cli),
      qualification:'reviewed-two-family-examples',qualification_receipt_sha256:q.reviewed_examples_sha256,preparation_sha256:hash(receiptFile),case_timeout_ms:120000,total_timeout_ms:900000,runtime_files_sha256:runtimeFiles};
    const manifest=path.join(output,'manifest.json');durableJSON(manifest,m,{exclusive:true});checkManifest(manifest);
    console.log(JSON.stringify({status:'offline-package-frozen-starting-synthetic-rehearsal',source_files:sources.length,compiler_files:compiler.length,reviewed_examples:examples.cases.length,live_requests:0}));
    const result=await collect({manifestPath:manifest,output:path.join(output,'batch')});
    assert.equal(result.status,'collection-complete-semantic-review-pending');
    const receipt={version:'owned-offline-rehearsal-1',status:'passed-synthetic-rehearsal-not-live-acceptance',manifest_sha256:hash(manifest),result_sha256:hash(path.join(output,'batch/result.json')),recorded_cases:14,native_bundles:5,compared_files:201,live_requests:0};
    // Validate the complete decision/bundle result before claiming success.
    const verified=authenticateBatch(manifest,path.join(output,'batch')),spec=read(source);
    assert.equal(result.recorded_outcomes,14);assert.equal(result.recorded_model_failures,0);
    for(const c of spec.cases)assert.equal(assessDecision(spec,c,verified.selections[c.id].decision).automatic_checks_pass,true,c.id);
    assert.equal(result.records.filter(r=>r.bundle).length,5);
    assert.equal(result.records.reduce((n,r)=>n+Object.keys(r.bundle?.files_compared??{}).length,0),201);
    durableJSON(path.join(output,'rehearsal.json'),receipt,{exclusive:true});verifyPackage(output);
    console.log(JSON.stringify({...receipt,directory:output,total_wall_seconds:result.total_wall_seconds}));return receipt;
  } catch(error) {
    // Once frozen, never rewrite the preparation bound into the manifest.
    durableJSON(path.join(output,'failure.json'),{status:'failed-offline-no-retry',error:String(error.message).slice(0,2000),live_requests:0},{exclusive:true});throw error;
  }
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  const [mode,...args]=process.argv.slice(2);
  if(mode==='--check'&&args.length===1) {try {console.log(JSON.stringify(verifyPackage(args[0])));} catch(error){console.error(error.message);process.exitCode=1;}}
  else if(mode==='--prepare'&&args.length===2)prepareRehearsal(...args).catch(error=>{console.error(error.message);process.exitCode=1;});
  else {console.error('usage: prepare-rehearsal.mjs --prepare NEW_DIRECTORY EXISTING_GO_CACHE_ROOT | --check DIRECTORY');process.exitCode=1;}
}
