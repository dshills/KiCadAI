// Read-only, offline verification of originals, native evidence and source.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {readFileSync,readdirSync,existsSync} from 'node:fs';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),repo=resolve(phase,'../../..');
const root=resolve(process.argv[2]||'/tmp/kicadai-pin-aware-readability-offline-v1-final');
const previous='/tmp/kicadai-ownership-locality-offline-v2-final';
const base='f8b29a9904dd02c38583cb4c15d72f55b5ee6b75';
const hash=b=>createHash('sha256').update(b).digest('hex');
const json=p=>JSON.parse(readFileSync(p,'utf8'));
const walk=(dir,prefix='',primary=false)=>readdirSync(join(dir,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{
  assert(!e.isSymbolicLink(),'No evidence symlinks');
  if(primary&&!prefix&&e.name==='.kicadai')return [];
  const p=join(prefix,e.name); return e.isDirectory()?walk(dir,p,primary):[p];
});
const inventory=(dir,primary=false)=>walk(dir,'',primary).map(path=>{const b=readFileSync(join(dir,path));return {path,bytes:b.length,sha256:hash(b)};});
// Strictly consume generated KiCad S-expressions, including quoted strings.
// This independent audit is deliberately not a general KiCad file reader.
function sexpr(text) {
  const token=/\s+|\(|\)|"(?:\\.|[^"\\])*"|[^\s()"]+/gy;
  const stack=[],roots=[]; let at=0,m;
  while(at<text.length){token.lastIndex=at;m=token.exec(text);assert(m,'Invalid S-expression token');at=token.lastIndex;const t=m[0];if(/^\s/.test(t))continue;
    if(t==='('){const a=[];(stack.at(-1)||roots).push(a);stack.push(a);}
    else if(t===')'){assert(stack.length,'Unmatched close');stack.pop();}
    else {assert(stack.length,'Atom outside expression');stack.at(-1).push(t.startsWith('"')?JSON.parse(t):t);}}
  assert.equal(stack.length,0);assert.equal(roots.length,1);return roots[0];
}
const children=(node,key)=>node.filter(n=>Array.isArray(n)&&n[0]===key);
function boardAudit(path,components) {
  const board=sexpr(readFileSync(path,'utf8'));assert.equal(board[0],'kicad_pcb');
  const nets=new Map(children(board,'net').map(n=>[n[1],n[2]]));assert.equal(nets.size,children(board,'net').length);
  const footprints=children(board,'footprint'),seen=new Set();let connectedPads=0,unconnectedPads=0;
  assert.equal(footprints.length,components.length);
  for(const f of footprints){const refs=children(f,'property').filter(p=>p[1]==='Reference');assert.equal(refs.length,1);const ref=refs[0][2];assert(!seen.has(ref));seen.add(ref);
    const component=components.find(c=>c.reference===ref);assert(component,'Unexpected footprint '+ref);
    const expected=new Map(component.pads.map(p=>[p.name,p.net||''])),actual=new Map();
    assert.equal(expected.size,component.pads.length);
    for(const p of children(f,'pad')){const net=children(p,'net');assert(net.length<=1);const name=net.length?nets.get(net[0][1]):'';assert.notEqual(name,undefined);if(net[0]?.length===3)assert.equal(net[0][2],name);
      if(actual.has(p[1]))assert.equal(actual.get(p[1]),name,'Duplicate pad has inconsistent net');actual.set(p[1],name);}
    for(const [pad,net] of expected)assert.equal(actual.get(pad),net,ref+'.'+pad+' PCB net');
    for(const [pad,net] of actual){if(net){assert.equal(expected.get(pad),net,'Unexpected connected pad '+ref+'.'+pad);connectedPads++;}else{assert(!expected.get(pad),'Expected connected pad is unconnected');unconnectedPads++;}}
  }
  // Across phases only: resolve numeric net identifiers to names and ignore
  // drawing-paper orientation. Same-phase replay above remains byte-exact.
  const normalize=node=>!Array.isArray(node)?node:node[0]==='net'?['net',nets.get(node[1])]:node.map(normalize);
  const normalized=board.filter(n=>!Array.isArray(n)||!['paper','net'].includes(n[0])).map(normalize);
  normalized.push(...[...nets.values()].sort().map(n=>['net',n]));
  const placements=footprints.map(f=>({reference:children(f,'property').find(p=>p[1]==='Reference')[2],at:children(f,'at')[0]})).sort((a,b)=>a.reference.localeCompare(b.reference,'en'));
  const routes=normalized.filter(n=>Array.isArray(n)&&['segment','via'].includes(n[0]));
  return {normalized,connectedPads,unconnectedPads,paper:children(board,'paper')[0],placements,routes_sha256:hash(JSON.stringify(routes)),segments:children(board,'segment').length,vias:children(board,'via').length};
}
const stages=['schematic','schematic_electrical','placement','routing','project_write','writer_correctness','validation','simulation','kicad_checks'];
const seal=json(join(root,'sealed-projects.json')),after=json(join(root,'seal-after-render.json'));
assert.equal(after.seal_sha256,hash(readFileSync(join(root,'sealed-projects.json'))));
assert.equal(after.originals_unchanged,true);
assert(seal.projects.length>0&&seal.projects.length<=4);
const cases=[];
for(const name of ['standalone_regulator','controller_adc_100ma']) {
  const dir=join(root,name),req=json(join(dir,'workflow_request.json')),prior=json(join(previous,name,'workflow_request.json'));
  assert(readFileSync(join(dir,'requirement.json')).equals(readFileSync(join(previous,name,'requirement.json'))),'Requirement changed');
  assert.equal(req.explicit_circuit.placement_seed?.policy,'annotation-independent-v1');
  assert.equal(req.explicit_circuit.placement_seed.hash,prior.explicit_circuit.placement_seed.hash,'Historical seed not preserved');
  for(const key of ['board','constraints','validation','routing_retry','fabrication','intent','name','libraries','component_policy'])assert.deepEqual(req[key],prior[key],key+' changed');
  for(const key of ['keepouts','zones'])assert.deepEqual(req.explicit_circuit[key],prior.explicit_circuit[key],key+' changed');
  const invariant=['components','nets','regions','simulation','schematic_support','routing_policy','catalog_id','catalog_hash'];
  for(const key of invariant)assert.deepEqual(req.explicit_circuit[key],prior.explicit_circuit[key],key+' changed');

  assert.equal(req.explicit_circuit.schematic.layout.native_profile,'annotation-v2');
  assert.equal(req.explicit_circuit.schematic.layout.functional_profile,'ownership-v3');
  // Full request equality except the explicitly versioned drawing layout.
  const physicalRequest=structuredClone(req);
  physicalRequest.explicit_circuit.schematic.layout=prior.explicit_circuit.schematic.layout;
  assert.deepEqual(physicalRequest,prior,'Non-layout request input changed');
  const owners=req.explicit_circuit.schematic.layout.functional_owners,groups=req.explicit_circuit.schematic.layout.groups;
  assert.equal(owners.length,req.explicit_circuit.schematic.circuit.components.length);
  assert.equal(new Set(owners.map(o=>o.component)).size,owners.length);
  for(const o of owners){const g=groups.find(g=>g.id===o.group);assert(g&&g.members.includes(o.component));if(o.parent)assert.equal(owners.find(p=>p.component===o.parent)?.group,o.group);}

  const circuitIntent=circuit=>{const c=structuredClone(circuit);for(const component of c.components){if(component.properties)delete component.properties['KiCadAI Resolution Hash'];}return c;};
  const resolutionHashes=circuit=>[...new Set(circuit.components.map(c=>c.properties?.['KiCadAI Resolution Hash']).filter(Boolean))];
  const currentResolution=resolutionHashes(req.explicit_circuit.schematic.circuit),previousResolution=resolutionHashes(prior.explicit_circuit.schematic.circuit);
  assert.equal(currentResolution.length,1);assert.equal(previousResolution.length,1);
  assert(/^[a-f0-9]{64}$/.test(currentResolution[0])&&/^[a-f0-9]{64}$/.test(previousResolution[0]));
  assert.deepEqual(circuitIntent(req.explicit_circuit.schematic.circuit),circuitIntent(prior.explicit_circuit.schematic.circuit),'Schematic circuit intent changed apart from versioned resolution provenance');
  const promotion=json(join(dir,'promotion.json'));
  assert.equal(promotion.report.status,'pass');
  const selected=promotion.report.candidates.filter(c=>c.fingerprint===promotion.report.selected.fingerprint);
  assert.equal(selected.length,1);
  const attempt=selected[0].attempts.at(-1),behaviors=json(join(dir,'requirement.json')).requirements.behavioral_requirements;
  assert.equal(attempt.assertions.length,behaviors.length);
  for(const b of behaviors){const matches=attempt.assertions.filter(a=>a.requirement_id===b.id);assert.equal(matches.length,1);const a=matches[0];assert.equal(a.pass,true);assert(Number.isFinite(a.actual));if(b.min!==undefined)assert(a.actual>=b.min);if(b.max!==undefined)assert(a.actual<=b.max);}
  const workflowRuns=['first','second'].filter(run=>existsSync(join(dir,run+'_workflow.json'))).map(run=>({run,result:json(join(dir,run+'_workflow.json'))}));
  const complete=workflowRuns.length===2&&workflowRuns.every(({result})=>stages.every(name=>result.stages.find(s=>s.name===name)?.status==='ok'));
  if(!complete){
    const runs=workflowRuns.map(({run,result})=>({run,stages:stages.map(name=>{const stage=result.stages.find(s=>s.name===name);return {name,status:stage?.status??'absent',issues:stage?.issues??[]};}),native_checks:result.stages.find(s=>s.name==='kicad_checks')?.summary??null,acceptance:result.acceptance}));
    for(const p of seal.projects.filter(p=>p.name===name))assert.deepEqual(inventory(join(dir,p.run),true),p.files);
    const replay=existsSync(join(dir,'replayed_workflow_request.json'));
    if(replay)assert(readFileSync(join(dir,'workflow_request.json')).equals(readFileSync(join(dir,'replayed_workflow_request.json'))));
    cases.push({name,technical_gate_passed:false,requirements_unchanged:true,non_layout_request_unchanged:true,assertions:attempt.assertions,runs,project_files:seal.projects.filter(p=>p.name===name).map(p=>({run:p.run,files:p.files.length})),actual_json_serialized_request_replay:replay,byte_exact_generated_project_replay:false,emitted_pin_pad_and_geometry_audit:'not certified: full native project did not pass',reason:'A failed or absent required stage cannot be counted as a technical or readability pass'});
    continue;
  }
  assert(readFileSync(join(dir,'workflow_request.json')).equals(readFileSync(join(dir,'replayed_workflow_request.json'))),'Serialized request differs');
  const runs=[];
  for(const run of ['first','second']) {
    const r=json(join(dir,run+'_workflow.json'));
    const audit=json(join(dir,run+'_annotation_audit.json'));
    assert(!audit.issues||audit.issues.length===0,'Emitted annotation audit failed');
    assert(audit.note_lines>0&&audit.labels>0&&audit.symbols>0,'Incomplete annotation audit');
    assert.equal(new Set(r.stages.map(s=>s.name)).size,r.stages.length);
    const stage=name=>r.stages.find(s=>s.name===name);
    for(const s of stages)assert.equal(stage(s)?.status,'ok',name+'/'+run+'/'+s);
    assert.equal(stage('routing').summary.failed_nets,0);
    assert.equal(stage('routing').summary.routed_nets,stage('routing').summary.net_count);
    assert.equal(stage('writer_correctness').summary.skipped_count,0);
    assert.equal(stage('writer_correctness').summary.fail_count,0);
    for(const kind of ['erc','drc']) {
      const summary=stage('kicad_checks').summary,check=summary[kind];
      assert.equal(summary[kind+'_required'],true);assert.equal(check.status,'pass');assert.equal(check.project_context,'full');
      assert(check.command.includes('--severity-all')&&check.command.includes('--exit-code-violations'));
      assert.equal(check.summary.total_findings,0);
      assert(resolve(check.report_path).startsWith(resolve(dir,run)+'/'));
      const raw=json(check.report_path);
      if(kind==='drc'){assert.deepEqual(raw.violations,[]);assert.deepEqual(raw.unconnected_items,[]);}
      else {assert(raw.sheets.length>0);assert(raw.sheets.every(s=>s.violations.length===0));}
    }
    const sealed=seal.projects.filter(p=>p.name===name&&p.run===run);assert.equal(sealed.length,1);
    assert.deepEqual(inventory(join(dir,run),true),sealed[0].files,'Original seal changed');
    for(const ext of ['.kicad_sch','.kicad_pcb','.kicad_pro'])assert(sealed[0].files.some(f=>f.path.endsWith(ext)));
    runs.push({run,required_stages:stages.length,erc_findings:0,drc_findings:0,writer_skips:0,achieved:r.acceptance.achieved,fabrication_ready:r.acceptance.fabrication_ready});
  }
  const first=seal.projects.find(p=>p.name===name&&p.run==='first').files,second=seal.projects.find(p=>p.name===name&&p.run==='second').files;
  assert.deepEqual(first,second,'Whole generated-project replay differs');
  const pcbFiles=first.filter(f=>f.path.endsWith('.kicad_pcb'));assert.equal(pcbFiles.length,1);
  const pcbPath=pcbFiles[0].path,pcb=boardAudit(join(dir,'first',pcbPath),req.explicit_circuit.components),oldPCB=boardAudit(join(previous,name,'first',pcbPath),prior.explicit_circuit.components);
  const pcbUnchanged=JSON.stringify(pcb.normalized)===JSON.stringify(oldPCB.normalized);
  const changedPlacements=pcb.placements.filter(p=>JSON.stringify(p)!==JSON.stringify(oldPCB.placements.find(old=>old.reference===p.reference)));
  assert.equal(pcbUnchanged,true,'PCB geometry changed');
  assert.equal(changedPlacements.length,0);
  assert.equal(pcb.routes_sha256,oldPCB.routes_sha256);
  // KiCad netlist export intentionally omits virtual power symbols; their ERC
  // receipts remain mandatory above. All physical symbol endpoints are exact.
  const sch=req.explicit_circuit.schematic.circuit,physical=new Set(req.explicit_circuit.components.map(c=>c.reference));
  assert.equal(physical.size,req.explicit_circuit.components.length);
  assert([...physical].every(ref=>typeof ref==='string'&&ref.length>0));
  const refs=new Map(sch.components.map(c=>[c.id,c.ref]));
  const xml=readFileSync(join(dir,'render','native-netlist.xml'),'utf8');
  const actual=new Map([...xml.matchAll(/<net code="[^"]+" name="([^"]+)"[^>]*>([\s\S]*?)<\/net>/g)].map(m=>[m[1].replace(/^\//,''),[...m[2].matchAll(/<node ref="([^"]+)" pin="([^"]+)"/g)].map(p=>p[1]+'.'+p[2]).sort()]));
  assert.equal(actual.size,sch.nets.filter(n=>n.role!=='no_connect').length,'Unexpected native net count');
  let endpoints=0;
  for(const net of sch.nets){
    if(net.role==='no_connect')continue;
    const expected=[...new Set(net.connect.map(e=>{const i=e.lastIndexOf('.'),ref=refs.get(e.slice(0,i));assert(ref);return {ref,pin:e.slice(i+1)};}).filter(e=>physical.has(e.ref)).map(e=>e.ref+'.'+e.pin))].sort();
    assert.deepEqual(actual.get(net.name),expected,'Native physical pin mismatch: '+net.name);endpoints+=expected.length;
  }
  cases.push({name,technical_gate_passed:true,components:physical.size,requirements_unchanged:true,resolution_provenance:{previous:previousResolution[0],current:currentResolution[0],cross_phase_exclusion:'only KiCadAI Resolution Hash; all other schematic circuit fields exact'},electrical_and_pcb_input_invariants:invariant,runs,native_physical_nets:sch.nets.length,native_physical_endpoints:endpoints,pcb:{connected_pads:pcb.connectedPads,unconnected_pads:pcb.unconnectedPads,physical_connectivity_verified:true,geometry_unchanged:pcbUnchanged,changed_placement_count:changedPlacements.length,previous_placements:oldPCB.placements,current_placements:pcb.placements,previous_route_sha256:oldPCB.routes_sha256,current_route_sha256:pcb.routes_sha256,previous_segments:oldPCB.segments,current_segments:pcb.segments,previous_vias:oldPCB.vias,current_vias:pcb.vias,previous_generation_seed:prior.explicit_circuit.generation_hash,current_generation_seed:req.explicit_circuit.generation_hash,current_placement_seed:req.explicit_circuit.placement_seed.hash,comparison_to_previous_phase:'net IDs resolved to names; drawing-paper setting excluded; all remaining board fields, including placements and routes, compared exactly',previous_paper:oldPCB.paper,current_paper:pcb.paper},project_files:first.length,actual_json_serialized_request_replay:true,byte_exact_generated_project_replay:true,normalization:'none',assertions:attempt.assertions});
}
const git=(...a)=>execFileSync('git',a,{cwd:repo,encoding:'utf8'}).trim();
const sourcePaths=[...new Set([...git('diff',base,'--name-only','--diff-filter=ACMR','--','internal').split('\n'),...git('ls-files','--others','--exclude-standard','--','internal').split('\n')])].filter(p=>p.endsWith('.go')).sort();
const source=sourcePaths.map(path=>({path,sha256:hash(readFileSync(join(repo,path)))}));
const label=/-(dev[123])$/.test(root)?'native-'+root.match(/-(dev[123])$/)[1]:'native-final';
const snapshot=json(join(repo,'.cache/pin-aware-readability-v1-sources',label+'.json'));
assert.deepEqual(source,snapshot.files.map(f=>({path:f.path,sha256:f.sha256})),'Final source changed after freeze');
for(const f of snapshot.files)assert.equal(hash(Buffer.from(f.source)),f.sha256);
const raw=inventory(root);
const result={schema:'kicadai.pin-aware-readability-offline-verification.v1',base,root,source,cases,inventory:{file_count:raw.length,total_bytes:raw.reduce((n,f)=>n+f.bytes,0),sha256:hash(JSON.stringify(raw))},provider_calls:0,frozen_cases_added:0,technical_gate_passed:cases.every(c=>c.technical_gate_passed),technical_examples_passed:cases.filter(c=>c.technical_gate_passed).length,complete_practical_benchmark_cases:0,planned_examples:2,readability:'separate visual review; native checks do not establish readability'};
const receipt=join(phase,label==='native-final'?'verification.json':label+'-verification.json');
if(existsSync(receipt))assert.deepEqual(result,json(receipt),'Evidence or source changed since receipt');
console.log(JSON.stringify(result,null,2));
