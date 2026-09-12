# Pin-aware schematic readability — offline v1

**Result: phase failed; do not promote `ownership-v3`.** The regulator passes technical checks and demonstrates pin-side support placement plus local explanatory panels. The controller cannot complete native writing. Complete readability remains unachieved, and no practical-board benchmark pass is added.

This is the separately approved offline follow-up to `f8b29a9904dd02c38583cb4c15d72f55b5ee6b75`. It introduces a new opt-in drawing profile; previous ownership profiles and electrical inputs are preserved. See the [preregistered scope and bounds](PLAN.md), [development failures](DEVELOPMENT.md), [local review](REVIEW.md) and [final QA](qa.json).

## Results, with denominators

| Gate | Standalone regulator | Controller ADC 100 mA | Planned total |
| --- | --- | --- | --- |
| All 9 strict native workflow stages, both runs | Pass | Fail at project write | 1/2 |
| Actual JSON request + byte-exact project replay | Pass, 14 primary files | Not reached | 1/2 |
| Emitted physical pin/pad connectivity and historical PCB geometry | Pass | Not certified | 1/2 |
| Native pin-side association | Pass, 4/4 bypasses | No final native output | 1/2 examples |
| Intact, locally associated explanatory panels | Pass, 2/2 panels | No final native output | 1/2 examples |
| Complete visual readability | Fail | Not evaluable; not a pass | 0/2 |
| New frozen practical-board benchmark passes | 0 | 0 | 0 |

The frozen controller fails with `native annotation placement exhausted after 3 bounded candidate visits`. Writer correctness, post-write validation and KiCad checks are skipped, and the workflow simulation stage is absent. The second request replay is not attempted after this failure. [Verification](verification.json) distinguishes these missing guarantees from the regulator's proven ones.

The regulator has 8 physical components, 3 nets and 16 connected physical endpoints/pads, with no unconnected pads. Both runs have zero all-severity full-project ERC/DRC findings and zero writer skips. Its 8 PCB poses, 37 segments and 3 vias match the preceding phase; route hash remains `6e3712832ae5454ac79859b0fff9a2477e215a484701997186befccc9e63b992`. No PCB geometry was redesigned. Acceptance is `erc-drc`, not fabrication-ready.

Both unchanged requirements still pass their pre-write closed-loop behavioral assertions: regulator rail 3.3 V and junction temperature 103.0495 °C; controller rail 3.3 V, junction temperature 114.049500001452 °C and ADC cutoff 923.3612800239786 Hz. These controller simulation results do not replace a completed native workflow. The original 150 mA thermal-rejection fixture remains unchanged.

## What improved and what did not

The [independent emitted pin/panel audit](pin-aware-audit.json) derives attachment direction from exact native library pin angles and symbol transforms. It confirms regulator C1/C3 left of VIN and C2/C4 right of VOUT, with two intact local panels totaling 12 lines. Notes use recorded pin names and net roles; no electrical alias or unproven voltage/function was invented.

The [visual review](REVIEW.md) still finds opaque net names, label-only wiring, weak overall flow and inconsistent reference/value association. The A4 regulator page is not a complete-readability pass. The controller remains A2 portrait in its failed pre-write transaction; no smaller usable native page was demonstrated.

All support distances, including regressions, are retained in [descriptive metrics](readability-metrics.json). Regulator decoupler mean/max changed from 33.020/35.560 mm to 37.503/37.503 mm. Controller transaction decoupler mean/max changed from 35.923/43.180 mm to 46.459/58.640 mm. All eleven decouplers became farther away. Controller C10 and J5 became closer; R3 regressed slightly. These are drawing-origin distances, not PCB distances or readability acceptance.

## Tests and bounded attempts

| Check | Result |
| --- | --- |
| Full short suite, once, 12-minute package bound | Pass: 151 packages; 14 packages have no tests; slowest `opentopologysynthesis` 679.069 s |
| Final focused functional/block tests | Pass in compositionlowering, schematicir, schematiclayout and designapi; transactions has no matching focused test, but passes in the full suite |
| Focused compositionlowering ownership/pin-aware race | Pass, 3.484 s |
| Full short schematiclayout / schematicir race | Pass, 3.283 / 186.659 s |
| Full short designapi race | Pass, 2.343 s |
| Vet all packages | Pass |
| Scoped lint | Pass, zero issues |

The full suite ran from `2026-09-12T11:09:11.441Z` to `2026-09-12T11:21:40.536Z`, with source unchanged. The original 150 mA thermal-rejection test remains covered by the passing compositionlowering package. Full execution, source and log hashes are checked by [QA](qa.json). These offline checks do not override the separate failing native evaluation.

All three development native runs and the one frozen final run exit 1, each with a passing regulator and failing controller. [Every attempt is accounted for](attempts.json); no failed attempt was overwritten. The final Go source was frozen at `2026-09-12T11:09:10.243Z`, with 21 changed source/test files. No Go edits or additional native attempts followed. Development 3 and final have identical requests and available primary project bytes.

The historical full compositionlowering race run remains uncertified because of its preceding bounded timeout. This phase's narrower ownership/pin-aware race pass is not a full compositionlowering or repository race certification.

## Evidence custody

Final raw root: `/tmp/kicadai-pin-aware-readability-offline-v1-final` — 170 files, 1,519,829,973 bytes; inventory SHA-256 `6eb57530693a8badf726b9136e416c81e8c44f64827dbdb90ab46ea0add8120a`.

Frozen source snapshot: `.cache/pin-aware-readability-v1-sources/native-final.json`; SHA-256 `33bf43932079a6f6a9eab43267f1a92d0daf0dfc264ab8884cea2db00e1c3386`.

The [archive receipt](archive.json) and [member manifest](archive-manifest.json) cover all four raw roots and every execution source snapshot. Each member is streamed and hashed without a second full extraction; exact-secret scans verify that the current key is absent. Raw originals are retained. [History authentication](history-authentication.json) verifies prior committed source, raw inventories, archives and the original campaign preservation chain. No historical evidence is modified.

Archive: `.cache/pin-aware-readability-offline-v1-all-runs.tar.gz` — 304,232,946 bytes, 824 raw/source members, zero AppleDouble metadata members; SHA-256 `b7028b8d2656edb083963df01b4e179655f254633c1772279f7039d9c817d064`.

Authentication of the final root and source is repeatable with `node specs/ai-requirement-contract-integration/pin-aware-readability-offline-v1/record-verification.mjs`; archive checking uses `verify-archive.mjs`; report checking uses `qa.mjs`. These are evidence checks, not additional evaluation runs. Do not invoke the exhausted native runner again.

## Goal and next authority boundary

The overall practical AI-board goal remains unachieved: the frozen campaign still has 0/8 complete primary boards, with its previous refusal/clarification/paraphrase results unchanged. These two offline examples are not additional frozen benchmark cases. No API call, external review, push, PR, merge, release or fabrication occurred in this phase.

The next useful work is a separately bounded diagnostic/repair phase that makes pin-label, support-component and annotation placement jointly satisfiable, beginning with the preserved controller conflict. It requires new approval; the present phase ends with this negative result and local evidence/review commit.
