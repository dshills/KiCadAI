# Generated Full-Board Retry Fixtures Implementation Plan

## Objective

Add deterministic generated full-board retry fixtures that prove `design create`
can either improve real routing/connectivity/validation evidence after pad
hydration or stop safely with precise blockers.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests independent of KiCad CLI, network access, and global KiCad
  library roots.
- Prefer fixture metadata and selected-field assertions over full JSON
  snapshots.
- Reuse existing workflow fixture helpers before adding new abstractions.
- Do not loosen hard placement, board, routing, or validation constraints to
  make retry appear successful.
- Do not mark fabrication readiness as complete in this project.

## Phase 1: Fixture Evidence Harness

### Goal

Create reusable helpers for loading generated full-board retry fixtures and
extracting stable before/after evidence from workflow results.

### Work

- Add or extend fixture metadata structs for
  `internal/designworkflow/testdata/full_board_retry`.
- Define expected fields for:
  - fixture name;
  - expected retry enabled state;
  - expected stop reason;
  - expected hint categories;
  - expected improvement metric;
  - hard constraints or fixed refs to preserve;
  - expected pad hydration presence.
- Add helper functions that run `design create` fixtures in-process and extract:
  - routing retry summary;
  - attempt history;
  - baseline routing status;
  - final routing status;
  - routed and unrouted net counts where available;
  - validation blocking issue counts where available;
  - pad hydration totals;
  - fixed-ref or constraint preservation evidence.
- Normalize paths and omit volatile fields.

### Tests

- Metadata loads for all existing full-board retry fixtures.
- Missing required metadata fields fail clearly.
- Evidence extraction handles fixtures with retry enabled and disabled.
- Existing full-board retry tests continue passing.

### Acceptance

- Later phases can add fixtures without duplicating workflow execution and
  evidence extraction logic.

### Commit

```text
Add generated retry evidence harness
```

## Phase 2: Generated Improvement Fixture

### Goal

Add at least one generated `design create` fixture where retry measurably
improves evidence.

### Work

- Add or rework a fixture under
  `internal/designworkflow/testdata/full_board_retry`.
- Use a realistic generated request with `routing_retry` enabled.
- Choose a deterministic improvement metric, such as:
  - lower unrouted net count;
  - higher routed net count;
  - lower routing failure count;
  - lower blocking validation issue count;
  - improved route completion ratio.
- Assert baseline and final values using the harness.
- Assert retry applied at least one supported adjustment.
- Assert selected best attempt is not merely accepted because retry ran.

### Tests

- Improvement metric is declared in metadata.
- Baseline metric is worse than final metric.
- Attempt history shows the transition.
- Hard constraints remain valid.

### Acceptance

- The corpus contains a true generated-board retry improvement case.

### Commit

```text
Add generated retry improvement fixture
```

## Phase 3: Safe Stop And Unsupported Boundary Fixtures

### Goal

Prove generated workflows stop safely when retry cannot or should not improve
placement.

### Work

- Add or strengthen a generated safe-stop fixture with an expected stop reason.
- Add or strengthen an unsupported boundary fixture for rule-only,
  unsupported-zone, missing-evidence, or no-eligible-hint behavior.
- Assert retry does not overstate success.
- Assert placement remains unchanged when retry has no safe action.
- Assert blockers remain visible in issues or stage summaries.

### Tests

- Safe-stop fixture returns expected stop reason.
- Unsupported boundary fixture has zero applied adjustments or an equivalent
  skipped signal.
- Best-attempt selection remains deterministic.
- No fixture hides blocking issues.

### Acceptance

- The corpus proves bounded failure behavior for generated boards.

### Commit

```text
Add generated retry safe-stop fixtures
```

## Phase 4: Constraint Preservation And Multi-Block Coverage

### Goal

Prove retry preserves hard constraints and works against a generated board with
multiple blocks and inter-block nets.

### Work

- Add a generated fixture with fixed refs, board regions, edge constraints, or
  keepouts.
- Add a generated multi-block fixture with real inter-block nets.
- Assert fixed refs and hard constraints remain unchanged after retry.
- Assert pad hydration evidence is present before interpreting retry evidence.
- Assert schematic-to-PCB net intent survives through placement, routing, and
  validation summaries.

### Tests

- Fixed refs retain expected placement.
- Hard constraints are not loosened or removed.
- Multi-block fixture exposes hydrated pad counts and net deltas.
- Existing generated workflow tests keep passing.

### Acceptance

- Generated retry evidence covers more than a single-block demo and proves
  safety around hard placement constraints.

### Commit

```text
Add generated retry constraint fixtures
```

## Phase 5: CLI Selected-Field Coverage

### Goal

Lock down the AI-facing JSON contract for generated full-board retry evidence.

### Work

- Add a CLI test for at least one generated full-board retry fixture.
- Assert selected stable JSON fields:
  - routing retry enabled;
  - attempts;
  - applied;
  - stop reason;
  - hint categories;
  - attempt history shape;
  - pad hydration evidence;
  - final stage status or validation delta.
- Normalize output paths.
- Avoid full output snapshots that would be noisy or brittle.

### Tests

- `go test ./cmd/kicadai ./internal/designworkflow`
- CLI output remains stable across temp directories.

### Acceptance

- AI callers have tested, stable fields for interpreting generated retry
  behavior.

### Commit

```text
Add generated retry CLI goldens
```

## Phase 6: Documentation And Roadmap

### Goal

Mark the generated full-board retry milestone complete and move the roadmap to
fabrication export/readiness gates.

### Work

- Update README generated retry documentation with:
  - fixture classes;
  - improvement and safe-stop interpretation;
  - caveat that retry is not fabrication readiness.
- Update `specs/ROADMAP.md`:
  - move generated full-board retry fixtures from near-term next item into
    implemented foundation;
  - set fabrication export/readiness gates as the next priority;
  - retain transaction provenance and catalog/block expansion as follow-up
    work.
- Add implementation notes to this plan if any expected fixture class had to be
  narrowed.

### Tests

- `go test ./...`

### Acceptance

- Documentation accurately describes the implemented corpus and the next
  project.

### Commit

```text
Document generated retry fixture milestone
```

## Implementation Notes

- Phase 2 proves measurable before/after retry improvement through the
  existing pad-backed `spacing_improves` full-board fixture. The deterministic
  improvement is routing status rank (`blocked` to `partial`), not full routed
  completion.
- True generated `design create` movement improvement remains blocked by
  current block-local placement semantics: realized block components with local
  routes are emitted as fixed placements. Generated fixtures now document this
  boundary rather than pretending retry can safely move those components.
- Phase 3 locks down generated LED hydrated-pad routing/connectivity evidence
  and no-eligible-hint retry behavior.
- Phase 4 adds generated multi-block sensor/header boundary evidence. The
  workflow hydrates pads and carries inter-block net intent into placement, but
  that specific multi-block fixture blocks on fixed generated component
  geometry before routing can run. The generated LED boundary fixture remains
  the generated case that reaches routing/connectivity diagnostics.
- Phase 5 extends CLI selected-field coverage for generated retry evidence,
  including pad count, applied count, and absence of attempt history/categories
  when no retry hint is eligible.
- The next implementation project should be fabrication export/readiness gates.
  A later placement project should make generated block-local placement
  semantics movable under retry while preserving required local-route intent and
  hard constraints.
