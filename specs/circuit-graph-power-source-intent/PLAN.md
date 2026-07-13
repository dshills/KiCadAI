# Circuit Graph Power-Source Intent Implementation Plan

## Phase 1: Contract And Plan

### Files

- `specs/circuit-graph-power-source-intent/SPEC.md`
- `specs/circuit-graph-power-source-intent/PLAN.md`

### Work

- Define explicit `power_flags` semantics, validation, lowering, and non-PCB
  behavior.
- Record security, repair, determinism, and regression constraints.

### Tests And Acceptance

- Documentation review confirms no implicit source inference or virtual
  footprint design.
- Prism reports no unresolved high-severity finding.

### Rollback Risk

Low. Documentation only.

## Phase 2: Strict Model, Schema, And Validation

### Likely Files

- `internal/circuitgraph/model.go`
- `internal/circuitgraph/schema.go`
- `internal/circuitgraph/parse.go`
- `internal/circuitgraph/validate.go`
- focused circuit-graph tests

### Work

- Add `PowerFlag` and `Document.PowerFlags`.
- Extend strict provider schema.
- Normalize declarations deterministically.
- Reject empty, unknown, duplicate, and non-power-net declarations.
- After trusted pin resolution, reject flags on nets that already contain an
  internal `power_out` driver.

### Tests And Acceptance

- Strict parser and schema tests cover valid and invalid declarations.
- Parse-normalize-parse is deterministic.
- Existing circuit graphs remain source-compatible.

### Rollback Risk

Medium. The provider-facing schema changes, but the new field is optional.

## Phase 3: Schematic-Only Lowering

### Likely Files

- `internal/circuitgraph/schematic.go`
- `internal/circuitgraph/schematic_test.go`
- `internal/circuitgraph/design_request_test.go`

### Work

- Generate deterministic collision-free PWR_FLAG component IDs and references.
- Attach flag pin `1` to the declared schematic net.
- Preserve the embedded KiCad PWR_FLAG pin's `power_out` electrical type.
- Keep synthetic flags out of resolved and explicit PCB component collections.
- Verify deterministic schematic transactions and read/write behavior.

### Tests And Acceptance

- Schematic IR contains one flag per declaration on the correct net.
- Explicit PCB component and endpoint counts are unchanged.
- Schematic transaction validation passes repeatedly with identical output.

### Rollback Risk

Medium. Incorrect lowering could create false ERC confidence or unreadable
schematics; tests must assert exact net attachment and PCB exclusion.

## Phase 4: Generic BMP280 Evidence

### Likely Files

- `examples/ai/generic_usb_c_bmp280_breakout/recorded-response.json`
- focused promotion tests and metadata when the gate advances

### Work

- Declare external drive for the USB source and return nets.
- Run the recorded workflow and direct KiCad ERC.
- Confirm power-driver findings disappear without suppressions.
- Re-run existing generic and bounded regressions.

### Tests And Acceptance

- Target ERC has no power-driver findings.
- Planning, routing, writer correctness, and round-trip evidence do not regress.
- Prism reports no unresolved high-severity finding.

### Rollback Risk

Low after shared lowering tests. The fixture declaration can be reverted without
affecting the optional contract.
