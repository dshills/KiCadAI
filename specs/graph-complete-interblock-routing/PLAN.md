# Graph-Complete Inter-Block Routing Implementation Plan

Date: 2026-07-01

## Objective

Turn route-tree evidence into graph-complete generated inter-block routing for
multi-endpoint nets, starting with `i2c_sensor_breakout_candidate`.

The previous project completed grouping, route-tree planning, branch routing
primitives, and completion summaries. This plan wires those pieces into the
workflow execution path and closes the remaining partial/missed-contact
behavior as far as the current router can support.

## Phase 1: Baseline Route-Tree Execution Audit

### Goals

- Capture the exact current I2C route-tree state in tests.
- Record branch-level blockers for VCC, GND, SDA, and SCL.
- Prove that current summaries are specific enough to guide execution changes.

### Tasks

- Add focused tests that run the I2C fixture through `RoutePlacement`.
- Extract and assert current `inter_block_routing` fields:
  - `multi_endpoint_nets`;
  - `required_endpoints`;
  - `proven_endpoints`;
  - `branches_planned`;
  - `branches_attempted`;
  - `branches_completed`;
  - `graph_component_count`;
  - `complete_groups`;
  - `partial_groups`;
  - `blocked_groups`.
- Add helper assertions for issue paths and blocked nets.
- Add a branch-level audit helper that maps route-tree branches to contact
  targets and existing route operations where available.
- Confirm generated outputs are not staged.

### Acceptance

- Tests deterministically reproduce the current I2C blockers without KiCad.
- The test names identify VCC/SDA legal-path failures, GND partial graph
  completion, and SCL contact miss evidence.
- Existing connector/LED route completion tests still pass.
- `go test ./internal/designworkflow -run 'TestRoutePlacementI2CSensorBreakout|TestRoutePlacementPromotedInterBlockConnectorLED' -count=1` passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Audit graph-complete inter-block routing blockers`.

## Phase 2: Workflow Route-Tree Executor Integration

### Goals

- Make `RoutePlacement` execute route trees for generated inter-block groups.
- Prevent route-tree-managed nets from being redundantly routed by the old
  net-level path.

### Tasks

- Add an internal route-tree execution function that accepts:
  - base routing request;
  - inter-block route groups;
  - inter-block route trees;
  - contact evidence;
  - context.
- Route each eligible group through `RouteInterBlockTreeBranches`.
- Merge branch route operations with existing local/anchor/non-managed route
  operations.
- Exclude managed inter-block nets from the normal routing request.
- Keep existing routing behavior for:
  - local-only designs;
  - skipped routing;
  - placement-blocked routing;
  - raw `route request`;
  - route groups that cannot be planned.
- Add tests for:
  - connector/LED still complete;
  - I2C uses route-tree managed nets;
  - old net-level router does not double-route managed nets;
  - skipped routing summaries remain unchanged.

### Acceptance

- Route-tree execution is visible in routing stage summaries.
- Managed inter-block nets are not duplicated between branch route operations
  and normal net-level route operations.
- Existing LED and connector/LED workflow tests pass.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Execute inter-block route trees in workflow routing`.

## Phase 3: Same-Net Graph And Contact Reproof

### Goals

- Recompute contact evidence from the merged operation set after route-tree
  execution.
- Count completion only when all required targets are in one graph.

### Tasks

- Ensure local, anchor, non-managed, and branch operations are all included in
  final inter-block contact proof.
- Add branch operation IDs or branch indexes to diagnostics where practical.
- Update summaries to distinguish:
  - planned branches;
  - attempted branches;
  - routed branches;
  - contact-proven branches;
  - graph-complete groups.
- Ensure contact misses name the branch endpoint and target ref/pad when
  available.
- Add tests for:
  - three-endpoint graph complete fixture;
  - split-graph fixture;
  - missing contact target fixture;
  - branch routed but contact-unproven fixture.

### Acceptance

- `routes_completed` and `complete_groups` require graph proof.
- Partial branch copper does not inflate completion counts.
- Contact proof evidence is generated from the final merged operation set.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Reprove route-tree contacts after branch routing`.

## Phase 4: Layer Access And Through-Hole Transition Support

### Goals

- Remove avoidable layer-access blockers for headers and through-hole pads.
- Preserve conservative layer behavior for SMD pads and wrong-net copper.

### Tasks

- Audit how `routingadapters.RequestFromPlacement` emits pad layer access for:
  - through-hole connector pads;
  - SMD pull-ups;
  - SMD decoupling capacitor;
  - sensor pads.
- Add same-net through-hole layer-transition evidence:
  - either in raw routing search;
  - or as pre-approved branch access when a through-hole pad belongs to the
    branch net.
- Ensure generated vias are still used when no pad-provided transition exists.
- Add diagnostics for layer-access failures.
- Add tests for:
  - through-hole pad can bridge F.Cu/B.Cu for the same net;
  - SMD pad cannot bridge layers unless its layers allow it;
  - wrong-net through-hole pad remains an obstacle;
  - I2C header endpoint layer access is represented.

### Acceptance

- Header through-hole pads can participate in route-tree completion without
  requiring unnecessary generated vias.
- Wrong-net pads remain blocking obstacles.
- No raw router regression in via-policy tests.
- `go test ./internal/routing ./internal/routingadapters ./internal/designworkflow -count=1` passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Support same-net through-hole route-tree transitions`.

## Phase 5: Branch-Level Repair Diagnostics And Retry Hints

### Goals

- Turn route-tree branch failures into repairable workflow diagnostics.
- Give placement retry enough information to improve failed multi-endpoint
  routes.

### Tasks

- Map branch legal-path failures to placement retry hint categories:
  - spacing;
  - distance;
  - fanout;
  - edge;
  - unsupported/rule-only.
- Preserve refs and nets from nearest obstacle diagnostics.
- Add route-tree contact failure diagnostics:
  - missing emitted contact;
  - missed target;
  - graph split;
  - wrong layer;
  - wrong net.
- Include branch endpoint IDs in issue paths.
- Add tests for:
  - VCC/SDA legal-path failures produce actionable placement hints;
  - SCL contact miss produces route/contact repair hint;
  - retry summary preserves selected attempt logic;
  - diagnostics remain stable across repeated runs.

### Acceptance

- I2C branch blockers appear as branch-scoped issues or repair guidance.
- Placement retry consumes route-tree blockers without regressing existing retry
  goldens.
- `go test ./internal/designworkflow -run 'Retry|RoutePlacementI2C|InterBlock' -count=1` passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Map route-tree blockers to repair diagnostics`.

## Phase 6: I2C Fixture Promotion Decision

### Goals

- Re-run the checked-in I2C fixture and decide whether it can move from
  `expected_fail` to `candidate`.

### Tasks

- Run:

```sh
kicadai \
  --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json \
  --output examples/.generated/i2c_sensor_breakout_candidate \
  --overwrite \
  design create
```

- Inspect:
  - routing stage status;
  - `inter_block_routing`;
  - `inter_block_contacts`;
  - promotion report;
  - project write stage;
  - writer correctness stage;
  - validation stage;
  - optional KiCad checks when configured.
- If internal route completion succeeds:
  - update metadata readiness to `candidate` only if promotion policy allows;
  - update expected artifacts/stages;
  - update README and roadmap.
- If it still blocks:
  - keep `expected_fail`;
  - update known gaps with the narrower branch-level blockers;
  - ensure the blocker is more specific than the current VCC/SDA/GND/SCL
    summary.

### Acceptance

- The fixture state matches actual workflow output.
- Either the fixture promotes to `candidate`, or the `expected_fail` blocker is
  narrowed to branch-level diagnostics.
- No generated files under `examples/.generated` are staged unless explicitly
  intended.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Advance graph-complete I2C fixture routing`.

## Phase 7: Documentation And Closeout

### Goals

- Document what graph-complete inter-block routing proves and what remains
  blocked.

### Tasks

- Update `docs/layout-routing.md` if summary fields or semantics changed.
- Update `examples/design/kicad-backed/README.md` if fixture readiness or
  blockers changed.
- Update `specs/ROADMAP.md`.
- Add notes to this spec or a status file if route-tree execution remains
  partially blocked.
- Run:
  - focused design workflow tests;
  - routing package tests;
  - `go test ./...`;
  - optional I2C fixture command.

### Acceptance

- Documentation does not overclaim fabrication readiness.
- The roadmap says whether I2C remains `expected_fail` or moves to
  `candidate`.
- Prism review has no unresolved high or medium findings.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document graph-complete inter-block routing`.

## Risks

- Route-tree execution could produce more partial copper unless incomplete
  groups are handled carefully.
- Through-hole transition support may touch raw router occupancy/search logic.
- Placement retry may improve one route group while regressing another; selected
  attempt logic must stay conservative.
- Fixture promotion may still require KiCad DRC-backed layout improvements even
  after internal route completion succeeds.

## Completion Definition

This project is complete when:

- generated inter-block route groups are executed through route trees;
- required endpoints are graph-proven before completion counts increment;
- branch failures are diagnostic and repairable;
- the I2C fixture is either promoted or has branch-level blockers that are
  strictly narrower than the current group-level blockers;
- docs and roadmap reflect the actual state.

