# Schematic Layout Readability Implementation Plan

Date: 2026-06-29

This plan implements `specs/schematic-layout-readability/SPEC.md`.

Each phase should be implemented, reviewed with Prism, tested, and committed
before moving to the next phase.

## Phase 1: Layout Model And Readability Report

### Goal

Introduce a schematic layout package with stable input/output models and a
machine-readable readability report without changing generated schematics yet.

### Work

- Add `internal/schematiclayout`.
- Define request/result types for:
  - sheet size and margins;
  - components, roles, groups, pins, nets, and constraints;
  - placed components, wire segments, labels, junctions, diagnostics, and
    reports.
- Add readability profile constants:
  - `off`;
  - `basic`;
  - `standard`;
  - `strict`.
- Add deterministic normalization helpers:
  - stable component ordering;
  - stable net ordering;
  - stable group ordering;
  - bounded diagnostic ordering;
  - grid snapping helpers.
- Add report fields for:
  - component count;
  - group count;
  - routed net count;
  - label fallback count;
  - overlap counts;
  - diagonal-wire count;
  - stage-order violations;
  - power-placement violations;
  - pass/fail status.

### Tests

- Report normalization is deterministic.
- Grid snapping is stable.
- Empty layout requests produce a valid empty report.
- Profile parsing and defaults are deterministic.

### Acceptance

- New package compiles.
- No existing generation behavior changes.
- `go test ./internal/schematiclayout ./...` passes.

### Commit

```text
Add schematic layout readability model
```

## Phase 2: Geometry And Overlap Validator

### Goal

Add the first readability validator so generated concrete schematic geometry can
be checked for obvious human-readability failures.

### Work

- Add geometry primitives:
  - point;
  - rectangle;
  - line segment;
  - margin/page bounds;
  - text estimate box.
- Add approximate bounding boxes for common schematic objects:
  - symbol body;
  - reference/value text;
  - labels;
  - wire segments;
  - junctions.
- Add validator checks for:
  - symbol body overlaps;
  - text-to-symbol overlaps;
  - text-to-wire overlaps;
  - label-to-label overlaps;
  - wire through symbol body;
  - object outside usable sheet area;
  - diagonal wire segments.
- Add severity mapping for `info`, `warning`, and `error`.
- Keep diagnostics stable and bounded.

### Tests

- Overlapping symbols produce an error.
- Text over a symbol produces a warning.
- Wire through a symbol body produces an error.
- Diagonal wire produces an error or strict-profile failure.
- Objects outside the sheet produce an error.
- Non-overlapping spaced objects pass.

### Acceptance

- The validator can be used independently of the writer.
- Diagnostics are deterministic and include object references where available.

### Commit

```text
Validate schematic layout geometry
```

## Phase 3: Stage And Role Classification

### Goal

Infer schematic drawing order from existing component and net metadata.

### Work

- Add component role classification for common roles:
  - input connector;
  - output connector;
  - power connector;
  - regulator;
  - protection;
  - MCU/controller;
  - sensor/peripheral;
  - op-amp;
  - transistor/output stage;
  - passive support;
  - feedback;
  - bias/reference;
  - decoupling;
  - load.
- Add net role classification for:
  - input signal;
  - output signal;
  - power;
  - ground/return;
  - negative rail;
  - bus;
  - feedback;
  - high-current/load.
- Add deterministic stage assignment:
  - boundary input;
  - conditioning/protection;
  - processing/gain/control;
  - driver/output;
  - boundary output.
- Add vertical lane assignment:
  - positive rails/top;
  - signal center;
  - references/bias lower center;
  - ground/return bottom;
  - negative rails lower/bottom.
- Preserve explicit caller constraints where present.

### Tests

- Input connectors sort left of processing components.
- Output connectors sort right of drivers/output passives.
- Positive rails classify above signal lane.
- Ground and negative rails classify below signal lane.
- Feedback and bias roles classify consistently.
- Missing roles fall back to deterministic reference ordering.

### Acceptance

- Stage and lane assignment can be reported without applying coordinates.
- Classifier handles existing block and design API metadata.

### Commit

```text
Classify schematic roles for readable layout
```

## Phase 4: Deterministic Placement Pass

### Goal

Place schematic components using stage, role, group, and lane rules.

### Work

- Implement a conservative first-pass placer:
  - arrange stages left to right;
  - arrange groups within stages;
  - reserve gutters between groups;
  - place power/ground/reference objects in vertical lanes;
  - keep support components near their anchor component;
  - preserve fixed coordinates where explicitly requested.
- Add role-specific placement rules:
  - decoupling near active supply pins;
  - feedback above/around op-amps;
  - bias/reference lower or beside the affected node;
  - connectors at visual boundaries;
  - loads/output protection to the right.
- Add page overflow diagnostics rather than silently cramming objects.
- Add deterministic tie-breaking.

### Tests

- A simple LED chain places input/power, resistor, LED, and ground in readable
  lanes.
- An op-amp gain stage places input left, op-amp center, output right,
  feedback above, and ground/reference lower.
- A sensor/MCU-style fixture keeps bus pullups near the bus and connector
  boundaries clear.
- Fixed coordinates are preserved and reported.
- Page overflow produces a diagnostic.

### Acceptance

- The layout result contains concrete component positions that pass basic
  spacing checks in representative unit fixtures.

### Commit

```text
Place schematic components by readability rules
```

## Phase 5: Orthogonal Routing And Label Fallback

### Goal

Generate readable schematic connections after placement.

### Work

- Add orthogonal routing for schematic nets:
  - direct aligned segment;
  - deterministic elbow path;
  - horizontal lane with vertical pin drops;
  - body-avoidance fallback where possible.
- Emit KiCad-compatible two-point wire segments.
- Add label fallback for:
  - long nets;
  - multi-endpoint nets;
  - crowded routes;
  - power/ground/bus nets;
  - routes crossing functional group gutters.
- Add junction generation for T-connections.
- Add route diagnostics for unresolved or cluttered nets.

### Tests

- Misaligned pins route through orthogonal segments only.
- Long net uses labels rather than one cluttered wire path.
- Multi-endpoint power or bus net uses bounded labels.
- Wire routes avoid simple symbol body obstacles.
- Junctions are emitted for branch routes and omitted for pure labels.

### Acceptance

- Representative routed fixtures have no diagonal schematic wires.
- Long/crowded connections are readable through labels.

### Commit

```text
Route schematics with labels and orthogonal wires
```

## Phase 6: Design API And Workflow Integration

### Goal

Apply the readability layout pass to generated schematic projects.

### Work

- Convert existing design API schematic content into layout requests.
- Apply layout results before `.kicad_sch` writing for generated projects.
- Add layout profile configuration to generated design workflow options.
- Default new generated design workflow output to `standard` once tests prove
  compatibility.
- Add readability report fields to `design create` schematic stage summary:
  - profile;
  - pass/fail status;
  - overlap counts;
  - diagonal-wire count;
  - label fallback count;
  - stage/power violations.
- Keep `off` available for compatibility tests and debugging.

### Tests

- `design create` emits schematic readability summary fields.
- Existing design API tests pass with the layout pass enabled.
- `off` profile preserves old placement behavior where required.
- Standard profile produces no diagonal wires in representative generated
  projects.

### Acceptance

- Generated projects use the layout pass by default.
- AI-facing CLI output includes enough evidence to explain schematic
  readability.

### Commit

```text
Apply schematic readability layout in design workflow
```

## Phase 7: Example And Amplifier Fixture Coverage

### Goal

Use the new rules to improve real examples and lock in amplifier readability.

### Work

- Add or update tests for existing examples:
  - LED indicator;
  - button pull-up;
  - RC filter;
  - 555 timer;
  - sensor node;
  - Class AB headphone amplifier;
  - Class A headphone amplifier;
  - op-amp buffer headphone amplifier.
- Regenerate or rewrite generated examples through the layout engine where
  practical.
- Add strict readability checks for at least one amplifier schematic.
- Add standard readability checks for simple examples.
- Keep hand-authored fixture rewrites scoped and format-safe.

### Tests

- Amplifier fixture passes strict checks for:
  - no symbol body overlap;
  - no diagonal wires;
  - input-to-output left-to-right ordering;
  - rails and ground in expected vertical lanes;
  - feedback above/around active gain stage.
- Simple examples pass standard checks.
- Existing semantic landmark tests continue to pass.

### Acceptance

- The Class AB headphone amplifier example no longer presents as crowded or
  visually misleading after regeneration/rewrite.
- Readability tests prevent regression to the screenshot-style layout.

### Commit

```text
Refresh schematic examples with readability checks
```

## Phase 8: Documentation And Roadmap Update

### Goal

Document schematic readability behavior and update project status.

### Work

- Update `README.md` or focused docs with:
  - schematic layout profiles;
  - readability report fields;
  - current limitations;
  - how generated schematics are laid out.
- Update `specs/ROADMAP.md`:
  - mark schematic readability foundation implemented;
  - call out remaining limitations such as hierarchy/page splitting and
    imported schematic mutation.
- Add developer notes for future layout rules.

### Tests

- Documentation command examples use the compiled `kicadai` binary.
- `go test ./...` still passes.

### Acceptance

- Users can understand what readability guarantees exist and what remains
  manual or future work.

### Commit

```text
Document schematic readability layout support
```
