// Render review viewports from native SVG exports, never edit generated designs.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,readdirSync,existsSync} from 'node:fs';
import {join,resolve} from 'node:path';
const root=resolve(process.argv[2]),commands=[];
const env={...process.env};for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY'])delete env[k];
const run=(command,args)=>{const r=spawnSync(command,args,{env,encoding:'utf8',timeout:60000});commands.push({command,args,status:r.status,stdout:r.stdout,stderr:r.stderr});assert.equal(r.status,0,r.stderr);};
const crops={standalone_regulator:[['upper',60,25,105,115],['lower',10,130,180,105]],controller_adc_100ma:[['capacitors',250,45,130,105],['analog',155,135,200,145],['controller',345,135,140,170]]};
for(const [name,views] of Object.entries(crops)){
  if(!existsSync(join(root,name,'render')))continue; // Explicitly unavailable final controller project is recorded in verification.json.
  const dir=join(root,name,'render'),svg=readdirSync(dir).filter(p=>p.endsWith('.svg')&&p!=='board.svg');assert.equal(svg.length,1);
  for(const [label,x,y,width,height] of views){const out=join(dir,'review-'+label+'.png');assert(!existsSync(out));run('/opt/homebrew/bin/rsvg-convert',['--dpi-x','254','--dpi-y','254',`--left=-${x}mm`,`--top=-${y}mm`,'--page-width',`${width}mm`,'--page-height',`${height}mm`,'--background-color','white','--output',out,join(dir,svg[0])]);}
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
console.log(JSON.stringify({root,commands:commands.length,dpi:254}));

