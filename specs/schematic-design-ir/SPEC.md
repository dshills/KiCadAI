# Schematic Design/Layout IR Specification

Date: 2026-07-09

## 1. Summary

Define the first KiCadAI schematic design/layout intermediate representation
for AI-generated schematics.

The IR is a strict, deterministic contract between an AI caller and KiCadAI.
The AI emits JSON or YAML-like structured data. KiCadAI parses, validates,
normalizes, lays out, and converts that data into the existing design workflow
or transaction model. The IR is not natural language, not KiCad S-expression
syntax, and not a replacement for KiCad.

The v1 workflow is:

```text
natural language prompt
  -> AI emits schematic design/layout IR
  -> KiCadAI strict parser and validator
  -> normalized circuit and layout intent
  -> existing design request or transaction adapter
  -> schematic readability checks
  -> KiCad schematic/project writer
```

## 2. Goals

- Provide a constrained AI-facing schema for schematic generation.
- Separate circuit intent, layout intent, and validation/repair policy.
- Prefer boring JSON-compatible data structures over a custom text language.
- Give AI agents enough structure to describe human-readable schematic layout.
- Validate unsupported, ambiguous, or unsafe input before generating KiCad data.
- Map supported IR documents into existing KiCadAI design workflows.
- Preserve deterministic output for stable tests, diffs, and fixture promotion.
- Support examples for:
  - LED indicator;
  - USB-C powered LED indicator;
  - I2C sensor breakout with protected 3.3 V rail.
- Document current gaps that prevent fully general human-readable schematics.

## 3. Non-Goals

- No free-form natural-language parser in this milestone.
- No new LLM provider integration.
- No KiCad S-expression authoring by AI callers.
- No arbitrary circuit synthesis guarantee from natural language; arbitrary
  validated IR is supported only when symbol geometry and pin metadata are
  available from explicit IR fields or a resolver index.
- No PCB routing, board placement, DRC, fabrication, or block-family expansion.
- No full graph-layout optimizer in v1.
- No mutation of imported user schematics by default.
- No guarantee that every valid IR document produces fabrication-ready output.

## 4. Design Principles

1. **AI emits intent, KiCadAI owns KiCad details.**
   AI callers should describe components, nets, groups, and layout rules. KiCadAI
   assigns concrete coordinates, labels, and KiCad-native syntax.

2. **Fail closed before writing files.**
   Missing required references, invalid pin names, unsupported layout policies,
   and unsafe auto-repair requests must produce structured validation issues.

3. **Layout is declarative first.**
   V1 allows relative groups and semantic lanes. Absolute coordinates should be
   optional escape hatches, not the normal path.
   All spatial measurements in IR fields use millimeters. KiCadAI may snap
   generated schematic coordinates to its internal grid after validation.

4. **Readable by construction, scored after generation.**
   The adapter should normalize group order and layout hints before generation,
   then existing readability checks should report remaining issues.

5. **Strict but extensible.**
   Unknown fields are rejected in v1 unless explicitly placed in a future
  `extensions` object.
6. **Forward-compatible experiments are explicit.**
   Unknown fields outside `extensions` are rejected. AI callers may attach
   namespaced metadata under `extensions`, but KiCadAI must not treat
   extension data as electrical truth in v1.

## 5. IR Format

V1 uses JSON as the canonical representation. YAML may be accepted later by
translating to the same Go model, but JSON is the compatibility baseline and
test fixture format.

Top-level shape:

```json
{
  "schema": "kicadai.schematic.ir.v1",
  "version": 1,
  "metadata": {},
  "circuit": {},
  "layout": {},
  "policy": {},
  "extensions": {}
}
```

### 5.1 Metadata

```json
{
  "metadata": {
    "name": "usb_c_i2c_sensor_3v3",
    "title": "USB-C I2C Sensor Breakout",
    "description": "USB-C powered I2C sensor breakout with protected 3.3 V rail",
    "seed": "usb-c-i2c-v1",
    "paper": "A4"
  }
}
```

Rules:

- `name` is required and must be a safe project/design identifier matching
  `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`.
  This is the generated project/design name, not a component identifier.
  Component identity is always `components[].id`; KiCad reference designators
  are `components[].ref`.
- `version` is required at the root. V1 documents must set
  `schema = "kicadai.schematic.ir.v1"` and `version = 1`.
- `title` is optional but recommended.
- `seed` is optional; if absent, KiCadAI derives it from `name`.
- `paper` defaults to `A4`.

### 5.2 Circuit Intent

Circuit intent describes what exists and how it is connected.
All IR identifiers are case-sensitive. Component IDs, net names, port names,
pin selectors, and group IDs must match exactly wherever they are referenced.

```json
{
  "circuit": {
    "components": [
      {
        "id": "r_limit",
        "ref": "R1",
        "role": "current_limiter",
        "symbol": "Device:R",
        "value": "1k",
        "footprint": "Resistor_SMD:R_0805_2012Metric",
        "pins": [
          {"number": "1", "role": "input"},
          {"number": "2", "role": "output", "no_connect": false}
        ],
        "properties": {
          "Tolerance": "1%"
        }
      }
    ],
    "nets": [
      {
        "name": "LED_A",
        "role": "signal",
        "connect": ["r_limit.2", "led.1"]
      }
    ],
    "ports": [
      {
        "name": "VIN",
        "direction": "input",
        "electrical_type": "power",
        "net": "VIN",
        "side": "left"
      }
    ],
    "extensions": {}
  }
}
```

Component fields:

- `id`: required stable local ID for all IR references.
  IDs must match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`; dots are forbidden because
  endpoint syntax uses `component_id.pin`.
- `ref`: optional KiCad reference. If absent, KiCadAI may assign one by role
  only when `policy.repair.allow_ref_assignment` is true. If that repair
  policy is false, a missing or empty `ref` is a validation error.
- `unit`: optional KiCad multi-unit identifier for shared-reference symbols,
  such as `A`, `B`, or `power`. Components with the same non-empty `ref` are
  allowed only when every member of that shared-reference set has a unique
  non-empty `unit`, identical `symbol`, and identical non-empty `footprint`
  values.
- `role`: required semantic role for layout and diagnostics.
  V1 accepts a bounded semantic vocabulary for common schematic generation:
  `connector`, `input_connector`, `output_connector`, `resistor`,
  `current_limiter`, `pullup`, `capacitor`, `decoupling_capacitor`,
  `bulk_capacitor`, `inductor`, `diode`, `indicator_led`, `ic`, `sensor`,
  `regulator`, `transistor`, `bjt`, `mosfet`, `switch`, `crystal`,
  `oscillator`, `protection`, `fuse`, `tvs`, `power_symbol`, `ground_symbol`,
  `testpoint`, and `generic`.
  Unknown roles are invalid unless a future version or explicit extension
  namespace declares them.
- `symbol`: required KiCad symbol library ID in `Library:Name` form.
- `value`: optional visible value string. Electrical values must include an
  explicit unit or KiCad-style suffix where a unit matters, such as `100k`,
  `10uF`, `1n`, `3.3 V`, or `2A`. Bare numeric values are allowed only for
  dimensionless identifiers or part names where the component role makes the
  value non-electrical.
  V1 validators must accept this conservative electrical value grammar:
  `^[+-]?([0-9]*\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][+-]?[0-9]+)?\s*(p|n|u|U|µ|μ|m|k|K|Meg|M|G|T)?\s*(Ohm|ohm|Ω|R|F|H|h|V|v|A|a|W|w|Hz|hz)?\s*$`
  or the common embedded-marker notation
  `^[+-]?([0-9]+(R|p|n|u|U|µ|μ|m|k|K|Meg|M|G|T)[0-9]*|[0-9]*(R|p|n|u|U|µ|μ|m|k|K|Meg|M|G|T)[0-9]+)\s*(Ohm|ohm|Ω|F|H|h|V|v|A|a|W|w|Hz|hz)?\s*$`,
  such as `4R7`, `2n2`, or `0R1`.
  Validators should trim leading and trailing whitespace before applying these
  patterns.
  Prefix parsing is intentionally not fully case-insensitive: `m` means milli,
  while `M` and `Meg` mean mega. V1 does not accept the femto prefix or
  lowercase `f` Farad spelling to avoid ambiguity with femtofarads.
  V1 `value` is a display string, not a typed numeric property. KiCadAI must
  not infer unitless numeric magnitudes from it. Any future typed numeric
  electrical fields must use SI base units such as Ohms, Farads, Henrys, Volts,
  Amps, Watts, and Hertz.
  If no unit token is present, the SI suffix itself is treated as a
  KiCad-style value abbreviation only for resistor, capacitor, and inductor
  roles where that convention is already unambiguous.
- `footprint`: optional KiCad footprint library ID.
- `pins`: optional pin hints for AI clarity, but strongly recommended for
  hermetic validation. If omitted, pin validation requires a symbol-library
  context or a known local template. If neither is available, validation must
  fail before adapter generation for connected components.
- `pins[].no_connect`: optional boolean for intentionally unconnected pins.
  This is the preferred v1 representation for NC pins because it avoids dummy
  net names. A pin marked `no_connect` must not appear in any net endpoint.
- `pins[].role`: optional IR-level semantic role. When a symbol-library context
  is available, validation must ensure the role does not contradict the KiCad
  electrical pin type. If no library context is available, role/type
  compatibility is reported as unchecked evidence rather than guessed.
- `pins[].offset_x_mm` and `pins[].offset_y_mm`: optional explicit schematic pin
  anchor coordinates relative to the symbol origin. These are required when an
  external or synthetic symbol is not covered by KiCadAI's verified template
  metadata. Omitted axes inherit the resolved template value as zero; an
  explicitly provided zero is preserved. Explicit geometry takes precedence
  over template geometry for both layout and generated transaction anchors.
- `body`: optional verified symbol graphics bounds relative to the symbol origin,
  with `min_x_mm`, `min_y_mm`, `max_x_mm`, and `max_y_mm`. When present, these
  bounds are used for body-aware placement, routing obstacles, field placement,
  and readability validation. They must be finite and have positive width and
  height. When omitted, KiCadAI uses its resolved template or conservative role
  fallback; arbitrary external symbols should provide this field or resolver
  evidence before strict readable acceptance.

On readback, the schematic adapter uses the embedded KiCad `lib_symbols` body
to recover pin anchors and graphics bounds for symbols without a KiCadAI
verified template. Verified templates remain authoritative because they may
contain deliberate connection-anchor corrections for existing generated
fixtures. This precedence must be deterministic and must never guess pin
geometry when neither source is available.
- `properties`: optional string map for symbol properties.

Net fields:

- `name`: required except for `no_connect` records. If a `no_connect` net
  omits `name`, normalization must assign a deterministic internal NC name
  from its single endpoint.
- `role`: one of `signal`, `bus`, `power`, `power_pos`, `power_neg`, `ground`,
  `return`, `feedback`, `bias`, `shield`, or `no_connect`. `power` is a
  generic power rail when polarity is unknown. In v1, `bus` means one logical
  bus member such as SDA or SCL and is emitted as a label-friendly electrical
  net. True KiCad vector-bus graphics, bus entries, and bus-member expansion
  remain unsupported and must fail closed rather than be guessed.
- `connect`: required list of endpoints. Endpoints use the mandatory component `id` plus pin selector: `component_id.pin`. The endpoint grammar is `<component_id>.<pin_selector>`, both parts must be non-empty, and a missing dot is invalid. The first dot separates the component ID from the pin selector; because component IDs cannot contain dots, pin selectors may still contain dots when a symbol library requires them. The pin selector must match the component `pins[].number` value or a pin number from resolved symbol metadata. Explicit KiCad `ref` values are output names only and must not be used for IR-local references. Future named-pin aliases may be accepted only after validation resolves them to pin numbers.
- Multiple net records may share the same `name` as an AI-authoring
  convenience. Normalization must merge them into one net before adapter
  generation. Duplicate-name net records are valid only when their `role`,
  `label`, and `use_label` values are absent or identical after defaults are
  applied. More precisely, all non-absent values for `role`, `label`, and
  `use_label` across records with the same name must be identical.
- `no_connect` may also be expressed as a one-endpoint net role for adapters
  that already reason in net records. `pins[].no_connect` is preferred for new
  AI-authored documents. A `no_connect` net with more than one endpoint is
  invalid because it would electrically connect those pins.
- `label`: optional requested display label.
- `use_label`: optional boolean. If absent, layout rules decide.

Port fields:

- `name`: required.
- `direction`: `input`, `output`, `bidirectional`, `passive`, `tri_state`, or
  `unspecified`.
- `electrical_type`: optional KiCad-facing electrical type. V1 accepts
  `input`, `output`, `bidirectional`, `tri_state`, `passive`, `unspecified`,
  `power_input`, `power_output`, `open_collector`, `open_emitter`, and
  `no_connect`.
- `net`: required.
- `side`: `left`, `right`, `top`, or `bottom`.

`direction` is a schematic-layout hint. `electrical_type` is the KiCad-facing
electrical mapping. If both are present and conflict, such as
`direction = input` with `electrical_type = power_output`, validation must fail
instead of guessing.

Valid direction/electrical-type combinations:

- `input`: `input`, `passive`, `power_input`, or `unspecified`.
- `output`: `output`, `passive`, `power_output`, `open_collector`,
  `open_emitter`, or `unspecified`.
- `bidirectional`: `bidirectional`, `passive`, or `unspecified`.
- `tri_state`: `tri_state` or `unspecified`.
- `passive`: `passive` or `unspecified`.
- `unspecified`: any supported `electrical_type`.

### 5.3 Layout Intent

Layout intent describes how the schematic should read.

```json
{
  "layout": {
    "flow": "left_to_right",
    "origin": "centered",
    "groups": [
      {
        "id": "input",
        "label": "Input",
        "role": "input_stage",
        "members": ["vin", "input_cap"],
        "rank": 0,
        "side": "left"
      },
      {
        "id": "output",
        "label": "Output",
        "role": "output_stage",
        "members": ["led", "output_connector"],
        "rank": 2,
        "side": "right"
      }
    ],
    "lanes": {
      "power": "top",
      "power_negative": "lower",
      "ground": "bottom",
      "signals": "middle"
    },
    "rules": {
      "positive_power_top": true,
      "ground_bottom": true,
      "center_on_page": true,
      "prefer_labels_for_long_nets": true,
      "avoid_wire_crossings": true,
      "min_group_spacing_mm": 12.7,
      "min_component_spacing_mm": 7.62
    },
    "placements": [
      {
        "target": "regulator_ic",
        "group": "regulator",
        "near": ["input_cap", "output_cap"],
        "orientation": "normal"
      }
    ]
  }
}
```

Required v1 semantics:

- `flow` supports `left_to_right` only in v1.
- `origin` supports `centered` and `page_upper_left`; `centered` is preferred.
- `groups` define functional stages and relative order.
- `rank` orders groups left to right and is authoritative for global group
  order.
- groups with identical `rank` are sorted by `id` for deterministic output.
- `side` is a port/edge affinity hint. It must not reorder groups. If `side`
  conflicts with `rank`, validation fails so strict IR documents do not carry
  contradictory layout intent.
- `lanes.power = top` means positive rails and power labels should be above
  the signal path.
- `lanes.ground = bottom` means ground symbols and return labels should be
  below the signal path.
- General rules such as `positive_power_top` and `ground_bottom` are shorthand
  defaults. Explicit `lanes` values are more specific and win over those
  shorthand rules. In v1, validators should reject contradictory lane values
  for required roles rather than silently inventing a mixed layout policy.
- `layout.origin` is authoritative for page anchoring. If
  `layout.origin = centered`, `rules.center_on_page` must be absent or true. If
  `layout.origin = page_upper_left`, `rules.center_on_page` must be absent or
  false. Explicit contradictions are validation errors.
- Net roles map to lanes as follows:
  - `power` and `power_pos` map to the power lane;
  - `power_neg` maps to `lanes.power_negative`. If any net uses `power_neg`,
    `lanes.power_negative` is required in v1 so negative rails are never hidden
    in the ground band;
  - `ground`, `return`, and `shield` map to the ground lane;
  - `signal`, `feedback`, and `bias` map to the signal lane unless an explicit
    future rule says otherwise;
  - `no_connect` pins are marked in place and never participate in lane-based
    flow.
- `prefer_labels_for_long_nets` lets KiCadAI use labels instead of long wires.
- `placements` are hints, not hard coordinates, unless `absolute` is added in a
  future version.
- Layout precedence is deterministic:
  1. group `rank` defines global left-to-right order;
  2. lane rules define top/middle/bottom vertical bands;
  3. `placements.near` defines local offsets inside or adjacent to the selected
     group;
  4. conflicting or circular `near` hints are non-binding heuristics; the
     deterministic fallback is first valid target by document order;
  5. spacing rules may expand distances but must not reorder groups.
- `side` refers to global schematic boundary affinity. `side` and `rank`
  conflict only for edge hints: `side = left` is valid only on the minimum rank
  in the document, and `side = right` only on the maximum rank. `side = top`
  and `side = bottom` are vertical hints and do not conflict with horizontal
  rank ordering.

### 5.4 Validation And Repair Policy

```json
{
  "policy": {
    "validation": {
      "missing_pins": "error",
      "unresolved_symbols": "error",
      "unresolved_footprints": "warning"
    },
    "repair": {
      "allow_ref_assignment": true,
      "allow_label_insertion": true,
      "allow_group_spacing_adjustment": true,
      "allow_symbol_substitution": false,
      "allow_pin_guessing": false
    },
    "acceptance": "structural"
  }
}
```

Policy fields:

- `validation`: controls fail-closed behavior.
- `repair`: defines what KiCadAI may normalize or auto-fix.
- `acceptance`: maps to existing design workflow acceptance values where
  possible.
  V1 supports:
  - `structural`: IR parses, validates, normalizes, and maps to schematic
    generation data without requiring KiCad ERC execution.
  - `erc_clean`: generated schematic must also pass KiCad ERC where KiCad CLI
    is available.
  - `readable`: `erc_clean` acceptance plus schematic readability checks when
    KiCad CLI is available; otherwise structural acceptance plus readability
    checks with an explicit missing-ERC evidence note.

For `readable` project writes, KiCadAI also re-reads every generated schematic
after serialization and applies the strict readability profile to the final
KiCad-native geometry. Layout-selected paper size is propagated through the
transaction apply path, including hierarchy child sheets.

V1 must default to conservative behavior:

- ref assignment allowed;
- group spacing adjustment allowed;
- labels allowed;
- symbol substitution disallowed;
- pin guessing disallowed.

## 6. Examples

### 6.1 LED Indicator

```json
{
  "schema": "kicadai.schematic.ir.v1",
  "version": 1,
  "metadata": {
    "name": "ir_led_indicator",
    "title": "LED Indicator"
  },
  "circuit": {
    "components": [
      {"id": "vin", "ref": "J1", "role": "input_connector", "symbol": "Connector_Generic:Conn_01x02", "value": "VIN", "pins": [{"number": "1"}, {"number": "2"}]},
      {"id": "r_limit", "ref": "R1", "role": "current_limiter", "symbol": "Device:R", "value": "1k", "footprint": "Resistor_SMD:R_0805_2012Metric", "pins": [{"number": "1"}, {"number": "2"}]},
      {"id": "led", "ref": "D1", "role": "indicator_led", "symbol": "Device:LED", "value": "LED", "footprint": "LED_SMD:LED_0805_2012Metric", "pins": [{"number": "1"}, {"number": "2"}]}
    ],
    "nets": [
      {"name": "VIN", "role": "power", "connect": ["vin.1", "r_limit.1"]},
      {"name": "LED_A", "role": "signal", "connect": ["r_limit.2", "led.1"]},
      {"name": "GND", "role": "ground", "connect": ["led.2", "vin.2"]}
    ]
  },
  "layout": {
    "flow": "left_to_right",
    "origin": "centered",
    "groups": [
      {"id": "input", "members": ["vin"], "rank": 0, "side": "left"},
      {"id": "indicator", "members": ["r_limit", "led"], "rank": 1, "side": "right"}
    ],
    "lanes": {"power": "top", "ground": "bottom", "signals": "middle"},
    "rules": {
      "positive_power_top": true,
      "ground_bottom": true,
      "center_on_page": true,
      "prefer_labels_for_long_nets": true,
      "avoid_wire_crossings": true,
      "min_group_spacing_mm": 18,
      "min_component_spacing_mm": 10
    }
  },
  "policy": {
    "repair": {
      "allow_ref_assignment": true,
      "allow_label_insertion": true,
      "allow_group_spacing_adjustment": true,
      "allow_symbol_substitution": false,
      "allow_pin_guessing": false
    },
    "acceptance": "structural"
  }
}
```

### 6.2 USB-C Powered LED Indicator

The USB-C LED example should use groups:

- `usb_input`, rank 0, left side;
- `protection`, rank 1;
- `indicator`, rank 2, right side;
- `ground_return`, bottom lane.

Required rules:

- VBUS and protected VBUS use the top power lane.
- CC pull-downs are placed near the USB-C connector.
- TVS ground return is below and close to the USB-C connector.
- LED resistor and LED flow left to right.

### 6.3 I2C Sensor With 3.3 V Regulator

The I2C sensor example should use groups:

- `usb_input`, rank 0;
- `regulator`, rank 1;
- `sensor`, rank 2;
- `header`, rank 3.

Required rules:

- VBUS and 3.3 V rails run through top power lanes.
- GND and returns stay in the lower lane.
- regulator input/output capacitors are near the regulator;
- sensor decoupling capacitor is near the sensor VCC/GND pins;
- SDA and SCL run left to right through the signal lane;
- pull-ups sit above the I2C signal lines near the sensor group;
- header sits on the right edge.

## 7. Mapping To Existing KiCadAI

V1 should map IR to existing systems in this order:

1. Parse and validate `schematicir.Document`.
2. Normalize duplicate-name nets, refs, groups, defaults, and layout rule
   values.
3. Convert known block-shaped IR to `designworkflow.Request` when possible.
4. Convert component/net-only IR to `transactions.Transaction` for schematic
   generation when block mapping is unavailable.
5. Use existing `designapi.Builder` and schematic writer for KiCad output.
6. Run existing schematic readability checks and expose diagnostics.

For arbitrary library symbols, callers should use the resolver-aware adapter and
pass a `libraryresolver.LibraryIndex`. That path supplies resolved pin anchors
and graphic bounds to layout before transactions are emitted, embeds the raw
KiCad symbol body with its qualified library ID during apply, and preserves the
embedded body for readback geometry recovery. Template geometry remains the
authoritative source for verified symbols with known KiCadAI connection-anchor
corrections.

When a resolver index is supplied, a missing requested symbol or assigned
footprint is a blocking adapter issue before transaction emission. This keeps
resolver-backed AI generation fail-closed instead of silently falling back to
incomplete geometry. Without an index, the template-only path remains available,
but external-symbol geometry and library compatibility are explicitly unchecked
evidence and strict readable acceptance requires explicit geometry or resolver
evidence.

### 7.1 Design Workflow Mapping

When the IR describes a supported block composition, the adapter should create a
`designworkflow.Request` with:

- `name` from metadata;
- block instances inferred from groups/components where safe;
- existing validation acceptance from policy;
- layout metadata preserved for future placement/readability stages.

### 7.2 Transaction Mapping

When no block mapping exists, the adapter may emit a schematic-only transaction:

- `create_project`;
- `add_symbol` for each component;
- `connect` or `add_label` for each net;
- optional `assign_footprint`;
- `write_project`.

IR ports are emitted as KiCad global labels on the physically edge-most
resolved endpoint for the requested `side` (left/right uses X position; top/
bottom uses Y position). Port-bearing nets always use direct wiring plus the
global port label. This keeps the port's external/cross-sheet semantics intact
and prevents generic local-label preferences from creating duplicate labels.

The v1 adapter may generate schematic-only projects. PCB output can remain a
future phase unless the IR maps cleanly to existing `design create` requests.

## 8. Readability Policy

The IR must support these readability goals:

- signal flow left to right;
- inputs on left and outputs on right;
- positive rails near top;
- ground references near bottom;
- decoupling near IC power pins;
- pull-ups near bus lines;
- readable group spacing;
- centered schematic on page;
- labels instead of long crossing wires.

V1 does not need a perfect layout solver. It must preserve enough explicit
layout intent for deterministic placement and readability scoring.

## 9. Repair Policy

Allowed v1 repairs:

- assign missing refs when `allow_ref_assignment` is true;
- insert labels when `allow_label_insertion` is true;
- increase group/component spacing when `allow_group_spacing_adjustment` is true;
- normalize enum aliases, such as `ltr` to `left_to_right`, if documented.

Forbidden v1 repairs:

- guessing unresolved pins;
- replacing symbols or footprints;
- changing electrical nets;
- inventing components;
- silently dropping components, nets, groups, or ports.

## 10. Validation Rules

Parser/validator must reject:

- unknown top-level fields;
- unsupported schema IDs;
- root documents whose `schema` and `version` do not agree;
- duplicate component IDs;
- duplicate explicit refs, unless all components sharing that `ref` declare
  unique non-empty `unit` values;
- components sharing the same `ref` whose `symbol` values differ, or whose
  non-empty `footprint` values differ;
- duplicate net names with conflicting roles, labels, or `use_label` values;
- component IDs that do not match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`;
- missing component roles;
- `symbol` values that do not contain exactly one `Library:Name` separator;
- non-empty `footprint` values that do not contain exactly one `Library:Name`
  separator;
- endpoints that reference unknown component IDs;
- endpoints whose pin selector does not match the target component's
  `pins[].number` list when that component defines `pins`;
- non-`no_connect` nets where the sum of unique component pin endpoints and
  ports referencing that net is less than two;
- ports whose `net` field does not match a declared `circuit.nets[].name`;
- `no_connect` nets with more than one endpoint;
- pins marked `no_connect` that also appear in any net endpoint;
- pin role compatibility with KiCad electrical types when symbol metadata is
  available;
- layout groups with unknown members;
- component IDs that appear in more than one layout group's `members` list;
- invalid enum values;
- bare numeric electrical `value` strings for roles that require unit-bearing
  numeric values: `resistor`, `current_limiter`, `pullup`, `capacitor`,
  `decoupling_capacitor`, `bulk_capacitor`, and `inductor`.
  Regulator, diode, LED, fuse, and TVS values are treated as part/display
  strings in v1 because values such as `LM7805`, `1N4148`, `RED`, `LED`,
  `0603L050`, and `SMAJ5.0A` are conventional;
- unsafe design names;
- metadata names that do not match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`;
- components without non-empty `ref` values when
  `policy.repair.allow_ref_assignment` is false;
- policies that request unsupported repair behavior.

Parser/validator should warn:

- footprint omitted where PCB generation is later requested;
- layout group exists but has no members;
- high-fanout net lacks label preference;

## 11. Test Strategy

- Unit tests for strict decode and validation.
- Golden examples for the three required fixtures.
- Adapter tests proving IR maps to existing design request or transaction data.
- Layout normalization tests for group rank, lanes, spacing defaults, and
  center-on-page defaults.
- CLI tests once a command is added.
- Existing design and KiCad-backed fixture tests must continue to pass.

## 12. Current Gaps

The current codebase already has:

- structured `designworkflow.Request`;
- transaction apply/write support;
- schematic writer support;
- schematic readability checks;
- natural-language intent draft infrastructure;
- block-level schematic and PCB generation.

Remaining gaps for high-quality AI-generated schematics:

- KiCad vector-bus graphics, bus entries, member-label expansion, grid
  normalization, KiCad ERC, and schematic round-trip coverage are implemented
  for root-sheet IR generation; cross-sheet vector-bus propagation remains
  unsupported and fails closed;
- resolver-aware output still depends on configured KiCad library roots or
  explicit symbol geometry; missing geometry fails closed for strict readable
  acceptance;
- explicit pin offsets must agree with KiCadAI’s validated connection-anchor
  overrides for embedded symbols; conflicting offsets fail closed before any
  file is written;
- inherited symbols in split `.kicad_symdir` libraries require the
  resolver-backed adapter to materialize their base symbol body before
  embedding;
- readability repair remains report-driven rather than a general automatic
  optimization loop;
- KiCad-backed ERC/DRC evidence remains environment-gated and is not implied by
  structural IR validation.

## 13. Open Questions

- Should YAML be accepted in v1 or deferred until JSON is stable?
- How much block inference should v1 attempt before falling back to
  schematic-only transactions?
- Should `extensions` be preserved in generated project metadata in v1, or only
  accepted and ignored by the core adapter?
- Should schematic readability repair run automatically for IR-generated
  projects, or remain report-only until the repair loop is mature?

## 14. Acceptance Criteria

This milestone is complete when:

- `SPEC.md` and `PLAN.md` are checked in;
- a Go data model exists for v1 IR;
- strict parsing and validation are tested;
- required example IR fixtures exist and validate;
- at least one adapter path maps IR into an existing KiCadAI generation path;
- layout intent is normalized into deterministic hints;
- a CLI or integration command validates or consumes IR;
- docs explain the AI-facing contract;
- staged code changes were reviewed with Prism before each commit.
