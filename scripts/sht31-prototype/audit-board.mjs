// Separate pad/net review, not a replacement for KiCad routing/clearance checks.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
export function auditBoard(pcb, bom) {
  const tokens=pcb.match(/"(?:\\.|[^"\\])*"|[()]|[^\s()]+/g)||[];let i=0;
  function parse(depth=0) {
    assert(depth<100);assert.equal(tokens[i++],'(');const a=[];
    while(tokens[i]!==')') {
      assert(i<tokens.length);
      a.push(tokens[i]==='('?parse(depth+1):tokens[i][0]==='"'?JSON.parse(tokens[i++]):tokens[i++]);
    }
    i++;return a;
  }
  const root=parse();assert.equal(i,tokens.length);assert.equal(root[0],'kicad_pcb');
  const child=(a,key)=>a.find(x=>Array.isArray(x)&&x[0]===key);
  const u1=Object.fromEntries(Array.from({length:39},(_,n)=>[String(n+1),null]));
  Object.assign(u1,{'1':'GND','2':'VCC','3':'controller_enable','15':'GND','25':'controller_boot',
    '28':'GPIO','30':'SCK','31':'MISO','33':'SDA','34':'controller_uart_rx','35':'controller_uart_tx',
    '36':'SCL','37':'MOSI','38':'GND','39':'GND'});
  const supply={'1':'VCC','2':'GND'};
  const expected={U1:u1,U2:{'1':'SDA','2':'GND','3':null,'4':'SCL','5':'VCC','6':null,'7':'GND','8':'GND','9':'GND'},
    J1:supply,J2:{'1':'VCC','2':'GND','3':'controller_uart_rx','4':'controller_uart_tx'},
    J3:{'1':'MOSI','2':'SCL','3':'SDA','4':'MISO','5':'SCK','6':'GPIO'},
    C1:supply,C2:supply,C4:supply,C5:supply,C3:{'1':'controller_enable','2':'GND'},
    R1:{'1':'VCC','2':'controller_enable'},R2:{'1':'VCC','2':'controller_boot'},
    R3:{'1':'VCC','2':'SDA'},R4:{'1':'VCC','2':'SCL'},
    SW1:{'1':'controller_enable','2':'GND'},SW2:{'1':'controller_boot','2':'GND'}};
  const seen=new Set();let pads=0;
  for(const f of root.filter(x=>Array.isArray(x)&&x[0]==='footprint')) {
    const ref=f.find(x=>x[0]==='property'&&x[1]==='Reference')?.[2];
    assert(expected[ref]&&!seen.has(ref),'unexpected/duplicate reference '+ref);seen.add(ref);
    const found=new Set();
    for(const p of f.filter(x=>x[0]==='pad'&&x[1]!=='')) {
      assert(Object.hasOwn(expected[ref],p[1]),'unexpected pad '+ref+'.'+p[1]);
      const net=child(p,'net');assert(!net||net.length===2,'requires KiCad 10 named nets');
      assert.equal(net?.[1]??null,expected[ref][p[1]]===null?null:'/'+expected[ref][p[1]],ref+'.'+p[1]);
      found.add(p[1]);pads++;
    }
    assert.deepEqual([...found].sort(),Object.keys(expected[ref]).sort());
  }
  assert.deepEqual([...seen].sort(),Object.keys(expected).sort());
  const mpns={U1:'ESP32-WROOM-32E-N4',U2:'SHT31-DIS-B2.5kS',R1:'RC0805FR-0710KL',R2:'RC0805FR-0710KL',
    R3:'RC0805FR-074K7L',R4:'RC0805FR-074K7L',C1:'GRM21BR71H104KA01L',C2:'GRM21BR61A106KE19L',
    C3:'GCM21BR71H105KA03L',C4:'GRM21BR71H104KA01L',C5:'GRM21BR71H104KA01L',
    SW1:'B3U-1000P',SW2:'B3U-1000P',J1:'TSW-102-07-L-S',J2:'TSW-104-07-L-S',J3:'TSW-106-07-L-S'};
  assert.equal(bom.length,16);const refs=new Set();
  for(const b of bom){assert(!refs.has(b.reference));refs.add(b.reference);assert.equal(b.mpn,mpns[b.reference]);}
  assert.deepEqual([...refs].sort(),Object.keys(mpns).sort());
  return {status:'pad_net_and_bom_audit_passed',references:seen.size,numbered_pad_instances:pads,
    rise_ns:0.8473*4700*1.0115*70e-3,physical_validation:'not_performed',
    limitation:'Named-net audit only; copper continuity, clearances and export geometry use the separate 14 native gates.'};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href){
  try{if(process.argv.length!==3)throw Error('usage: audit-board.mjs BOARD_DIRECTORY');
    console.log(JSON.stringify(auditBoard(fs.readFileSync(path.join(process.argv[2],'board.kicad_pcb'),'utf8'),
      JSON.parse(fs.readFileSync(path.join(process.argv[2],'bom.json'),'utf8'))),null,2));
  }catch(e){console.error(e.message);process.exitCode=1;}
}
