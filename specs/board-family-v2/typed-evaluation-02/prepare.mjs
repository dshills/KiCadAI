// Bind an unchanged, already qualified binary to a new runner and frozen plan.
// No rebuild, live request, existing-runtime overwrite or blanket rerun.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {hash,read,durableJSON,offlineEnvironment,inventory,authenticateFiles} from '../evaluation/acceptance-lib.mjs';
import {executeCase} from '../evaluation/run-language.mjs';
import {E,evaluationID,runtimeFile,freezeFile,cli} from './acceptance.mjs';
import {authenticatePlan,authenticateRuntime} from './runtime.mjs';

const out=process.argv[2];
assert.ok(process.argv.length===3 && /^\.cache\/board-family-v2\/typed-runtime-02-[0-9]+$/.test(out||''),'usage: prepare.mjs .cache/board-family-v2/typed-runtime-02-NN');
assert.equal(fs.existsSync(out),false,'retain previous preparation attempts');
assert.equal(fs.existsSync(runtimeFile),false,'runtime already bound; never overwrite');
const root=process.cwd(),toolchain='.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64',go=path.resolve(toolchain,'bin/go');
const env={...offlineEnvironment(),PATH:`${path.dirname(go)}:${process.env.PATH}`,GOROOT:path.resolve(toolchain),GOTOOLCHAIN:'local',GOENV:'off',GOWORK:'off',GOFLAGS:'',GOEXPERIMENT:'',GOPROXY:'off',GOSUMDB:'off',GOMAXPROCS:'4',GOCACHE:path.resolve('.cache/go/build'),GOMODCACHE:path.resolve('.cache/go/mod')};
const {freeze}=authenticatePlan();
const qbase='specs/board-family-v2/typed-intent-02',q=read(`${qbase}/verification/receipt.json`),old=read('specs/board-family-v2/evaluation/runtime-01.json');
assert.equal(q.status,'offline-typed-intent-pass-not-live-acceptance');
assert.ok(q.commands.every(c=>c.exitCode===0 && !c.signal && !c.timedOut && !c.spawnError));
assert.equal(hash(q.binary_path),q.binary_sha256);
authenticateFiles(qbase,read(`${qbase}/publication.json`).files_sha256);
authenticateFiles('.',q.source_sha256);
for(const [file,sha] of Object.entries(q.historical_bytes_unchanged_sha256))assert.equal(hash(file),sha);

// Compiler-selected production, assembly/embed and package-test dependencies.
// Every byte must come from the existing successor qualification or the unchanged
// original complete compiler closure; new/unqualified dependencies are rejected.
const fields=['GoFiles','CgoFiles','CFiles','CXXFiles','MFiles','HFiles','FFiles','SFiles','SwigFiles','SwigCXXFiles','SysoFiles','EmbedFiles','TestGoFiles','XTestGoFiles','TestEmbedFiles','XTestEmbedFiles'];
const template=`{{if not .Standard}}{{.Dir}}${fields.map(f=>`|{{join .${f} ","}}`).join('')}{{end}}`;
const listed=spawnSync(go,['list','-deps','-f',template,'./cmd/kicadai-board-family'],{env,encoding:'utf8',timeout:30000,maxBuffer:4000000});
assert.equal(listed.status,0,listed.stderr);
const selected=new Set(['go.mod','go.sum',`${toolchain}/bin/go`]);
for(const line of listed.stdout.trim().split('\n').filter(Boolean)) {
  const [directory,...groups]=line.split('|');
  for(const name of groups.flatMap(g=>g.split(',')).filter(Boolean)) {
    const file=path.relative(root,path.join(directory,name));
    assert.ok(file && !file.startsWith('../') && !path.isAbsolute(file));selected.add(file);
  }
}
const sourceHashes={},origins={};
for(const file of [...selected].sort()) {
  const expected=q.source_sha256[file]??old.files_sha256[file];
  assert.ok(expected,`dependency has no qualification: ${file}`);
  assert.equal(hash(file),expected,`dependency changed: ${file}`);
  sourceHashes[file]=expected;origins[file]=q.source_sha256[file]?'typed-intent-02':'unchanged-original-runtime-01';
}
fs.mkdirSync(out,{mode:0o700});
const binary=`${out}/kicadai-board-family`,commands=[];
const prep={status:'running-offline',evaluation_id:evaluationID,started_utc:new Date().toISOString(),source_sha256:sourceHashes,source_qualification_origin:origins,commands};
durableJSON(`${out}/preparation.json`,prep,{exclusive:true});
async function command(id,exe,args) {
  const r=await executeCase(exe,args,env,`${out}/${id}`);commands.push({id,command:exe,args,...r});
  durableJSON(`${out}/preparation.json`,prep);console.log(JSON.stringify({id,exit_code:r.exit_code,seconds:r.wall_seconds}));
  assert.ok(r.exit_code===0 && !r.signal && !r.timed_out && !r.spawn_error,`offline preparation failed: ${id}`);
}
try {
  fs.copyFileSync(q.binary_path,binary,fs.constants.COPYFILE_EXCL);fs.chmodSync(binary,0o755);
  assert.equal(hash(binary),q.binary_sha256);
  await command('historical-authentication',process.execPath,['specs/board-family-v2/evaluation/audit-final.mjs','--check']);
  await command('qualified-source-reuse',process.execPath,[`${qbase}/check.mjs`,'--current']);
  await command('new-runner-tests',process.execPath,['--test',`${E}/acceptance.test.mjs`]);
  await command('unchanged-runner-safeguards',process.execPath,['--test','specs/board-family-v2/evaluation/acceptance.test.mjs','specs/board-family-v2/development/replay-normalization.test.mjs']);
  await command('payload',path.resolve(binary),['--export-live-contract',`${out}/executed-contract.json`]);
  assert.equal(hash(`${out}/executed-contract.json`),hash(`${E}/LIVE_CONTRACT-02.json`));
  authenticatePlan();authenticateFiles('.',sourceHashes);
  prep.status='passed-offline-no-live-requests';prep.finished_utc=new Date().toISOString();durableJSON(`${out}/preparation.json`,prep);
  const files={...sourceHashes,...freeze.files_sha256,[freezeFile]:hash(freezeFile)};
  for(const file of inventory(out))files[`${out}/${file}`]=hash(`${out}/${file}`);
  const runtime={status:'offline-qualified-live-not-authorized',evaluation_id:evaluationID,created_utc:new Date().toISOString(),
    binary,binary_sha256:hash(binary),freeze_sha256:hash(freezeFile),node_path:process.execPath,node_sha256:hash(process.execPath),
    kicad_cli:cli,kicad_cli_sha256:hash(cli),files_sha256:files,historical_bytes_sha256:q.historical_bytes_unchanged_sha256,commands,
    qualification_reuse:{receipt:`${qbase}/verification/receipt.json`,receipt_sha256:hash(`${qbase}/verification/receipt.json`),
      source_binary:q.binary_path,source_binary_sha256:q.binary_sha256,qualified_source_commit:'8913e2b6e6a013b86db7641911881e2b5ffba5f6',
      compiler_selected_files:Object.keys(sourceHashes).length,source_qualification_origin:origins,
      full_short_packages:174,native_smoke_cases:2,native_compared_deliverables:81,all_five_profiles:'reuse unchanged published five-profile native qualification',
      notice:'No production, compiler-selected nonstandard dependency or template byte changed. New evaluator files are separately frozen/tested. Standard-library identity follows the previously pinned Go toolchain identity; no new physical qualification.'},
    live_requests:0,semantic_acceptance:'not-tested'};
  durableJSON(runtimeFile,runtime,{exclusive:true});
  authenticateRuntime();
  console.log(JSON.stringify({status:runtime.status,runtime:runtimeFile,runtime_sha256:hash(runtimeFile),freeze_sha256:runtime.freeze_sha256,binary_sha256:runtime.binary_sha256,compiler_selected_files:Object.keys(sourceHashes).length,live_requests:0}));
} catch(error) {
  prep.status='failed-offline';prep.error=String(error.message).slice(0,2000);durableJSON(`${out}/preparation.json`,prep);throw error;
}
