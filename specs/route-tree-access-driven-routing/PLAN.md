# Route-Tree Access-Driven Routing Implementation Plan

Date: 2026-07-02

## Objective

Use route-tree endpoint access and contact-graph evidence as actual routing
inputs, not only diagnostics, so generated branch routes can choose local-route
anchors, same-net merge points, and alternate pad access candidates before
failing. The target is the remaining I2C GND/SDA branch/contact blockers.

## Implementation Status

Implemented phases 1-6 on 2026-07-02. Route-tree branches now rank access
candidates, route through bounded synthetic access-pad pairs, preserve selected
local-anchor endpoints during post-route snapping, and expose selected access
roles in branch evidence. The regenerated I2C fixture remains `expected_fail`:
the selected retry still proves 11 of 12 required contacts with three complete
route-tree contact-graph groups and one partial group, leaving one VCC
contact/branch proof gap for follow-up work.

## Phase 1: Baseline And Access Gap Tests

### Goals

- Lock the current post-contact-graph I2C state.
- Identify the exact evidence gap between available access candidates and
  branch executor failures.

### Tasks

- Add focused tests for the I2C fixture asserting:
  - `route_tree_access.local_route_anchors > 0`;
  - `route_tree_contact_graph.proven_endpoints == 11`;
  - `route_tree_contact_graph.complete_groups == 3`;
  - `route_tree_contact_graph.partial_groups == 1`;
  - branch executor still reports GND/SDA blockers.
- Add tests that compare access candidate nets with route-tree managed nets.
- Add helper assertions for access roles by net.
- Ensure LED and connector/LED route-tree/contact evidence remains stable.

### Acceptance

- Tests document that access evidence exists but branch executor completion is
  still blocked.
- `go test ./internal/designworkflow -run 'RouteTree|I2C|ConnectorLED|Access' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Capture access-driven route-tree baseline`.

## Phase 2: Access Candidate Ranking

### Goals

- Rank endpoint access candidates deterministically for branch routing.
- Prefer useful local-route anchors before pad centers when legal.

### Tasks

- Add access candidate score model with dimensions:
  - role priority;
  - same-net proof confidence;
  - distance to opposite endpoint;
  - layer compatibility;
  - nearby obstacle pressure where available;
  - stable tie-breakers.
- Add functions to fetch access candidates by endpoint ID and net.
- Add deterministic pair generation for branch source/target candidates.
- Bound candidate pair count per branch.
- Add tests for local-route anchor priority, pad fallback, tie-breaking, and
  pair limit behavior.

### Acceptance

- Candidate ranking is deterministic.
- Local-route anchors outrank pad centers when both are valid.
- Pair generation is bounded and stable.
- `go test ./internal/designworkflow -run 'AccessCandidate|RouteTreeBranch' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Rank route-tree branch access candidates`.

## Phase 3: Point/Access Route Request Support

### Goals

- Allow branch routing to target explicit access coordinates without leaking
  synthetic objects into final KiCad files.

### Tasks

- Choose implementation strategy:
  - explicit point/layer route endpoints in `internal/routing`; or
  - internal-only synthetic components/pads around access candidates.
- Keep ordinary ref/pin routing behavior unchanged.
- Ensure access routes respect occupancy, layer/via policy, clearance, and
  board-edge rules.
- Add route tests for explicit point access or synthetic access pads.
- Add failure tests for wrong layer, blocked access, and missing access.

### Acceptance

- A route can be planned between explicit access points.
- Synthetic/helper access objects are not emitted as project artifacts.
- Existing route tests pass unchanged.
- `go test ./internal/routing -run 'PointAccess|RouteRequest|SameNet' -count=1`
  passes if routing internals change.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Support route-tree point access routing`.

## Phase 4: Multi-Candidate Branch Execution

### Goals

- Try ranked branch access pairs until one route both succeeds and proves
  contact/merge.

### Tasks

- Extend `RouteInterBlockTreeBranches` or add an access-driven variant.
- Generate candidate access pairs for each branch.
- Try pairs in deterministic order up to a bounded limit.
- Discard failed pair operations and copper.
- Stop on the first valid route/contact proof.
- Preserve existing branch indices and issue paths.
- Add branch evidence fields for:
  - attempted pair count;
  - selected roles;
  - selected coordinates;
  - merge kind;
  - access-driven status.
- Add tests for first-pair failure followed by second-pair success.
- Add tests that failed access attempts emit no partial copper.

### Acceptance

- Branch executor can recover from a blocked first access pair.
- Branch evidence records access-driven attempts.
- Existing branch-scoped issue paths remain stable.
- `go test ./internal/designworkflow -run 'AccessDriven|InterBlockTreeBranches|RouteTreeBranch' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Route branches through ranked access pairs`.

## Phase 5: Endpoint Snapping And Graph Proof

### Goals

- Ensure access-driven branch routes end at valid same-net graph nodes and
  eliminate contact misses when graph-valid contact exists.

### Tasks

- Snap branch route endpoints to selected access candidates within tolerance.
- Accept route merge into local-route anchors or same-net branch copper when
  graph proof confirms connectivity.
- Distinguish route failure from post-route contact miss in branch issues.
- Add tests for:
  - snapped target endpoint;
  - local-route anchor merge proof;
  - same-net branch copper merge proof;
  - wrong-net contact rejection;
  - wrong-layer contact rejection.

### Acceptance

- Contact proof uses access-driven branch endpoints.
- A route that physically reaches a local-route anchor proves graph contact.
- Invalid contacts remain blocked.
- `go test ./internal/designworkflow -run 'ContactGraph|AccessDriven|InterBlockContact|RouteTree' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Snap access-driven route-tree contacts`.

## Phase 6: I2C Fixture Run And Metadata Update

### Goals

- Run the actual I2C fixture and update promotion evidence based on reality.

### Tasks

- Run:

  ```sh
  kicadai --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json \
    --output examples/.generated/i2c_sensor_breakout_candidate \
    --overwrite \
    design create
  ```

- Inspect routing summaries, workflow result, promotion report, and issues.
- Compare against Phase 1 baseline:
  - proven endpoints;
  - complete/partial/blocked contact graph groups;
  - branch executor routed/blocked branches;
  - route-tree repair hints;
  - selected retry reason.
- Promote fixture readiness only if all declared gates pass.
- Otherwise update metadata and docs with narrower blockers.

### Acceptance

- Fixture metadata matches actual generated evidence.
- If still `expected_fail`, blockers are narrower or clearly justified.
- `go test ./internal/designworkflow -run 'I2CSensorBreakout|RouteTree|Promotion' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Update access-driven I2C route evidence`.

## Phase 7: Documentation And Next-Step Guidance

### Goals

- Explain access-driven route-tree behavior to AI callers and future repair
  work.

### Tasks

- Update routing docs with:
  - access candidate ranking;
  - multi-candidate branch attempts;
  - endpoint snapping;
  - graph proof semantics;
  - remaining limitations.
- Update README and roadmap with final fixture status.
- Ensure examples use `kicadai`, not `go run ./cmd/kicadai`.

### Acceptance

- Docs distinguish access evidence, branch executor evidence, contact graph
  evidence, and KiCad ERC/DRC evidence.
- README and roadmap match fixture metadata.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document access-driven route-tree routing`.

## Final Verification

- Run `go test ./...`.
- Run the I2C fixture and inspect generated promotion evidence.
- Run optional KiCad-backed tests if local KiCad CLI is configured and needed
  for a promotion claim.
- Confirm `git status --short` is clean after the final commit.

## Expected Outcome

Best case: the I2C fixture reaches full internal route-tree contact proof and
can proceed to project write, writer correctness, validation, and KiCad checks.

Acceptable intermediate case: the fixture remains `expected_fail`, but the
remaining blocker is narrower than today, such as a specific DRC-grade
neckdown, layer transition, or clearance problem that access-driven routing
cannot legally solve.
