import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

export const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
export const policy = Object.freeze({sample_ms:1000,case_wall_seconds:1200,campaign_wall_seconds:7200,process_tree_rss_bytes:16*1024**3,evidence_bytes:10*1024**3});

export function treeRSSFromPS(text, pid) {
  if (!Number.isSafeInteger(pid) || pid <= 0) throw new Error('invalid worker pid');
  const rows = text.trim() ? text.trim().split('\n').map(line=>line.trim().split(/\s+/).map(Number)) : [];
  const seen = new Set();
  for (const row of rows) {
    if(row.length!==3 || row.some(n=>!Number.isSafeInteger(n)||n<0) || row[0]===0 || seen.has(row[0])) throw new Error('invalid process sample');
    seen.add(row[0]);
  }
  const selected=new Set([pid]);
  for(let changed=true;changed;) { changed=false; for(const [p,parent] of rows) if(selected.has(parent)&&!selected.has(p)){selected.add(p);changed=true;} }
  return rows.reduce((sum,[p,,rss])=>sum+(selected.has(p)?rss*1024:0),0);
}

export function evidenceBytes(dir) {
  let bytes=0;
  for(const name of fs.readdirSync(dir)) {
    const p=path.join(dir,name);
    try {
      const stat=fs.lstatSync(p);
      if(stat.isSymbolicLink() || (!stat.isDirectory() && !stat.isFile())) throw new Error('nonregular evidence');
      bytes+=stat.isDirectory()?evidenceBytes(p):stat.size;
    } catch(err) {
      // Native tools remove their own temporary paths while sampling. Only
      // disappearance is benign; permissions and all other errors fail closed.
      if(err.code!=='ENOENT') throw err;
    }
  }
  return bytes;
}

export function checkEnvironment(expected, actual) {
  for(const key of ['node','node_binary_sha256','platform','arch','go_version','kicad_cli','kicad_version','kicad_binary_sha256','symbols_root','footprints_root']) {
    if(typeof expected[key]!=='string' || !expected[key] || actual[key]!==expected[key]) throw new Error('frozen environment mismatch: '+key);
  }
  if(JSON.stringify(expected.resource_policy)!==JSON.stringify(policy)) throw new Error('resource policy mismatch');
}

export function checkBuild(metadata, sourceCommit, goVersion) {
  if(metadata.split('\n')[0].split(/\s+/).at(-1)!==goVersion || !metadata.includes('\tvcs.revision='+sourceCommit+'\n') || !metadata.includes('\tvcs.modified=false\n')) throw new Error('worker is not a clean build of the current commit/toolchain');
}

export function providerStopReason(dir) {
  for(const name of fs.readdirSync(dir)) {
    const p=path.join(dir,name), stat=fs.lstatSync(p);
    if(stat.isSymbolicLink()) throw new Error('symlink in provider evidence');
    if(stat.isDirectory() && name==='follow-up') {
      const nested=providerStopReason(p); if(nested) return nested;
    } else if(/^attempt-\d+\.error\.json$/.test(name)) {
      const failure=JSON.parse(fs.readFileSync(p,'utf8'));
      if(['ai_provider_authentication','ai_provider_rate_limit','ai_provider_transport','ai_provider_timeout','ai_provider_configuration'].includes(failure.code)) return failure.code;
    }
  }
  return null;
}
