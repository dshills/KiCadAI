# I2C Sensor KiCad-Backed Promotion Implementation Plan

## Phase 1: Baseline The Fixture

### Objectives

- Establish the current generated-design behavior for
  `i2c_sensor_breakout_candidate`.
- Identify the exact blockers behind its `expected_fail` state.

### Tasks

- Run the fixture through `design create` into
  `examples/.generated/i2c_sensor_breakout_candidate`.
- Capture the workflow stages, promotion summary, writer correctness, board
  validation, route evidence, and KiCad check summaries.
- Confirm whether `skip_routing: true` is still the only reason route evidence
  is incomplete.
- Record current blockers by stage and net.

### Acceptance

- There is a clear before-state for the fixture.
- Any current blocker is attributable to a stage, operation category, and net
  where possible.

## Phase 2: Enable Routing For The Fixture

### Objectives

- Remove the stale routing skip from the request if routing is now viable.
- Ensure route attempts are created for `VCC`, `GND`, `SDA`, and `SCL`.

### Tasks

- Update `i2c_sensor_breakout_candidate.json` to stop skipping routing.
- Run the design workflow and inspect routing summaries.
- If route attempts are missing, trace block composition, PCB realization,
  schematic-to-PCB transfer, and route planning to find where net intent is
  lost.
- Add focused tests for required route intent if missing.

### Acceptance

- The fixture reaches the routing stage.
- Routing evidence includes all four expected inter-block nets, or the current
  missing-net blocker is documented precisely.

## Phase 3: Prove Physical Endpoint Contacts

### Objectives

- Ensure generated routes are not merely named correctly, but physically touch
  the correct same-net pads.

### Tasks

- Inspect endpoint-contact evidence for `VCC`, `GND`, `SDA`, and `SCL`.
- Confirm each route binds to a sensor-side and connector-side physical pad.
- Fix route endpoint discovery or pad-anchor propagation if a route has a
  same-net name but no physical endpoint contact.
- Add regression coverage for I2C fixture route-contact summaries.

### Acceptance

- Each required inter-block net has route-contact evidence for both endpoints.
- Same-net contact graph completion is true for the required inter-block nets.
- No route is counted complete solely because of matching net names.

## Phase 4: Resolve Schematic And Validation Blockers

### Objectives

- Remove schematic electrical and board-validation blockers that prevent
  candidate readiness.

### Tasks

- Run `evaluate project` or inspect the workflow validation stage for:
  - conflicting labels;
  - disconnected pads;
  - invalid net assignments;
  - missing outline;
  - unrouted required nets;
  - power-source or PWR_FLAG policy findings.
- Fix narrow writer, label, transfer, or evaluator issues exposed by the I2C
  fixture.
- Add focused tests for any fixed schematic or PCB semantic issue.

### Acceptance

- Writer correctness has no blocking issues.
- Board validation has no blocking issues.
- Schematic electrical validation has no blocking issues.

## Phase 5: KiCad-Backed Evidence Classification

### Objectives

- Classify real KiCad ERC/DRC output correctly for candidate readiness.

### Tasks

- Run the fixture with `KICADAI_KICAD_CLI` configured when available.
- Verify ERC is clean or produces only explicitly allowed warning evidence.
- Verify DRC findings are either warning-level candidate evidence or true
  blockers.
- Do not globally suppress KiCad findings. Preserve exact report artifacts and
  promotion issue summaries.

### Acceptance

- `kicad_checks` no longer block candidate readiness unless there is a true
  current KiCad ERC/DRC blocker.
- Promotion reports retain referenced KiCad artifacts and warning details.

## Phase 6: Update Metadata, Roadmap, And Docs

### Objectives

- Make checked-in fixture metadata match the achieved behavior.

### Tasks

- If promotion succeeds:
  - set metadata `readiness` to `candidate`;
  - expect `.kicadai/transaction.json`, `.kicadai/manifest.json`,
    `.kicadai/design-promotion.json`, and generated KiCad project files;
  - add `schematic_electrical` and `routing` to expected stages;
  - replace stale known gaps with current warning-level gaps;
  - update notes to describe route-contact and KiCad evidence.
- If promotion does not succeed:
  - keep `expected_fail`;
  - require only `.kicadai/design-promotion.json` when the workflow stops
    before project write;
  - replace stale known gaps with current exact blockers;
  - include the stage/net/finding category that blocks promotion.
- Update `specs/ROADMAP.md` to reflect the new status.
- Update README or focused docs only if user-facing commands or fixture status
  changed.

### Acceptance

- Metadata matches the workflow result.
- Roadmap no longer describes stale I2C blockers.
- Docs do not overclaim pass/fabrication readiness.

## Phase 7: Regression, Prism, And Commit

### Objectives

- Prove the implementation is stable and reviewed.

### Tasks

- Run focused tests:

```text
go test ./internal/designworkflow
go test ./internal/blocks ./internal/routing ./internal/schematicpcb ./internal/evaluate
```

- Run full tests:

```text
go test ./...
```

- Stage all intended changes.
- Run:

```text
prism review staged
```

- Fix all high and medium Prism findings.
- Commit the completed phase.

### Acceptance

- Full Go test suite passes.
- Prism has no unresolved high or medium findings.
- Work is committed with a focused commit message.
