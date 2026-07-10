# KiCad-Backed Schematic Anchor Calibration Plan

## Phase 1: Anchor Contract And Test Harness

Files: `internal/kicadfiles/schematic`, `internal/schematicir`,
`internal/kicadfiles/roundtrip`, `specs/schematic-anchor-calibration`.

- Add constrained IR mirror intent and propagate it through layout, writer,
  and symbol serialization.
- Centralize retrieval of schematic-space connection anchors.
- Add unit tests for all four right-angle transforms, both mirror axes, grid
  alignment, and agreement between layout and transaction pin data.
- Add an environment-gated KiCad test harness that reports symbol, pin, and
  rotation for failed direct connections.

Acceptance: the harness can express all planned orientation/mirror cases and
compare emitted endpoints to KiCad-parsed physical pins. Risk: shared
orientation semantics and serialized symbol representation.

## Phase 2a: Resistor Calibration

Files: `internal/kicadfiles/schematic/templates.go`, calibration fixtures and
tests.

- Calibrate `Device:R` against KiCad's parsed physical pins.
- Exercise each pin and all supported rotations/mirrors using direct-only
  nets.
- Preserve current label-based fixtures and run their KiCad ERC/round-trip
tests after each template change.

Acceptance: all resistor transform cases pass KiCad ERC and round-trip.
Status: completed by the `TestKiCadDirectResistorTransformMatrix` corpus.
Risk: existing direct passive routes can move.

## Phase 2b: Capacitor Calibration

Files: `internal/kicadfiles/schematic/templates.go`, calibration fixtures and
tests.

- Build a capacitor-specific direct-wire probe from KiCad-saved evidence.
- Do not reuse resistor offsets without a passing KiCad ERC matrix.
- Exercise each pin and all supported rotations/mirrors using direct-only
  nets.

Acceptance: all capacitor transform cases pass KiCad ERC and round-trip.
Status: pending; the initial isolated probe reports an unconnected pin 2.
Risk: capacitor pin endpoints may require a distinct instance/library rule.

## Phase 3: Connector Calibration

Files: connector templates, calibration fixtures and tests.

- Calibrate `Connector_Generic:Conn_01x02`, including its asymmetric second
  pin and any required explicit connection override.
- Prove direct wiring for both pins over all supported rotations/mirrors.

Acceptance: all connector cases pass with no new template mismatch warnings.
Risk: local-library connector fixtures may change anchors.

## Phase 4: Mixed-Pin Active Symbol Calibration

Files: LMV321 template, calibration fixtures and tests.

- Calibrate LMV321 input, output, and supply pins for all supported
  rotations/mirrors.
- Produce a small powered op-amp fixture with direct-only local wires and
  explicit KiCad ERC drivers.

Acceptance: all selected LMV321 pins and rotations pass ERC and round-trip.
Risk: active-symbol field/readability placement may need small deterministic
spacing adjustments.

## Phase 5: Shared Direct-Route Enforcement And Documentation

Files: adapter/transaction validation, docs, roadmap, tests.

- Ensure uncalibrated or conflicting anchors fail closed for explicit direct
  routes.
- Preserve automatic label fallback behavior.
- Maintain a checked-in calibrated-anchor inventory and fixture-output diff
  evidence for template migrations.
- Document calibrated built-ins and the resolver requirement for other
  symbols; update arbitrary-layout completion evidence.

Acceptance: full serial Go suite and all optional KiCad calibration tests pass.
Risk: callers with unsupported explicit direct routes receive a new actionable
failure instead of a potentially invalid schematic.

## Review Protocol

Each completed phase is staged, reviewed with `prism review staged`, tested
with focused and affected KiCad checks, then committed independently. The
KiCad corpus uses unique temporary projects and may run its isolated cases in
parallel; the full serial `go test -p 1 ./...` suite remains the final
shared-state regression check before the final phase is committed.
