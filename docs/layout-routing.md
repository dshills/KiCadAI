# Placement And Routing

Placement engine, routing engine, and related quality evidence.

### Placement

The placement engine accepts a structured board placement request and returns
placed components, geometry issues, metrics, and `place_footprint` transaction
operations for successful placements.

```sh
kicadai \
  --json \
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
hard-constraint preservation under retry adjustment.
Placement is still a deterministic heuristic, not a production-grade constraint
solver. Advanced placement rules are placement-level heuristics and evidence,
not thermal simulation, impedance calculation, or routed length matching.
Larger-board convergence, spatial acceleration for large hard-rule sets, and
final KiCad DRC-backed layout proof remain future work.


### Routing

The routing engine is a deterministic grid router for small placed boards. It
accepts a structured `routing.Request`, routes intentional nets, validates
connectivity and clearance in-process, and returns route segments, vias,
metrics, issues, AI-facing repair diagnostics, and route-shaped operations.

Run routing from JSON:

```sh
kicadai \
  --json \
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
