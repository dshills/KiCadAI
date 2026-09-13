// Small offline development helper; retains real diagnostics, never repairs output.
import fs from 'node:fs';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
const dir=path.resolve(process.argv[2]);
const env={...process.env};
for(const k of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS'])delete env[k];
const cli='/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';
const steps={
 erc:['sch','erc','--format','json','--severity-all','--exit-code-violations','--output',path.join(dir,'erc.json'),path.join(dir,'board.kicad_sch')],
 drc:['pcb','drc','--format','json','--severity-all','--all-track-errors','--schematic-parity','--exit-code-violations','--output',path.join(dir,'drc.json'),path.join(dir,'board.kicad_pcb')],
 schematic:['sch','export','svg','--output',path.join(dir,'preview')+'/',path.join(dir,'board.kicad_sch')],
 board:['pcb','export','svg','--layers','F.Cu,B.Cu,F.SilkS,Edge.Cuts','--mode-single','--fit-page-to-board','--exclude-drawing-sheet','--output',path.join(dir,'board.svg'),path.join(dir,'board.kicad_pcb')],
};
for(const [name,args] of Object.entries(steps)){
 const start=performance.now();const r=spawnSync(cli,args,{env,encoding:'utf8',timeout:120000,maxBuffer:4000000});
 fs.writeFileSync(path.join(dir,name+'.log'),(r.stdout||'')+'\n'+(r.stderr||''));
 console.log(JSON.stringify({step:name,exit:r.status,signal:r.signal,error:r.error?.message,seconds:(performance.now()-start)/1000}));
}
