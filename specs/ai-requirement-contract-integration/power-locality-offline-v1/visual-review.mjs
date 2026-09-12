// Record the exact final images already inspected by the local reviewer.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const phase=dirname(fileURLToPath(import.meta.url)),root='/tmp/kicadai-power-locality-offline-v1-final';
const names={standalone_regulator:['offline_regulated_output.png','review-functional_objective_regulate.png','review-functional_boundaries.png','review-F.Cu.png','review-B.Cu.png'],controller_adc_100ma:['offline_controller_adc.png','review-functional_objective_condition_adc.png','review-functional_participant_controller.png','review-functional_objective_regulate.png','review-functional_boundaries.png','review-F.Cu.png','review-In1.Cu.png','review-In2.Cu.png','review-B.Cu.png']};
const files=Object.entries(names).flatMap(([name,files])=>files.map(file=>{const path=join(root,name,'render',file),b=readFileSync(path);return {path,bytes:b.length,sha256:createHash('sha256').update(b).digest('hex'),inspected:true};}));
assert.equal(files.length,14);
const result={schema:'kicadai.power-locality-visual-review.v1',root,reviewer:'implementing agent; local review only',files,full_schematics:2,functional_crops:6,copper_layer_images:6,complete_readability_examples:0,planned_examples:2,technical_results:'verification.json',pin_side_and_panel_evidence:'pin-aware-audit.json',findings:'VISUAL_REVIEW.md',limitations:['85 mm group crops include each target panel and may show clipped neighboring groups; complete sheets were also inspected.','No independent external review, fabrication, signal-integrity or assembly sign-off.'],provider_calls:0,benchmark_passes_added:0};
const path=join(phase,'visual-review.json');if(existsSync(path))assert.deepEqual(JSON.parse(readFileSync(path)),result);else writeFileSync(path,JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({images:files.length,complete_readability_examples:0}));
