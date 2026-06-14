# Routing Engine Specification

## Purpose

Build a deterministic PCB routing engine that turns placed footprints and
electrical nets into KiCad-native copper objects.

The goal is not to compete with mature interactive or push-and-shove routers in
the first release. The goal is to give AI-generated designs a reliable path from:

```text
schematic intent -> footprints -> placement -> routed PCB -> DRC feedback
```

The first implementation should route small, known-good two-layer boards using
clear rules, understandable failures, and writer output that KiCad can open,
inspect, and validate. It must produce electrically meaningful boards, not only
parseable `.kicad_pcb` files.

## Current Context

The project already has:

- KiCad project, schematic, and PCB file writers;
- KiCad 10-compatible PCB layer/setup/net/footprint/segment/via support;
- transaction operations including `place_footprint`, `route`, and `add_zone`;
- a placement engine that emits deterministic footprint locations;
- footprint-library parsing and pad summaries;
- circuit block outputs with schematic and PCB hints;
- ERC/DRC feedback-loop tooling;
- round-trip compatibility work for preserving unsupported KiCad content;
- a CLI path for inspect, evaluate, place, and workflow commands.

The routing engine should sit after placement and before PCB writing/validation.
It should not create a separate board writer. It should return transaction
operations or route objects that the existing writer pipeline can emit.

## Goals

1. Define a routing request/response model that can be built from placed
   footprints, nets, pad geometry, board outlines, zones, and routing rules.
2. Route simple two-layer boards deterministically.
3. Support single-layer and two-layer Manhattan routes with vias.
4. Respect board outlines, keepouts, pad clearances, trace clearances, via
   clearances, and layer restrictions.
5. Preserve fixed/user-authored copper and treat it as an obstacle unless a
   caller explicitly allows rerouting.
6. Emit KiCad-native route transaction operations: segments and vias first,
   with arcs/differential pairs/decorations deferred.
7. Produce structured route reports, failures, and partial-routing diagnostics
   suitable for AI feedback.
8. Integrate with KiCad validation where available and internal geometry checks
   always.
9. Provide golden routed examples for small boards.

## Non-Goals

- High-speed signal-integrity routing.
- Differential-pair length matching in the first release.
- Interactive push-and-shove behavior.
- Autorouting dense BGA fanouts.
- RF tuning, impedance control, serpentine tuning, or teardrops.
- Automatic copper pour optimization.
- Thermal simulation.
- Replacing KiCad's interactive router.

These are future capabilities once deterministic small-board routing is solid.

## Package Boundary

Add a package:

```text
internal/routing
```

The package owns:

- route request/response models;
- board routing grid construction;
- obstacle extraction;
- net ordering and pad-pair planning;
- A*/maze routing;
- via insertion;
- route simplification;
- internal route validation;
- conversion to transaction operations.

The package must not write `.kicad_pcb` files directly. It should return:

- `transactions.Operation` values for route segments and vias;
- a `Result` containing route geometry, status, metrics, and issues; or
- both, with operations treated as the writer-facing output.

## Required Current Gaps

The following gaps must be closed or explicitly worked around:

- The PCB writer can emit route objects, but routing currently lacks a physical
  path planner.
- The design API exposes `Route`, but it does not yet compute legal copper paths.
- The placement engine optimizes proximity but does not guarantee routability.
- Existing connectivity checks prove parseability and some object correctness,
  but not that every intended pad is physically connected by valid copper.
- Footprint pad extraction exists, but routing needs exact pad centers, pad
  shapes, pad layers, drill/via constraints, and net assignments in one model.
- KiCad DRC integration can report failures, but routing must provide its own
  pre-DRC checks so AI callers receive fast, structured errors.
- Zone interaction is currently under-modeled. The first router may treat zones
  as obstacles or ignore unfilled copper pours, but it must report that choice.

## Input Model

The router should accept a `Request`.

```go
type Request struct {
    Board       Board
    Components  []Component
    Nets        []Net
    Obstacles   []Obstacle
    Existing    []ExistingCopper
    Rules       Rules
    Strategy    Strategy
    Seed        string
}
```

### Board

```go
type Board struct {
    Origin     Point
    WidthMM    float64
    HeightMM   float64
    Outline    []Shape
    Layers     []Layer
    MarginMM   float64
}
```

Initial implementation requirements:

- Rectangular boards are required.
- Arbitrary outlines are optional but the model must leave room for them.
- Coordinates are millimeters at the public API boundary.
- Internal grid calculations should use integer micrometers or fixed grid units
  to avoid floating-point drift.
- `Origin` follows the same convention as placement: local board coordinates
  convert to KiCad global coordinates by adding `Origin`.

### Layer

```go
type Layer struct {
    Name      string
    Kind      LayerKind
    Routable  bool
}
```

First release:

- support `F.Cu`;
- support `B.Cu`;
- support via transitions between `F.Cu` and `B.Cu`;
- reject or warn on additional copper layers unless the strategy explicitly
  says to ignore them.

### Component

```go
type Component struct {
    Ref       string
    Footprint string
    Position  Placement
    Pads      []Pad
    Fixed     bool
}
```

The routing engine assumes footprints have already been placed.

Rules:

- `Ref` is required.
- `Position` is required.
- `Pads` must include at least the pads referenced by nets.
- Missing pad geometry is a blocking issue for affected nets.
- Fixed components are still routable; fixed copper is represented separately.

### Pad

```go
type Pad struct {
    Ref        string
    Name       string
    Net        string
    Position   Point
    Shape      PadShape
    Size       Size
    Drill      *Drill
    Layers     []string
    Clearance  *float64
}
```

Requirements:

- `Position` is absolute board-local pad center after footprint rotation.
- Pad shape support for release 1:
  - circle
  - oval
  - rect
  - rounded rect approximated as rect for clearance
- SMD pads are routable on their copper layer.
- Through-hole pads are routable on all copper layers.
- Pads on nets not being routed are obstacles.
- Pads on the current net are valid endpoints but still obstacles except at
  their connection access point.

### Net

```go
type Net struct {
    Name       string
    Endpoints  []Endpoint
    Role       NetRole
    Class      string
    Priority   int
    Fixed      bool
}
```

Rules:

- `Name` is required except no-net.
- No-net endpoints must not be routed.
- Nets with fewer than two endpoints are skipped with an informational issue.
- `Fixed` nets preserve existing copper and are not routed unless reroute is
  requested.
- Multi-endpoint nets route as a tree. The first implementation should use a
  nearest-neighbor or minimum-spanning-tree approximation over pad centers.

### Endpoint

```go
type Endpoint struct {
    Ref string
    Pin string
}
```

Endpoint matching must normalize ref and pin strings consistently with
placement and library mapping.

### ExistingCopper

```go
type ExistingCopper struct {
    Kind      CopperKind
    Net       string
    Layer     string
    Geometry  Shape
    Fixed     bool
}
```

Existing copper handling:

- Fixed copper is always an obstacle for other nets.
- Existing copper on the same net may be reused as part of connectivity.
- Existing copper with unknown net is treated as an obstacle.
- Reroute support is optional for release 1. If unsupported, return a blocking
  issue when a caller requests it.

### Obstacle

```go
type Obstacle struct {
    Kind       ObstacleKind
    Layer      string
    Geometry   Shape
    Clearance  float64
    Source     string
}
```

Obstacle sources include:

- board edge margin;
- keepout region;
- other-net pad;
- same-net pad body excluding access;
- existing fixed copper;
- via keepout;
- mechanical hole;
- zone treated as obstacle.

### Rules

```go
type Rules struct {
    GridMM             float64
    TraceWidthMM       float64
    ClearanceMM        float64
    ViaDiameterMM      float64
    ViaDrillMM         float64
    ViaClearanceMM     float64
    EdgeClearanceMM    float64
    MaxSearchNodes     int
    MaxViasPerNet      int
    AllowVias          bool
    AllowBackLayer     bool
    PreferLayer        string
    NetClasses         map[string]NetClass
}
```

Defaults:

- `GridMM`: `0.25`
- `TraceWidthMM`: `0.25`
- `ClearanceMM`: `0.20`
- `ViaDiameterMM`: `0.60`
- `ViaDrillMM`: `0.30`
- `ViaClearanceMM`: `0.20`
- `EdgeClearanceMM`: `0.25`
- `MaxSearchNodes`: `250000`
- `MaxViasPerNet`: `4`
- `AllowVias`: `true`
- `AllowBackLayer`: `true`
- `PreferLayer`: `F.Cu`

Rules may be overridden by net class.

### Strategy

```go
type Strategy struct {
    Mode              RouteMode
    NetOrder          NetOrder
    RipupRetryLimit   int
    AllowPartial      bool
    PreserveExisting  bool
    TreatZonesAs      ZoneRoutingPolicy
}
```

Initial modes:

- `single_layer`
- `two_layer`
- `validate_only`

Initial net ordering:

- priority descending;
- power/ground before signal where priority ties;
- fewer endpoints before larger nets when otherwise equal;
- deterministic lexical fallback by net name.

## Output Model

```go
type Result struct {
    Status      Status
    Routes      []Route
    Operations  []transactions.Operation
    Issues      []reports.Issue
    Metrics     Metrics
}
```

Statuses:

- `routed`: all required nets routed and internally valid;
- `partial`: at least one required net failed, but partial output is allowed;
- `blocked`: validation failed before routing or no output should be applied.

### Route

```go
type Route struct {
    Net       string
    Segments  []Segment
    Vias      []Via
    Status    RouteStatus
    Issues    []reports.Issue
}
```

### Segment

```go
type Segment struct {
    Net       string
    Layer     string
    Start     Point
    End       Point
    WidthMM   float64
}
```

### Via

```go
type Via struct {
    Net          string
    At           Point
    DiameterMM   float64
    DrillMM      float64
    Layers       []string
}
```

### Metrics

```go
type Metrics struct {
    NetCount          int
    RoutedNetCount    int
    FailedNetCount    int
    SegmentCount      int
    ViaCount          int
    TotalLengthMM     float64
    SearchNodes       int
    MaxSearchNodesHit bool
}
```

## Routing Algorithm

### Release 1 Algorithm

Use a grid-based A* router.

Core steps:

1. Normalize and validate the request.
2. Build pad lookup from `Component.Pads`.
3. Build per-layer occupancy grids.
4. Inflate obstacles by clearance, trace half-width, via radius, and edge
   clearance as appropriate.
5. Order nets deterministically.
6. For each net:
   - choose endpoint pairs using a deterministic tree planner;
   - route each pair with A*;
   - add successful segments/vias to occupancy before routing later nets;
   - report failed pairs as route issues.
7. Simplify paths by merging collinear grid steps.
8. Convert route paths to KiCad segment/via operations.
9. Validate connectivity and clearances.

### A* Search State

```go
type State struct {
    X     int
    Y     int
    Layer int
}
```

Neighbors:

- left/right/up/down on same layer;
- optional layer transition at same X/Y through a via;
- no diagonal movement in release 1.

Costs:

- orthogonal grid step: grid distance;
- bend penalty: small configurable value;
- via penalty: larger configurable value;
- preferred-layer bonus or non-preferred-layer penalty;
- obstacle collision: forbidden;
- clearance violation: forbidden.

Heuristic:

- Manhattan distance on the current layer;
- add one via penalty when target layer differs.

The heuristic must be admissible enough to avoid pathological choices. If a
weighted heuristic is introduced, it must be deterministic and documented.

### Access Points

Pads are not single mathematical points in KiCad. The router should derive one
or more access points per pad:

- SMD rectangular/oval pads:
  - center point;
  - side points aligned to the routing grid when useful;
- through-hole pads:
  - center point on all copper layers;
  - optional nearby side points;
- unsupported shapes:
  - center point with warning.

The first version may use pad centers only, but the model must allow multiple
access points because center-only routing will fail unnecessarily on denser
boards.

### Multi-Endpoint Nets

Initial tree planner:

1. Start from the highest-priority or lexically first endpoint.
2. Repeatedly connect the nearest unrouted endpoint to any already-routed node
   in the same net.
3. Treat already-routed same-net copper as valid targets.

Future planner:

- rectilinear Steiner approximation;
- power/ground zone-aware connection strategy;
- fanout before long-route planning.

## Validation

The router must provide internal validation even when KiCad CLI is unavailable.

### Request Validation

Blocking issues:

- missing board dimensions;
- no routable copper layer;
- unsupported routing mode;
- missing endpoint pad;
- missing pad layer;
- invalid rule values;
- negative or zero trace width;
- margin/edge clearance leaves no usable board area.

Warnings:

- unsupported pad shape approximated;
- zones ignored or treated as obstacles;
- extra copper layers ignored;
- estimated placement bounds used upstream;
- center-only pad access mode used.

### Route Validation

Internal validation must check:

- every route segment has a declared net;
- every segment layer is routable;
- every via layer span is valid;
- segment endpoints are on grid within tolerance;
- segments stay inside board usable area;
- segment-to-obstacle clearance;
- segment-to-segment clearance for different nets;
- via-to-obstacle clearance;
- no route crosses board edge or keepout;
- intended net endpoints are connected by route geometry and same-net pads/vias;
- no other-net pads are connected accidentally.

### KiCad Validation

When KiCad CLI is available:

- write the routed board through the existing writer path;
- run DRC using the existing ERC/DRC feedback-loop utilities;
- parse DRC output into structured `reports.Issue` values;
- mark the route result `partial` or `blocked` depending on issue severity and
  `Strategy.AllowPartial`.

The router must still work without KiCad installed, using internal validation.

## Transaction Output

The routing engine should emit transaction operations, not raw writer calls.

Required operations:

- route segment operation with:
  - net name;
  - layer;
  - start/end points;
  - width;
- via operation with:
  - net name;
  - location;
  - diameter;
  - drill;
  - layer span.

If the current transaction package cannot express all required fields, extend
transactions first rather than bypassing them.

Operation ordering:

1. preserve existing fixed copper;
2. emit route segments sorted by net route order and path order;
3. emit vias at the point they appear in the path;
4. keep deterministic ordering for identical routes.

## CLI Integration

Add or extend commands under `cmd/kicadai`.

Suggested command:

```text
kicadai route --input placement.json --output routes.json
```

Potential project workflow command:

```text
kicadai generate-board --spec design.json --place --route --validate
```

CLI output should include:

- status;
- routed net count;
- failed net count;
- segment count;
- via count;
- total routed length;
- path to any generated board/report artifacts;
- structured issue list in JSON when requested.

## Design API Integration

The higher-level design API should expose routing through the existing operation
shape:

```go
Route(netName string, options RouteOptions) error
```

But AI workflows need a board-level route call:

```go
RouteBoard(options RouteBoardOptions) (*routing.Result, error)
```

Expected behavior:

- build routing request from current design state;
- call `internal/routing`;
- apply returned transactions;
- validate internally;
- optionally write board and run KiCad DRC;
- return structured issues for AI repair.

## AI-Facing Requirements

The router must make failures explainable.

Every failed net should report:

- net name;
- endpoint pair that failed;
- likely reason:
  - no pad geometry;
  - no legal path;
  - blocked by keepout;
  - clearance too large;
  - via limit reached;
  - layer not allowed;
  - search-node limit hit;
- suggested repair:
  - move component;
  - increase board size;
  - allow vias;
  - lower trace width/clearance;
  - add placement/routing keepout changes;
  - route manually.

AI repair loops should consume the report without scraping human text.

## Golden Examples

Add routed examples progressively:

1. LED plus resistor on one layer.
2. RC filter with labeled input/output.
3. LDO regulator with input/output capacitors and ground.
4. Op-amp gain stage.
5. I2C sensor breakout with pullups and connector.
6. USB-C power-only breakout.
7. MCU minimal system with decoupling, reset, SWD/UART header.

Each example should include:

- schematic;
- PCB with footprints placed;
- routed PCB;
- validation report;
- expected route metrics.

## Test Strategy

### Unit Tests

Cover:

- request validation;
- pad lookup and normalized endpoint matching;
- board/grid coordinate conversion;
- obstacle inflation;
- route graph neighbor generation;
- A* shortest path on simple grids;
- via insertion;
- net ordering;
- route simplification;
- connectivity validation;
- transaction conversion.

### Golden Tests

Golden tests should:

- route fixed small boards;
- compare normalized route output;
- confirm route metrics;
- run internal validation;
- run KiCad DRC when available.

### Property/Stress Tests

Add bounded randomized tests for:

- no segments outside board;
- no different-net clearances violated;
- deterministic output for identical seeds;
- failure without panic when no path exists;
- search-node limit behavior.

### Round-Trip Tests

For generated routed boards:

- write KiCad PCB;
- open/parse it with project parser where available;
- rewrite and diff normalized structure;
- optionally let KiCad save/upgrade and compare normalized route sections.

## Performance Requirements

Initial targets on a typical laptop:

- LED/resistor board: under 50 ms.
- 10-net breakout: under 500 ms.
- 40-component moderate board: under 5 seconds.

Performance controls:

- `MaxSearchNodes`;
- grid size;
- via limit;
- route mode;
- net order;
- optional partial routing.

The router must fail cleanly when limits are hit.

## Safety and Determinism

The router must be deterministic:

- no map iteration may affect route order;
- random choices require explicit seed;
- route tie-breakers must be stable;
- output coordinates must be rounded consistently;
- tests must not depend on KiCad being installed unless marked integration.

The router must not silently change user-authored fixed copper.

## Future Extensions

- Arbitrary polygon board outlines.
- Curved traces and route arcs.
- Differential pair routing.
- Length matching.
- Power/ground zone generation and thermal relief support.
- Fanout for fine-pitch ICs and BGAs.
- Multi-layer stackups beyond two copper layers.
- Cost tuning from KiCad DRC feedback.
- AI-guided placement/routing repair loops.
- Interactive hints exported for KiCad users.

## Acceptance Criteria

The routing engine is ready for first use when:

1. It can route at least three golden small boards without KiCad DRC errors.
2. It emits deterministic route/via transaction operations.
3. Internal validation catches disconnected pads, clearance violations, and
   board-edge crossings.
4. The CLI can route a placement result and write a structured report.
5. AI-facing issue output explains route failures and repair suggestions.
6. Existing placement, PCB writer, transaction, and validation tests continue to
   pass.
7. Generated routed examples open in KiCad without format warnings introduced by
   routing output.
