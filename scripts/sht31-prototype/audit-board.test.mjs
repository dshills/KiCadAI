import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {auditBoard} from './audit-board.mjs';
const pcb=fs.readFileSync(new URL('../../examples/board-family-v2/sht31-standard/board.kicad_pcb',import.meta.url),'utf8');
const bom=JSON.parse(fs.readFileSync(new URL('../../examples/board-family-v2/sht31-standard/bom.json',import.meta.url),'utf8'));
test('separate reviewed pin and BOM map',()=>assert.equal(auditBoard(pcb,bom).references,16));
test('reject wrong SDA, power, UART and reset nets',()=>{
  for(const name of ['SDA','VCC','controller_uart_rx','controller_enable'])
    assert.throws(()=>auditBoard(pcb.replace(`(net "/${name}")`,'(net "/wrong")'),bom));
});
test('reject substituted pull-ups and missing BOM parts',()=>{
  assert.throws(()=>auditBoard(pcb,bom.map(b=>b.reference==='R3'?{...b,mpn:'RC0805FR-0710KL'}:b)));
  assert.throws(()=>auditBoard(pcb,bom.slice(1)));
});
test('reject truncated or appended native syntax',()=>{
  assert.throws(()=>auditBoard(pcb.slice(0,-10),bom));assert.throws(()=>auditBoard(pcb+' ()',bom));
});
