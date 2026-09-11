import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {mkdtempSync,readFileSync,rmSync,writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
import test from 'node:test';

const corpus=JSON.parse(readFileSync(new URL('../corpus.json',import.meta.url),'utf8'));
const names=[...corpus.cases,...corpus.paraphrases].map(x=>`${x.id}.audit.json`);
for(const [label,files] of [['empty corpus',[]],['one missing audit',names.slice(1)],['extra audit',[...names,'EXTRA.audit.json']]]) {
  test(`publication rejects ${label} before reading raw evidence`,()=>{
    const directory=mkdtempSync(join(tmpdir(),'kicadai-audit-test-'));
    try {
      // Invalid dummy bodies prove completeness is checked before parsing any audit.
      for(const name of files)writeFileSync(join(directory,name),'not an audit');
      const env={...process.env};delete env.OPENAI_API_KEY;
      const run=spawnSync(process.execPath,[fileURLToPath(new URL('./verify-audits.mjs',import.meta.url)),directory],{env,encoding:'utf8'});
      assert.equal(run.status,1);
      assert.match(run.stderr,/Expected exactly one audit for every frozen case, with no extras/);
    } finally {rmSync(directory,{recursive:true,force:true});}
  });
}
