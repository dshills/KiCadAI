# Placement Engine Specification

## Purpose

Build a deterministic, constraint-aware PCB placement engine that turns
schematic/design intent into usable PCB footprint placement.

The goal is not full autorouting or global physical optimization in the first
release. The goal is to produce stable, explainable footprint coordinates that:

- fit inside the board outline;
- avoid footprint/courtyard collisions;
- keep related components near each other;
- place connectors and user-facing parts on sensible board edges;
- preserve fixed user-authored placements;
- feed existing PCB writers, transactions, and ERC/DRC checks.

This engine is a required step toward AI-generated full schematic and PCB
projects. It bridges the current gap between "the writer can emit KiCad PCB
objects" and "an agent can create a board layout that is mechanically and
electrically meaningful."

## Current Context

The project already has:

- direct KiCad project, schematic, and PCB writers;
- a high-level design API with `PlaceFootprint`, `Route`, `AddZone`, and
  `WriteProject` operations;
- transaction operations for `place_footprint`, `route`, and `add_zone`;
- circuit blocks with PCB hints such as relative placement, side, rotation, and
  keep-close relationships;
- symbol-to-footprint mapping and footprint library parsing;
- PCB object correctness checks;
- KiCad CLI-backed ERC/DRC report parsing and AI-facing findings.

The placement engine should use those existing pieces. It should not introduce a
separate PCB file model or bypass the current writer.

## Goals

1. Define a placement input model that can be produced from transactions,
   circuit blocks, or design API state.
2. Extract footprint placement bounds from existing generated footprints and
   parsed library footprints.
3. Place all non-fixed footprints deterministically within a board outline.
4. Respect fixed placements, keepouts, board edge constraints, side constraints,
   rotation constraints, and component grouping hints.
5. Keep electrically related components close enough for later routing to be
   plausible.
6. Produce transaction operations or design API calls rather than writing KiCad
   PCB files directly.
7. Return structured placement reports, warnings, and blocking issues.
8. Provide validation hooks for internal geometry checks and KiCad DRC.

## Non-Goals

- Full automatic routing.
- Signal-integrity-aware placement.
- Thermal simulation.
- RF placement.
- High-density BGA fanout planning.
- Multi-board panelization.
- Fabrication export.
- Replacing KiCad's interactive placement tools.

These can be future phases once deterministic placement is reliable.

## Package Boundary

Add a new package:

```text
internal/placement
```

The package owns:

- placement request/response models;
- footprint bounds and pad-summary models;
- board-outline and keepout handling;
- deterministic placement algorithms;
- placement validation;
- conversion to transaction operations.

It must not write `.kicad_pcb` files directly. It should return either:

- `transactions.Operation` values, primarily `place_footprint`; or
- a placement report that callers can apply through existing design API or
  transaction paths.

## Input Model

The placement engine should accept a `Request`.

```go
type Request struct {
    Board           BoardPlacementArea
    Components      []Component
    Nets            []Net
    Groups          []Group
    Keepouts        []Keepout
    Rules           Rules
    Existing        ExistingPlacementPolicy
    Seed            string
}
```

### BoardPlacementArea

The first implementation should support rectangular boards.

```go
type BoardPlacementArea struct {
    WidthMM  float64
    HeightMM float64
    Origin   Point
    MarginMM float64
}
```

Coordinate convention:

- units are millimeters at the placement API boundary;
- `BoardPlacementArea.Origin` is the board area's top-left point in global KiCad
  board coordinates;
- placement search coordinates are local to the board area, where local `(0, 0)`
  means `BoardPlacementArea.Origin`;
- emitted transaction coordinates must be global KiCad board coordinates:
  `global = BoardPlacementArea.Origin + local`;
- `X` increases to the right;
- `Y` increases downward, matching KiCad's displayed board coordinate behavior;
- internal calculations and comparisons must use fixed precision integer
  micrometers (`int64`) where practical; convert to/from `float64` millimeters
  only at API and transaction boundaries.

`MarginMM` must be less than half of both `WidthMM` and `HeightMM`; otherwise the
usable placement area is empty and validation must block placement.

Future versions may support arbitrary outlines by reusing `pcb.Drawing` edge
cuts or parsed board outlines.

### Component

```go
type Component struct {
    Ref          string
    Value        string
    FootprintID  string
    Role         string
    Bounds       Bounds
    Pads         []PadSummary
    Fixed        bool
    Position     *Placement
    Side         SideConstraint
    Rotation     RotationConstraint
    Edge         EdgeConstraint
    GroupID      string
    Priority     int
    Hints        []Hint
}
```

Rules:

- `Ref` is required.
- `FootprintID` is required unless a caller supplies explicit `Bounds`.
- `Bounds` must be known before placement.
- `Fixed` components keep their provided `Position`.
- `Priority` decides placement order when constraints are otherwise equal.

### Bounds

Bounds should describe the footprint occupied area.

```go
type Bounds struct {
    WidthMM       float64
    HeightMM      float64
    CourtyardMM   float64
    AnchorOffset  Point
    Source        BoundsSource
}
```

`AnchorOffset` is the vector from the top-left corner of the non-rotated
bounding box to the component's placement origin. Rotation occurs around the
component origin.

Occupied bounds for a placed component are computed by:

1. Build the non-rotated rectangle relative to the footprint origin:
   `left = -AnchorOffset.X`, `top = -AnchorOffset.Y`,
   `right = left + WidthMM`, `bottom = top + HeightMM`.
2. Rotate the rectangle corners around `(0,0)` by `RotationDeg`.
3. Take the axis-aligned bounding box of the rotated corners.
4. Translate that box by the component placement point.

This makes non-centered footprint anchors deterministic for 90/180/270 degree
rotations.

Bounds source values:

- `library_courtyard`
- `library_pads`
- `generated_pads`
- `explicit`
- `estimated`

`estimated` bounds are allowed for early AI planning, but they must produce a
warning and cannot be used for fabrication-ready placement.

### PadSummary

```go
type PadSummary struct {
    Name     string
    Net      string
    XMM      float64
    YMM      float64
    WidthMM  float64
    HeightMM float64
}
```

Pad summary coordinates are relative to the component anchor before placement,
not absolute board coordinates. They are needed for later grouping, decoupling
rules, rough routing cost, and routing-engine handoff.

### Net

```go
type Net struct {
    Name       string
    Endpoints  []Endpoint
    Role       NetRole
    Weight     int
    WidthClass string
}
```

Net roles:

- `power`
- `ground`
- `signal`
- `clock`
- `analog`
- `differential`
- `unknown`

The first placement engine should use net roles only for grouping and rough
cost. It should not attempt controlled-impedance placement.

Differential nets are special: the first engine must either keep both sides of a
differential pair in the same group with aligned placement constraints or return
a warning that differential-pair placement is not verified. It must not silently
treat differential pairs as unrelated ordinary signal nets.

### Group

```go
type Group struct {
    ID          string
    Role        string
    Components  []string
    Anchor      GroupAnchor
    KeepTogether bool
    MaxSpreadMM float64
    Priority    int
}
```

Common groups:

- regulator plus input/output capacitors;
- MCU plus decoupling capacitors;
- USB-C connector plus ESD/protection parts;
- op-amp plus feedback network;
- connector breakout rows;
- sensor plus pullups and decoupling.

### Keepout

```go
type Keepout struct {
    ID       string
    Bounds   Rect
    Layers   []string
    Reason   string
}
```

The first engine can treat keepouts as 2D rectangles affecting all components.

### Rules

```go
type Rules struct {
    GridMM                 float64
    ComponentSpacingMM     float64
    BoardEdgeClearanceMM   float64
    GroupSpacingMM         float64
    PreferTopLayer         bool
    AllowBackLayer         bool
    ConnectorEdgeClearanceMM float64
}
```

Defaults:

- grid: `0.5 mm`;
- component spacing: `0.5 mm`;
- board edge clearance: `1.0 mm`;
- group spacing: `1.0 mm`;
- prefer top layer: true;
- allow back layer: false for first release.

## Output Model

```go
type Result struct {
    Status      Status
    Placements  []PlacementResult
    Issues      []reports.Issue
    Metrics     Metrics
    Operations  []transactions.Operation
}
```

Status values:

- `placed`
- `partial`
- `blocked`

### PlacementResult

```go
type PlacementResult struct {
    Ref         string
    FootprintID string
    Position    Placement
    Bounds      Rect
    Fixed       bool
    GroupID     string
    Reason      string
}
```

### Placement

```go
type Placement struct {
    XMM         float64
    YMM         float64
    RotationDeg float64
    Layer       string
}
```

The first engine should restrict rotations to discrete PCB-friendly angles:
`0`, `90`, `180`, and `270` degrees unless a later phase adds arbitrary polygon
collision support.

### Metrics

```go
type Metrics struct {
    ComponentCount      int
    PlacedCount         int
    FixedCount          int
    UnplacedCount       int
    CollisionCount      int
    OutsideOutlineCount int
    EstimatedBoundsCount int
    HPWLMM              float64
}
```

`HPWLMM` is half-perimeter wire length over known nets using a 2D projection of
placed pad/component anchor coordinates. The first implementation may use zero
via penalty, but the model should allow a configurable via penalty in
millimeters when endpoints are on opposite sides in later phases.

## Placement Strategy

The first engine should be deterministic and rule-based.

### Stage 1: Normalize Inputs

- Validate board dimensions.
- Apply default rules.
- Normalize references and group IDs.
- Resolve missing side/rotation defaults.
- Validate fixed components.
- Compute padded component bounds from `Bounds` plus spacing rules.
- Validate that board margins leave positive usable area.
- Sort components deterministically by:
  1. fixed before free;
  2. group priority;
  3. component priority;
  4. role order;
  5. reference designator.

### Stage 2: Place Fixed Components

- Add fixed components to the occupancy map.
- Fail if a fixed component is outside the board or collides with another fixed
  component unless the caller explicitly allows preservation conflicts.
- Treat fixed components as obstacles for free placement.

### Stage 3: Edge Components

Place components with edge constraints first.

Examples:

- USB-C, barrel jack, and terminal blocks on board edges.
- User buttons and LEDs near accessible edges.
- Programming connectors on an edge unless explicitly internal.

The first implementation can support:

- `left`
- `right`
- `top`
- `bottom`
- `any`

### Stage 4: Group Placement

Create group boxes from grouped components.

For each group:

- choose a group anchor;
- place the primary component first;
- place keep-close components around it using small deterministic patterns;
- enforce `MaxSpreadMM` where provided;
- report a blocking issue if required group constraints cannot fit.

Early placement patterns:

- regulator: input connector/cap, regulator, output cap/load direction;
- MCU: MCU centered, decoupling caps near power pins, programming connector on
  edge;
- op-amp: op-amp centered, feedback network near inverting input;
- connector breakout: row-aligned connector plus support parts.
- differential pair endpoints: same group, mirrored/aligned orientation where
  footprint geometry allows it; otherwise warn that differential placement is
  unverified.

### Stage 5: Remaining Components

Use a deterministic shelf/grid placer:

- scan valid grid positions inside the board area;
- cap candidate scans with a documented maximum candidate count per component
  and expand search in deterministic bands rather than scanning every fine-grid
  point on large boards;
- implementation should prefer shelf rows, occupied-rectangle intervals, or a
  hierarchical/free-rectangle search over naive full-board point scans;
- reject positions outside board, inside keepout, or colliding;
- score candidate positions using:
  - rough net length to already placed connected components;
  - group distance;
  - distance from preferred edge;
  - board center/edge preference by role;
  - stable tie-breakers.

### Stage 6: Validate

Internal validation must run before returning operations:

- every required component placed;
- no component bounds overlap;
- no component outside board placement area;
- fixed placements preserved;
- side constraints satisfied;
- layer constraints satisfied, including rejecting `B.Cu` placement when
  `Rules.AllowBackLayer` is false;
- rotation constraints satisfied;
- edge constraints satisfied;
- keepouts respected;
- estimated bounds reported;
- unresolved footprint bounds reported.

KiCad DRC is a later validation layer. The placement engine should provide
enough structured data for `kicadai check drc` to be run after PCB writing.

## Transaction Output

The placement engine should produce `transactions.PlaceFootprintOperation`
operations:

```json
{
  "op": "place_footprint",
  "ref": "U1",
  "footprint_id": "Package_QFP:LQFP-48_7x7mm_P0.5mm",
  "at": {"x_mm": 35.0, "y_mm": 25.0},
  "rotation_deg": 0,
  "layer": "F.Cu"
}
```

It should not emit `route` operations. The routing engine will consume placement
and pad summaries later.

## Integration Points

### Design API

The placement engine should eventually be callable from the high-level design
API:

```go
placements, err := placement.Place(request)
```

Callers can then apply returned operations through the existing transaction
pipeline.

### Circuit Blocks

Block `PCBHints` should map into placement components, groups, edge constraints,
and keep-close rules.

The engine must not assume every block has perfect hints. Missing hints should
fall back to deterministic placement and produce warnings when quality is likely
poor.

### Library Resolver

The placement engine should use library footprint geometry when available:

- pads;
- courtyard drawings;
- fab outline;
- text extents only when needed;
- fallback to generated pad extents;
- fallback to estimated package sizes only with warnings.

### ERC/DRC Feedback Loop

Placement results should be tested internally first, then written to PCB and
checked through KiCad DRC where available. DRC findings should be correlated back
to placement components when possible.

## CLI Surface

Initial CLI command:

```sh
kicadai --json place request <request.json>
```

`place project <project-or-transaction>` can be added later once imported
project mapping is reliable.

Recommended flags:

- `--output`
- `--request`
- `--overwrite`
- `--seed`
- `--library-cache`
- `--symbols-root`
- `--footprints-root`
- `--keep-artifacts`
- `--artifact-dir`

All filesystem path flags must be cleaned with `filepath.Clean`. Hosted or
automated callers should restrict library roots to configured safe directories
and verify with `filepath.Rel` that resolved paths do not escape those roots.
The CLI must not use shell execution for path handling.

The first implementation may expose only a package API and tests. CLI exposure
should come once the request model is stable.

## Validation And Readiness

A generated design cannot be called placement-ready unless:

- every on-board symbol has a footprint;
- every required footprint has known bounds;
- every required footprint is placed;
- no component overlaps;
- all components are inside the board outline;
- fixed components are preserved;
- no keepout is violated;
- output applies cleanly through transactions;
- PCB writer validation passes.

A design cannot be called fabrication-ready unless placement readiness is
combined with routing, ERC/DRC, round-trip, pinmap, and fabrication-output
evidence.

## Open Questions

1. Should placement operate primarily on transaction files, design API state, or
   imported KiCad projects first?
2. How should arbitrary board outlines be represented before the PCB reader fully
   models all edge-cut variants?
3. Which KiCad footprint geometry should be the primary bounds source:
   courtyard, fab, pads, or a hybrid?
4. Should estimated bounds block placement readiness or only fabrication
   readiness?
5. How soon should the CLI expose placement directly versus using it inside
   block generation?
