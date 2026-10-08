import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { pathToFileURL } from 'node:url';
export function verifyPackage(root) {
  const manifest=JSON.parse(fs.readFileSync(path.join(root,'package-manifest.json'),'utf8'));
  if(manifest.version!=='sht31-prototype-package-1' || manifest.release_authorized!==false ||
    manifest.physical_validation!=='not_performed' || manifest.native_gates!==14) throw new Error('unexpected package state');
  const found={};
  function visit(dir) {
    for(const name of fs.readdirSync(dir)) {
      const file=path.join(dir,name),s=fs.lstatSync(file),rel=path.relative(root,file);
      if(s.isDirectory()) visit(file);
      else if(s.isFile()) {
        if(rel==='package-manifest.json') continue;
        found[rel]=crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
      } else throw new Error('nonregular file: '+rel);
    }
  }
  visit(root);
  if(JSON.stringify(Object.keys(found).sort())!==JSON.stringify(Object.keys(manifest.files_sha256).sort())) throw new Error('file inventory differs');
  for(const [file,h] of Object.entries(found)) if(manifest.files_sha256[file]!==h) throw new Error('hash differs: '+file);
  return {files:Object.keys(found).length,status:'hashes_match',release_authorized:false,physical_validation:'not_performed'};
}
if(process.argv[1] && import.meta.url===pathToFileURL(process.argv[1]).href) {
  try {
    if(process.argv.length!==3) throw new Error('usage: node verify-package.mjs PACKAGE_DIRECTORY');
    console.log(JSON.stringify(verifyPackage(path.resolve(process.argv[2])),null,2));
  } catch(e) { console.error(e.message);process.exitCode=1; }
}
