# Verified Coverage Expansion Specification

## Objective

Expand KiCadAI's verified component and circuit-block coverage so AI-generated
designs can use more real, evidence-backed building blocks without falling back
to placeholder active parts or unsupported block gaps.

This project is the next roadmap item after fabrication identity evidence. It
does not introduce natural-language intent planning yet. It strengthens the
verified design vocabulary that a future intent planner will depend on.

## Roadmap Context

`specs/ROADMAP.md` now recommends:

1. Expand verified component and block coverage alongside each new block family.
2. Add deeper fabrication checks.
3. Add intent-level planning after the evidence gates are reliable.

This specification covers item 1.

## Current Foundation

The repository already has:

- component catalog loading, validation, selection, coverage reports, and
  selection goldens in `internal/components`;
- seed component data under `data/components`;
- verified or policy-allowed records for common passives, pin headers, LEDs,
  diodes, one regulator, one op-amp, one MCU, an I2C sensor, USB-C power-only,
  and a protection part;
- block inventory, block realization, block verification corpus, and workflow
  evidence in `internal/blocks`;
- existing blocks for LED indicator, connector breakout, voltage regulator,
  MCU minimal, USB-C power, I2C sensor, and op-amp gain stage;
- placement/routing hints, local-route evidence, validation, repair, and
  fabrication-readiness gates.

The remaining gap is breadth. KiCadAI needs additional verified families and
variants before an AI can compose useful boards without hitting unsupported or
placeholder boundaries.

## Scope

### In Scope

- Add verified component records for the next seed families:
  - crystal or ceramic resonator oscillator support;
  - standalone reset/programming header support;
  - ESD protection for external connectors;
  - reverse-polarity or input protection;
  - additional regulator variant or package where needed by the new blocks.
- Add corresponding circuit blocks or block variants:
  - `crystal_oscillator`;
  - `reset_programming`;
  - `esd_protection`;
  - `reverse_polarity_protection`.
- Add block inventory entries, parameters, ports, electrical rules, PCB
  constraints, required route metadata, and known gaps.
- Add schematic and PCB realization fragments where the existing block
  framework can support them.
- Add resolver/pinmap checks for every concrete active, polarized, connector,
  and protection record.
- Add golden tests for catalog coverage, selection behavior, block inventory,
  block realization, and workflow evidence.
- Integrate selected component properties into generated schematic symbols when
  existing writer APIs support those properties.
- Keep fabrication-candidate behavior conservative: new blocks may be usable in
  design workflows, but fabrication readiness should remain blocked when
  exact-part, pinmap, local-route, ERC/DRC, or fabrication evidence is missing.

### Out Of Scope

- Live distributor availability, pricing, or lifecycle API calls.
- Full datasheet parsing.
- SPICE or oscillator startup simulation.
- USB compliance certification.
- ESD surge compliance guarantees.
- Thermal modeling beyond encoded ratings and notes.
- Natural-language intent planning.
- Imported-project mutation.
- Manufacturer acceptance guarantees.

## Evidence Policy

Every new verified component record must provide deterministic local evidence:

- stable component ID;
- concrete manufacturer and MPN when the part is not a generic symmetric
  passive;
- lifecycle status when known;
- symbol library ID;
- footprint library ID;
- package;
- component class;
- ratings relevant to selection;
- source or evidence note;
- pinmap evidence for all non-symmetric or active parts;
- acceptance/confidence level.

Every new block must provide deterministic block evidence:

- block ID and version;
- declared ports and port roles;
- parameter schema and defaults;
- required component roles;
- selected component IDs or selection criteria;
- schematic nets and exported nets;
- PCB constraints and placement hints;
- local-route expectations where applicable;
- validation rules;
- known gaps;
- verification level.

## Target Families

### Crystal Or Oscillator Support

Add a block for an MCU clock source that can be connected to a minimal MCU
system.

Required component coverage:

- one crystal or resonator record with package/footprint evidence;
- two load capacitor records or parameterized generic capacitors;
- optional series resistor or damping resistor policy if needed;
- pinmap evidence for the crystal or resonator footprint.

Required block behavior:

- exported ports for `XTAL1`, `XTAL2`, and `GND`;
- parameterized frequency;
- load capacitance metadata;
- placement hint near MCU oscillator pins;
- local-route expectation for the crystal loop;
- explicit warning that oscillator startup and layout quality are not simulated.

### Reset And Programming Support

Add a block for reset pull-up, reset button, and programming/debug header.

Required component coverage:

- tactile switch or reset button record;
- pull-up resistor policy;
- 1x03, 1x04, 1x05, or 2x03 programming header record as appropriate;
- connector pin-number evidence.

Required block behavior:

- exported ports for `RESET`, `VCC`, `GND`, and programming signals such as
  `MOSI`, `MISO`, `SCK`, `TX`, `RX`, `SWDIO`, or `SWCLK` depending on variant;
- variant policy for AVR ISP, UART, or SWD, with unsupported variants
  reported as gaps;
- edge/accessible placement hint for programming connectors;
- reset net validation rules.

### ESD Protection Support

Add a block for connector-facing ESD protection.

Required component coverage:

- one concrete TVS or ESD diode array record;
- package/footprint evidence;
- pinmap and polarity evidence;
- ratings metadata such as working voltage and channel count.

Required block behavior:

- exported protected and unprotected signal ports;
- GND port;
- proximity constraint to connector;
- local route from connector to protection to protected net;
- explicit unsupported gaps for high-speed impedance and compliance.

### Reverse-Polarity Or Input Protection

Add a block for simple power-input protection.

Required component coverage:

- one Schottky diode, ideal-diode controller placeholder explicitly blocked, or
  P-channel MOSFET record if evidence is available;
- package/footprint/pinmap evidence;
- voltage/current/rating metadata.

Required block behavior:

- exported `VIN_RAW`, `VIN_PROTECTED`, and `GND` ports;
- selected topology metadata;
- route-width/current policy;
- placement hint near power input connector;
- rating checks against requested input current and voltage when available.

## Data Model Expectations

Prefer extending existing models over inventing parallel structures.

Component data should remain under `data/components` unless a phase introduces a
clear reason to split files. Block metadata should use existing block registry,
inventory, realization, and verification structures.

New fields should be added only when they support an executable gate or an
AI-facing explanation. Avoid broad free-form blobs that cannot be validated.

## CLI And Workflow Expectations

The existing commands should expose new coverage naturally:

- `component coverage` should include new family and acceptance data.
- `component find/select/validate` should handle new records.
- `block inventory` should show new blocks and known gaps.
- `block verify --builtins` should cover new block manifests.
- `design create` should be able to use new block IDs when requested through
  structured block intent.

No new top-level command is required for this project.

## Validation Requirements

Required tests:

- catalog validation for every new record;
- coverage golden updates;
- selection goldens for safe and blocked cases;
- pinmap/resolver evidence tests for non-passive records;
- block inventory tests;
- block realization tests;
- block verification corpus updates;
- design workflow evidence test for at least one new block;
- regression tests proving unsupported variants produce structured issues.

External KiCad CLI evidence may remain optional, but the new records and blocks
must not make existing hermetic tests depend on a local KiCad install.

## Acceptance Criteria

This project is complete when:

- the new target families appear in component coverage;
- each new concrete active/polarized/connector/protection record has
  manufacturer/MPN, package, symbol, footprint, and pinmap evidence;
- each new block appears in block inventory with readiness, rules, ports, PCB
  constraints, and known gaps;
- supported variants can emit schematic and PCB realization fragments;
- unsupported variants fail with structured, AI-actionable issues;
- existing `go test ./...` passes;
- documentation and roadmap status are updated.

## Risks

- Incorrect pinmaps are worse than missing pinmaps. Treat uncertain pinmaps as
  blocked.
- Oscillator and ESD layout quality can look simple but carry real electrical
  risk. Keep claims narrow and evidence-specific.
- Adding too many fields without executable checks will create false
  confidence. Prefer small verified records over broad unverified catalogs.
- New blocks can increase placement/routing complexity. Start with constrained
  local-route expectations and explicit gaps.
