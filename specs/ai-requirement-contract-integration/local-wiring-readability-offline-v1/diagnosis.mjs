// Bind every offline diagnostic, including all three negative candidate checks.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,createReadStream} from 'node:fs';
import {createHash} from 'node:crypto';
import {dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),prior='/tmp/kicadai-joint-annotation-placement-offline-v1-final';
const sha=b=>createHash('sha256').update(b).digest('hex'),json=p=>JSON.parse(readFileSync(p));
async function digest(path){const h=createHash('sha256');let bytes=0;for await(const b of createReadStream(path)){h.update(b);bytes+=b.length;}return {path,bytes,sha256:h.digest('hex')};}
const inputs=await Promise.all(['controller_adc_100ma/workflow_request.json','controller_adc_100ma/schematic_transaction.json','library_index.json'].map(p=>digest(join(prior,p))));
const diagnostics=[];
for(const n of [1,2,3]){
 const layout=json(join(phase,`diagnostic-layout-${n}.execution.json`)),candidate=json(join(phase,`diagnostic-candidates-${n}.execution.json`));
 const txBytes=readFileSync(join(phase,`diagnostic-transaction-${n}.json`)),tx=JSON.parse(txBytes),log=readFileSync(join(phase,`diagnostic-candidates-${n}.log`),'utf8');
 assert.equal(layout.code,0);assert.equal(candidate.code,1);
 for(const r of [layout,candidate]){assert.equal(r.source_snapshot_sha256,sha(readFileSync(r.source_snapshot)));assert.equal(r.log_sha256,sha(readFileSync(join(phase,r.mode+'.log'))));assert.equal(r.source_unchanged,true);}
 assert(log.includes('sha256='+sha(txBytes)));assert(log.includes('library_index.json sha256='+inputs[2].sha256));
 const failures=log.split('\n').filter(l=>l.includes('placement_error=')||l.includes('count=0 origin='));assert(failures.length>1);
 const branches=tx.operations.filter(o=>o.op==='connect');
 diagnostics.push({number:n,layout_exit_code:layout.code,candidate_exit_code:candidate.code,layout_source:layout.source_snapshot_sha256,candidate_source:candidate.source_snapshot_sha256,transaction_sha256:sha(txBytes),operations:tx.operations.length,paper:tx.operations[0].paper,portrait:!!tx.operations[0].paper_portrait,direct_branches:branches.filter(o=>o.use_labels===false).length,label_branches:branches.filter(o=>o.use_labels===true).length,failures});
}
const result={schema:'kicadai.local-wiring-diagnosis.v1',inputs,diagnostics,conclusions:['Connector/high-fanout policy hid most local supply conductors. Full net-name placement corridors increased support spacing.','Blindly shortening every local corridor blocked PF2 and an ADC net label. Retaining signal and cross-group singleton corridors was necessary.','Early direct conductors blocked later connector labels. Routing now reserves narrow future-label corridors for foreign nets.','Ground labels still needed full placement clearance; only multi-endpoint canonical power nets receive shorter placement reservations.','The last two native development iterations balanced the group packing and removed duplicate outer padding while preserving the declared gutter.'],final_transaction_not_claimed_equal_to_earlier_diagnostics:true,diagnostics_are_native_evaluations:false,provider_calls:0,benchmark_passes_added:0};
writeFileSync(join(phase,'diagnosis.json'),JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({diagnostics:diagnostics.length,negative_candidate_checks:3,inputs:inputs.map(x=>({path:x.path,sha256:x.sha256}))}));
