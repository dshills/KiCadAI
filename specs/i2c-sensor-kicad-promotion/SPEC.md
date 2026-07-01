# I2C Sensor KiCad-Backed Promotion Specification

## 1. Purpose

Promote `examples/design/kicad-backed/i2c_sensor_breakout_candidate` from
`expected_fail` toward `candidate`.

This fixture is the next KiCad-backed generated-design promotion after
`connector_led_kicad_smoke` and `led_indicator_kicad_smoke`. It is the first
small multi-block bus-oriented fixture that should prove KiCadAI can generate a
useful sensor breakout with a connector, a sensor, pull-ups, power rails,
schematic labels, footprint assignment, placement, inter-block routing, and
KiCad-backed validation evidence.

## 2. Background

The LED and connector/LED smoke fixtures now provide candidate-level evidence
for:

- KiCad-native net declarations and pad/copper net assignment;
- block-local route endpoint binding to physical same-net pads;
- generated schematic labels for exported pseudo-ports;
- schematic electrical checks in the workflow and promotion report;
- promotion reports that classify declared and achieved readiness;
- warning-only KiCad evidence where local KiCad reports non-blocking warnings.

The I2C sensor fixture remains `expected_fail`. Its current metadata says:

- generic I2C sensor plus connector PCB realization has route-candidate and
  contact evidence;
- routed same-net completion and KiCad DRC evidence are incomplete;
- routing is intentionally skipped.

That expected-fail state is no longer specific enough for the next promotion
step. The fixture should either:

- become a `candidate` fixture with routing enabled and warning-only remaining
  KiCad evidence; or
- remain `expected_fail` with current, precise blockers emitted by the workflow
  and recorded in metadata.

## 3. Target Fixture

Request:

```text
examples/design/kicad-backed/i2c_sensor_breakout_candidate.json
```

Metadata:

```text
examples/design/kicad-backed/i2c_sensor_breakout_candidate.metadata.json
```

Current design shape:

- board: 55 mm x 35 mm, two layers;
- blocks:
  - `sensor`: `i2c_sensor`;
  - `io`: `connector_breakout` with `VCC`, `GND`, `SDA`, and `SCL`;
- connections:
  - `sensor.VCC` to `io.VCC`;
  - `sensor.GND` to `io.GND`;
  - `sensor.SDA` to `io.SDA`;
  - `sensor.SCL` to `io.SCL`;
- validation:
  - `acceptance: erc-drc`;
  - `require_erc: true`;
  - `require_drc: true`;
  - currently `skip_routing: true`.

## 4. Goals

- Enable routing for the I2C fixture unless implementation finds a current
  blocker that must keep it `expected_fail`.
- Generate deterministic route operations for `VCC`, `GND`, `SDA`, and `SCL`
  between the sensor block and connector block.
- Prove each routed inter-block net with endpoint-contact evidence against
  physical same-net pad anchors.
- Ensure generated PCB files preserve KiCad-native net declarations and
  pad/copper net names for all four exported nets.
- Ensure generated schematic output avoids conflicting local labels for shared
  exported nets.
- Ensure schematic electrical validation is included in the workflow and
  promotion report.
- Ensure writer correctness and board validation have no blocking findings.
- Run optional real KiCad ERC/DRC through the existing KiCad-backed fixture
  policy when `KICADAI_KICAD_CLI` is configured.
- Promote metadata to `candidate` only if achieved readiness is `candidate` or
  better.
- Keep default `go test ./...` independent of a local KiCad installation.

## 5. Non-Goals

- Do not require the fixture to reach `pass` readiness in this work.
- Do not solve arbitrary multi-block routing for every future board.
- Do not make the I2C sensor fixture fabrication-ready.
- Do not introduce live component sourcing.
- Do not replace the generic I2C sensor with a complete concrete production
  part unless that is required to remove a current blocker.
- Do not mutate imported/user-authored projects.
- Do not make real KiCad required for the default test suite.

## 6. Fixture Artifact Contract

While the fixture remains `expected_fail`, the only required generated artifact
is `.kicadai/design-promotion.json`, because the workflow intentionally stops
before project write when routing is blocked.

When the fixture is promoted to `candidate`, the expected artifacts must expand
to include:

- `.kicadai/transaction.json`;
- `.kicadai/manifest.json`;
- `.kicadai/design-promotion.json`;
- generated `.kicad_pro`;
- generated `.kicad_sch`;
- generated `.kicad_pcb`.

## 7. Candidate Acceptance Criteria

The fixture may be marked `candidate` when all of the following are true:

- Request validation passes with routing enabled.
- Metadata readiness is `candidate`.
- Expected artifacts satisfy the promoted candidate artifact contract.
- Expected stages include:
  - `block_planning`;
  - `component_selection`;
  - `schematic`;
  - `schematic_electrical`;
  - `pcb_realization`;
  - `placement`;
  - `routing`;
  - `project_write`;
  - `writer_correctness`;
  - `validation`;
  - `kicad_checks`.
- Workflow result reports `ok: true`.
- `data.acceptance.achieved` is at least `erc-drc`.
- Promotion summary reports:
  - `declared_readiness: "candidate"`;
  - `achieved_readiness: "candidate"` or `pass`;
  - `matches_expectation: true`.
- Route evidence reports route attempts for `VCC`, `GND`, `SDA`, and `SCL`.
- Endpoint-contact evidence proves each required route contacts physical
  same-net pad anchors at both ends.
- Same-net contact graph completion reports all required inter-block nets
  complete.
- Writer correctness has no blocking findings.
- Board validation has no disconnected pads, missing outlines, invalid net
  assignments, or unrouted required-net blockers.
- KiCad ERC is clean when real KiCad checks run.
- KiCad DRC either:
  - is clean;
  - contains warning-level findings acceptable for candidate readiness; or
  - hits an already-classified warning-only local KiCad DRC no-finding tool
    instability.

## 7. Expected Investigation Areas

### 7.1 Routing Skip

The request currently contains:

```json
"skip_routing": true
```

The first implementation step is to remove or override this skip and inspect
the real failure mode. If routing fails, the expected-fail metadata must name
the precise stage, net, and diagnostic category.

### 7.2 Inter-Block Route Completion

The fixture should produce routed contact evidence for four nets:

- `VCC`;
- `GND`;
- `SDA`;
- `SCL`.

The route-completion gate must only count a net as complete when the generated
copper contact graph reaches physical pads on both the sensor and connector
side. Name-only net assignment is not enough.

### 7.3 Pull-Up And Bus Semantics

The `i2c_sensor` block includes pull-ups. Promotion must verify that pull-up
pins attach to the intended `SDA`, `SCL`, and `VCC` nets without creating label
conflicts or disconnected one-pad nets.

### 7.4 Placement Quality

The connector and sensor should be placed with enough spacing and edge
orientation for deterministic route attempts. If routes fail due to spacing,
fanout, edge, or obstacle diagnostics, the implementation should prefer narrow
placement/routing evidence fixes over broad placement-engine rewrites.

### 7.5 Schematic Electrical Findings

The generated schematic should not produce blocking schematic electrical
findings for:

- conflicting labels;
- missing required pins;
- floating required bus pins;
- missing PWR_FLAG or power-source evidence where the current policy requires
  it.

Candidate readiness may tolerate explicitly warning-level component-selection
or source-evidence gaps, but not ERC-blocking schematic connectivity mistakes.

### 7.6 KiCad DRC Findings

If KiCad DRC reports true layout errors, keep the fixture `expected_fail` and
record those blockers. If KiCad reports warning-only library/footprint or
silkscreen findings comparable to the existing LED candidates, preserve them as
candidate warnings rather than suppressing them globally.

## 8. Metadata Updates

If promotion succeeds, update:

```text
examples/design/kicad-backed/i2c_sensor_breakout_candidate.metadata.json
```

Required changes:

- `readiness`: `candidate`;
- add `.kicadai/design-promotion.json` to `expected_artifacts`;
- add `schematic_electrical` and `routing` to `expected_stages`;
- remove stale language saying routing is intentionally skipped;
- replace expected-fail blockers with current candidate warnings;
- describe what the fixture now proves in `notes`.

If promotion does not succeed, keep `readiness: expected_fail`, but update
`known_gaps` with the current exact blockers discovered during implementation.

## 9. Test Requirements

Required focused tests:

```text
go test ./internal/designworkflow
go test ./internal/blocks ./internal/routing ./internal/schematicpcb ./internal/evaluate
```

Required full test:

```text
go test ./...
```

Recommended local fixture command:

```text
kicadai --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json --output examples/.generated/i2c_sensor_breakout_candidate --overwrite design create
```

The generated output directory remains ignored and must not be committed.

## 10. Done Definition

This work is complete when:

- `i2c_sensor_breakout_candidate` is either promoted to `candidate` or its
  expected-fail metadata is refreshed with precise current blockers;
- VCC/GND/SDA/SCL route-contact evidence is tested or explicitly identified as
  the remaining blocker;
- promotion report behavior is covered by design workflow regression tests;
- full Go tests pass;
- Prism review has no unresolved high or medium findings;
- changes are committed.
