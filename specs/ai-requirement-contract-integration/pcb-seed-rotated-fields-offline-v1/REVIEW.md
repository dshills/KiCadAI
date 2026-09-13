# Local implementation and evidence review

## Assessment

The two targeted native defects are corrected on the unchanged standalone regulator and controller ADC 100 mA examples. Final generation, actual JSON replay, exact project replay, physical connectivity, and historical PCB preservation pass. The final full suite passed 151 test packages (14 additional packages have no tests), with the longest package at 670.049 seconds under its 720-second bound. Focused tests, three-package race checks, lint and vet passed. This is ready to share with explicit caveats: it is scoped technical evidence, not a complete human-readability pass, a practical benchmark improvement, or fabrication approval.

## Implementation review

The new optional placement-seed object has an explicit policy and a validated lowercase SHA-256 digest. It is emitted only for the opt-in annotation profile. Request validation and direct placement both reject unknown policies, invalid hashes and use without that profile. Normalization deep-copies the new pointer. Requests without it retain the existing generation-hash/resolution-hash fallback and omit the new field from JSON.

Seed derivation calls the existing deterministic hash routine after removing only the native annotation-profile tag from a value copy of the resolved source. Tests check nonmutation, legacy serialization, equality to the corresponding legacy seed, continued provenance differences and sensitivity to board dimensions and routing constraints. No old seed literal, component coordinate, route segment or example name is embedded in production code. The two final boards independently match their prior native geometry, including all footprint positions, rotations, copper segments and vias.

Field placement accepts cardinal input/parent orientations and cancels the parent rotation when emitting visible properties/custom fields. The readback audit checks combined orientation rather than assuming a serialized zero angle is horizontal. Non-cardinal fields still fail closed. The final unit matrix covers all sixteen cardinal parent/field combinations, both property and custom-field storage, serialization, and unchanged stored geometry. Native render evidence covers the actual orientations present in these two examples; it is not a native-font calibration for every possible symbol/mirror/font combination.

`Design()` remains a snapshot-returning API, not an error-returning write API. The native failure boundary is still enforced by project/schematic write paths. The annotation profile remains opt-in; no provider schema, prompt, default enablement, catalog record or electrical contract changed.

## Remaining findings

1. **Functional readability remains incomplete (medium, high confidence).** Whole-sheet views have excessive empty space, remote capacitor groups and long generated net/source identifiers. The controller U3 reference is beneath the MCU and close to a separate reset-support net label; although glyphs do not intersect, the association is less immediate than a conventional schematic. The guide exposes existing connector roles and net/reference mappings, but is not a compact functional-block drawing. These two examples must not be reported as full practical-board passes.
2. **Geometry model remains bounded (medium, high confidence).** This audit uses conservative rectangles for supported default-size text and known symbol geometry. It is not a general typography proof. The first development run demonstrated why emitted native rendering is required in addition to planner/audit success; that failure is retained.
3. **Placement seed is reproducibility input, not an attestation (low, high confidence).** A direct programmatic caller can supply the versioned seed just as callers previously supplied generation hashes. Syntax/policy checks do not independently reconstruct a resolved graph from the request. Circuit provenance, frozen inputs and emitted PCB authentication must remain separate.

## Visual review

Reviewed both complete final sheets at the recorded 2200-pixel exports, five dense viewports at 254 DPI (10 pixels/mm), both regulator copper layers and all four controller copper layers. The controller is a four-layer board; the plan's shorthand “both copper layers” was applied to the regulator and expanded to every controller layer, without changing any board input.

The final controller analog crop shows upright C1/22n, R1/10k and R2/10k. Dense capacitor, MCU, op-amp, connector and regulator areas show no observed local annotation/wire crossings within the supported audit scope. Crops naturally clip content beyond their viewports; full-sheet exports were also inspected. Layer views are supplementary: raw native DRC and independent pin/pad/net comparison remain authoritative for connectivity, not visual impression.

All exports use disposable copies. Exact project seals are rechecked by `verify.mjs`; same-phase replay includes every primary generated file with no normalization. Across phases only numeric net IDs are resolved to net names and the drawing-paper setting is excluded; all remaining PCB S-expression fields are compared exactly. Both paper settings also happen to match.

## Historical and run evidence

`history-authentication.json` binds all 23 prior source files to `446e19c9`, the prior 170-file raw inventory, its archive, earlier offline evidence and original live campaign preservation. The prior 0/2 result remains unchanged. No live case, baseline, assertion limit, API budget or evaluator binary was rewritten.

Two bounded development native evaluations and one final native evaluation were used. Development 1's visual failure and the expanded test fixture failure are retained in `DEVELOPMENT.md` and the execution/source records. No production source changed after development 2; the final source additionally contains the corrected physical-anchor test fixture. No source changed after final freeze.

The archive was created once. The initial streaming verifier rejected macOS-added AppleDouble metadata; the follow-up verifier inventories and validates those metadata records explicitly while checking every original data byte. This is an archive-verification correction, not another native evaluation or rebuilt archive.

This is a local review by the implementing agent. No external reviewer, provider, PR, push, merge, release or manufacturing action was used or implied.
