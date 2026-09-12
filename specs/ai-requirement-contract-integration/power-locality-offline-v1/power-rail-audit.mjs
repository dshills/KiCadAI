// Independently measure visible regulator-to-capacitor paths in emitted KiCad
// geometry. Equal labels on disconnected islands are deliberately not joined.
import assert from 'node:assert/strict';
import {readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),root=process.argv[2]||'/tmp/kicadai-power-locality-offline-v1-final',prior='/tmp/kicadai-local-wiring-readability-offline-v1-final';
const json=p=>JSON.parse(readFileSync(p)),sha=b=>createHash('sha256').update(b).digest('hex');
function parse(text){const tokens=/\s+|\(|\)|"(?:\\.|[^"\\])*"|[^\s()"]+/gy,stack=[],roots=[];let at=0;while(at<text.length){tokens.lastIndex=at;const m=tokens.exec(text);assert(m);at=tokens.lastIndex;const t=m[0];if(/^\s/.test(t))continue;if(t==='('){const n=[];(stack.at(-1)||roots).push(n);stack.push(n);}else if(t===')'){assert(stack.length);stack.pop();}else{assert(stack.length);stack.at(-1).push(t.startsWith('"')?JSON.parse(t):t);}}assert.equal(stack.length,0);assert.equal(roots.length,1);return roots[0];}
const all=(n,k)=>n.filter(x=>Array.isArray(x)&&x[0]===k),one=(n,k)=>{const a=all(n,k);assert.equal(a.length,1,k);return a[0];};
function descendants(n,k){return n.flatMap(x=>Array.isArray(x)?[...(x[0]===k?[x]:[]),...descendants(x,k)]:[]);}
function measure(dir){
 const files=readdirSync(join(dir,'first')).filter(p=>p.endsWith('.kicad_sch'));assert.equal(files.length,1);
 const path=join(dir,'first',files[0]),bytes=readFileSync(path),native=parse(bytes.toString()),sch=json(join(dir,'workflow_request.json')).explicit_circuit.schematic;
 const libraries=new Map(all(one(native,'lib_symbols'),'symbol').map(s=>[s[1],s])),symbols=new Map(all(native,'symbol').map(s=>[all(s,'property').find(p=>p[1]==='Reference')[2],s]));
 const key=p=>p.join(','),point=n=>n.slice(1,3).map(v=>Math.round(Number(v)*1e6)),points=new Map(),add=p=>{points.set(key(p),p);return p;};
 const wires=all(native,'wire').map(w=>all(one(w,'pts'),'xy').map(n=>add(point(n))));
 for(const w of wires)assert(w.length===2&&(w[0][0]===w[1][0]||w[0][1]===w[1][1]));
 for(const j of all(native,'junction'))add(point(one(j,'at')));
 const pin=(ref,number)=>{const s=symbols.get(ref);assert(s);const lib=libraries.get(one(s,'lib_id')[1]);assert(lib);const ps=descendants(lib,'pin').filter(p=>one(p,'number')[1]===number);assert.equal(ps.length,1,ref+'.'+number);
  const local=one(ps[0],'at'),at=one(s,'at');let x=Number(local[1]),y=-Number(local[2]);const mirror=all(s,'mirror')[0]?.[1];if(mirror==='x')y=-y;else if(mirror==='y')x=-x;const a=Number(at[3]||0)*Math.PI/180;
  return add([Math.round((Number(at[1])+x*Math.cos(a)+y*Math.sin(a))*1e6),Math.round((Number(at[2])-x*Math.sin(a)+y*Math.cos(a))*1e6)]);};
 const components=new Map(sch.circuit.components.map(c=>[c.id,c])),rails=[];
 for(const owner of components.values()){
  if(owner.role!=='regulator')continue;
  const group=sch.layout.groups.find(g=>g.members.includes(owner.id));assert(group);
  for(const net of sch.circuit.nets){if(!['power','power_pos','power_neg'].includes(net.role))continue;
   const ends=net.connect.map(e=>{const i=e.lastIndexOf('.');return {id:e.slice(0,i),number:e.slice(i+1)};});
   for(const endpoint of ends.filter(e=>e.id===owner.id)){
    const caps=ends.filter(e=>group.members.includes(e.id)&&['decoupling_capacitor','bulk_capacitor','decoupling'].includes(components.get(e.id)?.role));if(!caps.length)continue;
    rails.push({net:net.name,owner:owner.ref,pin:endpoint.number,at:pin(owner.ref,endpoint.number),capacitors:caps.map(e=>({reference:components.get(e.id).ref,pin:e.number,at:pin(components.get(e.id).ref,e.number)}))});
   }
  }
 }
 const parents=new Map([...points.keys()].map(k=>[k,k])),find=k=>{let p=parents.get(k);assert(p!==undefined);while(p!==parents.get(p))p=parents.get(p);return p;},union=(a,b)=>parents.set(find(a),find(b));
 const on=(p,a,b)=>(a[0]===b[0]?p[0]===a[0]:p[1]===a[1])&&p[0]>=Math.min(a[0],b[0])&&p[0]<=Math.max(a[0],b[0])&&p[1]>=Math.min(a[1],b[1])&&p[1]<=Math.max(a[1],b[1]);
 for(const [a,b]of wires)for(const[k,p]of points)if(on(p,a,b))union(key(a),k);
 const islands=new Set(wires.map(w=>find(key(w[0]))));
 for(const rail of rails){const origin=find(key(rail.at));rail.capacitors=rail.capacitors.map(c=>({...c,visible_conductor_path:islands.has(origin)&&origin===find(key(c.at))}));rail.all_capacitors_visibly_connected=rail.capacitors.every(c=>c.visible_conductor_path);}
 return {schematic:path,sha256:sha(bytes),rails,eligible_supply_pins:rails.length,visibly_connected_supply_pins:rails.filter(r=>r.all_capacitors_visibly_connected).length,eligible_capacitor_paths:rails.reduce((n,r)=>n+r.capacitors.length,0),visible_capacitor_paths:rails.flatMap(r=>r.capacitors).filter(c=>c.visible_conductor_path).length};
}
const cases=['standalone_regulator','controller_adc_100ma'].map(name=>({name,previous:measure(join(prior,name)),current:measure(join(root,name))}));
const result={schema:'kicadai.emitted-power-rail-path-audit.v1',root,prior,cases,method:'Emitted library pin coordinates, mirror and rotation; integer-micrometer wire endpoint/junction/pin graph. Capacitor pins are not shorted through their component, and equal net labels do not join islands. Regulator role and capacitor membership come from unchanged functional ownership. This is visible-path evidence, not a complete-readability or fabrication score.',provider_calls:0,benchmark_passes_added:0};
const file=join(phase,root.endsWith('-final')?'power-rail-audit.json':root.split('-').at(-1)+'-power-rail-audit.json');if(existsSync(file))assert.deepEqual(json(file),result);else writeFileSync(file,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({cases:cases.map(c=>({name:c.name,previous:c.previous.visible_capacitor_paths,current:c.current.visible_capacitor_paths,total:c.current.eligible_capacitor_paths}))}));
