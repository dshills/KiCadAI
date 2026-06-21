# Placement Routing Retry Golden Fixtures Implementation Plan

## Objective

Add deterministic full-board and workflow-level golden coverage for the
placement-routing retry loop so future AI-facing changes cannot silently break
retry safety, summary evidence, or best-attempt behavior.

## Implementation Rules

- Keep default tests independent of KiCad CLI.
- Prefer workflow-level invariant checks over brittle full JSON snapshots.
- Normalize output paths before comparing snapshots.
- Keep each fixture small enough for normal `go test ./...`.
- Commit each phase independently after Prism review.
- Preserve existing default behavior: retry remains disabled unless requested.

## Phase 1: Golden Fixture Harness

### Goal

Create reusable helpers for retry request fixtures, workflow execution, and
stable summary assertions.

### Work

- Add a test fixture directory for placement-routing retry cases.
- Add helper to load fixture request JSON.
- Add helper to run `designworkflow.Create` into a temp output directory.
- Add summary extraction helpers for placement and routing stages.
- Add path-normalization helper for future CLI snapshots.
- Add invariant assertion helpers for retry summary fields.

### Tests

- Missing fixture reports a clear test failure.
- Retry summary extraction handles missing summary gracefully.
- Path normalization strips temp directories.
- Disabled retry fixture keeps current single-attempt behavior.

### Acceptance

- Later phases can add fixtures without duplicating workflow boilerplate.

### Commit

```text
Add placement routing retry golden harness
```

## Phase 2: Increase Spacing And Fanout Fixtures

### Goal

Cover spacing and fanout retry evidence through deterministic workflow fixtures.

### Work

- Add `increase_spacing` fixture.
- Add `improve_fanout` fixture.
- Assert hint categories and retry summary fields.
- Assert fixed components remain unchanged.
- Assert hard keepouts and edge constraints remain represented.
- If full geometry cannot reliably trigger the current router, use a focused
  workflow helper harness with synthetic routing diagnostics and real placement
  adjustment output.

### Tests

- `increase_spacing` produces `increase_spacing` hint evidence and an applied
  spacing adjustment.
- `improve_fanout` produces fanout evidence and retry category.
- Fixed refs do not move.
- Retry summary attempt count and applied count are deterministic.

### Acceptance

- Congestion/fanout retry categories are covered by golden tests.

### Commit

```text
Add spacing and fanout retry goldens
```

## Phase 3: Reduce Distance Fixture

### Goal

Cover length/HPWL-driven retry behavior and generated proximity rules.

### Work

- Add `reduce_distance` fixture.
- Trigger or synthesize a length-policy routing diagnostic.
- Assert `reduce_distance` hint category.
- Assert deterministic proximity rule IDs.
- Assert complexity-based anchor selection.
- Assert reruns do not duplicate proximity rules.

### Tests

- Multi-endpoint net creates anchor-to-target proximity rules for every target.
- Existing retry proximity rules are not duplicated.
- Rule IDs remain stable.
- Best attempt either improves route ranking or reports non-improvement.

### Acceptance

- Distance-reduction retry behavior is locked down.

### Commit

```text
Add reduce distance retry golden
```

## Phase 4: Stop Conditions

### Goal

Prove retry terminates safely when it cannot make useful progress.

### Work

- Add `non_improving` fixture or harness case.
- Add `repeated_state` fixture or harness case.
- Add optional context-canceled case if practical.
- Assert stop reasons and attempt histories.
- Assert best-so-far routing result is returned after regression.

### Tests

- Non-improving retry stops with `non_improving_retry`.
- Repeated movable placement state stops with `repeated_placement_state`.
- Attempt history includes the final attempted retry.
- Later regressed attempts do not replace the best attempt.

### Acceptance

- The retry loop has tested convergence boundaries.

### Commit

```text
Add retry stop condition goldens
```

## Phase 5: Unsupported And Rule-Only Skip Fixtures

### Goal

Prove retry does not mutate placement for failures outside placement scope.

### Work

- Add unsupported zone policy fixture or harness case.
- Add routing-rule-only fixture or harness case.
- Add input-model/pad-access boundary case where appropriate.
- Assert unsupported/rule-only hints are ineligible.
- Assert no placement adjustment is applied.
- Assert placement state is unchanged.

### Tests

- `unsupported` hint category is not retried.
- `relax_rules` hint category is not retried unless explicitly allowed by
  future policy.
- Retry summary records zero applied adjustments.
- Workflow result remains blocked or warning according to routing evidence.

### Acceptance

- Unsafe retry categories are locked down by golden coverage.

### Commit

```text
Add unsupported retry skip goldens
```

## Phase 6: CLI Snapshot Coverage

### Goal

Cover `design create` request/response behavior at the CLI boundary.

### Work

- Add one or two stable CLI fixtures using `routing_retry`.
- Normalize output directory and artifact paths.
- Assert JSON contains retry summary fields.
- Assert `max_attempts` semantics are visible in output.
- Keep snapshots compact; prefer selected-field snapshots over entire workflow
  payloads.

### Tests

- CLI request with retry disabled has no retry attempt beyond initial routing.
- CLI request with retry enabled returns `routing_retry` summary.
- Snapshot normalization is stable across temp paths.

### Acceptance

- External AI/CLI contract for retry is covered.

### Commit

```text
Add CLI retry summary golden
```

## Phase 7: Documentation And Examples

### Goal

Document retry usage and expected interpretation for AI callers.

### Work

- Add or update example request JSON with `routing_retry`.
- Update README retry section if needed.
- Add note to roadmap after fixtures are implemented.
- Document supported categories and stop reasons.

### Tests

- Full `go test ./...`.
- Example request parses under strict request decoder if included in testdata.

### Acceptance

- Users can find a runnable retry request and understand retry output.

### Commit

```text
Document retry golden fixtures
```

## Final Verification

Run:

```sh
go test ./...
```

Run Prism on staged changes for each phase before committing.
