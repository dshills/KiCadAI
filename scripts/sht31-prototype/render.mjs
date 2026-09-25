// Derived read-only inspection views. No CAD or manufacturing bytes are repaired.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {spawnSync} from 'node:child_process';
const [board,out,gerbonara,rsvg,kicad]=process.argv.slice(2);
if(process.argv.length!==7 || fs.existsSync(out)) throw new Error('usage: render.mjs BOARD NEW_OUTPUT GERBONARA RSVG KICAD');
const hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const m=JSON.parse(fs.readFileSync(path.join(board,'bundle-manifest.json')));
function verify(){for(const[f,h]of Object.entries(m.files_sha256))if(hash(path.join(board,f))!==h)throw new Error('board changed: '+f);}
verify();fs.mkdirSync(out,{recursive:true});fs.mkdirSync(path.join(out,'input'));
const env={HOME:process.env.HOME,PATH:process.env.PATH};
function run(exe,args,name){
  const r=spawnSync(exe,args,{env,encoding:'utf8',timeout:60000,maxBuffer:8*1024*1024});
  fs.writeFileSync(path.join(out,name+'.log'),(r.stdout||'')+(r.stderr||''),{flag:'wx'});
  if(r.error||r.status!==0)throw new Error('render failed: '+name);
}
run(gerbonara,['--version'],'gerbonara-version');
const map={};
for(const [name,layer]of Object.entries({'F_Cu':'top copper','B_Cu':'bottom copper','F_Mask':'top mask','B_Mask':'bottom mask',
  'F_Paste':'top paste','B_Paste':'bottom paste','F_Silkscreen':'top silk','B_Silkscreen':'bottom silk','Edge_Cuts':'mechanical outline'})){
  const f=`board-${name}.gbr`;map[f.replace('.','\\.')]=layer;
  fs.copyFileSync(path.join(board,'manufacturing/gerbers',f),path.join(out,'input',f),fs.constants.COPYFILE_EXCL);
}
for(const type of ['PTH','NPTH']){const f=`board-${type}.drl`;map[f.replace('.','\\.')]=type==='PTH'?'drill plated':'drill nonplated';
  fs.copyFileSync(path.join(board,'manufacturing/drill',f),path.join(out,'input',f),fs.constants.COPYFILE_EXCL);}
fs.writeFileSync(path.join(out,'input-map.json'),JSON.stringify(map),{flag:'wx'});
const views=[['top','--top',null,'-1,-81,121,1'],['bottom','--bottom',null,'-1,-81,121,1'],
 ['sensor-copper','--top','top copper','97,-18,106,-12'],['sensor-mask','--top','top mask','97,-18,106,-12'],
 ['sensor-paste','--top','top paste','97,-18,106,-12']];
for(const[name,side,layer,bounds]of views){
  const args=['render','--warnings','default','--no-builtin-name-rules','--input-map',path.join(out,'input-map.json'),side,'--force-bounds',bounds];
  if(layer){const colors=path.join(out,name+'-colors.json');fs.writeFileSync(colors,JSON.stringify({[layer]:'#dd8d43'}),{flag:'wx'});args.push('--no-filters','--colorscheme',colors);}
  else args.push('--pretty');
  args.push(path.join(out,'input'),path.join(out,name+'.svg'));run(gerbonara,args,name);
  run(rsvg,['-w','1800','-b','#111827','-o',path.join(out,name+'.png'),path.join(out,name+'.svg')],name+'-png');
}
run(kicad,['pcb','export','svg','--layers','F.Fab,F.Silkscreen,Edge.Cuts','--black-and-white','--sketch-pads-on-fab-layers','--fit-page-to-board',
 '--exclude-drawing-sheet','--mode-single','--output',path.join(out,'assembly-top.svg'),path.join(board,'board.kicad_pcb')],'assembly-top');
for(const[name,file]of [['schematic',path.join(board,'preview/board.svg')],['pcb',path.join(board,'preview/pcb.svg')],['assembly',path.join(out,'assembly-top.svg')]])
  run(rsvg,['-w',name==='schematic'?'3200':'2000','-b','#ffffff','-o',path.join(out,name+'.png'),file],name+'-png');
verify();fs.writeFileSync(path.join(out,'render-receipt.json'),JSON.stringify({status:'rendered-for-review',
 board_manifest_sha256:hash(path.join(board,'bundle-manifest.json')),source_unchanged:true,
 note:'Derived views only; logs preserve Gerbonara warnings. Human/agent inspection is separate.'},null,2)+'\n',{flag:'wx'});
