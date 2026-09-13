# Compact local wiring: bounded offline result

Both preserved examples now use roughly half their previous schematic paper area and have fewer disconnected labeled wire islands. Both pass strict native technical checks and exact replay. **Complete visual readability remains 0/2; this phase adds no practical-board benchmark passes.**

This is the user-approved offline follow-up to `3c1572f7cd9a8120b45e16c42c67500a5fb1097b`. It used three development evaluations and one frozen final. No API calls, external review, push, PR, merge, release or fabrication occurred. [Preregistered scope](PLAN.md), [development history](DEVELOPMENT.md), [local review](REVIEW.md).

## What improved, and what did not

All comparisons use the immediately preceding ownership-v4 final outputs, not a selected older failure or development run.

| Measure | Standalone regulator: v4 → v5 | Controller ADC 100 mA: v4 → v5 |
| --- | --- | --- |
| Schematic paper | A3 landscape → A4 landscape | A1 portrait → A2 landscape |
| Paper area, mm² | 124,740 → 62,370 | 499,554 → 249,480 |
| Direct transaction branches | 0 → 5 | 4 → 12 |
| Labeled geometric wire islands | 18 → 13 | 50 → 42 |
| Emitted labels | 18 → 16 | 51 → 46 |
| Repeated labels on the same island | 0 → 3 | 1 → 4 |
| Drawn wire length, mm | 104.14 → 391.16 | 703.58 → 1,073.15 |
| Mean decoupler-origin distance, mm | 50.10 → 48.47 | 58.58 → 58.62 |
| Complete readability | 0/1 → 0/1 | 0/1 → 0/1 |

The selected paper shrinks by 50.0% and approximately 50.1%, respectively. Labeled islands shrink by 27.8% and 16.0%. These are drawing metrics, not electrical yield or fabricated-board results. The added wires make some local relationships visible, but also introduce long rectangles and repeated same-island annotations. Explicit support parts are not uniformly closer: regulator C3/C4 move from about 50.10 to 61.38 mm; controller mean explicit support distance grows from 56.60 to 61.11 mm. See [independent emitted-wire measurements](wiring-metrics.json) and [complete locality measurements](readability-metrics.json).

The [visual review](VISUAL_REVIEW.md) records the remaining opaque rail naming, indirect VIN/VOUT associations, remote support parts, mixed field placement and fragmented filter/ADC/MCU flow. All 14 final images were inspected. Thirteen of thirteen unambiguous pin-side attachments and all six local note panels pass the [independent emitted audit](pin-aware-audit.json); J5's multi-sided MCU association remains explicitly ambiguous. None of these narrower passes implies complete readability.

## Final technical evidence

The [final verifier](verification.json) certifies both examples, each with an actual JSON-decoded second request and byte-identical complete primary project tree. There is no replay normalization. Every one of the nine required stages passed in both runs: schematic, schematic electrical, placement, routing, project write, writer correctness, validation, simulation and KiCad checks. Writer skips/failures and full-project all-severity ERC/DRC findings are zero.

| Final property | Regulator | Controller ADC 100 mA |
| --- | --- | --- |
| Physical components | 8 | 21 |
| Physical connected pin/pad endpoints | 16 | 52 |
| Intentional unconnected pads | 0 | 26 |
| Primary files per replayed project | 14 | 23 |
| Copper layers | 2 | 4 |
| PCB route segments / vias | 37 / 3 | 203 / 29 |
| Supply voltage | 3.3 V | 3.3 V |
| Computed junction temperature | 103.0495 °C | 114.0495 °C |
| ADC cutoff | Not applicable | 923.3613 Hz, required 900–1,100 Hz |

All requirement bytes, complete request fields other than `functional_profile`, electrical values/nets/pads, simulation assertions and placement/generation seeds match ownership-v4. Cross-phase PCB geometry is exact after numeric net-ID resolution and exclusion of drawing-paper settings only. Both remain `fabrication_ready: false`; no physical-board layout improvement or manufacturing sign-off is claimed.

The original 150 mA controller thermal rejection is unchanged, rather than relabeled as the 100 mA recovery case. Its regression remains `TestEndpointContractsReachRealLoweredNetsAndWriterRequests/regulator_filter_controller_150ma_thermal_rejected`; full-suite coverage is recorded below.

## Tests and attempt accounting

All **151 tested packages passed** the one full short-suite invocation; 14 packages have no tests. It ran from `2026-09-12T14:07:27.277Z` to `2026-09-12T14:19:51.563Z`, with the unchanged 12-minute per-package bound and no retry. The longest package, `opentopologysynthesis`, took 678.803 seconds; `compositionlowering`, including the unchanged 150 mA rejection regression, passed in 83.801 seconds. Final focused regressions, scoped short race checks, vet all and scoped lint also passed. Exact commands and source hashes are in [full.execution.json](full.execution.json) and [qa.json](qa.json).

Scoped races cover all short schematiclayout/schematicir/designapi tests and focused ownership/pin-aware/joint/local-wiring compositionlowering tests. The older full compositionlowering race timeout remains uncertified; the focused partition does not erase it.

[attempts.json](attempts.json) records all three development runs and the single final: each passed both technical examples and both request runs. Six diagnostic executions precede these native evaluations: three layout projections passed, and all three candidate checks failed with retained evidence before the repair was completed. Diagnostic failures are not converted into passes or counted as native boards. [diagnosis.json](diagnosis.json) binds their frozen inputs, source snapshots, transactions and logs.

## Authentication and retained artifacts

The final source contains 17 changed/new Go files and was frozen at `2026-09-12T14:02:36.713Z`; final native execution started one millisecond later and completed at `2026-09-12T14:03:03.609Z`. Source snapshot SHA-256: `e901d838de6727e7c43ce7558204d4185d41da1426d1727a5e5ef7595812f9b2`. No Go repair or native rerun followed the freeze.

The final raw root contains **387 files, 1,531,358,688 bytes**, inventory SHA-256 `c20782beadb33271b399fe509d9b2cf80e0717a78f2faff8b6fef1f985a40a3f`. Originals were sealed before exports, disposable copies were rendered, and originals were rechecked. Exact same-phase replay and final-to-dev3 primary-tree identity are separately verified.

[Historical authentication](history-authentication.json) covers the six preceding offline phases, including ownership-v4 committed source, raw inventory and archive, and the original frozen campaign. [Archive member manifest](archive-manifest.json), [archive authentication](archive.json), [image hashes](visual-review.json) and [final report QA](qa.json) retain the complete current-phase evidence. Original raw roots and source snapshots are kept; no historical evidence was deleted.

The archive stream-authenticated **1,571 exact raw/source files**: all four native roots plus 23 execution-source snapshots. It is 311,024,054 bytes, SHA-256 `afefb58edfe2ff75996b2a789d09ba6d103bf934b2523b82bf2cdcf54f67f2a5`, with zero metadata members and no full re-extraction. Every original member remains unchanged, and the exact current credential is absent from retained evidence. These are local integrity checks, not a third-party attestation.

Final schematics: [regulator](/tmp/kicadai-local-wiring-readability-offline-v1-final/standalone_regulator/render/offline_regulated_output.png) and [controller](/tmp/kicadai-local-wiring-readability-offline-v1-final/controller_adc_100ma/render/offline_controller_adc.png). Native projects: [regulator schematic](/tmp/kicadai-local-wiring-readability-offline-v1-final/standalone_regulator/first/offline_regulated_output.kicad_sch) and [controller schematic](/tmp/kicadai-local-wiring-readability-offline-v1-final/controller_adc_100ma/first/offline_controller_adc.kicad_sch).

## Overall milestone and boundary

This reused offline pair is not a new AI baseline-to-pass campaign. The original benchmark remains 0/8 complete primary boards, 1/4 refusals, 0/2 clarifications and 0/2 paraphrases; see the unchanged [campaign results](../RESULTS.md). The goal of at least six complete frozen positives, two materially different new live baseline-to-pass designs and a reviewed PR is still unachieved.

The approved iteration allowance is exhausted. Preserve the measured improvement and the negative complete-readability outcome. Further implementation/evaluation, live API testing or PR work requires separately authorized scope; none is implicitly started here.
