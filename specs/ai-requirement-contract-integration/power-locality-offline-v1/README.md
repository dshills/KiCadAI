# Visible local power rails: bounded offline result

Both preserved examples now show all regulator-to-capacitor supply paths as visible conductors, with closer capacitors and unchanged A4/A2 paper sizes. Both pass strict native technical checks and byte-exact project replay. **Complete readability remains 0/2, and this phase adds no practical-board benchmark passes.**

This is the user-authorized “start next” follow-up to `e6f648bbbb05d1d8bc883e61356511d8708aec2e`. It used two of three permitted development evaluations and one frozen final; the remaining development slot was deliberately unused. No API calls, external review, push, PR, merge, release or fabrication occurred. [Scope](PLAN.md), [development record](DEVELOPMENT.md), [local review](REVIEW.md).

## What changed

The opt-in `ownership-v6` drawing profile improves pure blocks containing one active device and its capacitors. It prefers the actual supply-pin axis, permits same-net power/return corridors to share rail access, honors the declared spacing without extra radial padding, and roots power trees at the active component. A demonstrated routing defect is fixed in this profile: pin access leaves the complete padded body envelope before turning, rather than stopping one grid step from a pin while still inside that envelope. All route/body/foreign-net checks remain enforced. Mixed MCU/support/interface blocks retain the previous routing and placement policy.

All comparisons below use the immediately preceding ownership-v5 final, not an earlier failure or selected development image.

| Drawing measure | Regulator: v5 → v6 | Controller ADC 100 mA: v5 → v6 |
| --- | --- | --- |
| Regulator supply pins visibly connected to both capacitors | 0/2 → 2/2 | 0/2 → 2/2 |
| Regulator-to-capacitor visible paths | 0/4 → 4/4 | 0/4 → 4/4 |
| Paper | A4 landscape → A4 landscape | A2 landscape → A2 landscape |
| Mean decoupler-origin distance, mm | 48.47 → 45.80 | 58.62 → 57.09 |
| Direct transaction branches | 5 → 8 | 12 → 15 |
| Labeled geometric wire islands | 13 → 10 | 42 → 39 |
| Emitted labels | 16 → 13 | 46 → 43 |
| Repeated labels on an existing island | 3 → 3 | 4 → 4 |
| Drawn wire length, mm | 391.16 → 322.58 | 1,073.15 → 1,057.91 |
| Complete readability | 0/1 → 0/1 | 0/1 → 0/1 |

The [independent emitted pin/wire audit](power-rail-audit.json) uses physical drawing islands, without joining matching labels or shorting pins through components. It confirms all eight regulator-to-capacitor paths. All eight regulator capacitors move closer: inner pairs 35.56 → 33.12 mm, outer pairs 61.38 → 58.48 mm. The remaining three decouplers and other explicit support distances are unchanged within floating-point tolerance; no measured capacitor regresses. These are schematic origin distances, not PCB placement distances. [Complete distance evidence](readability-metrics.json), [wire measurements](wiring-metrics.json).

All 13 unambiguous support-side associations and all six note panels remain intact; J5 is still explicitly ambiguous and excluded from that side denominator. [Pin/panel audit](pin-aware-audit.json). All 14 final images were inspected. Anonymous and redundant labels, inconsistent fields, external interface ordering, remote MCU support and fragmented analog/controller signal flow still prevent complete readability. [Visual review](VISUAL_REVIEW.md).

## Final technical evidence

The [final verifier](verification.json) certifies both examples, each with an actual JSON-decoded second request and byte-identical complete primary project tree. All nine required stages pass in all four workflow runs: schematic, schematic electrical, placement, routing, project write, writer correctness, validation, simulation and KiCad checks. Writer skips/failures and strict all-severity violation-sensitive full-project ERC/DRC findings are zero.

| Preserved property | Regulator | Controller ADC 100 mA |
| --- | --- | --- |
| Physical components | 8 | 21 |
| Connected physical pin/pad endpoints | 16 | 52 |
| Intentional unconnected pads | 0 | 26 |
| Primary files per replayed project | 14 | 23 |
| Copper layers | 2 | 4 |
| PCB segments / vias | 37 / 3 | 203 / 29 |
| Supply voltage | 3.3 V | 3.3 V |
| Computed junction temperature | 103.0495 °C | 114.0495 °C |
| ADC cutoff | Not applicable | 923.3613 Hz, required 900–1,100 Hz |

Requirement bytes, complete request fields except `functional_profile`, component values/nets/pads, simulation assertions, seeds and physical-board geometry are unchanged. Cross-phase PCB comparison resolves numeric net IDs and excludes drawing-paper settings only; same-phase project replay uses no normalization. Both remain `fabrication_ready: false`. This is no physical-board improvement or engineering/manufacturing sign-off.

The original 150 mA controller thermal rejection remains unchanged, not relabeled as the 100 mA recovery case. Its regression is `TestEndpointContractsReachRealLoweredNetsAndWriterRequests/regulator_filter_controller_150ma_thermal_rejected`.

## Test and attempt accounting

The single full short-suite invocation passed **151 tested packages**, with 14 additional packages reporting no test files: `go test ./... -short -count=1 -timeout=12m`. It ran from `2026-09-12T16:15:43.309Z` to `2026-09-12T16:28:18.596Z`; the longest package, `internal/opentopologysynthesis`, took 688.503 seconds, within the unchanged 12-minute per-package limit. `internal/compositionlowering` passed in 86.317 seconds, including the preserved 150 mA thermal rejection. [Execution receipt](full.execution.json), [full log](full.log).

Final focused regressions, all declared scoped short race partitions, repository-wide vet and scoped lint passed. [Report QA](qa.json) checks all 23 execution receipts, their exact logs and source snapshots, including the retained negative development outcomes. No provider credentials were passed to these execution children and dependency-network access was disabled.

Scoped short races cover schematiclayout/schematicir/designapi and focused compositionlowering ownership/pin-aware/joint/local-wiring/power-locality regressions. The historical full compositionlowering race timeout remains uncertified; these partitions do not erase it.

[All native attempts](attempts.json): development 1 passed the regulator but failed the controller's first strict ERC check with `label_multiple_wires` and `unconnected_wire_endpoint`; its second controller run was not attempted. Development 2 and final pass both examples and both replays. Final source, complete requests and primary project bytes match development 2. No third development run or post-final native retry occurred.

[Diagnostic accounting](diagnosis.json) preserves three successful non-writing layout/candidate pairs, two failed early unit-test invocations and the failed native controller outcome. Candidate passes were not treated as ERC passes. All source snapshots/logs are retained, including the broad initial projection rejected for worse MCU-support distance and the subsequent restricted implementation.

## Integrity and retained artifacts

The final freezes 16 changed/new Go files. Native execution ran from `2026-09-12T16:12:08.010Z` to `2026-09-12T16:12:36.643Z`; source snapshot SHA-256 is `08940fe78b21f76fd6023dea72e71fffe915e8f88b87e2fa5034600661a301b7`. No Go repair or native rerun followed the freeze.

The final raw root contains **387 files, 1,531,267,624 bytes**, inventory SHA-256 `6b0a51eb3d51023ede5c9737ffc47cb7bdbfd16612da5e987856b22c9cc5ce79`. Originals were sealed before exports; rendering used disposable copies, and originals were rechecked. [Image hashes](visual-review.json), [history authentication](history-authentication.json), [report QA](qa.json).

The [archive](archive.json) stream-authenticated **1,094 exact raw/source files**: 297 from development 1, 387 from development 2, 387 from final, plus 23 execution-source snapshots. Archive size: 232,971,592 bytes; SHA-256 `531c3c4ce8444883dfafdf80356659df04d9e936e70f72d0ecab1e59430546bc`. Zero AppleDouble members; no full re-extraction; all originals retained and unchanged; exact current credential absent. [Member manifest](archive-manifest.json). These are local integrity checks, not external attestation.

Historical authentication covers the seven preceding offline source commits, raw inventories and archives, plus the original frozen campaign. Historical inputs/results, classification and denominators are unchanged.

Final previews: [regulator](/tmp/kicadai-power-locality-offline-v1-final/standalone_regulator/render/offline_regulated_output.png), [controller](/tmp/kicadai-power-locality-offline-v1-final/controller_adc_100ma/render/offline_controller_adc.png). Native projects: [regulator schematic](/tmp/kicadai-power-locality-offline-v1-final/standalone_regulator/first/offline_regulated_output.kicad_sch), [controller schematic](/tmp/kicadai-power-locality-offline-v1-final/controller_adc_100ma/first/offline_controller_adc.kicad_sch).

## Overall milestone and stopping rule

This reused offline pair is not a fresh AI baseline-to-pass campaign. The original benchmark remains 0/8 complete primary boards, 1/4 refusals, 0/2 clarifications and 0/2 paraphrases; [original results](../RESULTS.md). The target of at least six complete positives, two materially different new live baseline-to-pass designs and a reviewed PR remains unachieved.

The frozen final ends this phase. Preserve the measured power-readability improvement and the incomplete overall result. The unused development slot is not permission for a post-final repair. Further implementation/evaluation, API work, external review or PR work needs separately authorized scope.
