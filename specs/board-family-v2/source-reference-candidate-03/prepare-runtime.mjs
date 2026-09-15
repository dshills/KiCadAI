// One fresh offline qualification. Importing this module performs no I/O. All
// children lose provider keys; this script never creates a live approval/batch.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {hash,read,durableJSON,offlineEnvironment,inventory,authenticateFiles,authenticateExamples,compareBundle} from '../evaluation/acceptance-lib.mjs';
import {executeRecorded,checkManifest,version} from './collector.mjs';
import {loadPlan,scoringDependencies} from './scoring.mjs';
import {qualificationVersion,requiredCommands,verifyQualification} from './runtime-qualification.mjs';

const dir='specs/board-family-v2/source-reference-candidate-03';
const toolchain='.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64';
const go=path.resolve(toolchain,'bin/go');
const cli='/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';
const fields=['GoFiles','CgoFiles','CFiles','CXXFiles','MFiles','HFiles','FFiles','SFiles','SwigFiles','SwigCXXFiles','SysoFiles','EmbedFiles'];
const testFields=['TestGoFiles','XTestGoFiles','TestEmbedFiles','XTestEmbedFiles'];
// Runtime os.ReadFile fixtures are not in Go's compiler/embed inventory.
// Bind every retained input used by the envelope/grounding regression tests.
export const capturedRegressionFiles=['prompt.txt','journal/request/body.bin','journal/response.bin','journal/selection/selection.json']
  .map(file=>`specs/board-family-v2/indexed-evaluation-03/batch/useful-01/${file}`);

export function preparationEnvironment() {
  const env={...offlineEnvironment(),PATH:`${path.dirname(go)}:${process.env.PATH}`,GOROOT:path.resolve(toolchain),GOTOOLCHAIN:'local',GOENV:'off',GOWORK:'off',GOFLAGS:'',GOEXPERIMENT:'',GOPROXY:'off',GOSUMDB:'off',GOMAXPROCS:'4',GOCACHE:path.resolve('.cache/go/build'),GOMODCACHE:path.resolve('.cache/go/mod'),GOLANGCI_LINT_CACHE:path.resolve('.cache/golangci-lint')};
  for(const key of ['KICADAI_INDEXED_TEST_BINARY','KICADAI_OFFLINE_NATIVE_CLI','KICADAI_INDEXED_CORPUS_FAILURE','KICADAI_INDEXED_PROCESS_MODE'])delete env[key];
  return env;
}

export function selectedDependencies(stdout,root=process.cwd()) {
  const selected=new Set();
  for(const line of stdout.split('\n').filter(Boolean)) {
    const [directory,...groups]=line.split('|');
    assert.ok(path.isAbsolute(directory)&&groups.length>0,'malformed compiler inventory');
    for(const name of groups.flatMap(g=>g.split(',')).filter(Boolean)) {
      const file=path.relative(root,path.join(directory,name));
      assert.ok(file&&!file.startsWith('../')&&!path.isAbsolute(file),'dependency outside workspace');
      selected.add(file);
    }
  }
  assert.ok(selected.size>0,'empty compiler inventory');return [...selected].sort();
}

export async function prepareRuntime(output) {
  assert.ok(/^\.cache\/board-family-v2\/indexed-runtime-03-[0-9]+$/.test(output??''),'usage: prepare-runtime.mjs .cache/board-family-v2/indexed-runtime-03-NN');
  assert.equal(process.platform,'darwin');assert.equal(process.arch,'arm64');
  assert.equal(fs.existsSync(output),false,'retain earlier qualification attempts');
  const env=preparationEnvironment(),planFile=`${dir}/evaluation-plan.json`,{plan,spec}=loadPlan(planFile);
  assert.equal(hash(go),'2ebc27dd4e38e9b86a9f41df0307785f4f7e2997e4be761a7b4af04b41a0de57','pinned Go compiler changed');
  // Include compiler-selected standard-library source as well as all nonstandard
  // production, assembly/embed and package-test inputs. No network lookup.
  const template=`{{.Dir}}${fields.map(f=>`|{{join .${f} ","}}`).join('')}{{if not .Standard}}${testFields.map(f=>`|{{join .${f} ","}}`).join('')}{{end}}`;
  const listed=spawnSync(go,['list','-deps','-f',template,'./cmd/kicadai-board-family'],{env,encoding:'utf8',timeout:30000,maxBuffer:4000000});
  assert.equal(listed.error,undefined);assert.equal(listed.signal,null);assert.equal(listed.status,0,listed.stderr);
  const selected=new Set([...selectedDependencies(listed.stdout),'go.mod','go.sum',`${toolchain}/bin/go`,`${toolchain}/VERSION`,
    ...['compile','asm','link','cgo'].map(f=>`${toolchain}/pkg/tool/darwin_arm64/${f}`),
    ...scoringDependencies,...capturedRegressionFiles,planFile,plan.cases_source.path,`${dir}/prepare-runtime.mjs`,`${dir}/runtime-qualification.mjs`,`${dir}/runtime-qualification.test.mjs`,`${dir}/collector.test.mjs`,`${dir}/collector.integration.test.mjs`,`${dir}/scoring.test.mjs`,`${dir}/scoring.integration.test.mjs`,`${dir}/fixtures/collector-child.mjs`,'.github/workflows/indexed-intent-evaluation.yml','.golangci.yml']);
  const sourceHashes=Object.fromEntries([...selected].sort().map(f=>[f,hash(f)]));
  const examples=authenticateExamples().receipt.cases;
  fs.mkdirSync(output,{mode:0o700});
  const receiptFile=`${output}/qualification.json`,binary=path.resolve(output,'kicadai-board-family'),testBinary=path.resolve(output,'board-family.test'),contractFile=`${output}/contract.json`;
  const q={version:qualificationVersion,status:'running-offline',evaluation_id:plan.evaluation_id,directory:output,started_utc:new Date().toISOString(),source_sha256:sourceHashes,plan_sha256:hash(planFile),compiler:{path:go,sha256:hash(go),version:'go1.26.8'},node_sha256:hash(process.execPath),kicad_cli_sha256:hash(cli),binary,commands:[],native:[],live_requests:0};
  durableJSON(receiptFile,q,{exclusive:true});
  async function command(id,executable,args,extraEnv={}) {
    assert.equal(id,requiredCommands[q.commands.length],'qualification command order changed');
    authenticateFiles('.',sourceHashes);
    const result=await executeRecorded(executable,args,{...env,...extraEnv},`${output}/${id}`,600000);
    q.commands.push({id,executable,args,result});durableJSON(receiptFile,q);
    console.log(JSON.stringify({id,exit_code:result.exit_code,wall_seconds:result.wall_seconds}));
    assert.ok(result.child_terminal_observed&&result.exit_code===0&&result.signal===null&&!result.timed_out&&!result.spawn_error&&!result.log_overflow&&!result.storage_error,`qualification failed: ${id}`);
  }
  try {
    await command('build-production',go,['build','-o',binary,'./cmd/kicadai-board-family']);q.binary_sha256=hash(binary);
    await command('build-test-helper',go,['test','-c','-o',testBinary,'./cmd/kicadai-board-family']);q.test_binary_sha256=hash(testBinary);
    await command('unit-safeguards',process.execPath,['--test',`${dir}/collector.test.mjs`,`${dir}/scoring.test.mjs`,`${dir}/runtime-qualification.test.mjs`]);
    const packages=['./internal/boardfamily','./cmd/kicadai-board-family','./internal/aiprovider'];
    await command('go-short',go,['test',...packages,'-short','-count=1']);
    await command('go-race',go,['test','-race',...packages,'-short','-count=1']);
    await command('lint','golangci-lint',['run','./...']);
    await command('go-process-integration',process.execPath,['--test',`${dir}/collector.integration.test.mjs`,`${dir}/scoring.integration.test.mjs`],{KICADAI_INDEXED_TEST_BINARY:testBinary});
    for(const family of ['bmp280','sht31']) {
      const example=examples.find(x=>x.id===`${family}-standard`);assert.ok(example);
      const target=`${output}/native-${family}`;
      await command(`native-${family}`,binary,['--config',`${example.destination}/configuration.json`,'--output',target,'--kicad-cli',cli]);
      q.native.push({id:example.id,output:target,comparison:compareBundle(target,example)});durableJSON(receiptFile,q);
    }
    await command('historical-authentication',process.execPath,['specs/board-family-v2/typed-evaluation-02/authenticate-publication-03.mjs','--check']);
    await command('export-indexed-contract',binary,['--intent-protocol','indexed-v3','--export-live-contract',contractFile]);
    const contract=read(contractFile);assert.equal(contract.admission_version,'3-indexed-quantities-experimental');assert.equal(contract.experimental,true);assert.equal(contract.model,'gpt-4.1-mini-2025-04-14');
    authenticateFiles('.',sourceHashes);authenticateExamples();
    q.contract_sha256=hash(contractFile);q.status='passed-offline-live-not-authorized';q.finished_utc=new Date().toISOString();durableJSON(receiptFile,q);
    const files={...sourceHashes};for(const f of inventory(output))files[`${output}/${f}`]=hash(`${output}/${f}`);
    const manifest={version,evaluation_id:plan.evaluation_id,status:'offline-qualified-pending-exact-source-review-ci-and-live-approval',policy:plan.proposed_policy,cases:spec.cases.map(({id,prompt})=>({id,prompt})),
      binary,binary_sha256:q.binary_sha256,prefix_args:[],node_sha256:q.node_sha256,collector_sha256:hash(`${dir}/collector.mjs`),kicad_cli:cli,kicad_cli_sha256:q.kicad_cli_sha256,qualification:'reviewed-two-family-examples',qualification_receipt_sha256:hash('specs/board-family-v2/evidence/examples-01.json'),
      case_timeout_ms:120000,total_timeout_ms:900000,evaluation_plan_sha256:q.plan_sha256,scoring_contract_path:contractFile,production_qualification:{path:receiptFile,sha256:hash(receiptFile)},runtime_files_sha256:files};
    const manifestFile=`${output}/manifest.json`;durableJSON(manifestFile,manifest,{exclusive:true});
    checkManifest(manifestFile);verifyQualification(manifest);
    console.log(JSON.stringify({status:manifest.status,manifest:manifestFile,manifest_sha256:hash(manifestFile),selected_source_files:Object.keys(sourceHashes).length,binary_sha256:manifest.binary_sha256,live_requests:0}));
    return manifestFile;
  } catch(error) {
    q.status='failed-offline';q.error=String(error.message).slice(0,2000);durableJSON(receiptFile,q);throw error;
  }
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  if(process.argv.length!==3) {console.error('usage: prepare-runtime.mjs NEW_RUNTIME_DIRECTORY');process.exitCode=1;}
  else prepareRuntime(process.argv[2]).catch(error=>{console.error(error.message);process.exitCode=1;});
}
