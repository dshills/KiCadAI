# Intent Planner And AI Orchestration Specification

## 1. Purpose

KiCadAI can already execute a structured `design create` request: it selects
known circuit blocks, chooses components, writes schematics and PCBs, places and
routes generated boards, runs validation, optionally repairs generated output,
and reports fabrication-candidate evidence. The remaining gap before AI-driven
generation feels natural is the layer above `design create`: turning user intent
into a validated, explainable design request.

This project adds a deterministic intent planner that accepts a higher-level
intent document, resolves it into a `designworkflow.Request`, explains every
assumption and selected design element, and optionally runs the existing design
workflow. The planner should be safe for AI agents to call repeatedly because it
has explicit uncertainty, requirements, assumptions, blockers, and validation
evidence.

The goal is not to add an opaque natural-language generator. The goal is to
create the structured planning substrate that a future LLM, CLI user, or MCP
server can use without bypassing component, block, writer, validation, repair,
and fabrication gates.

## 2. Goals

The intent planner must:

- define a stable intent schema above `designworkflow.Request`;
- translate supported intent into block, component, board, constraint,
  validation, and routing-retry request fields;
- produce a plan artifact before any KiCad files are written;
- explain selected blocks, parts, parameters, board assumptions, nets, endpoint
  assumptions, acceptance target, and validation requirements;
- represent ambiguity as structured clarification or blocker issues;
- reject unsupported or unsafe requirements rather than guessing;
- run existing `design create` only from a validated generated request;
- preserve deterministic output from the same intent, catalog, and seed;
- expose machine-readable evidence for AI feedback loops;
- keep natural-language interpretation out of the core deterministic planner
  until a later layer can map text into this structured schema.

## 3. Non-Goals

This project does not:

- call an LLM or external model;
- add MCP server behavior;
- perform live sourcing, pricing, or availability checks;
- certify manufacturing readiness beyond existing validation and fabrication
  readiness gates;
- support arbitrary unsupported circuit topologies;
- mutate imported user projects;
- bypass component confidence, block readiness, writer correctness, ERC/DRC,
  repair, or fabrication gates;
- replace `design create`; it builds on it.

## 4. Current Baseline

Existing foundations:

- `internal/designworkflow.Request` is the current executable design request.
- `design create` orchestrates block planning, component selection, schematic
  and PCB generation, placement, routing, writer correctness, validation, KiCad
  checks, fabrication preview, and optional repair.
- Circuit blocks expose readiness, rules, ports, required routes, PCB
  realization, verification levels, and known gaps.
- Component intelligence exposes confidence, pinmap, ratings, package,
  manufacturer, MPN, companion, and rejected-candidate evidence.
- Placement/routing emit quality reports and repairable diagnostics.
- Fabrication readiness emits package/readiness evidence and physical-rule
  summaries.

Current gaps:

- users or AI callers must already know exact block IDs and request fields;
- no first-class schema captures high-level design intent such as "breakout",
  "sensor node", power input, interface, target voltage, mounting, or
  validation goal;
- no planner artifact explains why a block/component/constraint was selected;
- unsupported features are only discovered after lower-level workflow stages;
- ambiguity cannot be represented as a clarification requirement;
- there is no single command that validates intent, shows a plan, and optionally
  executes it.

## 5. Command Surface

Add intent planning commands without changing the existing `design create`
contract.

### 5.1 Plan Only

```sh
kicadai --json intent plan --request intent.json --output ./out/plan
```

Behavior:

- read and strictly validate the intent schema;
- build a deterministic plan and generated `designworkflow.Request`;
- write plan artifacts only when `--output` is provided;
- do not create KiCad project files;
- return blockers for unsupported or ambiguous intent.

### 5.2 Explain

```sh
kicadai --json intent explain --request intent.json
```

Behavior:

- return the same planning result, optimized for inspection;
- include rationale, assumptions, selected blocks, rejected alternatives,
  validation requirements, and known gaps;
- never write files unless an explicit output path is provided.

### 5.3 Create

```sh
kicadai --json intent create --request intent.json --output ./out/project --overwrite
```

Behavior:

- run intent planning;
- fail before project writes if planning has blocking issues;
- pass the generated `designworkflow.Request` into `design create`;
- return both intent planning evidence and design workflow evidence;
- write `.kicadai/intent-plan.json` and `.kicadai/generated-request.json` in
  the output project when creation succeeds or when partial artifacts are
  explicitly requested.

## 6. Intent Request Schema

The first schema should be JSON and strict by default:

```json
{
  "version": "0.1.0",
  "name": "environment_sensor_breakout",
  "summary": "I need a USB-C powered I2C environmental sensor breakout",
  "kind": "breakout",
  "acceptance": "connectivity",
  "board": {
    "width_mm": 50,
    "height_mm": 30,
    "layers": 2,
    "mounting_holes": "optional"
  },
  "power": {
    "inputs": [{"kind": "usb_c", "voltage": "5V"}],
    "rails": [{"name": "VCC", "voltage": "3.3V", "current_ma": 150}]
  },
  "interfaces": [
    {"kind": "i2c", "voltage": "3.3V", "connector": "pin_header"}
  ],
  "functions": [
    {"kind": "sensor", "family": "i2c_sensor", "quantity": 1}
  ],
  "protection": {
    "esd": "preferred",
    "reverse_polarity": "optional"
  },
  "manufacturing": {
    "profile": "generic_assembly",
    "fabrication_candidate": false
  },
  "constraints": {
    "prefer_smd": true,
    "allow_placeholders": false
  }
}
```

### 6.1 Top-Level Fields

- `version`: schema version.
- `name`: generated project name.
- `summary`: human-readable intent summary.
- `kind`: one of the initially supported categories:
  - `breakout`;
  - `power_module`;
  - `sensor_node`;
  - `mcu_minimal`;
  - `amplifier`;
  - `custom_structured`.
- `acceptance`: maps to design workflow acceptance:
  - `draft`;
  - `structural`;
  - `connectivity`;
  - `erc-drc`;
  - `fabrication-candidate`.
- `board`: physical constraints.
- `power`: input and rail requirements.
- `interfaces`: external electrical interfaces.
- `functions`: desired functional blocks.
- `protection`: safety/protection preferences.
- `manufacturing`: profile and fabrication policy.
- `constraints`: planning preferences and hard limits.

### 6.2 Requirement Strength

Fields that represent optional design features should support requirement
strength:

- `required`: missing support blocks planning.
- `preferred`: planner may continue but emits a warning and rationale if omitted.
- `optional`: planner may omit without warning.
- `forbidden`: planner must not select matching blocks/components.

String shorthands may be accepted for common boolean-like requirements, but the
normalized plan should always emit explicit strength.

### 6.3 Supported Initial Intent Mappings

Initial planner mappings should cover:

- LED indicator:
  - function `indicator`;
  - block `led_indicator`;
  - optional connector breakout.
- Connector breakout:
  - interface `gpio`, `power`, or generic `connector`;
  - block `connector_breakout`.
- USB-C power input:
  - power input `usb_c`;
  - block `usb_c_power`;
  - optional ESD protection.
- Voltage regulator:
  - rail conversion from input voltage to lower output rail;
  - block `voltage_regulator`.
- I2C sensor breakout:
  - function `sensor` with family `i2c_sensor`;
  - block `i2c_sensor`;
  - optional connector breakout and regulator.
- MCU minimal:
  - function `mcu`;
  - block `mcu_minimal`;
  - optional crystal/canned oscillator and reset/programming block.
- Op-amp gain stage:
  - function `amplifier`;
  - block `opamp_gain_stage`;
  - gain parameter where supported.
- Protection:
  - ESD block for edge-facing signal/power interfaces;
  - reverse-polarity block for DC power input when requested.

Unsupported intent must produce issues that name the unsupported requirement and
suggest available alternatives.

## 7. Plan Output Schema

The planner result should be deterministic JSON:

```json
{
  "schema": "kicadai.intent.plan.v1",
  "status": "ready",
  "score": 85,
  "intent": {"name": "environment_sensor_breakout", "kind": "breakout"},
  "generated_request": {},
  "requirements": [],
  "selected_blocks": [],
  "selected_components": [],
  "connections": [],
  "assumptions": [],
  "clarifications": [],
  "known_gaps": [],
  "issues": [],
  "artifacts": []
}
```

### 7.1 Status

- `ready`: generated request has no blocking planning issues.
- `needs_clarification`: one or more required fields are ambiguous and no safe
  default exists.
- `blocked`: unsupported or unsafe requirements prevent request generation.
- `partial`: a generated request exists, but preferred requirements were omitted
  or downgraded.

### 7.2 Requirements

Each requirement record should include:

- stable ID;
- source path in the input intent;
- normalized type;
- strength;
- interpreted value;
- selected implementation or omission reason;
- evidence references.

### 7.3 Selected Blocks

Each selected block record should include:

- intent requirement IDs satisfied;
- block instance ID;
- block ID;
- parameters;
- readiness level;
- verification level;
- required routes;
- known gaps;
- rationale.

### 7.4 Selected Components

The planner should cite requested component constraints and planned component
policy overrides. Concrete component choice may remain in `design create`, but
the plan must explain the intended family, package preference, minimum
confidence, required ratings, and whether placeholders are allowed.

### 7.5 Assumptions And Clarifications

Assumptions are defaults the planner can safely choose, such as a two-layer board
when no layer count is provided. Clarifications are required missing decisions
that should stop the planner, such as ambiguous rail voltage, impossible board
size, or unsupported function family.

## 8. Planner Rules

### 8.1 Deterministic Normalization

The planner must:

- normalize project names;
- sort generated blocks, connections, assumptions, issues, and artifacts
  deterministically;
- assign stable block IDs from requirement IDs, not map iteration order;
- preserve explicit user-provided IDs where valid.

### 8.2 Safe Defaults

Allowed defaults:

- two-layer board when not specified;
- structural acceptance when not specified;
- component acceptance derived from requested design acceptance;
- routing retry disabled unless requested;
- KiCad checks optional unless acceptance requires ERC/DRC or fabrication.

Blocked defaults:

- unknown voltage/current;
- unknown connector pinout for named external interface;
- placeholder active component for connectivity or higher acceptance unless
  explicitly allowed;
- fabrication-candidate without known board size and concrete part policy.

### 8.3 Validation Coupling

The generated `designworkflow.Request` must set:

- `Validation.Acceptance` from intent acceptance;
- `RequireERC` and `RequireDRC` for `erc-drc` and `fabrication-candidate` when
  requested by policy;
- `StrictUnrouted` for connectivity or higher unless explicitly relaxed;
- `StrictZones` when zone-backed behavior is required;
- fabrication manufacturer profile through create/export orchestration where
  supported.

### 8.4 Component Policy Coupling

The planner must derive component policy from intent:

- `allow_placeholders: false` should require component confidence appropriate
  for the requested acceptance;
- package preferences should map into component policy package preferences;
- voltage/current/tolerance requirements should map to required ratings;
- fabrication candidate should prefer concrete manufacturer/MPN records.

## 9. Artifacts

When an output directory is provided, plan commands should write:

- `intent-plan.json`;
- `generated-request.json`;
- `intent-summary.md` if a human-readable summary is requested;
- during `intent create`, `.kicadai/intent-plan.json` and
  `.kicadai/generated-request.json` inside the generated project.

Artifacts must use project-relative paths in report output.

## 10. Feedback And Repair Integration

The first implementation should not invent a new repair system. It should:

- convert planner issues to `reports.Issue`;
- use issue paths like `intent.power.rails[0].voltage` or
  `intent.functions[1].family`;
- attach suggestions that an AI can turn into revised intent;
- preserve generated request and plan artifacts so failed `design create` runs
  can be diagnosed;
- group downstream workflow issues under the created request and selected block
  rationale.

Future repair loops can revise the intent document, regenerate the
`designworkflow.Request`, and rerun the existing validation/repair pipeline.

## 11. Testing Requirements

Default tests must be hermetic and should cover:

- strict intent JSON decode and unknown-field rejection;
- normalization and deterministic output;
- supported mappings for LED, connector, regulator, USB-C power, I2C sensor,
  MCU minimal, op-amp gain stage, ESD, and reverse-polarity protection;
- ambiguity/clarification cases;
- unsupported function cases;
- component policy derivation;
- generated `designworkflow.Request` validation;
- plan artifact writing;
- `intent create` orchestration with a small generated fixture;
- JSON golden snapshots for representative plan outputs.

Optional KiCad CLI tests may be added only behind the existing opt-in
environment patterns.

## 12. Acceptance Criteria

This project is complete when:

- `kicadai --json intent plan --request intent.json` returns a deterministic
  plan and generated design request;
- supported seed intents can create valid `designworkflow.Request` values;
- unsupported or ambiguous intent blocks with actionable issues;
- `intent create` can run `design create` from a generated request;
- plan artifacts are written deterministically and referenced in CLI output;
- AI callers can inspect selected blocks, assumptions, known gaps, and
  validation target before files are written;
- existing `design create` behavior remains backward compatible;
- `go test ./...` passes.

