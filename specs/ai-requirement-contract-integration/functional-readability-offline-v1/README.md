# Functional grouping: technical pass, readability incomplete

The frozen offline evaluation preserves both boards and exact serialized replay, but **0/2 examples meet complete schematic-readability expectations**. The full short suite passes; one bounded race package timed out, and a support-parent hardening finding remains open. This is a bounded experimental drawing improvement, not completion of the practical AI-board milestone. No API calls, external review, push, PR, merge, release or fabrication occurred.

## Results

| Final check | Standalone regulator | Controller ADC, 100 mA |
| --- | --- | --- |
| Functional groups | Interfaces; regulation | Interfaces; regulation; analog conditioning; controller |
| Physical components / nets | 8 / 3 | 21 / 10 |
| Strict native stages | 9/9 in both runs | 9/9 in both runs |
| ERC / DRC findings | 0 / 0 | 0 / 0 |
| Writer skips / failures | 0 / 0 | 0 / 0 |
| Exact JSON-decoded project replay | 14 primary files | 23 primary files |
| PCB geometry | All 8 poses, 37 segments, 3 vias unchanged | All 21 poses, 203 segments, 29 vias unchanged |
| Physical pin/pad endpoints | 16 connected; 0 unconnected pads | 52 connected; 26 intentionally unconnected pads |
| Native annotation audit | 18 labels, 10 symbols, 16 note lines; 0 issues | 51 labels, 23 symbols, 39 note lines; 0 issues |
| Complete visual readability | Not achieved | Not achieved |

The entire workflow request is equal to its predecessor after excluding only the explicitly versioned drawing layout. Circuit and generation hashes, placement seed, physical components, net names, values, constraints, simulation and closed-loop evidence are unchanged. Same-phase replay is byte-exact without normalization. Across phases, PCB comparison resolves numeric net IDs to names and excludes drawing-paper orientation; every remaining board field is exact. See [verification.json](verification.json) and its independent [verifier](verify.mjs).

Both examples produce 3.3 V. The regulator thermal result remains 103.0495 °C; the separately recorded 100 mA controller remains 114.0495 °C with 923.3613 Hz cutoff within 900–1100 Hz. The original 150 mA thermal rejection is not relaxed. These are model/native checks, not bench measurements or fabrication approval. The controller remains a four-layer board; the regulator remains two-layer.

## What changed

The opt-in `ownership-v1` policy derives primary membership from the exact selected fragment payload, then follows synthesis support-parent records. Membership and parent/source metadata persist in the schematic layout and survive JSON replay. Unknown profiles, missing members, duplicates and cyclic/invalid recorded ownership are covered by focused checks. The policy is applied only after electrical synthesis, preserving the existing PCB-seed policy rather than redefining its hashing semantics.

Each functional block uses the existing role/topology placement engine locally, then whole blocks are packed into stable rows. Pure decoupling blocks use a centered active device and a two-high support grid. The reading guide lists blocks and their references. Existing profiles remain the default; no CLI/UI or provider-schema default was changed. Enable experimentally through `ArchitectureSimulationPlanResolver.FunctionalLayoutProfile = schematiclayout.FunctionalOwnershipV1`.

## Why readability is still incomplete

The controller's functional regions and MCU support association are easier to distinguish, but improvement is uneven. Short, meaningful local descriptions are still missing; long generated net names and a detached guide dominate reading. Some reference fields sit between capacitor rows, the op-amp value is crowded by a net label, and large whitespace and long inter-block wires remain. A malformed support-parent fallback for boundary roles also needs hardening before broader promotion. See [the review](REVIEW.md).

| Descriptive symbol-origin distance, mm | Before | Final |
| --- | ---: | ---: |
| Regulator: mean decoupling-to-active distance, 4 capacitors | 72.6 | 67.2 |
| Controller: mean decoupling-to-active distance, 7 capacitors | 91.1 | 89.0 |
| Controller: mean explicit support-to-parent distance, 5 parts | 74.3 | 61.6 |

These post-run measures are descriptive, not preregistered acceptance thresholds or PCB distances. They do not imply every part improved: controller C7→U3 increases 112.3→139.1 mm, controller C8→U2 increases 56.4→79.6 mm, and standalone C3→U1 increases 45.7→79.6 mm. [All row-level measurements and methodology](readability-metrics.json) are retained. The regulator changes A4 portrait to landscape; the controller remains A2 landscape. Neither uses a smaller paper size than the preceding final run.

## Verification and preservation

The [preregistered plan](PLAN.md) allowed three development native evaluations and one frozen final. All four are retained, including the first controller's blocked annotation placement. No source repairs or native retries followed the final evaluation. [Development history](DEVELOPMENT.md) records the negative results and helper corrections.

Final source: 15 changed Go files, with exact contents in `.cache/functional-readability-v1-sources/native-final.json`; snapshot SHA-256 `cebd77a2407018e0166173689c5047afc89c749517a47d16568596d34a527933`. The final native execution took 30.978 seconds for the package. Focused final tests, lint and vet passed. The full short suite passed all 151 test packages, with 14 packages having no tests; its longest package took 675.774 seconds, below the unchanged 720-second limit. The composition-lowering race package timed out after 12 minutes in the pre-existing behavioral corpus; schematic IR and layout passed under race detection. The overall race check is therefore incomplete, with no retry or extension. Commands, exits, hashes and source snapshots are in the execution receipts and summarized by [QA](qa.json).

The final raw tree contains 387 files and 1,531,858,072 bytes; inventory SHA-256 `bdfc5ad00ab34601bcdf82121b97c9f69888208429e0c80f0cf5d108c9f80ac1`. Original project trees were sealed before rendering disposable copies and rechecked afterward. Both full sheets, six group-based dense-area crops and all six copper-layer views were visually inspected. The data-quality and validation workflow keeps byte authentication, native success and readability judgment separate.

[History authentication](history-authentication.json) verifies the immediately preceding 10-file source snapshot at `9ac56319`, its 386-file raw tree and 232,891,237-byte archive, the earlier 23-file/170-raw-file negative phase at `446e19c9`, and the preceding readability and original campaign preservation checks. Historical evidence is unchanged. This phase retains its own raw outputs and compressed archive without a second full extraction.

The new [archive receipt](archive.json) authenticates 1,299 raw/source files, including all four native roots and 13 execution-source snapshots. Archive: `.cache/functional-readability-offline-v1-all-runs.tar.gz`, 306,861,937 bytes, SHA-256 `798f6087e1affd6f4c91d7ace58dbd36677ef47d20a955dd2ca771483ea42bdb`. It was created once, verified as a stream against [the manifest](archive-manifest.json), and exact-scanned for the existing API key locally: absent. No API request was made. Source remained frozen through all final checks.

The original practical campaign remains **0/8 complete primary-board passes** ([frozen results](../../practical-sensor-controller-boards/PROTOCOL-V2-RESULTS.md)). This phase adds **zero** benchmark cases or passes. The six-positive/two-new-design milestone, broader live evaluation and reviewed PR remain outstanding.

## Inspect the final designs

- [Regulator schematic](/tmp/kicadai-functional-readability-offline-v1-final/standalone_regulator/render/offline_regulated_output.png)
- [Controller schematic](/tmp/kicadai-functional-readability-offline-v1-final/controller_adc_100ma/render/offline_controller_adc.png)
- [Analog close-up](/tmp/kicadai-functional-readability-offline-v1-final/controller_adc_100ma/render/review-functional_objective_condition_adc.png)
- [Controller close-up](/tmp/kicadai-functional-readability-offline-v1-final/controller_adc_100ma/render/review-functional_participant_controller.png)

The next step requires a separately bounded phase: harden malformed support-parent handling and improve local wiring, support placement, connector descriptions and anchored block headings while preserving electrical net identity and all current evidence. This phase does not authorize that next evaluation.
