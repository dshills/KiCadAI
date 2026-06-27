# Design Example Regression Implementation Plan

Date: 2026-06-27

## Phase 1: Audit Current Example Contracts

Goal: identify the exact mismatch between checked-in examples and current block
definitions before editing fixtures.

Tasks:

- Add a small test or helper that enumerates `examples/design/*.json`.
- Decode each request with `designworkflow.DecodeRequestStrict`.
- Validate each request against `blocks.NewBuiltinRegistry`.
- Report all blocking request and block-planning issues with request path,
  issue path, and issue message.
- Confirm the current expected failures:
  - `led_indicator.json` rejects `params.resistor_ohms`;
  - `sensor_breakout.json` rejects or blocks an endpoint involving
    `sensor.INT` or equivalent drift.

Review:

- `go test ./internal/designworkflow -run TestDesignExamples`
- `prism review staged`

Commit:

- `Add design example contract audit`

## Phase 2: Refresh Checked-In Example Requests

Goal: update public examples to use the current request schema and block
contracts.

Tasks:

- Replace stale LED parameters with the current `led_indicator` parameter names
  and value formats.
- Align the sensor breakout connector pins and connections with the active
  `i2c_sensor` and `connector_breakout` ports.
- If the sensor interrupt output is not supported end-to-end in the current
  workflow, remove it from the default example and document it as future
  expansion rather than leaving a blocking request.
- Ensure examples declare acceptance levels that match what the current engine
  can prove:
  - LED: `structural`, with `skip_routing` only if routing is intentionally not
    the point of the example.
  - Sensor breakout: `connectivity` only if placement/routing/validation can
    complete deterministically; otherwise use `structural` and document the
    limitation.
- Run each example manually through:
  - `kicadai --request examples/design/led_indicator.json --output <tmp> --overwrite design create`
  - `kicadai --request examples/design/sensor_breakout.json --output <tmp> --overwrite design create`

Review:

- `go test ./internal/designworkflow -run TestDesignExamples`
- Manual CLI runs for both examples.
- `prism review staged`

Commit:

- `Refresh design workflow examples`

## Phase 3: Add End-To-End Example Regression Tests

Goal: prevent examples from drifting again.

Tasks:

- Add a Go regression test that enumerates every `examples/design/*.json` file.
- For each request:
  - decode strictly;
  - validate request fields;
  - run `designworkflow.Create` or equivalent orchestration in a temporary
    output directory;
  - fail on blocking issues under the request's declared acceptance level;
  - assert expected artifacts exist:
    - `<name>.kicad_pro`;
    - `<name>.kicad_sch`;
    - `<name>.kicad_pcb`;
    - `.kicadai/workflow-result.json`.
- Read generated schematic and PCB files back with internal readers.
- Assert at least one component-selected example contains hidden schematic
  identity properties such as `KiCadAI Component ID`.
- Keep tests independent of `kicad-cli` by default.

Review:

- `go test ./internal/designworkflow`
- `go test ./...`
- `prism review staged`

Commit:

- `Add design example regression tests`

## Phase 4: Improve Failure Diagnostics

Goal: make example failures useful for humans and AI agents.

Tasks:

- Add a helper that formats workflow failures as:
  - example path;
  - output directory;
  - first blocked stage;
  - issue code;
  - issue path;
  - issue message;
  - suggestion when present.
- Use this helper in the example regression test.
- Include missing artifact paths in assertion failures.
- If useful, persist compact test diagnostics under `t.Logf` rather than
  writing files into the repository.

Review:

- Force or preserve one narrow negative unit test for diagnostic formatting.
- `go test ./internal/designworkflow`
- `prism review staged`

Commit:

- `Improve design example regression diagnostics`

## Phase 5: Update Documentation

Goal: make public docs match the executable examples.

Tasks:

- Update `examples/design/README.md` with:
  - one section per example;
  - exact compiled-binary command;
  - expected acceptance level;
  - whether routing is run or skipped;
  - generated artifacts to inspect;
  - known limitations.
- Update top-level `README.md` if it references a stale design example command
  or implies stronger behavior than the examples prove.
- Update `docs/kicadai-agent-skill.md` if its workflow guidance points agents
  at examples that no longer pass.
- Confirm no current README/docs snippet uses `go run ./cmd/kicadai`.

Review:

- `rg "go run ./cmd/kicadai|go run" README.md docs examples -n`
- `go test ./...`
- `prism review staged`

Commit:

- `Document runnable design examples`

## Phase 6: Optional KiCad-Backed Example Tier

Goal: prepare for future examples that require KiCad CLI evidence without
making the default suite fragile.

Tasks:

- Decide whether optional KiCad-backed examples should live under
  `examples/design/kicad-backed` or be annotated by metadata.
- Add a skipped-by-default test hook that runs only when a `kicad-cli` path is
  explicitly configured.
- Require optional examples to report skip reasons when tooling is unavailable.
- Do not add mandatory external-tool requirements to default `go test ./...`.

Review:

- `go test ./...`
- Optional local run with `kicad-cli` if available.
- `prism review staged`

Commit:

- `Prepare optional KiCad-backed design examples`

## Phase 7: Roadmap And Status Update

Goal: reflect that examples are now executable regression fixtures.

Tasks:

- Update `specs/ROADMAP.md` current-state or remaining-work bullets to mention
  runnable `design create` example regression coverage.
- Add any newly discovered limitations to the roadmap rather than hiding them
  in test comments.
- Confirm the working tree is clean after commits.

Review:

- `go test ./...`
- `prism review staged`

Commit:

- `Update roadmap for design example regression`

## Acceptance Checklist

- All `examples/design/*.json` requests pass strict decode and validation.
- All default design examples run through `design create` without blocking
  issues.
- Generated project artifacts are verified by automated tests.
- At least one generated schematic is checked for component identity
  properties.
- Docs use compiled `kicadai` commands and describe realistic current behavior.
- Optional KiCad-backed checks remain optional.
- `go test ./...` passes.

