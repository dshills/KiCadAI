// Deliberately synthetic collector unit-test child. This is NOT a journal
// verifier: its fake journal cannot pass the real Go auditor. Live mode rejects
// its interpreter prefix and non-design-only qualification mode.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
const args=process.argv.slice(2),value=name=>args[args.indexOf(name)+1];
const read=file=>JSON.parse(fs.readFileSync(file,'utf8'));
const write=(file,v)=>fs.writeFileSync(file,JSON.stringify(v)+'\n',{flag:'wx',mode:0o600});
const sha=file=>crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const model='gpt-4.1-mini-2025-04-14';
if(['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'].some(k=>process.env[k])) throw new Error('fixture inherited a provider credential');
if(args.includes('--inspect-indexed-journal')) {
  const root=value('--inspect-indexed-journal'),start=read(path.join(root,'start.json'));
  if(start.selection.original_request==='bad-audit') process.exit(2);
  const names=['request/body.bin','request/receipt.json','response.bin','response/receipt.json','selection/ledger.json','selection/receipt.json','selection/selection.json','start.json'];
  console.log(JSON.stringify({version:'indexed-journal-audit-1',...start,files_sha256:Object.fromEntries(names.map(n=>[n,sha(path.join(root,n))]))}));
  process.exit(0);
}
const root=path.dirname(value('--prompt-file')),prompt=fs.readFileSync(value('--prompt-file'),'utf8'),ledgerFile=value('--ledger'),policy=read(value('--live-budget'));
const ledger=fs.existsSync(ledgerFile)?read(ledgerFile):{version:2,...policy,entries:[]};
const index=ledger.entries.length+1,id=prompt==='duplicate-id'?ledger.entries[0].response_id:path.basename(root)+'-response';
const entry={index,model,status:['crash','timeout','transport'].includes(prompt)?'reserved_unknown_outcome':'completed',reserve_micro_usd:50000,response_id:id,input_tokens:100,output_tokens:200,estimated_micro_usd:360};
if(prompt==='tamper-prefix') ledger.entries[0].response_id='changed-history';
ledger.entries.push(entry);
fs.writeFileSync(ledgerFile,JSON.stringify(ledger)+'\n',{mode:0o600});
if(prompt==='crash') process.exit(74);
if(prompt==='timeout') {setInterval(()=>{},1000);await new Promise(()=>{});}
if(prompt==='log-overflow') {process.stdout.write('x'.repeat(2*1024*1024));setInterval(()=>{},1000);await new Promise(()=>{});}
const journal=value('--evidence-journal'),output=value('--output');
for(const dir of [journal,path.join(journal,'request'),path.join(journal,'response'),path.join(journal,'selection'),output]) fs.mkdirSync(dir,{mode:0o700});
const failure=['invalid','refusal','bad-audit','transport'].includes(prompt),disposition=prompt==='unsupported'?'unsupported':'clarify';
const selection={original_request:prompt,admission_version:'3-indexed-quantities-experimental',ledger_index:index,response_id:id,model,usage:{input_tokens:100,output_tokens:200,total_tokens:300},decision:{disposition,configuration:null}};
const outcome=prompt==='refusal'?'provider_refusal':failure?'invalid_extraction':'decision';
write(path.join(journal,'selection/selection.json'),selection);write(path.join(output,'selection.json'),selection);
write(path.join(journal,'selection/ledger.json'),ledger);
write(path.join(journal,'start.json'),{selection,outcome,policy,ledger});
for(const name of ['request/body.bin','request/receipt.json','response.bin','response/receipt.json','selection/receipt.json']) write(path.join(journal,name),{synthetic_fixture:true,name});
console.log(JSON.stringify({passed:false,disposition:failure?'failed':disposition}));
process.exit(failure||prompt==='native-failure'?1:0);
