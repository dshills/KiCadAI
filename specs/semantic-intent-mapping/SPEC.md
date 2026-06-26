# Semantic Intent Mapping Specification

Date: 2026-06-26

## 1. Purpose

KiCadAI now has a structured intent planner that can turn explicit JSON intent
into a `design create` request. The next gap is semantic depth: the planner
must understand enough electrical intent to connect supported MCU, interface,
clock, programming, and power-domain requirements without guessing.

This project expands the deterministic structured intent planner. It does not
add natural-language parsing. The expected caller is still an AI, CLI user, or
integration that provides structured JSON. The output must remain inspectable,
testable, and conservative.

## 2. Problem

The current intent planner can select blocks and basic connections, but several
important requirements still become known gaps:

- MCU I2C wiring is deferred because the MCU block does not expose distinct
  SDA/SCL-capable ports.
- MCU clock wiring is deferred because clock blocks cannot target clock-capable
  MCU ports.
- MCU programming wiring is deferred because programming/debug blocks cannot
  target exposed MCU programming ports.
- Multiple MCUs make support block targeting ambiguous.
- Some supply-voltage requirements depend on block defaults and partial block
  metadata instead of explicit voltage-domain evidence.
- Intent plan artifacts describe selected blocks and connections, but not a
  reusable semantic resolution trace that explains why a target, bus, or supply
  domain was selected.

These gaps are appropriate for the first planner foundation, but they prevent
useful AI generation for common boards such as "MCU plus I2C sensor", "MCU with
external crystal", and "MCU with ISP header".

## 3. Goals

1. Add explicit semantic target metadata to structured intent requests.
2. Resolve single- and multi-target requirements deterministically.
3. Expose MCU functional ports for supported built-in MCU templates.
4. Connect supported MCU I2C, clock, and programming requirements when metadata
   proves compatibility.
5. Strengthen voltage-domain and supply-source selection evidence.
6. Emit planner rationale, assumptions, known gaps, and blocking issues in a
   form useful to AI callers.
7. Add golden intent fixtures that prove supported semantic designs generate
   stable `design create` requests and unsupported designs fail closed.

## 4. Non-Goals

- Free-form natural-language parsing.
- LLM calls from the CLI.
- Arbitrary MCU family support.
- Full alternate-function extraction from KiCad libraries.
- PCB layout optimization beyond the existing placement/routing engines.
- Production fabrication-readiness claims for newly connected semantic paths
  unless existing downstream gates prove them.
- Imported project mutation.

## 5. Existing Foundations

The implementation must build on existing code:

- `internal/intentplanner`
  - `Request`, `FunctionIntent`, `InterfaceIntent`, `PowerIntent`
  - `PlanResult`, `RequirementRecord`, `SelectedBlockRecord`,
    `ConnectionRecord`, `PlanNote`
  - current mapping functions in `map.go`
- `internal/blocks`
  - `BlockDefinition`, `BlockPort`, `BlockParameter`
  - `mcu_minimal`, `i2c_sensor`, `crystal_oscillator`,
    `canned_oscillator`, `reset_programming_header`
  - block composition voltage-domain checks
- `internal/designworkflow`
  - generated request model consumed by `design create`
- Existing golden intent fixtures under `examples/intent/`

## 6. Request Model Extensions

The structured intent request should gain optional semantic targeting without
breaking existing fixtures.

### 6.1 Target References

Add a reusable target-reference shape:

```go
type TargetRef struct {
    ID   string `json:"id,omitempty"`
    Role string `json:"role,omitempty"`
}
```

Rules:

- `id` targets a planner instance or caller-declared semantic ID.
- `role` targets a semantic role such as `mcu`, `sensor`, `clock`,
  `programming`, `power_input`, or `connector`.
- Empty target means "planner may infer if unambiguous".
- If more than one compatible target exists and no target is provided, the plan
  must block or request clarification depending on requirement strength.

### 6.2 Function Intent Semantics

Extend `FunctionIntent` with optional fields:

```go
Target    TargetRef `json:"target,omitempty"`
Interface string    `json:"interface,omitempty"`
Bus       string    `json:"bus,omitempty"`
Supply    string    `json:"supply,omitempty"`
```

Allowed semantics:

- `target`: target MCU or support block.
- `interface`: functional interface requirement, initially `i2c`, `gpio`,
  `uart`, `spi`, `clock`, or `programming`.
- `bus`: stable bus name, initially most useful for `i2c`.
- `supply`: requested supply rail name or voltage-domain alias.

The existing `params` map remains supported for block-specific values. New
typed fields should be preferred for planner-level decisions.

### 6.3 Interface Intent Semantics

Extend `InterfaceIntent` with optional fields:

```go
Target TargetRef `json:"target,omitempty"`
Bus    string    `json:"bus,omitempty"`
```

Rules:

- `bus` groups connectors, sensors, and MCU ports into the same logical bus.
- If no `bus` is supplied for a single I2C group, default to logical bus alias
  `i2c1` and record an assumption. This alias is planner-local evidence, not a
  claim that the MCU peripheral is physically named `I2C1`; physical peripheral
  selection remains a later block/pin-mapping concern.
- Multiple I2C buses require explicit `bus` names or deterministic indexed
  defaults with clear evidence.

### 6.4 Power Intent Semantics

Extend `PowerRailIntent` with optional fields:

```go
SuppliedTargets []TargetRef `json:"supplied_targets,omitempty"`
Supplies        []TargetRef `json:"supplies,omitempty"` // legacy alias
Alias           string      `json:"alias,omitempty"`
```

Rules:

- `alias` names a voltage domain such as `vcc`, `3v3`, `vbus`, or `avcc`.
- `supplied_targets` lists load targets powered by this rail. It does not
  bypass net modeling; the planner still emits the concrete KiCad net aliases
  that connect the rail source to each target.
- `supplies` is accepted as a backward-compatible JSON alias and normalizes into
  `supplied_targets`.
- If no supplied targets are provided, compatible power targets may still be
  inferred from voltage metadata, but the plan must emit evidence.

## 7. Block Semantic Metadata

The planner needs a small internal semantic view of block instances. This can
be implemented as intent-planner-side metadata first, then promoted into block
definitions later if useful.

### 7.1 Semantic Ports

For each selected block instance, expose:

- instance ID;
- block ID;
- semantic role;
- port name;
- function role;
- voltage domain or required supply voltage;
- direction;
- bus capability;
- target compatibility.

Initial required roles:

- MCU:
  - `power.vcc`
  - `power.gnd`
  - `mcu.reset`
  - `mcu.aref`
  - `mcu.gpio`
  - `mcu.i2c.sda`
  - `mcu.i2c.scl`
  - `mcu.spi.mosi`
  - `mcu.spi.miso`
  - `mcu.spi.sck`
  - `mcu.uart.tx`
  - `mcu.uart.rx`
  - `mcu.clock.xtal1`
  - `mcu.clock.xtal2`
- I2C sensor:
  - `i2c.sda`
  - `i2c.scl`
  - `power.vcc`
  - `power.gnd`
- I2C connector:
  - `i2c.sda`
  - `i2c.scl`
  - `power.vcc`
  - `power.gnd`
- Clock blocks:
  - crystal: paired passive clock terminals
  - canned oscillator: clock output plus power
- Reset/programming block:
  - reset line
  - ISP or UART programming pins where mode supports them
  - power/gnd reference

`power.vcc` and `power.gnd` are normalized semantic roles, not literal pin
names. `power.vcc` covers positive supply pins such as VCC, VDD, VIN, VBUS, or
a named positive voltage domain; `power.gnd` covers ground/reference return
pins such as GND or VSS.

### 7.2 MCU Minimal Template

The first supported MCU semantic template is the existing ATmega328P-A block.
The planner may use its fixed role map, but must keep the current warning that
alternate-function metadata is not resolver-backed.

The MCU block should expose planner-visible ports for the supported template so
that:

- I2C sensor/connector SDA/SCL can connect to MCU SDA/SCL when requested and
  supported by the template.
- ISP programming can connect MOSI/MISO/SCK/RESET/VCC/GND.
- UART programming can connect UART_TX/UART_RX/VCC/GND when requested.
- External clock support remains blocked until the underlying `mcu_minimal`
  block supports non-internal clock mode or a composition path can prove safe
  schematic semantics.

The spec intentionally distinguishes "metadata can identify target pins" from
"block can instantiate that topology". If the block itself blocks external
clock mode, the planner must report that as a known unsupported topology, not
emit invalid connections.

## 8. Semantic Resolution

Add a planner resolution pass after block selection and before connection
generation.

### 8.1 Instance Identity

Every selected instance should have:

- generated instance ID;
- requirement IDs;
- semantic role;
- optional caller-visible target ID from request params or typed fields;
- block ID;
- declared or inferred supply voltage;
- exported semantic ports.

### 8.2 Target Resolution

Resolution algorithm:

1. Build candidate instances by role and block capability.
2. Apply explicit `TargetRef.ID` if present.
3. Apply `TargetRef.Role` if present.
4. Apply bus or interface constraints.
5. If exactly one candidate remains, select it.
6. If zero candidates remain:
   - required requirement: blocking issue;
   - preferred/optional requirement: known gap or omitted requirement.
7. If multiple candidates remain:
   - required requirement: clarification or blocking issue;
   - preferred/optional requirement: non-blocking `PlanNote` with the
     candidate IDs; no connection is emitted until the ambiguity is resolved,
     and the plan status becomes partial so AI callers cannot miss the skipped
     optional behavior.

If both `TargetRef.ID` and `TargetRef.Role` are present, they are treated as a
logical AND. A mismatch between the explicit ID and role leaves zero candidates
and produces the same fail-closed issue or note as any other unresolved target.

### 8.3 Bus Resolution

I2C bus resolution:

- group all I2C functions and interfaces by `bus`;
- default to `i2c1` for a single unnamed bus with an assumption;
- use the default only when it does not collide with an explicitly declared
  different bus; otherwise choose the next available deterministic logical
  alias and record the assumption;
- connect all compatible `SDA` endpoints on the same net;
- connect all compatible `SCL` endpoints on the same net;
- preserve existing I2C address-collision checks in block composition;
- emit `ConnectionRecord` entries with bus names and semantic rationale.

If an MCU exists and exposes I2C ports, sensors and I2C connectors should
connect to the MCU bus when compatible. If no MCU exists, the current
sensor-to-connector behavior remains valid.

I2C endpoints must also pass the voltage-domain compatibility checks in Section
8.6 before the planner emits SDA/SCL connections for the logical bus. Missing or
conflicting voltage metadata should produce a blocking issue for required bus
members and a visible partial-plan gap for optional members.

### 8.4 Programming Resolution

Programming resolution:

- target one MCU, inferred only when exactly one MCU exists;
- mode is read from typed intent if added later, or from `params` initially;
- supported modes: `isp`, `uart`, `none` where the block already supports
  them;
- emit reset/power/reference connections only when the selected mode exposes
  matching MCU and programming block ports;
- block unsupported modes with actionable suggestions.

### 8.5 Clock Resolution

Clock resolution:

- target one MCU, inferred only when exactly one MCU exists;
- support metadata-level target detection for crystal/canned oscillator;
- do not wire external clock to `mcu_minimal` unless the selected MCU block
  params and block instantiation support non-internal clock mode;
- report the current unsupported topology as a precise limitation:
  "target MCU ports are known, but generated MCU block only supports internal
  clock mode." If the request requires an external clock for a required target,
  this is a blocking issue; if the target is optional, it is a non-blocking
  known gap and the planner must not emit clock connections.

### 8.6 Voltage-Domain Resolution

Voltage-domain resolution:

- normalize voltage literals using existing parser behavior.
- assign each power input and rail a domain alias.
- assign each target instance a required voltage from:
  1. typed `Supply`/rail binding;
  2. explicit block params such as `supply_voltage`;
  3. block parameter defaults;
  4. block port voltage metadata.
- connect compatible targets to matching sources.
- a source is compatible only when its normalized numeric voltage matches the
  target nominal voltage within the planner tolerance, or falls within explicit
  operating-range metadata for that target. The default planner tolerance is an
  absolute `0.01 V` equality tolerance to absorb parser/formatting noise, not an
  electrical operating-margin assumption.
- if multiple compatible sources remain after compatibility checks, prefer an
  explicit supply alias; otherwise sort candidates by stable source ID and
  choose the first candidate, emitting an assumption.
- if no source has explicit compatibility evidence, fail closed with a blocking
  issue for required targets or a visible partial-plan gap for optional
  targets.
- if no compatible source exists for a required target, emit a blocking issue.
- if metadata is insufficient, emit a known gap with a path to the target.

## 9. Plan Output

The plan output must remain stable and AI-readable.

Add semantic evidence without breaking existing consumers:

- Use `RequirementRecord.Evidence` for bus, target, supply, and compatibility
  decisions.
- Use `SelectedBlockRecord.KnownGaps` for unsupported topologies tied to the
  selected block.
- Use `ConnectionRecord.Rationale` for semantic route explanations.
- Add `PlanNote` IDs under stable namespaces:
  - `semantic.target.*`
  - `semantic.bus.*`
  - `semantic.power.*`
  - `mcu.i2c.*`
  - `mcu.programming.*`
  - `mcu.clock.*`

If a new `SemanticEvidence` field is added to `PlanResult`, it must be
optional and versioned enough to avoid breaking older fixtures. Prefer using
existing fields first unless the implementation becomes awkward.

## 10. CLI Behavior

Existing commands remain:

- `intent plan`
- `intent explain`
- `intent create`

Behavior changes:

- supported semantic intents produce fewer known gaps and more concrete
  connections;
- unsupported semantic intents produce more precise blockers or known gaps;
- `intent explain` should summarize target/bus/supply decisions when present;
- `intent create` must still refuse blocked or clarification-required plans.

No new CLI command is required.

## 11. Examples

Add or update intent fixtures:

- `examples/intent/mcu_i2c_sensor.json`
  - one MCU;
  - one I2C sensor;
  - one I2C connector;
  - named bus `i2c1`;
  - 3.3 V or 5 V supply domain.
- `examples/intent/mcu_isp_programmer.json`
  - one MCU;
  - ISP programming requirement;
  - expected reset/MOSI/MISO/SCK/VCC/GND semantic evidence.
- `examples/intent/mcu_external_clock_blocked.json`
  - one MCU;
  - crystal or oscillator requirement;
  - precise known gap or blocked issue because generated MCU external clock
    topology is not supported yet.
- `examples/intent/multi_mcu_ambiguous_support.json`
  - two MCUs;
  - one support requirement without target;
  - clarification/blocking outcome.
- `examples/intent/voltage_domain_sensor.json`
  - power input, regulator rail, sensor, and connector;
  - expected supply-domain evidence.

## 12. Testing

Required tests:

- request normalization/validation for new typed fields;
- target resolution with explicit ID, explicit role, inferred single target,
  zero targets, and ambiguous targets;
- I2C bus resolution with MCU plus sensor plus connector;
- multiple I2C bus behavior;
- programming support target resolution;
- external clock unsupported topology reporting;
- voltage-domain matching and incompatible source blocking;
- golden fixture parse and plan stability;
- CLI `intent explain` includes semantic decisions;
- `intent create` blocks unsafe plans and runs supported plans through the
  existing workflow.

Tests must not require KiCad or network access.

## 13. Acceptance Criteria

This project is complete when:

- structured intent can express target, bus, and supply semantics;
- a supported MCU plus I2C sensor/connector intent creates concrete SDA/SCL
  connections instead of the existing `mcu.i2c.pin_assignment` known gap;
- a supported MCU programming intent resolves to the correct MCU support target
  or blocks with precise evidence;
- external MCU clock intent reports the exact remaining topology limitation
  without emitting unsafe connections;
- voltage-domain decisions are visible in plan evidence;
- multi-target ambiguity is never silently guessed;
- all new fixtures pass deterministic tests;
- `go test ./...` passes.

## 14. Risks

- The existing MCU block has fixed, non-resolver-backed role metadata. This is
  acceptable for the seed ATmega328P-A template only if the plan reports that
  limitation.
- Connecting semantic support blocks can expose downstream block limitations.
  The planner must stop rather than generate invalid block params.
- Adding new request fields can create fixture churn. Existing fixtures must
  remain valid and default behavior must stay compatible.
- Voltage-domain evidence can become misleading if string comparison is used.
  Voltage literals must be normalized numerically where possible.

## 15. Documentation

Update documentation after implementation:

- README intent planner section;
- `specs/ROADMAP.md` Priority 9 current foundation and remaining work;
- example comments or fixture README if one exists.
