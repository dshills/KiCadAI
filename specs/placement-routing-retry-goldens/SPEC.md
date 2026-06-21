# Placement Routing Retry Golden Fixtures Specification

## 1. Purpose

Prove the bounded placement-routing retry loop behaves correctly on realistic
generated board workflows, not only isolated helper tests.

The placement-routing retry foundation is now implemented:

- placement quality reports include congestion and fanout evidence;
- routing diagnostics map to placement retry hints;
- design requests can opt into `routing_retry`;
- retry adjustments can increase spacing or add proximity rules;
- `design create` can run bounded placement/routing retries and return the
  best attempt with retry summary evidence.

The missing confidence layer is a golden corpus that exercises the retry loop
end to end and locks down the workflow contract for AI agents.

## 2. Goals

This project must:

- add full-board golden fixtures that intentionally trigger placement-routing
  retry categories;
- validate retry summaries, attempt history, stop reasons, and best-attempt
  behavior;
- prove fixed components and hard constraints are preserved across retries;
- prove unsupported and rule-only routing failures do not trigger unsafe
  placement mutation;
- keep all fixtures deterministic, fast, and runnable in normal `go test`;
- add CLI/workflow snapshot tests for `routing_retry` request behavior;
- document how AI callers should interpret retry evidence.

## 3. Non-Goals

This project does not:

- improve the core placement algorithm;
- add new routing algorithms;
- require `kicad-cli` for default tests;
- claim fabrication readiness;
- solve imported-project mutation;
- add natural-language planning;
- make retry enabled by default.

## 4. Current Baseline

Implemented foundations include:

- `placement.QualityReport.CongestionReports`;
- `placement.QualityReport.FanoutReports`;
- `designworkflow.BuildPlacementRetryHints`;
- `designworkflow.RoutingRetryPolicySpec`;
- `designworkflow.BuildPlacementRetryAdjustment`;
- bounded `design create` placement-routing retry;
- retry summary attached to the routing stage;
- placement state hash and best-attempt selection;
- focused package tests for mapping, policies, adjustment helpers, and retry
  ranking.

Current gaps:

- no full-board workflow fixture proves retry improves or safely stops;
- no golden request/output snapshots prove JSON contract stability;
- no tests verify fixed-component preservation through a real retry attempt;
- no tests exercise repeated-state and non-improvement stop behavior through
  workflow-level outputs;
- no examples show `routing_retry` in a realistic request.

## 5. Golden Fixture Categories

The golden corpus must cover these scenarios.

### 5.1 Increase Spacing

Create a generated board where routing fails because a route channel is blocked
or overly congested, and retry can safely increase spacing.

Expected evidence:

- initial routing stage has blocked or weak route diagnostics;
- retry hints include `increase_spacing`;
- retry adjustment increases component/group spacing;
- retry summary records at least one applied adjustment;
- best attempt is deterministic;
- fixed components remain fixed.

### 5.2 Improve Fanout

Create a generated board with a high-pin or dense component near blocked escape
sides.

Expected evidence:

- placement quality includes `FanoutReport` warning or failure;
- routing diagnostics map to `improve_fanout` where supported by affected refs;
- retry summary includes fanout hint category;
- final output preserves hard edge constraints and fixed placements.

### 5.3 Reduce Distance

Create a generated board where route length policy or HPWL pressure produces a
`reduce_distance` hint.

Expected evidence:

- retry adjustment adds deterministic proximity rule IDs;
- proximity rules use complexity-based anchors;
- repeated retry does not duplicate rule IDs;
- best attempt improves route length evidence or stops as non-improving.

### 5.4 Non-Improving Retry Stop

Create a fixture where retry is eligible and adjustment is applied, but routing
rank does not improve.

Expected evidence:

- retry summary `stop_reason` is `non_improving_retry`;
- result returns best-so-far, not the regressed attempt;
- attempt history includes the non-improving attempt;
- no success is overstated.

### 5.5 Repeated Placement State Stop

Create a fixture where retry adjustment cannot produce a new movable placement
state after rerun.

Expected evidence:

- retry summary `stop_reason` is `repeated_placement_state`;
- state hashing ignores fixed components and considers movable refs only;
- workflow terminates within budget.

### 5.6 Unsupported Or Rule-Only Skip

Create fixtures for routing failures that should not trigger placement retry:

- zone policy unsupported;
- routing-rule-only failures;
- missing pad/footprint/input model issues.

Expected evidence:

- hints map to `unsupported` or `relax_rules`;
- retry summary `applied` remains zero;
- stop reason is `no_eligible_hints` or equivalent;
- placement output remains unchanged.

## 6. Fixture Shape

Golden fixtures should live under a deterministic test fixture directory, for
example:

```text
testdata/designworkflow/retry/
  increase_spacing/request.json
  improve_fanout/request.json
  reduce_distance/request.json
  non_improving/request.json
  repeated_state/request.json
  unsupported_zone/request.json
```

Each fixture should include:

- a request JSON using `routing_retry`;
- expected retry summary fields;
- expected stage status;
- expected hint categories;
- expected stop reason;
- expected invariants, such as fixed refs and no duplicate proximity rules.

Snapshot files should avoid unstable absolute paths. Normalize output
directories and artifact paths before comparison.

## 7. Workflow Contract

Retry summaries must remain stable enough for AI agents.

Required routing stage summary fields:

- `routing_retry.enabled`;
- `routing_retry.attempts`;
- `routing_retry.applied`;
- `routing_retry.stop_reason`;
- `routing_retry.hint_categories`;
- `routing_retry.attempt_history`;

Attempt history must be compact and bounded. It should include:

- attempt number;
- placement summary;
- routing status;
- failed net count;
- routed net count.

The final workflow result must:

- show the selected best placement stage;
- show the selected best routing stage;
- avoid hiding retry failure evidence;
- avoid claiming success when routing remains blocked;
- preserve validation and repair stages after retry.

## 8. Determinism Requirements

All golden tests must be deterministic:

- fixed seeds where applicable;
- stable request JSON ordering;
- stable hint ordering;
- stable stage summaries after path normalization;
- no dependence on local KiCad CLI;
- no network access;
- no wall-clock timestamps in snapshots.

## 9. Safety Requirements

Retry tests must explicitly verify:

- fixed components are unchanged;
- hard keepouts remain respected;
- required edge constraints remain respected;
- retry does not enable unsupported categories;
- max attempts is honored;
- context cancellation stops before further heavy work where testable;
- repeated state and non-improvement stop conditions are observable.

## 10. Test Strategy

Use layers of tests:

- small unit tests for new fixture helpers and snapshot normalization;
- designworkflow package tests for request-to-result retry summaries;
- optional CLI golden tests for `design create --request ...`;
- no KiCad CLI dependency in default test path.

When a fixture needs behavior that the current router cannot reliably produce,
prefer a deterministic test harness around workflow retry helpers rather than
fragile geometry. Full `design create` fixtures should be added where the
current block/placement/router stack can reproduce the scenario reliably.

## 11. Documentation Requirements

Update README and/or examples with:

- a `routing_retry` request snippet;
- explanation of `max_attempts` as total attempts;
- supported hint categories;
- retry stop reasons;
- caveat that retry is conservative and opt-in.

## 12. Acceptance Criteria

This project is complete when:

- golden tests cover all six fixture categories;
- workflow retry summaries are snapshot-tested or invariant-tested;
- fixed/hard-constraint preservation is tested through retry;
- non-improvement and repeated-state stops are tested;
- unsupported/rule-only failures skip placement mutation;
- full `go test ./...` passes;
- README or examples document how to opt into retry and interpret output.
