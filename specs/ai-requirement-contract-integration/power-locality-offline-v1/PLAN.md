# Power-pin locality — offline v1

Started on the user's “start next” authorization following local commit `e6f648bbbb05d1d8bc883e61356511d8708aec2e`. This is another bounded offline phase, not a live benchmark or PR campaign.

## Scope

Improve local power-pin/capacitor relationships using actual net roles, resolver pin geometry and functional ownership. Target visible VIN/VOUT-to-capacitor conductors and reduced support distance. Use a separately versioned `ownership-v6` profile; preserve all older profiles. No example-specific IDs, absolute positions, net renaming, selected component changes, or electrical/PCB changes.

Preserve the exact standalone regulator and separately recorded controller ADC 100 mA requirements. Compare every request field except the functional-profile string to ownership-v5. Preserve the original 150 mA thermal rejection. Historical source, reports, raw roots, seals and archives remain immutable.

## Bounds and authority

- At most three development native evaluations and one frozen final; both examples and real JSON-decoded replay in every evaluation. Twenty minutes per example and 45 minutes per native process. Retain all attempts, including failures. No Go repair or native retry after final starts.
- Up to three separately recorded diagnostic layout/candidate pairs over immutable recorded input; no KiCad/provider calls or generated-project writes in diagnostics.
- Final verification: one full short suite with unchanged 12-minute per-package timeout; focused regressions; short schematiclayout/schematicir/designapi race partitions and focused compositionlowering ownership tests; vet all and scoped lint. Historical full compositionlowering race timeout is not erased by scoped coverage.
- Require 12 GiB free before each native evaluation and retain a 10 GiB reserve. Keep all originals; archive and stream-authenticate every attempt and execution source snapshot without full extraction.
- Local implementation, tests, evidence, visual/local review and a local commit are in scope. No API calls, external review, credential/firewall changes, push, PR, merge, release or fabrication. Strip provider credentials from execution children; use the existing key in memory only for exact local evidence secret scans.

## Acceptance and honest reporting

Require all nine technical workflow stages, zero writer skips/failures, strict all-severity violation-sensitive full-project ERC/DRC, exact physical pin/pad connectivity, byte-exact primary project replay and unchanged PCB geometry (only numeric net IDs and drawing paper excluded in cross-phase comparison).

Measure all 11 decouplers, all explicit support attachments, geometric wire islands, labels and paper area against the immediate ownership-v5 baseline. Specifically report whether each regulator supply pin reaches both same-rail capacitors by visible conductors, rather than inferring it from logical net labels. Preserve pin-side associations and all six local note panels. Inspect complete final sheets, dense blocks and every copper layer on sealed disposable exports. Native correctness, narrower drawing improvements and complete readability are separate outcomes.

Final failure or exhausted allowance ends this phase with a negative result; do not add retries or expand scope. These two reused offline examples add no frozen practical-board benchmark passes. The overall six-of-eight/live baseline-to-pass milestone remains unmet unless independently demonstrated by its original acceptance set.
