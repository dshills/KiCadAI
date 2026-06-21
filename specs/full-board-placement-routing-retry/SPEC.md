# Full-Board Placement-Routing Retry Evidence Specification

## 1. Purpose

Prove that the bounded placement-routing retry loop improves or safely stops on
real generated board workflows, not only on focused helper fixtures.

The current retry foundation is implemented and covered by deterministic
goldens for fixture loading, retry summary extraction, spacing/fanout/distance
adjustments, unsupported skip behavior, selected stop conditions, and CLI output.
The remaining confidence gap is full-board evidence: generated schematic and PCB
workflows where retry materially changes the board state and produces better
routing evidence without violating hard constraints.

## 2. Problem Statement

AI callers need to know whether retry evidence means:

- the generated board improved;
- the workflow stopped because further retry was unsafe or unproductive;
- hard user constraints were preserved;
- the returned board is the best attempt, not merely the last attempt.

Today the retry system exposes the right mechanics, but most tests either use
synthetic diagnostics or small focused request fixtures. That is useful for
correctness, but it does not yet prove that retry helps on full board generation
paths created by `design create`.

## 3. Goals

This project must:

- add full-board `design create` fixtures that run block planning, component
  selection, schematic generation, PCB realization, placement, routing, and
  retry;
- include at least one fixture where retry produces measurable routing
  improvement;
- include at least one fixture where retry applies an adjustment but safely
  stops because the result does not improve;
- include at least one fixture where retry preserves fixed components,
  edge-facing constraints, keepouts, and local block routes;
- expose compact machine-readable retry improvement evidence for tests and CLI
  consumers;
- keep default tests independent of external KiCad CLI;
- keep fixtures deterministic and fast enough for normal `go test ./...`;
- document fixture intent, expected retry category, and expected stop or
  improvement behavior.

## 4. Non-Goals

This project does not:

- replace the placement or routing algorithms;
- enable retry by default;
- require `kicad-cli` for default tests;
- claim fabrication readiness;
- solve high-density autorouting;
- mutate imported projects;
- introduce natural-language planning;
- add new circuit block families unless a fixture cannot be built from existing
  blocks and the smallest supporting block is required.

## 5. Current Baseline

Implemented foundations include:

- `routing_retry` request policy and normalization;
- placement retry hint categories:
  - `increase_spacing`;
  - `improve_fanout`;
  - `reduce_distance`;
  - `move_from_edge`;
  - ineligible `relax_rules`;
  - ineligible `unsupported`;
- deterministic placement retry adjustment builder;
- proximity rule generation and duplicate prevention;
- best-attempt selection based on routing status, failed nets, and routed nets;
- repeated placement state detection;
- retry summaries attached to the routing stage;
- CLI selected-field retry summary tests;
- design example strict parse coverage for retry requests.

Known gaps:

- no full-board fixture currently proves retry improves a generated PCB;
- retry attempt history does not yet expose a normalized before/after
  improvement record;
- full-board hard-constraint preservation is not yet verified through real
  `design create` retry attempts;
- current full-board routing can fail for missing pad summaries before retry can
  address placement, so fixture design must avoid unsupported component data;
- repeated-state and non-improvement behavior are covered, but not yet through a
  realistic board that writes useful partial artifacts.

## 6. Fixture Strategy

Fixtures must be generated through `designworkflow.Create` or the
`design create` CLI using checked-in request JSON. They should prefer existing
verified blocks and components so failures represent placement/routing behavior,
not component evidence gaps.

Each fixture must declare:

- request path;
- board intent;
- expected retry category or categories;
- expected final stop reason or improvement condition;
- hard constraints to preserve;
- whether project write is expected to complete;
- whether routing is expected to be routed, partial, blocked, or skipped;
- why the fixture is deterministic.

The fixture corpus should start small:

1. **Spacing Improvement Board**
   - Goal: initial routing fails or weakens because components are too close.
   - Expected category: `increase_spacing`.
   - Expected improvement: fewer failed nets, more routed nets, or improved
     routing status after retry.

2. **Distance Improvement Board**
   - Goal: route length or HPWL pressure is reduced by proximity rules.
   - Expected category: `reduce_distance`.
   - Expected improvement: deterministic proximity rules and better route
     evidence, or clear non-improvement stop.

3. **Constraint Preservation Board**
   - Goal: retry changes movable placement while preserving hard constraints.
   - Expected constraints: fixed refs, connector edge orientation, keepouts,
     block-local routes, board outline, and net assignments.

4. **No-Improvement Full Board**
   - Goal: eligible retry applies an adjustment but does not improve routing.
   - Expected stop: `non_improving_retry` or `max_attempts` with best-so-far
     preserved.

## 7. Improvement Evidence

Tests should not depend on large full JSON snapshots. They should derive a
small retry evidence snapshot from workflow output:

```json
{
  "fixture": "spacing_improves",
  "routing_retry": {
    "enabled": true,
    "attempts": 2,
    "applied": 1,
    "stop_reason": "routed",
    "hint_categories": ["increase_spacing"]
  },
  "initial": {
    "status": "blocked",
    "routed_nets": 0,
    "failed_nets": 2
  },
  "best": {
    "status": "partial",
    "routed_nets": 1,
    "failed_nets": 1
  },
  "constraints": {
    "fixed_refs_preserved": true,
    "keepouts_preserved": true,
    "edge_constraints_preserved": true,
    "local_routes_preserved": true
  }
}
```

The exact shape may be implemented as Go structs in tests rather than persisted
JSON files, but the same fields must be asserted.

## 8. Hard Constraint Preservation

A retry attempt must not damage:

- fixed component references and positions;
- connector edge constraints;
- board dimensions and outline intent;
- hard keepouts and mechanical keepouts;
- existing block-local routes;
- net names and endpoint assignments;
- generated footprint IDs and pad net assignments;
- route width constraints and net classes.

Tests should compare these invariants before and after retry. When direct
comparison is too broad, tests must compare canonical subsets rather than full
object dumps.

## 9. CLI Contract

The CLI must continue to expose retry evidence through `design create --json`.
Full-board fixtures should add selected-field CLI checks for:

- request path;
- output path normalization;
- routing stage status;
- retry summary;
- attempt history or improvement summary;
- artifacts when project write completes;
- blocking issues when project write does not complete.

CLI tests should avoid full output snapshots because paths, artifact order, and
future summaries may change. They should assert stable fields only.

## 10. Test Requirements

Default tests must:

- run without KiCad installed;
- run under `go test ./...`;
- use deterministic seeds and explicit board sizes;
- avoid network and package downloads;
- avoid long-running autorouting cases;
- fail with actionable fixture names and summary diffs.

Optional KiCad-backed checks may be added later behind existing KiCad CLI skip
behavior, but they are not required for this project.

## 11. Acceptance Criteria

This project is complete when:

- at least three full-board retry fixtures exist;
- at least one fixture demonstrates measurable routing improvement after retry;
- at least one fixture demonstrates safe non-improvement behavior with best
  attempt preserved;
- hard-constraint preservation is asserted through real retry attempts;
- `design create --json` has selected-field coverage for one improving fixture;
- README and roadmap document the new full-board evidence and remaining
  limitations;
- `go test ./...` passes.

## 12. Risks

- Current component footprint pad metadata may block routing before placement
  retry can help.
- The deterministic router may be too capable or too limited to naturally
  produce improvement cases without carefully shaped boards.
- Full-board fixtures can become brittle if they assert too much geometry.
- Adding fixture-only hooks could weaken production code if not isolated.

## 13. Mitigations

- Start with the smallest existing verified blocks that have usable pad and
  route data.
- Prefer invariant and metric assertions over exact coordinates.
- Use seeded placement and explicit routing options.
- If a production evidence field is missing, add a small typed summary rather
  than test-only behavior.
- Keep any helper-only fixture builder inside `_test.go` files unless the data
  is genuinely useful to CLI users.
