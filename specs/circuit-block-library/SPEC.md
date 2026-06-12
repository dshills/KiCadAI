# Circuit Block Library Specification

## 1. Purpose

Build a reusable circuit block library that lets KiCadAI compose known-good
schematic and PCB fragments from higher-level design intent.

The first target blocks are:

- LED indicator
- voltage regulator
- MCU minimal system
- USB-C power input
- I2C sensor
- op-amp gain stage
- connector breakout

The long-term goal is for an AI agent to say "I need a breakout board that does
these things" and have KiCadAI assemble a complete schematic and PCB from
verified blocks, then run deterministic validation before presenting the result.

## 2. Context

KiCadAI already has:

- direct KiCad project, schematic, and PCB writers;
- a higher-level design API and transaction system;
- symbol and footprint library resolution;
- conservative symbol-footprint compatibility checks;
- pinmap candidate generation and verified pinmap validation;
- round-trip and KiCad CLI validation workflows;
- project template inventory;
- inspection and evaluation reports.

The circuit block library should build on those layers rather than inventing a
parallel representation. Blocks must emit the same transaction/design API
operations used elsewhere, and generated output must remain KiCad-native.

## 3. Goals

The block library must:

- expose reusable, parameterized blocks for common circuits;
- use real KiCad symbols and footprints resolved by the library resolver;
- define schematic connectivity, component values, net naming, footprints,
  placement hints, routing hints, and validation expectations;
- produce deterministic output for the same inputs;
- support composition of multiple blocks into a larger design;
- report uncertainty explicitly rather than claiming unverified correctness;
- support AI-facing discovery of available blocks and parameters;
- allow blocks to be promoted from experimental to verified through repeatable
  tests.

## 4. Non-Goals

Initial implementation does not need to:

- replace a human electrical engineer for final design signoff;
- implement SPICE simulation;
- perform full autorouting;
- guarantee regulator thermal performance or USB compliance beyond encoded
  checks;
- select every possible manufacturer part;
- support arbitrary analog or RF layout constraints;
- generate production-ready BOM sourcing data.

## 5. Definitions

### 5.1 Circuit Block

A circuit block is a reusable design fragment with:

- metadata;
- typed parameters;
- required library symbols and footprints;
- components;
- nets;
- schematic layout hints;
- PCB placement hints;
- routing/zone hints;
- validation rules;
- documentation notes;
- verification status.

### 5.2 Block Instance

A block instance is one configured use of a block inside a project. It has:

- an instance ID;
- parameter values;
- reference designator prefix or allocation scope;
- exported ports;
- internal nets;
- generated components;
- generated transactions.

### 5.3 Port

A port is a named connection point exported by a block. Ports can be electrical
or mechanical.

Examples:

- `VIN`
- `VOUT`
- `GND`
- `SDA`
- `SCL`
- `RESET`
- `USB_VBUS`
- `LED_ANODE`

### 5.4 Verification Level

Blocks must declare one of:

- `experimental`: parses and writes, but is not electrically verified.
- `structural`: symbol/footprint references, nets, and pinmaps validate.
- `roundtrip_verified`: generated files pass KiCad round-trip validation.
- `erc_drc_verified`: available KiCad ERC/DRC or equivalent validation passes.
- `reference_verified`: checked against a known-good reference design.

Only `roundtrip_verified` or stronger blocks may be used by fully autonomous
generation without an explicit warning.

## 6. Block Data Model

The initial Go model should be explicit and serializable.

```go
type BlockDefinition struct {
    ID                 string
    Name               string
    Description        string
    Version            string
    Category           string
    Parameters         []BlockParameter
    Ports              []BlockPort
    RequiredLibraries  []LibraryRequirement
    Components         []BlockComponent
    Nets               []BlockNet
    SchematicHints     []SchematicHint
    PCBHints           []PCBHint
    ValidationRules    []BlockValidationRule
    Verification       VerificationRecord
}
```

The concrete implementation may evolve, but the persisted JSON/YAML shape must
preserve these concepts.

### 6.1 Parameters

Each parameter must define:

- name;
- type: string, enum, number, bool, voltage, current, resistance, capacitance,
  frequency, footprint ID, symbol ID;
- default value when safe;
- allowed values or range;
- units;
- whether the parameter affects schematic, PCB, validation, or documentation;
- user-facing description.

Examples:

- LED block: `supply_voltage`, `led_forward_voltage`, `led_current`,
  `indicator_color`, `resistor_package`.
- Regulator block: `input_voltage_min`, `input_voltage_max`, `output_voltage`,
  `output_current`, `topology`, `package`.
- Op-amp gain block: `gain`, `input_bias_mode`, `supply_mode`, `feedback_package`.

### 6.2 Components

Each component must include:

- logical role;
- reference designator class;
- symbol library ID;
- footprint library ID or footprint selection rule;
- value;
- fields/properties;
- pin mapping requirement;
- placement group;
- optional alternatives.

The resolver must validate that the selected symbol and footprint exist. If the
pinmap is not verified, block generation must return a warning or block
fabrication readiness.

### 6.3 Nets

Nets must be declared intentionally.

Each net should include:

- net name template;
- visibility: internal, exported, global, power;
- connected component pins;
- expected electrical role;
- constraints such as width class or keepout needs.

Block composition must support net aliasing. For example, a regulator block's
`VOUT` can connect to an MCU block's `VCC`.

### 6.4 Layout Hints

Schematic hints should include:

- component relative positions;
- pin orientation expectations;
- label placement;
- power-symbol usage;
- sheet or block boundary grouping.

PCB hints should include:

- relative footprint placement;
- component side;
- preferred rotation;
- keep-close relationships;
- decoupling capacitor distance rules;
- route width class suggestions;
- zone requirements.

Hints are not a substitute for validation. If placement/routing cannot satisfy
required hints, the output must report that clearly.

## 7. Block Registry

The block library should expose a registry.

Required operations:

- list block definitions;
- get a block definition by ID;
- validate parameter values;
- instantiate a block;
- compose multiple block instances;
- produce transactions/design API operations;
- produce a validation report for a block instance.

Suggested Go API:

```go
type Registry interface {
    ListBlocks() []BlockSummary
    GetBlock(id string) (BlockDefinition, bool)
    Instantiate(ctx context.Context, request BlockRequest) (BlockInstance, []reports.Issue)
}
```

Suggested CLI:

```text
kicadai block list --json
kicadai block show <block-id> --json
kicadai block instantiate <block-id> --json --params params.json
kicadai block compose --json --request design.json
```

The CLI should use the existing `reports.Result` envelope.

## 8. Initial Block Specifications

### 8.1 LED Indicator

Purpose: show power, activity, or status.

Parameters:

- `name`
- `supply_voltage`
- `led_forward_voltage`
- `led_current`
- `color`
- `resistor_value`, optional if calculated
- `resistor_package`
- `led_package`
- `active_high`, default true

Components:

- resistor, default `Device:R`;
- LED, default `Device:LED`;
- optional connector or port pins.

Ports:

- `IN`
- `GND`
- optional `VCC`

Rules:

- calculate resistor value when not provided;
- resistor value must be positive and within configured E-series tolerance;
- LED current must be below configured maximum;
- resistor and LED footprint pinmaps must be verified or reported.

Validation:

- schematic net continuity from input to resistor to LED to return;
- footprint assignments present;
- LED polarity is not reversed relative to `active_high`;
- generated PCB has both footprints placed and connected.

### 8.2 Voltage Regulator

Purpose: generate a regulated rail from an input rail.

Initial supported topologies:

- linear regulator, fixed output;
- optional adjustable regulator later.

Parameters:

- `input_voltage_min`
- `input_voltage_max`
- `output_voltage`
- `output_current`
- `regulator_symbol`
- `regulator_footprint`
- `input_capacitance`
- `output_capacitance`
- `package`

Components:

- regulator IC;
- input capacitor;
- output capacitor;
- optional enable pull-up/down resistor;
- optional power LED using the LED block.

Ports:

- `VIN`
- `VOUT`
- `GND`
- optional `EN`

Rules:

- input voltage must exceed output voltage by dropout margin;
- capacitor values must meet regulator family requirements when encoded;
- decoupling capacitors must be placed close to regulator pins;
- thermal warning if estimated dissipation exceeds a configured threshold.

Validation:

- all regulator pins connected or intentionally no-connected;
- input/output capacitors connect to the correct rails;
- VOUT exported net is named consistently;
- PCB placement keeps capacitors near regulator.

### 8.3 MCU Minimal System

Purpose: provide the minimal supporting circuit for a microcontroller.

Parameters:

- `mcu_symbol`
- `mcu_footprint`
- `supply_voltage`
- `clock_mode`: internal, crystal, oscillator;
- `reset_mode`: pullup, supervisor, none;
- `programming_header`: swd, isp, updi, none;
- `decoupling_cap_count`
- `decoupling_cap_value`

Components:

- MCU;
- decoupling capacitors;
- reset pull-up and optional reset switch;
- optional crystal and load capacitors;
- programming/debug header.

Ports:

- `VCC`
- `GND`
- `RESET`
- programming pins;
- named GPIO exports.

Rules:

- every power pin must have a decoupling strategy;
- reset pin must be connected according to selected mode;
- programming header must map to real MCU pins;
- oscillator pins must only be used when clock mode requires them.

Validation:

- MCU symbol and footprint resolve;
- MCU power pins are connected;
- decoupling capacitors are assigned to VCC/GND;
- required programming pins have exported ports or header pins;
- no GPIO port is exported twice under conflicting names.

Known gap:

- MCU pin-function awareness depends on richer symbol metadata than the resolver
  currently extracts. Initial implementations may require a block-local pin map.

### 8.4 USB-C Power Input

Purpose: accept 5 V power from a USB-C receptacle without USB data negotiation.

Parameters:

- `connector_footprint`
- `current_limit`
- `include_fuse`
- `include_tvs`
- `include_power_led`
- `output_net_name`

Components:

- USB-C receptacle;
- CC pull-down resistors;
- optional fuse;
- optional TVS diode;
- optional bulk capacitor;
- optional LED indicator block.

Ports:

- `VBUS_OUT`
- `GND`
- optional `SHIELD`

Rules:

- CC1 and CC2 must each have Rd pull-down resistors for sink mode;
- VBUS pins must join the output rail through protection when selected;
- shield connection policy must be explicit;
- USB data pins are no-connect unless data support is requested later.

Validation:

- CC resistors are present and connected correctly;
- VBUS and GND pins are connected;
- optional fuse is in series with VBUS;
- connector footprint pinmap is verified or blocks fabrication readiness.

### 8.5 I2C Sensor

Purpose: add a common I2C sensor with pull-ups and optional interrupt pin.

Parameters:

- `sensor_symbol`
- `sensor_footprint`
- `supply_voltage`
- `i2c_address`
- `pullup_value`
- `include_pullups`
- `include_interrupt`
- `include_decoupling`

Components:

- sensor IC/module;
- SDA pull-up;
- SCL pull-up;
- decoupling capacitor;
- optional connector pins or exported ports.

Ports:

- `VCC`
- `GND`
- `SDA`
- `SCL`
- optional `INT`

Rules:

- SDA and SCL must have pull-ups unless explicitly provided externally;
- pull-up value must be plausible for bus speed and capacitance;
- sensor address conflicts must be detected during composition;
- interrupt net must be exported when enabled.

Validation:

- SDA/SCL are not swapped;
- pull-ups connect to selected rail;
- decoupling capacitor connects VCC/GND;
- address collision checks run across composed I2C blocks.

### 8.6 Op-Amp Gain Stage

Purpose: create a non-inverting or inverting op-amp gain stage.

Parameters:

- `topology`: non_inverting, inverting;
- `gain`
- `opamp_symbol`
- `opamp_footprint`
- `supply_mode`: single, dual;
- `input_coupling`: dc, ac;
- `feedback_resistor_package`
- `include_output_resistor`

Components:

- op-amp;
- feedback resistor network;
- input resistor where required;
- bias divider for single-supply AC-coupled mode;
- decoupling capacitors;
- optional output resistor.

Ports:

- `IN`
- `OUT`
- `VCC`
- `VEE` or `GND`
- optional `VREF`

Rules:

- resistor ratio must match requested gain within tolerance;
- op-amp supply pins must match supply mode;
- single-supply AC-coupled mode requires bias/reference network;
- decoupling capacitors must be present at supply pins.

Validation:

- feedback network connectivity matches selected topology;
- no op-amp input is floating;
- output net is exported;
- resistor values are finite and within supported ranges.

### 8.7 Connector Breakout

Purpose: break a connector or bus into labeled pins/test points.

Parameters:

- `connector_symbol`
- `connector_footprint`
- `pin_names`
- `pin_count`
- `pitch`
- `include_mounting_holes`
- `include_labels`

Components:

- connector;
- optional test points;
- optional mounting holes;
- optional protection components in later versions.

Ports:

- one exported port per pin name;
- optional mounting/mechanical ports.

Rules:

- pin count must match pin names;
- connector footprint pad count must match expected electrical pins;
- each pin name must be unique unless explicitly allowed;
- power pins should use power-net naming conventions.

Validation:

- every connector pin is labeled;
- exported nets map one-to-one to pads;
- footprint orientation and silkscreen labels are deterministic.

## 9. Composition Rules

Block composition must:

- allocate unique references deterministically;
- support explicit port-to-port connections;
- support net aliases;
- detect incompatible voltage domains;
- detect duplicate I2C addresses on the same bus;
- preserve block-local internal nets unless exported;
- keep block instance metadata so reports can trace generated objects back to
  block IDs.

Example composition request:

```json
{
  "name": "usb_sensor_breakout",
  "blocks": [
    { "id": "usb_c_power", "instance": "USBPWR", "params": { "include_fuse": true } },
    { "id": "regulator", "instance": "REG3V3", "params": { "output_voltage": "3.3V" } },
    { "id": "i2c_sensor", "instance": "SENSOR", "params": { "i2c_address": "0x48" } },
    { "id": "connector_breakout", "instance": "JOUT", "params": { "pin_names": ["3V3", "GND", "SDA", "SCL"] } }
  ],
  "connections": [
    { "from": "USBPWR.VBUS_OUT", "to": "REG3V3.VIN" },
    { "from": "REG3V3.VOUT", "to": "SENSOR.VCC", "net": "3V3" },
    { "from": "SENSOR.SDA", "to": "JOUT.SDA" },
    { "from": "SENSOR.SCL", "to": "JOUT.SCL" }
  ]
}
```

## 10. Verification Workflow

A block can only be marked verified after repeatable evidence is recorded.

Required evidence:

- unit tests for parameter validation;
- generated schematic parse tests;
- generated PCB parse tests where PCB output is supported;
- pinmap validation for every symbol-footprint pair;
- round-trip validation for generated examples;
- known-good fixture comparison or explicit engineer review notes.

Verification records should include:

- level;
- date;
- KiCad version or file format target;
- test command;
- source fixtures;
- unresolved warnings.

## 11. Validation and Reporting

Block instantiation must return `reports.Result` data through CLI workflows.

Reports must include:

- selected block definition and version;
- parameter values after defaults;
- generated refs and nets;
- selected symbols and footprints;
- pinmap status;
- compatibility status;
- validation issues;
- verification level.

Warnings must be explicit for:

- unverified pinmaps;
- heuristic footprint choices;
- missing external library roots;
- unsupported optional features;
- electrical assumptions such as regulator stability or USB current limits.

Blocking errors must be returned for:

- unresolved required symbol;
- unresolved required footprint;
- invalid parameter value;
- impossible net connection;
- missing required port;
- known unsafe electrical configuration;
- writer preservation conflict.

## 12. Storage Format

Initial built-in blocks can be Go definitions for type safety and rapid
iteration. The model should also support future JSON or YAML block packs.

Recommended repository layout:

```text
internal/blocks/
  registry.go
  model.go
  led.go
  regulator.go
  mcu.go
  usbc.go
  i2c_sensor.go
  opamp.go
  connector.go
```

Future user-authored block packs may live under:

```text
blocks/
  vendor_or_project/
    block.json
    examples/
```

External block packs must be loaded as data only. They must not execute code.

## 13. AI Integration

AI agents should interact with blocks through structured discovery and
instantiation, not by manually emitting raw KiCad file syntax.

Minimum AI-facing operations:

- list available blocks;
- inspect required parameters and ports;
- instantiate a block with parameters;
- compose block instances by ports;
- request validation;
- write project;
- inspect generated result.

The AI must be able to see which block outputs are verified and which are only
candidates.

## 14. Interaction With Existing Systems

### 14.1 Library Resolver

Blocks must use resolver-backed symbol and footprint IDs. The resolver supplies:

- symbol pins;
- footprint pads;
- compatibility status;
- pinmap candidates;
- KLC diagnostics;
- template inventory.

### 14.2 Transactions

Blocks should emit transaction operations where possible:

- `add_symbol`;
- `connect`;
- `assign_footprint`;
- `place_footprint`;
- `route`;
- `add_zone`;
- `write_project`.

If a block needs an operation not yet represented, the block must report an
unsupported feature rather than bypassing the transaction safety model.

### 14.3 Design API

Blocks may use the higher-level design API internally when it provides safer
layout primitives. The output must remain inspectable as transactions or
structured operations.

### 14.4 Evaluation

Block-generated projects must run through existing inspection, evaluation,
pinmap, and round-trip checks.

## 15. Example Outputs

Each initial block should eventually have at least one example under
`examples/blocks/<block-id>/`.

Required example artifacts:

- `.kicad_pro`;
- `.kicad_sch`;
- `.kicad_pcb` when PCB output is supported;
- JSON instantiation request;
- validation report snapshot.

## 16. Security and Safety

The block library must:

- avoid executing external block-pack code;
- avoid writing into KiCad library checkouts;
- avoid overwriting projects unless `--overwrite` is explicit;
- prevent path traversal in block-pack and example paths;
- mark unverified designs clearly;
- never claim fabrication readiness from heuristic-only compatibility.

## 17. Known Gaps Before Full Autonomy

The following gaps remain before AI can reliably build full schematic and PCB
designs from blocks without human intervention:

- richer symbol semantic extraction for MCU alternate functions;
- native KiCad pin-stack parsing;
- verified pinmaps for more ICs and connectors;
- stronger decoupling and power-integrity validation;
- regulator stability data per selected part;
- USB-C connector pinmap and protection variants;
- placement/routing constraint solving beyond simple hints;
- ERC/DRC integration where available;
- BOM/manufacturer part selection.

## 18. Acceptance Criteria

The circuit block library is ready for initial use when:

- all seven initial block definitions are registered;
- `block list` and `block show` return deterministic JSON;
- each block validates parameters and required ports;
- each block emits deterministic schematic transactions;
- LED, regulator, I2C sensor, op-amp, and connector blocks can emit basic PCB
  placement hints;
- symbol and footprint choices are resolver-backed;
- unverified pinmaps block fabrication readiness;
- generated examples parse with the existing readers;
- round-trip checks pass for at least LED and connector breakout examples;
- documentation explains verification level and limitations.
