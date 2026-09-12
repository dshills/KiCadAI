// Independent emitted-library pin-direction audit. No source-level layout
// helpers, component-name heuristics, network calls or artifact edits.
import assert from 'node:assert/strict';
import {readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {join,dirname,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),root=resolve(process.argv[2]||'/tmp/kicadai-local-wiring-readability-offline-v1-final');
const json=p=>JSON.parse(readFileSync(p));
function parse(text){const token=/\s+|\(|\)|"(?:\\.|[^"\\])*"|[^\s()"]+/gy,stack=[],roots=[];let at=0;while(at<text.length){token.lastIndex=at;const m=token.exec(text);assert(m);at=token.lastIndex;const t=m[0];if(/^\s/.test(t))continue;if(t==='('){const n=[];(stack.at(-1)||roots).push(n);stack.push(n);}else if(t===')'){assert(stack.length);stack.pop();}else {assert(stack.length);stack.at(-1).push(t.startsWith('"')?JSON.parse(t):t);}}assert.equal(stack.length,0);assert.equal(roots.length,1);return roots[0];}
const all=(n,k)=>n.filter(x=>Array.isArray(x)&&x[0]===k),one=(n,k)=>{const a=all(n,k);assert.equal(a.length,1,k);return a[0];};
function descendants(n,k){return n.flatMap(x=>Array.isArray(x)?[...(x[0]===k?[x]:[]),...descendants(x,k)]:[]);}
const cases=[];
for(const name of ['standalone_regulator','controller_adc_100ma']){
 const dir=join(root,name),request=json(join(dir,'workflow_request.json')),sch=request.explicit_circuit.schematic,tx=json(join(dir,'schematic_transaction.json'));
 if(!existsSync(join(dir,'first'))){cases.push({name,status:'not_evaluable',reason:'No emitted native project; writer gate failed',eligible_attachments:0,side_passes:0,notes_intact:false});continue;}
 const files=readdirSync(join(dir,'first')).filter(p=>p.endsWith('.kicad_sch'));
 if(files.length===0){cases.push({name,status:'not_evaluable',reason:'No emitted native schematic; writer gate failed',eligible_attachments:0,side_passes:0,notes_intact:false});continue;}
 assert.equal(files.length,1);
 const native=parse(readFileSync(join(dir,'first',files[0]),'utf8'));
 const libraries=new Map(all(one(native,'lib_symbols'),'symbol').map(s=>[s[1],s]));
 const symbols=new Map(all(native,'symbol').map(s=>[all(s,'property').find(p=>p[1]==='Reference')[2],s]));
 const components=new Map(sch.circuit.components.map(c=>[c.id,c])),owners=new Map(sch.layout.functional_owners.map(o=>[o.component,o]));
 const at=ref=>one(symbols.get(ref),'at').slice(1,3).map(Number);
 function side(ref,number){const s=symbols.get(ref),lib=libraries.get(one(s,'lib_id')[1]);assert(lib);const pins=descendants(lib,'pin').filter(p=>one(p,'number')[1]===number);assert.equal(pins.length,1,ref+'.'+number);
  const angle=Number(one(pins[0],'at')[3])*Math.PI/180;let x=-Math.cos(angle),y=Math.sin(angle);
  const mirror=all(s,'mirror')[0]?.[1];if(mirror==='x')y=-y;else if(mirror==='y')x=-x;
  const rotate=Number(one(s,'at')[3]||0)*Math.PI/180;[x,y]=[x*Math.cos(rotate)+y*Math.sin(rotate),-x*Math.sin(rotate)+y*Math.cos(rotate)];
  assert(Math.abs(x)<1e-6||Math.abs(y)<1e-6,'Non-cardinal pin');return Math.abs(x)>Math.abs(y)?(x<0?'left':'right'):(y<0?'top':'bottom');
 }
 const attachments=[];
 for(const c of components.values()){
  let parent=components.get(owners.get(c.id)?.parent),basis='explicit support parent';
  if(!parent&&['decoupling_capacitor','bulk_capacitor'].includes(c.role)){const g=sch.layout.groups.find(g=>g.members.includes(c.id)),active=g.members.map(id=>components.get(id)).filter(c=>['ic','regulator','sensor'].includes(c.role));if(active.length===1){parent=active[0];basis='unique active in owned group';}}
  if(!parent)continue;
  const shared=[];
  for(const n of sch.circuit.nets){if(['ground','return','no_connect'].includes(n.role)||!n.connect.some(e=>e.slice(0,e.lastIndexOf('.'))===c.id))continue;
   for(const e of n.connect){const i=e.lastIndexOf('.');if(e.slice(0,i)!==parent.id)continue;const pin=e.slice(i+1);shared.push({net:n.name,role:n.role,pin,side:side(parent.ref,pin),priority:['power','power_pos','power_neg'].includes(n.role)?0:1});}}
  const priority=Math.max(-1,...shared.map(p=>p.priority)),selected=shared.filter(p=>p.priority===priority),sides=[...new Set(selected.map(p=>p.side))];
  const attachment=sides.length===1?sides[0]:sides.length>1?'ambiguous':'unknown';
  const a=at(c.ref),b=at(parent.ref),delta=[a[0]-b[0],a[1]-b[1]];
  const passed=attachment==='left'?delta[0]<0:attachment==='right'?delta[0]>0:attachment==='top'?delta[1]<0:attachment==='bottom'?delta[1]>0:null;
  attachments.push({reference:c.ref,parent:parent.ref,basis,shared,attachment,delta_mm:delta,symbol_origin_on_pin_side:passed});
 }
 const blocks=tx.operations[0].native_schematic_blocks;assert.equal(blocks.length,sch.layout.groups.length);assert(!tx.operations[0].native_schematic_notes);
 const texts=all(native,'text'),wrapped=paragraphs=>paragraphs.flatMap(p=>{const lines=[];let line='';for(const word of p.split(/\s+/).filter(Boolean)){if(line&&Array.from(line+' '+word).length>64){lines.push(line);line='';}line+=(line?' ':'')+word;}lines.push(line);return lines;});
 const blockEvidence=[],used=new Set();
 for(const block of blocks.toSorted((a,b)=>a.id.localeCompare(b.id,'en'))){const lines=wrapped(block.lines),matches=[];
  // The native serializer sorts by UUID, not panel/line order. Authenticate
  // the physical contiguous text rows instead of relying on file ordering.
  for(const heading of texts.filter(t=>t[1]===lines[0]&&!used.has(t))){const h=one(heading,'at').slice(1,3).map(Number),emitted=[];
   for(let i=0;i<lines.length;i++){const candidates=texts.filter(t=>!used.has(t)&&!emitted.includes(t)&&t[1]===lines[i]&&Math.abs(Number(one(t,'at')[2])-h[1]-i*2.54)<1e-6&&Math.abs(Number(one(t,'at')[1])-h[0])<85);if(candidates.length!==1)break;emitted.push(candidates[0]);}
   if(emitted.length===lines.length)matches.push(emitted);
  }
  assert.equal(matches.length,1,'Missing, ambiguous or spatially split panel');const emitted=matches[0];for(const t of emitted)used.add(t);
  const positions=emitted.map(t=>one(t,'at').slice(1,3).map(Number));for(let i=1;i<positions.length;i++)assert(Math.abs(positions[i][1]-positions[i-1][1]-2.54)<1e-6,'Panel split across distant locations');
  blockEvidence.push({id:block.id,references:block.references,lines:lines.length,positions_mm:positions,intact:true});
 }
 assert.equal(used.size,texts.length,'Unaccounted native note');
 cases.push({name,attachments,eligible_attachments:attachments.filter(a=>a.symbol_origin_on_pin_side!==null).length,side_passes:attachments.filter(a=>a.symbol_origin_on_pin_side===true).length,ambiguous_attachments:attachments.filter(a=>a.attachment==='ambiguous').map(a=>a.reference),blocks:blockEvidence,notes_intact:true});
}
const result={schema:'kicadai.emitted-pin-side-and-panel-audit.v1',root,cases,method:'Exact emitted library pin angle, mirror-then-rotation and native symbol origins; signal/bias outranks supply, ground excluded. Origin-side checks supplement whole-body clearance unit tests and visual inspection, not a complete-readability score. Native text paragraphs independently wrapped and matched in intact contiguous panels.',provider_calls:0};
const file=join(phase,root.endsWith('-final')?'pin-aware-audit.json':root.split('-').at(-1)+'-pin-aware-audit.json');if(existsSync(file))assert.deepEqual(json(file),result);else writeFileSync(file,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify(result,null,2));
