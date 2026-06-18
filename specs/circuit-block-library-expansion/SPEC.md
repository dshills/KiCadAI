# Circuit Block Library Expansion Specification

## 1. Purpose

Expand KiCadAI's circuit block library from a working foundation into a
verification-backed catalog of reusable schematic and PCB fragments that an AI
workflow can compose with confidence.

This project implements Priority 2 from `specs/ROADMAP.md`: Circuit Block
Library Expansion.

The goal is for each supported block to produce meaningful schematic structure,
footprint assignments, PCB placement intent, local routing intent, constraints,
and validation evidence. Blocks must fail closed when required components,
pinmaps, nets, constraints, or validation evidence are missing.

## 2. Current Baseline

The repository already has strong foundations:

- `internal/blocks` contains block definitions, requests, parameters,
  components, ports, nets, schematic emitters, composition support, resolver
  integration, and PCB realization metadata.
- Built-in block families exist for LED indicators, voltage regulators, MCU
  minimal systems, USB-C power, I2C sensors, op-amp gain stages, and connector
  breakouts.
- `internal/components` provides verified component selection, confidence
  gates, richer part metadata, ratings, companions, and resolver/pinmap
  evidence checks.
- `internal/blocks/verification` provides a block-level verification harness
  with semantic assertions and reports.
- PCB realization work can produce placements, footprints, local routes, and
  constraints for known blocks.
- Design workflow integration can orchestrate block selection, schematic
  generation, footprint assignment, placement, routing, validation, repair, and
  evidence output.

This expansion must reuse those layers. It should not invent a separate circuit
description format or bypass the writer, resolver, component catalog, placement,
routing, or validation systems.

## 3. Goals

This project must:

- harden current built-in blocks with stricter electrical and PCB rules;
- expand the block catalog to cover the roadmap seed families;
- require verified or explicitly policy-allowed component selections;
- encode block-specific electrical requirements such as decoupling, pull-ups,
  enable pins, boot straps, rail compatibility, polarity, biasing, and thermal
  constraints;
- encode block-specific PCB requirements such as placement groups, proximity,
  edge constraints, keepouts, route priorities, zone requirements, and local
  DRC constraints;
- connect block rules to component catalog metadata and selected-part evidence;
- produce machine-readable validation evidence per block;
- add golden verification manifests for supported block variants;
- make autonomous workflows refuse under-specified or unsafe block requests;
- keep output deterministic across repeated runs.

## 4. Non-Goals

This project does not need to:

- implement full analog, RF, high-speed, or power-safety engineering review;
- implement SPICE simulation;
- implement global board planning or full-board autorouting;
- call live distributor APIs;
- guarantee real-time availability, pricing, lifecycle, or compliance status;
- generate production-ready fabrication packages by itself;
- support every KiCad library part;
- accept arbitrary natural-language circuit descriptions without a planning
  layer.

## 5. Supported Block Families

### 5.1 LED Indicator

The LED block must support:

- high-side or low-side indicator topology where safe;
- configurable supply voltage, LED forward voltage, target current, color, and
  package;
- resistor value derivation or verified resistor selection;
- LED polarity validation;
- exported `VCC` and `GND` ports;
- a local net between the resistor and LED;
- optional GPIO-driven variant with explicit drive polarity;
- PCB placement with resistor and LED grouped, LED orientation exposed, and a
  routed local series connection.

Required blockers:

- missing or non-polarity-verified LED;
- invalid target current;
- resistor power rating below calculated requirement;
- unresolved footprint;
- missing local route evidence when PCB realization is requested.

### 5.2 Voltage Regulator

The regulator block must support:

- at least one verified 3.3 V linear regulator path;
- input, output, ground, enable, and optional feedback nets;
- required input and output capacitors based on selected part metadata;
- optional enable pull-up or pull-down where required;
- voltage, current, dropout, thermal, polarity, and capacitor-rating checks;
- PCB placement that keeps capacitors close to the regulator pins;
- route priority for input/output power paths and local ground;
- optional copper area or thermal note evidence when the selected package
  requires it.

Required blockers:

- selected regulator cannot satisfy requested VIN, VOUT, current, or dropout;
- missing stability capacitor requirements;
- missing enable handling for parts that require it;
- missing verified regulator pinmap;
- PCB capacitor placement violates proximity constraints.

### 5.3 MCU Minimal System

The MCU minimal block must support:

- one verified concrete MCU package from the component catalog;
- power and ground pins with required decoupling;
- reset pull-up or reset circuit when required;
- programming/debug header or pads;
- optional crystal or oscillator support;
- boot strap handling when required by the selected MCU;
- exported GPIO, power, programming, reset, and bus ports;
- PCB grouping for MCU, decoupling capacitors, reset/programming parts, and
  crystal parts;
- short local routes for decoupling and oscillator nets where possible.

Required blockers:

- missing MCU concrete part evidence;
- missing package-specific pinmap;
- unhandled required power pins;
- missing decoupling for each required power domain;
- requested peripheral function cannot be mapped to an available pin;
- programming or reset path omitted when the request requires a programmable
  board.

### 5.4 USB-C Power Input

The USB-C power block must support:

- power-only USB-C receptacle usage;
- VBUS, GND, CC1, CC2, shield, and optional protection nets;
- verified CC pull-down resistors;
- optional fuse, TVS, reverse-polarity, or over-voltage protection companion
  parts;
- edge-facing connector placement;
- keepout and board-edge placement constraints;
- route priority for VBUS and GND;
- local routes for CC resistors and protection parts.

Required blockers:

- non-power-only-safe USB-C connector selection;
- missing CC resistors;
- missing connector pinmap evidence;
- missing edge/keepout constraints when PCB realization is requested;
- protection requested but no verified protection part is selected.

### 5.5 I2C Sensor

The I2C sensor block must support:

- one verified concrete I2C sensor record;
- VCC, GND, SDA, SCL, interrupt, and address-select nets where applicable;
- pull-up resistor policy with shared-bus awareness;
- required decoupling;
- optional address strap configuration;
- placement grouping for sensor and decoupling capacitor;
- exported bus and power ports.

Required blockers:

- sensor voltage range incompatible with requested bus rail;
- missing pull-ups when the block owns the bus;
- duplicate pull-ups when composition marks bus pull-ups as already provided;
- missing decoupling;
- unresolved sensor pinmap.

### 5.6 Op-Amp Gain Stage

The op-amp block must support:

- non-inverting and inverting gain-stage variants where supported;
- selected op-amp supply range and input/output swing checks;
- feedback resistor value calculation or verified selection;
- bias network requirements for single-supply operation;
- decoupling capacitor requirements;
- exported input, output, power, and reference ports;
- PCB placement that keeps feedback components close to the op-amp pins;
- local routes for feedback and bias nets.

Required blockers:

- requested gain cannot be represented by available resistor values;
- selected op-amp is incompatible with the requested supply or signal range;
- missing bias/reference handling for single-supply mode;
- feedback placement or routing evidence is missing.

### 5.7 Connector Breakout

The connector breakout block must support:

- verified pin-header families and common connector sizes;
- explicit pin-to-net assignment;
- optional labels, mounting orientation, and edge placement;
- per-pin electrical roles;
- optional protection or series components on selected pins;
- PCB placement that respects edge-facing or user-facing orientation.

Required blockers:

- pin count mismatch;
- duplicate or missing net assignments;
- unresolved connector footprint;
- pin numbering evidence unavailable.

### 5.8 Crystal And Oscillator

The crystal/oscillator block must support:

- crystal plus load capacitor topology;
- oscillator module topology where selected;
- frequency and load capacitance metadata;
- MCU clock-pin compatibility when composed with MCU minimal blocks;
- guard/keepout notes and short-route constraints;
- close placement to connected IC pins.

Required blockers:

- missing load capacitor calculation or selected values;
- missing connection to expected MCU oscillator pins;
- route length/proximity constraints unavailable for PCB realization;
- selected package lacks pinmap evidence.

### 5.9 Reset And Programming Header

The reset/programming block must support:

- reset switch or pull-up topology;
- SWD, UPDI, ICSP, UART boot, or generic programming header variants as
  available in verified metadata;
- target voltage, ground, reset, clock/data, and optional boot nets;
- connector pin numbering evidence;
- placement near MCU or board edge as requested.

Required blockers:

- selected target MCU does not expose required programming pins;
- programming header pinout is ambiguous;
- reset net is missing required pull state;
- requested programming interface is unsupported by the selected MCU record.

### 5.10 Protection Blocks

Protection blocks must support:

- ESD/TVS protection for USB, connectors, or external signals;
- reverse-polarity protection for power inputs;
- optional fuse or current-limiting element;
- polarity and power-rating checks;
- connector-near placement and route-through ordering.

Required blockers:

- protection component lacks polarity/pinmap evidence;
- rating is below requested voltage/current;
- requested protection topology cannot be placed in the required route path.

## 6. Block Rule Model

Each block must declare three categories of rules.

### 6.1 Electrical Rules

Electrical rules describe correctness before layout:

- required components and companions;
- allowed topologies;
- rail compatibility;
- current, voltage, power, tolerance, temperature, and frequency constraints;
- required pull-ups, pull-downs, decoupling, boot straps, enable states, and
  bias networks;
- polarity expectations;
- required exported ports and internal nets;
- component catalog selection requirements.

Electrical rule failures must block connectivity-level and fabrication-candidate
generation.

### 6.2 PCB Rules

PCB rules describe local layout expectations:

- placement groups;
- proximity constraints;
- edge-facing constraints;
- rotation/orientation constraints;
- keepout areas;
- short-route requirements;
- route priority;
- net class requirements;
- zone or copper area requirements;
- local DRC expectations.

PCB rule failures must block PCB realization readiness, even if schematic output
is structurally valid.

### 6.3 Evidence Rules

Evidence rules describe proof:

- selected components must carry confidence and source evidence;
- symbol and footprint IDs must resolve;
- pinmaps must prove required functional pins map to footprint pads;
- semantic assertions must prove expected nets and pins;
- writer correctness must pass for generated files;
- KiCad ERC/DRC evidence should be attached when available;
- skipped optional checks must be explicit and explain why.

Evidence failures must downgrade readiness or block autonomous use according to
the requested acceptance level.

## 7. Verification Manifest Requirements

Every supported block variant must have a verification manifest that records:

- block ID and variant ID;
- request parameters;
- expected selected component roles;
- expected references or reference prefixes;
- expected exported ports;
- expected internal nets and pin memberships;
- expected schematic properties that matter for downstream workflows;
- expected footprints;
- expected local placements;
- expected local routes, zones, and constraints;
- expected validation level;
- expected warnings or blockers for negative cases.

Positive fixtures must cover at least one usable request per block family.
Negative fixtures must cover at least one fail-closed condition for each block
family that has safety-critical rules.

## 8. AI-Facing Output

Block expansion must produce output that is useful to an AI planner:

- available block IDs and variants;
- required and optional parameters;
- supported rails, currents, interfaces, and topology choices;
- readiness level for each block;
- why a request passed, warned, or blocked;
- selected component metadata, including manufacturer, MPN, confidence,
  ratings, companions, and evidence;
- exported ports and composition constraints;
- schematic and PCB artifact paths when generated.

Output must be deterministic JSON-compatible data. Human-readable CLI summaries
may be added on top, but they must not be the only source of evidence.

## 9. Acceptance Gates

This project is complete when:

- all roadmap seed block families have at least one positive verification case;
- LED, regulator, MCU minimal, USB-C power, I2C sensor, op-amp gain stage, and
  connector breakout blocks have electrical and PCB rule coverage;
- crystal/oscillator, reset/programming, ESD, and reverse-polarity protection
  blocks exist or are explicitly represented as blocked/unsupported with clear
  planner diagnostics;
- block requests fail closed when required components, footprints, pinmaps,
  nets, or constraints are missing;
- verification manifests produce stable machine-readable reports;
- generated schematic and PCB fragments pass writer correctness checks where
  the existing validation stack supports them;
- KiCad-backed ERC/DRC checks are attached when available and marked skipped
  with reason when unavailable;
- tests cover positive generation, negative blockers, rule evidence, and
  deterministic output.

## 10. Risks

- Verified component coverage may limit how many block variants can honestly be
  marked ready.
- Some KiCad library symbols have generic pin names that require curated pinmap
  records before block rules can be enforced.
- PCB constraints may initially be semantic rather than fully enforced by KiCad
  DRC until later placement/routing work consumes every hint.
- MCU and USB-C correctness can become unsafe if generic placeholders are
  promoted without concrete evidence.

The implementation should prefer smaller verified coverage over broad
placeholder coverage.
