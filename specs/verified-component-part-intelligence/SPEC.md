# Verified Component And Part Intelligence Specification

## 1. Purpose

Build the next layer of component intelligence needed for AI-generated KiCad
projects to select real parts safely.

The existing component intelligence foundation can load a seed catalog, validate
basic model consistency, perform deterministic selection, gate low-confidence
records, and validate some resolver/pinmap evidence. This project turns that
foundation into a verified part intelligence system with enough metadata,
coverage, and tests to support common generated designs without relying on
placeholder active components.

The goal is not to create a complete distributor database. The goal is to make
KiCadAI's selected parts electrically meaningful, library-resolvable, pinmap
checked, and honest about missing evidence.

## 2. Roadmap Context

This implements Priority 1 from `specs/ROADMAP.md`: Verified Component And Part
Intelligence.

Priority 1 exists because the writer, block, placement, routing, validation, and
repair foundations are only as trustworthy as the parts they instantiate.
Generated designs cannot be considered autonomous or fabrication candidates
while regulators, MCUs, sensors, protection parts, and connectors are still
placeholder-level selections.

## 3. Current Baseline

The repository already contains:

- `internal/components` model, catalog loading, validation, selection, resolver
  evidence checks, and golden tests.
- `data/components` seed catalog files:
  - `families.json`;
  - `passives.json`;
  - `connectors.json`;
  - `diodes_leds.json`;
  - `active_blocks.json`.
- `internal/pinmap` built-in pinmap records.
- `internal/libraryresolver` for symbol and footprint resolution.
- `internal/blocks` and `internal/designworkflow` integrations that can use
  component selection.

The baseline is useful, but many active records are intentionally placeholders.
The next work should preserve that honesty while adding verified records and
stricter evidence.

## 4. Goals

This project must:

- expand the catalog with verified records for common AI-generated design
  families;
- distinguish generic design-time records from concrete manufacturer parts;
- record manufacturer part numbers, lifecycle, and ratings where they are known;
- encode package variants, footprint constraints, and symbol-footprint pinmaps;
- add pin-function and electrical-type expectations that can be checked against
  KiCad symbols;
- add polarity, package, derating, and family-specific selection rules;
- block under-evidenced active parts from connectivity, ERC/DRC, and fabrication
  candidate workflows;
- expose machine-readable coverage and evidence reports;
- integrate richer selected-part metadata into schematic properties, PCB rules,
  block verification, and design workflow outputs;
- add golden tests for safe selections, unsafe selections, evidence failures,
  and coverage thresholds.

## 5. Non-Goals

This project does not need to:

- call live distributor APIs;
- guarantee real-time availability, pricing, or lifecycle status;
- model every KiCad symbol or footprint;
- prove regulatory compliance;
- run SPICE simulation;
- fully solve thermal, EMI, RF, high-speed, or safety-critical design;
- convert ambiguous natural language directly into part choices;
- allow fabrication readiness from unknown or placeholder data.

## 6. Catalog Policy

### 6.1 Confidence Levels

Existing confidence levels remain the contract:

- `verified`;
- `library_derived`;
- `rule_inferred`;
- `placeholder`;
- `blocked`.

Connectivity and stronger acceptance levels may only use:

- `verified` records; or
- explicitly allowed symmetric passive `rule_inferred` records.

Connectors and active components must not reach connectivity readiness from
`rule_inferred` alone unless a future spec adds an explicit verified connector
rule policy.

### 6.2 Generic Versus Concrete Parts

The catalog should support two different records:

- Generic records for value-driven passive or structural design choices.
- Concrete records for manufacturer-specific active parts, polarized parts,
  protection parts, connectors, oscillators, and sensors.

Generic records may be acceptable for draft or structural workflows. Concrete
records are required for fabrication candidate workflows whenever pinout,
ratings, lifecycle, or package behavior materially affects correctness.

### 6.3 Source Evidence

Every verified record must cite deterministic local evidence:

- KiCad symbol ID;
- KiCad footprint ID;
- explicit pinmap or built-in pinmap entry;
- datasheet or curated-source reference string;
- test or fixture name proving the record was checked.

The source string does not need to embed full datasheets, but it must be stable
enough for a human reviewer to understand why the record is trusted.

### 6.4 Placeholder Policy

Placeholder records are allowed only when they are useful for draft workflows
and visibly identified as placeholders.

Placeholder active parts must:

- fail connectivity, ERC/DRC, and fabrication candidate selection;
- appear in coverage reports;
- include a clear note describing what evidence is missing;
- never be silently upgraded by inference.

## 7. Required Part Families

The first verified expansion should target families needed by current and near
term blocks.

### 7.1 Passives

Required coverage:

- resistors in 0603 and 0805;
- nonpolar ceramic capacitors in 0603 and 0805;
- polarized bulk capacitors with explicit polarity and voltage rating policy;
- value, tolerance, voltage, and power rating constraints.

Passives may use generic records when symmetry is true and the footprint/pinmap
evidence is clear. Polarized passives require explicit polarity evidence.

### 7.2 LEDs And Diodes

Required coverage:

- indicator LED with anode/cathode mapping;
- signal diode;
- Schottky diode;
- TVS diode for protection workflows.

Polarity must be explicit and checked against both symbol pins and footprint
pads.

### 7.3 Connectors

Required coverage:

- 1x02, 1x03, 1x04, and 1x05 pin headers;
- USB-C receptacle suitable for power-only blocks;
- programming/debug header records where current blocks need them.

Connector pin numbering must be explicit. Orientation, edge placement, mating,
and mechanical constraints should be recorded when known.

### 7.4 Regulators

Required coverage:

- at least one concrete 3.3 V linear regulator record;
- input voltage, output voltage, output current, dropout, thermal notes, and
  stability capacitor requirements;
- package variant with verified symbol-footprint pinmap.

Regulators must not be fabrication candidates without package-specific pinout
and required external component rules.

### 7.5 Op-Amps

Required coverage:

- at least one concrete single-supply single op-amp suitable for low-voltage
  gain stages;
- supply range, input common-mode notes, output swing notes, package, and
  pinmap;
- explicit non-inverting, inverting, output, positive supply, and negative
  supply functions.

Op-amp selection must account for requested supply voltage.

### 7.6 MCUs

Required coverage:

- at least one concrete MCU package for minimal-system blocks;
- power pins, ground pins, reset, programming, oscillator pins, and common bus
  pins;
- package-specific pinmap and footprint;
- notes for required decoupling, reset pull-up, boot/programming, and optional
  crystal use.

MCU records must be explicit about unsupported peripheral mapping.

### 7.7 Sensors

Required coverage:

- at least one concrete I2C sensor record;
- VCC, GND, SDA, SCL, and optional interrupt/address pins;
- supply range and pull-up expectations;
- package-specific symbol-footprint pinmap.

Structural sensor header placeholders may remain, but they must not be selected
when a concrete sensor is required.

### 7.8 Crystals And Oscillators

Required coverage:

- crystal or resonator records needed by MCU minimal blocks;
- load capacitance or external-capacitor expectations;
- package and pad mapping.

### 7.9 Protection And Power Components

Required coverage:

- reverse-polarity or Schottky protection;
- TVS/ESD device for USB or connector protection;
- current-limited or fuse placeholder policy where verified records do not yet
  exist.

## 8. Data Model Extensions

The existing `internal/components` model should be extended only where needed.
Likely additions include:

- manufacturer part metadata per package variant;
- lifecycle status with an explicit enum or validation policy;
- ordering code versus base MPN distinction;
- tolerance constraints;
- temperature range constraints;
- package height or 3D/mechanical notes;
- required companion components;
- derating rules;
- placement constraints derived from the component;
- routing or net-class hints derived from ratings;
- schematic property emission metadata.

The model should remain JSON-friendly and deterministic. Avoid embedding opaque
Go-only behavior in catalog records.

## 9. Evidence And Validation

### 9.1 Resolver Evidence

Validation must prove:

- every symbol ID resolves when a library index is configured;
- every footprint ID resolves when a library index is configured;
- declared function pins exist on the resolved symbol;
- declared pad functions exist on the resolved footprint;
- electrical types match declared expectations when symbol metadata provides
  that information;
- multi-unit symbols have an explicit unit policy.

### 9.2 Pinmap Evidence

Validation must prove:

- verified active records have an explicit symbol-footprint pinmap;
- all required function pins map to package pads;
- polarity-sensitive pins have matching symbol and pad polarity;
- duplicate or missing function mappings are blocking issues;
- incompatible symbol/footprint combinations are blocking issues.

### 9.3 Rating Evidence

Selection must prove:

- requested voltage, current, power, and supply ranges are satisfied;
- exact values are matched when required;
- minimum ratings use conservative comparisons;
- missing ratings block fabrication candidate selection for active or
  safety-relevant parts;
- failed rating checks produce stable issue codes.

### 9.4 Coverage Evidence

Add a coverage report that answers:

- which families have verified records;
- which families only have placeholders;
- which current built-in blocks can reach draft, connectivity, ERC/DRC, and
  fabrication candidate evidence;
- which records fail resolver, pinmap, rating, or metadata checks;
- which roadmap-required families are still missing.

## 10. Selection Behavior

Selection should become stricter and more explainable.

Required behavior:

- rank candidates by confidence, exact value/package match, rating margin, and
  deterministic ID order;
- block ambiguous equal-score candidates unless alternatives are explicitly
  allowed;
- return machine-readable reasons for selected and rejected candidates;
- support family-specific query fields where generic matching is too weak;
- require concrete records for active fabrication candidate selections;
- prevent placeholder active parts from being hidden behind generic family
  queries;
- return selected symbol, footprint, pinmap, rating summary, and design-rule
  notes.

## 11. Integration Requirements

### 11.1 Blocks

Circuit blocks should request components by family, function, value, ratings,
and acceptance level instead of hard-coding unresolved placeholder records.

Block verification should report:

- selected component IDs;
- selected variant IDs;
- confidence level;
- symbol, footprint, and pinmap evidence;
- unresolved component blockers.

### 11.2 Design Workflow

`design create` should include selected component evidence in its JSON output.

The workflow should block or degrade readiness when:

- active components are placeholders;
- selected records lack required pinmaps;
- ratings are missing or insufficient;
- resolver evidence is unavailable for selected verified parts.

### 11.3 Schematic Writer

Generated symbols should include useful component properties where available:

- component catalog ID;
- manufacturer;
- MPN or ordering code;
- lifecycle;
- value;
- rating notes;
- footprint;
- datasheet/source reference when appropriate.

### 11.4 PCB Rules

Selected components should feed downstream rules:

- footprint selection;
- net class hints for current or voltage;
- placement constraints for decoupling, connectors, crystals, and regulators;
- routing hints for power, analog, differential, or high-current nets.

This spec only requires emitting the metadata. Full placement and routing
behavior belongs to later roadmap priorities.

## 12. CLI Requirements

Add or extend component CLI commands to support:

- listing records by family, confidence, package, and verification status;
- selecting a component from a structured request;
- validating catalog model consistency;
- validating resolver/pinmap evidence;
- producing a coverage report;
- producing JSON output by default or via existing CLI conventions.

The CLI must be deterministic and suitable for AI agents.

## 13. Test Requirements

Add tests for:

- catalog schema/model validation;
- source catalog loading;
- verified record evidence;
- placeholder blocking;
- active-part rating failures;
- polarity mismatch detection;
- missing pinmap detection;
- ambiguous selection;
- family coverage report output;
- block integration with component selections;
- design workflow readiness gating.

Tests that depend on external KiCad libraries must be optional integration tests.
Default tests should run from checked-in fixtures and built-in pinmaps.

## 14. Acceptance Gates

The project is complete when:

- common design intents can select concrete parts for passives, LEDs, diodes,
  connectors, at least one regulator, at least one op-amp, at least one MCU,
  and at least one I2C sensor;
- every selected connectivity-level active part has symbol, footprint, and
  pinmap evidence;
- unsafe or under-evidenced selections block the workflow with actionable
  issues;
- the coverage report clearly distinguishes verified, rule-inferred,
  placeholder, and blocked records;
- built-in circuit blocks expose component evidence in verification reports;
- design workflow output contains selected-part evidence and readiness gates;
- default tests cover passing and failing catalog, selection, evidence, and
  integration paths.

## 15. Risks

- KiCad library symbols and footprints can change across versions.
- Generic active parts can create a false sense of safety.
- Datasheet-backed pinouts require careful human curation.
- Too much catalog breadth without tests will reduce trust.
- Rating comparisons can be misleading if units and derating are not modeled
  conservatively.

## 16. Open Questions

- Which exact manufacturer parts should be the first verified regulator, op-amp,
  MCU, sensor, USB-C connector, and protection device?
- Should verified catalog records live entirely in JSON, or should some
  high-value records be generated from curated fixtures?
- How strict should fabrication candidate selection be before live lifecycle or
  availability data exists?
- Should connector rule-inference ever be allowed above structural readiness?
