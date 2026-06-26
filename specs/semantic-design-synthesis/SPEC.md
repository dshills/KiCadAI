# Semantic Design Synthesis Specification

Date: 2026-06-26

## 1. Summary

KiCadAI can now draft intent, plan structured requests, generate KiCad projects,
and explain generated artifacts through `design-rationale.json`. The next gap is
semantic design synthesis: turning higher-level structured intent into a richer,
internally justified circuit topology before `design create` runs.

This project extends the deterministic `intentplanner` layer so it can reason
about supported MCU peripherals, interfaces, voltage domains, calculated values,
ratings, and fabrication policy as a connected synthesis problem. The output is
still a conservative `designworkflow.Request`; unsupported or ambiguous designs
must become clarifications, known gaps, or blockers rather than guessed
schematics.

## 2. Goals

- Add a machine-readable synthesis trace to intent plans.
- Resolve richer MCU/interface/voltage-domain topology for supported blocks.
- Expand supported peripherals beyond the first I2C path where evidence exists.
- Safely synthesize external-clock support for the seed MCU only when the block
  and writer can produce the required topology.
- Propagate voltage, current, package, and acceptance constraints into component
  policy and block parameters.
- Add value-calculation evidence for supported simple networks.
- Feed synthesis decisions into `design-rationale.json`.
- Preserve deterministic behavior and fail closed on unsupported topology.

## 3. Non-Goals

- No LLM calls.
- No natural-language parser expansion beyond consuming the existing
  `intentdraft` output.
- No arbitrary MCU alternate-function extraction from KiCad symbols in this
  project; resolver-backed alternate-function support may be added later.
- No full analog design synthesis.
- No production fabrication guarantee beyond existing validation and
  fabrication-readiness gates.
- No mutation of imported user projects.
- No broad component sourcing, pricing, or availability integration.

## 4. Existing Foundations

This work builds on:

- `internal/intentplanner`
  - strict `Request` schema;
  - `TargetRef`, `FunctionIntent`, `InterfaceIntent`, `PowerRailIntent`;
  - `PlanResult`, requirements, selected blocks/components, connections,
    assumptions, known gaps, and issues;
  - semantic target, bus, and supply mapping foundation.
- `internal/designworkflow`
  - executable generated request consumed by `design create`;
  - stage summaries, acceptance, validation, retry, repair, fabrication policy.
- `internal/blocks`
  - verified block definitions and PCB realization metadata;
  - MCU minimal, I2C sensor, reset/programming, crystal/canned oscillator,
    regulator, USB-C power, ESD, reverse-polarity, op-amp, connector blocks.
- `internal/components`
  - catalog-backed selection, confidence, acceptance, ratings, packages,
    pinmaps, companions, and rejected-candidate evidence.
- `internal/rationale`
  - consolidated AI-facing report and known-limit taxonomy.

## 5. Problem Statement

The planner currently understands enough semantics to connect selected seed
blocks, but it is still closer to a rule mapper than a synthesis engine:

- MCU support is centered on one seed template and a small set of explicit
  roles.
- external-clock intent remains blocked because the generated MCU block cannot
  yet prove a safe non-internal-clock topology.
- voltage-domain evidence is present but does not consistently drive every
  downstream part rating, block parameter, connector, and validation decision.
- current and package requirements are captured, but not always traceable to
  candidate component constraints.
- simple values such as regulator companions, pull-ups, decoupling, oscillator
  load capacitors, LED current limit, and op-amp gain are either block-local or
  implicit rather than planner-visible evidence.
- rationale reports summarize planner output, but they do not yet distinguish
  "synthesis chose this topology because..." from lower-level workflow issues.

## 6. Synthesis Trace

Add a stable synthesis trace to `intentplanner.PlanResult`.

```go
type SynthesisTrace struct {
    Schema        string               `json:"schema"`
    Status        string               `json:"status"`
    Decisions     []SynthesisDecision  `json:"decisions,omitempty"`
    Evidence      []SynthesisEvidence  `json:"evidence,omitempty"`
    Constraints   []SynthesisConstraint `json:"constraints,omitempty"`
    Calculations  []SynthesisCalculation `json:"calculations,omitempty"`
    Gaps          []SynthesisGap       `json:"gaps,omitempty"`
}
```

Schema:

- `kicadai.intent.synthesis.v1`

Status values:

- `ready`
- `partial`
- `needs_clarification`
- `blocked`

The status must not be more optimistic than the parent plan status.

### 6.1 Synthesis Decisions

```go
type SynthesisDecision struct {
    ID             string   `json:"id"`
    Type           string   `json:"type"`
    Path           string   `json:"path,omitempty"`
    Selected       string   `json:"selected"`
    Rationale      string   `json:"rationale"`
    RequirementIDs []string `json:"requirement_ids,omitempty"`
    EvidenceIDs    []string `json:"evidence_ids,omitempty"`
    Confidence      string   `json:"confidence,omitempty"`
}
```

Decision types:

- `topology`
- `target_resolution`
- `bus_resolution`
- `voltage_domain`
- `component_constraint`
- `value_calculation`
- `validation_policy`
- `unsupported_gap`

### 6.2 Synthesis Evidence

```go
type SynthesisEvidence struct {
    ID         string   `json:"id"`
    Kind       string   `json:"kind"`
    Path       string   `json:"path,omitempty"`
    Summary    string   `json:"summary"`
    Source     string   `json:"source,omitempty"`
    Confidence string   `json:"confidence,omitempty"`
    Refs       []string `json:"refs,omitempty"`
}
```

Evidence kinds:

- `intent_field`
- `semantic_port`
- `block_capability`
- `component_policy`
- `component_rating`
- `calculation_input`
- `workflow_policy`
- `known_gap`

### 6.3 Constraints

```go
type SynthesisConstraint struct {
    ID          string `json:"id"`
    Path        string `json:"path,omitempty"`
    Kind        string `json:"kind"`
    Subject     string `json:"subject"`
    Operator    string `json:"operator,omitempty"`
    Value       string `json:"value,omitempty"`
    Source      string `json:"source,omitempty"`
    Requirement string `json:"requirement_id,omitempty"`
}
```

Constraint kinds:

- `voltage`
- `current`
- `package`
- `confidence`
- `acceptance`
- `interface`
- `target`
- `fabrication`
- `routing`

### 6.4 Calculations

```go
type SynthesisCalculation struct {
    ID          string            `json:"id"`
    Kind        string            `json:"kind"`
    Path        string            `json:"path,omitempty"`
    Inputs      map[string]string `json:"inputs,omitempty"`
    Result      map[string]string `json:"result,omitempty"`
    Formula     string            `json:"formula,omitempty"`
    Assumptions []string          `json:"assumptions,omitempty"`
    Confidence  string            `json:"confidence,omitempty"`
}
```

Initial calculation kinds:

- `led_resistor`
- `i2c_pullup`
- `regulator_headroom`
- `decoupling_policy`
- `crystal_load_cap`
- `opamp_gain`

Calculations may produce policy recommendations before exact component value
selection is fully implemented. If a calculation cannot be completed with known
inputs, emit a gap and a clarification/assumption as appropriate.

## 7. Supported Initial Synthesis Capabilities

### 7.1 MCU Peripheral Topology

For the ATmega328P-A seed template:

- continue supporting I2C SDA/SCL resolution;
- add structured evidence for UART and SPI support when selected by
  reset/programming or connector intent;
- expose clock-capable ports in the synthesis trace;
- distinguish internal-clock, external-crystal, and canned-oscillator topology.

External clock generation must remain blocked until the MCU block can emit
schematic/PCB content that proves the topology. Once implemented, the planner
must:

- select crystal or oscillator block based on request;
- connect XTAL1/XTAL2 or clock input according to block capability;
- add required local routes and placement proximity constraints;
- emit calculation evidence for load caps or oscillator decoupling;
- request ERC/DRC or fabrication evidence according to acceptance.

### 7.2 Interface And Bus Synthesis

Supported interfaces:

- I2C:
  - resolve shared bus alias;
  - connect MCU, sensor, and connector members;
  - require or infer pull-ups when no block supplies them;
  - calculate default pull-up range when voltage and rough bus role are known.
- UART:
  - connect TX/RX between MCU and connector/programming block;
  - emit target-resolution evidence and direction notes.
- SPI/ISP:
  - connect MOSI/MISO/SCK/RESET/VCC/GND for supported programming intent;
  - block if target MCU is ambiguous or unsupported.
- GPIO connector:
  - support simple connector breakouts with explicit net aliases;
  - do not guess pin assignments for unsupported MCU GPIO functions.

### 7.3 Voltage-Domain Synthesis

The planner must build a voltage-domain table from:

- power inputs;
- rails;
- regulator outputs;
- USB-C VBUS;
- block supply requirements;
- explicit `functions[].supply`;
- `power.rails[].alias`;
- `power.rails[].supplied_targets`.

The table must drive:

- selected block parameters;
- generated connection aliases;
- component rating constraints;
- connector voltage labels;
- validation assumptions;
- rationale evidence.

Conflicts block:

- unknown supply alias;
- incompatible explicit voltage;
- multiple compatible rails with no target or alias;
- current requirement beyond known policy.

### 7.4 Component Constraint Synthesis

The planner must translate intent into component policy constraints:

- minimum confidence from requested acceptance;
- package preferences;
- required voltage/current ratings;
- temperature/lifecycle/fabrication constraints when modeled;
- placeholder allowance;
- required symbol/footprint/pinmap evidence level.

These constraints should be visible in both:

- `PlanResult.SelectedComponents`;
- `PlanResult.Synthesis.Constraints`.

### 7.5 Value Calculation Synthesis

Add deterministic planner-visible calculations for supported simple cases:

- LED series resistor from input/LED voltage and current when provided.
- I2C pull-up recommended range from voltage and default low-speed bus policy.
- Regulator headroom warning from input and output voltage.
- Crystal load capacitor estimate when crystal CL and stray capacitance are
  provided or defaulted.
- Op-amp gain resistor ratio from requested gain.
- Decoupling policy evidence for power pins and IC blocks.

If a value is only block-local today, the synthesis trace can initially record
the expected policy and defer exact component mutation to later phases.

## 8. Request Schema Extensions

Avoid large breaking changes. Add optional fields only.

### 8.1 Function Params

Use existing `FunctionIntent.Params` for:

- `frequency`
- `clock_source`
- `gain`
- `load_cap_pf`
- `led_forward_voltage`
- `led_current_ma`
- `bus_speed_hz`
- `pullup_ohms`

Future typed fields may be added after repeated patterns stabilize.

### 8.2 Constraints

Extend `ConstraintIntent` only if needed:

```go
type ConstraintIntent struct {
    PreferSMD          bool              `json:"prefer_smd,omitempty"`
    AllowPlaceholders  bool              `json:"allow_placeholders,omitempty"`
    PackagePreferences map[string]string `json:"package_preferences,omitempty"`
    RouteWidthMM       float64           `json:"route_width_mm,omitempty"`
    ClearanceMM        float64           `json:"clearance_mm,omitempty"`
    SkipRouting        bool              `json:"skip_routing,omitempty"`
    TemperatureRangeC  string            `json:"temperature_range_c,omitempty"`
    Lifecycle          string            `json:"lifecycle,omitempty"`
}
```

If these fields are added, strict decoding tests must prove old fixtures remain
valid and unknown fields still fail.

## 9. Rationale Integration

`internal/rationale` must map synthesis trace records into:

- `Decision` records:
  - topology;
  - target resolution;
  - bus resolution;
  - voltage-domain choice;
  - value calculation;
  - component constraints.
- `EvidenceRecord` records:
  - semantic ports;
  - block capabilities;
  - component policy;
  - calculation inputs/results.
- `KnownLimit` records for synthesis gaps.

Rationale reports must let an AI answer:

- which topology was selected;
- which targets and buses were resolved;
- which values were calculated or deferred;
- why a request was blocked or partially supported;
- which downstream validation stage still needs proof.

## 10. CLI Behavior

No new top-level command is required.

Existing commands should include the synthesis trace:

```sh
kicadai --json --request ./intent.json intent plan
kicadai --json --request ./intent.json intent explain
kicadai --json --request ./intent.json intent rationale
kicadai --json --request ./intent.json --output ./out/project intent create
```

`intent explain` may return a compact subset, but `intent plan` and persisted
`intent-plan.json` must include the full trace.

## 11. Testing Requirements

Required tests:

- unit tests for synthesis trace normalization and deterministic ordering;
- golden tests for:
  - I2C bus synthesis;
  - UART connector/programming synthesis;
  - ISP programming synthesis;
  - unknown supply alias blocking;
  - multi-target ambiguity blocking;
  - external-clock unsupported gap before topology generation;
  - external-clock supported path once block generation exists;
  - regulator headroom warning;
  - value-calculation trace records;
  - rationale report mapping from synthesis trace;
- CLI tests proving `intent plan`, `intent explain`, `intent rationale`, and
  `intent create` preserve existing output compatibility while adding trace
  data.

Normal `go test ./...` must not require KiCad CLI or network access.

## 12. Acceptance Criteria

- `intent plan` emits `synthesis` with deterministic decisions, evidence,
  constraints, calculations, and gaps.
- Supported MCU/interface/power-domain requests produce traceable topology
  decisions and generated workflow requests.
- Ambiguous or unsupported topology fails closed with precise blockers or gaps.
- Component policy includes voltage/current/package/confidence constraints from
  the intent where known.
- Rationale reports include synthesis decisions and known limits.
- Existing intent fixtures remain compatible.
- Full `go test ./...` passes without KiCad or network access.
