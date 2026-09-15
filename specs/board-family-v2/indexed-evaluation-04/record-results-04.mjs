// Post-run publication only. Manual judgments below are source-bound to the
// unchanged, authenticated batch; they neither repair answers nor call an API.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {authenticateBatch,reviewTemplate,scoreReview} from '../source-reference-candidate-03/scoring.mjs';
import {hash,inventory,durableJSON} from '../evaluation/acceptance-lib.mjs';

const destination='specs/board-family-v2/indexed-evaluation-04';
const batch='.cache/board-family-v2/indexed-final-04';
const manifest='.cache/board-family-v2/indexed-runtime-03-02/manifest.json';
const approval='.cache/board-family-v2/indexed-final-04-approval.json';
const plan='specs/board-family-v2/source-reference-candidate-03/evaluation-plan.json';
const mode=process.argv[2];
assert.ok(['--record','--check'].includes(mode));
for(const key of ['OPENAI_API_KEY','ANTHROPIC_API_KEY','GEMINI_API_KEY','GOOGLE_API_KEY','KICADAI_LIVE_PROVIDER_TESTS'])assert.ok(!process.env[key],`${key} must be unset`);
assert.equal(hash(manifest),'d3996a549ae1995f933179f8f73e93e21e23cb559465830ec5291b9ecc8322e4');
const inputs=authenticateBatch(plan,manifest,batch,approval);
assert.equal(inputs.state.status,'collection-complete-semantic-review-pending');
assert.equal(inputs.state.recorded_outcomes,14);
assert.equal(inputs.state.recorded_model_failures,2);
const review=reviewTemplate(inputs);
review.status='source-bound-semantic-review';
review.reviewer={kind:'implementing-agent',name:'Codex implementing agent; not independent review'};
review.reviewed_utc='2026-09-15T11:17:57Z';

// Each tuple is [meaning correct, source/quantity scope correct, rationale].
// Requirements are explicit supporting fact indices, or null where the frozen
// whole-extraction structural check or a missing/corrupted fact prevents credit.
const judgments=[
  {id:'useful-01',complete:false,omitted:[],requirements:[0,2],
    notes:'Both requested facts are present, but an invented affirmative wireless fact causes false refusal of a supported wired pressure board.',
    facts:[
      [true,true,'Clause 1 explicitly asks for pressure measurement. No named sensor is inferred.'],
      [false,false,'Clause 1 asks for a wired monitor, not wireless operation. The affirmative wireless requirement is invented.'],
      [true,true,'Clause 2 expressly accepts the standard profile and reviewed defaults.'],
    ],decision:[false,false,'False refusal: the wireless restriction is true for the catalog but irrelevant to this wired request. No board was generated.']},
  {id:'useful-02',complete:true,omitted:[],requirements:[0,1,2,3,4,5],
    notes:'All six requirements and their negation/quantity roles are faithful. Fast is inferred by the application from exact values, not invented as an explicitly named profile fact.',
    facts:[
      [true,true,'Clause 0 explicitly requests BMP280.'],
      [true,true,'Quantity 0 in clause 0 is 2.2 kilohm pull-ups, exactly 2200 ohms.'],
      [true,true,'Quantity 1 in clause 0 is a 400 kilohertz I2C bus, exactly 400000 Hz.'],
      [true,true,'Quantity 2 in clause 0 declares 100 pF total bus capacitance.'],
      [true,true,'Clause 1 says humidity is not needed; not_required preserves the exclusion without inventing a prohibition.'],
      [true,true,'Clause 1 says wireless telemetry is not needed; the state is not_required, not required.'],
    ],decision:[true,true,'The exact BMP280 fast configuration with 100 pF and reviewed operating defaults was generated. All 39 compared native/export deliverables matched the reviewed example.']},
  {id:'useful-03',complete:false,omitted:[],requirements:[null,null,null,null,null,null,null],
    notes:'The intended requirements are textually represented, but invalid cross-clause quantity citations invalidate the complete extraction. Correct individual facts cannot rescue a structurally invalid response. No facts or outputs were repaired.',
    facts:[
      [true,false,'BMP280 is explicitly required in clause 0. The additional citation to clause 2 (normal operating limits) does not support sensor identity.'],
      [true,true,'Clause 0 explicitly selects the pull-up-only low_current profile.'],
      [true,true,'Quantity 0 in clause 0 denotes 10k pull-ups, exactly 10000 ohms.'],
      [true,true,'Quantity 1 in clause 0 denotes a 100 kHz bus, exactly 100000 Hz.'],
      [true,true,'Quantity 2 in clause 0 denotes 100 pF total capacitance.'],
      [true,false,'Clause 1 excludes whole-board power optimization, but quantity 0 is an unrelated 10k pull-up value in uncited clause 0. The recorded validator rejects fact 5 for invalid, duplicated or uncited source quantity.'],
      [true,false,'Clause 1 excludes battery operation, but the attached quantity 0 belongs to uncited clause 0 and measures pull-up resistance, not battery scope.'],
    ],decision:[false,false,'Safe withholding is not useful-case success: a generic extraction-error clarification neither generates the supported board nor asks about an actual unresolved user requirement.']},
  {id:'useful-04',complete:true,omitted:[],requirements:[0,1,3,4,2],
    notes:'Both measurements, the standard profile, 70 pF and the pressure exclusion are preserved without inventing a named sensor.',
    facts:[
      [true,true,'Clause 0 explicitly requests an indoor temperature monitor.'],
      [true,true,'Clause 0 also explicitly requests humidity measurement.'],
      [true,true,'Clause 2 says pressure measurement is not needed.'],
      [true,true,'Clause 1 explicitly selects the standard profile.'],
      [true,true,'Quantity 0 in clause 1 declares exactly 70 pF total bus capacitance.'],
    ],decision:[true,true,'The application selected SHT31 standard from required measurements, retained 70 pF and defaults, and produced 42 matching native/export deliverables.']},
  {id:'useful-05',complete:true,omitted:[],requirements:[0,1,3,2],
    notes:'The explicitly named SHT31 fast configuration, bus loading and global heater prohibition are faithfully preserved.',
    facts:[
      [true,true,'Clause 0 explicitly names SHT31.'],
      [true,true,'Clause 0 explicitly selects the fast profile.'],
      [true,true,'Clause 1 says not to run the heater at any time, supporting forbidden heater operation.'],
      [true,true,'Quantity 0 in clause 0 states 100 pF total bus loading.'],
    ],decision:[true,true,'SHT31 fast with 100 pF and reviewed defaults was generated, with the heater-off contract retained and all 42 native/export comparisons passing.']},
  {id:'choice-01',complete:true,omitted:[],requirements:[],
    notes:'No sensor, measurement or electrical requirement is supplied. Empty facts are correct; no capability is invented from indoor context or politeness.',facts:[],
    decision:[true,true,'The application asks exactly which measurement is needed and does not silently choose a family or generate a board.']},
  {id:'choice-02',complete:true,omitted:[],requirements:[0,1],
    notes:'Both explicitly unresolved alternatives are retained; their named sensor facts satisfy the frozen alternative-based rubric without duplicate measurement facts.',
    facts:[
      [true,true,'Clause 0 presents BMP280 pressure sensing as one either/or possibility, not a selected requirement. The later unchosen statement corroborates uncertain.'],
      [true,true,'Clause 0 presents SHT31 humidity sensing as the other either/or possibility. Neither family is required or forbidden.'],
    ],decision:[true,true,'The targeted measurement/family question resolves the genuine choice and does not refuse a fabricated combined-sensor demand.']},
  {id:'choice-03',complete:true,omitted:[],requirements:[0,1,2],
    notes:'SHT31 is selected while the two named profiles remain explicitly undecided. The application does not silently default.',
    facts:[
      [true,true,'Clause 0 directly selects SHT31.'],
      [true,true,'Clause 1 says standard is one undecided profile alternative.'],
      [true,true,'Clause 1 says fast is the other undecided profile alternative.'],
    ],decision:[true,true,'The application asks which I2C profile to use. Its extra BMP280-only low_current option is unnecessary UX detail but explicitly labeled as belonging to the other family, not an incorrect selection.']},
  {id:'refuse-01',complete:true,omitted:[],requirements:[0,1],
    notes:'The conjunction and prohibition on dropping either sensor are preserved along with the two required measurements.',
    facts:[
      [true,true,'Clause 0 requests BMP280 together with SHT31; clause 2 reinforces that neither sensor may be omitted.'],
      [true,true,'Clause 0 also requires SHT31; clause 2 rejects single-sensor substitution.'],
      [true,true,'Clause 1 requires pressure with humidity; clause 2 reinforces the mandatory combined scope.'],
      [true,true,'Clause 1 also requires humidity; clause 2 rejects omitting the corresponding sensor.'],
    ],decision:[true,true,'The refusal correctly explains that neither reviewed family combines both measurements and does not substitute a single-sensor board.']},
  {id:'refuse-02',complete:true,omitted:[],requirements:[0,1,2,3,4],
    notes:'All fixed quantities and the selected SHT31 standard profile are retained, including the out-of-range 100 pF rather than a substituted default.',
    facts:[
      [true,true,'Clause 0 explicitly requests SHT31.'],
      [true,true,'Clause 0 selects standard, and clause 2 explicitly requires keeping that profile unchanged.'],
      [true,true,'Quantity 0 in clause 0 denotes 4.7k pull-ups, exactly 4700 ohms.'],
      [true,true,'Quantity 1 in clause 0 denotes 100 kHz I2C, exactly 100000 Hz.'],
      [true,true,'Quantity 2 in clause 1 declares 100 pF; it is not replaced by the 70 pF supported default.'],
    ],decision:[true,true,'The refusal explains the standard-profile 50–70 pF bound and retains the stated incompatible value without changing profile or sensor.']},
  {id:'refuse-03',complete:false,omitted:['Required low_current profile is corrupted into a forbidden profile.'],requirements:[0,null,2,3],
    notes:'Question-form wording is an affirmative configuration request. The model wrongly marks the requested low_current profile forbidden. The refusal happens to remain correct because 10k does not match a reviewed SHT31 profile.',
    facts:[
      [true,true,'Clause 0 explicitly requests the SHT31 board.'],
      [false,false,'Clause 0 asks for low_current; it never forbids that profile. Catalog incompatibility must not be rewritten as user prohibition.'],
      [true,true,'Quantity 0 in clause 0 requests 10k I2C pull-ups, exactly 10000 ohms.'],
      [true,true,'Quantity 1 in clause 1 declares 100 pF total capacitance.'],
    ],decision:[true,true,'The refusal truthfully states that the profile/pull-up request does not match a reviewed SHT31 profile and makes no substitution. This correct final refusal does not earn raw-extraction credit.']},
  {id:'refuse-04',complete:true,omitted:[],requirements:[0,2],
    notes:'The temporally scoped startup prohibition and later affirmative heater operation are both retained. The first does not erase the second.',
    facts:[
      [true,true,'Clause 0 directly requests SHT31.'],
      [true,true,'Clause 1 forbids heater operation specifically at startup; its source citation preserves that local scope.'],
      [true,true,'Clause 2 explicitly requires energizing the heater once normal readings begin.'],
    ],decision:[true,true,'The refusal explains the reviewed heater-off condition, which excludes the later required heater operation at any time. No board or generic firmware exception was supplied.']},
  {id:'refuse-05',complete:false,omitted:[],requirements:[null,null,null],
    notes:'Positive USB and wireless demands are represented, but cross-clause quantity citations invalidate the extraction and an external-adapter exclusion is misclassified as a GPIO-load prohibition.',
    facts:[
      [true,true,'Clause 0 explicitly requests BMP280.'],
      [true,true,'Clause 0 directly demands USB power and its cited quantity 0 is 5 V in the same clause.'],
      [true,true,'Direct 5 V supply supports a 5 V upper input bound; quantity 0 is correctly located in clause 0.'],
      [true,true,'Direct 5 V supply also supports a 5 V lower input bound; quantity 0 is correctly located in clause 0.'],
      [true,false,'Clause 1 affirmatively requires wireless telemetry, but the attached 5 V quantity belongs to uncited clause 0. The recorded validator rejects fact 4 for invalid, duplicated or uncited source quantity.'],
      [false,false,'No external adapter is not a ban on external GPIO load. The attached 5 V quantity is also from uncited clause 0 and does not support this feature.'],
    ],decision:[false,false,'Generic extraction-error clarification is not the required truthful refusal explaining both unsupported direct 5 V USB power and wireless telemetry. Withholding an invalid configuration is safe but not task success.']},
  {id:'refuse-06',complete:false,omitted:[],requirements:[0,11],
    notes:'The accuracy guarantee is correctly recognized, but the response invents five prohibitions and treats an accuracy tolerance as ambient operating bounds. Correct refusal must not hide these extra-fact errors.',
    facts:[
      [true,true,'Clause 0 explicitly names SHT31.'],
      [true,true,'Clause 0 explicitly requests temperature measurement.'],
      [true,true,'Clause 0 also explicitly requests humidity measurement.'],
      [true,true,'Clause 0 asks to use standard electrical defaults; retaining standard is consistent with that stated preference.'],
      [false,false,'Neither cited clause states a heater prohibition. Catalog heater-off restrictions must not become user facts; the 0.1 C tolerance does not support this prohibition.'],
      [false,false,'The user excludes calibration and bench characterization, not delivered firmware. Neither cited clause nor the accuracy quantity supports this prohibition.'],
      [false,false,'No custom-geometry prohibition is expressed. The accuracy tolerance and standard electrical defaults do not establish one.'],
      [false,false,'No external-GPIO-load prohibition is expressed. The 0.1 C accuracy tolerance does not describe GPIO load.'],
      [false,false,'No wireless-operation prohibition is expressed in either cited clause. A catalog radio-off condition is not an extracted user demand.'],
      [false,false,'Quantity 0 is an accuracy tolerance within 0.1 degrees C, not a maximum operating ambient temperature. Correct numeric bytes do not prove a correct quantity role.'],
      [false,false,'The same accuracy tolerance is not a minimum operating ambient temperature. The cited clause explicitly concerns guaranteed accuracy.'],
      [true,true,'Clause 1 explicitly demands assembled-board accuracy within the cited 0.1 C tolerance without calibration/bench work. The feature correctly preserves that guarantee demand.'],
    ],decision:[true,true,'The refusal truthfully explains that software qualification cannot guarantee assembled-board ambient accuracy without bench characterization. No invented temperature bound is used to claim a working board.']},
];

assert.deepEqual(judgments.map(j=>j.id),review.cases.map(c=>c.id));
for(const [i,c] of review.cases.entries()){
  const j=judgments[i];assert.equal(j.facts.length,c.facts.length);assert.equal(j.requirements.length,c.requirements.length);
  c.raw_complete=j.complete;c.omitted_requirements=j.omitted;c.notes=j.notes;
  for(const [k,f]of c.facts.entries())[f.meaning_correct,f.source_scope_correct,f.notes]=j.facts[k];
  for(const [k,r]of c.requirements.entries()){
    const index=j.requirements[k];r.passed=index!==null;r.fact_indices=index===null?[]:[index];
    r.notes=index===null?'No passing source-bound support under the unchanged whole-extraction structure and meaning checks. '+j.notes:'Supported by fact '+index+': '+c.facts[index].notes;
  }
  [c.decision.correct,c.decision.targeted_or_truthful,c.decision.notes]=j.decision;
}
const result=scoreReview({...inputs,review});
assert.equal(result.raw_passes,9);assert.equal(result.application_passes,11);assert.equal(result.complete_passes,9);
assert.equal(result.acceptance,'complete-acceptance-not-met');
const ledger=JSON.parse(fs.readFileSync(path.join(batch,'ledger.json')));
assert.equal(ledger.entries.length,14);assert.ok(ledger.entries.every(e=>e.status==='completed'));assert.equal(new Set(ledger.entries.map(e=>e.response_id)).size,14);
const receipts=inputs.state.records.map(r=>{
  const root=path.join(batch,r.id,'journal');
  const request=JSON.parse(fs.readFileSync(path.join(root,'request/receipt.json'))),response=JSON.parse(fs.readFileSync(path.join(root,'response/receipt.json')));
  assert.equal(request.sha256,hash(path.join(root,'request/body.bin')));assert.equal(response.sha256,hash(path.join(root,'response.bin')));
  assert.equal(response.http_status,200);assert.equal(response.eof_observed,true);
  for(const k of ['truncated','transport_error','read_error','close_error'])assert.equal(response[k],false);
  return {id:r.id,request_sha256:request.sha256,response_sha256:response.sha256,http_status:response.http_status,eof_observed:response.eof_observed,command_exit_code:r.execution.exit_code,audit_exit_code:r.audit_execution.exit_code,wall_seconds:r.execution.wall_seconds,native_files_compared:Object.keys(r.bundle?.files_compared??{}).length};
});
const sum=key=>ledger.entries.reduce((n,e)=>n+(e[key]??0),0);
const metrics={version:'indexed-evaluation-04-metrics-1',batch_id:'indexed-final-04',request_revision:'indexed-request-04',evaluated_source_commit:'7130f4db199d72468409d2d8d3d6a2f365fb9257',benchmark_id:result.evaluation_id,planned_cases:14,physical_requests:14,accounted_responses:14,unattempted_cases:0,retries:0,recorded_model_failures:2,raw_passes:result.raw_passes,application_passes:result.application_passes,complete_passes:result.complete_passes,useful_cases:5,verified_useful_native_bundles:receipts.filter(r=>r.native_files_compared>0).length,native_files_compared:receipts.reduce((n,r)=>n+r.native_files_compared,0),input_tokens:sum('input_tokens'),output_tokens:sum('output_tokens'),estimated_micro_usd:sum('estimated_micro_usd'),historical_reserved_micro_usd:sum('reserve_micro_usd'),unresolved_reservations:0,total_collection_wall_seconds:inputs.state.total_wall_seconds,median_all_useful_seconds:result.median_all_useful_seconds,maximum_all_useful_seconds:result.maximum_all_useful_seconds,timing_target_met:result.timing_target_met,acceptance:result.acceptance,receipts,caveats:['Known 14-case regression corpus, not an unseen holdout or statistical reliability estimate.','Implementing-agent semantic review, not independent review.','Hashes and offline replay establish local byte/source consistency, not provider-signed attestation or resistance to a malicious evidence author.','Cost is ledger-estimated from verified response usage at the frozen rate, not an account invoice. Historical reservations are not additive settled spend.','Timing covers all five useful requests, including two failures; it is not five successful board-generation times.','Native/export validation is software qualification only, not fabrication or bench approval.','The previous stopped indexed-final-03 batch and failed_or_unknown reservation remain unchanged.']};
assert.equal(metrics.input_tokens,43291);assert.equal(metrics.output_tokens,1697);assert.equal(metrics.estimated_micro_usd,20037);assert.equal(metrics.verified_useful_native_bundles,3);assert.equal(metrics.native_files_compared,123);
const archive=path.join(destination,'batch');
const copies=[[approval,'approval.json'],[manifest,'runtime-manifest.json'],['.cache/board-family-v2/indexed-runtime-03-02/qualification.json','runtime-qualification.json'],['.cache/board-family-v2/indexed-runtime-03-02/contract.json','runtime-contract.json'],['.cache/board-family-v2/request-grounding-review-04.md','readiness-review.md']];
if(mode==='--record'){
  assert.ok(!fs.existsSync(archive));fs.mkdirSync(archive,{mode:0o700});
  for(const file of inventory(batch)){const to=path.join(archive,file);fs.mkdirSync(path.dirname(to),{recursive:true,mode:0o700});fs.copyFileSync(path.join(batch,file),to,fs.constants.COPYFILE_EXCL);}
  for(const [from,name]of copies)fs.copyFileSync(from,path.join(destination,name),fs.constants.COPYFILE_EXCL);
  for(const [name,value]of [['review.json',review],['results.json',result],['metrics.json',metrics]])durableJSON(path.join(destination,name),value,{exclusive:true});
}else{
  for(const [name,value]of [['review.json',review],['results.json',result],['metrics.json',metrics]])assert.deepEqual(JSON.parse(fs.readFileSync(path.join(destination,name))),value);
}
assert.deepEqual(inventory(archive),inventory(batch));
for(const file of inventory(batch))assert.equal(hash(path.join(archive,file)),hash(path.join(batch,file)));
for(const [from,name]of copies)assert.equal(hash(path.join(destination,name)),hash(from));
assert.deepEqual(authenticateBatch(plan,manifest,archive,path.join(destination,'approval.json')).bindings,inputs.bindings);
console.log(JSON.stringify({status:'complete-batch-reviewed-and-authenticated',archive_files:inventory(archive).length,physical_requests:14,raw_passes:9,application_passes:11,complete_passes:9,verified_native_bundles:3,native_files_compared:123,estimated_micro_usd:20037,acceptance:result.acceptance,live_requests_in_this_check:0}));
