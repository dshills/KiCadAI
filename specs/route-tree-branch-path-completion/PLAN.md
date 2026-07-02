# Route-Tree Branch Path Completion Implementation Plan

Date: 2026-07-02

## Objective

Complete the next routing step after route-tree repair hints: make
route-tree-managed branches connect through valid same-net pads, local-route
anchors, and existing same-net copper so the first KiCad-backed generated
design fixture can move from `expected_fail` toward `candidate` or `pass`.

The target fixture is `i2c_sensor_breakout_candidate`, whose latest selected
attempt still blocks on GND/SDA branch path/contact proof gaps.

## Phase 1: Baseline And Evidence Fixtures

### Goals

- Freeze the current selected-attempt route-tree failure shape.
- Make improvements measurable before changing routing behavior.
- Confirm LED and connector/LED candidate fixtures remain protected from
  regression.

### Tasks

- Add or extend focused I2C fixture tests that extract selected-attempt
  route-tree metrics from workflow summaries.
- Record current counts for:
  - managed nets;
  - groups complete/partial/blocked;
  - branches attempted/routed/blocked;
  - required/proven endpoints;
  - contact misses;
  - route-tree repair hint count;
  - promotion blocker codes.
- Add assertions that blockers are branch/contact scoped, not generic routing
  skips.
- Add regression assertions for LED and connector/LED candidate fixtures:
  - no new blocking route-tree failures;
  - local-route contact proof remains present;
  - promotion metadata remains internally consistent.
- Keep numeric assertions loose enough to allow deterministic improvement but
  strict enough to catch loss of route-tree ownership or contact proof.

### Acceptance

- The current I2C failure is reproducible without KiCad.
- Tests fail if route-tree-managed nets fall back to generic net routing.
- Tests fail if contact proof is silently dropped.
- `go test ./internal/designworkflow -run 'I2C|RouteTree|ConnectorLED|LED' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Capture route-tree path completion baseline`.

## Phase 2: Same-Net Occupancy Semantics

### Goals

- Teach the router to distinguish same-net conductive geometry from blockers.
- Keep other-net and keepout behavior conservative.

### Tasks

- Audit `internal/routing/occupancy.go`, `pads.go`, `search.go`, and related
  route validation code.
- Add a same-net contact/merge policy for:
  - same-net pads as route terminals;
  - same-net pads as legal pass-through/merge candidates when layer access is
    allowed;
  - same-net generated copper as legal merge candidates;
  - local-route same-net copper when ownership and net evidence are proven.
- Keep other-net pads/copper as obstacles.
- Keep keepouts, edge constraints, unsupported zones, and clearance rules as
  blockers.
- Expose route report evidence for terminal contact versus same-net merge.
- Add routing unit tests for same-net pad/copper merge and other-net blocking.

### Acceptance

- Same-net pads and same-net generated copper no longer block legal branch
  completion.
- Other-net pads still produce clearance/search failures.
- Existing routing golden tests remain stable.
- `go test ./internal/routing -run 'SameNet|Occupancy|Route|Pad' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Allow route branches to merge with same-net geometry`.

## Phase 3: Endpoint Access Candidate Model

### Goals

- Give route-tree branches deterministic physical access points instead of
  abstract endpoints.
- Prefer proven local-route anchors when they are better branch terminals.

### Tasks

- Add internal endpoint access structs in `designworkflow` or `routing` as
  appropriate.
- Build access candidates from:
  - hydrated footprint pads;
  - pad layer/access data;
  - generated block-local route endpoints;
  - existing same-net branch copper;
  - external anchors where already supported.
- Include ref, pad, net, layer, coordinates, source, and endpoint role.
- Implement deterministic access ranking:
  - same-net local-route anchor;
  - nearest compatible pad access;
  - layer-compatible low-collision access;
  - stable ref/pad/source tie-breakers.
- Add tests for candidate extraction from I2C local routes and hydrated pads.
- Add tests for deterministic tie-breaking and missing/ambiguous access
  diagnostics.

### Acceptance

- I2C VCC/GND/SDA/SCL endpoints expose physical access candidates.
- Local-route anchors are visible as branch access candidates where the block
  has already proven same-net local routing.
- Missing access produces structured route-tree issues.
- `go test ./internal/designworkflow -run 'EndpointAccess|RouteTree|PadHydration|LocalRoute' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Resolve route-tree endpoint access candidates`.

## Phase 4: Contact Graph Completion

### Goals

- Measure route-tree completion by physical same-net graph connectivity.
- Avoid counting a branch complete unless it actually connects required
  endpoints.

### Tasks

- Build a route-tree contact graph for each managed net.
- Add nodes for required endpoints, selected access points, route segments,
  vias, pads, local-route anchors, and same-net merge points.
- Add edges for geometric contact on compatible layers and legal through-hole
  or via transitions.
- Compute complete/partial/blocked group status from graph components.
- Report missing endpoints and split components as structured issues.
- Add tolerance for writer/readback coordinate rounding.
- Add tests for:
  - all endpoints in one component -> complete;
  - split graph -> partial or blocked;
  - same-net local-route merge -> proven endpoint;
  - false contact on different net/layer -> rejected.

### Acceptance

- Route-tree summary completion is graph-derived.
- Contact misses decrease when valid same-net merge/contact geometry exists.
- Invalid same-location different-net contacts are rejected.
- Existing inter-block contact tests remain stable.
- `go test ./internal/designworkflow -run 'ContactGraph|InterBlockContact|RouteTree' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Compute route-tree completion from contact graphs`.

## Phase 5: Branch Ordering And Clean Reroute

### Goals

- Improve completion by routing constrained branches first.
- Prevent failed partial branches from poisoning later branch search.

### Tasks

- Add deterministic branch difficulty scoring using:
  - endpoint access count;
  - route distance;
  - local-route anchor availability;
  - immediate obstacle pressure if already available;
  - layer/via constraints.
- Route constrained branches before easier branches where this improves graph
  completion.
- Allow later branches to merge with earlier successful same-net branch copper.
- Ensure failed branch operations are discarded before the next branch.
- Ensure retry attempts rebuild route-tree-managed copper from a clean
  generated transaction state.
- Add tests for:
  - constrained branch ordered first;
  - failed branch leaves no partial operations;
  - successful same-net branch becomes a merge target;
  - repeated retry attempts do not accumulate stale route-tree copper.

### Acceptance

- Branch ordering is deterministic.
- Failed branch attempts do not emit partial copper.
- Same-net merge into earlier branch copper is tested and reported.
- `go test ./internal/designworkflow -run 'RouteTreeBranch|Reroute|Retry' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Improve route-tree branch ordering and reroute cleanup`.

## Phase 6: I2C Fixture Run And Promotion Update

### Goals

- Run the actual generated fixture.
- Promote only if the evidence supports it.
- Document remaining blockers precisely if it still fails.

### Tasks

- Run:

  ```sh
  kicadai design create \
    --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json \
    --output examples/.generated/i2c_sensor_breakout_candidate \
    --overwrite
  ```

- Inspect `.kicadai/workflow-result.json`,
  `.kicadai/design-promotion.json`, routing summaries, board validation, writer
  correctness, and optional KiCad evidence if configured.
- Compare route-tree metrics against the Phase 1 baseline.
- If all declared gates support it, update fixture metadata readiness from
  `expected_fail` to `candidate` or `pass`.
- If not, keep `expected_fail` but update metadata with narrowed blockers,
  selected-attempt evidence, and next repair guidance.
- Update `examples/design/kicad-backed/README.md`, `README.md`, and
  `specs/ROADMAP.md`.

### Acceptance

- The fixture metadata matches actual generated evidence.
- The promotion report distinguishes internal route-tree blockers from KiCad
  ERC/DRC or fabrication blockers.
- Route-tree branch/contact metrics are no worse than baseline unless the new
  blockers reveal a stricter correctness issue.
- `go test ./internal/designworkflow -run 'I2CSensorBreakout|RouteTree|Promotion' -count=1`
  passes.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Update I2C route-tree promotion evidence`.

## Phase 7: Documentation And AI-Facing Guidance

### Goals

- Make the new route-tree completion behavior understandable to AI callers and
  future repair work.

### Tasks

- Update routing/placement docs with:
  - same-net merge semantics;
  - endpoint access candidate rules;
  - contact graph completion definitions;
  - route-tree summary fields;
  - known remaining limitations.
- Update AI skill/docs if they describe promotion or routing evidence.
- Add a short troubleshooting section for route-tree blockers:
  - missing endpoint access;
  - split same-net graph;
  - same-net merge unavailable;
  - other-net obstacle;
  - KiCad DRC blocker after internal completion.
- Ensure all command examples use `kicadai`, not `go run ./cmd/kicadai`.

### Acceptance

- Docs describe how an AI caller should interpret route-tree completion and
  repair blockers.
- README and roadmap agree with fixture state.
- No stale references claim the I2C fixture is blocked by already-fixed
  pad/copper net-code or routing-skip issues.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document route-tree path completion evidence`.

## Final Verification

After all phases:

- run `go test ./...`;
- run the I2C generated fixture;
- run optional KiCad-backed tests if local KiCad CLI is configured and policy
  requires it;
- run `prism review staged` for any remaining staged docs or metadata;
- confirm `git status --short` is clean after the final commit.

## Expected Outcome

Best case: `i2c_sensor_breakout_candidate` promotes to `candidate` with
route-tree-managed VCC/GND/SDA/SCL completion and remaining evidence limited to
warning-level or optional KiCad checks.

Acceptable intermediate case: the fixture remains `expected_fail`, but the
remaining blockers are narrower than today and identify a specific next project
such as DRC-grade neckdown, through-hole zero-cost transitions, or exact KiCad
zone/copper handling.

