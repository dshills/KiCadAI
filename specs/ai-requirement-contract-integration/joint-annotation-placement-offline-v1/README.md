# Joint component and annotation placement — offline v1

**The controller's native-writing regression is repaired: both examples pass the frozen technical evaluation. Complete readability remains 0/2, and no practical-board benchmark pass is added.** The new opt-in `ownership-v4` profile preserves the requirements, electrical inputs and PCB geometry while reserving support-component and annotation corridors together.

This is the approved local-only follow-up to `7647dda9ba4d94d57213fe8f7ecaf5d474c667fe`. See the [preregistered plan](PLAN.md), [diagnosis and development record](DEVELOPMENT.md), [local source review](REVIEW.md), [visual review](VISUAL_REVIEW.md), and [final QA](qa.json). No API request, external review, push, PR, merge, release or fabrication is authorized or performed in this phase.

## Results and denominators

| Gate | Standalone regulator | Controller ADC 100 mA | Total |
|---|---|---|---|
| All 9 strict workflow stages, both native runs | Pass | Pass | 2/2 examples |
| Actual JSON request and byte-exact primary project replay | Pass, 14 files | Pass, 23 files | 2/2 |
| Emitted physical connectivity and historical PCB geometry | Pass | Pass | 2/2 |
| Unambiguous native support-side association | 4/4 | 9/9; J5 explicitly ambiguous | 13/13 eligible attachments |
| Intact local note panels | 2/2 | 4/4 | 6/6 panels |
| Complete visual readability | Fail | Fail | 0/2 |
| New frozen practical-board benchmark passes | 0 | 0 | 0 |

[Independent verification](verification.json) requires each of `schematic`, `schematic_electrical`, `placement`, `routing`, `project_write`, `writer_correctness`, `validation`, `simulation` and `kicad_checks` to pass in both runs. Each final run has zero writer skips, zero writer failures, and zero all-severity, violation-sensitive, full-project ERC/DRC findings. No absent or skipped stage is treated as success.

The regulator has 8 physical components, 3 nets and 16 connected physical endpoints/pads, with no unconnected pads. Its 8 PCB poses, 37 segments and 3 vias are preserved. The controller has 21 physical components, 10 nets and 52 connected physical endpoints/pads, plus 26 intentionally unconnected pads; its 21 poses, 203 segments and 29 vias are preserved. Board comparison resolves numeric net identifiers to names and excludes only the drawing-paper setting; all remaining PCB fields are exact. Same-phase replay has no normalization at all.

Whole workflow requests are compared with the preceding phase, excluding only the versioned schematic layout. For board geometry, both examples are compared to the last successful ownership-v2 native projects; ownership-v3 emitted no final controller board. No circuit was redesigned or silently weakened.

Behavioral assertions remain: regulator output 3.3 V and junction temperature 103.0495 °C; controller output 3.3 V, junction temperature 114.049500001452 °C, and cutoff 923.3612800239786 Hz within the unchanged 900–1100 Hz range. The original 150 mA thermal-rejection fixture is unchanged. Workflow acceptance is `erc-drc`; both `fabrication_ready` values remain false.

## Diagnosed repair and tradeoffs

The [authenticated diagnostic](diagnosis.json) reproduces the ownership-v3 controller failure after exactly three candidate visits. A PA13 label and a supply label each have one legal candidate, and those rectangles intersect. This is not a failure to spend enough search effort. The new profile reserves the support's own pin/label corridors against the owner's and other placed components' corridors; it also includes corridor extents when packing functional groups. The native writer's actual wire/glyph checks remain unchanged and fail closed.

Supported role variants are normalized only in this new drawing projection: `power_pos`/`power_neg` become supply-priority `power`, and `return` becomes excluded `ground`. Electrical role strings, net names, components, values and bindings remain unmodified. Regression tests cover canonical/variant roles, priority conflicts, rotation/mirror, shuffled inputs, read-only diagnostics and legacy isolation.

The repair is not compact. The regulator grows from A4 to A3 landscape, and the controller from A2 to A1 portrait. All 11 measured decouplers move farther from their owners: regulator mean 37.503 → 50.097 mm; controller mean 46.459 → 58.575 mm. J5 becomes closer, while C10/R3 become farther away. These are schematic-origin distances, not PCB distances. [All descriptive measurements](readability-metrics.json) and [the visual findings](VISUAL_REVIEW.md) retain these regressions. Long opaque net names, label-only islands and weak interface-to-signal flow still prevent a complete-readability pass.

## Frozen evaluation and tests

Two development native evaluations and one frozen final evaluation were used; all pass both examples. The third development allowance was unused. Each invocation was bounded to 20 minutes per example and 45 minutes per test process, used new exclusive roots and exact source snapshots, and stripped provider keys. [All attempts](attempts.json) are accounted for. Final requests and all available primary project bytes match development 2 exactly.

Final source: 16 changed Go source/test files, frozen at `2026-09-12T11:57:14.902Z`; snapshot SHA-256 `3105e538b2bb29384dbea0cc36f4e6aed4b86b82d1ece982c152a9eda882d6ca`. There were no subsequent Go changes or native evaluations.

| Check | Result |
|---|---|
| Final focused functional/block/diagnostic tests | Pass across all five scoped packages; environment-gated diagnostics/native cases skip without explicit opt-in |
| Focused compositionlowering ownership/pin-aware/joint race | Pass, 3.105 s |
| Full short schematiclayout / schematicir race | Pass, 3.014 / 176.724 s |
| Full short designapi race | Pass, 2.105 s |
| Vet all packages | Pass |
| Scoped lint | Pass, zero issues |
| Full short suite, once, 12-minute per-package bound | Pass: 151 packages; 14 packages have no tests; slowest `opentopologysynthesis` 674.270 s |

The full suite ran from `2026-09-12T12:01:50.874Z` to `2026-09-12T12:14:08.306Z`, with source unchanged. The compositionlowering package, including the unchanged original 150 mA thermal-rejection fixture, passed in 81.149 s. No full-suite retry or timeout extension was used.

The historical full compositionlowering race timeout is still uncertified. Passing the focused partition does not certify all compositionlowering or repository tests under the race detector. The failed first offline reconstruction is retained as a diagnostic harness failure, not a native attempt or a board failure.

## Evidence custody

Final raw root: `/tmp/kicadai-joint-annotation-placement-offline-v1-final` — 387 files, 1,531,026,223 bytes; inventory SHA-256 `b2a6f27ba2b4928e5ce2513ea65128ad467a6079a5533e0a522c3593ef8af8c6`.

Primary projects were sealed before export. Rendering used disposable copies, then the originals were rechecked. [The visual record](visual-review.json) authenticates the 14 final images inspected: two full schematics, six functional crops and all six copper-layer views. The second request is actually JSON-encoded/decoded and emits byte-identical complete primary project trees.

The [archive manifest](archive-manifest.json) and [archive receipt](archive.json) cover all three native roots and every source snapshot. Member-by-member streaming authentication creates no full extraction; exact local secret scans check the current key is absent. Old evidence is retained, not deleted. [History authentication](history-authentication.json) verifies the preceding committed source/raw/archive and the older preservation chain, including the original campaign.

Archive: `.cache/joint-annotation-placement-offline-v1-all-runs.tar.gz` — 232,216,947 bytes; 1,180 exact raw/source members and zero AppleDouble members; SHA-256 `db853bb88dd2adc9ef9f9893b07a619a7fc5b4fc44ab74dce63f5ff4eaad2af0`. The archive covers 19 execution source snapshots as well as all native originals and disposable review copies. Phase logs, receipts and this review are retained in the local Git commit.

Repeatable evidence checks: `record-verification.mjs dev2 final`, `diagnosis.mjs`, `attempts.mjs`, `visual-review.mjs`, `verify-archive.mjs` and `qa.mjs` in this directory. These checks do not run another evaluation. Development 1 was verified against its own then-current source; its receipt, source and originals remain authenticated by the archive and QA, but the current-source verifier should not be used to reinterpret it after the diagnostic-only hardening in development 2.

## Overall goal

This phase establishes an opt-in technical repair, not the practical AI-board milestone. The original frozen campaign remains 0/8 complete primary boards; refusal, clarification and paraphrase outcomes are unchanged. Two reused offline examples cannot establish six positive frozen passes or two new live baseline-to-pass designs. No new live campaign or PR has been started.

The useful next step is a separately approved bounded readability phase: reduce annotation-envelope expansion, compact support parts and make rails/interfaces/signal flow explicit without changing net identity or electrical requirements. The current implementation and evidence stop at this phase's local review/commit boundary.
