# Routing Engine Implementation Plan

## Objective

Implement a deterministic routing engine that converts placed footprints and
intended nets into KiCad-native copper transactions, then validates that the
result is electrically connected and geometrically legal.

The first implementation should prioritize correctness, deterministic output,
clear AI-facing diagnostics, and small-board usefulness over advanced routing
features.

## Implementation Rules

- Keep routing logic in `internal/routing`.
- Do not write KiCad files directly from the routing package.
- Emit transaction operations for route segments and vias.
- Preserve fixed/user-authored copper by default.
- Keep all algorithms deterministic for the same request and seed.
- Return structured `reports.Issue` values for all request, route, and
  validation problems.
- Prefer simple rectangular/two-layer behavior first, with explicit warnings
  for unsupported advanced KiCad features.
- Run `gofmt` and `GOCACHE=/tmp/kicadai-gocache go test ./...` before each
  phase commit.
- Run `prism review staged` before each phase commit and address actionable
  findings.
- Commit after each completed phase.

## Phase 1: Core Models And Request Validation

### Goals

Create the routing package, public request/result model, defaults, and
validation foundation.

### Tasks

1. Add `internal/routing`.
2. Define core types:
   - `Request`;
   - `Board`;
   - `Layer`;
   - `Component`;
   - `Pad`;
   - `Net`;
   - `Endpoint`;
   - `Obstacle`;
   - `ExistingCopper`;
   - `Rules`;
   - `Strategy`;
   - `Result`;
   - `Route`;
   - `Segment`;
   - `Via`;
   - `Metrics`.
3. Add status enums:
   - `routed`;
   - `partial`;
   - `blocked`.
4. Add route modes:
   - `single_layer`;
   - `two_layer`;
   - `validate_only`.
5. Add default rules and normalization.
6. Validate:
   - positive board dimensions;
   - at least one routable copper layer;
   - supported routing mode;
   - positive trace width, clearance, grid, via diameter, and via drill;
   - via drill smaller than via diameter;
   - valid layer names;
   - unique component refs;
   - unique pad keys per component;
   - nets reference known pads.
7. Add unit tests for valid defaults and invalid requests.

### Acceptance Criteria

- `go test ./internal/routing` passes.
- A minimal valid request normalizes defaults.
- Invalid requests return structured blocking issues.
- No route search occurs when request validation fails.

### Suggested Commit

`Add routing engine models`

## Phase 2: Geometry, Grid, And Coordinate Conversion

### Goals

Add deterministic geometry primitives and grid conversion utilities used by
obstacle generation and path search.

### Tasks

1. Define geometry helpers:
   - `Point`;
   - `Size`;
   - `Rect`;
   - `Shape`;
   - grid coordinate type.
2. Implement board-local to grid conversion.
3. Implement grid to board-local conversion.
4. Implement board-local to KiCad-global coordinate conversion.
5. Add fixed-precision rounding policy.
6. Add rectangle containment, intersection, expansion, and distance helpers.
7. Add layer index lookup.
8. Add tests for:
   - coordinate round trip;
   - board origin handling;
   - grid snapping;
   - rectangle inflation;
   - layer lookup.

### Acceptance Criteria

- Grid conversion is deterministic and stable under repeated conversion.
- Board margin and edge clearance can be represented as blocked grid cells.
- Floating-point drift does not appear in route output tests.

### Suggested Commit

`Add routing geometry and grid helpers`

## Phase 3: Pad Lookup And Access Points

### Goals

Turn placed footprint pads into route endpoints and first-pass pad access
points.

### Tasks

1. Build normalized pad lookup by `ref/pin`.
2. Normalize endpoint refs and pins consistently with placement.
3. Represent pad shapes:
   - circle;
   - oval;
   - rect;
   - rounded rect approximated as rect.
4. Support pad layer semantics:
   - SMD pad on explicit copper layer;
   - through-hole pad on all copper layers;
   - wildcard copper layers where available.
5. Generate release-1 access points:
   - pad center on valid copper layers;
   - optional side access points for rectangular pads when they snap to grid.
6. Return warnings for unsupported pad shapes or center-only fallbacks.
7. Add tests for:
   - lookup normalization;
   - SMD layer access;
   - through-hole multi-layer access;
   - missing pad diagnostics;
   - unsupported shape warnings.

### Acceptance Criteria

- Every routable endpoint resolves to at least one access point.
- Missing pad geometry blocks only affected routes, or the whole request when
  the mode requires all nets.
- Access point output is deterministic.

### Suggested Commit

`Add routing pad access model`

## Phase 4: Obstacle Extraction And Clearance Inflation

### Goals

Build per-layer obstacle occupancy from board edges, pads, keepouts, existing
copper, vias, and explicit obstacles.

### Tasks

1. Add obstacle kinds:
   - board edge;
   - keepout;
   - other-net pad;
   - same-net pad body;
   - existing fixed copper;
   - via keepout;
   - mechanical hole;
   - zone obstacle.
2. Inflate obstacles by:
   - trace half-width;
   - trace clearance;
   - via radius;
   - via clearance;
   - edge clearance.
3. Build per-layer occupancy grids.
4. Mark pads from other nets as blocked.
5. Allow current-net access points as legal start/target cells.
6. Treat existing unknown-net copper as obstacle.
7. Treat existing same-net copper as reusable target metadata, but not yet as a
   full graph edge unless implemented.
8. Add tests for:
   - board edge blockage;
   - other-net pad blockage;
   - same-net access exception;
   - keepout blockage;
   - fixed copper blockage;
   - via clearance blockage.

### Acceptance Criteria

- Obstacles are layer-aware.
- The router cannot place traces through other-net pads or keepouts.
- Same-net access points remain usable.

### Suggested Commit

`Add routing obstacle occupancy`

## Phase 5: Net Ordering And Pair Planning

### Goals

Select deterministic net order and endpoint-pair route order for multi-endpoint
nets.

### Tasks

1. Implement net filtering:
   - skip no-net;
   - skip one-endpoint nets with informational issue;
   - skip fixed nets unless reroute is requested.
2. Implement route order:
   - priority descending;
   - power and ground before signal when tied;
   - fewer endpoints before larger nets;
   - lexical fallback by net name.
3. Implement endpoint pair planning:
   - release-1 nearest-neighbor tree;
   - deterministic tie breaks by endpoint key.
4. Include existing same-net copper as an optional tree seed when available.
5. Add metrics for net count and skipped nets.
6. Add tests for:
   - net ordering;
   - one-endpoint skip;
   - pair planning for two-endpoint and multi-endpoint nets;
   - deterministic ties.

### Acceptance Criteria

- The same request always produces the same net and pair order.
- Multi-endpoint nets produce a route tree plan, not an arbitrary pair list.

### Suggested Commit

`Add deterministic route planning`

## Phase 6: Single-Layer A* Router

### Goals

Route simple two-pad nets on one layer using a grid-based A* search.

### Tasks

1. Add A* state:
   - X grid coordinate;
   - Y grid coordinate;
   - layer index.
2. Add priority queue implementation or reuse standard heap.
3. Add orthogonal neighbors.
4. Add Manhattan heuristic.
5. Add costs:
   - grid distance;
   - bend penalty;
   - preferred direction tie-break;
   - deterministic stable tie-break.
6. Enforce blocked cells and board bounds.
7. Reconstruct path after target reached.
8. Convert grid path to board-local point path.
9. Add tests for:
   - straight route;
   - L-shaped route;
   - obstacle avoidance;
   - no-path failure;
   - deterministic route.

### Acceptance Criteria

- A simple unobstructed net routes on `F.Cu`.
- Obstacles are avoided.
- No-path conditions produce structured route issues.

### Suggested Commit

`Implement single layer A star routing`

## Phase 7: Path Simplification And Segment Generation

### Goals

Convert grid paths into compact KiCad segment geometry.

### Tasks

1. Merge collinear path steps.
2. Preserve bend points.
3. Snap output coordinates to stable precision.
4. Compute segment lengths.
5. Build `Segment` values with net, layer, start, end, and width.
6. Add route metrics:
   - segment count;
   - total length;
   - search nodes.
7. Add tests for:
   - straight path merges to one segment;
   - L path merges to two segments;
   - zero-length segments are removed;
   - length metrics are correct.

### Acceptance Criteria

- Segment output is compact and deterministic.
- No zero-length segments are emitted.

### Suggested Commit

`Simplify route paths into segments`

## Phase 8: Two-Layer Routing And Vias

### Goals

Support via transitions between `F.Cu` and `B.Cu`.

### Tasks

1. Extend A* neighbors with layer transitions at same grid coordinate.
2. Add via legality checks:
   - allowed by rules;
   - within board;
   - respects via clearance;
   - does not overlap obstacles.
3. Add via cost and `MaxViasPerNet`.
4. Track via count per path.
5. Generate `Via` values from layer transitions.
6. Support single-layer mode that forbids vias.
7. Add tests for:
   - route requiring one via;
   - via forbidden failure;
   - max via limit;
   - via obstacle clearance;
   - back-layer disabled failure.

### Acceptance Criteria

- Two-layer routes can avoid a same-layer obstacle using vias.
- Vias are emitted with deterministic diameter, drill, and layer span.
- Single-layer mode never emits vias.

### Suggested Commit

`Add two layer routing with vias`

## Phase 9: Route Application, Occupancy Updates, And Partial Results

### Goals

Route whole requests net-by-net and update occupancy as new copper is produced.

### Tasks

1. Add main `Route(request Request) Result` entrypoint.
2. Route nets in planned order.
3. After each successful pair:
   - add segments as obstacles for other nets;
   - add vias as obstacles for other nets;
   - mark same-net copper reusable.
4. Support `Strategy.AllowPartial`.
5. Return `routed`, `partial`, or `blocked`.
6. Track failed nets and failed endpoint pairs.
7. Add route-level issues.
8. Add tests for:
   - two independent nets;
   - second net avoids first net;
   - failed first net with partial disallowed blocks output;
   - failed one net with partial allowed preserves successful routes.

### Acceptance Criteria

- Whole-board routing produces a stable `Result`.
- Routed copper affects later nets.
- Partial routing behavior is explicit and tested.

### Suggested Commit

`Route complete requests with occupancy updates`

## Phase 10: Connectivity And Clearance Validation

### Goals

Validate routed results without requiring KiCad.

### Tasks

1. Add `ValidateResult(request Request, result Result) ValidationReport`.
2. Check:
   - segment layers are routable;
   - via layer spans are valid;
   - segment endpoints are on grid;
   - segments are inside board area;
   - no segment crosses keepout or board edge;
   - different-net segment clearance;
   - different-net via clearance;
   - route-to-pad clearance;
   - all intended endpoints are connected;
   - no accidental other-net pad connections.
3. Build connectivity graph by net.
4. Include route validation issues in `Result`.
5. Add tests for:
   - disconnected endpoint detection;
   - segment outside board;
   - clearance violation;
   - via clearance violation;
   - accidental other-net pad contact.

### Acceptance Criteria

- Internal validation catches illegal copper before KiCad DRC.
- Connectivity validation proves all routed net endpoints are connected.

### Suggested Commit

`Validate routed connectivity and clearance`

## Phase 11: Transaction Operation Output

### Goals

Convert routing results into existing transaction operations for the writer
pipeline.

### Tasks

1. Inspect current `internal/transactions` route/via operation support.
2. Extend transaction models if required for:
   - segment net name;
   - layer;
   - width;
   - start/end;
   - via location;
   - via diameter;
   - via drill;
   - via layers.
3. Add route-to-operation conversion.
4. Preserve deterministic operation ordering:
   - net order;
   - path order;
   - segment/via order as encountered.
5. Add tests for:
   - segment operation fields;
   - via operation fields;
   - operation ordering;
   - empty route operation behavior.

### Acceptance Criteria

- `Result.Operations` can be consumed by the existing PCB writer path.
- No routing package code writes KiCad files directly.

### Suggested Commit

`Emit routing transaction operations`

## Phase 12: Placement And Block Adapters

### Goals

Build routing requests from placement results, transaction streams, and circuit
block composition outputs.

### Tasks

1. Add adapter from `placement.Result` and placement request data.
2. Add adapter from transaction operation streams.
3. Add adapter from circuit block output where available.
4. Reuse footprint pad summaries and library resolver output.
5. Preserve board origin and layer information.
6. Detect missing pad layers or positions as blocking issues.
7. Add tests for:
   - placed two-component request to routing request;
   - block LED circuit to routing request;
   - missing footprint pad data failure;
   - net names preserved.

### Acceptance Criteria

- The router can consume output from the current placement workflow.
- Adapters do not duplicate writer-specific logic.

### Suggested Commit

`Connect routing to placement outputs`

## Phase 13: CLI Route Command

### Goals

Expose routing from the CLI for manual testing and AI tool workflows.

### Tasks

1. Add `kicadai route`.
2. Support input JSON request.
3. Support output JSON result.
4. Include human-readable summary:
   - status;
   - routed nets;
   - failed nets;
   - segments;
   - vias;
   - total length;
   - issue count.
5. Add optional flags:
   - `--mode`;
   - `--grid`;
   - `--trace-width`;
   - `--clearance`;
   - `--allow-partial`;
   - `--seed`;
   - `--pretty`.
6. Add CLI tests for:
   - valid route request;
   - invalid request exits non-zero;
   - JSON output shape.

### Acceptance Criteria

- Users can run routing from the command line without writing Go code.
- CLI output is stable and scriptable.

### Suggested Commit

`Add routing CLI command`

## Phase 14: Design API Workflow Integration

### Goals

Add board-level routing to the higher-level design workflow.

### Tasks

1. Add or extend design API route options:
   - `RouteBoard`;
   - `RouteOptions`;
   - `RouteBoardOptions`.
2. Build routing request from current design state.
3. Apply returned route operations to the project transaction stream.
4. Preserve existing single-net `Route` API if present.
5. Add workflow command path:
   - generate schematic;
   - assign footprints;
   - place;
   - route;
   - write project.
6. Add tests for:
   - API route board call;
   - route operations appear in written PCB;
   - route failure returns structured issues.

### Acceptance Criteria

- AI-facing workflows can call a single board route operation.
- Routed output flows into the existing writer.

### Suggested Commit

`Integrate routing with design workflows`

## Phase 15: KiCad DRC Feedback Integration

### Goals

Feed routed generated boards through existing KiCad validation where available.

### Tasks

1. Reuse ERC/DRC feedback-loop package.
2. Write routed board through existing project writer.
3. Run KiCad DRC when CLI is available.
4. Parse findings into `reports.Issue`.
5. Attach DRC issues to route result/report.
6. Mark integration tests skippable when KiCad CLI is unavailable.
7. Add tests using mocked DRC output and optional real KiCad integration.

### Acceptance Criteria

- Route validation can include KiCad DRC findings.
- Missing KiCad CLI does not break normal unit tests.
- DRC issues are structured for AI repair.

### Suggested Commit

`Integrate routing with DRC feedback`

## Phase 16: Golden Routed Examples

### Goals

Create small routed examples and golden tests that prove routing is useful, not
only parseable.

### Tasks

1. Add routed example data for:
   - LED plus resistor;
   - RC filter;
   - LDO regulator;
   - op-amp gain stage;
   - I2C sensor breakout.
2. Generate routed PCBs through the workflow.
3. Store expected route metrics.
4. Add golden tests for normalized route output.
5. Run internal validation on each example.
6. Run KiCad DRC where available.
7. Ensure examples open in KiCad without new format warnings.

### Acceptance Criteria

- At least three examples route cleanly end-to-end.
- Golden tests catch route regressions.
- Example files remain deterministic.

### Suggested Commit

`Add golden routed examples`

## Phase 17: AI-Facing Repair Diagnostics

### Goals

Make routing failures actionable for AI design loops.

### Tasks

1. Add structured issue metadata:
   - net;
   - endpoint pair;
   - layer;
   - obstacle source;
   - failure reason;
   - suggested repair.
2. Define canonical failure reason constants:
   - `missing_pad_geometry`;
   - `no_legal_path`;
   - `blocked_by_keepout`;
   - `clearance_too_large`;
   - `via_limit_reached`;
   - `layer_not_allowed`;
   - `search_limit_hit`.
3. Add repair suggestions:
   - move component;
   - increase board size;
   - allow vias;
   - reduce trace width;
   - reduce clearance;
   - add keepout exception;
   - route manually.
4. Add JSON report shape for AI consumers.
5. Add tests for route failures producing machine-readable diagnostics.

### Acceptance Criteria

- AI workflows can identify why a net failed without scraping prose.
- Every failed net includes at least one suggested repair.

### Suggested Commit

`Add routing repair diagnostics`

## Phase 18: Performance And Stress Hardening

### Goals

Add bounded stress tests and performance guardrails for the router.

### Tasks

1. Add benchmark cases:
   - LED/resistor;
   - 10-net breakout;
   - moderate 40-component board.
2. Add stress tests for:
   - impossible routing;
   - dense obstacle grids;
   - large board with low node limit;
   - repeated deterministic runs.
3. Ensure search-node limits stop cleanly.
4. Profile hot paths if benchmarks exceed targets.
5. Optimize only measured bottlenecks.
6. Document benchmark expectations.

### Acceptance Criteria

- Benchmarks run locally.
- Search limit failures are clean and structured.
- Deterministic stress tests pass reliably.

### Suggested Commit

`Harden routing performance`

## Phase 19: Documentation And README Update

### Goals

Document routing capabilities, limitations, and CLI usage.

### Tasks

1. Update project README with routing status.
2. Add routing CLI examples.
3. Document supported features:
   - rectangular boards;
   - two copper layers;
   - Manhattan segments;
   - vias;
   - clearances;
   - partial routing.
4. Document limitations:
   - no differential pairs;
   - no length matching;
   - no advanced zone interaction;
   - no dense BGA autorouting.
5. Add troubleshooting notes for route failures.
6. Link routing spec and plan.

### Acceptance Criteria

- README accurately describes current routing functionality.
- Users can try routing from the CLI with an example.

### Suggested Commit

`Document routing engine`

## Phase 20: Final End-To-End Review

### Goals

Verify the full schematic-to-routed-PCB path and identify remaining gaps.

### Tasks

1. Run all unit tests.
2. Run golden routed examples.
3. Run KiCad DRC integration if available.
4. Review generated PCB files in KiCad manually if possible.
5. Run Prism on final staged docs or code changes.
6. Write a short status note identifying:
   - completed routing capabilities;
   - limitations;
   - next recommended work.

### Acceptance Criteria

- Full test suite passes.
- Routed examples produce deterministic output.
- Remaining gaps are documented rather than implicit.

### Suggested Commit

`Finalize routing engine implementation`

## Initial Implementation Slice

The minimum useful vertical slice is phases 1 through 11:

1. models and validation;
2. geometry/grid;
3. pad access;
4. obstacles;
5. net planning;
6. single-layer A*;
7. segment generation;
8. vias;
9. whole-request routing;
10. internal validation;
11. transaction output.

After that slice, the CLI, design API, DRC feedback, and golden examples can be
layered on with less risk.

## Risks

- Grid routing can fail on boards a human can route with off-grid paths.
- Coarse grids can miss valid paths; fine grids can become slow.
- Center-only pad access can create unnecessary failures near dense footprints.
- Existing transaction route operations may need expansion before router output
  can be applied cleanly.
- KiCad DRC output may vary by installed KiCad version and environment.
- Placement quality strongly affects routing success.

## Open Questions

- Should release 1 allow diagonal jogs, or strictly Manhattan routes?
- Should ground/power nets be routed as traces first, or deferred to zones?
- How much existing same-net copper should be considered reusable during A*?
- Should the router own net-class defaults, or inherit them only from the PCB
  writer/project model?
- Should route failures trigger automatic placement repair in this project, or
  remain a higher-level AI workflow responsibility?
