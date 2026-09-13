// Account for every bounded native invocation, including negative outcomes.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {join,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),json=p=>JSON.parse(readFileSync(p)),sha=b=>createHash('sha256').update(b).digest('hex');
const attempts=[];
for(const label of ['dev1','dev2','dev3','final']){
 if(!existsSync(join(phase,'native-'+label+'.execution.json')))continue;
 const receipt=json(join(phase,'native-'+label+'.execution.json')),snapshot=json(receipt.source_snapshot);
 assert.equal(receipt.source_snapshot_sha256,sha(readFileSync(receipt.source_snapshot)));assert.equal(receipt.log_sha256,sha(readFileSync(join(phase,'native-'+label+'.log'))));assert.equal(receipt.source_unchanged,true);
 for(const f of snapshot.files){assert.equal(Buffer.byteLength(f.source),f.bytes);assert.equal(sha(Buffer.from(f.source)),f.sha256);}
 const cases=[];
 for(const name of ['standalone_regulator','controller_adc_100ma']){
  const runs=[];
  for(const run of ['first','second']){const file=join(receipt.root,name,run+'_workflow.json');if(!existsSync(file)){runs.push({run,status:'not_attempted'});continue;}
   const r=json(file),expected=['schematic','schematic_electrical','placement','routing','project_write','writer_correctness','validation','simulation','kicad_checks'];
   runs.push({run,status:expected.every(name=>r.stages.find(s=>s.name===name)?.status==='ok')?'pass':'fail',receipt_sha256:sha(readFileSync(file)),stages:expected.map(name=>{const s=r.stages.find(s=>s.name===name);return {name,status:s?.status??'absent',issues:s?.issues??[]};}),acceptance:r.acceptance});
  }
  cases.push({name,runs,complete_technical_pass:runs.every(r=>r.status==='pass')});
 }
 attempts.push({label,root:receipt.root,exit_code:receipt.code,started_utc:receipt.started_utc,finished_utc:receipt.finished_utc,source_snapshot_sha256:receipt.source_snapshot_sha256,source_files:snapshot.files.length,cases});
}
assert(attempts.length>=2&&attempts.length<=4);const final=attempts.at(-1),dev=attempts.at(-2);assert.equal(final.label,'final');
assert.deepEqual(json(join(final.root,'standalone_regulator/workflow_request.json')),json(join(dev.root,'standalone_regulator/workflow_request.json')));
assert.deepEqual(json(join(final.root,'controller_adc_100ma/workflow_request.json')),json(join(dev.root,'controller_adc_100ma/workflow_request.json')));
assert.deepEqual(json(join(final.root,'sealed-projects.json')).projects,json(join(dev.root,'sealed-projects.json')).projects);
const result={schema:'kicadai.pin-aware-bounded-attempts.v1',attempts,development_attempts:attempts.length-1,frozen_final_attempts:1,final_matches_last_development_inputs_and_available_primary_projects:true,provider_calls:0,benchmark_passes_added:0};
const file=join(phase,'attempts.json');if(existsSync(file))assert.deepEqual(json(file),result);else writeFileSync(file,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({attempts:attempts.map(a=>({label:a.label,exit_code:a.exit_code,technical_passes:a.cases.filter(c=>c.complete_technical_pass).length})),final_matches_last_development:true}));
