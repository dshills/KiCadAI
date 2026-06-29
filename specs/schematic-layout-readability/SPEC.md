# Schematic Layout Readability Specification

Date: 2026-06-29

## Purpose

Make generated KiCad schematics readable by humans, not merely valid KiCad
files.

The current schematic generation pipeline can produce technically parseable
schematics, but the visual result may crowd components together, put power and
ground in unintuitive locations, route wires diagonally or through functional
groups, and place labels where they overlap symbols or wires. This makes the
output harder for engineers to review and harder for AI agents to explain,
repair, or extend.

This project adds a deterministic schematic layout layer and a readability
validator that enforce schematic drawing conventions across all generated
schematics.

## Background

KiCadAI already has:

- a transaction model for adding schematic symbols, wires, labels, and
  no-connects;
- direct `.kicad_sch` writer support;
- symbol resolver and pin metadata foundations;
- schematic-to-PCB transfer;
- circuit block metadata with component roles and block-level intent;
- first-pass improvements to the op-amp gain-stage block layout;
- design API wiring that can emit orthogonal two-point wire segments.

Those foundations are necessary, but not sufficient. Hardcoding better
coordinates inside individual blocks does not solve global schematic
readability because the final drawing must consider all blocks, connectors,
power rails, net roles, labels, and sheet boundaries together.

## Goals

1. Introduce an explicit schematic layout model between design transactions and
   `.kicad_sch` writing.
2. Place functional stages left to right by signal flow.
3. Place higher-voltage and positive supply rails toward the top of the
   drawing.
4. Place ground, return, and lower/negative rails toward the bottom.
5. Keep related parts grouped while preserving readable whitespace.
6. Route schematic wires orthogonally by default.
7. Prefer net labels over long, crossing, or cluttered wires.
8. Detect and report overlaps between symbols, properties, labels, junctions,
   and wires.
9. Keep output deterministic for stable diffs and reproducible AI workflows.
10. Provide machine-readable readability diagnostics in tests and CLI output.
11. Improve checked-in schematic examples through the same rules, not by
    one-off manual formatting.

## Non-Goals

- Building a full interactive schematic editor.
- Replacing KiCad's own schematic editing and cleanup tools.
- Solving every possible aesthetic convention in the first pass.
- Global graph optimization, stochastic layout, simulated annealing, or
  force-directed placement.
- Mutating imported user schematics by default.
- Splitting hierarchical sheets automatically in the first implementation.
- Guaranteeing fabrication readiness. This project is schematic presentation
  quality, not electrical correctness.

## Definitions

### Readable Schematic

A generated schematic is readable when:

- signal flow is visually apparent from left to right;
- power and return structure is visually apparent from top to bottom;
- components are not crowded;
- component references and values do not overlap nearby symbols or wires;
- wires are mostly horizontal or vertical;
- long-distance connections use labels when direct wiring would obscure
  intent;
- feedback loops, local bypassing, bias networks, and connector boundaries are
  easy to identify;
- the result can be inspected by a human without first rearranging it in KiCad.

### Layout Engine

The schematic layout engine consumes an intermediate schematic design model and
returns concrete coordinates, rotations, wire segments, label positions, and
diagnostic evidence.

### Readability Validator

The readability validator evaluates concrete schematic geometry and returns
warnings or errors for rule violations. It must run in tests and may also feed
CLI/workflow summaries.

## Existing Pipeline Fit

The layout engine should sit after design intent/block expansion and before
final schematic write:

```text
intent / blocks / design API
  -> schematic transaction model
  -> schematic layout model
  -> rule-based schematic layout
  -> readability validation
  -> .kicad_sch writer
  -> KiCad parse / round-trip / ERC where configured
```

The implementation should not bypass the existing writer. It should produce the
same schematic object types that the writer already understands.

## Data Model

Add a package, likely `internal/schematiclayout`, responsible for layout
planning and validation.

### Layout Input

The input model should represent:

- sheet size and drawing margins;
- components with references, values, roles, block IDs, library IDs, bounding
  boxes, pin anchors, and preferred rotation;
- nets with names, roles, endpoint pins, voltage domain, and signal direction
  hints;
- block groups with stage order, group role, and optional anchor components;
- explicit constraints from circuit blocks or design requests;
- existing coordinates where a caller has fixed a component;
- writer profile constraints such as grid and page size.

Recommended shape:

```go
type Request struct {
    Sheet      Sheet
    Components []Component
    Nets       []Net
    Groups     []Group
    Rules      Rules
}
```

The exact names may differ to fit existing package conventions.

### Layout Output

The result must include:

- positioned components;
- property and label positions;
- wire routes as KiCad-compatible two-point orthogonal segments;
- junction positions;
- labels created or moved by the layout pass;
- unreadable or unresolved conditions;
- deterministic evidence explaining major layout decisions.

Recommended shape:

```go
type Result struct {
    Components []PlacedComponent
    Wires      []WireSegment
    Labels     []Label
    Junctions  []Junction
    Diagnostics []Diagnostic
    Report     Report
}
```

## Coordinate And Grid Rules

Coordinates must be deterministic and KiCad-friendly:

- default grid: 2.54 mm for symbols and major wires;
- optional minor grid: 1.27 mm for labels and small offsets;
- default page: A4 unless the project explicitly requests another page size;
- minimum component-to-component body spacing: 10.16 mm;
- minimum text-to-symbol/wire spacing: 2.54 mm;
- minimum stage spacing: 25.4 mm;
- minimum group gutter: 12.7 mm;
- margin from title block and sheet border: at least 10.16 mm;
- all final schematic coordinates must snap to the selected grid unless a
  KiCad symbol pin anchor requires an existing compatible offset.

The first implementation may use conservative static spacing. Later versions
can adapt spacing from component count and sheet density.

## Placement Rules

### Signal Flow

The layout engine must infer or accept a stage order:

1. inputs and upstream connectors;
2. protection and conditioning;
3. power conversion or references if they feed signal stages;
4. processing, gain, logic, MCU, or sensor stages;
5. drivers, buffers, loads, and downstream connectors.

Components belonging to earlier stages should appear left of later stages.

Acceptance examples:

- an input connector appears left of an op-amp gain stage;
- an op-amp output resistor appears right of the op-amp;
- a headphone or output connector appears right of the final coupling or
  protection component;
- an I2C sensor appears right of the MCU or connector that drives it unless the
  request fixes another direction.

### Power And Return

Power symbols and supply-related components should follow vertical conventions:

- positive supply rails and input power symbols above the functional stage they
  feed;
- local decoupling above or near supply pins, with ground side routed down;
- ground symbols below the circuit element they return;
- negative rails below ground/reference or at the lower side of dual-supply
  stages;
- power entry and regulator blocks may be placed as a left-to-right subflow
  above or below the main signal path, but their rails must remain visually
  clear.

### Functional Groups

Components in the same circuit block should remain visually grouped:

- local passives near their active component;
- bias dividers lower or beside the affected input/reference node;
- feedback networks above or around op-amps;
- decoupling capacitors near supply pins;
- load/output protection near output connectors;
- oscillator/crystal components near oscillator pins;
- programming/reset parts near MCU pins but separated from unrelated buses.

Groups must not be packed so tightly that labels or values overlap.

### Connector Conventions

Connectors should be placed at visual boundaries:

- input connectors on the left;
- output connectors on the right;
- power connectors above-left or left/top depending on the design;
- programming connectors near MCU blocks but outside the main signal path;
- mechanical or user connectors near the sheet edge when practical.

### Analog Conventions

Analog and amplifier drawings should prefer:

- input coupling and bias before the active gain stage;
- feedback loop drawn above the op-amp where possible;
- output buffer/load path to the right;
- return/load components lower than the signal path;
- supply decoupling above/below the active device, not in the main signal lane;
- high-current or load labels placed clear of small-signal labels.

### Digital And Bus Conventions

Digital schematics should prefer:

- MCU/controller near the center-left or center depending on board role;
- sensors and peripherals to the right;
- programming/debug connectors at a side boundary;
- pullups near the bus they affect but not blocking the bus path;
- named bus labels for repeated or long bus connections;
- power decoupling vertically separated from signal pins.

## Routing Rules

### Orthogonal Wires

All generated schematic wires must be horizontal or vertical two-point KiCad
segments. Multi-segment paths must be represented as several two-point wire
objects.

### Elbow Strategy

For nearby point-to-point routes:

- use direct horizontal or vertical segments when pins align;
- otherwise use deterministic elbows;
- prefer elbows that avoid symbol bodies and label boxes;
- prefer horizontal signal lanes with vertical drops into pins;
- avoid routing through component bodies.

### Label Fallback

Use labels instead of long wires when:

- route length exceeds a configurable threshold;
- a route would cross unrelated functional groups;
- a route would pass through crowded regions;
- a net has many endpoints and a star of wires would be unreadable;
- a bus or power rail is shared across multiple stages.

The label fallback must preserve electrical connectivity by using identical
net names and compatible KiCad label types.

### Junctions

Junctions should only be emitted where a T-connection or multi-branch
connection requires one. The validator should flag redundant junctions and
missing junctions where the writer model can prove a branch exists.

## Property And Label Placement

The layout engine must place or adjust:

- reference text;
- value text;
- component identity properties that are visible;
- local/global labels;
- power labels;
- no-connect markers.

Rules:

- reference and value text must not overlap the symbol body or each other;
- text should sit near the symbol it describes;
- labels should sit on the associated wire segment or pin lane;
- repeated labels on the same net should be bounded;
- label text should not cover wires, symbols, or other labels.

The first version may use bounding-box approximations from symbol extents and
text length. It does not need perfect font rendering.

## Overlap Detection

The readability validator must detect:

- symbol body overlaps;
- symbol-to-wire body intersections;
- text-to-symbol overlaps;
- text-to-wire overlaps;
- label-to-label overlaps;
- component properties outside the usable sheet area;
- wires outside the usable sheet area;
- title-block overlap.

Severity guidance:

- `error`: symbol body overlap, wire through symbol body, outside sheet,
  unreadable title-block collision;
- `warning`: text overlap, crowded groups, long wire where label fallback would
  help, backward stage flow, dense area;
- `info`: cosmetic suggestions with no correctness impact.

## Readability Report

Every layout run should optionally return a report containing:

- component count;
- group count;
- routed net count;
- label fallback count;
- overlap counts by type;
- long-wire count;
- diagonal-wire count;
- stage-order violations;
- power/ground placement violations;
- sheet density estimate;
- whether the output passed the selected readability profile.

The report must be machine-readable and stable enough for golden tests.

## Profiles

Support at least these readability profiles:

- `off`: preserve existing behavior as much as possible;
- `basic`: orthogonal wires, spacing, and overlap diagnostics;
- `standard`: default profile for new generated schematics;
- `strict`: fail tests on warnings that would make examples hard to inspect.

The default for `design create` should become `standard` after the initial
foundation is implemented and validated.

## CLI And Workflow Integration

The `design create` workflow should expose schematic layout evidence:

- layout profile;
- readability pass/fail status;
- major warnings/errors;
- summary metrics;
- label fallback count;
- overlap count;
- stage-order/power placement violations.

Future CLI commands may expose a dedicated schematic layout check, but this is
not required for the first implementation.

## Example Regeneration

Existing examples should eventually be regenerated or normalized through the
same layout rules:

- simple LED indicator;
- button pull-up;
- RC filter;
- 555 timer;
- sensor node;
- Class AB headphone amplifier;
- Class A headphone amplifier;
- op-amp buffer headphone amplifier.

The amplifier examples are especially useful acceptance tests because they
exercise analog signal flow, feedback, bias, rails, output loads, and connector
boundaries.

## Testing Requirements

Add tests for:

- deterministic component placement from equivalent inputs;
- signal-flow stage ordering;
- power rail and ground vertical ordering;
- component spacing;
- orthogonal wire emission;
- label fallback for long/crowded nets;
- no symbol body overlaps;
- no text overlaps in representative fixtures;
- op-amp feedback drawn above or around the op-amp;
- connector placement at sheet boundaries;
- generated amplifier fixture readability.

Tests should start with unit fixtures and then add generated project goldens.
Optional KiCad-backed tests may remain environment gated.

## Determinism Requirements

The layout engine must be deterministic:

- stable ordering of components, nets, groups, wires, labels, and diagnostics;
- stable tie-breaks by stage, group, role, reference, and original operation
  order;
- no map iteration order in output;
- no randomness;
- bounded diagnostic output;
- stable UUID behavior through the existing generator when possible.

## Compatibility And Migration

The implementation must preserve existing APIs until callers are migrated.

Recommended migration:

1. Add layout package and validator without changing default writer behavior.
2. Add opt-in layout application in design API/workflow.
3. Make new generated schematic examples opt in.
4. Promote `standard` profile to default for generated projects.
5. Regenerate or rewrite checked-in examples once the validator is stable.

Imported/user-authored schematic mutation remains out of scope unless the
caller explicitly opts into generated-project rewrite behavior.

## Acceptance Criteria

This project is complete when:

1. Generated schematics can run through a deterministic readability layout pass.
2. The default generated design workflow reports schematic readability evidence.
3. Representative generated schematics have no diagonal wires.
4. Representative generated schematics have no symbol body overlaps.
5. Power and ground placement rules are enforced in tests.
6. Signal-flow left-to-right rules are enforced in tests.
7. Label fallback is used for at least one long/crowded-net fixture.
8. At least one amplifier schematic fixture passes strict readability checks.
9. Existing schematic parse/write tests continue to pass.
10. `go test ./...` passes.
