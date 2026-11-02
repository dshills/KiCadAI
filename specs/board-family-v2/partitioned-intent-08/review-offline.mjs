// Read-only schema/native equivalence review; no model, network, key or retry.
// This authenticates local bytes, not provider attestation or model semantics.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { normalizeReplay } from '../development/replay-normalization.mjs';

const require = createRequire(import.meta.url);
const Ajv = require('ajv/dist/2020');
const [directory, mode] = process.argv.slice(2);
assert.ok(directory && (!mode || mode === '--check'), 'usage: node review-offline.mjs REVIEW_DIRECTORY [--check]');
const root = path.resolve(directory);
const sha = b => crypto.createHash('sha256').update(b).digest('hex');
const read = p => JSON.parse(fs.readFileSync(p, 'utf8'));
const hash = p => sha(fs.readFileSync(p));
function safeFile(base, relative) {
  assert.ok(relative && !path.isAbsolute(relative) && relative.split('/').every(x=>x && x!=='.' && x!=='..'), 'invalid relative path');
  let current = base;
  for (const part of relative.split('/')) { current=path.join(current,part); assert.ok(!fs.lstatSync(current).isSymbolicLink(), 'symlink in evidence path'); }
  assert.ok(fs.statSync(current).isFile(), 'not a regular file');
  return current;
}
function inventory(base, prefix='') {
  return fs.readdirSync(path.join(base,prefix),{withFileTypes:true}).flatMap(entry=>{
    assert.ok(!entry.isSymbolicLink(), 'symlink in evidence inventory');
    const relative=path.posix.join(prefix,entry.name);
    return entry.isDirectory()?inventory(base,relative):[relative];
  }).sort();
}
const corpusPath='specs/board-family-v2/typed-evaluation-02/cases-02.json';
const corpus=read(corpusPath).cases;
assert.equal(corpus.length,14);
const ajv=new Ajv({strict:true,allErrors:true});
const sizes=[];
let negativeControls=0;
assert.deepEqual(inventory(path.join(root,'fixtures')),corpus.map(c=>c.id+'.json').sort());
for(const c of corpus) {
  const fixture=read(safeFile(root,'fixtures/'+c.id+'.json'));
  assert.equal(fixture.synthetic,true);assert.equal(fixture.prompt,c.prompt);
  assert.equal(fixture.expected_disposition,c.expected_disposition);
  assert.equal(fixture.contract.source.request,c.prompt);
  assert.equal(fixture.contract.model,'gpt-4.1-mini-2025-04-14');
  assert.equal(fixture.contract.max_output_tokens,1600);
  const schema=fixture.contract.schema, raw=JSON.parse(fixture.raw_text);
  const validate=ajv.compile(schema);
  assert.ok(validate(raw),JSON.stringify(validate.errors));
  const mutations=[{...raw,version:'old-version'},{...raw,extra:true},{...raw,requirements:null}];
  const qs=Object.keys(raw.quantities);
  if(qs.length) {
    const missing=structuredClone(raw);delete missing.quantities[qs[0]];mutations.push(missing);
    const substitute=structuredClone(raw);substitute.quantities[qs[0]]=[{kind:'feature',value:'skip_crc',state:'requested',context:[]}];mutations.push(substitute);
  }
  for(const mutation of mutations){assert.equal(validate(mutation),false,'invalid fixture passed schema');negativeControls++;}
  const compactBytes=Buffer.byteLength(fixture.raw_text), prettyBytes=Buffer.byteLength(JSON.stringify(raw,null,2));
  assert.equal(compactBytes,fixture.compact_bytes);assert.equal(prettyBytes,fixture.pretty_bytes);
  sizes.push({id:c.id,compact_bytes:compactBytes,indented_bytes:prettyBytes,raw_sha256:sha(fixture.raw_text),schema_sha256:sha(JSON.stringify(schema))});
}
const publicationPath='specs/board-family-v2/evidence/examples-01.json';
const publication=read(publicationPath);
assert.equal(publication.cases.length,5);
let authenticatedExamples=0;
for(const c of publication.cases) for(const [file,h]of Object.entries(c.files_sha256)) {
  assert.equal(hash(safeFile(c.destination,file)),h,'published example changed: '+c.id+'/'+file);authenticatedExamples++;
}
const qualificationPath='specs/board-family-v2/evidence/deterministic-replay-02.json';
const qualified=read(qualificationPath);
assert.equal(hash(qualificationPath),publication.replay_sha256);
const native=[];
for(const c of corpus.filter(c=>c.expected_disposition==='supported')) {
  const config=read(path.join(root,'native',c.id,'output/configuration.json'));
  assert.equal(config.family,c.family);assert.equal(config.profile,c.profile);
  assert.equal(config.total_bus_capacitance_pf,c.total_bus_capacitance_pf);
  const sensor={esp32_sht31_v1:'sht31',esp32_bmp280_v1:'bmp280'}[config.family];
  assert.ok(sensor,'unknown family');
  const name=sensor+'-'+c.profile;
  const existing=qualified.cases.find(x=>x.id===name);
  assert.ok(existing,'no reviewed example for '+name);
  const example=publication.cases.find(x=>x.id===name);
  const base=path.join(root,'native',c.id,'output');
  const validation=read(path.join(base,'validation.json')), reference=read(path.join(example.destination,'validation.json'));
  assert.equal(validation.passed,true);assert.equal(validation.kicad_version,'10.0.3');
  assert.equal(validation.checks.length,14);assert.ok(validation.checks.every(x=>x.passed&&!x.error));
  assert.deepEqual(validation.checks.map(x=>x.name),reference.checks.map(x=>x.name));
  assert.deepEqual(validation.native_sha256,reference.native_sha256);
  const manifest=read(path.join(base,'manufacturing/manifest.json'));
  assert.equal(manifest.source_pcb_sha256,validation.native_sha256['board.kicad_pcb']);
  assert.equal(Object.keys(manifest.files_sha256).length,19);
  for(const [file,h]of Object.entries({...validation.native_sha256,...Object.fromEntries(Object.entries(manifest.files_sha256).map(([f,h])=>['manufacturing/'+f,h]))}))
    assert.equal(hash(safeFile(base,file)),h,'new manifest mismatch: '+file);
  const compared={};
  const deliverables=inventory(base).filter(f=>/^(?:lib|footprints)\//.test(f)||/^(?:board\.kicad_(?:pcb|sch|pro)|sym-lib-table|fp-lib-table|bom\.(?:json|csv)|configuration\.json|electrical\.json)$/.test(f)||/^preview\/(?:board|pcb)\.svg$/.test(f)||/^manufacturing\//.test(f)&&f!=='manufacturing/manifest.json'&&!f.endsWith('.log'));
  assert.deepEqual(deliverables,Object.keys(existing.files).sort(),'complete deliverable inventory differs');
  for(const [file,prior]of Object.entries(existing.files)) {
    const a=fs.readFileSync(safeFile(example.destination,file)), b=fs.readFileSync(safeFile(base,file));
    const left=normalizeReplay(file,a),right=normalizeReplay(file,b);
    assert.equal(sha(left.content),prior.compared_sha256,'reviewed comparison differs');
    assert.deepEqual(right,left,'native deliverable changed: '+name+'/'+file);
    compared[file]={raw_sha256:sha(b),compared_sha256:sha(right.content),normalized_fields:right.normalized_fields};
  }
  native.push({id:c.id,configuration:name,passed_gates:14,manifest_files:19,compared});
}
assert.equal(native.length,5);
const nativeReceipt=read(path.join(root,'native-receipt.json'));
assert.equal(nativeReceipt.code,0);assert.equal(nativeReceipt.live_api_requests,0);
assert.equal(hash(path.join(root,'native.stdout.log')),nativeReceipt.stdout_sha256);
assert.equal(hash(path.join(root,'native.stderr.log')),nativeReceipt.stderr_sha256);
assert.equal(hash('cmd/kicadai-board-family/grounded_review_test.go'),nativeReceipt.source_sha256);
const sources={};
for(const dir of ['internal/boardfamily','internal/aiprovider','cmd/kicadai-board-family'])
  for(const name of fs.readdirSync(dir).filter(x=>x.endsWith('.go')))sources[dir+'/'+name]=hash(dir+'/'+name);
const evidence=Object.fromEntries(inventory(root).filter(x=>x!=='review.json').map(f=>[f,hash(safeFile(root,f))]));
const result={status:'offline-review-pass-not-live-acceptance',live_api_requests:0,reviewer:'implementing-agent',
 ajv_version:require('ajv/package.json').version,ajv_entry_sha256:hash(require.resolve('ajv/dist/2020')),
 recipe_sha256:hash(fileURLToPath(import.meta.url)),normalization_sha256:hash('specs/board-family-v2/development/replay-normalization.mjs'),
 corpus_sha256:hash(corpusPath),publication_sha256:hash(publicationPath),source_sha256:sources,evidence_sha256:evidence,
 schema_valid_synthetic_cases:sizes.length,schema_negative_controls:negativeControls,synthetic_visible_json_sizes:sizes,
 token_count_status:'No tokenizer or token-count endpoint used. Byte lengths are not measured provider tokens; exact fixture sizes do not bound free-form model output or prove completion.',
 published_example_files_authenticated:authenticatedExamples,native_cases:native,
 compared_deliverables:native.reduce((n,c)=>n+Object.keys(c.compared).length,0),
 limitations:['Synthetic extractions, not model accuracy.','Local hashes and deterministic comparisons, not independent certification.','Only declared timestamps normalized in memory; raw output is never repaired.','No physical qualification, fabrication or new live authorization.']};
const output=path.join(root,'review.json');
if(mode==='--check')assert.deepEqual(read(output),result,'review evidence changed');
else fs.writeFileSync(output,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({status:result.status,schema_cases:sizes.length,negative_controls:negativeControls,example_files:authenticatedExamples,native_cases:native.length,compared_deliverables:result.compared_deliverables,compact_bytes:[Math.min(...sizes.map(x=>x.compact_bytes)),Math.max(...sizes.map(x=>x.compact_bytes))],indented_max_bytes:Math.max(...sizes.map(x=>x.indented_bytes))}));
