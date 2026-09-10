// Frozen supervisor: serial workers, no transparent retries and no overwrite.
// Invoke with: node run-campaign.mjs <binary> <root> <campaign-root> <phase>
// The campaign root contains one shared journal for baseline AND final usage.
import fs from 'node:fs';
import path from 'node:path';
import {spawn, execFileSync} from 'node:child_process';
import {sha,policy,evidenceBytes,treeRSSFromPS,checkEnvironment,checkBuild,providerStopReason} from './evidence-utils.mjs';

const [binaryArg, rootArg, campaignRootArg, phase] = process.argv.slice(2);
if (!binaryArg || !rootArg || !campaignRootArg || !['baseline','final','paired'].includes(phase) || process.argv.length!==6) {
  throw new Error('usage: node run-campaign.mjs <binary> <root> <campaign-root> <baseline|final|paired>');
}
const root=path.resolve(rootArg), binary=path.resolve(binaryArg), campaignRoot=path.resolve(campaignRootArg);
const spec=path.join(root,'specs/practical-sensor-controller-boards');
const readJSON=p=>JSON.parse(fs.readFileSync(p,'utf8'));
const save=(p,v)=>fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n',{flag:'wx',mode:0o600});
const corpus=readJSON(path.join(spec,'corpus.json')), freeze=readJSON(path.join(spec,'freeze.json'));
if(corpus.status!=='frozen') throw new Error('corpus is not frozen');
for(const file of freeze.files) {
  const p=path.resolve(root,file.path);
  if(!p.startsWith(root+path.sep) || fs.lstatSync(p).isSymbolicLink()) throw new Error('unsafe freeze path');
  const bytes=fs.readFileSync(p);
  if(bytes.length!==file.bytes || sha(bytes)!==file.sha256) throw new Error('freeze mismatch: '+file.path);
}
const git=(...args)=>execFileSync('git',args,{cwd:root,encoding:'utf8'}).trim();
const sourceCommit=git('rev-parse','HEAD');
if(git('status','--porcelain')) throw new Error('evaluation requires a clean source tree');
for(const key of Object.keys(process.env)) {
  if(key.startsWith('KICADAI_') && !['KICADAI_KICAD_CLI','KICADAI_SYMBOLS_ROOT','KICADAI_FOOTPRINTS_ROOT'].includes(key)) throw new Error('unfrozen application environment override: '+key);
}
const sealed=new Set(freeze.files.map(f=>f.path));
const required=[...git('ls-files','internal/practicalboardeval','cmd/practical-board-eval').split('\n'),...['SPEC.md','SCORING.md','FEASIBILITY.md','PLAN.md','PREPARATION.md','QUALITY.md','corpus.json','environment.json','run-campaign.mjs','evidence-utils.mjs','evidence-utils.test.mjs'].map(p=>'specs/practical-sensor-controller-boards/'+p)];
if(required.some(p=>!sealed.has(p))) throw new Error('incomplete evaluator freeze');
const productionChanges=git('diff','--name-only',freeze.baseline_commit,'--','.',
  ':(exclude)internal/practicalboardeval/**',':(exclude)cmd/practical-board-eval/**',
  ':(exclude)specs/practical-sensor-controller-boards/**');
if(phase==='baseline' && productionChanges) throw new Error('baseline production tree differs from merged main: '+productionChanges);
const binaryBytes=fs.readFileSync(binary);
const cli=process.env.KICADAI_KICAD_CLI;
if(!cli || !fs.existsSync(cli)) throw new Error('installed KiCad CLI is required');
const environment={node:process.version,node_binary_sha256:sha(fs.readFileSync(process.execPath)),platform:process.platform,arch:process.arch,
  go_version:execFileSync('go',['version'],{encoding:'utf8'}).trim().split(/\s+/)[2],
  kicad_cli:cli,kicad_version:execFileSync(cli,['--version'],{encoding:'utf8'}).trim(),kicad_binary_sha256:sha(fs.readFileSync(cli)),
  symbols_root:process.env.KICADAI_SYMBOLS_ROOT,footprints_root:process.env.KICADAI_FOOTPRINTS_ROOT};
const frozenEnvironment=readJSON(path.join(spec,'environment.json'));
checkEnvironment(frozenEnvironment,environment);
const buildMetadata=execFileSync('go',['version','-m',binary],{encoding:'utf8'});
checkBuild(buildMetadata,sourceCommit,environment.go_version);
if(!fs.existsSync(campaignRoot)) fs.mkdirSync(campaignRoot,{mode:0o700});
const journal=path.join(campaignRoot,'request-journal');
if(!fs.existsSync(journal)) fs.mkdirSync(journal,{mode:0o700});
const lock=path.join(campaignRoot,'active-campaign.lock');
fs.writeFileSync(lock,JSON.stringify({pid:process.pid,phase})+'\n',{flag:'wx',mode:0o600});
process.once('exit',()=>fs.unlinkSync(lock));
const output=path.join(campaignRoot,phase);
fs.mkdirSync(output,{mode:0o700}); // EEXIST is deliberately terminal.
const started=Date.now();
save(path.join(output,'campaign-start.json'),{
  phase,started_utc:new Date(started).toISOString(),source_commit:sourceCommit,
  source_diff_sha256:sha(execFileSync('git',['diff',freeze.baseline_commit,'--','.'],{cwd:root})),
  binary_sha256:sha(binaryBytes),freeze_sha256:sha(fs.readFileSync(path.join(spec,'freeze.json'))),
  binary_build_metadata:buildMetadata,...environment,resource_policy:policy,
  model:'gpt-5.6-sol',max_output_tokens:16384,parallel_cases:1,GOMAXPROCS:4,
  assistance:{credential_reuse_authorized:true,manual_implementation_repairs:0,human_active_minutes:null}
});
// No provider request occurs in this snapshot. Catalog/model changes are
// allowed only in the scoped final engine; native libraries and policy stay fixed.
const snapshotDir=path.join(output,'environment-snapshot');
execFileSync(binary,['--mode','snapshot','--root',root,'--output',snapshotDir],{cwd:root,timeout:120000,env:{...process.env,GOMAXPROCS:'4'}});
const snapshot=readJSON(path.join(snapshotDir,'snapshot.json'));
for(const key of ['library_index_sha256',...(phase==='baseline'?['catalog_sha256','models_sha256','capabilities_sha256']:[])]) {
  if(snapshot[key]!==frozenEnvironment.baseline_snapshot[key]) throw new Error('frozen snapshot mismatch: '+key);
}
if(sha(fs.readFileSync(path.join(snapshotDir,'closed-loop-policy.json')))!==frozenEnvironment.closed_loop_policy_sha256) throw new Error('frozen closed-loop policy mismatch');
function treeRSS(pid) {
  return treeRSSFromPS(execFileSync('ps',['-axo','pid=,ppid=,rss='],{encoding:'utf8'}),pid);
}
function stopGroup(child) {
  try{process.kill(-child.pid,'SIGKILL');}catch(err){if(err.code!=='ESRCH')throw err;}
}
const inputs=[...corpus.cases,...corpus.paraphrases];
const outcomes=[];
let campaignStop=null;
for(const item of inputs) {
  if(Date.now()-started>=7200000) campaignStop='campaign_wall_cap';
  if(evidenceBytes(campaignRoot)>=10*1024**3) campaignStop='evidence_cap';
  if(campaignStop) { outcomes.push({id:item.id,status:'not_run',reason:campaignStop}); continue; }
  const caseDir=path.join(output,item.id), workerLog=path.join(output,item.id+'.worker.log');
  const logFD=fs.openSync(workerLog,'wx',0o600);
  const args=['--mode','case','--root',root,'--output',caseDir,'--case',item.id,'--campaign',phase,'--journal',journal,'--kicad-cli',cli];
  if(phase==='paired') args.push('--paired-input',path.join(campaignRoot,'baseline',item.id));
  const begin=Date.now();
  const child=spawn(binary,args,{cwd:root,detached:true,stdio:['ignore',logFD,logFD],env:{...process.env,GOMAXPROCS:'4',KICADAI_PRACTICAL_EVAL_SUPERVISED:'1'}});
  let peakRSS=null,stopReason=null,samplingErrors=0,sampling=false;
  const sample=()=>{
    if(sampling||stopReason)return;
    sampling=true;
    try{
      peakRSS=Math.max(peakRSS??0,treeRSS(child.pid));
      if(peakRSS>16*1024**3) stopReason='case_rss_cap';
      else if(Date.now()-begin>=1200000)stopReason='case_wall_cap';
      else if(Date.now()-started>=7200000)stopReason=campaignStop='campaign_wall_cap';
      else if(evidenceBytes(campaignRoot)>10*1024**3)stopReason=campaignStop='evidence_cap';
    }catch{samplingErrors++;stopReason=campaignStop='resource_monitor_failed';}
    finally{sampling=false;}
    if(stopReason)stopGroup(child);
  };
  const interval=setInterval(sample,1000);
  const result=await new Promise(resolve=>{
    child.once('error',err=>resolve({exit_code:null,signal:null,spawn_error:err.message}));
    child.once('close',(code,signal)=>resolve({exit_code:code,signal}));
  });
  clearInterval(interval);
  fs.closeSync(logFD);
  const metric={id:item.id,...result,wall_seconds:(Date.now()-begin)/1000,peak_sampled_process_tree_rss_bytes:peakRSS,sampling_interval_ms:1000,sampling_errors:samplingErrors,stop_reason:stopReason,evidence_bytes:fs.existsSync(caseDir)?evidenceBytes(caseDir):0};
  save(path.join(output,item.id+'.resources.json'),metric);
  outcomes.push(metric);
  console.log(JSON.stringify(metric));
  if(fs.existsSync(caseDir)) {
    campaignStop=providerStopReason(caseDir)??campaignStop;
  }
}
save(path.join(output,'campaign-end.json'),{phase,completed_utc:new Date().toISOString(),wall_seconds:(Date.now()-started)/1000,stop_reason:campaignStop,outcomes});
