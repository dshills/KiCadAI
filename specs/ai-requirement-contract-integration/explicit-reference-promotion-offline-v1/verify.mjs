// Read-only offline evidence verification. No provider client or network call.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {readFileSync, readdirSync, existsSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const phase = dirname(fileURLToPath(import.meta.url));
const repo = resolve(phase, '../../..');
const root = resolve(process.argv[2] || '/tmp/kicadai-explicit-reference-promotion-offline-v1-final');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const json = path => JSON.parse(readFileSync(path, 'utf8'));
const walk = (dir, prefix = '', primary = false) => readdirSync(join(dir, prefix), {withFileTypes: true}).sort((a,b)=>a.name.localeCompare(b.name, 'en')).flatMap(entry => {
  if (primary && entry.name === '.kicadai') return [];
  assert(!entry.isSymbolicLink(), 'Evidence cannot traverse symlinks');
  const path = join(prefix, entry.name);
  return entry.isDirectory() ? walk(dir, path, primary) : [path];
});
const inventory = dir => walk(dir).map(path => { const bytes = readFileSync(join(dir,path)); return {path,bytes:bytes.length,sha256:hash(bytes)}; });
const requiredStages = ['schematic','schematic_electrical','placement','routing','project_write','writer_correctness','validation','simulation','kicad_checks'];
const cases = [];
for (const name of ['standalone_regulator','controller_adc_100ma']) {
  const dir = join(root,name);
  const promotion = json(join(dir,'promotion.json'));
  assert.equal(promotion.report.status,'pass');
  const requirement=json(join(dir,'requirement.json'));
  const selected=promotion.report.candidates.filter(candidate=>candidate.fingerprint===promotion.report.selected.fingerprint);
  assert.equal(selected.length,1);
  assert.equal(selected[0].status,'pass');
  const attempt=selected[0].attempts.at(-1);
  assert.equal(attempt.score.critical_failures,0);
  assert.equal(attempt.score.failures,0);
  assert.equal(attempt.assertions.length,requirement.requirements.behavioral_requirements.length);
  for(const behavior of requirement.requirements.behavioral_requirements) {
    const assertions=attempt.assertions.filter(assertion=>assertion.requirement_id===behavior.id);
    assert.equal(assertions.length,1);
    const assertion=assertions[0];
    assert.equal(assertion.pass,true);
    assert(Number.isFinite(assertion.actual));
    if(behavior.min!==undefined) assert(assertion.actual>=behavior.min);
    if(behavior.max!==undefined) assert(assertion.actual<=behavior.max);
  }
  const request = json(join(dir,'workflow_request.json'));
  assert.equal(promotion.report.selected_circuit_hash,request.explicit_circuit.resolution_hash);
  assert.equal(request.explicit_circuit.closed_loop.selected_circuit_hash,request.explicit_circuit.resolution_hash);
  const runs = [];
  for (const run of ['first','second']) {
    const result = json(join(dir,run+'_workflow.json'));
    assert.equal(new Set(result.stages.map(s=>s.name)).size,result.stages.length,'Duplicate stage');
    const stage = name => result.stages.find(s=>s.name===name);
    for (const name of requiredStages) assert.equal(stage(name)?.status,'ok',name);
    assert.equal(result.acceptance.achieved,'erc-drc');
    assert.equal(stage('routing').summary.failed_nets,0);
    assert.equal(stage('routing').summary.routed_nets,stage('routing').summary.net_count);
    assert.equal(stage('writer_correctness').summary.skipped_count,0);
    assert.equal(stage('writer_correctness').summary.fail_count,0);
    const checks = stage('kicad_checks').summary;
    for (const kind of ['erc','drc']) {
      assert.equal(checks[kind+'_required'],true);
      const check = checks[kind];
      assert.equal(check.status,'pass');
      assert.equal(check.project_context,'full');
      assert(check.command.includes('--severity-all'));
      assert(check.command.includes('--exit-code-violations'));
      assert.equal(check.summary.total_findings,0);
      const rawPath = check.report_path;
      assert(rawPath.startsWith(join(dir,run)+ '/'));
      const raw = json(rawPath);
      if (kind === 'drc') {
        assert.deepEqual(raw.violations,[]);
        assert.deepEqual(raw.unconnected_items,[]);
      } else {
        assert(Array.isArray(raw.sheets));
        assert(raw.sheets.every(sheet => Array.isArray(sheet.violations) && sheet.violations.length===0));
      }
    }
    runs.push({run,stages:requiredStages.length,routed_nets:stage('routing').summary.routed_nets,failed_nets:0,erc_findings:0,drc_findings:0,writer_skipped:0,fabrication_ready:result.acceptance.fabrication_ready});
  }
  const first = walk(join(dir,'first'),'',true), second = walk(join(dir,'second'),'',true);
  const onlyFirst=first.filter(path=>!second.includes(path)), onlySecond=second.filter(path=>!first.includes(path));
  assert(first.some(path=>path.endsWith('.kicad_sch')) && first.some(path=>path.endsWith('.kicad_pcb')) && first.some(path=>path.endsWith('.kicad_pro')));
  const files = first.filter(path=>second.includes(path)).map(path => {
    const a=readFileSync(join(dir,'first',path)),b=readFileSync(join(dir,'second',path));
    assert(a.equals(b),'Byte-exact project replay differs: '+path);
    return {path,bytes:a.length,sha256:hash(a)};
  });
  cases.push({name,components:request.explicit_circuit.components.length,runs,project_files:files,replay:{common_files_byte_exact:true,normalization:'none',only_first:onlyFirst,only_second:onlySecond,full_tree_identical:onlyFirst.length===0&&onlySecond.length===0,note:'Native SVG export subsequently created first-run .kicad_prl local-state files. They are retained; no strict whole-tree replay pass is claimed.'},diagnostic_subtrees:'.kicadai retained and hashed separately, not compared as generated project content',readability:'failed; separately recorded visual review'});
}
const raw = inventory(root);
const base='44223a065e645f7abccf67d66988edea925ec9a9';
const git=(...args)=>execFileSync('git',args,{cwd:repo,encoding:'utf8'}).trim();
const paths=[...new Set([...git('diff',base,'--name-only','--diff-filter=ACMR','--','internal').split('\n'),...git('ls-files','--others','--exclude-standard','--','internal').split('\n')])].filter(p=>p.endsWith('.go')).sort();
const source=paths.map(path=>({path,sha256:hash(readFileSync(join(repo,path)))}));
const result={schema:'kicadai.explicit-reference-offline-verification.v1',root,base,source,cases,inventory:{file_count:raw.length,total_bytes:raw.reduce((n,f)=>n+f.bytes,0),sha256:hash(JSON.stringify(raw))},provider_calls:0,frozen_evaluation_cases_added:0};
const receiptPath=join(phase,'verification.json');
if(existsSync(receiptPath)) assert.deepEqual(result,json(receiptPath),'Source or native evidence changed since receipt');
console.log(JSON.stringify(result,null,2));
