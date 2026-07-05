# I2C KiCad ERC Connectivity Closeout Specification

## Summary

Promote `examples/design/kicad-backed/i2c_sensor_breakout_candidate` from
`expected_fail` toward `candidate` by closing the remaining required KiCad ERC
schematic-connectivity blocker.

The I2C fixture now reaches project-write, writer-correctness, structural board
validation, route-tree branch completion, and route-tree contact graph proof.
The remaining expected failure is narrower: KiCad ERC reports generated
schematic connectivity findings such as disconnected labels, unconnected pins,
off-grid connection points, and unconnected wire endpoints.

This project must make the generated schematic KiCad-ERC clean enough for
candidate promotion without weakening the route-tree, writer-correctness, or
board-validation evidence already earned.

## Problem

The generated I2C schematic is internally meaningful to KiCadAI, but KiCad ERC
still sees schematic-geometry problems. The currently observed blocker shape
includes:

- `Label not connected`
- `Pin not connected`
- `Symbol pin or wire end off connection grid`
- `Label connected to only one pin`
- `Unconnected wire endpoint`

These findings keep the metadata readiness at `expected_fail` even though the
PCB-side route-tree evidence is complete.

The likely root is that generated schematic label stubs and/or block-composed
aliases represent net intent in a way that KiCad ERC does not accept as a fully
connected schematic. KiCadAI must either emit KiCad-native schematic geometry
that ERC accepts, or deliberately mark intentional non-connections and exported
net labels in a way that KiCad treats correctly.

## Goals

- Capture the current KiCad ERC finding shape for
  `i2c_sensor_breakout_candidate` in a focused regression.
- Identify which generated schematic objects produce the ERC connectivity
  failures.
- Emit schematic wires, labels, junctions, and no-connect markers on KiCad's
  expected grid.
- Preserve left-to-right readability and existing schematic-layout rules.
- Keep PCB routing, net aliases, route-tree contact proof, writer correctness,
  and structural validation passing.
- Promote I2C metadata to `candidate` only after required ERC no longer blocks
  candidate readiness.

## Non-Goals

- Do not require real local KiCad in default `go test ./...`.
- Do not claim `pass` readiness unless KiCad ERC and DRC evidence are clean
  under the configured pass policy.
- Do not weaken design-example metadata validation to allow expected-fail cases
  to pass silently.
- Do not remove route-tree evidence or convert I2C back to fallback net routing.
- Do not solve the protected Class AB amplifier schematic conflicts in this
  project.

## Current Evidence

Current `i2c_sensor_breakout_candidate` evidence:

- `block_planning`: ok
- `component_selection`: warning-only, expected catalog gaps
- `schematic`: ok
- `schematic_electrical`: ok
- `pcb_realization`: ok
- `placement`: ok
- `routing`: ok
- `project_write`: ok
- `writer_correctness`: warning-only, expected library/root gaps
- `validation`: ok with optional external KiCad checks skipped
- `kicad_checks`: blocked by required KiCad ERC schematic connectivity

The implementation must maintain or improve this stage shape. Any regression in
project-write, writer-correctness, validation, or route-tree completion is a
failure unless the new failure exposes a stricter correctness issue and the
metadata is updated accordingly.

## Required Behavior

### 1. ERC Finding Capture

The workflow must expose enough structured KiCad ERC evidence to identify:

- failing rule/message;
- schematic object path or UUID when available;
- reference/pin/net context when available;
- generated source provenance when available;
- whether the finding blocks candidate or only pass.

When using a fake KiCad runner, fixture tests must reproduce the blocker shape
deterministically. When using a real KiCad CLI locally, optional smoke tests may
record the real report path and summary.

### 2. Grid-Safe Schematic Geometry

Generated schematic wires, labels, junctions, and no-connect markers must align
to the connection grid used by the writer and KiCad ERC. Long or generated alias
nets may still use labels, but label stubs must be real connected KiCad
schematic geometry:

- no off-grid wire endpoints;
- no isolated labels;
- no stubs that intersect unrelated nets;
- no dangling wire endpoint unless intentionally marked;
- no generated local labels that create conflicting same-segment labels.

### 3. Alias And Exported Net Semantics

Block-composed nets such as `VCC`, `GND`, `SDA`, and `SCL` must be represented
consistently across:

- schematic symbol pin nets;
- generated schematic wires/labels;
- transaction net aliases;
- PCB pad and route nets;
- project-write/readback evidence.

The schematic writer must not rely on PCB-only alias metadata to satisfy KiCad
ERC. If a net is a real schematic connection, the `.kicad_sch` output must make
that connection clear to KiCad.

### 4. Intentional Non-Connections

Any unused generated symbol pin must be intentionally handled:

- connect it to an appropriate net;
- add a KiCad no-connect marker on the exact pin anchor; or
- model the symbol/pin metadata so the pin is not emitted as a required
  electrical connection.

No-connect markers must not be used to hide real required I2C or power
connections.

### 5. Promotion Metadata

When KiCad ERC no longer blocks candidate readiness:

- change `i2c_sensor_breakout_candidate.metadata.json` readiness to
  `candidate`;
- remove stale KiCad ERC known gaps;
- document remaining pass-level evidence, usually KiCad DRC and broader
  rich-board coverage;
- update `examples/design/kicad-backed/README.md`, `README.md`,
  `docs/layout-routing.md`, and `specs/ROADMAP.md`.

If ERC remains blocked after implementation, metadata must stay
`expected_fail`, but the known gaps must name the exact remaining ERC findings
and generated source locations.

## Design Constraints

- Default tests must remain KiCad-independent.
- Real KiCad CLI tests must remain opt-in.
- Fake KiCad runner tests must cover promotion-gate behavior.
- Schematic writer changes must preserve round-trip compatibility and unknown
  node preservation.
- Schematic readability rules still apply:
  - inputs on the left;
  - outputs on the right;
  - higher voltage rails upward;
  - lower rails and ground downward;
  - components spread enough for labels and wires to remain readable.
- Do not introduce ad hoc string rewriting of KiCad files when structured
  writer APIs exist.

## Acceptance Criteria

- Focused I2C design-example tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|ERC|Promotion|DesignExamples' -count=1
  ```

- Full tests pass:

  ```sh
  go test ./...
  ```

- The generated I2C schematic no longer reports candidate-blocking KiCad ERC
  connectivity findings in the configured fake/real KiCad evidence path.
- I2C route-tree summaries still show:
  - all 8 route-tree branches emitted;
  - all required contacts proven;
  - VCC/GND/SDA/SCL graph-complete.
- `project_write`, `writer_correctness`, and `validation` remain at their
  current accepted status.
- Fixture metadata and docs match achieved readiness.

## Risks

- Fixing ERC by adding more wires may create schematic label conflicts or reduce
  readability.
- Shortening or moving label stubs may improve ERC while breaking net alias
  semantics.
- Symbol pin models may hide real connectivity if no-connect logic is too broad.
- Real KiCad ERC may report additional library-symbol warnings after the first
  connectivity findings are resolved.

## Deliverables

- Focused regression tests for the current KiCad ERC blocker shape.
- Schematic writer/layout fixes for ERC-accepted generated connections.
- Metadata/docs update reflecting the achieved readiness.
- Prism-reviewed commits for each implementation phase.
