// Frozen supervisor: serial workers, no transparent retries and no overwrite.
// Invoke with: node run-campaign.mjs <binary> <root> <campaign-root> <phase>
// The campaign root contains one shared journal for baseline AND final usage.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {spawn, execFileSync} from 'node:child_process';

const [binaryArg, rootArg, campaignRootArg, phase] = process.argv.slice(2);
if (!binaryArg || !rootArg || !campaignRootArg || !['baseline','final','paired'].includes(phase) || process.argv.length!==6) {
  throw new Error('usage: node run-campaign.mjs <binary> <root> <campaign-root> <baseline|final|paired>');
}
const root=path.resolve(rootArg), binary=path.resolve(binaryArg), campaignRoot=path.resolve(campaignRootArg);
const spec=path.join(root,'specs/practical-sensor-controller-boards');
const readJSON=p=>JSON.parse(fs.readFileSync(p,'utf8'));
const sha=b=>crypto.createHash('sha256').update(b).digest('hex');
const save=(p,v)=>fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n',{flag:'wx',mode:0o600});
const corpus=readJSON(path.join(spec,'corpus.json')), freeze=readJSON(path.join(spec,'freeze.json'));
if(corpus.status!=='frozen') throw new Error('corpus is not frozen');
for(const file of freeze.files) {
  const p=path.resolve(root,file.path);
  if(!p.startsWith(root+path.sep) || fs.lstatSync(p).isSymbolicLink()) throw new Error('unsafe freeze path');
  const bytes=fs.readFileSync(p);
  if(bytes.length!==file.bytes || sha(bytes)!==file.sha256) throw new Error('freeze mismatch: '+file.path);
}
if(!fs.existsSync(campaignRoot)) fs.mkdirSync(campaignRoot,{mode:0o700});
const journal=path.join(campaignRoot,'request-journal');
if(!fs.existsSync(journal)) fs.mkdirSync(journal,{mode:0o700});
const lock=path.join(campaignRoot,'active-campaign.lock');
fs.writeFileSync(lock,JSON.stringify({pid:process.pid,phase})+'\n',{flag:'wx',mode:0o600});
process.once('exit',()=>fs.unlinkSync(lock));
const output=path.join(campaignRoot,phase);
fs.mkdirSync(output,{mode:0o700}); // EEXIST is deliberately terminal.
const git=(...args)=>execFileSync('git',args,{cwd:root,encoding:'utf8'}).trim();
const sourceCommit=git('rev-parse','HEAD');
const productionChanges=git('diff','--name-only',freeze.baseline_commit,'--','.',
  ':(exclude)internal/practicalboardeval/**',':(exclude)cmd/practical-board-eval/**',
  ':(exclude)specs/practical-sensor-controller-boards/**');
if(phase==='baseline' && productionChanges) throw new Error('baseline production tree differs from merged main: '+productionChanges);
const binaryBytes=fs.readFileSync(binary);
const cli=process.env.KICADAI_KICAD_CLI;
if(!cli || !fs.existsSync(cli)) throw new Error('installed KiCad CLI is required');
const started=Date.now();
save(path.join(output,'campaign-start.json'),{
  phase,started_utc:new Date(started).toISOString(),source_commit:sourceCommit,
  source_diff_sha256:sha(execFileSync('git',['diff',freeze.baseline_commit,'--','.'],{cwd:root})),
  binary_sha256:sha(binaryBytes),freeze_sha256:sha(fs.readFileSync(path.join(spec,'freeze.json'))),
  binary_build_metadata:execFileSync('go',['version','-m',binary],{encoding:'utf8'}),
  kicad_cli:cli,kicad_version:execFileSync(cli,['--version'],{encoding:'utf8'}).trim(),
  kicad_binary_sha256:sha(fs.readFileSync(cli)),
  node:process.version,platform:process.platform,arch:process.arch,
  resource_policy:{sample_ms:1000,case_wall_seconds:1200,campaign_wall_seconds:7200,process_tree_rss_bytes:16*1024**3,evidence_bytes:10*1024**3},
  model:'gpt-5.6-sol',max_output_tokens:16384,parallel_cases:1,GOMAXPROCS:4,
  assistance:{credential_reuse_authorized:true,manual_implementation_repairs:0,human_active_minutes:null}
});

function evidenceBytes(dir) {
  let bytes=0;
  for(const entry of fs.readdirSync(dir,{withFileTypes:true})) {
    const p=path.join(dir,entry.name);
    if(entry.isSymbolicLink()) throw new Error('symlink in evidence');
    bytes+=entry.isDirectory()?evidenceBytes(p):fs.statSync(p).size;
  }
  return bytes;
}
function treeRSS(pid) {
  const rows=execFileSync('ps',['-axo','pid=,ppid=,rss='],{encoding:'utf8'}).trim().split('\n').map(x=>x.trim().split(/\s+/).map(Number));
  const selected=new Set([pid]);
  for(let changed=true;changed;) { changed=false; for(const [p,parent] of rows) if(selected.has(parent)&&!selected.has(p)){selected.add(p);changed=true;} }
  return rows.reduce((sum,[p,,rss])=>sum+(selected.has(p)?rss*1024:0),0);
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
    for(const name of fs.readdirSync(caseDir).filter(x=>/^attempt-\d+\.error\.json$/.test(x))) {
      const failure=readJSON(path.join(caseDir,name));
      if(['ai_provider_authentication','ai_provider_rate_limit','ai_provider_transport','ai_provider_timeout','ai_provider_configuration'].includes(failure.code)) campaignStop=failure.code;
    }
  }
}
save(path.join(output,'campaign-end.json'),{phase,completed_utc:new Date().toISOString(),wall_seconds:(Date.now()-started)/1000,stop_reason:campaignStop,outcomes});
