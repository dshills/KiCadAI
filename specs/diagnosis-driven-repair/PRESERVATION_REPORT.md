# Local Preservation Report

## Passed

- `go test ./internal/repairloop ./internal/opentopologysynthesis ./internal/designworkflow -count=1 -timeout 20m`
- protected-current repair trace replay and bounded taxonomy tests
- six-component, twelve-pad real-router blocked-to-routed replay
- installed-KiCad dense repair: writer correctness, zero-diff round trip,
  internal connectivity/route validation, clean ERC, and strict DRC
- six-design/four-adversarial architecture-generalization acceptance
- installed-KiCad architecture-generalization promotion: five promoted designs
  retain their published synthesis, topology, physical, project, and evidence
  hashes; protected current remains fail-closed
- installed-KiCad Class A, Class AB, and notch promotion with unchanged hashes
- ten-case simulation-grounded offline workflow

## Aggregate-suite boundary

`go test ./... -count=1 -timeout 30m` passed every reported package except
`internal/compositionlowering`, where the pre-existing
`TestDynamicElectrothermalControlLoopCorpusPassesOfflineWorkflow` reached the
30-minute package timeout while evaluating
`sequenced_dual_rail_controller.json`. The timeout stack was inside nonlinear
transient/electrothermal simulation. The focused ten-case
simulation-grounded corpus passed separately in 218 seconds, and none of the
changed repair packages failed.

No GitHub Actions run was used.

## Prism review

Prism reviewed the staged diff with the configured Gemini provider. Its two
reported high-severity findings were adjudicated as false positives against
the staged source: `AutonomousCorrectionReport.Applied` is declared as `int`,
and `crypto/sha256` plus `encoding/hex` are present in the import block. A
post-review staged compile/test run covering the shared trace, protected-current
replay, dense physical recovery, and report JSON round trip passed.
