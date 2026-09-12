// Derive review viewports from the recorded functional groups and transaction.
// Native exports use disposable project copies; no generated design is edited.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,readdirSync,existsSync} from 'node:fs';
import {join,resolve} from 'node:path';
const root=resolve(process.argv[2]),commands=[],crops=[];
const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const run=(command,args)=>{const r=spawnSync(command,args,{env,encoding:'utf8',timeout:60000});commands.push({command,args,status:r.status,stdout:r.stdout,stderr:r.stderr});assert.equal(r.status,0,r.stderr);};
for(const name of ['standalone_regulator','controller_adc_100ma']){
 const dir=join(root,name,'render');if(!existsSync(dir))continue;
 const req=JSON.parse(readFileSync(join(root,name,'workflow_request.json'))),tx=JSON.parse(readFileSync(join(root,name,'schematic_transaction.json')));
 const svg=readdirSync(dir).filter(p=>p.endsWith('.svg')&&p!=='board.svg');assert.equal(svg.length,1);
 const schematic=req.explicit_circuit.schematic,refs=new Map(schematic.circuit.components.map(c=>[c.id,c.ref]));
 for(const group of schematic.layout.groups){
  const symbols=tx.operations.filter(o=>o.op==='add_symbol'&&group.members.some(id=>refs.get(id)===o.ref));assert.equal(symbols.length,group.members.length);
  const x=Math.max(0,Math.floor(Math.min(...symbols.map(s=>s.at.x_mm))-55)),y=Math.max(0,Math.floor(Math.min(...symbols.map(s=>s.at.y_mm))-55));
  const width=Math.ceil(Math.max(...symbols.map(s=>s.at.x_mm))+55-x),height=Math.ceil(Math.max(...symbols.map(s=>s.at.y_mm))+55-y);
  const out=join(dir,'review-'+group.id+'.png');assert(!existsSync(out));
  crops.push({name,group:group.id,label:group.label,x,y,width,height,margin_mm:55});
  run('/opt/homebrew/bin/rsvg-convert',['--dpi-x','254','--dpi-y','254',`--left=-${x}mm`,`--top=-${y}mm`,'--page-width',`${width}mm`,'--page-height',`${height}mm`,'--background-color','white','--output',out,join(dir,svg[0])]);
 }
 const copy=join(root,name,'render-copy'),pcb=readdirSync(copy).filter(p=>p.endsWith('.kicad_pcb'));assert.equal(pcb.length,1);
 const text=readFileSync(join(copy,pcb[0]),'utf8');
 for(const layer of ['F.Cu','In1.Cu','In2.Cu','B.Cu']){
  if(!text.includes(`"${layer}"`))continue;
  const out=join(dir,'review-'+layer+'.svg');assert(!existsSync(out));
  run('/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli',['pcb','export','svg','--layers',layer+',Edge.Cuts','--mode-single','--fit-page-to-board','--exclude-drawing-sheet','-o',out,join(copy,pcb[0])]);
  run('/opt/homebrew/bin/rsvg-convert',['--width','1200','--background-color','white','--output',out.replace(/\.svg$/,'.png'),out]);
 }
}
writeFileSync(join(root,'visual-review-commands.json'),JSON.stringify({dpi:254,pixels_per_mm:10,crops_mm:crops,commands},null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({root,commands:commands.length,crops:crops.length,dpi:254}));
