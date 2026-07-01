# LED Indicator KiCad-Backed Promotion Specification

## 1. Purpose

Promote `examples/design/kicad-backed/led_indicator_kicad_smoke` from
`expected_fail` to `candidate`.

This is the next smallest KiCad-backed design fixture after
`connector_led_kicad_smoke`. Promoting it gives KiCadAI a single-block
generated design that proves the full path from block intent to KiCad-accepted
schematic, PCB net assignment, block-local routing, writer correctness,
promotion reporting, and optional KiCad ERC/DRC evidence.

## 2. Background

`connector_led_kicad_smoke` is now a `candidate` fixture. That promotion
established several foundations:

- KiCad-native PCB net declarations are emitted.
- PCB pads render `(net code "name")`.
- tracks, arcs, vias, and zones render numeric net references where KiCad
  expects them.
- exact embedded symbol bodies for `Device:R`, `Device:LED`, and
  `Connector_Generic:Conn_01x02` eliminate schematic ERC
  `lib_symbol_mismatch` warnings for those seed symbols.
- KiCad DRC local tool instability with no parsed findings is warning-only
  when it matches the observed no-output exit-code `-1` DRC crash case.
- promotion reports can represent candidate readiness with warning gates.

The LED fixture still has stale metadata that says it is expected to fail
because KiCad evidence was missing. That is no longer sufficient. The fixture
should either promote or expose a current, concrete blocker.

## 3. Target Fixture

Request:

```text
examples/design/kicad-backed/led_indicator_kicad_smoke.json
```

Metadata:

```text
examples/design/kicad-backed/led_indicator_kicad_smoke.metadata.json
```

Current request summary:

- one `led_indicator` block;
- 40 mm x 25 mm two-layer board;
- ERC/DRC acceptance;
- `require_erc: true`;
- `require_drc: true`;
- currently `skip_routing: true`.

## 4. Goals

- Enable routing for the LED smoke fixture unless a current blocker proves it
  must remain skipped.
- Ensure the generated LED project writes:
  - `.kicadai/transaction.json`;
  - `.kicadai/manifest.json`;
  - `.kicadai/design-promotion.json`;
  - `.kicad_pro`;
  - `.kicad_sch`;
  - `.kicad_pcb`.
- Ensure the generated schematic passes KiCad ERC when `KICADAI_KICAD_CLI` is
  configured.
- Ensure the generated PCB has KiCad-native net declarations and no internal
  pad/copper net assignment blockers.
- Ensure block-local LED route evidence proves physical same-net endpoint
  contact between resistor and LED pads.
- Promote metadata readiness from `expected_fail` to `candidate` only if
  promotion achieved readiness is `candidate` or better.
- Keep normal `go test ./...` independent of KiCad.
- Preserve warning evidence for the local KiCad DRC no-finding crash if it
  still occurs.

## 5. Non-Goals

- Do not make the LED fixture a required `pass` fixture yet.
- Do not solve all KiCad DRC tool instability.
- Do not broaden block-library variants.
- Do not change the I2C or amplifier fixtures.
- Do not require real KiCad for default tests.
- Do not introduce user-project mutation.

## 6. Candidate Acceptance Criteria

The LED fixture may be marked `candidate` when all of the following are true:

- Metadata validates for `candidate` readiness.
- Expected artifacts include `.kicadai/design-promotion.json`.
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
- Workflow JSON reports `ok: true`.
- `data.acceptance.achieved` is at least `erc-drc`.
- promotion summary reports:
  - `declared_readiness: "candidate"`;
  - `achieved_readiness: "candidate"` or `pass`;
  - `matches_expectation: true`.
- KiCad ERC returns zero findings when real KiCad checks run.
- DRC warnings are non-blocking only when they are:
  - true KiCad warning findings; or
  - the already-observed DRC tool crash with no stdout, no stderr, no parsed
    findings, and exit code `-1`.
- Internal board validation reports a pass-level board status.
- Writer correctness has no blocking checks.
- LED local route/contact evidence proves required same-net pad contact.

## 7. Expected Investigation Areas

### 7.1 Routing Skip

The fixture currently uses:

```json
"skip_routing": true
```

Promotion should attempt to remove this. If the request still needs routing
skip, the fixture should stay `expected_fail` and metadata should name the exact
remaining route blocker.

### 7.2 Component Selection Warning

The LED block may still use generic or policy-allowed component records. This
can remain a candidate warning if the selected resistor and LED have sufficient
symbol, footprint, pinmap, and value evidence for this smoke fixture.

### 7.3 KiCad Footprint Mismatch

If KiCad DRC reports footprint library mismatch warnings for generated
footprints, the promotion gate may remain warning-level for `candidate`, but the
report must preserve those findings. Do not hide or globally allowlist them.

### 7.4 DRC Tool Instability

The local KiCad 10.0.3 CLI currently crashes on some generated and checked-in
boards during `pcb drc` while producing no parsed findings. This is acceptable
for `candidate` only through the narrow DRC no-finding crash classifier already
implemented. Any parser issue, stdout/stderr evidence, non-DRC tool error, or
different exit code must remain blocking unless separately justified.

## 8. Metadata Updates

If promotion succeeds, update:

```text
examples/design/kicad-backed/led_indicator_kicad_smoke.metadata.json
```

Required changes:

- `readiness`: `candidate`;
- `expected_artifacts`: add `.kicadai/design-promotion.json`;
- `expected_stages`: include `schematic_electrical` and `routing`;
- `known_gaps`: replace stale expected-fail language with current
  non-blocking candidate warnings;
- `notes`: describe what the fixture now proves.

## 9. Test Requirements

Required tests:

- `go test ./internal/designworkflow`;
- `go test ./internal/blocks ./internal/kicadfiles/designapi ./internal/kicadfiles/schematic ./internal/kicadfiles/pcb`;
- `go test ./...`.

Manual/local command:

```text
kicadai --request examples/design/kicad-backed/led_indicator_kicad_smoke.json --output examples/.generated/led_indicator_kicad_smoke --overwrite design create
```

The generated output directory remains ignored and must not be committed.

## 10. Done Definition

This work is complete when:

- `led_indicator_kicad_smoke` is promoted to `candidate`, or the spec is amended
  with a concrete current blocker found during implementation;
- all stale expected-fail metadata for the LED fixture is removed or made
  current;
- generated project output proves ERC-clean schematic evidence locally when
  KiCad is configured;
- full Go tests pass;
- Prism review has no unresolved high or medium findings;
- changes are committed.
