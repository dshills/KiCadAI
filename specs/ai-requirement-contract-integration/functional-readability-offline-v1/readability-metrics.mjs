// Descriptive locality measurements, not a readability acceptance threshold.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),root='/tmp/kicadai-functional-readability-offline-v1-final',prior='/tmp/kicadai-pcb-seed-rotated-fields-offline-v1-final';
const json=p=>JSON.parse(readFileSync(p));
const distance=(a,b)=>Math.hypot(a.x_mm-b.x_mm,a.y_mm-b.y_mm);
const cases=[];
for(const name of ['standalone_regulator','controller_adc_100ma']){
 const req=json(join(root,name,'workflow_request.json')),sch=req.explicit_circuit.schematic,tx=json(join(root,name,'schematic_transaction.json')),oldTx=json(join(prior,name,'schematic_transaction.json'));
 const current=new Map(tx.operations.filter(o=>o.op==='add_symbol').map(o=>[o.ref,o.at])),old=new Map(oldTx.operations.filter(o=>o.op==='add_symbol').map(o=>[o.ref,o.at]));
 const components=new Map(sch.circuit.components.map(c=>[c.id,c])),owners=new Map(sch.layout.functional_owners.map(o=>[o.component,o]));
 const decoupling=[],support=[];
 for(const group of sch.layout.groups){
  const active=group.members.map(id=>components.get(id)).filter(c=>['ic','regulator','sensor'].includes(c.role));
  if(active.length!==1)continue;
  for(const id of group.members){const c=components.get(id);if(c.role!=='decoupling_capacitor')continue;
   const a=active[0];decoupling.push({component:id,reference:c.ref,group:group.id,anchor:a.id,anchor_reference:a.ref,anchor_basis:'only active component in explicitly owned group',previous_mm:distance(old.get(c.ref),old.get(a.ref)),current_mm:distance(current.get(c.ref),current.get(a.ref))});
  }
 }
 for(const owner of owners.values())if(owner.parent){const c=components.get(owner.component),p=components.get(owner.parent);assert(c&&p);support.push({component:c.id,reference:c.ref,parent:p.id,parent_reference:p.ref,previous_mm:distance(old.get(c.ref),old.get(p.ref)),current_mm:distance(current.get(c.ref),current.get(p.ref))});}
 const mean=(list,key)=>list.reduce((n,r)=>n+r[key],0)/list.length;
 cases.push({name,groups:sch.layout.groups.map(g=>({id:g.id,label:g.label,members:g.members.map(id=>components.get(id).ref)})),decoupling,mean_decoupling_distance_mm:{previous:mean(decoupling,'previous_mm'),current:mean(decoupling,'current_mm')},explicit_support_parents:support,mean_support_parent_distance_mm:{previous:mean(support,'previous_mm'),current:mean(support,'current_mm')},paper:{previous:oldTx.operations[0].paper,previous_portrait:!!oldTx.operations[0].paper_portrait,current:tx.operations[0].paper,current_portrait:!!tx.operations[0].paper_portrait}});
}
const result={schema:'kicadai.functional-locality-descriptive.v1',root,prior,cases,method:'Euclidean symbol-origin distance in millimeters from sealed generation transactions. Group anchors require exactly one active component. Support-parent values use recorded explicit ownership. These are descriptive post-run measures, not preregistered pass thresholds or PCB distances.',readability_passes:0,practical_benchmark_passes_added:0};
const path=join(phase,'readability-metrics.json');if(existsSync(path))assert.deepEqual(json(path),result);else writeFileSync(path,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify(result,null,2));
