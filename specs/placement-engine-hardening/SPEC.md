# Placement Engine Hardening Specification

## Purpose

Harden the existing deterministic placement engine so generated boards are not
merely collision-free, but physically plausible, block-aware, routing-friendly,
and repairable from validation feedback.

The current placement foundation can model components, bounds, groups, keepouts,
edge/side/rotation constraints, quality reports, diagnostics, and transaction
operations. This hardening project turns those pieces into a stricter layout
policy suitable for AI-generated schematic-plus-PCB workflows.

## Goals

1. Preserve deterministic placement for identical requests and seeds.
2. Place circuit blocks as coherent physical groups.
3. Respect connector edge/orientation constraints as first-class requirements.
4. Place decoupling, crystal, reset, regulator, op-amp feedback, and connector
   support parts near their electrical anchors.
5. Separate analog, digital, clock, high-current, and noisy regions where
   intent metadata is available.
6. Model board outline, margin, keepout, mounting-hole, and mechanical
   constraints consistently.
7. Score placement quality with route-length, congestion, edge, grouping,
   thermal, and validation-pressure metrics.
8. Feed placement issues back into repair workflows as structured, actionable
   diagnostics.
9. Keep placement output compatible with the existing transaction and writer
   pipeline.

## Non-Goals

- Global nonlinear PCB placement optimization.
- Full autorouting.
- RF layout synthesis.
- Thermal simulation.
- Differential-pair length tuning.
- KiCad GUI automation.
- Mutating imported/user-authored placements without explicit preservation
  support.

## Existing Foundation

The implementation already includes:

- `internal/placement` request/result models.
- Board area, component bounds, pads, groups, keepouts, side/edge/rotation
  constraints, rules, and metrics.
- Deterministic component placement and operation emission.
- Footprint geometry hydration from library and generated footprint data.
- Quality reports and placement diagnostics.
- Design workflow integration through `designworkflow.PlaceFragments`.
- Routing handoff through `routingadapters.RequestFromPlacement`.
- Repair-facing issue codes for outside-board and collision failures.

This hardening work should extend those pieces rather than introduce a parallel
placer.

## Placement Policy

### Determinism

Placement must remain reproducible for the same request, seed, libraries, and
rules.

Requirements:

- Stable sorting for components, groups, nets, keepouts, and candidate points.
- No map iteration leaks into placement order.
- Ties are broken by explicit priority, group ID, role, reference, and stable
  candidate index.
- Scores are numeric and deterministic; no random exploration unless a seed is
  explicitly used and recorded in result metadata.

### Block-Aware Grouping

The placer must treat circuit block metadata as layout intent.

Supported group behavior:

- Anchor component placement.
- Relative placement pattern preservation where blocks provide verified
  relative coordinates.
- Group spread constraints.
- Group spacing constraints.
- Group-level diagnostics when a block is too spread out, too close to another
  block, or forced outside its preferred area.

Initial block policies:

- LED indicator: resistor and LED inline and close.
- Regulator: regulator centered between input/output capacitors; power LED
  nearby but secondary.
- MCU minimal: MCU as anchor; decoupling capacitors near power pins; reset
  pull-up near reset pin; crystal/oscillator close when available.
- USB-C power: connector edge-facing; protection/power-path parts near
  connector; bulk capacitance downstream.
- I2C sensor: sensor near connector or MCU depending request role; pull-ups
  and decoupling close to sensor.
- Op-amp gain stage: feedback network near input pins; output resistor close to
  output; decoupling near supply pins.
- Connector breakout: connector at requested edge; routed pins fan out in a
  predictable row or column.

### Electrical Proximity

The placer must recognize electrical roles and apply proximity rules.

Required proximity categories:

- `decoupling`: capacitor near IC/regulator supply pins.
- `clock`: crystal/oscillator near MCU clock pins.
- `feedback`: feedback parts near op-amp feedback/input pins.
- `power_path`: regulator, connector, diode, fuse, and bulk capacitor chain.
- `pullup`: I2C pull-ups near the bus owner or sensor where requested.
- `programming`: programming header near MCU programming pins.
- `reset`: reset pull-up/button/header near MCU reset pin.

When exact pin coordinates are available, score distance from relevant pads.
When only component centers are available, score center-to-center distance and
mark the evidence as weaker.

### Connector And Edge Constraints

Connector placement must be stricter than general component placement.

Requirements:

- Edge-constrained components are placed before ordinary components.
- Edge distance and rotation are scored together.
- A connector with a fixed edge must fail when no legal candidate satisfies the
  edge.
- Edge-facing rotations must be derived from explicit constraints or connector
  role defaults.
- Connector groups must reserve keepout/fanout area inside the board.

### Mechanical Constraints

The placer must model mechanical layout constraints as blocking geometry.

Required constraints:

- Board outline/margin.
- Component keepouts.
- Mounting-hole keepouts.
- Connector clearance to board edge.
- Courtyard/component spacing.
- User/fixed-placement exclusion zones.

Future-compatible model:

- Rectangular constraints are required now.
- Arbitrary outline and polygon keepouts can be added later through the PCB
  geometry model.

### Region Separation

The placer should support coarse region intent:

- analog;
- digital;
- power/high-current;
- clock/noisy;
- user-facing/connector;
- thermal.

Initial implementation may use soft scoring rather than hard partitioning, but
the result must report whether region goals were satisfied or only approximated.

### Quality Scoring

Placement result quality must be inspectable.

Required score dimensions:

- board-fit score;
- collision/spacing score;
- group cohesion score;
- electrical proximity score;
- edge-constraint score;
- side/layer score;
- rough route-length score;
- congestion score;
- keepout/mechanical score;
- estimated-geometry penalty;
- validation-feedback pressure score.

Scores must produce:

- machine-readable totals;
- per-dimension values;
- per-component or per-group diagnostics when a score is poor;
- actionable suggestions for repair.

### Routing Handoff

Placement should improve routing readiness.

Requirements:

- Expose rough HPWL by net and total HPWL.
- Expose high-congestion areas where many nets cross the same coarse grid cell.
- Preserve placed pad summaries for routing.
- Emit warnings when placement makes required local routes implausible.
- Keep routing handoff compatible with `routingadapters.RequestFromPlacement`.

### Validation And Repair Feedback

Placement validation must produce structured issues that repair can classify.

Required issue behavior:

- Outside-board placement uses `reports.CodePlacementOutsideBoard`.
- Collision uses `reports.CodePlacementCollision`.
- Missing geometry blocks unless estimated bounds are explicitly allowed.
- Failed hard edge/side/rotation/keepout constraints block.
- Soft quality misses produce warning diagnostics.
- Issues include refs, operation IDs where available, and suggested repair
  actions.

Repair-facing actions:

- increase board size;
- move group together;
- move connector to requested edge;
- move decoupling capacitor near anchor;
- move component out of keepout;
- assign richer footprint geometry;
- retry placement with larger margin/spacing;
- relax non-critical soft constraints.

## Data Model Additions

### Placement Intent

Add or extend placement models with normalized intent:

```go
type IntentRole string

const (
    IntentDecoupling IntentRole = "decoupling"
    IntentClock      IntentRole = "clock"
    IntentFeedback   IntentRole = "feedback"
    IntentPowerPath  IntentRole = "power_path"
    IntentPullup     IntentRole = "pullup"
    IntentConnector  IntentRole = "connector"
    IntentThermal    IntentRole = "thermal"
)
```

Intent can be derived from:

- circuit block component roles;
- component catalog function metadata;
- net roles;
- explicit request constraints.

### Proximity Rule

```go
type ProximityRule struct {
    ID            string
    Source        string
    AnchorRef     string
    TargetRefs    []string
    AnchorPins    []string
    TargetPins    []string
    MaxDistanceMM float64
    Weight        int
    Required      bool
}
```

### Region Rule

```go
type RegionRule struct {
    ID        string
    Region    string
    Refs      []string
    NetRoles  []NetRole
    Preferred Rect
    Weight    int
    Required  bool
}
```

### Score Report

```go
type ScoreReport struct {
    Total      float64
    Dimensions []ScoreDimension
}

type ScoreDimension struct {
    Name       string
    Score      float64
    Weight     float64
    Status     string
    Refs       []string
    Groups     []string
    Nets       []string
    Message    string
    Suggestion string
}
```

Existing `QualityReport` may be extended instead of adding a separate top-level
type if that fits the codebase better.

## Workflow Integration

### Design Workflow

`design create` should expose placement hardening evidence in the placement
stage:

- quality summary;
- score dimensions;
- group diagnostics;
- proximity diagnostics;
- routing-readiness metrics;
- repair suggestions.

### Circuit Blocks

Block PCB realization metadata should be converted into placement intent:

- placement groups;
- required local routes;
- keepouts;
- edge constraints;
- role-specific proximity rules.

### Component Catalog

Component metadata should feed:

- estimated/fallback bounds policy;
- user-facing connector roles;
- power/analog/clock/noisy roles;
- placement priority.

### Repair Loop

Placement failures and weak placement diagnostics should be usable by:

- `validation_repair` workflow stage;
- persisted repair apply;
- future iterative generate/validate/repair loops.

## Acceptance Criteria

The project is complete when:

- placement is deterministic across repeated runs;
- common block examples produce coherent local layouts;
- edge connectors satisfy edge/orientation constraints or fail with structured
  blocking issues;
- decoupling and support parts are placed near anchors when metadata exists;
- group spread and proximity metrics appear in placement-stage output;
- impossible placement produces actionable issues instead of silent bad output;
- routing handoff includes enough evidence to diagnose long or congested nets;
- golden examples cover LED, regulator, MCU minimal, USB-C power, I2C sensor,
  op-amp gain, and connector breakout placement;
- `go test ./...` passes;
- Prism review has no unresolved high or medium findings.

## Risks

- Overfitting heuristics to current small examples.
- Making soft quality scores block too aggressively.
- Missing footprint geometry causing noisy false failures.
- Connector edge conventions differing across libraries.
- Adding placement rules that routing cannot yet exploit.

## Open Questions

- Should region partitioning be a hard constraint for some request acceptance
  levels?
- Should placement repair mutate original placement operations or produce a
  replacement transaction layer?
- How much KiCad DRC evidence should be required before a placement is called
  routing-ready?
- Should fabrication-ready acceptance forbid all estimated footprint bounds?
