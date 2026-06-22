# Generated Full-Board Retry Fixtures Specification

## 1. Purpose

Prove that the generated `design create` placement-routing retry loop improves
real full-board workflow outputs after footprint pad hydration, not only
hand-built seed fixtures or isolated retry helpers.

The current foundation can generate KiCad projects from structured block
requests, hydrate footprint pads, route small boards, report routing retry
summaries, and preserve best-attempt evidence. The remaining confidence gap is
broader generated-board proof: multiple `design create` requests should show
baseline evidence, retry attempts, selected best attempt, and final validation
deltas in a deterministic golden corpus.

## 2. Goals

This project must:

- add generated full-board retry fixtures driven by real `design create`
  requests;
- capture baseline, retry attempt, best-attempt, routing, connectivity, and
  validation evidence for each fixture;
- prove at least one generated board measurably improves after retry;
- prove at least one generated board stops safely when retry cannot improve the
  board;
- prove hard placement constraints, fixed refs, board outlines, and generated
  net intent survive retry;
- keep retry evidence stable enough for AI callers and future regression tests;
- avoid requiring KiCad CLI, network access, or global KiCad library state in
  default tests;
- document how AI callers should interpret generated-board retry evidence.

## 3. Non-Goals

This project does not:

- redesign the placement engine;
- implement a new autorouter;
- make retry enabled by default;
- add natural-language intent planning;
- claim fabrication readiness;
- require real KiCad ERC/DRC in default tests;
- mutate imported user projects;
- expand the component catalog except where a fixture needs a minimal verified
  record that already fits the current catalog model.

## 4. Current Baseline

Implemented foundations include:

- generated workflow `design create` orchestration;
- block planning, component selection, schematic generation, PCB realization,
  placement, routing, validation, optional retry, and optional repair stages;
- resolver-backed footprint graphics, bounds, pads, and model hydration;
- generated workflow pad hydration evidence;
- routing diagnostic to placement retry hint mapping;
- bounded placement-routing retry with attempt history and best-attempt
  selection;
- pad-backed full-board seed fixtures for spacing improvement, distance rules,
  safe stop, generated LED connectivity, and selected CLI evidence;
- existing retry unit/golden fixtures under
  `internal/designworkflow/testdata/retry` and
  `internal/designworkflow/testdata/full_board_retry`.

Current gaps:

- generated-board retry evidence is still narrow and does not prove improvement
  across enough `design create` requests;
- tests mostly verify that generated workflows reach routing/connectivity
  diagnostics, not that retry improves final validation deltas;
- fixture metadata does not consistently describe baseline metrics, expected
  attempts, expected best-attempt selection, or hard-constraint preservation;
- CLI assertions cover selected retry summary fields but not a durable
  generated-board evidence contract;
- README and roadmap language describe the next milestone, but the repository
  does not yet contain a focused spec for it.

## 5. Fixture Corpus

The corpus should live under:

```text
internal/designworkflow/testdata/full_board_retry/
```

Each fixture directory must contain:

- `request.json`: a realistic `design create` request with `routing_retry`;
- `metadata.json`: stable expected fields and fixture intent;
- optional fixture-local notes only when needed to explain unsupported
  behavior.

Required fixture classes:

### 5.1 Generated Improvement Fixture

A generated board where retry measurably improves evidence.

Required evidence:

- baseline attempt has worse routing, connectivity, or validation metrics;
- retry applies at least one supported adjustment;
- selected best attempt is not merely the initial attempt;
- final evidence improves at least one declared metric without regressing hard
  constraints;
- attempt history records the before/after transition.

Candidate requests:

- LED indicator with intentionally tight placement;
- regulator plus connector with blocked route channel;
- sensor breakout with long or congested I2C/power nets.

### 5.2 Generated Safe Stop Fixture

A generated board where retry is eligible or requested but cannot improve the
result.

Required evidence:

- retry terminates with a stable stop reason such as `no_eligible_hints`,
  `non_improving_retry`, `repeated_placement_state`, or
  `no_safe_adjustment`;
- selected output is the best available attempt;
- no status overstates success;
- hard constraints and generated files remain valid.

### 5.3 Constraint Preservation Fixture

A generated board with fixed refs, board regions, edge constraints, or
keepouts.

Required evidence:

- fixed refs retain placement;
- hard regions and keepouts are not violated by retry;
- retry may move eligible refs only;
- validation reports any remaining issue as structured, attributable, and
  actionable.

### 5.4 Multi-Block Fixture

A generated board with more than one circuit block and real inter-block nets.

Required evidence:

- schematic-to-PCB transfer creates stable net intent;
- footprint pads are hydrated for every routed component;
- retry evidence is attached to a real generated routing stage;
- final board validation includes route completion or explicit unrouted-net
  deltas.

### 5.5 Unsupported Boundary Fixture

A generated board where retry should not mutate placement because the failure
belongs to unsupported routing, missing evidence, or rule policy.

Required evidence:

- retry reports skipped or unsupported categories;
- attempt count stays within policy;
- placement output remains unchanged;
- blockers are exposed for AI planning rather than hidden.

## 6. Evidence Contract

Each full-board retry fixture must assert a compact generated-board evidence
contract.

Required summary fields:

- retry enabled flag;
- attempt count;
- applied adjustment count;
- stop reason;
- hint categories;
- selected best attempt index or equivalent stable indicator;
- attempt history;
- baseline and final routing status;
- baseline and final routed/unrouted net counts where available;
- baseline and final board validation blocking counts where available;
- pad hydration totals and missing-pad counts;
- preserved fixed refs or hard-constraint indicators.

The evidence may be spread across existing workflow stages, but tests should
centralize extraction through helper functions so the contract is easy to
audit.

## 7. Improvement Metrics

Fixtures may use one or more deterministic improvement metrics:

- lower unrouted net count;
- higher routed net count;
- lower routing failure count;
- lower blocking validation issue count;
- improved route completion ratio;
- transition from missing route diagnostics to real route/connectivity
  diagnostics;
- better retry ranking score if already exposed by the workflow.

An improvement fixture must explicitly declare which metric is expected to
improve. A fixture must not pass merely because retry ran.

## 8. Determinism Requirements

Default tests must be deterministic:

- fixed seeds in requests and policies;
- stable fixture JSON ordering;
- no absolute paths in snapshots;
- no dependence on local KiCad CLI;
- no network access;
- bounded attempt counts;
- stable issue and hint category ordering;
- stable metadata assertions rather than full volatile output snapshots.

## 9. Safety Requirements

- Retry must not loosen hard placement or board constraints to satisfy a test.
- Retry must not invent pads, nets, footprints, or constraints.
- Missing evidence must remain a blocking or warning issue as appropriate.
- Generated output must remain inspectable even when retry stops blocked.
- Tests must verify selected best attempt behavior, not just final stage shape.
- Existing seed and retry fixtures must keep passing.

## 10. Test Requirements

Required tests:

- fixture metadata loading and validation;
- generated-board improvement assertions;
- generated-board safe-stop assertions;
- hard constraint preservation assertions;
- multi-block generated-board evidence assertions;
- unsupported boundary assertions;
- CLI selected-field assertions for at least one generated retry fixture;
- regression tests proving pad hydration evidence is present before retry
  evidence is interpreted.

Recommended package coverage:

- `internal/designworkflow` for workflow fixture execution and evidence
  extraction;
- `cmd/kicadai` for CLI selected-field coverage;
- `internal/placement` or `internal/routing` only if reusable metric helpers
  naturally belong there.

## 11. Documentation Requirements

Update documentation to explain:

- generated full-board retry is opt-in through request policy;
- retry evidence proves bounded improvement or bounded safe stop, not
  fabrication readiness;
- AI callers should inspect baseline/final deltas, stop reason, hint
  categories, and hard-constraint preservation;
- the next roadmap item after this corpus is fabrication export/readiness
  gates.

## 12. Acceptance Gates

This project is complete when:

- at least three generated full-board fixtures execute through `design create`;
- at least one fixture proves measurable improvement after retry;
- at least one fixture proves safe stop without overstating success;
- at least one fixture proves hard-constraint preservation;
- multi-block generated routing evidence includes hydrated pads and net deltas;
- CLI output exposes stable selected retry evidence for generated workflows;
- README and `specs/ROADMAP.md` are updated to mark this milestone complete and
  move the next priority to fabrication export/readiness gates;
- `go test ./...` passes without KiCad CLI, network access, or global library
  dependencies.
