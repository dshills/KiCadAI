# Circuit Block PCB Realization Specification

## 1. Purpose

Circuit Block PCB Realization turns verified schematic-oriented circuit blocks
into reusable KiCad schematic + PCB fragments.

The goal is for each supported block to produce:

- schematic operations;
- footprint assignments;
- PCB placement intent;
- local routing intent;
- copper zones where required;
- board constraints and keepouts;
- validation expectations;
- machine-readable evidence that the realized block is electrically meaningful.

This is the next layer between the existing circuit block library and fully
autonomous board generation. A block should no longer merely say "these symbols
and nets exist"; it should also say "this is how the local PCB fragment should
look and how to prove it is correct."

## 2. Context

KiCadAI already has the required lower layers:

- `internal/blocks` defines block metadata, parameters, components, nets,
  schematic hints, PCB hints, validation rules, block requests, block outputs,
  and transaction emission.
- Built-in blocks exist for LED indicator, voltage regulator, MCU minimal,
  USB-C power, I2C sensor, op-amp gain stage, and connector breakout.
- The library resolver can load KiCad symbols and footprints, validate
  symbol-footprint assignments, and hydrate footprint geometry.
- The placement engine can turn block output into placement requests and emit
  `place_footprint` operations.
- The routing engine can route small local nets and emit route operations.
- `designapi.Builder` can assign footprints, place footprints, add zones, route
  boards, and write projects.
- `internal/schematicpcb` can transfer assigned schematic symbols and nets into
  PCB placement transactions.
- The PCB writer can produce KiCad-native `.kicad_pcb` files.
- `internal/boardvalidation` can validate PCB structural correctness,
  net-to-pad assignments, generated connectivity, unrouted nets, route
  completion, zones, and optional KiCad DRC evidence.

The missing piece is a block-level PCB realization contract that tells these
layers how to place, route, constrain, and validate block-local PCB fragments.

## 3. Goals

Circuit Block PCB Realization must:

- extend block definitions with explicit PCB realization data;
- keep schematic generation and PCB realization tied to the same block instance;
- generate deterministic placement and routing intent for the same block request;
- support block-local coordinate systems so blocks can be composed and moved as
  units;
- use resolver-backed footprint geometry whenever available;
- preserve existing block transaction output and extend it with PCB operations;
- produce reusable fragments that can be placed into a larger board;
- validate each realized block through connectivity-first board validation;
- expose enough structured metadata for AI agents to reason about why a block is
  ready, partial, or blocked;
- support progressive verification, from placement-only fragments to locally
  routed and DRC-evidenced blocks.

## 4. Non-Goals

This project does not need to:

- implement a global board floorplanner;
- fully autoroute entire products;
- guarantee fabrication readiness without KiCad DRC and human review;
- solve RF, high-speed, impedance-controlled, thermal, or safety-critical layout
  in the first pass;
- pick manufacturer-specific components or BOM sourcing;
- replace existing circuit block schematic instantiation;
- rewrite the placement or routing engines.

## 5. Definitions

### 5.1 PCB Realization

A PCB realization is a block-owned layout contract. It describes how a block's
components should be placed and locally routed relative to an origin.

It includes:

- footprint selection requirements;
- relative placements;
- relative keepouts;
- local routes;
- zones;
- net classes or route constraints;
- board-edge or connector orientation constraints;
- validation expectations.

### 5.2 Block-Local Coordinate System

Each realized block uses a local coordinate system in millimeters. The block
origin is placed onto a board by a higher-level composer. All component
placements, routes, keepouts, and zones are relative to that origin.

Rules:

- local X/Y coordinates are in millimeters;
- rotation is degrees clockwise in KiCad-compatible terms;
- the default PCB side is `F.Cu`;
- realized block output must record the transform applied to convert local
  geometry to board geometry;
- local coordinates must be deterministic and stable across runs.

### 5.3 Fragment

A fragment is the concrete schematic + PCB output for one block instance.

It may contain:

- block instance metadata;
- generated references and nets;
- transaction operations;
- placement request;
- routing request;
- realized placements;
- realized routes;
- realized zones;
- validation report;
- warnings or blockers.

### 5.4 Local Net

A local net is internal to the block and should usually be routed inside the
fragment. Examples:

- LED resistor-to-LED node;
- regulator feedback divider node;
- op-amp feedback node;
- USB-C CC resistor nets.

### 5.5 Exported Port Net

An exported port net connects the block to the larger design. It may be routed
locally to a connector, pad, test point, or edge-facing component, but final
inter-block routing belongs to the composition layer.

## 6. Required User-Facing Behavior

The block CLI should eventually support PCB realization directly:

```sh
kicadai --json block realize led_indicator --request request.json
```

and project generation:

```sh
kicadai --json \
  --request request.json \
  --output ./out/status_led \
  --overwrite \
  block realize led_indicator
```

Initial implementation may use existing `block instantiate` plus new Go APIs if
that is less disruptive, but the end state should provide a user-facing command
that returns a complete realization result.

The command should return a `reports.Result` with:

- block output;
- realization data;
- placement result;
- routing result;
- board-validation result;
- artifacts when a project is written;
- structured issues.

## 7. Data Model

Add PCB realization concepts to `internal/blocks`.

The exact Go shape may evolve, but it must preserve these concepts.

```go
type PCBRealization struct {
    Version              string
    VerificationLevel    VerificationLevel
    Components           []PCBComponentRealization
    Groups               []PCBPlacementGroup
    LocalRoutes          []PCBLocalRoute
    Zones                []PCBZoneRealization
    Keepouts             []PCBKeepout
    Constraints          []PCBConstraint
    Validation           PCBValidationExpectations
}
```

### 7.1 Component Realization

```go
type PCBComponentRealization struct {
    ComponentRole     string
    FootprintID       string
    Placement         RelativePlacement
    Bounds            *RelativeBounds
    Side              string
    Locked            bool
    PlacementGroup    string
    OrientationRole   string
    Constraints       []string
}
```

Required semantics:

- `ComponentRole` links to `BlockComponent.Role`.
- `FootprintID` must match the resolved/assigned footprint unless explicitly
  parameterized.
- `Placement` is relative to block origin.
- `Bounds` may come from resolver geometry, explicit block metadata, or
  estimated fallback.
- `OrientationRole` is a semantic hint such as `input_edge`, `output_edge`,
  `thermal_path`, `decoupling_near_pin`, or `user_connector`.

### 7.2 Relative Placement

```go
type RelativePlacement struct {
    XMM         float64
    YMM         float64
    RotationDeg float64
    Layer       string
}
```

Rules:

- rotations should be right-angle only unless the existing placement engine can
  validate the footprint at that angle;
- default layer is `F.Cu`;
- placements must be finite numbers;
- overlapping placements are invalid unless explicitly marked as mechanical or
  non-electrical.

### 7.3 Placement Groups

Placement groups preserve local topology and composition intent.

```go
type PCBPlacementGroup struct {
    ID          string
    Role        string
    Components  []string
    Anchor      *RelativePoint
    KeepTogether bool
    MaxSpreadMM *float64
}
```

Examples:

- regulator input capacitor near regulator VIN/GND pins;
- op-amp feedback resistor/capacitor near op-amp pins;
- USB-C CC resistors near the receptacle;
- decoupling capacitors near MCU power pins.

### 7.4 Local Routes

Local routes describe block-owned copper that should be generated inside the
fragment.

```go
type PCBLocalRoute struct {
    NetNameTemplate string
    From            RouteEndpoint
    To              []RouteEndpoint
    WidthMM         float64
    ClearanceMM     float64
    PreferredLayer  string
    Strategy        string
    Required        bool
}
```

`Strategy` values:

- `direct`: short deterministic route, usually one or two segments;
- `router`: pass endpoints into the routing engine;
- `zone`: connectivity is expected through a zone;
- `external`: exported net, not routed inside the block.

Rules:

- local internal nets with two or more endpoints should be routed or explicitly
  zone-dependent;
- exported nets may stop at component pads or connector pads;
- required local routes are blocking if unrouted;
- route output must become existing route/transaction operations, not a parallel
  copper model.

### 7.5 Zones

```go
type PCBZoneRealization struct {
    NameTemplate string
    NetNameTemplate string
    Layers       []string
    Polygon      []RelativePoint
    Priority     int
    Required     bool
}
```

Initial zones:

- regulator ground pour;
- USB-C shield or ground pour where appropriate;
- op-amp analog ground area only if explicitly modeled;
- connector breakout ground fill when requested.

Zone expectations:

- zones are explicit evidence, not a substitute for a route unless the net is
  marked `zone_dependent`;
- missing fill evidence is a warning by default and blocking under strict
  validation;
- KiCad DRC/refill remains the authoritative zone connectivity check.

### 7.6 Keepouts And Constraints

Keepouts describe block-local areas that placement and routing should avoid.

Constraints describe electrical or geometric rules.

Examples:

- keep copper away from USB-C connector shell mechanical region;
- keep regulator feedback node short;
- keep decoupling capacitors within a maximum distance from MCU power pins;
- require connector to face board edge;
- prefer input/output connectors on opposite sides of a regulator block.

Constraints should be machine-readable, even when the first implementation only
warns rather than enforces them.

## 8. Realization Pipeline

The realization pipeline should be deterministic and layered:

1. Instantiate block schematic operations using the existing block registry.
2. Resolve symbol and footprint requirements.
3. Validate symbol-footprint compatibility and pinmaps.
4. Build a block-local PCB realization from definition + request parameters.
5. Convert relative placements into placement components/operations.
6. Convert local route intents into routing requests or direct route operations.
7. Convert zones and keepouts into existing transaction/design API operations.
8. Optionally write a project.
9. Run connectivity-first board validation.
10. Return a single realization report.

The pipeline must preserve operation IDs and references where possible so AI
agents can map validation failures back to block roles.

## 9. Integration Points

### 9.1 Blocks

`BlockDefinition` should gain optional realization metadata, or the package
should provide a separate map keyed by block ID if keeping the existing model
stable is preferred.

Built-in block definitions should gradually move from generic `PCBHints` to
typed realization data.

### 9.2 Placement

The placement engine should receive block-local placement requests with:

- explicit component bounds from resolver geometry;
- placement groups;
- fixed or preferred relative coordinates;
- keep-together constraints;
- board-local transform support.

Existing `placement.RequestFromBlockOutput` can be extended or wrapped.

### 9.3 Routing

The routing engine should receive only local nets that the block owns. It should
not attempt to route arbitrary inter-block nets in this project.

Existing routing operations and `designapi.Builder.RouteBoard` should remain the
route application path.

### 9.4 Transactions

Realization output should produce existing transaction operations:

- `assign_footprint`;
- `place_footprint`;
- `route`;
- `add_zone`;
- `write_project`.

If new operations are needed, they must be justified by a gap in existing
operations.

### 9.5 Board Validation

Every realized block fixture should run `boardvalidation.Validate` or
`ValidateBoard`.

Validation expectations:

- structural validation passes;
- net-to-pad validation passes;
- generated connectivity passes for locally routed nets;
- local internal multi-pad nets are not unrouted;
- route completion passes for generated local routes;
- zone limitations are explicit;
- DRC evidence is optional in default tests.

## 10. Initial Block Requirements

### 10.1 LED Indicator

PCB realization must include:

- resistor and LED footprints;
- relative placement with short resistor-to-LED local route;
- input/output port orientation;
- optional current-limiting resistor package parameter;
- no required zone.

Acceptance:

- resistor-to-LED net is fully routed;
- exported supply/GND or input/output ports are preserved;
- board validation passes without DRC.

### 10.2 Voltage Regulator

PCB realization must include:

- regulator footprint;
- input capacitor near VIN/GND;
- output capacitor near VOUT/GND;
- optional feedback divider group;
- ground zone hint;
- input/output port orientation constraints.

Acceptance:

- local feedback and capacitor nets are routed or explicitly zone-dependent;
- decoupling placement constraints are reported;
- missing thermal evidence remains a warning unless a rule is strict.

### 10.3 MCU Minimal System

PCB realization must include:

- MCU footprint;
- decoupling capacitors near power pins;
- reset pull-up and programming connector where configured;
- crystal/resonator placement group if configured;
- boot/config resistor placement if configured.

Acceptance:

- decoupling caps are placed near MCU power pins;
- local reset/boot/config nets route;
- exported GPIO/power/programming ports remain available for composition.

### 10.4 USB-C Power

PCB realization must include:

- USB-C receptacle footprint;
- CC resistors placed near receptacle;
- VBUS and GND exported ports;
- shield handling as explicit policy;
- connector edge orientation constraint;
- keepout/mechanical region.

Acceptance:

- CC resistor nets are locally routed;
- VBUS/GND are available as exported ports;
- connector orientation/edge constraint is machine-readable;
- missing USB compliance evidence is explicit.

### 10.5 I2C Sensor

PCB realization must include:

- sensor footprint;
- pull-up resistors where requested;
- decoupling capacitor;
- connector or exported SDA/SCL/VCC/GND ports;
- optional address pin strapping.

Acceptance:

- pull-up and decoupling nets are locally meaningful;
- SDA/SCL exported nets remain available;
- address strap nets route when configured.

### 10.6 Op-Amp Gain Stage

PCB realization must include:

- op-amp footprint;
- feedback resistor network placed near op-amp;
- input/output passives where configured;
- supply decoupling caps;
- optional guard/keepout constraints for sensitive inputs.

Acceptance:

- feedback path is locally routed or explicitly short/direct;
- supply decoupling is placed and connected;
- analog sensitivity constraints are reported, even if not fully enforced.

### 10.7 Connector Breakout

PCB realization must include:

- connector footprint;
- optional pin labels/test points;
- fanout/local route stubs when requested;
- edge orientation constraints.

Acceptance:

- all connector pads have intentional nets;
- generated fanout routes validate;
- exported ports map to connector pins deterministically.

## 11. Verification Levels

Realized block verification should be separate from schematic-only block
verification.

Suggested levels:

- `pcb_unrealized`: block has no PCB realization.
- `pcb_placement_verified`: footprints and relative placements validate.
- `pcb_connectivity_verified`: local routes and net-to-pad connectivity validate.
- `pcb_drc_verified`: KiCad DRC evidence is available and clean or allowlisted.
- `pcb_reference_verified`: compared to a checked-in known-good reference.

The first autonomous generation target should require at least
`pcb_connectivity_verified` for all included internal block nets.

## 12. Reporting

Add a realization report:

```go
type RealizationReport struct {
    BlockID          string
    InstanceID       string
    Status           string
    Verification     VerificationRecord
    Operations       []transactions.Operation
    Placement        *placement.Result
    Routing          *routing.Result
    BoardValidation  *boardvalidation.Result
    Issues           []reports.Issue
    Artifacts        []reports.Artifact
}
```

Status values:

- `ready`;
- `partial`;
- `blocked`;
- `error`.

Reports must distinguish:

- unsupported block realization;
- missing footprint geometry;
- invalid pinmap;
- placement failure;
- routing failure;
- board-validation failure;
- missing optional KiCad DRC evidence.

## 13. Test Strategy

Default tests must not require external KiCad repositories, KiCad GUI, or real
`kicad-cli`.

Test layers:

- model validation tests for realization definitions;
- block-specific realization tests;
- placement request conversion tests;
- routing request conversion tests;
- transaction operation tests;
- generated PCB model validation tests;
- boardvalidation tests for realized fixtures;
- optional integration tests with library roots and KiCad CLI skipped by
  default.

Fixture policy:

- keep block PCB fixtures small;
- prefer generated fixtures over large checked-in boards;
- add golden signatures only for stable output such as operation order and net
  status, not full file text unless necessary.

## 14. Acceptance Criteria

The project is complete when:

- every initial built-in block has a PCB realization definition or an explicit
  unsupported realization report;
- LED indicator and connector breakout produce placement + local route output
  that passes board validation;
- at least one regulator-like block produces placements, zones, and local route
  evidence with explicit thermal/zone caveats;
- block realization reports use stable structured issues;
- CLI or Go API can generate a realized block project;
- docs explain what is verified and what remains heuristic;
- normal `go test ./...` passes without KiCad CLI.

## 15. Open Design Questions

1. Should block realization metadata live directly inside `BlockDefinition`, or
   in a separate `RealizationDefinition` registry keyed by block ID?
2. Should the first CLI be `block realize` or should `block instantiate` gain a
   `--with-pcb-realization` flag?
3. Should block-local routes be emitted as explicit `route` operations or routed
   through `designapi.Builder.RouteBoard` first?
4. How strict should missing resolver footprint geometry be for the first pass:
   blocking or estimated-bounds warning?
5. Which blocks should become `pcb_connectivity_verified` first after LED and
   connector breakout?
