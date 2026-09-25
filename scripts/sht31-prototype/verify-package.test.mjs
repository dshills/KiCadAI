import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import {verifyPackage} from './verify-package.mjs';
test('package integrity: exact inventory, changed bytes, unsafe entries, release state',()=>{
  const d=fs.mkdtempSync(path.join(os.tmpdir(),'sht31-package-'));
  try {
    fs.writeFileSync(path.join(d,'data'),'original');
    const manifest={version:'sht31-prototype-package-1',release_authorized:false,physical_validation:'not_performed',native_gates:14,
      files_sha256:{data:crypto.createHash('sha256').update('original').digest('hex')}};
    const save=()=>fs.writeFileSync(path.join(d,'package-manifest.json'),JSON.stringify(manifest));
    save();assert.equal(verifyPackage(d).files,1);
    fs.writeFileSync(path.join(d,'data'),'changed');assert.throws(()=>verifyPackage(d));
    fs.writeFileSync(path.join(d,'data'),'original');
    fs.writeFileSync(path.join(d,'extra'),'extra');assert.throws(()=>verifyPackage(d));fs.unlinkSync(path.join(d,'extra'));
    fs.symlinkSync('data',path.join(d,'link'));assert.throws(()=>verifyPackage(d));fs.unlinkSync(path.join(d,'link'));
    manifest.release_authorized=true;save();assert.throws(()=>verifyPackage(d));
    manifest.release_authorized=false;save();fs.unlinkSync(path.join(d,'data'));assert.throws(()=>verifyPackage(d));
  } finally {fs.rmSync(d,{recursive:true,force:true});}
});
