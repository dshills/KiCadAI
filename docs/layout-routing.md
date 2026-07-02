# Placement And Routing

Placement engine, routing engine, and related quality evidence.

### Placement

The placement engine accepts a structured board placement request and returns
placed components, geometry issues, metrics, and `place_footprint` transaction
operations for successful placements.

```sh
kicadai \
  --request ./examples/placement/simple_request.json \
  place request
```

Current placement support includes:

- fixed components and top/bottom side constraints;
- connector edge intent;
- hard and optional keepouts plus mechanical constraints;
- component spacing, group anchors, and group spread checks;
- proximity rules and region preferences;
- semantic candidate scoring for component role, group cohesion, electrical
  proximity, route length, congestion, fanout, edge, region, and mobility;
- advanced placement rules for thermal spacing/edge preference, high-current
  source-load proximity, creepage/clearance domain spacing, differential-pair
  placement readiness, and controlled-impedance corridor/reference-plane
  evidence;
- timing-sensitive placement scoring for clock-source proximity rules, used by
  crystal, canned oscillator, and reset/programming realization evidence;
- hard candidate rejection for checkable advanced-rule violations;
- per-net HPWL and routing-readiness reports;
- footprint-derived bounds helpers;
- transaction operation output.

Requests can use explicit component bounds or hydrate bounds from the library
resolver in Go before calling `placement.Place`. The usable board area uses the
larger of the JSON `board.margin_mm` and `rules.board_edge_clearance_mm` values
(`Board.MarginMM` and `Rules.BoardEdgeClearanceMM` in Go). Board coordinates
follow KiCad's schematic/PCB file convention: X increases to the right and Y
increases downward.

`placement.BuildQualityReport` returns AI-facing evidence for repair loops:

- `group_reports`, `proximity_reports`, `region_reports`, `net_reports`, and
  `keepout_reports` describe why a placement is good or poor; edge constraint
  evidence is exposed through edge counters and score dimensions.
- `score.dimensions` currently covers group cohesion, edge constraints,
  mechanical keepouts, region satisfaction, routing readiness, and proximity.
- `diagnostics` maps placement quality issues to repairable actions for
  missing placements, keepouts, regions, proximity, routing readiness,
  estimated footprint geometry, grouping, advanced placement rules, and
  validation issues.
- Design workflow placement summaries include advanced-rule dimension counts,
  worst scores, hard violations, warning counts, and unsupported/missing proof
  evidence for AI callers.
- Design workflow PCB realization summaries include `timing_results`, and
  timing evidence findings are surfaced as stage issues with refs, nets, and
  repair suggestions when thresholds are violated.

The placement hardening golden corpus covers representative LED, regulator,
MCU minimal, USB-C power, I2C sensor, op-amp gain-stage, and connector-breakout
layouts. Placement-routing retry coverage now also includes deterministic
goldens for retry summary shape, spacing/fanout/distance adjustments,
unsupported skip behavior, selected stop conditions, CLI output, and pad-backed
full-board seed evidence. Generated full-board boundary coverage now includes
hydrated-pad retry evidence for LED and multi-block sensor/header workflows,
generated placement mobility summaries, local-route mobility summaries, and
hard-constraint preservation under retry adjustment. Generated inter-block
connections are promoted into placement nets for connector/LED and I2C breakout
workflows, and routing summaries now expose `inter_block_routing` evidence for
candidate, attempted, partial, unrouted, and completed generated inter-block
nets. The companion `inter_block_contacts` summary reports required contacts,
proven contacts, failed contacts, contact misses, net/layer mismatches, and
missing targets. `routes_completed` now means same-net contact graph completion,
not merely route operation emission.

For generated multi-block designs, `inter_block_routing` also reports
multi-endpoint route-tree evidence:

- `multi_endpoint_nets`, `required_endpoints`, and `proven_endpoints`;
- `branches_planned`, `branches_attempted`, and `branches_completed`;
- `graph_component_count` and `missing_required_endpoints`;
- `complete_groups`, `partial_groups`, and `blocked_groups`.

These fields are intended for AI repair loops and fixture promotion decisions.
They prove endpoint grouping, target resolution, branch planning, contact proof,
and graph-level completion status. They do not yet mean KiCad DRC-clean routing
for richer generated boards; the I2C breakout fixture remains an
`expected_fail` case until every VCC/GND/SDA/SCL route group is graph-complete
and KiCad-backed evidence passes.

The routing stage also exposes `inter_block_route_trees` when generated
inter-block nets are managed by route-tree execution instead of the fallback
net-level router. That summary includes planned/attempted/complete/partial/
blocked group counts, planned/attempted/routed/blocked branch counts, issue
counts, and `managed_nets`. Managed nets are intentionally removed from the
fallback `routing.Request.Nets` list to avoid double-routing; use
`inter_block_route_trees.managed_nets` to see route-tree ownership.

Route-tree branch execution is now access-driven. For each branch, KiCadAI
ranks available endpoint access candidates from physical pads and generated
block-local route anchors, builds bounded synthetic access-pad route requests,
and tries candidate pairs in deterministic order. Selected access routes are
emitted as normal KiCad route operations, but are marked internally so
post-route endpoint snapping does not move a local-anchor merge back to the
original pad center. Branch evidence includes attempted access-pair counts and
selected source/target access roles.

When route-tree branches or endpoint contacts fail, the routing stage also
emits `route_tree_repair`. It summarizes classified branch/contact failures,
repairable failures, generated hint count, affected nets, and affected refs.
The current categories include other-net pad blockers, keepouts, board-edge
blockers, existing copper blockers, layer/via access, search exhaustion,
contact misses, missing contact targets, graph splits, unsupported failures,
and unknown failures. Bounded placement-routing retry consumes repairable
route-tree hints alongside normal routing diagnostics and ranks attempts using
route-tree completion evidence such as complete/partial/blocked groups, proven
endpoints, routed branches, contact misses, and issue counts.

The routing stage also emits `route_tree_contact_graph` for route-tree-managed
nets. It reports required/proven endpoint counts, graph component counts,
complete/partial/blocked group counts, and same-net/local-route merge evidence.
This graph evidence lets generated block-local routes participate in
route-tree contact proof without inflating inter-block emitted-segment counts.
The I2C fixture now reaches three complete contact-graph groups and one partial
contact-graph group, with 11 of 12 required endpoint contacts proven; the
remaining expected-fail blocker is one VCC contact/branch proof gap after
bounded retry.

Placement is still a deterministic heuristic, not a production-grade constraint
solver. Advanced placement rules are placement-level heuristics and evidence,
not thermal simulation, impedance calculation, or routed length matching.
Larger-board convergence, spatial acceleration for large hard-rule sets, richer
contact repair execution, and final KiCad DRC-backed layout proof remain future
work.

#### Route-tree troubleshooting

Use the route-tree summaries together rather than reading one field in
isolation:

- `inter_block_route_trees` reports branch executor ownership and branch
  success/failure.
- `route_tree_access` reports whether generated pads and local-route anchors
  were available as physical access evidence.
- `route_tree_contact_graph` reports same-net graph connectivity, including
  local-route merge evidence, without inflating emitted inter-block route
  segment counts.
- `route_tree_repair` reports classified branch/contact blockers and retry
  hint inputs.

Common blocker meanings:

- Missing endpoint access means footprint pad hydration, net assignment, or
  local-route binding did not expose a usable physical point.
- A split or partial contact graph means some same-net endpoints are proven,
  but not all required endpoints are in one same-net component.
- A same-net merge gap means pads/local routes are known but the branch router
  did not legally reach or merge into them.
- An other-net obstacle means placement, fanout, clearance, or layer access
  must change before that branch can route.
- A clean internal contact graph followed by KiCad DRC findings means the
  remaining blocker is external ERC/DRC evidence, not internal graph proof.


### Routing

The routing engine is a deterministic grid router for small placed boards. It
accepts a structured `routing.Request`, routes intentional nets, validates
connectivity and clearance in-process, and returns route segments, vias,
metrics, issues, AI-facing repair diagnostics, and route-shaped operations.

Run routing from JSON:

```sh
kicadai \
  --request ./examples/routing/simple_request.json \
  --mode single_layer \
  --grid 1 \
  --trace-width 0.1 \
  --clearance 0.2 \
  route request
```

For `route request`, CLI flags override the matching values from the JSON
request. Omit the flags when you want to run the fixture exactly as written.

Useful route flags:

- `--mode single_layer|two_layer|validate_only`
- `--grid <mm>`
- `--trace-width <mm>`
- `--clearance <mm>`
- `--allow-partial`

Routing JSON uses `rules.edge_clearance_mm` for board-edge clearance. Placement
JSON uses `rules.board_edge_clearance_mm`; the two request schemas are related
but intentionally separate. There is not currently a route CLI override for
edge clearance, so set this value in the JSON request.

The Go API entry points are:

- `routing.RouteRequest(request)` for raw route planning.
- `routing.ValidateResult(request, result)` for in-process route checks.
- `routing.DiagnosticsForResult(result)` for repair categories/actions.
- `routingadapters.RequestFromPlacement(...)` to route placement output.
- `designapi.Builder.RouteBoard(...)` to apply routed tracks/vias to a design.
