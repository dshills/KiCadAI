# Compact local wiring — offline v1

Approved follow-up to `3c1572f7cd9a8120b45e16c42c67500a5fb1097b`, preregistered before implementation and evaluation. The user approved compacting both schematics and reducing label-only wiring, limited to three development evaluations and one final evaluation, with no API calls or PR. This is not a new live benchmark campaign; the overall practical AI-board goal remains unachieved.

## Scope and invariants

Use reusable layout/routing rules and recorded pin metadata to improve local direct connections, support-part proximity, page utilization and human signal flow. Diagnose the existing routing policy first. Introduce a separately versioned drawing profile if necessary; preserve all older profile behavior. Do not use example-specific coordinates, component IDs, net renaming, altered values or relaxed checks.

The standalone regulator and separately recorded controller ADC 100 mA requirement bytes remain unchanged. Preserve the original 150 mA thermal rejection, all electrical components, values, net names, pin/pad bindings, simulation requirements, seeds and exact PCB geometry. Compare complete requests except the versioned drawing profile against the immediate ownership-v4 baseline. Old phase directories, raw evidence, generated projects, snapshots, logs, receipts and archives are immutable.

No API calls, provider probes, external review, credential/firewall changes, push, PR, merge, release or fabrication. A local implementation/test/evidence/review commit is authorized. Remove provider credentials from test and KiCad/render child environments; the current key may be checked in memory solely for exact local secret scans.

## Preregistered bounds

- At most three development native evaluations and one frozen final, each using both unchanged examples and an actual JSON-decoded second-request replay. Each example is bounded to 20 minutes; the Go process to 45 minutes. Exclusive attempt roots and exact execution-source snapshots retain every failure. No Go repair or native retry after final begins.
- Separately recorded offline diagnostics may inspect frozen requests, transactions and recorded library metadata, without provider calls, KiCad calls, generated-project writes or changes to frozen artifacts. Diagnostics are not native evaluations or board passes.
- Final verification includes one full short suite with the unchanged 12-minute per-package bound; focused regressions; full short schematiclayout/schematicir/designapi race partitions and focused ownership/pin-aware/joint/local-wiring compositionlowering races; vet all and scoped lint. The historical full compositionlowering race timeout remains uncertified.
- Require at least 12 GiB free before each native evaluation and retain a 10 GiB reserve. Archive every used native attempt and execution source snapshot; authenticate every archive member by streaming without full extraction. Retain originals and do not delete old evidence for space.
- Seal native originals before exports; render disposable copies only; recheck originals afterward. Inspect every emitted final schematic, dense circuit and annotation regions, and every copper layer. Missing final outputs cannot be substituted with development or historical images.

## Acceptance and evidence

1. Test deterministic local-wiring behavior, electrical connectivity preservation, bounded/fail-closed routing, rotation/mirror where relevant, shuffled inputs, and legacy profile isolation. Show whether label-only islands and paper area actually decrease against ownership-v4, separately from native correctness.
2. Require all nine strict workflow stages for both final examples, zero writer skips, all-severity violation-sensitive full-project ERC/DRC, emitted annotation audit, exact physical pin/pad connectivity, serialized request replay and byte-exact primary project replay. Compare PCB geometry to the immediate successful ownership-v4 native outputs.
3. Independently review support pin-side association, intact locally associated notes, reference/value clarity, input-to-output flow, power/interface intent, page utilization and label/wire clutter. Quantify support distances and emitted wire/label/island counts where feasible. Do not infer complete readability from electrical or geometric checks; retain ambiguous cases explicitly.
4. Authenticate ownership-v4 committed source/raw/archive and the earlier preservation chain. Report exact denominators, every failed/absent/skipped stage, coverage limits and unresolved local review findings. The two reused offline examples cannot add frozen practical-board benchmark passes.

If the allowance is exhausted or final fails, publish the negative outcome without another implementation/native iteration. A further phase needs fresh approval.
