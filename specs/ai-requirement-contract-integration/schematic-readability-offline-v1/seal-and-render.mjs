// Offline generation evidence: seal originals before any visual export.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,readdirSync,existsSync,mkdirSync,copyFileSync} from 'node:fs';
import {join,dirname,resolve,basename} from 'node:path';
const root=resolve(process.argv[2]);
const cli='/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';
const hash=b=>createHash('sha256').update(b).digest('hex');
const walk=(dir,prefix='')=>readdirSync(join(dir,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(e=>{
  assert(!e.isSymbolicLink(),'No evidence symlinks');
  if(!prefix && e.name==='.kicadai') return [];
  const p=join(prefix,e.name);
  return e.isDirectory()?walk(dir,p):[p];
});
const inventory=dir=>walk(dir).map(path=>{const b=readFileSync(join(dir,path));return {path,bytes:b.length,sha256:hash(b)};});
const write=(path,obj)=>writeFileSync(path,JSON.stringify(obj,null,2)+'\n',{flag:'wx'});
const cases=['standalone_regulator','controller_adc_100ma'];
const projects=cases.flatMap(name=>['first','second'].filter(run=>existsSync(join(root,name,run))).map(run=>({name,run,files:inventory(join(root,name,run))})));
assert(projects.length>0,'No generated projects to seal');
const seal={schema:'kicadai.sealed-before-render.v1',sealed_utc:new Date().toISOString(),root,excluded_subtrees:['.kicadai (diagnostic copies, retained in raw evidence)'],projects};
write(join(root,'sealed-projects.json'),seal);
const commands=[];
const run=(command,args)=>{
  const env={...process.env};
  for(const name of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY']) delete env[name];
  const r=spawnSync(command,args,{env,encoding:'utf8',timeout:60000,maxBuffer:16*1024*1024});
  commands.push({command,args,status:r.status,stdout:r.stdout,stderr:r.stderr,error:r.error?.message});
  assert.equal(r.status,0,'Native rendering failed');
};
try {
  for(const name of cases) {
    const project=projects.find(p=>p.name===name&&p.run==='first');
    if(!project) continue;
    const copy=join(root,name,'render-copy'),render=join(root,name,'render');
    mkdirSync(copy); mkdirSync(render);
    for(const {path} of project.files) {
      mkdirSync(dirname(join(copy,path)),{recursive:true});
      copyFileSync(join(root,name,'first',path),join(copy,path));
    }
    assert.deepEqual(inventory(copy),project.files,'Copy differs before export');
    // The root schematic export includes every hierarchical page.
    const main=project.files.filter(f=>!f.path.includes('/')&&f.path.endsWith('.kicad_sch'));
    assert.equal(main.length,1);
    run(cli,['sch','export','netlist','--format','kicadxml','-o',join(render,'native-netlist.xml'),join(copy,main[0].path)]);
    run(cli,['sch','export','svg','--exclude-drawing-sheet','-o',render,join(copy,main[0].path)]);
    const svgs=walk(render).filter(p=>p.endsWith('.svg'));
    assert(svgs.length>=project.files.filter(f=>f.path.endsWith('.kicad_sch')).length,'Missing sheet exports');
    for(const path of svgs) run('/opt/homebrew/bin/rsvg-convert',['--width','2200','--background-color','white','--output',join(render,basename(path,'.svg')+'.png'),join(render,path)]);
    const pcb=project.files.filter(f=>!f.path.includes('/')&&f.path.endsWith('.kicad_pcb'));
    assert.equal(pcb.length,1);
    run(cli,['pcb','export','svg','--layers','F.Cu,B.Cu,F.Silkscreen,Edge.Cuts','--mode-single','--fit-page-to-board','--exclude-drawing-sheet','-o',join(render,'board.svg'),join(copy,pcb[0].path)]);
    run('/opt/homebrew/bin/rsvg-convert',['--width','1600','--background-color','white','--output',join(render,'board.png'),join(render,'board.svg')]);
  }
} finally {
  write(join(root,'render-commands.json'),commands);
  for(const project of projects) assert.deepEqual(inventory(join(root,project.name,project.run)),project.files,'Original changed during rendering');
  write(join(root,'seal-after-render.json'),{verified_utc:new Date().toISOString(),seal_sha256:hash(readFileSync(join(root,'sealed-projects.json'))),originals_unchanged:true,project_count:projects.length});
}
console.log(JSON.stringify({root,projects:projects.length,originals_unchanged:true,commands:commands.length}));
