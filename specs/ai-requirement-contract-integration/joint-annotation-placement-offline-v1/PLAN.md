# Joint component and annotation placement — offline v1

Approved follow-up to `7647dda9ba4d94d57213fe8f7ecaf5d474c667fe`, recorded before implementation or evaluation. The full practical AI-board goal remains unchanged and unachieved. This phase cannot convert two offline examples into six frozen positive passes or authorize a live campaign/PR.

## Scope and invariants

Diagnose and repair the preserved controller component/pin-label placement conflict and noncanonical supply/return role handling. Use reusable geometry and recorded metadata, not example-specific coordinates, component-ID naming heuristics, net renaming or relaxed electrical/readability checks. Begin by reproducing the failed candidate arrangement in an offline regression test. Prefer a separately versioned drawing profile when changed placement would otherwise alter frozen profile behavior.

Keep the standalone regulator and separately recorded controller ADC 100 mA requirement bytes unchanged. Retain the original 150 mA thermal rejection. Preserve all electrical components, values, net names, pin/pad bindings, simulation requirements, placement/generation seeds and PCB geometry; old raw evidence, source snapshots, logs, receipts and archives are immutable.

No API calls, external review, new live campaign, push, PR, merge, release or fabrication. A local implementation/test/evidence/review commit is authorized. Provider keys are removed from all tests and child KiCad/render commands. The existing key may be used in memory solely for exact local secret scans; no credential or firewall changes.

## Preregistered bounds

- At most three development native evaluations and one frozen final native evaluation, each using both unchanged requirements and an actual JSON-decoded second-request replay. Bound each example to 20 minutes and the test process to 45 minutes. Use new exclusive roots and exact source snapshots; retain every failure. No Go repair or native rerun after final starts.
- Offline focused regressions may inspect the frozen transaction and recorded library metadata without KiCad CLI calls, provider calls, generated-project writes or modifying the frozen transaction. These diagnostics are not complete native evaluations or board passes; record their commands, source and output separately.
- Final checks: one full short suite with the unchanged 12-minute per-package bound; final focused regressions; full short schematiclayout/schematicir/designapi race partitions and focused ownership/joint-placement compositionlowering race tests, each partition bounded to 12 minutes; vet all and scoped lint. The historical full compositionlowering race timeout remains uncertified.
- Require at least 12 GiB free before every native evaluation, preserving a 10 GiB reserve. Archive all attempts and source snapshots, stream-authenticate every member without full re-extraction, and retain all originals. Do not delete old evidence to obtain space.
- Seal primary project trees before export, render only disposable copies, recheck originals afterward, and inspect every emitted final schematic, dense circuit/annotation regions and every copper layer. A missing final project cannot be replaced by development or historical imagery.

## Acceptance and evidence

1. Reproduce and expose the preserved conflict with actual candidate identities/geometry; add a reusable regression for the resulting repair. Keep deterministic bounded searches and explicit failure for genuinely unsatisfiable arrangements.
2. Normalize or explicitly reject supported noncanonical supply/return role variants before pin-side priority/ground exclusion; cover canonical and variant roles, priority conflicts, rotation/mirror, shuffled input and legacy isolation.
3. Both final examples must pass all nine strict workflow stages, zero-skipped writer checks, all-severity violation-sensitive full-project ERC/DRC, emitted annotation audit, physical pin/pad connectivity, exact serialized request/project replay and unchanged historical PCB geometry. Independently compare the entire request except versioned drawing layout. A pre-write simulation pass is not a native-board pass.
4. Separately review pin-aware side association, locally associated intact panels, reference/value clarity, input-to-output flow, power/interface intent, page utilization and label/wire clutter. Complete visual readability is not inferred from geometric or electrical checks. Report all distances and regressions descriptively against ownership-v3; use the last successful ownership-v2 controller native board for geometry preservation because ownership-v3 emitted no final controller project.
5. Authenticate the immediate preceding committed source/raw/archive and the older preservation chain. Publish exact denominators, all failed/absent/skipped stages, coverage limitations and unresolved local review findings. No new frozen practical-board benchmark passes may be claimed from this phase.

If the allowance is exhausted or the frozen final fails, publish the negative result without another repair/evaluation iteration. A further phase requires new approval.
