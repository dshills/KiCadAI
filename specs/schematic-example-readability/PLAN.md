# Schematic Example Readability Implementation Plan

Date: 2026-06-29

This plan implements `specs/schematic-example-readability/SPEC.md`.

Each phase should be implemented, reviewed with Prism, tested, and committed
before moving to the next phase.

## Phase 1: Example Audit And Baseline Reports

### Goal

Create a deterministic audit of current checked-in schematic examples before
changing fixtures.

### Work

- Add an audit helper or test-only utility that reads example project
  directories.
- Parse each target `.kicad_sch` using the existing schematic reader.
- Extract counts for:
  - symbols;
  - wires;
  - labels;
  - junctions;
  - diagonal wires;
  - symbols without positions;
  - available pin anchors.
- Produce a compact audit report for the eight scoped examples.
- Save the report under `specs/schematic-example-readability/AUDIT.md`.

### Tests

- Audit helper finds all scoped examples.
- Audit output is deterministic.
- Missing example paths fail clearly.

### Acceptance

- Current readability risks are visible before fixture edits.

### Commit

```text
Audit schematic example readability
```

## Phase 2: Parsed Schematic To Layout Adapter

### Goal

Convert parsed `.kicad_sch` files into `schematiclayout` validation inputs.

### Work

- Add adapter code, likely in `internal/schematiclayout` or
  `internal/amplifiers`, that maps:
  - `schematic.SchematicFile.Symbols` to `PlacedComponent`;
  - `schematic.Wires` to `WireSegment`;
  - `schematic.Labels` to layout labels;
  - `schematic.Junctions` to layout junctions;
  - paper settings to sheet bounds.
- Infer roles from parsed symbols using existing layout classification.
- Preserve references, values, library IDs, positions, and pin anchors.
- Add deterministic default symbol/text boxes.

### Tests

- Adapter converts a small synthetic schematic.
- Adapter preserves references and positions.
- Adapter preserves wire segment endpoints.
- Adapter handles missing pin anchors without panics.

### Acceptance

- Parsed example schematics can be validated by `schematiclayout.Validate`.

### Commit

```text
Adapt parsed schematics for readability validation
```

## Phase 3: Standard Example Readability Tests

### Goal

Add standard readability gates for simple examples.

### Work

- Add tests for:
  - `01_led_indicator`;
  - `02_button_pullup`;
  - `03_rc_filter`;
  - `04_555_timer`;
  - `05_sensor_node`.
- Enforce:
  - no diagonal wires;
  - no symbol body overlaps;
  - no wire-through-symbol errors;
  - all objects inside usable sheet area.
- Keep warnings visible but non-blocking for the first pass.

### Tests

- Standard examples pass standard readability.
- Test failures include example name and diagnostics.

### Acceptance

- Simple examples cannot regress to diagonal/crowded basics.

### Commit

```text
Add standard schematic example readability tests
```

## Phase 4: Amplifier Strict Readability Rules

### Goal

Add amplifier-specific readability checks.

### Work

- Add strict amplifier checker for parsed schematics.
- Detect:
  - input labels/connectors;
  - op-amp or active gain stage;
  - feedback symbols/labels;
  - output stage transistors/buffers;
  - load/output connector;
  - positive and negative rails;
  - ground/return/load.
- Enforce:
  - input left of gain stage;
  - output/load right of gain/output stage;
  - feedback above or around active stage;
  - positive rail above signal lane;
  - ground/load/return lower than signal lane;
  - no diagonal wires;
  - no symbol body overlaps.

### Tests

- `06_class_ab_headphone_amp` strict readability test.
- `09_class_a_headphone_amp` strict readability test.
- `10_opamp_buffer_headphone_amp` strict readability test.
- Failure messages identify the violated convention.

### Acceptance

- Amplifier fixtures have objective readability gates.

### Commit

```text
Add strict amplifier schematic readability tests
```

## Phase 5: Fixture Improvement Or Regeneration

### Goal

Fix examples that fail the new readability gates.

### Work

- For each failing example:
  - prefer deterministic regeneration if a generator exists;
  - otherwise apply focused coordinate/wire edits;
  - preserve project filenames and semantic landmarks.
- Start with `06_class_ab_headphone_amp`.
- Keep changes scoped to `.kicad_sch` and directly related project files.
- Run existing parser, semantic, and readability tests after each fixture
  update.

### Tests

- Updated fixtures pass readability tests.
- Existing amplifier semantic landmark tests pass.
- Existing schematic/project read tests pass.

### Acceptance

- The Class AB headphone amplifier no longer has screenshot-style crowding or
  misleading diagonal output wiring.

### Commit

```text
Improve checked-in schematic example readability
```

## Phase 6: Workflow Regression Coverage

### Goal

Ensure newly generated schematics expose and satisfy baseline readability
evidence.

### Work

- Add/extend `design create` tests to assert readability summary shape.
- Add generated op-amp gain-stage or amplifier-seed readability checks where
  the workflow can progress far enough.
- Add checks that design API-generated wires remain orthogonal.
- Keep KiCad-backed tests optional.

### Tests

- Workflow schematic stage includes readability profile and diagnostic counts.
- Representative generated schematic has zero diagonal wires.
- Generated op-amp block preserves left-to-right ordering.

### Acceptance

- Readability remains covered for both checked-in examples and generated
  workflow output.

### Commit

```text
Cover generated schematic readability regressions
```

## Phase 7: Documentation And Roadmap Update

### Goal

Document example readability guarantees and remaining limitations.

### Work

- Update README with example readability test coverage.
- Update `docs/development.md` or a focused docs page with:
  - profiles;
  - what strict amplifier readability checks mean;
  - how to inspect failures.
- Update `specs/ROADMAP.md`:
  - mark example readability gates implemented;
  - preserve remaining limitations.

### Tests

- Documentation examples use `kicadai`, not `go run`.
- `go test ./...` passes.

### Acceptance

- Users can understand what readability is enforced and what remains future
  work.

### Commit

```text
Document schematic example readability gates
```
