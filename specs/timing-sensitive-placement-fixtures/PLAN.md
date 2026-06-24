# Timing-Sensitive Placement Fixtures Implementation Plan

## Overview

This plan adds timing-sensitive PCB fixtures, starting with crystal and oscillator blocks. The work should be implemented in phases, reviewed with Prism after each phase, and committed before moving to the next phase when implementation begins.

## Phase 1: Inventory Existing Block and Placement Models

### Goal

Identify the smallest extension points needed for timing-sensitive fixture metadata without duplicating existing block, placement, routing, or validation systems.

### Work

- Review existing circuit block realization models.
- Review placement semantic and advanced-rule models.
- Review routing evidence and validation summary structures.
- Identify where timing roles should be represented.
- Document any existing model fields that can be reused.

### Tests

- No code tests required if this phase is documentation-only.
- If model inspection produces scaffolding, run `go test ./...`.

### Acceptance

- A short implementation note or code comments identify the selected extension points.
- No new parallel timing-only pipeline is introduced.

### Commit

`Document timing fixture integration points`

## Phase 2: Add Timing Fixture Metadata

### Goal

Represent timing-sensitive intent in reusable Go structures.

### Work

- Add timing role constants or typed identifiers.
- Add timing group metadata for source, consumer, capacitors, ground return, and routes.
- Add threshold fields for proximity, symmetry, route length, and noise keepout.
- Keep metadata optional so existing designs remain unchanged.
- Add JSON serialization support where workflow evidence already serializes design metadata.

### Tests

- Unit tests for timing metadata defaults.
- Serialization tests for timing metadata.
- Regression tests confirming existing block fixtures remain unchanged when timing metadata is absent.

### Acceptance

- Timing metadata can be attached to block-generated components and routes.
- Existing generated examples remain stable unless intentionally updated.

### Commit

`Add timing-sensitive fixture metadata`

## Phase 3: Implement Crystal Fixture Realization

### Goal

Generate a reusable MCU crystal block with placement, footprints, nets, and local routes.

### Work

- Add a crystal block definition.
- Assign symbols and footprints using existing resolver-backed mapping.
- Generate two crystal nets between MCU pins, crystal pins, and load capacitors.
- Generate a ground net for load capacitors.
- Place the crystal near the MCU clock pins.
- Place load capacitors symmetrically near the crystal.
- Add local routes or route intents for the crystal and load capacitors.
- Emit timing evidence for generated components and nets.

### Tests

- Unit tests for schematic net assignment.
- Unit tests for footprint assignment.
- PCB writer tests for generated footprint and net correctness.
- Golden fixture for a known-good crystal block.

### Acceptance

- Generated crystal project opens in KiCad.
- Clock nets terminate on expected pads.
- Load capacitors connect to the expected nets and ground.
- Timing evidence records proximity and symmetry measurements.

### Commit

`Add crystal timing fixture realization`

## Phase 4: Implement Oscillator Fixture Realization

### Goal

Generate a reusable canned oscillator block with power, ground, enable, decoupling, and clock output behavior.

### Work

- Add an oscillator block definition.
- Assign oscillator and decoupling capacitor footprints.
- Generate power, ground, enable, and clock output nets.
- Place decoupling capacitor near oscillator power pin.
- Place oscillator near the consuming clock input.
- Route or express route intents for local power, ground, and clock output.
- Emit timing evidence for oscillator output route length and local decoupling.

### Tests

- Unit tests for oscillator net generation.
- Unit tests for decoupling placement constraints.
- Golden fixture for a known-good oscillator block.

### Acceptance

- Generated oscillator project opens in KiCad.
- Oscillator output connects to the consuming clock input.
- Decoupling evidence appears in validation output.

### Commit

`Add oscillator timing fixture realization`

## Phase 5: Add Timing-Aware Placement Scoring

### Goal

Teach placement evaluation to score timing-sensitive constraints and explain its decisions.

### Work

- Add timing proximity scoring for source-to-consumer placement.
- Add load capacitor symmetry scoring.
- Add local ground proximity scoring.
- Add noisy-region keepout checks using existing high-current, thermal, or semantic region metadata.
- Expose timing scoring in placement diagnostics.

### Tests

- Unit tests for pass/fail proximity thresholds.
- Unit tests for load capacitor asymmetry.
- Unit tests for noisy-region penalty behavior.
- Regression tests proving non-timing placement rules still behave as before.

### Acceptance

- Good timing layouts score better than intentionally poor layouts.
- Diagnostics identify measured values and thresholds.
- Placement engine remains deterministic.

### Commit

`Score timing-sensitive placement rules`

## Phase 6: Add Timing Validation Findings

### Goal

Fail known-bad timing fixtures with actionable evidence.

### Work

- Add timing validation finding IDs.
- Validate source-to-consumer distance.
- Validate load capacitor proximity and symmetry.
- Validate local ground evidence.
- Validate timing route length.
- Surface unsupported timing constraints explicitly.
- Include timing findings in machine-readable and human-readable reports.

### Tests

- Positive validation test for crystal fixture.
- Positive validation test for oscillator fixture.
- Negative validation tests for far crystal, asymmetric load capacitors, missing ground, and excessive route length.

### Acceptance

- Known-good timing fixtures pass.
- Known-bad timing fixtures fail with specific finding IDs.
- CLI output is concise and useful for AI feedback loops.

### Commit

`Validate timing-sensitive placement fixtures`

## Phase 7: Add Golden Fixtures and Round-Trip Coverage

### Goal

Protect timing fixture writer behavior with deterministic examples and round-trip tests.

### Work

- Add generated golden projects for crystal and oscillator fixtures.
- Add negative fixture artifacts where useful, or generate them in tests.
- Add round-trip preservation checks for timing metadata and KiCad files.
- Confirm generated projects survive parse/write/parse workflows.
- Add fixture update instructions if the repository has existing golden-update conventions.

### Tests

- Golden file tests.
- Round-trip tests.
- `go test ./...`.

### Acceptance

- Golden fixtures are deterministic.
- Round-trip tests preserve timing-relevant nodes and evidence.
- Existing golden corpus remains stable.

### Commit

`Add timing fixture golden coverage`

## Phase 8: Wire Timing Fixtures Into Design Workflow CLI

### Goal

Allow AI-oriented design creation to select timing fixtures and report timing quality.

### Work

- Add request parsing for MCU clock requirements where the design workflow supports block selection.
- Select crystal or oscillator block based on request metadata.
- Include timing fixture evidence in design workflow JSON.
- Include timing summary in CLI output.
- Add failure guidance when timing constraints are unsupported or violated.

### Tests

- CLI workflow test for crystal-backed MCU design.
- CLI workflow test for oscillator-backed MCU design.
- JSON evidence snapshot tests.

### Acceptance

- `kicadai design create` can produce a project with a timing fixture from request data.
- CLI output reports timing validation status.
- AI consumers can inspect structured timing evidence.

### Commit

`Expose timing fixtures in design workflow`

## Phase 9: Documentation and Roadmap Update

### Goal

Keep user-facing documentation aligned with the implemented timing fixture support.

### Work

- Update `README.md` with timing-sensitive fixture support.
- Update `specs/ROADMAP.md` to mark this item complete or partially complete.
- Document known limitations around SI, oscillator start-up, parasitics, and DRC coverage.

### Tests

- Documentation-only phase; no tests required unless generated docs are validated by tooling.

### Acceptance

- README accurately describes available timing fixture behavior.
- Roadmap reflects remaining gaps.

### Commit

`Document timing-sensitive fixture support`
