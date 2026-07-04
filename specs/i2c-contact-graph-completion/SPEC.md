# I2C Contact Graph Completion Specification

Date: 2026-07-04

## Summary

Close the remaining `i2c_sensor_breakout_candidate` routing blocker by making
route-tree contact proof complete for the VCC/GND/SDA/SCL generated inter-block
nets where the emitted copper is electrically valid.

The fixture currently has these important properties:

- all 8 route-tree branches emit;
- VCC/GND/SDA/SCL are route-tree-managed and excluded from fallback net routing;
- route-tree endpoint access exists for every required endpoint;
- same-net segment-interior contact is recognized;
- completion is based on same-net contact graph evidence, not route operation
  counts or warning-level route-tree notices;
- one route-tree net is graph-complete;
- three route-tree nets are partial;
- 9 of 12 required endpoint contacts are proven;
- the fixture remains `expected_fail` because the three partial route-tree
  groups still lack complete same-net contact graph proof.

This project must move the fixture from "branches emit but graph proof remains
partial" toward "all required route-tree endpoints are in one same-net graph
component per net." Promotion to `candidate` is allowed only if the generated
project also reaches the declared internal and KiCad-backed evidence gates.

## Goals

1. Identify the exact missing endpoint contacts and graph components for the
   three partial I2C route-tree nets.
2. Make same-net graph completion account for every legal contact surface used
   by generated route-tree copper:
   - pad center contacts;
   - segment endpoint contacts;
   - segment-interior contacts;
   - generated local-route anchors;
   - generated same-net route-tree copper merge points;
   - via/layer transitions when modeled by emitted operations.
3. Improve diagnostics so partial groups report actionable missing endpoints,
   split components, contributing branch IDs, and access points.
4. Promote `i2c_sensor_breakout_candidate` only if the full route-tree,
   writer, validation, and optional KiCad gates support promotion.
5. Preserve LED and connector/LED candidate behavior.

## Non-Goals

- Do not add a general autorouter.
- Do not claim KiCad ERC/DRC pass without configured `kicad-cli` evidence.
- Do not silence contact graph failures to force promotion.
- Do not broaden component catalogs or circuit block functionality.
- Do not change KiCad file writers except where generated evidence exposes a
  writer correctness defect.
- Do not mutate imported projects.

## Current Evidence Contract

The current baseline is captured by workflow tests and fixture metadata.
Expected current I2C evidence:

- `inter_block_routing.multi_endpoint_nets == 4`
- `inter_block_routing.required_endpoints == 12`
- `inter_block_routing.proven_endpoints == 9`
- `inter_block_routing.branches_attempted == 8`
- `inter_block_routing.routes_completed == 1`
- `inter_block_routing.partial_nets == 3`
- `inter_block_routing.complete_groups == 1`
- `inter_block_routing.partial_groups == 3`
- `route_tree_execution.groups_complete == 4`
- `route_tree_execution.branches_routed == 8`
- `route_tree_contact_graph.required_endpoints == 12`
- `route_tree_contact_graph.proven_endpoints == 9`
- `route_tree_contact_graph.complete_groups == 1`
- `route_tree_contact_graph.partial_groups == 3`

These numbers are not the target end state. They are the starting point and
must move only with explicit evidence.

## Desired End State

For each I2C route-tree managed net:

- every required endpoint resolves to a physical contact target;
- every selected access point is represented in graph evidence;
- emitted copper contributes graph nodes and edges for compatible same-net
  geometry;
- all required endpoints for that net are connected through one same-net graph
  component;
- non-blocking route-tree warnings remain diagnostic but do not prevent graph
  completion;
- blocking issues remain blocking and name exact endpoints/components.

If all four route-tree nets complete:

- the routing stage should not block on route-tree contact proof;
- project write, writer correctness, validation, and KiCad checks should run
  according to request options and fixture metadata;
- the fixture may be promoted from `expected_fail` to `candidate` only if
  metadata gates and KiCad policy are satisfied.

If any nets remain partial:

- the fixture stays `expected_fail`;
- metadata must name the narrower blockers;
- tests must lock the remaining blocker shape.

## Contact Graph Model

The graph must model physical same-net electrical connectivity, not just route
operation endpoints.

### Nodes

The graph may include:

- required endpoint pads from `InterBlockContactTarget`;
- selected source/target access points from route-tree branch evidence;
- route operation vertices;
- projected points where a target/access lies on a route segment;
- generated local-route anchors from block-local route operations;
- generated same-net copper access points;
- vias, when route operations expose them.

### Edges

The graph may connect:

- adjacent vertices in the same route operation on the same layer;
- a target/access point to a same-layer segment when the perpendicular distance
  is within contact tolerance;
- overlapping/touching same-net segment geometry on the same layer;
- local-route anchor points to generated route-tree segments;
- same-net copper merge points to route-tree branch segments;
- via/contact transitions between layers when emitted operations provide
  explicit via evidence.

### Rejections

The graph must reject:

- wrong-net contacts;
- wrong-layer contacts without via evidence;
- contacts outside tolerance;
- unsupported geometry under strict evidence;
- ambiguous contact when multiple incompatible net/layer surfaces are present.

## Diagnostics

Partial or blocked route-tree groups must report:

- net name;
- required endpoint count;
- proven endpoint count;
- component count;
- missing endpoint IDs;
- endpoint refs/pads;
- graph component IDs or stable component indexes;
- contributing operation IDs and branch indexes where available;
- blocker issue codes;
- repair suggestions.

The `route_tree_contact_graph` summary should remain compact, while deeper
detail can be emitted in a separate summary field or branch/contact evidence
structure.

## Promotion Policy

The fixture must remain `expected_fail` unless all required gates support
promotion.

Promotion to `candidate` requires:

- all route-tree managed I2C nets graph-complete;
- routing stage not blocked;
- expected workflow stages reached;
- writer correctness and validation gates reached and not blocked;
- KiCad ERC/DRC evidence present when metadata requires it, or metadata adjusted
  to accurately represent optional KiCad policy.

Promotion to `pass` requires:

- candidate criteria;
- required KiCad checks clean;
- no known gaps requiring expected-fail status.

## Test Requirements

Add or update tests for:

- exact missing endpoint/component diagnostics for the current I2C partial nets;
- same-net overlapping segment merge;
- same-net local-route anchor to route-tree segment merge;
- same-net route-tree branch-to-branch segment intersection merge;
- wrong-net overlap rejection;
- wrong-layer overlap rejection without via evidence;
- via/layer transition behavior where supported;
- I2C route-tree completion movement;
- LED strict prompt and connector/LED candidate non-regression;
- fixture metadata matching actual promotion evidence.

## Acceptance Criteria

- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|RouteTree|ContactGraph|InterBlockContact|Promotion|DesignExamples|ConnectorLED|LEDPrompt' -count=1
  ```

- Full test suite passes:

  ```sh
  go test ./...
  ```

- `prism review staged` reports no high or medium findings, or any remaining
  high/medium finding is demonstrably false with test evidence.

- `examples/design/kicad-backed/i2c_sensor_breakout_candidate.metadata.json`
  reflects actual evidence after the implementation.

