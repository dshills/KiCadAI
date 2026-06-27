# Design Example Regression Specification

Date: 2026-06-27

## Summary

KiCadAI's checked-in `examples/design` requests must remain executable
end-to-end examples for the current `design create` workflow. They are the
quickest path for a user or AI agent to understand whether structured
block-based generation is working, so they must be treated as regression
fixtures rather than static documentation.

The immediate trigger for this work is that both current design examples have
drifted from the active workflow:

- `examples/design/led_indicator.json` uses the old `resistor_ohms` parameter,
  which the current `led_indicator` block rejects.
- `examples/design/sensor_breakout.json` connects `sensor.INT`, but the current
  generated planning path blocks on that port in the example workflow.

The goal is to make every checked-in design example load, validate, generate
KiCad files, and expose useful workflow evidence without requiring a running
KiCad GUI.

## Goals

- Keep `examples/design/*.json` aligned with the current `designworkflow`
  request schema and built-in block parameter contracts.
- Add automated coverage that runs all design examples through `design create`
  in temporary output directories.
- Confirm generated examples include the core artifacts users expect:
  `.kicad_pro`, `.kicad_sch`, `.kicad_pcb`, `.kicadai/workflow-result.json`,
  and validation/feedback evidence.
- Ensure examples exercise current important capabilities:
  component selection, schematic identity properties, PCB realization,
  placement, routing or explicit skip-routing behavior, and validation
  summaries.
- Keep README/docs commands truthful and runnable with the compiled `kicadai`
  binary.

## Non-Goals

- This does not expand general natural-language design synthesis.
- This does not require live KiCad IPC mutation.
- This does not require `kicad-cli` to be installed for default example
  regression tests.
- This does not guarantee production fabrication readiness for every example.
  Fabrication-candidate examples may be added later as a separate fixture tier.

## Current Example Contract

All files under `examples/design/*.json` are public examples. Each example must:

- decode with `designworkflow.DecodeRequestStrict`;
- pass `designworkflow.ValidateRequest`;
- use only block IDs present in `blocks.NewBuiltinRegistry`;
- use only parameters declared by the selected block definitions;
- connect only ports exported by each instantiated block after parameters and
  defaults are applied;
- complete `design create` without blocking issues under its declared
  acceptance level;
- write to a caller-provided output directory without relying on the current
  working directory;
- produce deterministic output for a fixed request and clean output directory.

## Example Tiers

Examples should be grouped by the level of workflow they are meant to prove.

### Structural Examples

Structural examples prove schematic and PCB file generation without requiring
complete routing success.

Expected properties:

- `validation.acceptance` is `structural`;
- routing may be skipped only when the example exists to demonstrate schematic
  and component identity behavior;
- generated schematic contains expected symbols, nets, and selected component
  identity properties where component evidence exists;
- generated PCB exists and parses with the internal PCB reader;
- writer correctness passes internal checks.

The LED indicator example belongs in this tier unless it is upgraded to a
connectivity example.

### Connectivity Examples

Connectivity examples prove the generated board is electrically meaningful
under internal validation.

Expected properties:

- `validation.acceptance` is `connectivity`;
- block planning, component selection, schematic generation, PCB realization,
  placement, routing, project write, writer correctness, and validation stages
  complete without blocking issues;
- strict unrouted behavior may be enabled only if the current routing engine can
  complete the fixture deterministically;
- generated validation evidence reports no disconnected pads or invalid net
  assignments.

The I2C sensor breakout example should live in this tier once its block ports
and connector ports are aligned.

### Optional KiCad-Backed Examples

Optional examples may require `kicad-cli` for ERC/DRC or round-trip proof.
Those examples must be skipped by default when `kicad-cli` is unavailable and
must clearly expose the skip reason in test output.

This tier is not required for the first implementation.

## Fixture Metadata

Each design example should have machine-readable expectations. The first
implementation may either embed conventions in test code or add adjacent
metadata files later. The desired expectation model is:

```json
{
  "request": "led_indicator.json",
  "acceptance": "structural",
  "expected_project_name": "led_indicator",
  "required_artifacts": [
    ".kicad_pro",
    ".kicad_sch",
    ".kicad_pcb",
    ".kicadai/workflow-result.json"
  ],
  "required_stages": [
    "block_planning",
    "component_selection",
    "schematic",
    "pcb_realization",
    "placement",
    "routing",
    "project_write",
    "writer_correctness",
    "validation"
  ],
  "allow_skipped_stages": ["kicad_checks"],
  "required_symbol_properties": ["KiCadAI Component ID"]
}
```

The first phase can hard-code these expectations in Go tests to avoid adding
metadata churn. A later phase may promote them to JSON if examples grow.

## CLI Behavior

The canonical command shape is:

```sh
kicadai --request examples/design/led_indicator.json --output /tmp/kicadai-led --overwrite design create
```

JSON output is the default. Documentation should not require `--json`, though
the compatibility alias may remain mentioned in the CLI reference.

Example commands must not use `go run ./cmd/kicadai`.

## Validation Requirements

The regression suite must validate at three levels.

### Request-Level Validation

Tests must decode each request strictly and fail on:

- unknown JSON fields;
- unsafe names or output assumptions;
- missing board dimensions;
- unknown block IDs;
- unknown block parameters;
- invalid block parameter types;
- unknown connection endpoints;
- duplicate or incompatible connector port names.

### Workflow-Level Validation

Tests must run the workflow in a temporary directory and fail on blocking
issues for the example's declared acceptance level.

The test must inspect the structured workflow result, not only the process exit
code. The workflow should expose stage status, issue summaries, artifact paths,
and feedback.

### File-Level Validation

Generated files must be read back with internal readers:

- `.kicad_sch` through `internal/kicadfiles/schematic`;
- `.kicad_pcb` through `internal/kicadfiles/pcb`;
- `.kicad_pro` through existing project/file checks where available.

At least one example with component evidence must assert hidden schematic
identity properties are present on generated symbols.

## Documentation Requirements

`examples/design/README.md` must document:

- what each example demonstrates;
- the expected command to run it;
- whether it routes or intentionally skips routing;
- whether optional KiCad checks are required;
- the main generated artifacts to inspect.

README snippets that reference examples must point to examples that currently
pass the default regression suite.

## Failure Reporting

When an example fails, test output should make the cause obvious:

- request path;
- output directory;
- first blocking stage;
- blocking issue code/path/message;
- missing artifact path if applicable.

This matters because these examples are also intended for AI agents. Failures
should be diagnosable from structured data without opening KiCad manually.

## Acceptance Criteria

- `examples/design/led_indicator.json` runs successfully through
  `design create` under its declared acceptance level.
- `examples/design/sensor_breakout.json` runs successfully through
  `design create` under its declared acceptance level, or its acceptance level
  is intentionally adjusted and documented with a non-blocking rationale.
- A Go regression test enumerates all `examples/design/*.json` files and runs
  them through strict decode, request validation, workflow generation, and
  artifact checks.
- Generated example schematics include selected component identity properties
  when the workflow has component selection evidence.
- Documentation uses the compiled `kicadai` binary and default JSON behavior.
- `go test ./...` passes.

## Risks

- Running full design generation for every example can slow the test suite if
  examples grow too large. Keep examples small and deterministic.
- If examples depend on optional external KiCad tooling by default, they will
  become fragile in CI and local development. Keep default examples internal
  unless explicitly marked optional.
- If examples are only validated by docs snippets, they will drift again.
  Automated regression coverage is required.

