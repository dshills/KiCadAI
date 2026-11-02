// Offline only: no credential inheritance, provider client, or live mode.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {executeRecorded,cleanTerminal} from '../source-reference-candidate-03/collector.mjs';
import {read,hash,durableJSON,inventory,authenticateFiles} from '../evaluation/acceptance-lib.mjs';
const [mode,baseline,output]=process.argv.slice(2);
assert.ok(['--run','--check'].includes(mode));assert.ok(path.isAbsolute(baseline)&&path.isAbsolute(output));
const source=process.cwd(),base='16a925bcdd6cc1db3341d1e5cb2d027d1d088a5f';
const cleanEnv={HOME:process.env.HOME,PATH:'/opt/homebrew/bin:/usr/bin:/bin',TMPDIR:'/private/tmp'};
function git(args){const r=spawnSync('git',args,{env:cleanEnv,encoding:'utf8'});assert.equal(r.error,undefined);assert.equal(r.status,0,r.stderr);return r.stdout;}
assert.equal(git(['rev-parse','HEAD']).trim(),base);
const sourceInventory=()=>[...new Set(git(['ls-files','--cached','--others','--exclude-standard','-z']).split('\0').filter(Boolean))].sort();
const original=spawnSync(process.execPath,['.cache/partitioned-08-live-review-01/close.mjs','--check'],{cwd:baseline,env:cleanEnv,encoding:'utf8',timeout:30000});
assert.equal(original.error,undefined);assert.equal(original.status,0,original.stderr);
const closure=JSON.parse(original.stdout);assert.equal(closure.status,'evaluation-closed-acceptance-failed');assert.equal(closure.complete_cases,6);
const corpus=read('specs/board-family-v2/typed-evaluation-02/cases-02.json');assert.equal(corpus.cases.length,14);
const externalNames=['.cache/partitioned-08-live-review-01/closure-freeze.json',...corpus.cases.map(c=>`.cache/partitioned-08-production-02/batch/${c.id}/journal/request/body.bin`)];
const external=Object.fromEntries(externalNames.sort().map(f=>[f,hash(path.join(baseline,f))]));
if(mode==='--check') {
  const v=read(path.join(output,'verification.json'));assert.equal(v.status,'passed-offline-not-live-acceptance');assert.equal(v.provider_requests,0);
  assert.deepEqual(sourceInventory(),Object.keys(v.source_files_sha256).sort());authenticateFiles(source,v.source_files_sha256);
  authenticateFiles(output,v.evidence_files_sha256);assert.deepEqual(external,v.baseline_files_sha256);
  assert.deepEqual(inventory(output).filter(f=>f!=='verification.json'),Object.keys(v.evidence_files_sha256).sort());
  for(const c of v.commands){const p=read(path.join(output,c.name+'.process.json'));cleanTerminal(p);assert.equal(p.exit_code,0);}
  console.log(JSON.stringify({status:v.status,checks:v.commands.length,focused_test_pass_events:v.focused_test_pass_events,provider_requests:0,baseline_complete_cases:6}));process.exit(0);
}
fs.mkdirSync(output,{mode:0o700});
const before=Object.fromEntries(sourceInventory().map(f=>[f,hash(f)]));
const tc=process.env.KICADAI_OFFLINE_TOOLCHAIN;assert.ok(tc&&path.isAbsolute(tc));
const go=path.join(tc,'bin/go'),env={...cleanEnv,PATH:path.join(tc,'bin')+':'+cleanEnv.PATH,GOROOT:tc,GOTOOLCHAIN:'local',GOPROXY:'off',GOSUMDB:'off',GOMAXPROCS:'4',
  GOCACHE:process.env.KICADAI_OFFLINE_GOCACHE,GOMODCACHE:process.env.KICADAI_OFFLINE_GOMODCACHE,GOLANGCI_LINT_CACHE:'/private/tmp/kicadai-model-10-lint-cache'};
assert.ok(env.GOCACHE&&env.GOMODCACHE);assert.equal(env.OPENAI_API_KEY,undefined);
const commands=[];
async function run(name,command,args,extra={}) {
  const p=await executeRecorded(command,args,{...env,...extra},path.join(output,name),120000);cleanTerminal(p);
  assert.equal(p.exit_code,0,fs.readFileSync(path.join(output,name+'.stdout.log'),'utf8')+fs.readFileSync(path.join(output,name+'.stderr.log'),'utf8'));
  commands.push({name,command,args,wall_seconds:p.wall_seconds});console.log(JSON.stringify({check:name,status:'passed',seconds:p.wall_seconds}));
}
await run('focused',go,['test','./internal/boardfamily','./cmd/kicadai-board-family','-run','^TestGroundedFull','-count=1','-json'],{
  KICADAI_FULL_COMPARISON_BASELINE:path.join(baseline,'.cache/partitioned-08-production-02/batch')});
const packages=['./internal/boardfamily','./cmd/kicadai-board-family','./internal/aiprovider'];
await run('regression',go,['test','-short',...packages,'-count=1']);
await run('race',go,['test','-race','./internal/boardfamily','./cmd/kicadai-board-family','-run','^TestGroundedFull','-count=1']);
await run('vet',go,['vet',...packages]);
await run('lint','/opt/homebrew/bin/golangci-lint',['run',...packages]);
await run('diff-check','/usr/bin/git',['diff','--check']);
const goFiles=[...new Set([...git(['diff','--name-only','-z']).split('\0'),...git(['ls-files','--others','--exclude-standard','-z']).split('\0')])].filter(f=>f.endsWith('.go')).sort();
await run('gofmt',path.join(tc,'bin/gofmt'),['-l',...goFiles]);assert.equal(fs.readFileSync(path.join(output,'gofmt.stdout.log'),'utf8'),'');
const events=fs.readFileSync(path.join(output,'focused.stdout.log'),'utf8').trim().split('\n').map(l=>JSON.parse(l));
assert.ok(!events.some(e=>['fail','skip'].includes(e.Action)));const passes=events.filter(e=>e.Action==='pass'&&e.Test).length;assert.ok(passes>=60);
assert.deepEqual(sourceInventory(),Object.keys(before).sort());authenticateFiles(source,before);
durableJSON(path.join(output,'baseline-closure-check.json'),{exit_code:0,result:closure},{exclusive:true});
const evidence=Object.fromEntries(inventory(output).map(f=>[f,hash(path.join(output,f))]));
durableJSON(path.join(output,'verification.json'),{version:'model-comparison-10-offline-1',status:'passed-offline-not-live-acceptance',base_commit:base,
  model:'gpt-4.1-2025-04-14',accounting_profile:'gpt-4.1-full-standard-2026-09-16',provider_requests:0,commands,focused_test_pass_events:passes,
  source_files_sha256:before,evidence_files_sha256:evidence,baseline_files_sha256:external,
  limitations:['Synthetic responses are software tests, not extraction-fidelity evidence.','Native generator bytes and prior qualification are reused, not requalified here.',
    'The original failed evaluation and exhausted allowance are unchanged.','No remote CI, independent review, firewall rule or new live authorization is claimed.']},{exclusive:true});
console.log(JSON.stringify({status:'passed-offline-not-live-acceptance',checks:commands.length,focused_test_pass_events:passes,provider_requests:0}));
