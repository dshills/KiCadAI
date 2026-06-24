# Semantic Placement Scoring Implementation Plan

Date: 2026-06-24

This plan implements `specs/semantic-placement-scoring/SPEC.md`.

Each phase should be implemented, reviewed with Prism, tested, and committed
before moving to the next phase.

## Phase 1: Candidate Score Model

### Goal

Add placement candidate score types and deterministic normalization without
changing placement decisions.

### Work

- Add candidate score model types in `internal/placement`.
- Add score dimension and rejection reason enums/constants.
- Add score policy/options to placement rules with conservative defaults.
- Add deterministic normalization:
  - score dimension ordering;
  - bounded evidence strings;
  - stable candidate ordering;
  - rounded numeric scores for reports.
- Add empty/no-op scoring output to placement results.

### Tests

- Score normalization is deterministic.
- Dimension ordering is stable.
- Empty scoring policy preserves existing placement behavior.
- Bounded evidence truncates predictably.

### Acceptance

- Existing placement behavior and golden outputs remain compatible except for
  additive empty scoring fields.

### Commit

```text
Add placement candidate score model
```

## Phase 2: Hard Constraint Candidate Rejection

### Goal

Separate candidate legality from soft scoring so unsafe candidates cannot win.

### Work

- Add candidate rejection checks for:
  - outside board;
  - collisions;
  - hard keepouts;
  - fixed/mobility violations;
  - hard edge/side/rotation violations.
- Record rejection reasons and counts.
- Ensure rejected candidates are excluded from winner selection.
- Preserve existing placement issues for final selected placements.

### Tests

- Rejected outside-board candidate cannot win.
- Rejected collision candidate cannot win.
- Rejected fixed/mobility violation cannot win.
- Rejection counts are deterministic and bounded.

### Acceptance

- Hard constraints dominate all soft score dimensions.

### Commit

```text
Reject illegal placement candidates before scoring
```

## Phase 3: Semantic Role And Group Scoring

### Goal

Use component roles and block groups to prefer physically coherent placements.

### Work

- Add semantic role score dimension.
- Add group cohesion score dimension.
- Score candidates relative to:
  - group anchor;
  - already placed group members;
  - role priority inside group;
  - group spread limits where present.
- Add evidence for group ID, anchor ref, distance, and spread.
- Keep tie-breaking deterministic.

### Tests

- Decoupling/support part candidate near its group anchor scores higher than a
  far candidate.
- Regulator support components prefer the regulator group.
- Equal-score ties break by stable candidate index/ref.
- Missing group metadata omits group score without blocking.

### Acceptance

- Representative block-local support components are placed closer to anchors
  than under generic candidate scoring.

### Commit

```text
Score placement candidates by semantic role and group
```

## Phase 4: Electrical Proximity And Route-Length Scoring

### Goal

Prefer candidates that reduce expected electrical path length.

### Work

- Add electrical proximity score dimension.
- Add route-length score dimension based on incremental HPWL impact.
- Prefer pad-to-pad distance where pad summaries are available.
- Fall back to center-to-center distance with weaker evidence.
- Score required local-route endpoints more strongly than generic nets.
- Record net/ref/pad evidence.

### Tests

- Pad-backed proximity beats center fallback when pads exist.
- Center fallback still gives deterministic evidence when pads are missing.
- Local-route endpoints score higher when close.
- Route-length score lowers predicted HPWL in a controlled fixture.

### Acceptance

- Candidate score evidence explains why electrically related parts were placed
  closer together.

### Commit

```text
Score placement candidates by electrical proximity
```

## Phase 5: Congestion And Fanout Scoring

### Goal

Use existing congestion and fanout concepts before placement is finalized.

### Work

- Add congestion score dimension using coarse grid crossing/occupancy pressure.
- Add fanout score dimension using:
  - pad escape room;
  - nearby components;
  - keepouts;
  - edge pressure;
  - local-route demand.
- Penalize candidates that create obvious routing bottlenecks.
- Preserve coarse, deterministic calculations; do not route.

### Tests

- Candidate that blocks high-pin-count fanout scores lower.
- Candidate in crowded coarse grid cell scores lower.
- Connector/internal fanout clearance score is reported.
- Score output identifies affected refs/nets.

### Acceptance

- Placement summaries expose congestion and fanout penalties before routing.

### Commit

```text
Score placement candidates by congestion and fanout
```

## Phase 6: Candidate Selection Integration

### Goal

Use candidate scores to choose placements while preserving deterministic output.

### Work

- Integrate candidate scoring into candidate selection.
- Add score-aware winner selection:
  - reject illegal candidates;
  - sort by total score;
  - tie-break by stable candidate order.
- Add aggregate candidate scoring report to placement result quality.
- Bound alternative candidate evidence per component.
- Add a placement rule switch if needed for compatibility.

### Tests

- Candidate scoring changes selected placement in controlled fixtures.
- Scoring does not override hard constraints.
- Scores are stable across repeated runs.
- Existing placement corpus still passes.

### Acceptance

- Placement uses semantic scores for selection and remains reproducible.

### Commit

```text
Use semantic scores for placement candidate selection
```

## Phase 7: Workflow And Retry Evidence

### Goal

Expose scoring evidence to AI-facing workflow output and retry summaries.

### Work

- Add candidate scoring summary to `design create` placement stage.
- Add selected scoring fields to placement-routing retry attempt summaries.
- Preserve evidence when retry creates adjusted placement requests.
- Add explanation fields for top score penalties and rejected candidate counts.
- Keep JSON additions backward-compatible.

### Tests

- `design create` placement summary includes candidate scoring status and
  aggregate score fields.
- Retry summaries include scoring evidence for attempts.
- Existing CLI golden tests remain stable or are intentionally updated.

### Acceptance

- AI callers can see why placements were selected and why retry attempts did or
  did not improve.

### Commit

```text
Expose semantic placement scoring evidence
```

## Phase 8: Golden Fixtures And Regression Proof

### Goal

Prove semantic scoring improves representative generated layouts without
breaking existing behavior.

### Work

- Add or update golden placement fixtures for:
  - decoupling/proximity improvement;
  - connector/fanout improvement;
  - group cohesion tie-break;
  - missing semantic metadata fallback.
- Add before/after assertions where feasible using deterministic fixture
  comparisons.
- Add documentation notes to README and roadmap.

### Tests

- Focused placement golden tests.
- `go test ./internal/placement ./internal/designworkflow`.
- Full `go test ./...`.

### Acceptance

- At least two representative fixtures show improved semantic placement
  evidence.
- Existing behavior remains deterministic.

### Commit

```text
Add semantic placement scoring goldens
```

## Phase 9: Final Verification

### Goal

Close the implementation with full verification and review.

### Work

- Run focused placement/design workflow tests.
- Run full `go test ./...`.
- Run Prism on staged changes.
- Resolve all high and medium findings.
- Update this plan with implementation status and commit trail.

### Acceptance

- Full test suite passes.
- Prism has no unresolved high or medium findings.
- Worktree is committed phase by phase.

### Commit

```text
Complete semantic placement scoring
```
