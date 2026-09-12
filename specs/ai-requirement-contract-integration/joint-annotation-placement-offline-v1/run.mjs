// Bounded, key-free execution with create-exclusive logs and exact source.
import assert from 'node:assert/strict';
import {spawn,execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {readFileSync,writeFileSync,mkdirSync,existsSync,statfsSync,openSync,writeSync,closeSync} from 'node:fs';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..'),mode=process.argv[2];
const base='7647dda9ba4d94d57213fe8f7ecaf5d474c667fe',prefix='kicadai-joint-annotation-placement-offline-v1';
const hash=b=>createHash('sha256').update(b).digest('hex');
const write=(path,obj)=>writeFileSync(path,JSON.stringify(obj,null,2)+'\n',{flag:'wx'});
const git=(...a)=>execFileSync('git',a,{cwd:repo,encoding:'utf8'}).trim().split('\n').filter(Boolean);
const paths=[...new Set([...git('diff',base,'--name-only','--','internal'),...git('ls-files','--others','--exclude-standard','--','internal')])].filter(p=>p.endsWith('.go')).sort();
const files=paths.map(path=>{const b=readFileSync(join(repo,path));return {path,bytes:b.length,sha256:hash(b),source:b.toString('utf8')};});
const sourceRoot=join(repo,'.cache/joint-annotation-placement-v1-sources');
const toolchain=join(repo,'.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64');
const env={...process.env,PATH:join(toolchain,'bin')+':'+process.env.PATH,GOROOT:toolchain,GOTOOLCHAIN:'local',GOENV:'off',GOWORK:'off',GOFLAGS:'',GOEXPERIMENT:'',CGO_ENABLED:'1',GOMAXPROCS:'4',GOCACHE:join(repo,'.cache/go/build'),GOMODCACHE:join(repo,'.cache/go/mod'),GOPROXY:'off',GOSUMDB:'off',GOLANGCI_LINT_CACHE:join(repo,'.cache/golangci-lint')};
const removed=['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_OPENAI_LIVE_TEST'];for(const k of removed)delete env[k];
let command=join(toolchain,'bin/go'),args,timeout=30*60*1000,root;
const packages=['./internal/compositionlowering','./internal/schematicir','./internal/schematiclayout','./internal/kicadfiles/designapi','./internal/transactions'];
if(/^native-(dev[123]|final)$/.test(mode)){
  const label=mode.slice(7);root=join('/tmp',prefix+'-'+label);assert(!existsSync(root));
  assert(!existsSync(join(sourceRoot,'native-final.json')),'No native execution after final starts');
  const fs=statfsSync(repo);assert(fs.bavail*fs.bsize>12*1024**3,'Insufficient free space with evidence reserve');
  env.KICADAI_JOINT_REPLAY_ARTIFACTS=root;
  env.KICADAI_KICAD_CLI='/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';
  env.KICADAI_SYMBOLS_ROOT='/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols';
  env.KICADAI_FOOTPRINTS_ROOT='/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints';
  args=['test','./internal/compositionlowering','-run','^TestFunctionalJointSerializedReplay$','-v','-count=1','-timeout=45m'];timeout=46*60*1000;
}else if(mode==='full')args=['test','./...','-short','-count=1','-timeout=12m'];
else if(mode==='race-layout')args=['test','-race','./internal/schematiclayout','./internal/schematicir','-short','-count=1','-timeout=12m'];
else if(mode==='race-designapi')args=['test','-race','./internal/kicadfiles/designapi','-short','-count=1','-timeout=12m'];
else if(mode==='race-ownership')args=['test','-race','./internal/compositionlowering','-run','^TestFunctional(Ownership|PinAware|Joint)','-short','-count=1','-timeout=12m'];
else if(/^focused(?:-dev[23]|-final)?$/.test(mode))args=['test',...packages,'-run','Test(Functional|NativeBlock|NativeJoint)','-count=1','-timeout=3m'];
else if(/^diagnostic-frozen(?:-[2-9])?$/.test(mode)){env.KICADAI_JOINT_DIAGNOSTIC_INPUT='/tmp/kicadai-pin-aware-readability-offline-v1-final';args=['test','./internal/transactions','-run','^TestNativeJointFrozenCandidateDiagnostic$','-v','-count=1','-timeout=5m'];}
else if(/^diagnostic-(layout|candidates)-[1-3]$/.test(mode)){env.KICADAI_JOINT_DIAGNOSTIC_INPUT='/tmp/kicadai-pin-aware-readability-offline-v1-final';env.KICADAI_JOINT_DIAGNOSTIC_TRANSACTION=join(phase,'diagnostic-transaction-'+mode.at(-1)+'.json');args=['test',mode.includes('layout')?'./internal/compositionlowering':'./internal/transactions','-run',mode.includes('layout')?'^TestFunctionalJointRecordedLayoutDiagnostic$':'^TestNativeJointFrozenCandidateDiagnostic$','-v','-count=1','-timeout=5m'];}
else if(mode==='vet')args=['vet','./...'];
else if(/^lint(?:-dev[23]|-final)?$/.test(mode)){command='/Users/dshills/Development/Go/bin/golangci-lint';args=['run','--timeout=12m',...packages];}
else throw new Error('Unknown bounded execution mode');
mkdirSync(sourceRoot,{recursive:true});
const snapshot=join(sourceRoot,mode+'.json');write(snapshot,{base,mode,recorded_utc:new Date().toISOString(),files});
const log=join(phase,mode+'.log'),fd=openSync(log,'wx');
const start=new Date().toISOString();
const child=spawn(command,args,{cwd:repo,env,stdio:['ignore','pipe','pipe'],timeout});
for(const stream of [child.stdout,child.stderr])stream.on('data',b=>{writeSync(fd,b);process.stdout.write(b);});
const result=await new Promise(resolve=>{let error;child.on('error',e=>{error=e.message;});child.on('close',(code,signal)=>resolve({code,signal,error}));});
closeSync(fd);
const source_unchanged=files.every(f=>hash(readFileSync(join(repo,f.path)))===f.sha256);
const receipt={base,mode,command,args,root,started_utc:start,finished_utc:new Date().toISOString(),...result,removed_provider_variables:removed,dependency_network:'disabled',source_snapshot:snapshot,source_snapshot_sha256:hash(readFileSync(snapshot)),source_unchanged,log_sha256:hash(readFileSync(log))};
write(join(phase,mode+'.execution.json'),receipt);console.log(JSON.stringify(receipt));
process.exitCode=result.code??1;
