# Placement Congestion And Routing Retry Specification

## Purpose

Add placement congestion/fanout scoring and a bounded placement-to-routing retry
loop so generated boards can react to routing failures instead of stopping at
the first poor placement.

The current placement engine produces deterministic placements, quality
reports, proximity/region/mechanical/routing-readiness evidence, and repairable
diagnostics. The routing engine now produces per-net route quality, failure
categories, length/search-pressure evidence, and repairable diagnostics. This
project connects those two foundations: routing failures should become
structured placement retry hints when the failure is plausibly caused by
placement density, fanout pressure, or net topology.

## Goals

1. Add placement congestion evidence that is more concrete than HPWL.
2. Add fanout and escape-readiness evidence for components with many connected
   pads or constrained local routing needs.
3. Convert routing diagnostics into placement retry hints when appropriate.
4. Add a bounded placement-routing retry loop to `design create`.
5. Preserve deterministic output and clear convergence rules.
6. Report retry attempts, changes, and non-convergence as first-class workflow
   evidence.
7. Keep imported/user-authored project mutation out of scope.

## Non-Goals

- Full interactive placement optimization.
- Simulated annealing, force-directed global placement, or stochastic placement.
- Dense BGA escape routing.
- Full push-and-shove routing.
- KiCad GUI automation.
- Mutating imported projects.
- Infinite or open-ended AI repair loops.

## Existing Foundation

Already implemented:

- Placement request/result model.
- Deterministic placer with fixed components, constraints, groups, keepouts,
  regions, edge intent, and transaction operations.
- Placement `QualityReport` with group, proximity, region, net, keepout, score,
  routing-readiness, and repair diagnostics.
- Routing adapter from placement result.
- Routing engine with route quality reports, net-class routing rules, obstacle
  diagnostics, via/layer/length/zone policy, coupled-net intent, and repairable
  diagnostics.
- Design workflow stages for placement and routing.
- Workflow feedback with retry scopes.

This project must extend those foundations rather than introduce a separate
placement engine.

## Design Principles

- Deterministic for identical request, seed, library context, and retry budget.
- Bounded by explicit attempt count and convergence checks.
- Explainable: every retry must include the diagnostics that triggered it.
- Conservative: if improvement is not measurable, stop and report blocked.
- Local changes first: prefer spacing/group/region/fanout adjustments over
  wholesale placement reshuffling.
- Preserve fixed placements and explicit user constraints.
- Do not claim routing success unless routing and validation evidence agree.

## Placement Congestion Evidence

Placement quality should include coarse congestion reports derived from placed
components and routed net intent before routing.

Required model:

- `CongestionReport`
  - grid cell ID or bounds;
  - component refs in or crossing the cell;
  - net names crossing the cell;
  - weighted crossing count;
  - estimated channel capacity;
  - utilization ratio;
  - status: `pass`, `warning`, `fail`;
  - suggested placement action.

Congestion estimation should be deterministic and cheap:

- Use a coarse grid derived from board size.
- Scale grid resolution by board size and component density, cap it initially
  at no more than 200 x 200 cells, and enforce a minimum physical cell size so
  tiny boards do not produce noisy grids.
- Estimate net crossings from deterministic direct-line or monotonic
  Manhattan-path intersections between placed endpoints. Bounding boxes may be
  used only as a low-confidence fallback, must be identified as such, and must
  carry lower scoring weight so uncertain evidence does not aggressively spread
  otherwise routable designs.
- Weight power/high-current/clock/differential nets more heavily.
- Penalize cells near keepouts, edges, and dense component clusters.
- Do not require detailed route search.

The congestion grid is a quality signal, not a final DRC rule.

## Fanout And Escape Readiness

Placement should report when components are hard to escape before routing.

Required model:

- `FanoutReport`
  - component ref;
  - pad count;
  - connected pad count;
  - local net count;
  - available side/channel summary;
  - nearby keepout/edge pressure;
  - estimated escape demand;
  - status: `pass`, `warning`, `fail`;
  - suggested action.

Initial fanout evidence can be approximate:

- Count connected pads and distinct nets.
- Estimate available escape sides from component bounds, board edges, and
  keepouts.
- Flag components near board edges or keepouts when many nets must escape.
- Prefer geometry-derived signals such as pad density, connected-net density,
  and connected nets per component perimeter over semantic labels. Component
  roles such as connectors, MCUs, USB-C, regulators, and sensors may add weight
  only when verified metadata is available.

Future phases can replace approximations with pin-level escape models.

## Routing Diagnostic To Placement Hint Mapping

Routing diagnostics should feed placement only when the failure points to
placement as the likely repair scope.

Placement retry hint categories:

- `reduce_distance`: long route, excessive length, or high HPWL.
- `increase_spacing`: clearance, keepout, obstacle pressure, blocked access.
- `improve_fanout`: pad access, via/layer access near a dense component.
- `move_from_edge`: endpoint access blocked by edge/connector pressure.
- `separate_regions`: analog/clock/power conflict or noisy proximity.
- `relax_rules`: routing rules, not placement, are the primary blocker.
- `unsupported`: retry should not run; user or future feature required.

Mapping rules:

- Routing rule failures become `relax_rules`, not placement retry.
- Zone-sufficient unsupported failures become `unsupported`.
- Missing pads/footprints become `unsupported` or upstream library repair.
- No-path failures with obstacle/keepout evidence become `increase_spacing`.
- Length failures become `reduce_distance` unless rule limits are clearly too
  strict.
- Layer/via policy failures become `relax_rules` unless placement has obvious
  same-layer endpoint separation evidence.

## Retry Policy

Retry must be explicit and bounded.

Required options:

- enabled/disabled;
- maximum attempts;
- minimum improvement threshold;
- allowed hint categories;
- preserve fixed components;
- stop on new blocking issue;
- stop on non-improvement;
- stop on repeated diagnostic signature.

Default behavior:

- disabled unless workflow opts in or request policy enables it;
- `max_attempts` means total placement/routing attempts, including the initial
  attempt. A value of `1` means no retry. A value of `2` means one retry.
- maximum attempts should default to a small number, such as 1 or 2;
- fixed/user-specified placements are never moved;
- retry summaries are emitted even when retry is skipped.

## Placement Adjustment Strategy

Initial retry adjustments should be deterministic and conservative:

- Increase component spacing within configured bounds.
- Expand group spread only when group constraints allow it.
- Move movable components away from high-congestion cells.
- Move endpoints closer for length failures when no stronger constraint blocks
  the move.
- Increase fanout clearance around components with fanout failures.
- Preserve connector edge intent and required regions.
- Preserve block-local relative placement where required.

The first implementation can represent adjustments as modified placement rules
and retry hints rather than arbitrary coordinate transforms.

## Workflow Integration

`design create` should support:

```text
placement attempt 1
  -> routing attempt 1
  -> if routing blocked and retry allowed:
       route diagnostics -> placement hints
       placement attempt 2 with adjusted rules/hints
       routing attempt 2
  -> stop when success, non-improvement, blocked unsupported, or budget exhausted
```

Workflow output must include:

- retry enabled;
- attempt count;
- per-attempt placement score;
- per-attempt routing score;
- triggering diagnostics;
- applied hints/adjustments;
- convergence result;
- final selected attempt;
- skipped/blocked reason when no retry runs.

## Convergence Rules

A retry improves only if at least one of these changes is true:

- routing status improves from blocked to partial/routed;
- failed net count decreases;
- routed net count increases;
- routing quality score improves by the configured threshold;
- placement congestion/fanout failures decrease;
- blocking diagnostic signature changes in a meaningful way.

Non-improvement cases:

- same blocking route diagnostics recur;
- a previous movable-placement state hash recurs, even if diagnostics alternate;
- placement score improves but routing score does not;
- routing score improves below threshold;
- retry introduces new placement blocking issues;
- retry exceeds budget.

Non-improvement must produce a structured blocked result.

Diagnostic signatures must be stable and intentionally coarse. They should hash
diagnostic category, report code, action, affected net names, affected refs,
normalized issue paths, and quantized failure locations when available. Raw
coordinates must never be used directly; quantization should follow the
congestion grid or another deterministic internal unit so distinct failures on
the same net can still be distinguished.

Opposing retry hints must be dampened deterministically. If a net/ref receives a
hint category that opposes a previous attempt, such as `reduce_distance` after
`increase_spacing` or the reverse, the next adjustment magnitude must be reduced
by 50%. A second opposing reversal for the same net/ref should stop retry for
that target and report oscillation.

The workflow must retain the best-so-far attempt with a strict lexicographic
ranking:

1. routing status rank: routed > partial > blocked;
2. failed net count ascending;
3. routed net count descending;
4. routing quality score descending;
5. placement score descending;
6. attempt number ascending.

If a later retry regresses or exhausts the retry budget, the workflow should
return the best attempt and report the stop reason.

## CLI And JSON Surface

`design create` request/options should eventually expose retry policy.

Candidate request shape:

```json
{
  "placement_retry": {
    "enabled": true,
    "max_attempts": 2,
    "min_routing_score_delta": 0.05,
    "allowed_hints": ["reduce_distance", "increase_spacing", "improve_fanout"],
    "preserve_fixed": true
  }
}
```

`min_routing_score_delta` is a secondary convergence check. A retry also counts
as improved when failed net count decreases or routed net count increases, so
small and large boards are not judged solely by one fixed score threshold.

CLI flags can be added after the JSON model is stable.

## Acceptance Criteria

- Placement quality reports include congestion and fanout evidence.
- Routing failures map to deterministic placement retry hints.
- `design create` can run a bounded placement-routing retry loop when enabled.
- Retry stops on success, unsupported blockers, repeated diagnostics,
  non-improvement, or budget exhaustion.
- Workflow output explains every retry attempt and final decision.
- Existing placement/routing/design workflow tests continue to pass.
- Golden tests cover improved retry and non-improving retry cases.

## Open Questions

- Should retry policy live under `validation`, `placement`, or a new
  `optimization` request section?
- What default board-grid size gives useful congestion evidence without noisy
  reports?
- Should the first adjustment mechanism modify placement rules only, or also
  introduce explicit placement hints?
- How much block-local placement is allowed to move during retry?
