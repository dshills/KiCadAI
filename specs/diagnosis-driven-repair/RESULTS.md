# Diagnosis-Driven Repair Results

## Outcome

The first shared diagnosis-driven repair evidence loop is implemented.
Electrical and physical repair both emit
`kicadai.diagnosis-driven-repair.v1`, including normalized evidence,
deterministic proposals, stage re-entry, expected effects, bounded consumption,
outcomes, and before/after/result hashes.

The protected programmable current driver remains fail-closed. Two runs are
byte-identical, no physical project is emitted, and the post-change synthesis
result hash is
`a73d7087606e4a19f83b63adeea27e7e7c32df936c50755222668e458080aa8d`.
The preserved gap is specific: bounded graph and catalog value trials do not
resolve the current-transfer error and active-state nonconvergence under the
declared rating, thermal, SOA, and simulation requirements.

The physical repair benchmark expands the identity-neutral catalog seed to
six components, twelve pads, and two required six-endpoint nets. Its initial
real-router result is blocked. Diagnosis-derived relative-spacing repair
re-enters placement and routing, completes both nets with zero failed nets,
preserves protected invariants, and produces identical repair traces and route
operations on a second run.

## Focused reproduction

```sh
go test ./internal/repairloop -count=1
go test ./internal/opentopologysynthesis -run 'TestProtectedCurrentDriverRepairTraceReplaysAndFailsClosedPrecisely|TestElectricalRepairDiagnosticCategoriesCoverBoundedTaxonomy' -count=1
go test ./internal/designworkflow -run 'TestAutonomousCorrectionStressFixtureRecoversRealRoutingFailure|TestAutonomousCorrectionDenseStressRecoversRealRoutingFailure' -count=1
```
