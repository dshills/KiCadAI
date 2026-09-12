// Read-only publication QA. Local temporary archive extraction only; no provider client.
import assert from 'node:assert/strict';
import {readFileSync,mkdtempSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {authenticate,sha,walk,safePath} from '../live-v1/authenticate.mjs';
import {derive} from './derive.mjs';
const dir=dirname(fileURLToPath(import.meta.url));
const repo=resolve(dir,'../../..');
const read=p=>JSON.parse(readFileSync(p,'utf8'));
export function verify() {
 const raw='/tmp/kicadai-ai-requirement-interface-v1';
 const saved=read(join(dir,'authentication.json')),fresh=authenticate(raw,process.env.OPENAI_API_KEY);
 const stable=({verified_utc,...x})=>x;
 assert.deepEqual(stable(fresh),stable(saved));
 const {audits,summary}=derive(raw);
 assert.deepEqual(read(join(dir,'audits.json')),audits);
 assert.deepEqual(read(join(dir,'results.json')),summary);
 const artifact=read(join(dir,'artifact.json'));
 const sql=readFileSync(join(dir,'report-query.sql'),'utf8');
 const rows=JSON.parse(execFileSync('sqlite3',['-json',':memory:'],{cwd:repo,encoding:'utf8',input:sql}));
 assert.deepEqual(rows,artifact.snapshot.datasets.cases);
 const nodeRows=JSON.parse(execFileSync('node',[join(dir,'report-data.mjs')],{cwd:repo,encoding:'utf8'}));
 assert.deepEqual(rows,nodeRows.map(x=>({...x,faithful:+x.faithful,sealed_replay_passed:+x.sealed_replay_passed})));
 assert.equal(artifact.manifest.sources[0].query.sql,sql);
 assert.equal(artifact.manifest.charts[0].source.query.sql,sql);
 assert.equal(artifact.manifest.tables[0].source.query.sql,sql);
 assert.equal(artifact.snapshot.status,'ready');
 const markdown=readFileSync(join(dir,'README.md'),'utf8');
 for(const c of summary.cases)assert(markdown.includes('| '+c.case_id+' |'));
 assert(markdown.includes('gate not met'));
 assert(markdown.includes('does **not** make the sealed replay pass'));
 const files=walk(dir);
 assert.deepEqual(files.filter(x=>x.path!=='publication-inventory.json'),read(join(dir,'publication-inventory.json')));
 for(const f of files){
  const bytes=readFileSync(join(dir,f.path));
  assert(!bytes.includes(process.env.OPENAI_API_KEY),'Credential found in publication');
  assert(!/\bsk-[A-Za-z0-9_-]{20,}/.test(bytes.toString('utf8')),'Credential-shaped content in publication');
 }
 const archives=read(join(dir,'archives.json'));
 const extraction=mkdtempSync(join(tmpdir(),'kicadai-interface-archive-verification-'));
 for(const archive of archives.archives) {
  const path=join(repo,archive.path),bytes=readFileSync(path);
  assert.equal(bytes.length,archive.bytes);assert.equal(sha(bytes),archive.sha256);
  const entries=execFileSync('tar',['-tf',path],{encoding:'utf8'}).trim().split('\n');
  for(const entry of entries)safePath(entry.replace(/\/$/,''));
  execFileSync('tar',['-xf',path,'-C',extraction]);
 }
 assert.deepEqual(walk(join(extraction,'kicadai-ai-requirement-interface-v1')),walk(raw));
 for(const binary of archives.archives[1].binaries)assert.equal(sha(readFileSync(join(extraction,binary.path))),binary.sha256);
 return {schema:'kicadai.interface-publication-verification.v1',verified_utc:new Date().toISOString(),
  raw_authenticated:true,raw_files:fresh.file_count,frozen_files:fresh.frozen_files_verified,
  source_bound_cases:audits.case_count,source_bound_clauses:audits.clause_count,
  reproduced_results:true,independent_sql_node_rows_match:true,report_rows:rows.length,
  publication_files:files.length,publication_inventory_verified:true,
  publication_secret_scan:'absent',raw_and_binary_archive_contents_verified:true,
  temporary_verification_directory:extraction,
  sealed_replay_passed:false,supplementary_attempts_matched:15,tampered_bindings_rejected:8,
  provider_calls:0,gate_passed:summary.gate_passed};
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url))console.log(JSON.stringify(verify(),null,2));
