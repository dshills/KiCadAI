# KiCad-Backed Design Promotion Audit

## Scope

This audit records the baseline state for optional KiCad-backed generated design
fixtures before promotion infrastructure is implemented. The fixtures live in
`examples/design/kicad-backed/` and are exercised by
`TestDesignExamplesOptionalKiCadBackedTier` when `KICADAI_KICAD_CLI` is set.

Default `go test ./...` must remain KiCad-independent.

## Fixture Inventory

| Fixture | Request | Tier | Acceptance | Readiness | ERC | DRC |
| --- | --- | --- | --- | --- | --- | --- |
| `connector_led_kicad_smoke` | `connector_led_kicad_smoke.json` | `block-composition` | `erc-drc` | `expected_fail` | required | required |
| `i2c_sensor_breakout_candidate` | `i2c_sensor_breakout_candidate.json` | `block-composition` | `erc-drc` | `expected_fail` | required | required |
| `led_indicator_kicad_smoke` | `led_indicator_kicad_smoke.json` | `smoke` | `erc-drc` | `expected_fail` | required | required |
| `opamp_headphone_buffer_kicad_candidate` | `opamp_headphone_buffer_kicad_candidate.json` | `fabrication` | `fabrication-candidate` | `expected_fail` | required | required |

## Existing Metadata Coverage

Each request file has a sibling metadata file in the same directory using the
current `foo.json` and `foo.metadata.json` convention.

Each fixture already declares:

- `id`;
- `request`;
- `tier`;
- `readiness`;
- `acceptance`;
- `require_erc`;
- `require_drc`;
- `allowlists`;
- `expected_artifacts`;
- `expected_stages`;
- `known_gaps`;
- `notes`.

The existing loader validates basic enum values, local request filenames,
required acceptance, explicit ERC/DRC booleans, and non-empty expected stages.
It does not yet require `known_gaps` for `expected_fail`, validate expected
artifact paths, or produce a normalized promotion report.

## Current Expected Artifacts

The three non-amplifier fixtures currently expect:

- `.kicadai/transaction.json`;
- `.kicadai/manifest.json`.

The amplifier fixture expects no artifacts because it should fail closed during
early planning/component evidence stages before project output is trustworthy.

## Current Expected Stages

`led_indicator_kicad_smoke`, `connector_led_kicad_smoke`, and
`i2c_sensor_breakout_candidate` expect the full generated-design path:

- `block_planning`;
- `component_selection`;
- `schematic`;
- `pcb_realization`;
- `placement`;
- `routing`;
- `project_write`;
- `writer_correctness`;
- `validation`;
- `kicad_checks`.

`opamp_headphone_buffer_kicad_candidate` expects only:

- `block_planning`;
- `component_selection`.

That shorter stage list is intentional because fabrication-candidate amplifier
evidence should fail closed before schematic/PCB output is treated as useful.

## Fixture-Specific Baseline

### `led_indicator_kicad_smoke`

Purpose:

- smallest optional real-KiCad design-create smoke fixture;
- tracks generated project output, writer correctness, local route endpoint
  binding, and eventual ERC/DRC-clean promotion.

Known blocker:

- full internal validation plus optional KiCad ERC/DRC evidence must prove the
  generated schematic and PCB are clean before promotion.

Current promotion risk:

- parseable output is not enough; promotion must prove electrical connectivity
  and KiCad-clean artifacts.

### `connector_led_kicad_smoke`

Purpose:

- tracks multi-block connector-to-LED composition;
- exercises local LED route binding and inter-block route-contact diagnostics.

Known blocker:

- the `LED_EN` inter-block path has contact-miss evidence and is not yet a
  graph-connected required route.

Current promotion risk:

- route candidate discovery or route emission could be mistaken for complete
  copper connectivity. Promotion must require same-net contact graph
  completion.

### `i2c_sensor_breakout_candidate`

Purpose:

- tracks richer sensor breakout generation with connector, sensor, pullups, and
  promoted VCC/GND/SDA/SCL inter-block route candidates.

Known blocker:

- routed same-net completion and KiCad DRC evidence are still pending.

Current promotion risk:

- the fixture name contains `candidate`, but metadata readiness is still
  `expected_fail`. The promotion runner must treat metadata readiness as the
  source of truth.

### `opamp_headphone_buffer_kicad_candidate`

Purpose:

- keeps amplifier-specific fabrication-candidate blockers visible in the
  optional KiCad-backed tier.

Known blockers:

- verified amplifier component evidence is missing;
- output DC-blocking/protection realization is incomplete;
- analog layout proof is missing;
- KiCad ERC/DRC promotion evidence is missing.

Current promotion risk:

- this fixture must not promote based on schematic or PCB parseability. It
  needs component, safety, layout, and fabrication-candidate evidence first.

## Current Test Harness Behavior

`TestDesignExamplesOptionalKiCadBackedTier`:

- skips unless `KICADAI_KICAD_CLI` is configured;
- loads each metadata file;
- skips `blocked` fixtures;
- runs `Create` with ERC/DRC requirements from metadata;
- asserts expected stages and artifacts;
- treats `pass` and `candidate` as requiring `kicad_checks: ok`;
- treats `expected_fail` as requiring blocked evidence.

The harness does not yet:

- emit a stable promotion report;
- classify achieved readiness independently from declared readiness;
- normalize internal evidence gates;
- record skipped KiCad evidence as a reportable gate;
- include repair guidance for promotion blockers.

## Baseline Conclusion

The next implementation should not start by changing fixture readiness. The
right first step is to add strict metadata validation and a promotion report
model, then layer internal and optional KiCad evidence gates over the existing
test harness. Fixture readiness should change only after the new report proves
that a fixture satisfies the candidate or pass gates.
