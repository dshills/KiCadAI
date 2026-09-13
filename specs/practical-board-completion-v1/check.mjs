// Source-bound offline regression/verification commands, including retained failures.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync,spawn} from 'node:child_process';
import {mkdirSync,readFileSync,writeFileSync,existsSync} from 'node:fs';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../..');
const [id,mode]=process.argv.slice(2);assert(/^[a-z0-9-]+$/.test(id??''));
const modes={
 reference:['test','-count=1','-timeout=3m','-run','TestExplicitReference|TestReferenceDomain','./internal/architecturesearch'],
 reference_race:['test','-race','-count=1','-timeout=3m','-run','TestExplicitReference|TestReferenceDomain','./internal/architecturesearch'],
 adapter:['test','-tags=completion_candidate','-race','-count=1','-timeout=3m','./specs/practical-board-completion-v1/engine'],
 evaluator_race:['test','-race','-count=1','-timeout=3m','./internal/practicalboardeval','./cmd/practical-board-eval'],
 full:['test','-short','-count=1','-timeout=12m','./...'],
 vet:['vet','./...'],
 lint:['run','--timeout=10m','./cmd/...','./internal/...'],
 node:['--test','specs/practical-sensor-controller-boards/evidence-utils.test.mjs','specs/practical-sensor-controller-boards/protocol-v2.test.mjs','specs/practical-board-completion-v1/evidence.test.mjs'],
};
assert(mode in modes);const args=modes[mode];
const tc=join(repo,'.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64'),removed=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_OPENAI_LIVE_TEST'];
const env={...process.env,PATH:tc+'/bin:'+process.env.PATH,GOROOT:tc,GOTOOLCHAIN:'local',GOENV:'off',GOWORK:'off',GOFLAGS:'',GOEXPERIMENT:'',CGO_ENABLED:'1',GOMAXPROCS:'4',GOCACHE:repo+'/.cache/go/build',GOMODCACHE:repo+'/.cache/go/mod',GOPROXY:'off',GOSUMDB:'off'};for(const k of removed)delete env[k];
const sha=b=>createHash('sha256').update(b).digest('hex'),secret=process.env.OPENAI_API_KEY;
const git=(...a)=>execFileSync('git',a,{cwd:repo,env,encoding:'utf8'}).trim();
const revision=git('rev-parse','HEAD');
const snapshot=()=>{
 const paths=[...new Set([...git('diff','--name-only','HEAD').split('\n'),...git('ls-files','--others','--exclude-standard', 'internal', 'cmd', 'specs/practical-board-completion-v1').split('\n'),'internal/architecturesearch/reference_domains.go','internal/architecturesearch/reference_domains_test.go'])].filter(p=>p&&existsSync(join(repo,p))).sort();
 return paths.map(path=>{const b=readFileSync(join(repo,path));assert(!secret||!b.includes(Buffer.from(secret)));return {path,bytes:b.length,sha256:sha(b),source:b.toString()};});
};
const before=snapshot(),parent=join(repo,'.cache/practical-board-completion-v1/checks');mkdirSync(parent,{recursive:true});const output=join(parent,id);mkdirSync(output,{mode:0o700});
const write=(name,v)=>writeFileSync(join(output,name),JSON.stringify(v,null,2)+'\n',{flag:'wx',mode:0o600});write('source.json',{revision,files:before});
const started=new Date().toISOString(),chunks=[];let limitFailure=null,error=null,bytes=0;
const command=mode==='lint'?'/Users/dshills/Development/Go/bin/golangci-lint':mode==='node'?process.execPath:tc+'/bin/go';
const child=spawn(command,args,{cwd:repo,env,detached:true,stdio:['ignore','pipe','pipe']});
const kill=()=>{try{process.kill(-child.pid,'SIGKILL');}catch{}};
const timeoutMs=mode==='full'?7200000:900000;
const timer=setTimeout(()=>{limitFailure='overall_check_deadline';kill();},timeoutMs);
for(const stream of [child.stdout,child.stderr])stream.on('data',b=>{bytes+=b.length;if(bytes<=32*1024*1024)chunks.push(b);else{limitFailure='32_mib_log_limit';kill();}});
child.on('error',e=>{error=e.message;});
const terminal=await new Promise(resolve=>child.on('close',(code,signal)=>resolve({code,signal})));clearTimeout(timer);
const log=Buffer.concat(chunks);assert(!secret||!log.includes(Buffer.from(secret)));writeFileSync(join(output,'output.log'),log,{flag:'wx',mode:0o600});
const unchanged=git('rev-parse','HEAD')===revision&&JSON.stringify(snapshot())===JSON.stringify(before);
const receipt={schema:'kicadai.completion-check.v1',id,mode,revision,command,args,started_utc:started,finished_utc:new Date().toISOString(),...terminal,error,limit_failure:limitFailure,source_snapshot_sha256:sha(readFileSync(join(output,'source.json'))),source_unchanged:unchanged,log_sha256:sha(log),provider_calls:0,board_attempts:0,removed_provider_variables:removed,dependency_network:'disabled'};
write('execution.json',receipt);console.log(JSON.stringify(receipt,null,2));console.log(log.toString().slice(-10000));
assert(unchanged);assert(!limitFailure&&!error);process.exitCode=terminal.code??1;
