import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';

const source=process.env.KICADAI_PARTITIONED_REVIEW_ROOT;
const script='specs/board-family-v2/partitioned-intent-08/review-offline.mjs';
for(const mode of ['unchanged','missing-fixture','invented-quantity-role','modified-native','coherent-manifest-rehash','extra-deliverable','extra-evidence','symlink']) {
  test('offline reviewer '+mode,{skip:!source},()=>{
    const root=fs.mkdtempSync(path.join(os.tmpdir(),'partitioned-review-tamper-'));
    try {
      fs.cpSync(source,root,{recursive:true});
      const edit=(relative,fn)=>{
        const p=path.join(root,relative), v=JSON.parse(fs.readFileSync(p,'utf8'));
        fn(v);fs.writeFileSync(p,JSON.stringify(v,null,2)+'\n');
      };
      if(mode==='missing-fixture')fs.unlinkSync(path.join(root,'fixtures/useful-01.json'));
      if(mode==='invented-quantity-role')edit('fixtures/useful-02.json',v=>{
        const raw=JSON.parse(v.raw_text);raw.quantities.q0=[{kind:'feature',value:'skip_crc',state:'requested',context:[]}];v.raw_text=JSON.stringify(raw);
      });
      const native='native/useful-01/output/board.kicad_pcb';
      if(mode==='modified-native'||mode==='coherent-manifest-rehash')fs.appendFileSync(path.join(root,native),'\n');
      if(mode==='coherent-manifest-rehash') {
        // Rewriting a local envelope cannot erase disagreement with the
        // unchanged, independently retained historical example identity.
        const replacement=crypto.createHash('sha256').update(fs.readFileSync(path.join(root,native))).digest('hex');
        edit('native/useful-01/output/validation.json',v=>{v.native_sha256['board.kicad_pcb']=replacement;});
        edit('native/useful-01/output/manufacturing/manifest.json',v=>{v.source_pcb_sha256=replacement;});
      }
      if(mode==='extra-deliverable')fs.writeFileSync(path.join(root,'native/useful-01/output/manufacturing/extra.gbr'),'unexpected');
      if(mode==='extra-evidence')fs.writeFileSync(path.join(root,'unexpected.json'),'{}');
      if(mode==='symlink') {
        fs.unlinkSync(path.join(root,'fixtures/useful-01.json'));
        fs.symlinkSync(path.join(source,'fixtures/useful-01.json'),path.join(root,'fixtures/useful-01.json'));
      }
      const env=Object.fromEntries(Object.entries(process.env).filter(([k])=>!/KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL/i.test(k)));
      const result=spawnSync(process.execPath,[script,root,'--check'],{env,encoding:'utf8'});
      assert.equal(result.status===0,mode==='unchanged',result.stderr);
    } finally {fs.rmSync(root,{recursive:true,force:true});}
  });
}
