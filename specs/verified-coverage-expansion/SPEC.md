# Verified Coverage Expansion Specification

Date: 2026-06-26

## Summary

KiCadAI can already generate, validate, and explain a meaningful set of
structured-intent designs, but autonomous generation remains constrained by the
small verified component catalog and limited verified block variants. The next
step is to expand coverage deliberately: add parts and block variants only when
they carry enough symbol, footprint, pinmap, rating, and validation evidence for
agents to rely on them.

This project defines the expansion workflow for verified component and circuit
block coverage. It is not a request to add random catalog entries. Every new
entry must increase supported design intent while preserving deterministic
selection, fail-closed validation, and rationale evidence.

## Goals

- Expand the checked-in component catalog with verified, KiCad-resolvable parts
  for high-value AI generation workflows.
- Add block variants that use those parts through explicit symbols,
  footprints, pinmaps, component roles, placement intent, and local route
  expectations.
- Improve planner coverage by allowing structured intent to select new verified
  blocks or variants without guessing.
- Ensure each added part and block variant has machine-readable evidence for
  component selection, rationale reports, validation, and repair workflows.
- Keep fabrication-candidate behavior conservative: missing evidence must block
  rather than degrade into placeholders.

## Non-Goals

- Do not add live distributor sourcing, price, stock, or lifecycle scraping.
- Do not add unverified catalog entries just to broaden search results.
- Do not implement arbitrary analog or digital design synthesis.
- Do not change KiCad file format writers except where a new verified block
  exposes an existing writer gap.
- Do not claim production readiness solely because a part is in the catalog.
- Do not bypass resolver or pinmap evidence for active parts.

## Candidate Coverage Areas

The first expansion wave should prioritize parts and blocks that unlock common
board requests:

- power:
  - 3.3 V and 5 V LDO variants;
  - buck regulator placeholder only if verified symbol, footprint, and required
    companion components are modeled;
  - input/output capacitor families with voltage and capacitance ratings;
  - ferrite bead and fuse/polyfuse families where protection blocks need them.
- connectivity:
  - common 2.54 mm and JST connectors;
  - USB-C sink connector variants with CC role evidence;
  - test pads and programming headers.
- digital:
  - additional MCU seed templates only when symbol pins, package pads, power
    pins, reset, programming, and core bus roles are mapped;
  - I2C sensor concrete candidates with address, supply, SDA/SCL, interrupt,
    and decoupling evidence.
- analog:
  - op-amp variants with supply range and package evidence;
  - feedback resistor/capacitor families with value and tolerance metadata.
- protection:
  - ESD diode variants;
  - reverse-polarity diode/MOSFET variants;
  - TVS and input protection families with working-voltage/current ratings.

## Family-Specific Evidence Reference

Follow-on expansion slices must retain the family requirements that drove the
original coverage backlog:

- clock sources need frequency, load-capacitance, package, drive-level,
  stability metadata, `XTAL1`/`XTAL2`/`GND` port roles, crystal or resonator
  symbol pin evidence, footprint pad evidence, and companion load-capacitor
  policy;
- reset/programming support needs reset pull-up policy, reset switch or header
  evidence, and programming roles such as `MISO`, `MOSI`, `SCK`, `RESET`,
  `SWDIO`, `SWCLK`, `UART_TX`, and `UART_RX` where the selected MCU family
  supports them;
- protection blocks need entry-side and protected-side roles, polarity or
  bidirectional behavior, working-voltage/current ratings, package pad evidence,
  and placement evidence near the board entry point;
- USB-C sink variants need CC role evidence, VBUS/GND pad aggregation policy,
  optional shield treatment, and explicit no-connect or unsupported-data-lane
  handling.

## Component Evidence Requirements

Every new component record must include:

- `id`, `family`, `name`, and package variant identifiers;
- KiCad symbol binding with function pins;
- footprint binding with pad functions;
- confidence level and verification metadata;
- ratings relevant to safe use, such as voltage, current, power, capacitance,
  tolerance, supply voltage, working voltage, drive level, or frequency
  stability;
- value constraints for passives;
- companion requirements when the part cannot be used safely alone;
- placement/routing hints when the part affects layout quality;
- tags or functions that make selection explainable.

Active and connector parts must have resolver/pinmap evidence. Passive
rule-inferred records may remain narrower, but fabrication-candidate flows must
still see enough value/rating evidence to avoid unsafe selection.

## Block Variant Requirements

Every new or expanded block variant must define:

- supported parameters and defaults;
- explicit component roles and component queries or IDs;
- symbol and footprint requirements;
- port definitions and voltage-domain behavior;
- local nets and role-based connectivity expectations;
- PCB realization where available, including placements, groups, local routes,
  constraints, and validation expectations;
- unsupported behaviors and readiness level;
- verification fixture coverage.

Blocks must not silently choose a component variant when several electrically
different choices exist. Ambiguity must become a planner clarification, known
gap, or blocked issue.

## Planner Integration

Structured intent should gain new coverage only through explicit mapping rules:

- function family or block family selection;
- component policy defaults and overrides;
- target/bus/supply metadata;
- acceptance-level gates;
- calculated value and rating requirements where available.

Planner output must explain:

- why a block or variant was selected;
- which catalog component satisfied each role;
- which evidence was verified;
- which ratings were checked;
- which gaps or unsupported behaviors remain.

## Validation And Acceptance

Each expansion slice must pass:

- component catalog validation;
- catalog evidence validation where resolver data is available;
- component selection tests for positive and negative cases;
- block registry validation;
- block instantiation tests;
- workflow tests that verify generated request and component-selection output;
- writer correctness and board validation for generated fixtures where PCB
  realization exists;
- optional KiCad ERC/DRC fixtures when the block claims those evidence levels.

No expansion is accepted if it only adds JSON without tests proving selection,
mapping, and validation behavior.

## CLI And Artifacts

Existing commands should remain the public interface:

- `kicadai --json component validate`
- `kicadai --json component coverage`
- `kicadai --json component find`
- `kicadai --json component select`
- `kicadai --json block list`
- `kicadai --json block show`
- `kicadai --json block verify`
- `kicadai --json --request request.json intent plan`
- `kicadai --json --request request.json intent create`
- `kicadai --json --request request.json --output ./out design create`

New coverage must surface in existing JSON artifacts:

- component coverage reports;
- component selection stage output;
- block readiness and verification output;
- intent synthesis trace;
- design rationale report.

## Success Criteria

- At least one high-value design family gains verified component and block
  coverage beyond current seed examples.
- New parts are selectable through catalog queries and rejected when ratings or
  functions are insufficient.
- New block variants instantiate deterministically and validate with existing
  writer and board gates.
- Intent planning can select the new coverage from structured requests without
  guessing.
- Rationale output explains selected parts, ratings, and remaining gaps.
- Full `go test ./...` passes without requiring network access.

## Appendix: Deferred Coverage Backlog

The current implementation slice is the verified regulator path, but these
deferred families remain part of the verified coverage backlog. They are kept
here so narrowing the first slice does not remove the concrete requirements
needed for later phases.

### Crystal Or Oscillator Support

Required component coverage:

- one crystal or ceramic resonator record with package and footprint evidence;
- two load capacitor records or parameterized generic capacitors;
- optional series or damping resistor policy when required by the selected
  clock source;
- pinmap evidence for the crystal or resonator footprint;
- frequency, load-capacitance, package, lifecycle, drive-level, and stability
  metadata where available.

Required block behavior:

- exported ports for `XTAL1`, `XTAL2`, and `GND`;
- parameterized frequency and load capacitance;
- placement hint near MCU oscillator pins;
- local-route expectation for the crystal loop;
- explicit warning that oscillator startup and layout quality are not simulated.

### Reset And Programming Support

Required component coverage:

- tactile switch or reset button record;
- pull-up resistor policy;
- 1x03, 1x04, 1x05, 2x03, or other programming header record as appropriate;
- connector pin-number evidence and role evidence.

Required block behavior:

- exported ports for `RESET`, `VCC`, `GND`, and programming signals such as
  `MOSI`, `MISO`, `SCK`, `TX`, `RX`, `SWDIO`, or `SWCLK` depending on variant;
- variant policy for AVR ISP, UART, or SWD, with unsupported variants reported
  as structured gaps;
- edge or accessible placement hints for programming connectors;
- reset net validation rules.

### ESD Protection Support

Required component coverage:

- one concrete TVS or ESD diode-array record;
- package and footprint evidence;
- pinmap and polarity evidence;
- working-voltage, channel-count, and clamp/current metadata where available.

Required block behavior:

- exported protected and unprotected signal ports;
- `GND` port;
- proximity constraint to the connector;
- local route from connector to protection to protected net;
- explicit unsupported gaps for high-speed impedance and compliance.

### Reverse-Polarity Or Input Protection

Required component coverage:

- one Schottky diode, P-channel MOSFET, or explicitly blocked ideal-diode
  controller placeholder when evidence is incomplete;
- package, footprint, and pinmap evidence;
- voltage, current, topology, and thermal rating metadata.

Required block behavior:

- exported `VIN_RAW`, `VIN_PROTECTED`, and `GND` ports;
- selected topology metadata;
- route-width and current policy;
- placement hint near the power input connector;
- rating checks against requested input current and voltage when available.
