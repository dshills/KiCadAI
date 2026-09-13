# Local source and evidence review

Assessment: share with caveats. The bounded reference contract is verified locally; the practical-board milestone is not achieved. This is a local review, not an independent external review or PR approval.

## Source checks

- Traced the declared return from normalized domain through participant search obligation, physical lowering, exact symbol-pin/pad selection, load/stimulus harness and observation reference.
- Checked invalid and missing explicit IDs fail closed. Name/order-based selection cannot override a declaration. New field omission preserves legacy JSON. Version, scalar-pin and reviewed-isolation gates remain enforced.
- Confirmed explicit declarations agree with common or side-specific objective references. Broader isolated-converter and side-role conventions are not silently enabled; unsupported forms are rejected. No claim of general multi-reference board promotion is made.
- Review caught a compatibility issue in transient/event harness defaults. Final code applies the new lookup only when an explicit field exists and respects the semantic target namespace. Final native verification was rerun after that correction.
- Provider schema uses the existing strict serialization mechanism. Offline schema and request-cap regressions pass; no API/network client is involved.
- Prior phase source/evidence receipts are immutable historical snapshots; only this phase's receipt describes the new source hashes.

## High-severity remaining finding: schematic readability

Both native schematic renders fail human-review readiness. Vertical symbol stacking, generic net names, detached net-label groups and label/symbol overlap obscure electrical function. The controller/op-amp/support region is particularly congested. ERC cannot establish readability. Do not count either example as a complete practical-board success or send these outputs for fabrication based on the native pass alone.

No layout fix was applied in this phase. A new approved phase should establish functional grouping, visible reference/rail semantics, non-overlapping labels and component text, connector purpose/pin clarity and inspection at a specified print/zoom scale. The negative images and original outputs must remain intact.

## Evidence caveat: render side effects

The SVG exporter created `.kicad_prl` state files in each first-run project. All 37 common generated files are identical, but the post-render inventories differ. The verifier records this as non-identical full trees rather than deleting or silently normalizing the extra files. Render from copies after sealing inventories in future validation.

## Evidence quality checks

Grain is one synthetic electrical design with two native generations, not four independent boards and not frozen evaluation cases. Required stage identities are unique and complete; raw ERC/DRC reports confirm zero findings; routed and total net counts reconcile; selected circuit hashes match promoted writer requests. Source and raw inventory hashes bind the receipts. Historical raw runs, archives, publications, sealed executables and prior phase reports are preserved. The 150 mA thermal rejection remains a rejection.

No causality or generalization claim is made from these two fixtures. Firmware behavior, physical prototypes, assembly, full mechanical/thermal-layout review and fabrication eligibility have not been established. The full milestone remains open and requires new scope approval before another repair/evaluation phase.
