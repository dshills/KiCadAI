# VCC Route-Tree Path Completion Implementation Plan

Date: 2026-07-02

## Objective

Close the next concrete routing blocker after route-tree VCC proof closeout:
make the I2C fixture's remaining VCC route-tree branch complete when legal, or
produce narrower same-net merge and obstacle evidence for the next repair loop.

## Phase 1: Lock Current VCC Failure Evidence

### Goals

- Freeze the current VCC-only failure boundary.
- Prevent regression back to broad SDA/GND route-tree blockers.
- Make the selected VCC access attempts easy to inspect in tests.

### Tasks

- Add or extend tests that extract VCC branch evidence from the routing stage:
  - branch index;
  - selected source/target roles;
  - source/target refs, pads, layers, and coordinates;
  - access pair count, limit, and truncation;
  - access attempt primary issue code/message;
  - contact graph status.
- Assert route-tree repair hints contain only VCC for the I2C fixture.
- Assert fixed-net skip notices and missing-net-class warnings do not inflate
  `route_tree_repair.branch_failures`.
- Add a small helper to extract branch evidence by managed net and branch
  index.
- Preserve current contact graph baseline:
  - 12 required endpoints;
  - 11 proven endpoints;
  - 3 complete groups;
  - 1 partial group.

### Acceptance

- `go test ./internal/designworkflow -run 'VCC|RouteTreeBranch|I2C' -count=1`
  passes.
- Tests fail if SDA/GND regain selected-attempt branch blockers.
- Tests fail if VCC access attempt evidence disappears.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Lock VCC route-tree path blocker evidence`.

## Phase 2: Same-Net Merge Audit

### Goals

- Determine whether the remaining VCC route can legally merge with existing
  same-net geometry.
- Identify the exact rejection reason when a merge candidate exists.

### Tasks

- Add routing/designworkflow diagnostics for selected VCC branch attempts:
  - same-net pad merge candidate count;
  - same-net local-route anchor candidate count;
  - same-net existing branch copper candidate count;
  - nearest other-net obstacle kind/ref/net;
  - layer/via compatibility status;
  - first blocked grid step or search exhaustion indicator where available.
- Add small routing unit tests for:
  - same-net pad terminal contact;
  - same-net copper merge;
  - other-net pad remains blocking;
  - same-net candidate rejected by layer/via policy.
- Add I2C test assertions that VCC failure evidence contains merge/obstacle
  audit fields.

### Acceptance

- Same-net merge candidates are counted separately from blockers.
- VCC failure evidence identifies whether the blocker is merge legality,
  obstacle clearance, layer/via policy, or search exhaustion.
- `go test ./internal/routing ./internal/designworkflow -run 'SameNet|Merge|VCC|RouteTree' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Audit VCC same-net merge blockers`.

## Phase 3: Access Candidate Ranking Refinement

### Goals

- Prefer VCC candidate pairs that are physically more likely to route.
- Avoid raising candidate limits unless evidence proves truncation excludes a
  better legal pair.

### Tasks

- Add candidate scoring inputs for:
  - immediate obstacle pressure;
  - same-layer compatibility;
  - via/layer policy compatibility;
  - local-route anchor distance benefit;
  - blocked first/last grid step.
- Keep deterministic tie-breaks by role, ref, pad, layer, coordinates, and
  source.
- Add tests showing:
  - local-route anchors still win when they legalize or shorten the path;
  - pad access wins when a local-route anchor is immediately blocked;
  - candidate order is stable;
  - candidate limit remains justified for VCC.
- Update branch evidence with ranking factors only if needed for reviewability.

### Acceptance

- VCC branch selected pair is justified by deterministic ranking evidence.
- No broader routing behavior regresses.
- `go test ./internal/designworkflow -run 'AccessCandidate|Ranking|VCC|RouteTreeBranch' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Refine VCC route-tree access ranking`.

## Phase 4: Same-Net Merge Execution

### Goals

- Let VCC branches terminate at or merge into legal same-net geometry.
- Keep other-net and unsupported geometry conservative.

### Tasks

- Update routing occupancy/search semantics so legal same-net VCC pads/copper
  can serve as terminal or merge targets.
- Ensure other-net pads/copper, keepouts, board edges, and unsupported zones
  remain blockers.
- Record route result evidence for:
  - target pad contact;
  - local-route anchor merge;
  - existing same-net branch copper merge;
  - failed merge attempt.
- Ensure failed attempts do not emit partial copper.
- Add unit tests for same-net merge execution and other-net blocking.
- Add route-tree branch tests proving a later branch can merge into earlier
  successful same-net branch copper.

### Acceptance

- Same-net VCC merge is legal only when geometry and layer policy allow it.
- Failed branches still leave no partial copper.
- `go test ./internal/routing ./internal/designworkflow -run 'SameNet|Merge|RouteTreeBranch|PartialCopper' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Route VCC branches into same-net merge targets`.

## Phase 5: Contact Graph Completion Recheck

### Goals

- Verify that any new VCC merge/contact evidence contributes to route-tree
  graph completion.
- Keep graph-split reporting when the graph still remains split.

### Tasks

- Extend contact graph tests for:
  - branch-to-local-route merge;
  - branch-to-same-net-copper merge;
  - branch-to-pad contact;
  - wrong-net/wrong-layer false contacts.
- Update I2C assertions according to actual results:
  - if VCC completes, expect 12/12 proven endpoints and 4 complete groups;
  - if VCC still blocks, expect narrower blocker evidence than Phase 1.
- Confirm repair hints match the graph result.

### Acceptance

- Contact graph proof matches physical same-net connectivity.
- Invalid contacts remain rejected.
- `go test ./internal/designworkflow -run 'ContactGraph|VCC|RouteTreeRepair|I2C' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Recheck VCC route-tree contact completion`.

## Phase 6: Fixture Rerun And Promotion Decision

### Goals

- Run the real KiCad-backed I2C fixture and update tracked evidence honestly.

### Tasks

- Rebuild `bin/kicadai`.
- Run:

  ```sh
  kicadai \
    --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json \
    --output examples/.generated/i2c_sensor_breakout_candidate \
    --overwrite \
    design create
  ```

- Inspect:
  - routing stage summary;
  - route-tree branch evidence;
  - route-tree contact graph;
  - route-tree repair summary;
  - `.kicadai/design-promotion.json`.
- If VCC reaches 12/12 proof and all route groups are complete, inspect the
  next promotion gates before changing readiness.
- If the fixture remains `expected_fail`, update metadata and docs with the
  exact remaining blocker.

### Acceptance

- Fixture metadata matches actual generated evidence.
- No promotion occurs without supporting gates.
- `go test ./internal/designworkflow -run 'I2CSensorBreakout|RouteTree|Promotion' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Update I2C VCC route completion evidence`.

## Phase 7: Full Regression And Documentation

### Goals

- Keep the routing closeout understandable for future AI repair work.

### Tasks

- Update:
  - `README.md`;
  - `docs/layout-routing.md`;
  - `specs/ROADMAP.md`;
  - this plan status.
- Document:
  - whether VCC is graph-complete;
  - remaining route-tree or KiCad blockers;
  - how to interpret same-net merge evidence;
  - next follow-up if promotion remains blocked.
- Run full regression.

### Acceptance

- `go test ./...` passes.
- Prism staged review has no high or medium findings.
- Worktree is clean after commit.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document VCC route completion status`.

