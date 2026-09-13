# Diagnosis and bounded development

The plan was recorded before implementation. This phase used no provider request and no edits to old evidence, generated native projects or electrical requirements.

## Reproduction

The frozen schematic transaction has 115 non-writing operations. An opt-in diagnostic reads that transaction and the recorded native library inventory. Its whitelist accepts only project setup, symbol/footprint binding, no-connect and connect operations. It does not call the project writer, KiCad CLI or a provider.

The first diagnostic failed at waypoint-anchor validation because it omitted the resolver-preference flag used by the real workflow. That failure is retained in `diagnostic-frozen.log` and its source/execution receipt. The corrected reconstruction applies exactly that workflow hydration flag to an in-memory operation copy, without changing the frozen file. Two subsequent reproductions both report 52 labels and the original failure after three visits.

The constrained-first solver arrangement includes:

| Label | Candidate count | Only candidate glyph rectangle, mm |
|---|---|---|
| `SUPPORT_PARTICIPANT_CONTROLLER__MCU_PA13` | 1 | x 330.20–331.47, y 335.28–386.08 |
| `composition_net_003` | 1 | x 311.15–335.28, y 349.25–350.52 |

Their rectangles cross. Raising the existing 50,000-visit limit would not make this arrangement satisfiable. [diagnosis.json](diagnosis.json) binds the exact labels, candidates, source/log hashes and the original transaction/library hashes.

Two early runner syntax errors occurred while scaffolding the new JavaScript runner, before any child test process, source snapshot or native invocation started. They were corrected before the recorded execution series. No successful native result was overwritten or hidden.

## Implementation

`ownership-v4` is explicitly selected and validated; no default changes. Its layout-only role normalization preserves caller-owned electrical metadata. Joint support placement checks the support's own pin/label corridors against both bodies and label corridors of already placed components. Packing includes those corridor extents. All ordinary native annotation search bounds, wire/glyph collision checks and electrical checks are unchanged.

A read-only builder diagnostic exposes constrained label candidates while restoring the builder's original schematic and routed-label bookkeeping. It rejects absent/legacy schematics and missing required routed labels. Unit tests verify repeatability, lack of aliasing/mutation and explicit failure on an unsatisfiable panel.

The recorded-layout diagnostic changes only the drawing profile in the frozen workflow request. It produces a separate diagnostic JSON transaction and no native project. The annotation solver succeeds with 51 labels. The final native transaction exactly matches that diagnosed projection. The label-count change follows changed route islands; independent emitted pin/net connectivity, not label count, is the correctness gate.

## Attempt accounting

| Attempt | Result | Difference |
|---|---|---|
| Development 1 | Both examples pass all native stages and replay | First joint-corridor implementation |
| Development 2 | Both examples pass | Diagnostic API input/error hardening plus regular isolated-diagnostic regression; drawing algorithm unchanged |
| Development 3 | Not used | No invocation/root/receipt |
| Frozen final | Both examples pass | Same 16-file Go source as development 2; no subsequent Go edits |

All first/second workflow receipts, native reports, round-trip files, source snapshots and logs are retained. Development 2 and final have identical requests and primary project files. [attempts.json](attempts.json) authenticates the complete three-invocation ledger.

The final visual result is deliberately negative for complete readability, despite technical success. The larger paper choices, all eleven decoupler-distance regressions and remaining label/flow problems are documented in [VISUAL_REVIEW.md](VISUAL_REVIEW.md). No post-final repair or additional native attempt was made.
