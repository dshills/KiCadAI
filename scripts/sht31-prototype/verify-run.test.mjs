import test from 'node:test';
import assert from 'node:assert/strict';
import { verifyRun } from './verify-run.mjs';
function records() {
  return [{type:'boot',firmware:'sht31-prototype-1',family:'esp32_sht31_v1',profile:'standard',sda:21,scl:22,hz:100000,address:68},
    {type:'ready',heater:false,radio_initialized:false},
    ...Array.from({length:591},(_,i)=>({type:'sample',seq:i+1,ms:500+i*1025,
      raw_t:26214,raw_rh:32768,celsius:25,rh_percent:50.0008,crc_ok:true,status_ok:true}))];
}
const log = r => r.map(x=>JSON.stringify(x)).join('\n');
test('synthetic valid ten-minute log (not hardware evidence)', () => {
  assert.equal(verifyRun('boot ROM text\n'+log(records())).samples,591);
});
for (const [name, change] of [
  ['fault', r=>r.push({type:'fault',reason:'crc'})],
  ['restart', r=>r.push(r[0])], ['truncated',r=>r.splice(20)],
  ['missing sample',r=>r.splice(10,1)], ['CRC flag',r=>r[10].crc_ok=false],
  ['status flag',r=>r[10].status_ok=false], ['heater',r=>r[1].heater=true],
  ['radio',r=>r[1].radio_initialized=true], ['wrong speed',r=>r[0].hz=400000],
  ['conversion',r=>r[10].celsius=26], ['time reversal',r=>r[10].ms=1],
  ['long gap',r=>r[10].ms+=3000], ['missing boot',r=>r.shift()],
  ['unknown',r=>r.push({type:'bogus'})], ['raw word',r=>r[10].raw_t=65536],
  ['out of envelope',r=>{r[10].raw_t=0;r[10].celsius=-45;}],
]) test(`reject ${name}`,()=>{const r=records();change(r);assert.throws(()=>verifyRun(log(r)));});
test('reject malformed or post-boot console text',()=>{
  assert.throws(()=>verifyRun(log(records())+'\n{broken'));
  assert.throws(()=>verifyRun(log(records())+'\nbrownout detected'));
});
