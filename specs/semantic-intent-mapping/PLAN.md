# Semantic Intent Mapping Implementation Plan

Date: 2026-06-26

## Overview

Implement the next roadmap item: richer deterministic semantic mapping inside
the structured intent planner. The work should be committed phase by phase, with
`go test ./...` and `prism review staged` before each commit.

The work intentionally stays below natural-language parsing. It expands the
current JSON intent schema, planner resolution, plan evidence, and golden
fixtures so AI callers can request MCU buses, programming support, and supply
domains without hardcoding block IDs.

## Phase 1: Request Schema And Validation

Add typed semantic fields to intent requests while preserving compatibility.

Tasks:

1. Add `TargetRef` to `internal/intentplanner/model.go`.
2. Extend `FunctionIntent` with:
   - `target`;
   - `interface`;
   - `bus`;
   - `supply`.
3. Extend `InterfaceIntent` with:
   - `target`;
   - `bus`.
4. Extend `PowerRailIntent` with:
   - `supplied_targets`;
   - legacy `supplies` alias;
   - `alias`.
5. Normalize target IDs, roles, bus names, interface names, and supply aliases
   with existing token normalization where appropriate.
6. Validate unsupported interface names conservatively.
7. Validate malformed target references:
   - target `id` and `role` are optional, but if present must normalize to a
     non-empty token;
   - no unknown JSON fields because strict decoding already applies.
8. Add model tests for normalization, strict decoding, and backward
   compatibility with existing fixtures.

Acceptance:

- Existing intent fixtures decode unchanged.
- New typed fields round-trip through strict decode.
- Invalid semantic fields produce deterministic `reports.Issue` paths.

Commit message:

`Add semantic fields to intent requests`

## Phase 2: Semantic Instance Index

Create a planner-side semantic index for selected block instances.

Tasks:

1. Add internal types in `internal/intentplanner`, likely in a new
   `semantics.go`:
   - `semanticInstance`;
   - `semanticPort`;
   - `semanticIndex`;
   - target resolution result structs.
2. Populate semantic instances when `addBlock` succeeds:
   - instance ID;
   - block ID;
   - role/prefix;
   - requirement IDs;
   - params;
   - supply voltage from params/defaults/port metadata.
3. Add semantic-port extraction for current built-in blocks:
   - connector breakout pins;
   - I2C sensor;
   - MCU minimal ATmega328P-A seed;
   - reset/programming header;
   - crystal oscillator;
   - canned oscillator;
   - power blocks and regulators.
4. Keep semantic extraction private to the planner unless a cleaner shared
   block API naturally emerges.
5. Add unit tests for semantic index creation using selected blocks.

Acceptance:

- Planner tests can query instance capabilities by role/function.
- Semantic metadata is deterministic and sorted where exposed.
- No generated workflow behavior changes yet.

Commit message:

`Add intent semantic instance index`

## Phase 3: Target And Ambiguity Resolution

Implement deterministic target resolution using the semantic index.

Tasks:

1. Add resolver helpers:
   - by explicit `TargetRef.ID`;
   - by explicit `TargetRef.Role`;
   - by functional capability;
   - by bus name;
   - by supply alias/voltage where relevant.
2. Add issue/note helpers for:
   - zero candidates;
   - multiple candidates;
   - unsupported target role;
   - explicit target missing;
   - target lacks required capability.
3. Replace the current broad `supportTargetingIsAmbiguous` behavior with
   semantic target resolution.
4. Preserve fail-closed behavior:
   - required ambiguous target blocks or clarifies;
   - preferred/optional ambiguous target becomes an omitted known gap.
5. Add tests for single target inference, explicit target success, explicit
   target failure, and multi-MCU ambiguity.

Acceptance:

- Multiple MCUs no longer block unrelated support requirements.
- Ambiguous support requirements do not silently choose a target.
- Plan notes include stable target-resolution IDs.

Commit message:

`Resolve semantic intent targets`

## Phase 4: I2C Bus Mapping

Use semantic bus metadata to connect MCU, sensors, and connectors.

Tasks:

1. Group I2C requirements by `bus`.
2. Default a single unnamed I2C group to `i2c1` and record an assumption.
   This is a logical planner bus alias; it must map through the selected
   target's semantic SDA/SCL ports and must not be treated as an MCU-native
   peripheral name. The default may be used only when it does not collide with
   an explicitly declared different bus; otherwise the planner must choose the
   next deterministic logical alias and record that assumption.
3. Connect compatible endpoints:
   - MCU SDA to sensor SDA;
   - MCU SCL to sensor SCL;
   - connector SDA/SCL to the same bus nets when present.
4. Preserve current sensor-to-connector behavior when no MCU is present.
5. Use stable net aliases:
   - `I2C1_SDA`, `I2C1_SCL` for named/defaulted bus;
   - deterministic variants for multiple buses.
6. Add connection evidence to requirements and connection rationale.
7. Add tests:
   - MCU + I2C sensor + I2C connector;
   - no MCU fallback;
   - two buses;
   - missing MCU capability.

Acceptance:

- Supported MCU/I2C intents no longer emit `mcu.i2c.pin_assignment`.
- Generated request has explicit connection operations for SDA/SCL paths.
- Bus net aliases are stable.

Commit message:

`Map semantic I2C intent`

## Phase 5: Programming Support Mapping

Resolve reset/programming requirements to supported MCU ports.

Tasks:

1. Interpret reset/programming mode from existing `params` first:
   - `programming_mode`;
   - `programming_header`;
   - or block-compatible names already used by requests.
2. Target one MCU via target resolution.
3. For `isp`, require MOSI/MISO/SCK/RESET/VCC/GND semantic ports.
4. For `uart`, require UART_TX/UART_RX/VCC/GND semantic ports.
5. Add connections only when both the MCU and support block expose matching
   ports.
6. Report unsupported modes and missing ports with actionable suggestions.
7. Add tests for ISP success, UART success where currently supported, missing
   target, ambiguous target, and unsupported mode.

Acceptance:

- Supported programming intent produces concrete connection records.
- Unsupported programming intent blocks with precise paths.
- Existing MCU minimal behavior remains compatible.

Commit message:

`Map semantic MCU programming intent`

## Phase 6: Clock Support Mapping

Resolve clock requirements honestly without generating unsafe external clock
topologies.

Tasks:

1. Target one MCU through the semantic resolver.
2. Detect crystal/canned oscillator support block capabilities.
3. Detect MCU clock-capable semantic ports from the seed metadata.
4. If the selected MCU block still requires `clock_mode=internal`, emit a
   precise severity-scoped result:
   - target ports are known;
   - generated MCU block external clock topology is not supported.
   - blocking `Issue` when the external clock is required;
   - non-blocking known gap/`Note` when the clock target is optional.
5. Do not emit clock connections unless block instantiation supports them.
6. Add tests for:
   - external clock known-gap behavior;
   - ambiguous target behavior;
   - no target behavior;
   - future-compatible success path if block support is later added.

Acceptance:

- The previous generic `mcu.clock.pin_assignment` gap is replaced by a specific
  topology limitation.
- No invalid external-clock connection is emitted.

Commit message:

`Report semantic MCU clock limitations`

## Phase 7: Voltage-Domain Evidence

Strengthen supply-domain resolution and plan evidence.

Tasks:

1. Add normalized voltage-domain aliases for inputs and rails.
2. Bind `PowerRailIntent.SuppliedTargets` to semantic targets, accepting legacy
   `Supplies` as an input alias.
3. Resolve target supply requirements using:
   - typed `Supply`;
   - rail aliases;
   - explicit `supply_voltage` params;
   - block defaults;
   - block port voltage metadata.
4. Add evidence to requirement records:
   - selected source;
   - selected rail;
   - compatible voltage;
   - assumption when multiple compatible sources exist.
5. Convert vague missing-supply known gaps into target-specific issues or
   notes.
6. Add tests for compatible rail, incompatible rail, ambiguous source,
   explicit rail supply, and missing metadata.

Acceptance:

- Plans explain why each powered target is connected to a source.
- Incompatible required supplies block instead of producing partial wiring.
- Current existing fixtures remain stable except for improved evidence.

Commit message:

`Add intent voltage-domain evidence`

## Phase 8: Golden Fixtures And CLI Coverage

Lock the new semantic planner behavior with examples and CLI tests.

Tasks:

1. Add fixtures in `examples/intent/`:
   - `mcu_i2c_sensor.json`;
   - `mcu_isp_programmer.json`;
   - `mcu_external_clock_blocked.json`;
   - `multi_mcu_ambiguous_support.json`;
   - `voltage_domain_sensor.json`.
2. Extend `internal/intentplanner/golden_test.go`:
   - supported fixtures must not have blocking issues;
   - intentionally blocked fixtures must match expected issue or gap IDs.
3. Extend CLI tests:
   - `intent explain` includes target/bus/supply semantics;
   - `intent create` refuses blocked semantic plans;
   - supported semantic plan can write planner artifacts.
4. Keep tests KiCad-independent.

Acceptance:

- Golden fixtures are stable and readable.
- CLI output proves semantic decisions are visible to AI callers.

Commit message:

`Add semantic intent golden coverage`

## Phase 9: Documentation And Roadmap

Update user-facing and planning docs.

Tasks:

1. Update README intent planner section:
   - target/bus/supply fields;
   - semantic I2C/programming support;
   - external clock limitation;
   - examples.
2. Update `specs/ROADMAP.md` Priority 9:
   - move semantic mapping items into current foundation as implemented;
   - leave natural-language adapter and broader component coverage as remaining
     work.
3. Mention that external clock generation is still blocked by the MCU block
   topology if not implemented.

Acceptance:

- Documentation matches implemented CLI and request behavior.
- Roadmap clearly identifies the next post-semantic item.

Commit message:

`Document semantic intent mapping`

## Cross-Phase Review Requirements

For each phase:

1. Run focused tests for touched packages during development.
2. Run `go test ./...` before staging final phase changes.
3. Stage only the files for that phase.
4. Run `prism review staged`.
5. Fix real findings.
6. Commit before moving to the next phase.

## Expected Final State

After all phases:

- structured intent requests can express semantic targets, buses, and supplies;
- MCU/I2C plans produce concrete semantic connections;
- programming support is target-aware;
- external clock intent reports a precise limitation instead of a vague missing
  pin-assignment gap;
- voltage-domain choices are visible in plan evidence;
- ambiguous multi-target requests fail closed;
- README and roadmap describe the new planner capability;
- `go test ./...` passes.
