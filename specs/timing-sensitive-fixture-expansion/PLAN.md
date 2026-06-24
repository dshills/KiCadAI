# Timing-Sensitive Fixture Expansion Implementation Plan

## Overview

This plan expands timing-sensitive fixture support beyond the current crystal
oscillator path. The first implementation target is a canned oscillator fixture
because it exercises clock source proximity, decoupling proximity, clock-output
route length, ground return, and optional enable-control evidence without
requiring high-speed SI simulation.

Each implementation phase should be reviewed with Prism and committed before
moving to the next phase.

## Phase 1: Model Gap Review and Finding Constants

### Goal

Prepare the timing evidence model for oscillator-style fixtures without
breaking existing crystal support.

### Work

- Review `blocks.PCBTimingFixture` and `blocks.TimingFixtureEvidence`.
- Add new timing finding constants for decoupling, enable/control, and
  reset/programming evidence.
- Add optional timing fixture fields only if the current role map cannot express
  oscillator needs clearly.
- Extend clone and validation tests if model fields are added.
- Keep existing crystal oscillator metadata valid.

### Tests

- `go test ./internal/blocks`

### Acceptance

- New finding IDs are available to workflow issue mapping.
- Existing crystal fixture tests pass unchanged or with only intentional
  expected-value updates.

### Commit

`Add oscillator timing finding model`

## Phase 2: Canned Oscillator Block Definition

### Goal

Add a structurally verified canned oscillator block.

### Work

- Add a built-in oscillator block definition or extend the timing block family
  with a distinct `canned_oscillator` block ID.
- Define ports for clock output, supply, ground, and optional enable.
- Define components for oscillator, decoupling capacitor, and optional enable
  pull component.
- Add required library metadata for symbols and footprints.
- Add component queries where catalog-backed selection is possible.
- Add block inventory and registry coverage.

### Tests

- Block registry test for `canned_oscillator`.
- Instantiate test for refs, nets, operations, and unsupported parameter
  rejection.

### Acceptance

- `canned_oscillator` appears in built-in inventory.
- Instantiation produces deterministic schematic operations.
- Unsupported or incomplete parameter choices produce blocking issues.

### Commit

`Add canned oscillator circuit block`

## Phase 3: Oscillator PCB Realization and Evidence

### Goal

Generate PCB placement, local routes, and timing evidence for the canned
oscillator block.

### Work

- Add PCB realization metadata for oscillator, decoupling capacitor, and enable
  pull component where present.
- Add placement groups and proximity constraints.
- Add local routes for clock output, decoupling, ground return, and enable where
  supported.
- Add timing fixture metadata for oscillator evidence.
- Extend evidence generation to measure decoupling distance and enable/control
  presence.
- Add threshold checks for decoupling proximity and clock-output route length.

### Tests

- Realization test for component placements and local routes.
- Timing evidence test for satisfied oscillator fixture.
- Negative test for missing decoupling evidence.
- Negative test for excessive clock-output route length.

### Acceptance

- Realized oscillator fixtures include timing evidence with no findings in the
  known-good case.
- Known-bad fixtures emit stable findings with refs, nets, measurements, and
  thresholds.

### Commit

`Realize canned oscillator timing fixtures`

## Phase 4: Workflow and Placement Integration

### Goal

Expose oscillator timing evidence in generated design workflows and placement
scoring.

### Work

- Ensure oscillator placement groups produce timing-relevant proximity rules.
- Confirm timing-sensitive candidate scoring emits dimensions for oscillator
  source, consumer, decoupling, and enable proximity where applicable.
- Extend workflow timing issue suggestions for new finding IDs.
- Add design workflow tests for oscillator timing results in stage summaries.
- Add repair suggestion expectations for oscillator-specific failures.

### Tests

- `go test ./internal/placement ./internal/designworkflow ./internal/blocks`

### Acceptance

- Design workflow summaries include oscillator timing results.
- Timing findings are surfaced as relative PCB realization stage issues.
- Placement scoring includes timing-sensitive dimensions for oscillator
  proximity rules.

### Commit

`Expose oscillator timing evidence in workflows`

## Phase 5: Golden and Negative Fixture Coverage

### Goal

Protect oscillator fixture behavior with deterministic regression coverage.

### Work

- Add a good oscillator fixture snapshot or golden test.
- Add a negative oscillator fixture generated in test code for missing
  decoupling or long clock route.
- Include stable JSON evidence where existing golden conventions support it.
- Avoid adding large generated KiCad artifacts unless they match repository
  conventions.

### Tests

- Golden/snapshot tests for oscillator timing evidence.
- `go test ./...`

### Acceptance

- Golden coverage fails on accidental timing evidence regressions.
- Negative fixture tests fail when bad oscillator evidence is not reported.

### Commit

`Add oscillator timing fixture goldens`

## Phase 6: Reset/Programming Timing Evidence Follow-Up

### Goal

Extend the same timing evidence pattern to reset/programming fixtures if the
oscillator path is stable.

### Work

- Review existing reset/programming block metadata.
- Add timing fixture metadata for reset path, programming clock path, pull
  proximity, and programming header ground reference.
- Add route-length and proximity evidence where the current model can prove it.
- Add workflow suggestions for reset/programming findings.

### Tests

- Reset/programming realization timing evidence tests.
- Negative route-length or missing-ground test.

### Acceptance

- Existing reset/programming block emits timing evidence where relevant.
- Bad reset/programming fixture cases produce structured findings.

### Commit

`Add reset programming timing evidence`

## Phase 7: Documentation and Roadmap Update

### Goal

Keep user-facing documentation aligned with expanded timing fixture support.

### Work

- Update README with canned oscillator fixture support.
- Update `specs/ROADMAP.md` to mark oscillator fixture expansion complete or
  partially complete.
- Document remaining limitations around SI, jitter, startup, and datasheet
  constraints.

### Tests

- `go test ./...`

### Acceptance

- README and ROADMAP accurately describe implemented oscillator timing support.
- Full test suite passes.

### Commit

`Document oscillator timing fixture expansion`
