# Placement Congestion And Routing Retry Implementation Plan

## Objective

Implement placement congestion/fanout scoring and bounded placement-routing
retry in small, reviewable phases while preserving deterministic placement,
routing, workflow output, and existing request compatibility.

## Implementation Rules

- Extend `internal/placement` quality reports before changing placement
  behavior.
- Keep routing diagnostics in `internal/routing`; map them in
  `internal/designworkflow`.
- Keep retry disabled by default until the workflow policy exists.
- Preserve fixed components, explicit positions, connector edge intent, required
  regions, and hard keepouts.
- Bound every retry by attempt count and convergence checks.
- Add tests with every behavior change.
- Run focused tests per phase.
- Run `prism review staged` before every phase commit.
- Run full `go test ./...` before the final docs commit.

## Phase 1: Congestion Report Model

### Goal

Expose coarse placement congestion without changing placement behavior.

### Tasks

1. Add congestion report structs to `internal/placement.QualityReport`.
2. Compute a deterministic coarse grid from board dimensions.
3. Scale grid resolution by board size and component density, cap it initially
   at 200 x 200 cells, and enforce a minimum physical cell size.
4. Estimate net crossings per cell using deterministic direct-line or fixed
   horizontal-then-vertical Manhattan path/cell intersection evidence.
   Bounding boxes may be used as a fallback but should be marked as
   conservative to avoid overclaiming precision.
5. Include weighted crossing count, estimated capacity, utilization, status,
   refs, nets, and suggested action.
6. Include congestion in placement score dimensions.
7. Add tests for:
   - empty board/pass case;
   - dense crossing warning/failure;
   - deterministic report ordering;
   - score impact.

### Acceptance Criteria

- Placement quality reports include congestion evidence.
- Existing placement output remains unchanged.
- Congestion reports are deterministic.

### Suggested Commit

`Add placement congestion reports`

## Phase 2: Fanout And Escape Readiness Reports

### Goal

Report components that are likely difficult to escape before routing runs.

### Tasks

1. Add fanout report structs to placement quality.
2. Estimate connected pad count and distinct local net count per component.
3. Estimate available escape sides from board edges, keepouts, and component
   bounds.
4. Flag high-demand components near edges, keepouts, or dense neighbors.
5. Add fanout score dimension and diagnostics.
6. Add tests for:
   - connector near edge with acceptable fanout;
   - MCU-like component with many nets and poor escape area;
   - keepout pressure around a component;
   - deterministic report ordering.

### Acceptance Criteria

- Placement quality identifies fanout pressure by component ref.
- Diagnostics suggest increasing spacing or moving the component.
- Existing placement behavior remains deterministic.

### Suggested Commit

`Add placement fanout reports`

## Phase 3: Routing Diagnostic To Placement Hint Mapping

### Goal

Convert routing diagnostics into explicit placement retry hints.

### Tasks

1. Add design workflow placement retry hint model:
   - category;
   - source diagnostic category/action;
   - affected nets;
   - affected refs;
   - severity;
   - suggested adjustment;
   - retry eligibility.
2. Map routing diagnostics:
   - no path/obstacle -> `increase_spacing`;
   - length failure -> `reduce_distance`;
   - pad/layer access near component -> `improve_fanout`;
   - routing rules -> `relax_rules`;
   - zone unsupported -> `unsupported`;
   - library/model issues -> `unsupported`.
3. Include placement congestion/fanout evidence when available.
4. Add tests for each mapping category.

### Acceptance Criteria

- Routing failures produce deterministic retry hints.
- Unsupported or rule-only failures do not trigger placement retry.
- Hints preserve refs/nets where available.

### Suggested Commit

`Map routing failures to placement retry hints`

## Phase 4: Retry Policy Model

### Goal

Represent bounded retry policy in workflow options and request normalization.

### Tasks

1. Add retry policy type with:
   - enabled;
   - max attempts;
   - min routing score delta;
   - allowed hint categories;
   - preserve fixed;
   - stop on new blockers;
   - stop on repeated diagnostic signature.
2. Add policy normalization defaults.
3. Add validation for negative attempts, invalid thresholds, and unknown hint
   categories.
4. Keep retry disabled by default.
5. Add request JSON support if it fits the existing request model.
6. Add tests for normalization and validation.

### Acceptance Criteria

- Retry policy is explicit and bounded.
- Existing requests behave exactly as before.
- Invalid retry policy blocks early with actionable issues.

### Suggested Commit

`Add placement routing retry policy`

## Phase 5: Placement Retry Adjustment Builder

### Goal

Translate retry hints into deterministic placement request adjustments.

### Tasks

1. Add adjustment model:
   - spacing multiplier or delta;
   - fanout clearance delta;
   - congestion avoidance hints;
   - net proximity emphasis;
   - region/group relaxation only when allowed.
2. Apply adjustments to placement rules and soft hints.
3. Preserve fixed components and hard constraints.
4. Ignore hints that would require moving fixed components, violating required
   regions, crossing board edges, or entering hard keepouts; report a fallback
   reason when no safe adjustment exists.
5. Bound adjustment magnitude per attempt.
6. Add tests for:
   - spacing increase;
   - fanout clearance increase;
   - reduce-distance proximity emphasis;
   - fixed component preservation;
   - deterministic adjustment ordering.

### Acceptance Criteria

- Retry adjustments are deterministic and explainable.
- Adjustments never mutate hard constraints unsafely.
- Adjusted placement requests remain valid.

### Suggested Commit

`Build placement retry adjustments`

## Phase 6: Bounded Workflow Retry Loop

### Goal

Run placement -> routing -> placement retry -> routing when policy allows it.

### Tasks

1. Add retry orchestration inside `designworkflow.Create` or a helper used by
   `Create`.
2. Preserve existing single-attempt flow when retry is disabled.
3. On routing blocked/weak result:
   - build retry hints;
   - check policy and eligibility;
   - build adjusted placement request/options;
   - rerun placement and routing.
4. Track best-so-far attempt by routing status, failed net count, routed net
   count, routing score, and placement score.
5. Track visited movable-placement state hashes and stop if a state recurs:
   - sort components by stable ref;
   - include only movable component refs/positions/layers/rotations;
   - quantize coordinates to a fixed internal unit small enough to preserve
     routing-relevant obstacle changes, initially no coarser than 0.01 mm.
6. Track applied hint history per component/net and dampen or block opposing
   adjustments such as alternating `reduce_distance` and `increase_spacing`.
   First opposition halves the adjustment magnitude; a second reversal for the
   same target stops retry for that target as oscillating.
7. Stop on:
   - routed success;
   - unsupported blocker;
   - max attempts;
   - repeated diagnostic signature;
   - repeated placement state hash;
   - opposing hint oscillation;
   - no improvement in best-attempt lexicographic rank;
   - new blocking placement issue.
8. Return the best-so-far attempt when later retries regress or budget is
   exhausted, using the strict lexicographic ranking from the spec.
9. Add workflow tests:
   - disabled policy unchanged;
   - eligible retry runs exactly once;
   - non-improving retry blocks with summary;
   - unsupported blocker skips retry.
   - regressing retry returns best-so-far;
   - repeated placement state stops retry.
   - opposing hint oscillation stops or dampens retry.

### Acceptance Criteria

- Workflow retry is deterministic and bounded.
- No retry loop can run indefinitely.
- Final workflow result includes the selected/final attempt.
- Regressing retries do not replace a better previous attempt.

### Suggested Commit

`Add bounded placement routing retry loop`

## Phase 7: Retry Attempt Reporting

### Goal

Make retry behavior inspectable in workflow output.

### Tasks

1. Add retry attempt summary artifact or stage summary fields:
   - attempt number;
   - placement score;
   - routing score;
   - placement status;
   - routing status;
   - routed/failed nets;
   - hint categories applied;
   - convergence status;
   - stop reason.
2. Include retry summaries in `design create` JSON output.
3. Include repair suggestions with placement retry scope where appropriate.
4. Add tests for summary shape and blocked/non-improving output.

### Acceptance Criteria

- Users can tell whether retry ran, why, and what changed.
- Non-improvement and budget exhaustion are explicit.
- Feedback points to placement retry only when retry is appropriate.

### Suggested Commit

`Report placement routing retry attempts`

## Phase 8: Golden Retry Corpus

### Goal

Lock representative retry behavior.

### Tasks

1. Add compact golden cases for:
   - retry disabled baseline;
   - routing length failure -> reduce-distance hint;
   - keepout/no-path -> spacing hint;
   - unsupported zone policy -> no retry;
   - non-improving retry -> blocked with budget evidence.
2. Compare compact summaries:
   - attempt count;
   - final status;
   - hint categories;
   - routed/failed nets;
   - placement/routing score deltas;
   - stop reason.
3. Keep fixtures deterministic and small.

### Acceptance Criteria

- Retry behavior is regression-tested without brittle full JSON snapshots.
- Golden output explains retry and non-retry outcomes.

### Suggested Commit

`Add placement routing retry golden corpus`

## Phase 9: Documentation And Roadmap

### Goal

Document the new retry capability and advance the roadmap.

### Tasks

1. Update README:
   - placement congestion/fanout reports;
   - retry policy;
   - retry output;
   - limitations.
2. Update `specs/ROADMAP.md`:
   - mark near-term retry foundation implemented;
   - add remaining gaps.
3. Run full `go test ./...`.

### Acceptance Criteria

- README accurately describes current behavior.
- Roadmap points to the next highest-value gap.
- Full test suite passes.

### Suggested Commit

`Document placement routing retry`
