import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';

const dir=path.dirname(fileURLToPath(import.meta.url));
const root=path.resolve(dir,'../..');
const read=name=>fs.readFileSync(path.join(dir,name),'utf8');
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const freeze=JSON.parse(read('freeze.json'));

test('v2 preserves every original frozen input except the approved evaluator timeout',()=>{
  assert.equal(sha(read('freeze.json')),'d5da087982ff7cb4281a51c9ca86b6b48a1c2859806926e80d2b222947a8a94e');
  const helper=`// Protocol v2 changes only the experimental provider request deadline.
// Production defaults and all acceptance, attempt and resource caps are unchanged.
func providerHTTPClient(transport http.RoundTripper) *http.Client {
\treturn &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

`;
  for(const file of freeze.files){
    let bytes=fs.readFileSync(path.join(root,file.path));
    if(file.path==='internal/practicalboardeval/engine.go'){
      let text=bytes.toString('utf8');
      assert.equal(text.split(helper).length,2,'exact approved helper occurs once');
      text=text.replace(helper,'');
      assert.equal(text.split('HTTPClient: providerHTTPClient(recorder)').length,2);
      text=text.replace('HTTPClient: providerHTTPClient(recorder)','HTTPClient: &http.Client{Transport: recorder, Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}');
      bytes=Buffer.from(text);
    }
    assert.equal(bytes.length,file.bytes,file.path);
    assert.equal(sha(bytes),file.sha256,file.path);
  }
});

test('v2 supervisor changes only seal routing, required coverage and protocol metadata',()=>{
  let expected=read('run-campaign.mjs');
  for(const [from,to] of [
    ['node run-campaign.mjs','node run-campaign-v2.mjs'],
    ["path.join(spec,'freeze.json')","path.join(spec,'freeze-v2.json')"],
    ["'evidence-utils.test.mjs'].map","'evidence-utils.test.mjs','run-campaign-v2.mjs','protocol-v2.test.mjs','PROTOCOL-V2.md','protocol-v2-authorization.json','freeze.json'].map"],
    ['  phase,started_utc:',"  protocol:'v2',provider_request_timeout_seconds:300,\n  phase,started_utc:"],
    ["'--mode','case','--root',root","'--mode','case','--freeze',path.join(spec,'freeze-v2.json'),'--root',root"],
  ]){
    assert(expected.includes(from),from);
    expected=expected.replaceAll(from,to);
  }
  assert.equal(read('run-campaign-v2.mjs'),expected);
});
