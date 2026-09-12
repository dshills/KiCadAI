// Store the read-only verifier's exact JSON, never converting failure to pass.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {writeFileSync,existsSync,readFileSync} from 'node:fs';
import {dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url));
for(const label of (process.argv.slice(2).length?process.argv.slice(2):['dev1','final'])){
 const root='/tmp/kicadai-power-locality-offline-v1-'+label;
 const env={...process.env};for(const key of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[key];
 const text=execFileSync(process.execPath,[join(phase,'verify.mjs'),root],{env,encoding:'utf8',maxBuffer:16*1024*1024}),value=JSON.parse(text);
 const file=join(phase,label==='final'?'verification.json':'native-'+label+'-verification.json');
 if(existsSync(file))assert.deepEqual(JSON.parse(readFileSync(file)),value);else writeFileSync(file,text,{flag:'wx'});
 console.log(JSON.stringify({label,inventory:value.inventory,technical_examples_passed:value.technical_examples_passed,phase_passed:value.technical_gate_passed}));
}
