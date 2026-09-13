// Preserve exact current source for each native development execution.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {join,resolve} from 'node:path';
const base='ec2ececb4e52643f0767d0fcbb70213f1ec7209c',label=process.argv[2];assert(/^[a-z0-9-]+$/.test(label));
const git=(...args)=>execFileSync('git',args,{encoding:'utf8'}).trim().split('\n').filter(Boolean);
const paths=[...new Set([...git('diff',base,'--name-only','--','internal'),...git('ls-files','--others','--exclude-standard','--','internal')])].filter(p=>p.endsWith('.go')).sort();
const files=paths.map(path=>{const b=readFileSync(path);return {path,bytes:b.length,sha256:createHash('sha256').update(b).digest('hex'),source:b.toString('utf8')};});
const root=resolve('.cache/native-readability-replay-v1-sources');mkdirSync(root,{recursive:true});
const path=join(root,label+'.json'),body=JSON.stringify({base,label,recorded_utc:new Date().toISOString(),files},null,2)+'\n';writeFileSync(path,body,{flag:'wx'});
console.log(JSON.stringify({path,source_files:files.length,sha256:createHash('sha256').update(body).digest('hex')}));
