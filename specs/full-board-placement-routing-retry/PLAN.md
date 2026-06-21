# Full-Board Placement-Routing Retry Evidence Implementation Plan

## Objective

Add realistic full-board retry fixtures proving that bounded placement-routing
retry can improve generated board routing or stop safely while preserving hard
constraints.

## Implementation Rules

- Keep default tests independent of KiCad CLI.
- Use real `designworkflow.Create` and `design create` paths wherever possible.
- Avoid brittle full JSON snapshots; assert selected evidence fields.
- Keep fixtures small and deterministic.
- Preserve retry default behavior: disabled unless requested.
- Commit phase-by-phase after Prism review.

## Phase 1: Full-Board Retry Fixture Harness

### Goal

Create reusable helpers for running full-board retry fixtures and extracting
normalized improvement evidence.

### Work

- Add a full-board retry fixture root under `internal/designworkflow/testdata`.
- Add fixture metadata helpers describing expected category, stop reason,
  improvement policy, and preserved constraints.
- Add helpers to run `designworkflow.Create` with deterministic options.
- Add helpers to extract:
  - initial routing status;
  - best routing status;
  - routed/failed net counts;
  - retry summary;
  - hint categories;
  - attempt history;
  - artifact/write status.
- Add hard-constraint comparison helpers for fixed refs, keepouts, edge
  constraints, local routes, and net assignments.

### Tests

- Fixture metadata loads and validates.
- Missing fixture reports the fixture path.
- Evidence extractor handles absent retry summary.
- Constraint comparison reports targeted diffs.

### Acceptance

- Later phases can add full-board fixtures without duplicating boilerplate.

### Commit

```text
Add full-board retry fixture harness
```

## Phase 2: Identify Routable Seed Fixtures

### Goal

Find or construct small generated boards that can route far enough for retry to
produce meaningful evidence.

### Work

- Audit existing built-in blocks and examples for usable footprint pad
  summaries.
- Add small fixture requests using existing verified block/component data.
- Run each candidate through placement and routing without retry.
- Record why rejected candidates fail:
  - missing pad summaries;
  - unsupported component evidence;
  - no route pressure;
  - too brittle geometry;
  - project write blocked too early.
- Keep only deterministic candidates.

### Tests

- Candidate fixture requests strict-decode and validate.
- Candidate baseline runs are deterministic across repeated test runs.
- Rejected candidates have documented reasons in fixture metadata or test logs.

### Acceptance

- At least two viable full-board fixtures are available for retry categories.

### Commit

```text
Add full-board retry candidate fixtures
```

## Phase 3: Spacing Improvement Fixture

### Goal

Add the first full-board fixture proving retry can improve routing evidence.

### Work

- Shape a generated board where initial routing produces a route-search or
  clearance/congestion diagnostic.
- Enable `routing_retry` with `increase_spacing`.
- Assert retry applies exactly the expected safe adjustment class.
- Assert best attempt improves by one or more of:
  - higher routing status rank;
  - fewer failed nets;
  - more routed nets.
- Assert fixed refs, keepouts, edge constraints, local routes, and net
  assignments survive retry.

### Tests

- Initial attempt is worse than best attempt.
- Retry summary includes `increase_spacing`.
- Applied count is at least one.
- Best attempt is returned, not the last attempt if a later attempt regresses.

### Acceptance

- The project has one deterministic full-board improvement fixture.

### Commit

```text
Add spacing improvement full-board retry fixture
```

## Phase 4: Distance Or Fanout Fixture

### Goal

Add a second full-board fixture covering another supported retry category.

### Work

- Prefer `reduce_distance` if route length/HPWL pressure can be produced
  deterministically.
- Use `improve_fanout` if pad-access/fanout pressure is easier to trigger with
  existing blocks.
- Assert category-specific evidence:
  - `reduce_distance`: deterministic proximity rules and no duplicate rules;
  - `improve_fanout`: fanout evidence tied to affected refs.
- Assert hard constraints remain unchanged.

### Tests

- Retry summary contains the expected category.
- Category-specific adjustment evidence is present.
- Constraint preservation holds after retry.

### Acceptance

- The full-board corpus covers at least two supported retry categories.

### Commit

```text
Add second full-board retry category fixture
```

## Phase 5: Safe Stop Full-Board Fixture

### Goal

Prove full-board retry stops safely when it cannot improve routing.

### Work

- Add a fixture where eligible retry applies an adjustment but routing rank does
  not improve.
- Enable `stop_on_non_improvement` where appropriate.
- Assert stop reason and attempt history.
- Assert best-so-far routing result is preserved.
- Assert hard constraints survive the attempted retry.

### Tests

- Stop reason is `non_improving_retry`, `max_attempts`, or a documented safe
  stop reason.
- Attempt history includes the attempted retry.
- Best evidence is not replaced by a worse attempt.

### Acceptance

- Full-board retry convergence boundaries are covered.

### Commit

```text
Add safe stop full-board retry fixture
```

## Phase 6: CLI Full-Board Retry Evidence

### Goal

Expose the full-board retry evidence contract through `design create --json`.

### Work

- Add selected-field CLI test for the improving fixture.
- Normalize output paths.
- Assert:
  - routing stage status;
  - retry summary;
  - improvement fields;
  - issues or artifacts as appropriate;
  - output path.
- Avoid full JSON snapshots.

### Tests

- CLI improving fixture returns retry summary and improvement evidence.
- CLI safe-stop fixture reports the expected stop reason or blocked issues.

### Acceptance

- AI callers can rely on stable CLI retry evidence fields.

### Commit

```text
Add full-board retry CLI evidence
```

## Phase 7: Documentation And Roadmap Update

### Goal

Document what full-board retry evidence now proves and what remains limited.

### Work

- Update README retry section with the full-board fixture examples.
- Update roadmap current state and near-term sequence.
- Document fixture categories and interpretation of improvement evidence.
- Note that retry still does not imply fabrication readiness.

### Tests

- `go test ./...`

### Acceptance

- README and roadmap distinguish focused retry goldens from full-board evidence.

### Commit

```text
Document full-board retry evidence
```

## Final Verification

Run:

```sh
go test ./...
```

Run Prism on staged changes for each phase before committing.
