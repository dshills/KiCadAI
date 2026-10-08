import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';

const [root,output]=process.argv.slice(2);
assert(root&&output&&process.argv.length===4,'usage: node scripts/board-layouts/audit.mjs MATRIX_ROOT NEW_REPORT');
const sha=b=>crypto.createHash('sha256').update(b).digest('hex');
const read=p=>fs.readFileSync(p);
const json=p=>JSON.parse(read(p));
function inventory(root){
  const files={};
  function visit(dir){for(const item of fs.readdirSync(dir,{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name))){
    const p=path.join(dir,item.name);if(item.isDirectory())visit(p);else{assert(item.isFile()&&!item.isSymbolicLink(),'nonregular evidence');files[path.relative(root,p).split(path.sep).join('/')]=sha(read(p));}
  }}visit(root);return files;
}
const gates=['electrical_contract','reference_and_bom_integrity','pcb_writer_and_connectivity','schematic_writer','native_parse_render','kicad_version','kicad_erc','kicad_strict_drc_and_parity','kicad_roundtrip_kicad_sch','kicad_roundtrip_kicad_pcb','schematic_preview','pcb_preview','manufacturing_exports','input_immutability'];
const records=[];
for(const [family,profiles] of [['esp32_bmp280_v1',['standard','fast','low_current']],['esp32_sht31_v1',['standard','fast']]]){
  for(const profile of profiles)for(const size of ['120x70','140x80'])for(let mask=0;mask<8;mask++){
    const id=[family,profile,size,String(mask)].join('/');const dir=path.join(root,id);
    const config=json(path.join(dir,'configuration.json'));assert.equal(config.family,family);assert.equal(config.profile,profile);
    const layout=json(path.join(dir,'layout.json'));assert.deepEqual(layout,{version:'board-layout-1',size,power_connector:mask&1?'west':'inboard',uart_connector:mask&2?'south':'inboard',gpio_connector:mask&4?'east':'inboard'});
    const validation=json(path.join(dir,'validation.json'));assert(validation.passed);assert.equal(validation.kicad_version,'10.0.3');assert.deepEqual(validation.checks.map(x=>x.name),gates);assert(validation.checks.every(x=>x.passed));
    const files=inventory(dir);for(const [name,hash] of Object.entries(validation.native_sha256))assert.equal(files[name],hash);
    const manufacturing=json(path.join(dir,'manufacturing/manifest.json'));
    assert.equal(manufacturing.source_pcb_sha256,files['board.kicad_pcb']);
    for(const [name,hash] of Object.entries(manufacturing.files_sha256))assert.equal(files['manufacturing/'+name],hash);
    assert.equal(Object.keys(files).filter(x=>x.startsWith('manufacturing/')).length,Object.keys(manufacturing.files_sha256).length+1);
    const report=json(path.join(dir,'layout-report.json'));assert.equal(report.manual_repair,false);assert.deepEqual(report.review.specification,layout);
    assert.equal(report.routed_connector_pads,(mask&1?2:0)+(mask&2?4:0)+(mask&4?6:0));
    const drc=json(path.join(dir,'drc.json'));for(const key of ['violations','unconnected_items','schematic_parity'])assert.deepEqual(drc[key],[]);
    records.push({id,passed:true,validation_gates:14,native_sha256:validation.native_sha256,files:Object.keys(files).length,inventory_sha256:sha(JSON.stringify(files)),validation_sha256:files['validation.json'],manufacturing_manifest_sha256:files['manufacturing/manifest.json'],layout_report_sha256:files['layout-report.json'],validation_seconds:validation.seconds});
  }
}
const source={};for(const file of ['internal/boardfamily/layout.go','internal/boardfamily/native.go','internal/boardfamily/validate.go','internal/boardfamily/manufacturing.go','internal/boardfamily/specification.go','cmd/kicadai-board-family/main.go','cmd/kicadai-board-family/specification.go'])source[file]=sha(read(file));
const report={version:'board-layout-qualification-1',status:'80_configurations_passed',kicad_version:'10.0.3',manual_artifact_repair:false,physical_validation:'not_performed',source_sha256:source,cases:records};
fs.mkdirSync(path.dirname(output),{recursive:true});fs.writeFileSync(output,JSON.stringify(report,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({cases:records.length,gates_per_case:14,report:output,sha256:sha(read(output))}));
