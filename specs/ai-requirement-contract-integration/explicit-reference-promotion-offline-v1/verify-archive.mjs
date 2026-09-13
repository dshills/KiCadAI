// Authenticate the local archive through a fresh temporary extraction. No API.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {readFileSync,readdirSync,mkdtempSync,rmSync} from 'node:fs';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url));
const repo=resolve(phase,'../../..');
const archive=JSON.parse(readFileSync(join(phase,'archive.json'),'utf8'));
const expected=JSON.parse(readFileSync(join(phase,'verification.json'),'utf8')).inventory;
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
const bytes=readFileSync(join(repo,archive.path));
assert.equal(bytes.length,archive.bytes);
assert.equal(hash(bytes),archive.sha256);
const temporary=mkdtempSync(join(repo,'.cache/reference-archive-verification-'));
try {
  execFileSync('tar',['-xzf',join(repo,archive.path),'-C',temporary]);
  const root=join(temporary,archive.root_directory);
  const walk=(prefix='')=>readdirSync(join(root,prefix),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en')).flatMap(entry=>{
    assert(!entry.isSymbolicLink());
    const path=join(prefix,entry.name);
    return entry.isDirectory()?walk(path):[path];
  });
  const files=walk().map(path=>{const b=readFileSync(join(root,path));return {path,bytes:b.length,sha256:hash(b)};});
  const actual={file_count:files.length,total_bytes:files.reduce((n,f)=>n+f.bytes,0),sha256:hash(JSON.stringify(files))};
  assert.deepEqual(actual,expected);
  console.log(JSON.stringify({verified_utc:new Date().toISOString(),archive_sha256:archive.sha256,archive_bytes:archive.bytes,fresh_extraction_inventory:actual,provider_calls:0},null,2));
} finally {
  // Only the directory freshly created by this invocation is removed.
  rmSync(temporary,{recursive:true,force:true});
}
