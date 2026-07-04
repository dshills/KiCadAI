# I2C Connector Endpoint Proof Implementation Plan

Date: 2026-07-04

## Objective

Close or narrowly diagnose the remaining `i2c_sensor_breakout_candidate`
route-tree contact graph gaps for GND (`io.2`), SDA (`io.3`), and SCL
(`io.4`).

Implement in phases. After each implementation phase:

1. run focused tests for that phase;
2. run `go test ./...` when production behavior changes;
3. stage only that phase;
4. run `prism review staged`;
5. fix high/medium findings;
6. commit before moving to the next phase.

## Phase 1: Missing Connector Endpoint Geometry Baseline

### Goals

- Freeze the current GND/SDA/SCL missing endpoint shape.
- Capture exact geometry between each missing connector pad and the nearest
  emitted same-net copper.

### Tasks

- Add focused I2C test helpers that extract, for each missing endpoint:
  - net name;
  - endpoint ID;
  - ref, pad, instance ID, block ID;
  - target coordinate and layer;
  - nearest same-net route operation ID;
  - nearest same-net segment endpoint or projection point;
  - distance to nearest same-net copper;
  - closest copper layer;
  - contact tolerance;
  - route operation IDs and net-level evidence that mention the endpoint gap.
- Prefer production summary additions if existing stage summaries cannot expose
  the needed geometry without ad hoc parsing.
- Keep existing evidence counts unchanged in this phase.

### Acceptance

- Tests prove the current missing endpoints are exactly:
  - `GND`: `io.2`;
  - `SDA`: `io.3`;
  - `SCL`: `io.4`.
- Tests include closest-copper geometry for all three missing endpoints.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'I2C|ConnectorEndpoint|ContactGraph|RouteTree' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Capture I2C connector endpoint proof gaps`.

## Phase 2: Connector Endpoint Branch Selection Evidence

### Goals

- Determine whether branch planning selects the unresolved connector endpoints
  or routes to other access points first.
- Make branch/access selection evidence explicit enough to explain misses.

### Tasks

- Inspect route-tree branch planning and selected access pair summaries.
- Add or extend branch evidence with:
  - required endpoint ID;
  - selected source/target access endpoint ID, role, layer, coordinate;
  - whether selected access already belongs to the proven graph component;
  - reason for selecting local-route anchor, same-net copper, or pad access.
- Add tests that assert GND/SDA/SCL missing connector endpoints appear in
  branch/access candidate evidence.
- If selection incorrectly deprioritizes missing connector pad access, adjust
  ranking so unresolved required endpoints are preferred over already-proven
  same-net access for the branch that must close the endpoint.

### Acceptance

- Branch evidence explains why each missing connector endpoint is or is not
  targeted.
- Ranking changes, if any, are deterministic and do not regress VCC.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'RouteTree|EndpointAccess|I2C|ConnectorEndpoint' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Expose connector endpoint branch selection`.

## Phase 3: Connector Pad Termination Routing

### Goals

- Emit valid route-tree branch geometry that reaches unresolved connector pads
  when a route is legal.
- Keep wrong-net and wrong-layer behavior conservative.

### Tasks

- Use Phase 1 and 2 evidence to identify whether the missing endpoints need:
  - branch endpoint selection changes;
  - route endpoint snapping to connector pad center;
  - pad-outline or through-hole access modeling;
  - route tree branch ordering changes;
  - local-route merge repair.
- Implement the smallest deterministic routing change that makes legal
  connector pad termination possible.
- Add tests for:
  - a synthetic route-tree branch ending at a connector pad center;
  - a connector endpoint reached through same-net route-tree branch copper;
  - wrong-net connector pad rejection;
  - wrong-layer connector pad rejection without via evidence.

### Acceptance

- Valid connector pad route termination is graph-proven.
- Invalid connector pad contact does not mask blockers.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'ConnectorEndpoint|ContactGraph|RouteTree|InterBlockContact' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Route missing I2C connector endpoints`.

## Phase 4: I2C Fixture Completion Attempt

### Goals

- Re-run the actual I2C fixture after connector endpoint routing changes.
- Promote only if real evidence supports it.

### Tasks

- Run the I2C workflow and compare against Phase 1:
  - proven endpoints;
  - complete/partial groups;
  - missing endpoint IDs;
  - reached workflow stages;
  - route/contact issue codes;
  - promotion gates.
- If `route_tree_contact_graph.proven_endpoints == 12` and
  `complete_groups == 4`, inspect downstream writer/validation/KiCad evidence.
- If downstream gates block promotion, keep `expected_fail` and update metadata
  to distinguish downstream gates from internal contact graph blockers.
- If the contact graph still blocks, update metadata with the narrower
  connector endpoint cause.

### Acceptance

- Metadata matches fresh evidence.
- Tests lock either:
  - 12/12 internal route-tree contact proof; or
  - exact remaining connector endpoint blockers.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Update I2C connector endpoint promotion evidence`.

## Phase 5: Candidate Fixture Non-Regression

### Goals

- Ensure the connector endpoint repair does not regress existing candidate
  fixtures or route graph semantics.

### Tasks

- Run focused non-regression tests for:
  - LED prompt;
  - connector/LED KiCad-backed smoke fixture;
  - route-tree contact graph unit tests;
  - inter-block contact proof tests.
- Add a regression test if any fixture behavior changes.
- Verify stage summaries remain backward-compatible JSON.

### Acceptance

- LED and connector/LED candidate behavior remains stable.
- Existing route graph semantics still reject wrong-net/wrong-layer contact.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'LEDPrompt|ConnectorLED|ContactGraph|InterBlockContact|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Guard connector endpoint routing regressions`.

## Phase 6: Documentation And Roadmap Update

### Goals

- Keep docs aligned with the new I2C state.
- Make the next blocker obvious to AI agents and humans.

### Tasks

- Update:
  - `README.md`;
  - `docs/layout-routing.md`;
  - `examples/design/kicad-backed/README.md`;
  - `specs/ROADMAP.md`;
  - `examples/design/kicad-backed/i2c_sensor_breakout_candidate.metadata.json`
    if not already updated in Phase 4.
- Remove stale references to previous blockers.
- If the fixture promotes, document which gates remain before `pass`.
- If the fixture stays `expected_fail`, document the exact narrower blocker.

### Acceptance

- Docs match fresh test evidence.
- No stale references to obsolete GND/SDA/SCL endpoint blockers remain if they
  are fixed.
- Full tests pass:

  ```sh
  go test ./...
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Document I2C connector endpoint proof status`.

## Phase 7: Full Regression

### Goals

- Confirm the repo is stable after the connector endpoint proof work.

### Tasks

- Run:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|RouteTree|ContactGraph|InterBlockContact|Promotion|DesignExamples|ConnectorLED|LEDPrompt' -count=1
  go test ./...
  ```

- Check `git status --short`.

### Acceptance

- All tests pass.
- Worktree is clean except intentional follow-up specs.
