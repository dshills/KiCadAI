// Compact offline revalidation after CI error-handling corrections. No API use.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const base='specs/board-family-v1',src='.cache/board-family-v1/acceptance-configurations-05',dest=base+'/evidence/integration';
const read=p=>JSON.parse(fs.readFileSync(p)),hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
assert(!fs.existsSync(dest));
const s=read(src+'/summary.json'),old=read(base+'/evidence/offline/summary.json'),reg='.cache/board-family-v1/regression-11';
assert.equal(s.passed,true);assert.equal(s.api_requests,0);assert.equal(s.cases.length,10);assert.equal(s.replays.length,3);
assert.equal(s.spec_sha256,old.spec_sha256);assert.equal(s.runner_sha256,old.runner_sha256);
assert.equal(read(reg+'/execution.json').exit_code,0);assert.equal(read(reg+'/execution.json').provider_keys_removed,true);
const files={},copies=[[src+'/summary.json',dest+'/summary.json'],[reg+'/execution.json',dest+'/bounded-regression.json'],[reg+'/tests.log',dest+'/bounded-regression.log']];
const unchanged=[];
for(const c of [...s.cases,...s.replays]){
 const prior=[...old.cases,...old.replays].find(x=>x.id===c.id);assert(prior);
 assert.equal(c.exit_code,0);assert.equal(c.result.passed,true);assert.deepEqual(c.artifacts,prior.artifacts);
 if(c.matches_original!==undefined)assert.equal(c.matches_original,true);
 const p=src+'/'+c.id,v=read(p+'/validation.json'),erc=read(p+'/erc.json'),drc=read(p+'/drc.json');
 assert.equal(v.passed,true);assert.equal(v.kicad_version,'10.0.3');assert.equal(v.checks.length,13);assert(v.checks.every(x=>x.passed));
 assert(erc.sheets.length>0&&erc.sheets.every(x=>x.violations.length===0));
 for(const k of ['violations','unconnected_items','schematic_parity'])assert.deepEqual(drc[k],[]);
 for(const [f,sha] of Object.entries(c.artifacts))assert.equal(hash(p+'/'+f),sha);
 unchanged.push(c.id);for(const f of ['validation.json','erc.json','drc.json'])copies.push([p+'/'+f,dest+'/'+c.id+'/'+f]);
}
const changedFiles=['cmd/kicadai-board-family/main.go','internal/boardfamily/ledger.go','internal/boardfamily/ledger_test.go','internal/boardfamily/validate.go'];
for(const f of changedFiles)assert.equal(hash(f),s.source_sha256[f]);
assert.equal(hash('internal/boardfamily/interpret.go'),read(base+'/evidence/acceptance/assessment.json').corrected_source_sha256['internal/boardfamily/interpret.go']);
const assessment={source_commit:s.base_commit,binary_sha256:s.binary_sha256,api_requests:0,passed:true,configuration_passes:10,deterministic_replays:3,unchanged_artifact_cases:unchanged,source_sha256:Object.fromEntries(changedFiles.map(f=>[f,hash(f)])),prior_validate_sha256:old.source_sha256['internal/boardfamily/validate.go'],median_seconds:s.metrics.median_seconds,max_seconds:s.metrics.max_seconds,bounded_regression_seconds:read(reg+'/execution.json').seconds,change_scope:'Propagate command encoding/read-close and validator temporary-cleanup errors; preserve ledger errors/state on cleanup failure. No configuration, decision, reference, generation, native-check selection or acceptance-policy change.',local_lint:{version:'2.13.1',new_scope_issues:0,repository_issues:2,baseline_verified:true,baseline_ci:'https://github.com/dshills/KiCadAI/actions/runs/34755942929',baseline_files:['specs/ai-requirement-contract-integration/publication-live-v1/replay-audit/main.go:258','specs/practical-board-completion-v1/engine/main.go:61']},notice:'This offline revalidation does not change any live outcome or consume another API request. Historical evidence remains unchanged.'};
for(const [from,to] of copies){fs.mkdirSync(path.dirname(to),{recursive:true});fs.copyFileSync(from,to,fs.constants.COPYFILE_EXCL);assert.equal(hash(from),hash(to));files[to]=hash(to)}
fs.writeFileSync(dest+'/assessment.json',JSON.stringify(assessment,null,2)+'\n',{flag:'wx'});files[dest+'/assessment.json']=hash(dest+'/assessment.json');
fs.writeFileSync(dest+'/manifest.json',JSON.stringify({files},null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({copied_hashes:Object.keys(files).length,configuration_passes:10,replays:3,api_requests:0,median_seconds:s.metrics.median_seconds,max_seconds:s.metrics.max_seconds}));
