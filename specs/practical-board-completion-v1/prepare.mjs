// Offline preparation only. Every invocation gets an exclusive, source-bound receipt.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync, spawn} from 'node:child_process';
import {mkdirSync, readFileSync, readdirSync, writeFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const phase=dirname(fileURLToPath(import.meta.url)), repo=resolve(phase,'../..');
const [id,variant,mode,sourceArgument]=process.argv.slice(2);
assert(/^[a-z0-9-]+$/.test(id??''),'exclusive preparation ID required');
assert(['baseline','candidate'].includes(variant));
assert(['test','race','build','snapshot','validate'].includes(mode),'not a board execution runner');
const source=resolve(sourceArgument??repo), engineRelative='specs/practical-board-completion-v1/engine';
const tc=join(repo,'.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64');
const sha=b=>createHash('sha256').update(b).digest('hex');
const secret=process.env.OPENAI_API_KEY;
const removed=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_OPENAI_LIVE_TEST'];
const env={...process.env,PATH:tc+'/bin:'+process.env.PATH,GOROOT:tc,GOTOOLCHAIN:'local',GOENV:'off',GOWORK:'off',GOFLAGS:'',GOEXPERIMENT:'',CGO_ENABLED:'1',GOMAXPROCS:'4',GOCACHE:repo+'/.cache/go/build',GOMODCACHE:repo+'/.cache/go/mod',GOPROXY:'off',GOSUMDB:'off'};
for(const key of removed)delete env[key];
const git=(...args)=>execFileSync('git',args,{cwd:source,env,encoding:'utf8'}).trim();
assert.equal(git('status','--porcelain','--untracked-files=no'),'','commit production edits before preparation');
const revision=git('rev-parse','HEAD');
if(variant==='baseline')assert.equal(revision,'87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d');
const adapter=()=>readdirSync(join(source,engineRelative)).filter(p=>p.endsWith('.go')).sort().map(path=>{
 const bytes=readFileSync(join(source,engineRelative,path));
 assert(!secret||!bytes.includes(Buffer.from(secret)),'secret in adapter');
 assert.equal(sha(bytes),sha(readFileSync(join(phase,'engine',path))),'both engines require identical adapter source');
 return {path,bytes:bytes.length,sha256:sha(bytes),source:bytes.toString()};
});
const adapterFiles=adapter(), adapterSHA=sha(Buffer.from(JSON.stringify(adapterFiles.map(({path,bytes,sha256})=>({path,bytes,sha256})))));
const parent=join(repo,'.cache/practical-board-completion-v1/preparation');
mkdirSync(parent,{recursive:true});const output=join(parent,id);mkdirSync(output,{mode:0o700});
const write=(name,value)=>writeFileSync(join(output,name),JSON.stringify(value,null,2)+'\n',{flag:'wx',mode:0o600});
const runnerBytes=readFileSync(fileURLToPath(import.meta.url));
write('source.json',{revision,variant,adapter_sha256:adapterSHA,files:adapterFiles,runner_source:runnerBytes.toString(),runner_sha256:sha(runnerBytes)});
const tags=variant==='candidate'?['-tags=completion_candidate']:[];
let command=join(tc,'bin/go'), args;
if(mode==='test'||mode==='race')args=['test',...tags,...(mode==='race'?['-race']:[]),'-count=1','-timeout=3m','./'+engineRelative];
if(mode==='build')args=['build',...tags,'-trimpath','-ldflags',`-X main.engineSource=${revision} -X main.adapterSHA256=${adapterSHA}`,'-o',join(output,'engine'),'./'+engineRelative];
if(mode==='snapshot'||mode==='validate'){
 const build=join(parent,mode==='snapshot'?id.replace(/-snapshot$/, '-build'):id.replace(/-validate-p0[1-6]$/, '-build'));
 assert.notEqual(build,output,'snapshot ID must end in -snapshot and refer to a matching -build');
 const receipt=JSON.parse(readFileSync(join(build,'execution.json')));
 assert.equal(receipt.exit_code,0);assert.equal(receipt.source_revision,revision);assert.equal(receipt.adapter_sha256,adapterSHA);
 command=join(build,'engine');assert.equal(sha(readFileSync(command)),receipt.binary_sha256);
 args=['--mode','snapshot','--output',join(output,'snapshot'),'--timeout','3m'];
 if(mode==='validate'){
  const match=id.match(/-validate-(p0[1-6])$/);assert(match,'fixed nonreserved validation ID required');
  args=['--mode','validate','--input',join(phase,'probes',match[1].toUpperCase()+'.json'),'--output',join(output,'validation'),'--timeout','3m'];
 }
}
const started=new Date().toISOString(), logs={stdout:[],stderr:[]};
let total=0, limitFailure=null, spawnError=null;
const child=spawn(command,args,{cwd:source,env,detached:true,stdio:['ignore','pipe','pipe']});
const kill=()=>{try{process.kill(-child.pid,'SIGKILL');}catch{}};
const timer=setTimeout(()=>{limitFailure='five_minute_preparation_timeout';kill();},300000);
for(const stream of ['stdout','stderr'])child[stream].on('data',b=>{
 total+=b.length;
 if(total<=4*1024*1024)logs[stream].push(b);
 else {limitFailure='four_mib_output_limit';kill();}
});
child.on('error',e=>{spawnError=e.message;});
const terminal=await new Promise(resolve=>child.on('close',(exit_code,signal)=>resolve({exit_code,signal})));
clearTimeout(timer);
const outputs={};
for(const stream of ['stdout','stderr']){
 const b=Buffer.concat(logs[stream]);
 assert(!secret||!b.includes(Buffer.from(secret)),'secret in child output: refusing persistence');
 writeFileSync(join(output,stream+'.log'),b,{flag:'wx',mode:0o600});outputs[stream+'_sha256']=sha(b);
}
const unchanged=git('rev-parse','HEAD')===revision&&git('status','--porcelain','--untracked-files=no')===''&&JSON.stringify(adapter())===JSON.stringify(adapterFiles)&&sha(readFileSync(fileURLToPath(import.meta.url)))===sha(runnerBytes);
const receipt={schema:'kicadai.completion-preparation-execution.v1',id,mode,variant,source_revision:revision,adapter_sha256:adapterSHA,source_snapshot_sha256:sha(readFileSync(join(output,'source.json'))),source_unchanged:unchanged,command,args,cwd:source,started_utc:started,finished_utc:new Date().toISOString(),...terminal,spawn_error:spawnError,limit_failure:limitFailure,...outputs,removed_provider_variables:removed,dependency_network:'disabled',provider_calls:0,board_attempts:0};
if(mode==='build'&&terminal.exit_code===0)receipt.binary_sha256=sha(readFileSync(join(output,'engine')));
write('execution.json',receipt);console.log(JSON.stringify(receipt,null,2));
assert(unchanged,'source changed during execution');assert(!limitFailure&&!spawnError,'preparation failed');
assert.equal(terminal.exit_code,0,Buffer.concat(logs.stderr).toString());assert.equal(terminal.signal,null);
