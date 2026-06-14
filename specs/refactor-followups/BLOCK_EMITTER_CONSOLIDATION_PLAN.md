# Block Emitter Consolidation Plan

## Goal

Reduce duplication across circuit block emitters while preserving the exact
transactions emitted by existing verified blocks.

## Current Pain

Block implementations repeat parameter validation, component construction,
reference allocation, operation wrapping, issue handling, and output finalization.
That makes new verified blocks slower to add and makes it easier for one block to
miss validation behavior that another block already solved.

## Target Abstractions

- Typed parameter validators for voltage, resistance, capacitance, current,
  enum, symbol ID, and footprint ID values.
- Shared component emitter for common two-pin, connector, power symbol,
  decoupling capacitor, pull-up/down, and header patterns.
- Shared operation wrapping and issue path helpers.
- Shared finalization that constructs `BlockOutput`, resolves port voltages, and
  attaches issues consistently.

## Phase 1 - Snapshot Current Outputs

1. Add or confirm golden transaction tests for 100% of current built-in blocks.
   The initial inventory includes LED, regulator, MCU minimal, USB-C power, I2C
   sensor, op-amp gain stage, connector breakout, and any additional blocks
   present when the refactor starts.
2. Include important parameter variants where behavior branches.

Acceptance:

- Every refactor phase can prove emitted transactions did not change.
- Snapshot comparison normalizes volatile values such as UUIDs, timestamps, and
  transient IDs before comparing transaction content.
- Snapshot generation uses canonical JSON with stable map-key ordering and a
  defined floating-point representation, such as RFC 8785 JSON
  Canonicalization Scheme output.
- Current raw issue path strings are captured so centralized issue helpers can
  prove string equality.

## Phase 2 - Parameter Validators

1. Introduce typed validator helpers in a small package-local file.
2. Define the shared issue construction interface with stable path/message
   behavior before migrating individual validators.
3. Migrate one block at a time from ad hoc validation to shared validators.
4. Preserve issue paths and messages unless intentionally improved in a separate
   commit.

Acceptance:

- Golden outputs and validation tests pass after each migration.

## Phase 3 - Component Emitters

1. Extract helpers for common component shapes:
   - two-pin passive;
   - diode/LED;
   - capacitor;
   - connector/header;
   - power symbol;
   - ground symbol.
2. Keep helpers data-oriented; avoid hiding net semantics inside generic code.

Acceptance:

- Each migrated block has fewer operation-construction lines without changing
  transaction JSON.

## Phase 4 - Finalization And Issue Handling

1. Extract shared `BlockOutput` finalization.
2. Centralize operation wrapping with stable path generation.
3. Centralize blocking/non-blocking issue propagation.

Acceptance:

- All blocks report issue paths consistently.
- Issue path strings match the Phase 1 snapshots unless a separate behavior
  change explicitly updates those snapshots.

## Phase 5 - Block-Specific Cleanup

1. Remove duplicate helpers now covered by shared emitters.
2. Keep block-specific electrical decisions in each block file.
3. Document remaining intentional duplication where readability wins.

Acceptance:

- New block skeletons require only parameters, components, nets, and
  block-specific checks.

## Non-Goals

- No block behavior changes.
- No automatic electrical design synthesis.
- No resolver metadata expansion in this refactor.
