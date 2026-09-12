# Local source, evidence and visual review

## Disposition

Do not promote `ownership-v3` as a successful two-board profile. This is an opt-in experimental drawing implementation with a reproducible negative controller result. Preserve the implementation and evidence locally; no PR, API campaign, external review, merge, release or fabrication is part of this phase.

The review used source inspection, offline regression/race tests, independent emitted-file checks and native visual exports. It is not a second human or external-model review. Credential-safety and evidence-validation workflows kept provider keys out of test subprocesses and kept technical, drawing and benchmark claims separate.

## Unresolved findings

1. **Blocking for profile promotion: controller annotation placement remains unsatisfied.** The frozen controller stops at `project_write` with `native annotation placement exhausted after 3 bounded candidate visits`. Writer correctness, post-write validation and native KiCad checks are skipped; the workflow simulation stage is absent. The controller's earlier closed-loop behavioral assertions pass, but that is not a substitute for the missing completed workflow. No final controller schematic/PCB, replay or pin/pad equality is certified.
2. **Complete readability is still not achieved on the emitted regulator.** Four bypass parts now follow VIN-left/VOUT-right, and the intact local panels explain exact connections. However, long opaque net labels dominate, the circuit still relies on label-only islands instead of an immediately traceable input-to-output drawing, and reference/value placement is inconsistent: C1's reference is substantially below its symbol while its value is beside it. The interfaces occupy a detached left-hand region, with output J3 still left of the regulator. These are visual limitations despite a clean glyph audit and native ERC.
3. **Pin-side correctness trades away distance.** All eleven measured decouplers are farther from their owner than in ownership-v2. The four regulator decouplers average 37.503 mm versus 33.020 mm; the seven controller transaction decouplers average 46.459 mm versus 35.923 mm. Controller values describe the retained pre-write transaction, not a successful native result. Report every regression, including the small R3 support regression; do not use the improved C10/J5 distances to imply aggregate readability success.
4. **The placement stages remain only loosely coupled.** Component support placement reserves pin-label corridors, while the writer later searches label, field and panel locations. The final failure after only three candidate visits shows an unsatisfied candidate arrangement, not exhaustion of the 50,000-visit cap. Increasing that cap would not itself demonstrate a fix. A future phase should first expose/reproduce the conflicting candidate geometry offline, then coordinate placement and annotation constraints without changing electrical names.
5. **Broader net-role handling is not certified.** Post-freeze source review found that support-side priority special-cases only the literal `power`, and exclusion only the literal `ground`/`no_connect`. The IR adapter passes net-role strings through; IR also permits `power_pos`, `power_neg` and `return`. Those variants are not normalized here, so a supply variant can tie with signal/bias instead of yielding to it, or a return role can influence side choice. The evaluated examples use canonical `power`/`ground`, so their measured results are unaffected. This needs normalization or explicit rejection and regression tests before broader profile promotion. It is recorded, not repaired after freeze.

## Source checks

- The new profile is explicit in composition lowering, IR validation and the layout adapter. Ownership-v1/v2 retain their layout paths and legacy annotation output; the v2 provenance validation also applies to v3. No new profile is selected by default.
- Pin side comes from exact shared endpoints and transformed resolver pin direction, with geometry fallback when direction is unavailable. Nonground signal/bias wins over supply; ambiguous equal-priority sides remain ambiguous. Component IDs are not interpreted for voltage, ADC assignment, reset or SWD semantics.
- Support placement remains deterministic and bounded, preserves fixed-placement requests, and returns the original positions on cycles or exhausted support search. The native writer remains fail-closed. No emitted project was hand-edited.
- Local block metadata is deep-cloned, validated, serialized through `CreateProjectOperation`, and passed to the builder. Blocks cannot coexist with the global guide; unknown/duplicate IDs or references, empty/oversized text and missing anchors fail explicitly. Panels are indivisible under the bounded placement search.
- The newly exposed existing-wire label omission has a profile-scoped fix: a same-named label on a disconnected island cannot suppress an unlabeled wired island's required label. A regression test verifies no extra stub, no duplicate label and no relocation away from the original island. The change does not alter legacy profiles. The failed final controller does **not** establish native recovery of the original controller ERC issue.
- Whole-body left/right/top/bottom placement, cardinal rotation, mirror, off-center pin direction, shuffled input, ambiguous attachments, fixed placement, long-label corridors, provenance, block isolation/clone/validation and idempotence have targeted offline tests. Actual JSON request replay is independently authenticated for the successful regulator only.

## Final visual coverage

Inspected the full regulator schematic, both group crops and both F.Cu/B.Cu layer exports. The two final native runs produce byte-identical primary files, so one rendering covers their identical drawing. The failed controller has no final native sheet or copper layers to inspect. Development-1 controller imagery is retained as development evidence only; it is not substituted for final evidence.

- Regulator pin-side association: 4/4 eligible supports pass the independent emitted-library-angle/origin audit and visual inspection.
- Local panels: 2/2 emitted panels intact and visibly associated with their groups. The independently checked 7-line interface panel and 5-line regulator panel are matched by physical rows, because native serialization sorts text by UUID.
- Complete visual readability: 0/1 emitted final example passes; with the ungenerated controller retained in the planned denominator, 0/2 planned examples pass.
- Copper review: both layers were inspected; exact historical geometry/pin-pad connectivity and clean full-project DRC support technical preservation, not manufacturability certification. `fabrication_ready` remains false.

![Final regulator schematic](/tmp/kicadai-pin-aware-readability-offline-v1-final/standalone_regulator/render/offline_regulated_output.png)

## Authority boundary

The three development native attempts and one frozen final attempt are exhausted. No Go source changed after the final snapshot. The next useful step requires a separately approved, bounded diagnostic/repair phase for joint pin-label/support placement. Do not silently retry, rename nets, relax checks, alter requirements or count offline examples as new benchmark passes.
