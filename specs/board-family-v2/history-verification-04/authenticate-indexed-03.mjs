// Versioned, read-only historical verification. No source checkout, rewriting,
// provider request, budget recovery, or reinterpretation of the failed score.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

export const sourceCommit='8744d7fc032452b2683afac0e5f9082f963de164';
export const publicationCommit='a0d719cce11c51fa6c510058a73869900a5bae7f';
export const publication='specs/board-family-v2/indexed-evaluation-03';
const manifestSHA='429f02b613038d22adaacc1cc9ef054e8b18703afe748312c3e925b59c977023';
const keys=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS'];
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const hash=file=>sha(fs.readFileSync(file));
const read=file=>JSON.parse(fs.readFileSync(file,'utf8'));
export function relativeFile(file) {
  assert.ok(typeof file==='string'&&file&&!/[\0\r\n\\]/.test(file));
  assert.ok(!path.posix.isAbsolute(file)&&path.posix.normalize(file)===file&&file!=='.'&&!file.startsWith('../'));
  return file;
}
function offlineEnv() {
  return {PATH:process.env.PATH,HOME:process.env.HOME,TMPDIR:process.env.TMPDIR,
    LANG:'C.UTF-8',GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:'/dev/null'};
}
function git(repo,args,input) {
  const r=spawnSync('git',['-C',repo,'-c','core.fsmonitor=false',...args],{
    input,env:offlineEnv(),timeout:30000,maxBuffer:256*1024*1024});
  assert.equal(r.error,undefined,'Git snapshot read failed');assert.equal(r.signal,null);
  assert.equal(r.status,0,'Git snapshot unavailable; fetch recorded history explicitly before checking');
  return r.stdout;
}
export function parseBatch(buffer,names) {
  let offset=0;const hashes={};
  for(const name of names) {
    relativeFile(name);assert.ok(!Object.hasOwn(hashes,name),'duplicate snapshot path');
    const end=buffer.indexOf(10,offset);assert.ok(end>=offset,'missing Git blob header');
    const header=buffer.subarray(offset,end).toString('ascii');
    const m=/^[0-9a-f]{40} blob ([0-9]+)$/.exec(header);assert.ok(m,'missing or non-blob Git snapshot object');
    const size=Number(m[1]);assert.ok(Number.isSafeInteger(size)&&size<=64*1024*1024);
    offset=end+1;assert.ok(offset+size<buffer.length,'truncated Git blob');
    hashes[name]=sha(buffer.subarray(offset,offset+size));offset+=size;
    assert.equal(buffer[offset++],10,'invalid Git blob terminator');
  }
  assert.equal(offset,buffer.length,'unexpected Git batch content');return hashes;
}
function tree(repo,commit) {
  assert.match(commit,/^[a-f0-9]{40}$/);
  return git(repo,['ls-tree','--full-tree','-r','--name-only','-z',commit]).toString('utf8').split('\0').filter(Boolean).map(relativeFile);
}
function gitHashes(repo,commit,names) {
  assert.match(commit,/^[a-f0-9]{40}$/);names.forEach(relativeFile);
  return parseBatch(git(repo,['cat-file','--batch'],names.map(n=>`${commit}:${n}\n`).join('')),names);
}
function inventory(root,prefix='') {
  return fs.readdirSync(path.join(root,prefix),{withFileTypes:true}).flatMap(e=>{
    const p=relativeFile(path.posix.join(prefix,e.name));assert.ok(!e.isSymbolicLink(),'symlink in historical archive');
    assert.ok(e.isDirectory()||e.isFile(),'unexpected archive entry');return e.isDirectory()?inventory(root,p):[p];
  }).sort();
}
export function authenticate({repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../..'),runtime=false}={}) {
  for(const key of keys)assert.ok(!process.env[key],'run historical checks without provider credentials');
  repo=fs.realpathSync(repo);
  const archived=tree(repo,publicationCommit).filter(p=>p.startsWith(publication+'/'));
  assert.equal(archived.length,31,'publication snapshot inventory changed');
  const publishedHashes=gitHashes(repo,publicationCommit,archived);
  assert.deepEqual(inventory(path.join(repo,publication)),archived.map(p=>p.slice(publication.length+1)).sort());
  for(const p of archived)assert.equal(hash(path.join(repo,p)),publishedHashes[p],'historical publication changed: '+p);
  const root=path.join(repo,publication),manifestFile=path.join(root,'runtime-manifest.json');
  assert.equal(hash(manifestFile),manifestSHA);
  const m=read(manifestFile),q=read(path.join(root,'runtime-qualification.json'));
  assert.equal(hash(path.join(root,'runtime-qualification.json')),m.production_qualification.sha256);
  const sourceTree=new Set(tree(repo,sourceCommit)),frozen=Object.keys(m.runtime_files_sha256).map(relativeFile);
  const tracked=frozen.filter(p=>sourceTree.has(p)),cached=frozen.filter(p=>!sourceTree.has(p));
  assert.equal(tracked.length,1155);assert.equal(frozen.length,2530);
  assert.ok(cached.every(p=>p.startsWith('.cache/')),'unversioned source outside frozen cache');
  const sourceHashes=gitHashes(repo,sourceCommit,tracked);
  for(const p of tracked)assert.equal(sourceHashes[p],m.runtime_files_sha256[p],'frozen source differs from recorded Git commit: '+p);
  for(const [p,h]of Object.entries(q.source_sha256))assert.equal(m.runtime_files_sha256[relativeFile(p)],h);
  const approval=read(path.join(root,'approval.json')),state=read(path.join(root,'batch/result.json'));
  const ledger=read(path.join(root,'batch/ledger.json')),start=read(path.join(root,'batch/start.json'));
  assert.equal(start.manifest_sha256,manifestSHA);assert.equal(approval.manifest_sha256,manifestSHA);
  assert.equal(start.approval_sha256,hash(path.join(root,'approval.json')));
  assert.equal(approval.status,'explicit-user-approved');assert.equal(approval.max_physical_requests,14);assert.equal(approval.max_micro_usd,1000000);
  assert.equal(state.status,'stopped-no-retry');assert.equal(state.planned_cases,14);assert.equal(state.launched_cases,1);
  assert.equal(state.recorded_outcomes,0);assert.equal(state.unattempted_case_ids.length,13);
  assert.equal(ledger.entries.length,1);assert.equal(ledger.entries[0].status,'failed_or_unknown');assert.equal(ledger.entries[0].reserve_micro_usd,50000);
  const outcome=state.records[0],caseRoot=path.join(root,'batch/useful-01');
  assert.deepEqual(read(path.join(caseRoot,'outcome.json')),outcome);
  assert.deepEqual(inventory(caseRoot).filter(p=>p!=='outcome.json'),Object.keys(outcome.files_sha256).sort());
  for(const [p,h]of Object.entries(outcome.files_sha256))assert.equal(hash(path.join(caseRoot,relativeFile(p))),h);
  for(const stem of ['command','audit']) {
    const p=read(path.join(caseRoot,stem+'.process.json'));
    assert.deepEqual(p,stem==='command'?outcome.execution:outcome.audit_execution);
    assert.equal(p.child_terminal_observed,true);assert.equal(p.exit_code,1);assert.equal(p.signal,null);assert.equal(p.timed_out,false);
  }
  const results=read(path.join(root,'results.json')),review=read(path.join(root,'review.json'));
  assert.equal(results.acceptance,'complete-acceptance-not-met');
  for(const key of ['raw_passes','application_passes','complete_passes'])assert.equal(results[key],0);
  assert.equal(review.status,'source-bound-semantic-review');assert.equal(review.bindings.manifest_sha256,manifestSHA);
  assert.equal(review.bindings.result_sha256,hash(path.join(root,'batch/result.json')));
  assert.equal(review.bindings.selection_sha256['useful-01'],hash(path.join(caseRoot,'journal/selection/selection.json')));
  let replay='not-run';
  if(runtime) {
    // The original binary/cache locations remain pinned. Only mutable source
    // files are resolved from Git; no replacement executable is substituted.
    const runtimeRoot=path.resolve(path.dirname(m.binary),'../../..');
    for(const p of cached)assert.equal(hash(path.join(runtimeRoot,p)),m.runtime_files_sha256[p],'frozen cache changed: '+p);
    assert.equal(hash(m.binary),m.binary_sha256);assert.equal(hash(process.execPath),m.node_sha256);assert.equal(hash(m.kicad_cli),m.kicad_cli_sha256);
    assert.equal(q.binary,m.binary);assert.equal(q.binary_sha256,m.binary_sha256);assert.equal(hash(q.compiler.path),q.compiler.sha256);
    for(const c of q.commands)assert.deepEqual(read(path.join(runtimeRoot,q.directory,c.id+'.process.json')),c.result);
    // Git does not retain private journal permissions. Stage exact bytes in an
    // owned private scratch directory; never chmod or rewrite the publication.
    const scratch=fs.mkdtempSync(path.join(os.tmpdir(),'kicadai-history-03-'));
    try {
      for(const p of inventory(path.join(caseRoot,'journal'))) {
        const from=path.join(caseRoot,'journal',p),to=path.join(scratch,p);
        fs.mkdirSync(path.dirname(to),{recursive:true,mode:0o700});
        fs.copyFileSync(from,to,fs.constants.COPYFILE_EXCL);fs.chmodSync(to,0o600);
        assert.equal(hash(to),hash(from),'private replay copy changed');
      }
      const r=spawnSync(m.binary,['--inspect-indexed-journal',scratch],{env:offlineEnv(),encoding:'utf8',timeout:10000,maxBuffer:1024*1024,cwd:repo});
      assert.equal(r.error,undefined);assert.equal(r.signal,null);assert.equal(r.status,1);
      assert.equal(r.stdout,fs.readFileSync(path.join(caseRoot,'audit.stdout.log'),'utf8'));
      assert.equal(r.stderr,fs.readFileSync(path.join(caseRoot,'audit.stderr.log'),'utf8'));
    } finally {fs.rmSync(scratch,{recursive:true});}
    replay='original-auditor-failure-reproduced';
  }
  return {status:'unchanged-failed-publication-verified',scope:runtime?'archive-and-original-runtime':'archive-and-git-source-only',
    source_commit:sourceCommit,publication_commit:publicationCommit,publication_files:archived.length,git_source_files:tracked.length,
    frozen_cache_files_checked:runtime?cached.length:0,runtime_verified:runtime,replay,planned_cases:14,physical_requests:1,
    unattempted_cases:13,complete_passes:0,reserved_micro_usd:50000,live_requests_in_this_check:0,
    notice:'Historical consistency only; no live acceptance, repaired result, billing attestation, or new execution authority.'};
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  try {
    assert.ok(process.argv.length===3&&['--check-archive','--check-runtime'].includes(process.argv[2]));
    console.log(JSON.stringify(authenticate({runtime:process.argv[2]==='--check-runtime'}),null,2));
  } catch(e) {console.error(e.message);process.exitCode=1;}
}
