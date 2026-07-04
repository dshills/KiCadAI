# I2C Connector Endpoint Proof Specification

Date: 2026-07-04

## Summary

Complete the next narrow blocker for
`examples/design/kicad-backed/i2c_sensor_breakout_candidate`: prove the
remaining connector-side endpoints for the route-tree-managed I2C nets.

The previous I2C contact graph completion work added graph semantics for:

- per-net contact graph detail;
- same-net segment intersection and overlap merges;
- local-route anchor participation;
- explicit via layer transitions;
- exact missing endpoint reporting.

After those changes, the fixture still remains `expected_fail` with a precise
internal blocker:

- `GND` proves 2 of 3 endpoints and misses connector endpoint `io.2`;
- `SDA` proves 2 of 3 endpoints and misses connector endpoint `io.3`;
- `SCL` proves 2 of 3 endpoints and misses connector endpoint `io.4`;
- `VCC` is graph-complete;
- all 8 route-tree branches emit;
- branch pathfinding no longer reports selected-attempt branch blockers for
  the current baseline.

This project must close or narrowly diagnose why emitted route-tree/local-route
copper does not join the three connector pads into their same-net contact graph
components.

## Goals

1. Identify the physical geometry gap between each missing connector pad and
   the emitted same-net route graph.
2. Ensure route-tree endpoint access and branch planning prioritize required
   connector endpoints that are still missing from the contact graph.
3. Emit deterministic connector-side branch or merge geometry when a required
   endpoint is not yet in the same-net component.
4. Preserve conservative correctness:
   - no wrong-net joins;
   - no implicit wrong-layer joins without via evidence;
   - no promotion unless downstream writer, validation, and KiCad policy gates
     support it.
5. Keep LED and connector/LED candidate behavior stable.

## Non-Goals

- Do not build a general autorouter.
- Do not loosen contact tolerance to hide routing defects.
- Do not bypass the same-net contact graph proof.
- Do not claim KiCad ERC/DRC success without real configured KiCad evidence.
- Do not change component selection, circuit block catalogs, or schematic
  generation except where a routing evidence defect proves they are involved.
- Do not mutate imported KiCad projects.

## Current Evidence Contract

The current I2C fixture evidence must remain locked until this project moves it
with explicit tests:

- route-tree-managed nets: `VCC`, `GND`, `SDA`, `SCL`;
- route-tree branches emitted: 8;
- `route_tree_contact_graph.required_endpoints == 12`;
- `route_tree_contact_graph.proven_endpoints == 9`;
- `route_tree_contact_graph.complete_groups == 1`;
- `route_tree_contact_graph.partial_groups == 3`;
- missing endpoint IDs:
  - `GND`: `io.2`;
  - `SDA`: `io.3`;
  - `SCL`: `io.4`;
- selected-attempt branch pathfinding blockers are absent for the current
  baseline.

If any of these counts change, tests and metadata must explain why.

## Desired End State

Best case:

- `GND`, `SDA`, and `SCL` connector endpoints join their same-net graph
  components;
- all four route-tree-managed nets are graph-complete;
- `route_tree_contact_graph.proven_endpoints == 12`;
- `route_tree_contact_graph.complete_groups == 4`;
- routing is no longer blocked by internal route-tree contact graph proof;
- the fixture advances to writer correctness, validation, and configured KiCad
  gates;
- metadata promotes from `expected_fail` only if all required gates support it.

Acceptable intermediate case:

- the fixture remains `expected_fail`;
- the remaining blockers are narrower than "connector endpoint missing";
- metadata names exact geometry, rule, layer, pad-shape, or router limitation
  causes;
- diagnostics include enough evidence for the next repair project.

## Connector Endpoint Proof Model

For every route-tree-managed net, required endpoint pads should have an
explicit proof path:

1. resolved endpoint target: instance, ref, pad, net, layer, point, tolerance;
2. selected endpoint access candidates: pad center, local route anchor,
   same-net copper, or via-backed access;
3. branch route attempts targeting the unresolved endpoint or a valid access
   candidate;
4. emitted copper operation IDs and branch indexes that should contact the pad;
5. contact graph component result after emitted copper is added;
6. failure category if the endpoint remains missing.

The branch/router must be allowed to choose a route that terminates at a
connector pad center or another contact-valid point on the connector pad when
that endpoint is not graph-proven.

## Required Diagnostics

For each missing endpoint, expose enough detail in tests and summaries to
answer:

- Which required endpoint is missing?
- What are its ref, pad, instance, footprint, layer, and coordinate?
- Which same-net route operations are closest?
- What is the closest distance to emitted same-net copper?
- Is the closest copper on the same layer?
- Did a branch route explicitly target the endpoint?
- If the branch targeted a different access point, why was it ranked higher?
- Was the route emitted but outside contact tolerance?
- Was the route suppressed because the branch was considered already covered?
- Did fixed-net preservation, obstacle avoidance, net-class policy, or
  placement retry affect the endpoint?

## Implementation Constraints

- Route-tree branch ordering must stay deterministic.
- Retry attempts must not accumulate stale route-tree copper.
- Existing candidate fixtures must not regress.
- Contact graph proof remains the source of truth for route completion.
- New summaries must be backward-compatible JSON additions.
- Generated operation IDs and issue paths must remain stable enough for tests
  and AI repair loops.

## Promotion Policy

Keep `i2c_sensor_breakout_candidate` as `expected_fail` unless all required
gates support promotion.

Promotion to `candidate` requires:

- all four I2C route-tree groups graph-complete;
- routing stage not blocked by internal route/contact evidence;
- expected workflow stages reached;
- writer correctness and board validation gates reached and clean enough for
  candidate policy;
- KiCad ERC/DRC evidence present when metadata requires it, or metadata updated
  to honestly represent optional KiCad policy.

Promotion to `pass` requires candidate criteria plus required clean KiCad
ERC/DRC and round-trip evidence.

## Test Requirements

Add or update tests for:

- current missing connector endpoint baseline;
- per-missing-endpoint nearest same-net copper diagnostics;
- route-tree branch selection targeting unresolved connector endpoints;
- connector pad center branch termination contact proof;
- contact graph completion after connector endpoint repair;
- no regression for VCC complete group;
- no regression for LED prompt and connector/LED candidate fixtures;
- wrong-net and wrong-layer connector endpoint rejection;
- metadata matching actual promotion evidence.

## Acceptance Criteria

- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|RouteTree|ContactGraph|InterBlockContact|Promotion|DesignExamples|ConnectorLED|LEDPrompt' -count=1
  ```

- Full suite passes:

  ```sh
  go test ./...
  ```

- `prism review staged` reports no high or medium findings, or any remaining
  high/medium finding is demonstrably false with test evidence.

- `examples/design/kicad-backed/i2c_sensor_breakout_candidate.metadata.json`
  accurately describes final status.

