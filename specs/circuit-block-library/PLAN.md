# Circuit Block Library Implementation Plan

## 1. Objective

Implement the circuit block library described in `SPEC.md` in small,
reviewable phases. The end state is a reusable, resolver-backed block system
that can instantiate verified or explicitly warned schematic/PCB fragments for:

- LED indicator
- voltage regulator
- MCU minimal system
- USB-C power input
- I2C sensor
- op-amp gain stage
- connector breakout

The implementation must build on the existing transaction system, design API,
library resolver, pinmap validation, inspection/evaluation reports, examples,
and round-trip validation rather than creating a separate file-generation path.

## 2. Implementation Rules

- Keep each phase independently useful and commit-sized.
- Blocks must emit existing transaction/design operations wherever possible.
- Use resolver-backed KiCad symbol and footprint IDs.
- Never mark heuristic or inferred mappings as verified.
- Return structured `reports.Issue` diagnostics instead of panics.
- CLI output must use `reports.Result`.
- Generated operations must be deterministic for the same request.
- Do not write into external KiCad library repositories.
- Default tests must not require external KiCad roots or KiCad GUI.
- Add opt-in integration or round-trip tests only when they are skipped by
  default.
- Run `gofmt` and `GOCACHE=/tmp/kicadai-gocache go test ./...` before each
  commit.
- Run `prism review staged` before each phase commit and address concrete
  correctness findings.

## 3. Phase 1: Block Package Skeleton and Core Model

### Goal

Create the block library package and data model without implementing real
blocks yet.

### Work

- Add `internal/blocks`.
- Define:
  - `BlockDefinition`;
  - `BlockSummary`;
  - `BlockParameter`;
  - `BlockParameterType`;
  - `BlockPort`;
  - `LibraryRequirement`;
  - `BlockComponent`;
  - `BlockNet`;
  - `SchematicHint`;
  - `PCBHint`;
  - `BlockValidationRule`;
  - `VerificationRecord`;
  - `VerificationLevel`;
  - `BlockRequest`;
  - `BlockInstance`;
  - `BlockOutput`.
- Add deterministic JSON tags for all exported report-facing fields.
- Add helpers for:
  - stable block ID validation;
  - parameter lookup;
  - default parameter application;
  - parameter type validation;
  - issue construction.
- Keep the model independent from CLI concerns.

### Tests

- JSON shape for a minimal block definition.
- Parameter defaults are applied deterministically.
- Invalid block IDs produce structured issues.
- Invalid parameter types produce structured issues.
- Verification levels marshal as stable strings.

### Acceptance Criteria

- `internal/blocks` compiles.
- No CLI behavior changes yet.
- Normal tests pass without external roots.

### Commit Message

```text
Add circuit block library model
```

## 4. Phase 2: Registry and Built-In Discovery

### Goal

Expose an in-process registry for built-in block definitions.

### Work

- Define a `Registry` interface.
- Implement a deterministic built-in registry.
- Add:
  - `ListBlocks`;
  - `GetBlock`;
  - `ValidateDefinition`;
  - `ValidateRequest`.
- Add block summaries that hide verbose component/net details.
- Ensure duplicate block IDs are detected at registry construction time.
- Add a placeholder definition for each initial block:
  - `led_indicator`;
  - `voltage_regulator`;
  - `mcu_minimal`;
  - `usb_c_power`;
  - `i2c_sensor`;
  - `opamp_gain_stage`;
  - `connector_breakout`.
- Mark placeholders `experimental`.

### Tests

- Registry lists block IDs in sorted order.
- `GetBlock` returns exact definitions.
- Duplicate IDs fail registry validation.
- All seven initial block IDs are present.
- Placeholder definitions have parameters, ports, and verification metadata.

### Acceptance Criteria

- AI and CLI layers can discover block metadata through Go APIs.
- No block instantiation yet.

### Commit Message

```text
Add built-in circuit block registry
```

## 5. Phase 3: Block CLI Surface

### Goal

Expose block discovery and request validation through the CLI.

### Work

- Add `block` command family to `cmd/kicadai`.
- Support:
  - `kicadai --json block list`;
  - `kicadai --json block show <block-id>`;
  - `kicadai --json block validate <block-id> --params params.json`.
- Reuse existing global `--request` for parameter JSON when practical, or add a
  narrowly scoped `--params` flag if cleaner.
- Use `reports.Result` for all output.
- Return missing block and invalid argument diagnostics as structured issues.
- Do not instantiate designs yet.

### Tests

- `block list` returns the seven built-in summaries.
- `block show led_indicator` returns full definition details.
- missing block returns `MISSING_FILE` or equivalent structured issue.
- invalid params return `VALIDATION_FAILED`.
- non-JSON block commands are rejected consistently if the current CLI requires
  JSON for structured output.

### Acceptance Criteria

- A user or AI can inspect available blocks and parameter schemas from the CLI.

### Commit Message

```text
Expose circuit block CLI discovery
```

## 6. Phase 4: Transaction Emission Foundation

### Goal

Add shared machinery for converting block instances into existing transaction
operations.

### Work

- Define a block instantiation result that includes:
  - normalized parameters;
  - generated references;
  - generated nets;
  - exported ports;
  - transaction operations;
  - validation issues;
  - verification level.
- Add deterministic reference allocation helpers.
- Add deterministic net naming helpers.
- Add component-to-transaction helpers for:
  - `add_symbol`;
  - `connect`;
  - `assign_footprint`;
  - `place_footprint`.
- Support schematic-only output first.
- Add a dry-run instantiation mode for tests and CLI output.

### Tests

- reference allocation is deterministic and prefix-scoped.
- net names are deterministic and instance-scoped.
- component helper emits stable transaction JSON.
- exported ports are present and unique.
- unsupported operation needs produce structured issues.

### Acceptance Criteria

- Blocks can generate transaction payloads without writing projects.

### Commit Message

```text
Add circuit block transaction emission
```

## 7. Phase 5: LED Indicator Block

### Goal

Implement the first real block end to end.

### Work

- Implement `led_indicator` parameters:
  - `supply_voltage`;
  - `led_forward_voltage`;
  - `led_current`;
  - `resistor_value`;
  - `color`;
  - `active_high`;
  - `resistor_footprint`;
  - `led_footprint`.
- Calculate resistor value when omitted.
- Select default symbols:
  - `Device:R`;
  - `Device:LED`.
- Select conservative default footprints.
- Emit schematic transactions for resistor, LED, labels, and wiring.
- Emit basic PCB placement hints or placement transactions if enough data is
  available.
- Add validation:
  - current range;
  - positive resistor;
  - polarity;
  - required ports.

### Tests

- default LED request instantiates deterministic operations.
- resistor value calculation is correct for known inputs.
- invalid current blocks.
- invalid voltage relationship blocks.
- active-high and active-low net order is correct.
- generated transaction validates with existing transaction validation.

### Acceptance Criteria

- `block instantiate led_indicator` returns usable transaction operations.
- Output remains explicitly non-fabrication-ready unless pinmaps validate.

### Commit Message

```text
Implement LED indicator circuit block
```

## 8. Phase 6: Connector Breakout Block

### Goal

Implement a parameterized connector breakout block because it is foundational
for board-level composition.

### Work

- Implement `connector_breakout` parameters:
  - `pin_names`;
  - `pin_count`;
  - `connector_symbol`;
  - `connector_footprint`;
  - `include_labels`;
  - `include_mounting_holes`.
- Validate pin count and unique pin names.
- Default to common generic connector symbols and pin header footprints.
- Emit one exported port per connector pin.
- Emit schematic symbol and labels.
- Emit footprint placement with deterministic pad-to-net assignment where
  resolver data exists.

### Tests

- 1x02 and 1x04 connector requests instantiate.
- pin count mismatch blocks.
- duplicate pin names block unless explicitly allowed later.
- exported ports match input order.
- generated operations validate.

### Acceptance Criteria

- A breakout can be generated from a pin-name list.

### Commit Message

```text
Implement connector breakout circuit block
```

## 9. Phase 7: Resolver-Backed Block Validation

### Goal

Integrate block instantiation with the library resolver and pinmap checks.

### Work

- Add optional resolver context to block instantiation.
- Validate required symbols and footprints exist.
- Validate symbol-footprint compatibility for each component.
- Attach pinmap candidate or verified pinmap status to block output.
- Add warnings when resolver roots are not configured.
- Block fabrication readiness when:
  - required records are missing;
  - compatibility is incompatible or unknown;
  - pinmap is unverified.
- Keep block generation usable in schematic-only mode without roots, but with
  clear warnings.

### Tests

- missing resolver roots warn, not panic.
- missing symbol blocks resolver-backed validation.
- missing footprint blocks resolver-backed validation.
- compatible resistor/LED mappings pass structural checks with fixtures.
- unverified pinmap emits warning and blocks fabrication readiness.

### Acceptance Criteria

- Blocks use real resolver data when available.
- AI output can distinguish generated shape from verified readiness.

### Commit Message

```text
Validate circuit blocks with library resolver
```

## 10. Phase 8: Block Composition Engine

### Goal

Allow multiple block instances to connect through named ports.

### Work

- Add composition request model:
  - project name;
  - block instances;
  - instance params;
  - port connections;
  - net aliases.
- Validate:
  - unique instance IDs;
  - known block IDs;
  - known ports;
  - compatible port directions/classes;
  - no conflicting net aliases.
- Merge generated transactions deterministically.
- Allocate references across block instances.
- Preserve instance metadata in the output.
- Add basic voltage-domain conflict detection where block ports declare voltage.

### Tests

- LED plus connector composition.
- regulator output connected to LED input.
- unknown port blocks.
- duplicate instance ID blocks.
- conflicting net alias blocks.
- output operation ordering is deterministic.

### Acceptance Criteria

- Multiple blocks can be composed into one transaction payload.

### Commit Message

```text
Add circuit block composition engine
```

## 11. Phase 9: Project Generation Integration

### Goal

Write composed block outputs as KiCad projects through existing transaction or
design writer paths.

### Work

- Add CLI:
  - `kicadai --json block instantiate <block-id> --output <dir>`;
  - `kicadai --json block compose --request design.json --output <dir>`.
- Support dry-run output when `--output` is omitted.
- Use existing transaction planning/apply where possible.
- Require `--overwrite` for existing output directories.
- Run existing inspection/evaluation after write and include report summary.
- Keep generated block metadata in sidecar report JSON if useful.

### Tests

- LED block writes a project.
- connector breakout writes a project.
- composed LED plus connector writes a project.
- existing output without `--overwrite` blocks.
- generated project passes existing inspect/evaluate parse checks.

### Acceptance Criteria

- Users can generate KiCad projects from block requests.

### Commit Message

```text
Generate projects from circuit blocks
```

## 12. Phase 10: Voltage Regulator Block

### Goal

Implement the first power-management block.

### Work

- Implement `voltage_regulator` for fixed linear regulator topology.
- Parameters:
  - input voltage min/max;
  - output voltage;
  - output current;
  - regulator symbol;
  - regulator footprint;
  - input/output capacitance;
  - optional enable handling.
- Add input and output capacitors.
- Add optional power LED by composing or internally reusing LED block logic.
- Add dropout and dissipation warnings.
- Add placement hints for close capacitors.

### Tests

- default 5 V to 3.3 V regulator request instantiates.
- invalid dropout blocks or warns according to configured severity.
- capacitors connect to correct nets.
- optional enable mode validates.
- optional power LED composes deterministically.

### Acceptance Criteria

- Regulated rail block can feed other composed blocks.

### Commit Message

```text
Implement voltage regulator circuit block
```

## 13. Phase 11: I2C Sensor Block

### Goal

Implement a reusable I2C sensor block with pull-ups and address checks.

### Work

- Implement `i2c_sensor` parameters:
  - sensor symbol;
  - sensor footprint;
  - supply voltage;
  - address;
  - pull-up value;
  - include pullups;
  - include interrupt;
  - include decoupling.
- Emit SDA/SCL/VCC/GND ports.
- Add pull-up resistors unless externally supplied.
- Add decoupling capacitor.
- Add optional INT port.
- Add composition-level I2C address collision check.

### Tests

- default sensor request instantiates using fixture IDs.
- pull-ups connect to selected rail.
- address format validation.
- duplicate address on same composed bus blocks.
- optional interrupt exports port.

### Acceptance Criteria

- I2C sensor block can compose with regulator and connector blocks.

### Commit Message

```text
Implement I2C sensor circuit block
```

## 14. Phase 12: Op-Amp Gain Stage Block

### Goal

Implement a parameterized analog gain block.

### Work

- Support non-inverting topology first.
- Add inverting topology if the model supports it cleanly.
- Parameters:
  - topology;
  - gain;
  - op-amp symbol;
  - op-amp footprint;
  - supply mode;
  - input coupling;
  - feedback package;
  - include output resistor.
- Calculate resistor ratios.
- Add bias network for single-supply AC-coupled mode.
- Add decoupling capacitors.
- Validate no op-amp input is floating.

### Tests

- non-inverting gain of 2 instantiates.
- invalid gain blocks.
- resistor ratio calculation is deterministic.
- single-supply AC mode includes bias network.
- output port is exported.

### Acceptance Criteria

- Op-amp gain stage can be generated with explainable resistor selections.

### Commit Message

```text
Implement op-amp gain stage circuit block
```

## 15. Phase 13: USB-C Power Input Block

### Goal

Implement a USB-C sink power block for 5 V input.

### Work

- Parameters:
  - connector footprint;
  - current limit;
  - include fuse;
  - include TVS;
  - include bulk capacitor;
  - include power LED;
  - shield policy.
- Add CC1/CC2 Rd pull-down resistors.
- Add VBUS and GND connectivity.
- Add optional fuse in series with VBUS.
- Add optional TVS and bulk capacitor.
- Add explicit no-connect handling for data pins where supported.

### Tests

- default USB-C power request instantiates.
- CC resistors are present and connected.
- fuse is in series when enabled.
- VBUS output port is exported.
- unsupported data mode blocks with a structured issue.

### Acceptance Criteria

- USB-C power block can feed regulator or 5 V output blocks.

### Commit Message

```text
Implement USB-C power input circuit block
```

## 16. Phase 14: MCU Minimal System Block

### Goal

Implement the most constrained initial block, with explicit gaps for MCU
semantic metadata.

### Work

- Parameters:
  - MCU symbol;
  - MCU footprint;
  - supply voltage;
  - clock mode;
  - reset mode;
  - programming header mode;
  - decoupling capacitor count/value.
- Require a block-local pin role map until resolver symbol semantics improve.
- Add VCC/GND/RESET/programming ports.
- Add decoupling capacitors.
- Add reset pull-up and optional reset switch.
- Add optional programming header.
- Add clear warnings for unresolved alternate-function metadata.

### Tests

- fixture MCU minimal request instantiates.
- missing pin role map blocks.
- decoupling count validation.
- reset mode validation.
- programming header pins connect to expected ports.

### Acceptance Criteria

- MCU block is usable with explicitly mapped MCU families and honest warnings.

### Commit Message

```text
Implement MCU minimal system circuit block
```

## 17. Phase 15: Examples and Validation Reports

### Goal

Add checked-in examples for each block and at least one composed design.

### Work

- Add `examples/blocks/`.
- Generate example projects for:
  - LED indicator;
  - connector breakout;
  - regulator;
  - I2C sensor;
  - op-amp gain stage;
  - USB-C power;
  - MCU minimal system;
  - composed USB-C/regulator/I2C/connector design.
- Include request JSON files.
- Include validation report snapshots if stable.
- Run inspect/evaluate on generated examples.
- Run round-trip for the simplest examples where `kicad-cli` is available or
  keep tests opt-in.

### Tests

- examples parse with existing readers.
- request JSON files validate.
- generated examples evaluate without unexpected blocking issues.

### Acceptance Criteria

- Users can inspect real block-generated KiCad projects.

### Commit Message

```text
Add circuit block examples
```

## 18. Phase 16: Documentation

### Goal

Document block usage, verification levels, CLI commands, and known limitations.

### Work

- Update `README.md`.
- Add `docs/circuit-block-library.md`.
- Document:
  - block list/show/instantiate/compose commands;
  - parameter file shape;
  - verification levels;
  - resolver requirements;
  - examples;
  - AI usage pattern;
  - current limitations.
- Include sample requests for LED, connector breakout, and composed sensor
  breakout.

### Tests

- Normal test suite.
- Optional markdown link sanity if available.

### Acceptance Criteria

- A new developer can instantiate and compose blocks from documentation.

### Commit Message

```text
Document circuit block library
```

## 19. Phase 17: Release Readiness and Gap Review

### Goal

Confirm the block library is reliable enough for AI-assisted design experiments
and clearly document remaining gaps before full autonomy.

### Work

- Run full normal tests.
- Run available integration tests.
- Run opt-in KiCad CLI round-trip checks where local tooling is configured.
- Review each block's verification level.
- Add or update a gap matrix:
  - resolver gaps;
  - writer gaps;
  - validation gaps;
  - electrical-domain gaps;
  - placement/routing gaps.
- Ensure unverified blocks cannot claim fabrication readiness.
- Ensure CLI failures are structured.

### Tests

- `GOCACHE=/tmp/kicadai-gocache go test ./...`
- opt-in resolver integration tests if roots are configured;
- opt-in round-trip tests if `kicad-cli` is configured.

### Acceptance Criteria

- All implemented block phases pass tests.
- Documentation and reports accurately reflect verification status.
- Remaining full-autonomy blockers are explicit.

### Commit Message

```text
Review circuit block library readiness
```

## 20. Cross-Phase Risks

### 20.1 Over-Trusting Blocks

Risk: AI treats a generated block as production-ready.

Mitigation: every output carries verification level, pinmap status, and
fabrication-readiness blockers.

### 20.2 Part-Specific Electrical Requirements

Risk: generic regulator, MCU, sensor, or op-amp assumptions are invalid for a
specific part.

Mitigation: block-local rule sets are conservative; unknown requirements warn or
block.

### 20.3 Resolver Semantics

Risk: symbol/footprint metadata is insufficient for MCU alternate functions,
USB-C connectors, and complex ICs.

Mitigation: require explicit pin role maps and verified pinmaps until resolver
semantics improve.

### 20.4 Layout Quality

Risk: generated placement/routing is parseable but electrically poor.

Mitigation: encode keep-close hints, decoupling rules, route classes, zones, and
evaluation warnings; do not claim DRC/ERC verification unless actually run.

### 20.5 Scope Growth

Risk: blocks become a full EDA expert system before the core model is stable.

Mitigation: implement simple verified variants first, add optional complexity
only after validation coverage exists.

## 21. Definition of Done

The circuit block library project is complete when:

- all seven initial blocks are registered and documented;
- LED and connector blocks are implemented end to end;
- regulator, I2C sensor, op-amp, USB-C power, and MCU minimal blocks generate
  deterministic schematic transactions;
- supported blocks can emit PCB placement hints or explicit warnings;
- block composition connects ports and catches common conflicts;
- resolver-backed validation runs where roots are configured;
- unverified pinmaps block fabrication readiness;
- examples exist and parse;
- normal tests pass without KiCad or external roots;
- prism review is clean or has only documented non-blocking findings for each
  phase.
