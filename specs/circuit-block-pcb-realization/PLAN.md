# Circuit Block PCB Realization Implementation Plan

## 1. Objective

Implement Circuit Block PCB Realization from `SPEC.md` in small, reviewable
phases.

The end state is a block realization layer that can take verified block
instances and produce schematic + PCB fragments with placements, footprints,
local routes, zones, constraints, and validation evidence.

The first practical milestone is:

- LED indicator produces a locally placed and routed PCB fragment.
- Connector breakout produces placed connector footprints and optional fanout.
- At least one power block produces local placement, zone, and caveat evidence.
- Realized block output can be validated with connectivity-first board
  validation.

## 2. Implementation Rules

- Build on existing `internal/blocks`, `placement`, `routing`, `transactions`,
  `designapi`, `libraryresolver`, and `boardvalidation`.
- Do not create a parallel PCB writer or route model.
- Keep normal tests independent of KiCad GUI and external KiCad library roots.
- Use resolver-backed geometry when available, with explicit warnings for
  estimated bounds.
- Preserve existing block instantiation behavior unless a phase explicitly
  extends it.
- Every phase should include tests.
- Run `gofmt` on edited Go files.
- Run `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` before each commit
  where practical.
- Run `prism review staged` before each phase commit and address concrete
  correctness findings.
- Commit between phases.
- Do not stage unrelated changes such as the existing modified
  `specs/ROADMAP.md` unless explicitly requested.

## 3. Phase 1: Realization Model

### Goal

Add explicit PCB realization types without changing behavior.

### Work

- Add realization model types to `internal/blocks` or a new
  `internal/blocks/realization.go`.
- Define:
  - `PCBRealization`;
  - `PCBComponentRealization`;
  - `RelativePlacement`;
  - `RelativePoint`;
  - `RelativeBounds`;
  - `PCBPlacementGroup`;
  - `PCBLocalRoute`;
  - `RouteEndpoint`;
  - `PCBZoneRealization`;
  - `PCBKeepout`;
  - `PCBConstraint`;
  - `PCBValidationExpectations`;
  - `PCBVerificationLevel`.
- Add deterministic JSON tags.
- Add validation helpers:
  - finite coordinate checks;
  - valid layer checks;
  - known component role checks;
  - duplicate group IDs;
  - duplicate local route IDs;
  - missing endpoint detection.
- Keep realization optional on `BlockDefinition`.

### Tests

- JSON shape for a minimal realization.
- Invalid coordinates return structured issues.
- Unknown component role returns structured issues.
- Duplicate groups/routes are rejected.
- Empty realization validates as unsupported but not a panic.

### Acceptance Criteria

- Existing block tests continue to pass.
- No CLI behavior changes.

### Commit Message

```text
Add circuit block PCB realization model
```

## 4. Phase 2: Realization Registry And Definition Wiring

### Goal

Attach PCB realization metadata to built-in blocks in a deterministic way.

### Work

- Add accessors:
  - `RealizationForBlock(blockID)`;
  - `ValidateRealization(definition, realization)`;
  - `BlockSupportsPCBRealization(blockID)`.
- Decide whether realization data is embedded in `BlockDefinition` or stored in
  a separate registry keyed by block ID.
- Add placeholder realization records for all seven built-in blocks.
- Mark unsupported or partial blocks explicitly.
- Add summaries to block `show` output if the data is embedded in definitions.

### Tests

- All built-in block IDs have a realization status.
- Missing realization returns a structured unsupported issue.
- Placeholder realization metadata is deterministic.
- Definition/realization role references are validated.

### Acceptance Criteria

- AI agents can discover which blocks support PCB realization and at what level.

### Commit Message

```text
Register PCB realization metadata for blocks
```

## 5. Phase 3: Realization Result And API Skeleton

### Goal

Expose a Go API that realizes one block instance and returns a structured
report, even before all blocks have full PCB support.

### Work

- Add `RealizeBlock(ctx, registry, request, options)`.
- Define `RealizationOptions`:
  - board area;
  - origin transform;
  - seed;
  - library index;
  - write project option;
  - strict validation options.
- Define `RealizationReport`:
  - block output;
  - operations;
  - placement result;
  - routing result;
  - board-validation result;
  - artifacts;
  - issues;
  - status.
- Status values:
  - `ready`;
  - `partial`;
  - `blocked`;
  - `error`.
- Return unsupported realization reports for blocks without implementation.

### Tests

- Unsupported block returns `blocked` with `UNSUPPORTED_OPERATION`.
- Invalid request returns structured validation issues.
- Empty realization does not panic.
- Report status aggregation handles placement/routing/validation issue groups.

### Acceptance Criteria

- There is one stable Go entry point for block PCB realization.

### Commit Message

```text
Add circuit block PCB realization API
```

## 6. Phase 4: Placement Request Adapter

### Goal

Convert block PCB realization metadata into placement engine requests.

### Work

- Build a `placement.Request` from:
  - block output references;
  - component roles;
  - relative placements;
  - placement groups;
  - footprint geometry from resolver records;
  - estimated fallback bounds.
- Preserve fixed placements when realization defines exact relative positions.
- Add origin/rotation transform support for block-local coordinates.
- Convert placement result back into `place_footprint` operations.
- Emit warnings for estimated geometry.

### Tests

- LED realization converts to two placement components.
- Placement groups map to existing placement group fields.
- Origin transform shifts placements deterministically.
- Missing footprint geometry returns warning or blocker per options.
- Placement operations preserve refs, footprint IDs, layers, and rotations.

### Acceptance Criteria

- Realization reports can include concrete placement operations.

### Commit Message

```text
Convert block realizations to placement requests
```

## 7. Phase 5: LED Indicator PCB Realization

### Goal

Make `led_indicator` produce a complete locally placed and routed PCB fragment.

### Work

- Define LED realization:
  - resistor placement;
  - LED placement;
  - fixed relative route between resistor and LED;
  - exported input/output or power ports.
- Use resolver-backed footprints when available.
- Emit placement operations.
- Emit route operations for the resistor-to-LED local net.
- Build a minimal project fixture in tests.
- Run boardvalidation on the realized board/model.

### Tests

- Realization status is `ready`.
- Operations include `assign_footprint`, `place_footprint`, and `route`.
- Local resistor-to-LED net is fully routed.
- Boardvalidation status passes without KiCad DRC.
- Output is deterministic for same request/seed.

### Acceptance Criteria

- LED indicator is the first `pcb_connectivity_verified` block.

### Commit Message

```text
Realize LED indicator PCB fragment
```

## 8. Phase 6: Connector Breakout PCB Realization

### Goal

Make `connector_breakout` produce deterministic connector placement and optional
fanout route stubs.

### Work

- Define connector placement:
  - connector footprint;
  - edge orientation constraint;
  - exported ports for each pin;
  - optional test point/fanout route behavior if already modeled.
- Add route stubs only when request/config enables them.
- Add keepout or edge-facing constraint metadata.
- Validate all connector pads have intentional nets.

### Tests

- Connector breakout realization produces placed connector footprints.
- Port-to-pin mapping is deterministic.
- Fanout route option emits route operations.
- Boardvalidation catches missing/unknown pad nets.
- Edge orientation constraint is present in report.

### Acceptance Criteria

- Connector breakout is at least `pcb_placement_verified`, and
  `pcb_connectivity_verified` when fanout routes are enabled.

### Commit Message

```text
Realize connector breakout PCB fragment
```

## 9. Phase 7: Voltage Regulator PCB Realization

### Goal

Make the voltage regulator block produce useful PCB placement, route, zone, and
caveat evidence.

### Work

- Place regulator, input capacitor, output capacitor, and optional feedback
  divider as local groups.
- Add placement constraints:
  - input cap near VIN/GND;
  - output cap near VOUT/GND;
  - feedback divider near feedback pin;
  - input/output orientation hints.
- Add local routes for feedback/capacitor nets where endpoints are known.
- Add ground zone realization.
- Emit explicit thermal/current caveats.

### Tests

- Capacitors are placed near regulator.
- Feedback net routes when feedback divider exists.
- Ground zone operation is emitted.
- Strict zone validation changes missing-fill warning to blocking in tests.
- Report includes thermal/current caveat issue or note.

### Acceptance Criteria

- Regulator realization is useful for AI layout composition while not
  overstating thermal/fabrication readiness.

### Commit Message

```text
Realize voltage regulator PCB fragment
```

## 10. Phase 8: I2C Sensor PCB Realization

### Goal

Make the I2C sensor block produce placement and local routing for pull-ups,
decoupling, and optional address strapping.

### Work

- Place sensor, pull-up resistors, decoupling capacitor, and connector/exported
  port anchor.
- Route decoupling and address strap nets.
- Keep SDA/SCL as exported nets unless local connector routing is requested.
- Add optional pull-up enable/disable handling.

### Tests

- Pull-ups are placed and connected when enabled.
- Decoupling capacitor is near sensor supply pins.
- Address strapping routes when configured.
- SDA/SCL exported ports remain available.
- Boardvalidation catches intentionally removed local route.

### Acceptance Criteria

- I2C sensor can be composed into larger boards without losing local passive
  intent.

### Commit Message

```text
Realize I2C sensor PCB fragment
```

## 11. Phase 9: Op-Amp Gain Stage PCB Realization

### Goal

Make the op-amp gain stage produce local feedback placement and routing.

### Work

- Place op-amp, feedback resistor network, input/output passives, and decoupling
  capacitors.
- Route feedback path locally.
- Route supply decoupling locally.
- Add analog sensitivity constraints:
  - short feedback;
  - keep sensitive input node away from noisy routes;
  - optional guard/keepout.
- Report unsupported precision/analog layout constraints as warnings.

### Tests

- Feedback network is placed near op-amp.
- Feedback net is locally routed.
- Supply decoupling is placed and connected.
- Analog caveats are included.
- Boardvalidation passes for deterministic local nets.

### Acceptance Criteria

- Op-amp gain stage has useful local PCB realization and honest analog caveats.

### Commit Message

```text
Realize op amp gain stage PCB fragment
```

## 12. Phase 10: MCU Minimal PCB Realization

### Goal

Make the MCU minimal block produce local placement/routing for decoupling,
reset, programming, clock, and boot/config passives where configured.

### Work

- Place MCU footprint.
- Place decoupling capacitors near power pins.
- Place reset pull-up and programming connector.
- Place optional crystal/resonator group.
- Route local reset/boot/config/decoupling nets where endpoints are known.
- Keep GPIO and bus nets exported.
- Add constraints for decoupling proximity and clock placement.

### Tests

- Decoupling caps are within configured max distance.
- Reset net routes.
- Programming connector ports are exported.
- Optional clock group appears only when configured.
- Boardvalidation passes local routed nets.

### Acceptance Criteria

- MCU minimal can serve as a practical core block for autonomous board
  generation.

### Commit Message

```text
Realize MCU minimal PCB fragment
```

## 13. Phase 11: USB-C Power PCB Realization

### Goal

Make the USB-C power block produce connector placement, CC resistor routing,
VBUS/GND exports, mechanical constraints, and compliance caveats.

### Work

- Place USB-C receptacle.
- Place CC resistors near receptacle.
- Route CC resistor local nets.
- Export VBUS and GND.
- Add connector edge orientation constraint.
- Add mechanical keepout for connector shell region.
- Add shield policy metadata.
- Report USB compliance caveats explicitly.

### Tests

- CC resistors are placed near connector.
- CC nets are routed.
- VBUS/GND are exported.
- Edge orientation and keepout constraints are present.
- Boardvalidation passes local routes.
- Compliance caveat is present.

### Acceptance Criteria

- USB-C power is useful as a block while clearly marking limits of compliance
  evidence.

### Commit Message

```text
Realize USB C power PCB fragment
```

## 14. Phase 12: Realization CLI

### Goal

Expose PCB realization through the CLI.

### Work

- Add:
  - `kicadai --json block realize <block-id>`;
  - `--request` for block request JSON;
  - `--output` and `--overwrite` for project writing;
  - optional board dimensions/margin flags if existing placement flags are not
    enough.
- Return `reports.Result` with `RealizationReport`.
- Preserve existing `block instantiate` behavior.
- If `--output` is set, write a KiCad project and include artifacts.
- Run boardvalidation after writing or model generation.

### Tests

- `block realize led_indicator` returns ready report.
- Unsupported block returns structured blocked report.
- `--output` writes project files.
- Invalid request returns structured issues.
- Required boardvalidation failure makes command nonzero.

### Acceptance Criteria

- Users and agents can realize a block from CLI without writing custom Go code.

### Commit Message

```text
Expose circuit block PCB realization CLI
```

## 15. Phase 13: Examples And Golden Fixtures

### Goal

Add durable examples that prove block PCB realization works.

### Work

- Add examples under `examples/blocks/realized` or similar.
- Include:
  - realized LED indicator;
  - realized connector breakout;
  - realized voltage regulator;
  - one composed small board if implementation is ready.
- Add README notes for examples.
- Add golden JSON snippets for report status, operations, and validation
  summaries where stable.

### Tests

- Example generation tests produce deterministic operation/report signatures.
- Generated PCB models pass boardvalidation.
- Fixture docs match available commands.

### Acceptance Criteria

- A user can inspect concrete realized block outputs.

### Commit Message

```text
Add realized circuit block examples
```

## 16. Phase 14: Documentation And Handoff

### Goal

Document the realization layer, verification levels, caveats, and next steps.

### Work

- Update `README.md`.
- Add or update `docs/circuit-block-library.md`.
- Document:
  - realization model;
  - CLI usage;
  - verification levels;
  - current supported blocks;
  - known limitations;
  - relationship to placement/routing/boardvalidation.
- Add a short future-work list:
  - global block composition;
  - inter-block routing;
  - KiCad DRC evidence fixtures;
  - richer analog/high-speed constraints.

### Tests

- Run full test suite.
- Run Prism on staged docs/code.

### Acceptance Criteria

- The next contributor can understand how to add or verify a realized block.

### Commit Message

```text
Document circuit block PCB realization
```

## 17. Suggested Implementation Order

Recommended order:

1. Realization model.
2. Realization registry.
3. Realization report/API.
4. Placement adapter.
5. LED indicator.
6. Connector breakout.
7. CLI for realized blocks.
8. Examples.
9. Regulator.
10. I2C sensor.
11. Op-amp.
12. MCU.
13. USB-C.
14. Documentation.

This order gets one complete vertical slice early, then broadens block coverage.

## 18. Risks And Mitigations

### Risk: Overclaiming Readiness

Mitigation:

- keep verification levels explicit;
- require boardvalidation evidence for `ready`;
- make DRC absence explicit;
- keep analog/thermal/USB caveats visible.

### Risk: Resolver Roots Not Available

Mitigation:

- allow estimated geometry with warnings for default tests;
- add resolver-backed tests that skip when roots are absent;
- keep fixture footprints small and deterministic.

### Risk: Local Routes Do Not Match Real Footprint Pads

Mitigation:

- derive endpoints from placed pad geometry when resolver data exists;
- run boardvalidation on generated board models;
- fail tests on disconnected pads or route endpoints.

### Risk: Realization Model Becomes A Second PCB Writer

Mitigation:

- emit transactions, placement requests, routing requests, and designapi calls;
- avoid standalone copper serialization in block code.

### Risk: Blocks Need Domain-Specific Constraints

Mitigation:

- represent constraints as structured metadata first;
- enforce only constraints that can be validated deterministically;
- report unsupported constraints as warnings, not hidden assumptions.

## 19. Completion Definition

The project is complete when:

- the realization model exists and validates;
- at least LED indicator and connector breakout are realized end to end;
- realized outputs include placement, footprint, route, and validation evidence;
- CLI can realize a block;
- generated examples exist;
- docs explain capabilities and caveats;
- `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` passes;
- staged changes have been reviewed with Prism before each commit.
