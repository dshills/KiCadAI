# SPEC.md

# KiCad AI Design CLI Specification

## 1. Purpose

This project provides a command-line interface and structured API for creating, modifying, validating, and exporting KiCad schematic and PCB designs through machine-readable commands. The system is intended to allow AI coding/design agents to participate directly in electronic design workflows without manually editing KiCad files as unstructured text.

The core idea is simple:

```text
AI intent
  -> structured design schema / command transaction
  -> deterministic KiCad adapter
  -> ERC / DRC / render / netlist verification
  -> AI review and correction loop
```

The tool should make AI-assisted PCB design practical, repeatable, reviewable, and safe enough for real prototyping work.

## 2. Design Philosophy

The system must be **constraint-first** and **verification-heavy**.

The AI should not primarily freehand raw KiCad schematic or board files. Raw KiCad files are output artifacts and import/export targets, not the canonical reasoning interface.

The AI-facing interface should express electrical and layout intent:

- place this decoupling capacitor near this IC power pin
- keep this feedback loop short
- route this output trace with a wider net class
- keep input traces away from output traces
- connect the Zobel network return to power/load ground, not input ground
- run ERC/DRC and report failures in a structured form

The system should then convert that intent into KiCad-native files and prove the resulting design is sane using deterministic checks.

## 3. Goals

### 3.1 Primary Goals

- Provide CLI access to create and modify KiCad schematics.
- Provide CLI access to create and modify KiCad PCB layouts.
- Provide a structured canonical schema for components, nets, footprints, placement intent, routing constraints, and verification results.
- Allow AI agents to generate KiCad projects without directly editing KiCad internals as their primary workflow.
- Support transactional design modifications with machine-readable success/failure results.
- Run KiCad ERC and DRC checks programmatically.
- Generate reviewable artifacts including schematic previews, PCB previews, netlists, BOMs, Gerbers, drill files, pick-and-place files, and validation reports.
- Support analog/audio layout rules beyond generic DRC.
- Make generated designs auditable and reproducible.

### 3.2 Secondary Goals

- Support partial manual workflows where humans can open the generated KiCad project, adjust it, and round-trip changes back into the tool.
- Support design review tools such as SpecCritic, PlanCritic, Prism, or future board-specific critics.
- Provide a foundation for AI-assisted board layout planning, schematic review, manufacturability checks, and test-point coverage analysis.
- Support reusable component blocks/modules, such as op-amp input stages, split power supplies, Class AB output stages, protection circuits, and connector patterns.

## 4. Non-Goals

The project does not initially attempt to:

- Replace KiCad’s GUI.
- Become a full commercial ECAD platform.
- Guarantee that generated circuits are electrically correct without review.
- Automatically design arbitrary circuits from vague natural language alone.
- Perform full analog simulation unless integrated with an external simulator.
- Perform perfect autorouting for analog boards.
- Certify designs for safety, EMC, regulatory compliance, or production release.
- Hide KiCad from the user. KiCad remains the canonical editor and final review tool.

## 5. Target Users

- Electronics hobbyists moving from breadboards/point-to-point wiring to PCBs.
- Engineers who want scriptable KiCad workflows.
- AI coding agents that need a deterministic PCB generation backend.
- Open-source hardware developers.
- Analog/audio designers who want layout linting and repeatable prototype generation.
- Educators building structured electronics labs and assignments.

## 6. Example Use Cases

### 6.1 Audio Amplifier Prototype

A user describes a small split-supply headphone amplifier. The AI generates:

- schematic
- footprints
- net classes
- placement constraints
- decoupling placement
- star-ground strategy
- test points
- initial PCB layout
- ERC/DRC report
- Gerber package

The tool warns if:

- feedback traces are too long
- input and output traces run too close in parallel
- decoupling capacitors are too far from op-amp power pins
- output/load return current shares the input ground path
- Zobel return connects to small-signal ground instead of power/load ground

### 6.2 Op-Amp Filter Board

The AI creates a small board with:

- input protection
- op-amp stage
- configurable gain resistors
- capacitor footprints for tuning
- BNC or 3.5 mm connectors
- test points

The user can request alternate cutoff frequencies and regenerate the design.

### 6.3 Power Supply Board

The AI creates a linear regulator board with:

- input connector
- rectifier or DC input path
- regulator footprints
- bulk capacitance
- decoupling
- LEDs
- test points
- thermal spacing rules

## 7. Architecture Overview

```text
+-----------------------------+
| AI Agent / Human CLI User    |
+-------------+---------------+
              |
              v
+-----------------------------+
| Command / Transaction Layer  |
+-------------+---------------+
              |
              v
+-----------------------------+
| Canonical Design Model       |
| - schematic graph            |
| - components                 |
| - nets                       |
| - footprints                 |
| - constraints                |
| - placement intent           |
| - routing intent             |
+-------------+---------------+
              |
              v
+-----------------------------+
| KiCad Adapter                |
| - generate schematic         |
| - generate PCB               |
| - import KiCad files         |
| - sync design state          |
+-------------+---------------+
              |
              v
+-----------------------------+
| Verification Engine          |
| - ERC                        |
| - DRC                        |
| - netlist checks             |
| - layout lint                |
| - manufacturing checks       |
+-------------+---------------+
              |
              v
+-----------------------------+
| Artifacts                    |
| - .kicad_sch                 |
| - .kicad_pcb                 |
| - previews                   |
| - reports                    |
| - Gerbers                    |
| - BOM                        |
+-----------------------------+
```

## 8. Canonical Project Structure

A generated project should use a predictable structure:

```text
project-root/
  SPEC.md
  board.json
  schematic.graph.json
  components.json
  footprints.json
  constraints.json
  placement.json
  routing.json
  verification/
    erc.json
    drc.json
    layout-lint.json
    manufacturing.json
  kicad/
    project.kicad_pro
    project.kicad_sch
    project.kicad_pcb
  previews/
    schematic.svg
    pcb-front.svg
    pcb-back.svg
    pcb-3d.png
  fabrication/
    gerbers/
    drill/
    bom.csv
    positions.csv
  logs/
    transactions.jsonl
```

## 9. Canonical Data Model

### 9.1 Project Metadata

```json
{
  "project": {
    "name": "split-supply-headphone-amp",
    "version": "0.1.0",
    "units": "mm",
    "kicad_version": "target-configurable",
    "created_by": "kicad-ai-cli",
    "design_intent": "Small analog audio amplifier prototype board"
  }
}
```

### 9.2 Component Model

Each component must have:

- reference designator
- logical type
- value
- symbol
- footprint
- pin-to-net mapping
- placement constraints
- metadata

Example:

```json
{
  "ref": "R12",
  "type": "resistor",
  "value": "22k",
  "symbol": "Device:R",
  "footprint": "Resistor_THT:R_Axial_DIN0207_L6.3mm_D2.5mm_P7.62mm_Horizontal",
  "pins": {
    "1": "AMP_OUT",
    "2": "OPAMP_NEG"
  },
  "roles": ["feedback", "small_signal"],
  "constraints": {
    "place_near": ["U1"],
    "max_distance_mm": 12
  }
}
```

### 9.3 Net Model

Each net must include:

- name
- type
- voltage/current expectations when known
- net class
- electrical role
- routing constraints

Example:

```json
{
  "name": "AMP_OUT",
  "type": "signal",
  "role": "high_current_output",
  "net_class": "output_power",
  "expected_voltage": {
    "min": -6,
    "max": 6,
    "unit": "V"
  },
  "constraints": {
    "min_width_mm": 1.0,
    "avoid_parallel_with": ["INPUT_SIGNAL", "OPAMP_POS"],
    "keepout_distance_mm": 3.0
  }
}
```

### 9.4 Footprint Assignment

Footprint assignment must explicitly track pin-map verification.

```json
{
  "ref": "Q3",
  "footprint": "Package_TO_SOT_THT:TO-92_Inline",
  "pinmap_verified": true,
  "pinmap_source": "datasheet",
  "notes": "Verify E/B/C orientation for selected transistor before fabrication."
}
```

A design must not be considered fabrication-ready if critical component pin maps are unverified.

## 10. Command and Transaction Model

All design modifications should be performed as transactions.

### 10.1 Transaction Requirements

Each transaction must include:

- transaction ID
- operation list
- preconditions
- expected postconditions
- validation steps
- result status
- artifact references

Example:

```json
{
  "transaction_id": "amp-layout-0042",
  "description": "Place op-amp decoupling capacitor near positive rail pin",
  "operations": [
    {
      "type": "place_component",
      "ref": "C5",
      "strategy": "near_pin",
      "target_ref": "U1",
      "target_pin": "V+",
      "max_distance_mm": 3
    }
  ],
  "validate": ["placement", "drc", "layout_lint"]
}
```

### 10.2 Transaction Result

```json
{
  "transaction_id": "amp-layout-0042",
  "status": "failed",
  "errors": [
    {
      "code": "COMPONENT_OVERLAP",
      "message": "C5 overlaps R8",
      "refs": ["C5", "R8"]
    }
  ],
  "warnings": [
    {
      "code": "DECOUPLING_DISTANCE_MARGIN",
      "message": "C5 is 2.9 mm from U1 V+ pin; within limit but close to threshold."
    }
  ],
  "artifacts": {
    "pcb_preview": "previews/pcb-front.svg",
    "drc_report": "verification/drc.json"
  }
}
```

## 11. CLI Requirements

The CLI should be deterministic, scriptable, and friendly to both humans and AI agents.

### 11.1 Proposed Command Structure

```text
kicad-ai init
kicad-ai add-component
kicad-ai connect
kicad-ai assign-footprint
kicad-ai set-constraint
kicad-ai place
kicad-ai route
kicad-ai render
kicad-ai erc
kicad-ai drc
kicad-ai lint-layout
kicad-ai export
kicad-ai transaction apply
kicad-ai transaction rollback
kicad-ai inspect
kicad-ai diff
```

### 11.2 Example Commands

```bash
kicad-ai init --name split-supply-headphone-amp

kicad-ai add-component \
  --ref R12 \
  --type resistor \
  --value 22k \
  --symbol Device:R

kicad-ai connect --ref R12 --pin 1 --net AMP_OUT
kicad-ai connect --ref R12 --pin 2 --net OPAMP_NEG

kicad-ai assign-footprint \
  --ref R12 \
  --footprint Resistor_THT:R_Axial_DIN0207_L6.3mm_D2.5mm_P7.62mm_Horizontal

kicad-ai erc --json verification/erc.json
kicad-ai drc --json verification/drc.json
kicad-ai render --pcb --output previews/pcb-front.svg
```

## 12. AI-Facing API Requirements

The AI-facing API should accept JSON transactions and return structured JSON results.

### 12.1 Design Creation

```json
{
  "action": "create_project",
  "project": {
    "name": "headphone-amp-v1",
    "units": "mm",
    "board_shape": {
      "type": "rectangle",
      "width_mm": 100,
      "height_mm": 70
    }
  }
}
```

### 12.2 Component Placement Intent

```json
{
  "action": "place_component",
  "ref": "C7",
  "strategy": "near_pin",
  "target_ref": "U1",
  "target_pin": "V+",
  "max_distance_mm": 3,
  "preferred_side": "front"
}
```

### 12.3 Routing Constraint

```json
{
  "action": "route_constraint",
  "net": "INPUT_SIGNAL",
  "preferred_layer": "F.Cu",
  "avoid_nets": ["AMP_OUT", "+6V", "-6V"],
  "max_length_mm": 35,
  "min_clearance_mm": 0.5
}
```

### 12.4 Verification Request

```json
{
  "action": "verify",
  "checks": ["erc", "drc", "layout_lint", "manufacturing"],
  "render_artifacts": true
}
```

## 13. Schematic Generation Requirements

The tool must be able to:

- create a KiCad project
- create schematic sheets
- add symbols
- assign references
- assign values
- connect nets
- add net labels
- add power symbols
- add hierarchical sheets
- annotate schematic
- run ERC
- export schematic previews
- export netlists

### 13.1 Schematic Correctness Checks

The system must detect:

- unconnected pins
- duplicate reference designators
- missing values
- missing footprints
- power input pins without drivers
- conflicting net names
- pin-type conflicts
- single-pin nets, unless intentionally marked
- unverified transistor/op-amp/power-device pin mappings

## 14. PCB Generation Requirements

The tool must be able to:

- create a board outline
- import/update from schematic netlist
- place footprints
- create zones
- create copper traces
- create vias
- set net classes
- set design rules
- add silkscreen labels
- add mounting holes
- add test points
- run DRC
- render board previews
- export fabrication artifacts

### 14.1 Board Outline

Board outline should support at least:

- rectangle
- rounded rectangle
- custom polygon
- imported DXF/SVG outline, optional future feature

### 14.2 Net Classes

Minimum default net classes:

```text
small_signal
feedback
power_rail
output_power
ground
chassis_earth
high_voltage
noisy_switching
sensitive_input
```

Each class should define:

- trace width
- clearance
- via size
- differential or special constraints when needed
- preferred routing layers

## 15. Placement Constraint System

The placement system must support semantic constraints.

### 15.1 Placement Strategies

Required strategies:

- absolute coordinate
- relative to board edge
- near component
- near pin
- grouped with components
- mirrored/symmetric placement
- keep away from component/net/group
- place in region
- place by functional block

Example:

```json
{
  "ref": "C3",
  "strategy": "near_pin",
  "target_ref": "U1",
  "target_pin": "V-",
  "max_distance_mm": 3,
  "reason": "Negative rail decoupling must be close to op-amp power pin."
}
```

### 15.2 Functional Blocks

The design model should support named functional blocks:

```json
{
  "block": "input_filter",
  "refs": ["J1", "RV1", "C1", "R1", "R2", "C2"],
  "region": {
    "x_min_mm": 0,
    "x_max_mm": 25,
    "y_min_mm": 0,
    "y_max_mm": 70
  },
  "constraints": {
    "keep_away_from_blocks": ["output_stage", "power_entry"]
  }
}
```

## 16. Routing Constraint System

Routing should support both manual intent and automated/semi-automated routing.

### 16.1 Required Routing Rules

The system must support:

- min/max trace width
- max length
- preferred layer
- avoid net/group
- route as pair
- star connection
- kelvin sense connection
- no vias
- max vias
- keepout areas
- copper zone assignment
- controlled return path
- output/load current separation

### 16.2 Star Ground Constraint

Example:

```json
{
  "type": "star_ground",
  "star_net": "0V",
  "star_point_ref": "TP_GND_STAR",
  "branches": [
    {
      "name": "input_ground",
      "members": ["J1.GND", "R_INPUT.GND", "U1.INPUT_REF"]
    },
    {
      "name": "power_ground",
      "members": ["C_BULK_POS.GND", "C_BULK_NEG.GND", "PSU.0V"]
    },
    {
      "name": "load_return",
      "members": ["J_OUT.GND", "ZOBEL.GND"]
    }
  ]
}
```

## 17. Verification Engine

Verification is mandatory. A project must not be marked fabrication-ready until all required checks pass or are explicitly waived.

### 17.1 Required Checks

- KiCad ERC
- KiCad DRC
- unconnected net check
- footprint assignment check
- pin-map verification check
- design rule check
- board outline check
- manufacturing export check
- layout lint check
- artifact generation check

### 17.2 Waivers

Warnings may be waived only with explicit rationale.

```json
{
  "waiver_id": "W-0007",
  "check": "SINGLE_PIN_NET",
  "net": "TEST_PAD_UNUSED",
  "rationale": "Intentional spare test pad for future measurement.",
  "approved_by": "human"
}
```

## 18. Analog and Audio Layout Lint

The system must include analog/audio-specific layout checks. These checks are not replacements for engineering judgment, but they should catch common prototype-killing mistakes.

### 18.1 Required Audio/Analog Checks

Warn when:

- input trace runs close and parallel to output trace
- feedback trace is too long
- feedback trace runs near output current path
- op-amp decoupling capacitor is too far from supply pin
- output transistor decoupling capacitor is too far from transistor supply node
- Zobel return connects to small-signal/input ground instead of output/load ground
- output/load current return shares long trace with input ground
- power rail traces are too narrow for declared current
- emitter/source resistors lack Kelvin/test access
- bias diodes are far from output devices when thermal tracking is desired
- split-supply midpoint/0V node is ambiguous
- input impedance node is physically large
- high-impedance nodes lack clearance from noisy nets
- ground plane is fragmented under sensitive analog sections
- no local bulk capacitance near output stage
- no test point exists for each power rail
- no test point exists for amplifier output
- no test point exists for bias current measurement

### 18.2 Example Lint Finding

```json
{
  "code": "FEEDBACK_LOOP_TOO_LONG",
  "severity": "warning",
  "message": "Feedback path from AMP_OUT to OPAMP_NEG is 84 mm. Recommended maximum is 30 mm for this class of analog amplifier.",
  "nets": ["AMP_OUT", "OPAMP_NEG"],
  "refs": ["R12", "C8", "U1"],
  "suggestion": "Move feedback resistor and compensation capacitor closer to U1 and route feedback away from output connector."
}
```

## 19. Manufacturing Checks

The tool should support a manufacturer profile. At minimum:

- minimum trace width
- minimum clearance
- minimum drill size
- annular ring
- solder mask expansion
- silkscreen clearance
- board edge clearance
- via limits
- copper-to-edge clearance

Example:

```json
{
  "manufacturer_profile": "generic_2_layer_low_cost",
  "rules": {
    "min_trace_width_mm": 0.15,
    "min_clearance_mm": 0.15,
    "min_drill_mm": 0.30,
    "min_silkscreen_height_mm": 1.0
  }
}
```

## 20. Artifact Generation

The tool must generate:

- KiCad project files
- schematic SVG/PDF preview
- PCB front/back SVG previews
- optional 3D board render
- ERC report
- DRC report
- layout lint report
- BOM
- position file
- Gerbers
- drill files
- zipped fabrication package
- transaction log

Artifact paths must be returned in structured command output.

## 21. Human Review Requirements

Before fabrication, the tool should produce a human review checklist:

```text
- Are all component values correct?
- Are all transistor/MOSFET/op-amp pin mappings verified?
- Are polarized capacitors oriented correctly?
- Are diodes and LEDs oriented correctly?
- Are power rails correct?
- Is the board outline correct?
- Are connectors oriented and labeled correctly?
- Are test points accessible?
- Are mounting holes correct?
- Are silkscreen labels readable?
- Did ERC pass?
- Did DRC pass?
- Did layout lint pass or have justified waivers?
- Were generated Gerbers visually inspected?
```

## 22. Import and Round-Trip Support

The system should support importing existing KiCad projects.

Required import capabilities:

- read project metadata
- parse schematic components
- parse nets
- parse footprints
- parse PCB placement
- parse board outline
- parse tracks/zones/vias
- generate canonical JSON model
- diff imported state against canonical state

Round-tripping must preserve user edits where possible. If a conflict occurs, the system must report it instead of silently overwriting.

## 23. Diff and Review

The tool should support semantic diffs.

Example diff categories:

- component added/removed
- value changed
- net changed
- footprint changed
- placement changed
- route changed
- constraint changed
- DRC status changed
- generated artifact changed

Example:

```json
{
  "diff": [
    {
      "type": "value_changed",
      "ref": "C8",
      "from": "100pF",
      "to": "68pF"
    },
    {
      "type": "placement_changed",
      "ref": "C8",
      "from": {"x_mm": 42.1, "y_mm": 30.2},
      "to": {"x_mm": 38.0, "y_mm": 28.5}
    }
  ]
}
```

## 24. Error Handling

All failures must be structured and actionable.

Bad:

```text
Failed to generate board.
```

Good:

```json
{
  "status": "failed",
  "errors": [
    {
      "code": "MISSING_FOOTPRINT",
      "message": "Component Q2 has no assigned footprint.",
      "ref": "Q2",
      "suggestion": "Assign a footprint and verify transistor pin mapping."
    }
  ]
}
```

## 25. Logging and Reproducibility

The tool must log:

- commands
- transactions
- generated file hashes
- KiCad version used
- plugin/tool versions
- ERC/DRC results
- waivers
- export settings

A design should be reproducible from:

```text
SPEC.md + canonical JSON files + transaction log + tool version
```

## 26. Safety and Risk Controls

The tool must not imply that generated designs are safe for mains, high voltage, medical, automotive, aerospace, or regulatory-controlled applications without qualified human review.

For high-voltage or mains-connected boards, the tool must support stricter rule profiles and prominent warnings.

For generated analog/audio amplifier boards, the tool should encourage:

- current-limited first power-up
- dummy load testing
- output DC offset measurement
- rail voltage verification
- thermal inspection
- oscilloscope stability checks
- no headphones/speakers until bias and DC offset are verified

## 27. Testing Strategy

### 27.1 Unit Tests

Test:

- schema validation
- command parsing
- transaction application
- rollback behavior
- net connectivity model
- footprint assignment logic
- constraint evaluation
- lint rules
- report generation

### 27.2 Golden File Tests

Maintain known-good projects and compare generated outputs.

Examples:

- resistor divider
- op-amp buffer
- inverting op-amp stage
- non-inverting op-amp stage
- split supply connector board
- simple Class A headphone amp
- Class AB headphone amp

### 27.3 KiCad Integration Tests

For each golden project:

- generate KiCad files
- run ERC
- run DRC
- render schematic
- render PCB
- export fabrication package
- verify expected reports

### 27.4 Layout Lint Tests

Construct intentionally bad designs to verify that lint catches:

- missing decoupling
- long feedback trace
- input/output crosstalk risk
- bad Zobel grounding
- narrow output trace
- no test points
- ambiguous 0V midpoint

## 28. Recommended Implementation Notes

The implementation language is not mandated, but Go is a strong fit because the tool is CLI-heavy, schema-driven, and benefits from static binaries.

Recommended implementation approach:

```text
/internal/model          canonical design model
/internal/schema         JSON schema validation
/internal/kicad          KiCad read/write adapter
/internal/commands       CLI command handlers
/internal/transactions   transaction apply/rollback
/internal/verify         ERC/DRC/layout-lint integration
/internal/render         preview generation
/internal/export         Gerber/BOM/fab export
/internal/report         structured report output
```

The tool should expose both:

- human-friendly CLI commands
- AI-friendly JSON transaction commands

## 29. MVP Scope

### 29.1 MVP Must Include

- initialize KiCad project
- create schematic from JSON model
- add components
- connect nets
- assign footprints
- create simple rectangular PCB outline
- place components by explicit coordinates
- create net classes
- create basic traces
- run ERC
- run DRC
- render schematic and PCB previews
- export BOM and Gerbers
- write transaction log

### 29.2 MVP Should Include

- placement-by-intent for decoupling caps
- simple layout lint rules
- test point generation
- semantic diff
- manufacturer rule profile

### 29.3 MVP May Exclude

- full autorouting
- advanced KiCad round-trip import
- hierarchical schematic support
- SPICE simulation
- automatic 3D enclosure checks

## 30. Roadmap

### Phase 1: Schema and Schematic Generation

- Define canonical schema.
- Generate KiCad project and schematic.
- Add components and nets.
- Assign footprints.
- Run ERC.
- Generate schematic preview.

### Phase 2: Basic PCB Generation

- Create board outline.
- Place components by coordinate.
- Create net classes.
- Route simple traces.
- Add zones.
- Run DRC.
- Generate PCB previews.

### Phase 3: Constraint-Based Placement

- Place by functional block.
- Place near pin/component.
- Add keepouts.
- Add grouping and symmetry.
- Add decoupling placement rules.

### Phase 4: Analog Layout Lint

- Implement audio/analog lint rules.
- Add star-ground analysis.
- Add feedback-loop length analysis.
- Add high-current return-path checks.
- Add test-point coverage analysis.

### Phase 5: AI Review Loop

- Structured AI transaction API.
- Report-to-prompt workflow.
- Automatic correction proposals.
- Rendered artifact feedback loop.
- Integration with external review tools.

### Phase 6: Fabrication Package and Release Workflow

- Export manufacturing package.
- Generate review checklist.
- Require explicit human approval before fabrication-ready state.
- Package design artifacts for versioned release.

## 31. Acceptance Criteria

The project is successful when an AI agent can:

1. Create a simple audio amplifier schematic from a structured specification.
2. Assign verified footprints.
3. Generate a KiCad schematic and board file.
4. Place critical components according to analog layout intent.
5. Route or partially route the PCB using declared constraints.
6. Run ERC and DRC.
7. Run audio/analog layout lint.
8. Generate schematic and PCB previews.
9. Export Gerbers, drill files, and BOM.
10. Return structured errors and warnings suitable for iterative correction.
11. Produce a design that a human can open in KiCad and meaningfully review.

## 32. Key Principle

The system should not merely let AI write KiCad files.

The system should let AI express electrical design intent, transform that intent into KiCad artifacts, verify the result, and iterate based on evidence.

That is the difference between a demo and a usable engineering tool.
