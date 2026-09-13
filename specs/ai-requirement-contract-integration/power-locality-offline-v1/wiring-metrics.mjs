// Independent emitted-wire-island measurements. Labels do not electrically
// merge distant islands here: the purpose is to measure reliance on labels.
import assert from 'node:assert/strict';
import {readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),root=process.argv[2]||'/tmp/kicadai-power-locality-offline-v1-final',prior='/tmp/kicadai-local-wiring-readability-offline-v1-final';
const sha=b=>createHash('sha256').update(b).digest('hex');
function parse(text){const token=/\s+|\(|\)|"(?:\\.|[^"\\])*"|[^\s()"]+/gy,stack=[],roots=[];let at=0;while(at<text.length){token.lastIndex=at;const m=token.exec(text);assert(m);at=token.lastIndex;const t=m[0];if(/^\s/.test(t))continue;if(t==='('){const n=[];(stack.at(-1)||roots).push(n);stack.push(n);}else if(t===')'){assert(stack.length);stack.pop();}else{assert(stack.length);stack.at(-1).push(t.startsWith('"')?JSON.parse(t):t);}}assert.equal(stack.length,0);assert.equal(roots.length,1);return roots[0];}
const all=(n,k)=>n.filter(x=>Array.isArray(x)&&x[0]===k),one=(n,k)=>{const a=all(n,k);assert.equal(a.length,1);return a[0];};
function measure(dir){
 const files=readdirSync(join(dir,'first')).filter(p=>p.endsWith('.kicad_sch'));assert.equal(files.length,1);
 const path=join(dir,'first',files[0]),bytes=readFileSync(path),native=parse(bytes.toString());
 const point=n=>n.slice(1,3).map(x=>Math.round(Number(x)*1000000)),key=p=>p.join(','),points=new Map();
 const add=p=>{points.set(key(p),p);return p;};
 const wires=all(native,'wire').map(w=>{const p=all(one(w,'pts'),'xy').map(n=>add(point(n)));assert.equal(p.length,2);assert(p[0][0]===p[1][0]||p[0][1]===p[1][1]);return p;});
 const labels=all(native,'label').map(l=>({text:l[1],at:add(point(one(l,'at')))}));
 for(const j of all(native,'junction'))add(point(one(j,'at')));
 const parents=new Map([...points.keys()].map(k=>[k,k]));
 const find=k=>{let p=parents.get(k);while(p!==parents.get(p))p=parents.get(p);return p;};
 const union=(a,b)=>parents.set(find(a),find(b));
 const on=(p,a,b)=>(a[0]===b[0]?p[0]===a[0]:p[1]===a[1])&&p[0]>=Math.min(a[0],b[0])&&p[0]<=Math.max(a[0],b[0])&&p[1]>=Math.min(a[1],b[1])&&p[1]<=Math.max(a[1],b[1]);
 for(const [a,b] of wires)for(const [k,p] of points)if(on(p,a,b))union(key(a),k);
 const wireIslands=new Set(wires.map(w=>find(key(w[0])))),named=new Map();
 for(const l of labels){const island=find(key(l.at));assert(wireIslands.has(island),'Label not on any conductor');if(!named.has(island))named.set(island,[]);named.get(island).push(l.text);}
 for(const names of named.values())assert.equal(new Set(names).size,1,'Different electrical nets share a geometric island');
 const tx=JSON.parse(readFileSync(join(dir,'schematic_transaction.json'))),branches=tx.operations.filter(o=>o.op==='connect');
 const paper=one(native,'paper')[1],sizes={A0:[1189,841],A1:[841,594],A2:[594,420],A3:[420,297],A4:[297,210]};assert(sizes[paper]);
 return {schematic:path,sha256:sha(bytes),paper,paper_area_mm2:sizes[paper][0]*sizes[paper][1],wire_segments:wires.length,wire_length_mm:wires.reduce((sum,[a,b])=>sum+Math.abs(a[0]-b[0])+Math.abs(a[1]-b[1]),0)/1000000,labels:labels.length,geometric_wire_islands:wireIslands.size,labeled_islands:named.size,repeated_labels_on_same_island:[...named.values()].reduce((n,a)=>n+Math.max(0,a.length-1),0),direct_transaction_branches:branches.filter(o=>o.use_labels===false).length,label_transaction_branches:branches.filter(o=>o.use_labels===true).length};
}
const cases=['standalone_regulator','controller_adc_100ma'].map(name=>({name,previous:measure(join(prior,name)),current:measure(join(root,name))}));
const result={schema:'kicadai.local-wiring-measurements.v1',root,prior,cases,method:'Integer-micrometer native wire endpoints, label anchors and explicit junctions; endpoint-on-segment union. Interior wire crossings without a junction/anchor are not joined. Equal label names on disconnected conductors remain separate physical drawing islands. Components never short their pins in this analysis. Counts are descriptive, not readability or electrical certification.',benchmark_passes_added:0,provider_calls:0};
const file=join(phase,root.endsWith('-final')?'wiring-metrics.json':root.split('-').at(-1)+'-wiring-metrics.json');if(existsSync(file))assert.deepEqual(JSON.parse(readFileSync(file)),result);else writeFileSync(file,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify(result));
