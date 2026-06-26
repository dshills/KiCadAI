# CLI Reference

Command examples and live KiCad IPC setup details for the compiled `kicadai` binary.

## KiCad IPC Setup

Live IPC commands require KiCad to be running with the API enabled. Open the
project/editor you want to inspect, then run:

```sh
kicadai --json config
kicadai --json ping
kicadai --json version
kicadai --json documents
kicadai --json capabilities
```

Connection flags:

```sh
kicadai \
  --socket ipc:///tmp/kicad/api.sock \
  --token "$KICAD_API_TOKEN" \
  --timeout-ms 5000 \
  --json ping
```

Environment variables:

```sh
export KICAD_API_SOCKET=ipc:///tmp/kicad/api.sock
export KICAD_API_TOKEN=your-token-if-required
export KICAD_CLIENT_NAME=kicadai-dev
export KICAD_TIMEOUT_MS=5000
```

Configuration precedence is flag first, environment second, platform default
last. Tokens are redacted from CLI output. If no token is configured, the client
captures KiCad's returned token in memory for that process only.


## CLI Overview

All structured project-analysis and generation commands currently require
`--json`.

```sh
kicadai --json config
kicadai --json ping
kicadai --json version
kicadai --json documents
kicadai --json capabilities
kicadai --json inspect project ./examples/07_generated_pcb
kicadai --json evaluate project ./examples/07_generated_pcb
kicadai --json writer check ./examples/07_generated_pcb
kicadai --json validate board ./examples/07_generated_pcb
kicadai --json check erc ./examples/checks/erc_fail/erc_fail.kicad_sch
kicadai --json check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
kicadai --json component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai --json pinmap list
kicadai --json pinmap validate ./examples/01_led_indicator
kicadai --json --request ./examples/placement/simple_request.json place request
kicadai --json --request ./examples/routing/simple_request.json route request
kicadai --json --request ./examples/repair/missing_footprint_stage_issues.json repair plan
kicadai --json --request ./examples/intent/sensor_breakout.json --output ./out/intent_plan --overwrite intent plan
kicadai --json --request ./examples/intent/sensor_breakout.json intent explain
kicadai --json --text "make a 3.3V I2C temperature sensor breakout" intent rationale
kicadai --json --request ./examples/intent/sensor_breakout.json --output ./out/intent_sensor --overwrite intent create
kicadai --json --target ./out/project --request ./examples/repair/missing_footprint_stage_issues.json repair export-bundle
kicadai --json --execute --overwrite --target ./out/project --request ./examples/repair/missing_footprint_stage_issues.json repair export-bundle
# For integrations that already produce a generated repair bundle:
kicadai --json --execute --overwrite --target ./out/project --request ./path/to/generated-repair-bundle.json repair apply
# Generate/apply a repair bundle during design create, then replay that saved
# bundle later for reproducible target-apply validation:
kicadai --json --request ./examples/design/led_indicator.json --output ./out/led_indicator --overwrite --repair-apply --skip-routing design create
kicadai --json --execute --overwrite --target ./out/led_indicator --request ./out/led_indicator/.kicadai/repair-bundle.json repair apply
kicadai --json --feedback transaction validate ./examples/transactions/invalid_feedback.json
```

### Live IPC Commands

- `config`: print resolved connection configuration.
- `ping`: check whether KiCad responds.
- `version`: print KiCad version information.
- `documents`: list open KiCad documents.
- `capabilities`: report detected API command support.
- `plan-led-demo`: print a deterministic LED schematic action plan.
- `draw-led-demo --execute`: attempts live schematic automation after
  capability preflight, but currently returns a structured blocked result when
  schematic write commands are not available.

### Direct Generation Commands

- `generate-led-demo` and `generate-project`: generate a deterministic LED
  indicator project, with optional PCB output.
- `design create`: run the AI design workflow from explicit circuit-block
  intent through schematic/PCB write, placement, routing, optional bounded
  placement-routing retry, validation, optional repair planning or apply, and
  feedback.
- `writer check`: verify generated project, schematic, PCB, net, footprint,
  pad, copper, zone, and optional KiCad round-trip writer correctness.
- `generate breakout`: generate a connector breakout project from a structured
  JSON request.
- `block realize-pcb <block_id>`: instantiate a circuit block and return its
  PCB realization fragment plus the placement request that can feed the
  placement engine.
- `component list|show|find|select|validate`: inspect the curated component
  catalog, choose symbol/footprint bindings, and enforce confidence gates.
- `repair plan|export-bundle|apply`: classify stage issues, package generated
  target repair evidence, and emit deterministic repair attempts.
  `repair export-bundle` accepts stage issue JSON via `--request` plus
  `--target`, defaults to a dry run, and writes
  `.kicadai/repair-bundle.json` only with `--execute`.
  With `--target` and a repair bundle, `repair apply` with `--execute` and
  `--overwrite` persists safe transaction-level repairs back through the
  generated KiCad writer and can run built-in post-repair validators for writer
  correctness, board validation, ERC/DRC, and round-trip evidence when the
  corresponding validation flags are enabled.

Results for the `repair apply` command include:

- `validation`: the transaction check plus enabled post-repair adapters.
- `summary`: adapter count, skipped count, issue totals, blocking counts, and
  artifacts.
- `delta`: before/after issue summaries with cleared, repeated, and new issues.
- `normalized_findings`: AI-facing post-apply findings with source, category,
  repairability, subject, stable key, and evidence path fields.
- `convergence`: normalized before/after convergence with cleared, repeated,
  new, worsened, and stop-reason counts.
- `budget`: normalized retry limits, attempt count, and exhaustion status.

Post-repair statuses are intended for AI callers:

- `repaired`: required validation is clean after apply.
- `partial`: blocking issues are absent, but non-blocking warnings or skipped
  optional evidence remain.
- `blocked`: required validation still has blocking issues, a required external
  validator is unavailable, or safety policy prevents mutation.

KiCad-backed post-repair evidence is opt-in. `--require-drc` makes missing DRC
evidence blocking. `--allow-missing-drc` requests optional DRC evidence and
keeps a missing `kicad-cli` visible as a warning. Default tests use fake or
missing CLI paths and do not require a local KiCad install. By default, KiCad
DRC is skipped unless `--require-drc` or `--allow-missing-drc` is provided.

When `design create` runs the persisted `validation_repair` stage, it writes a
repair bundle artifact at `.kicadai/repair-bundle.json` under the generated
project directory. That bundle can be passed back via `--request` to
`repair apply --target` for reproducible generated-project repair.

New generated projects also include `.kicadai/transaction.json`, and
`.kicadai/manifest.json` records a hash for that provenance file.
Target-oriented repair commands use that evidence to reconstruct the original
transaction when a caller does not provide one. Targets with missing, stale, or
malformed metadata, plus imported or unsupported targets, remain blocked before
mutation.

For non-`design create` flows, `repair export-bundle` packages structured
stage issues against a generated target:

```sh
kicadai \
  --json \
  --target ./out/led_indicator \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair export-bundle

kicadai \
  --json \
  --execute \
  --overwrite \
  --target ./out/led_indicator \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair export-bundle
```

Export is generated-target-only and refuses output paths outside the target
root. For valid generated targets, export hydrates transaction provenance and
reports `summary.has_transaction=true`, so the exported bundle can feed
`repair apply`. Legacy generated projects without `.kicadai/transaction.json`
must be regenerated or supplied with a repair bundle that already contains the
transaction.

LED generation:

```sh
kicadai \
  --json \
  --output ./out/led_indicator \
  --name led_indicator \
  --with-pcb \
  --overwrite \
  generate-project
```

Breakout generation request:

```json
{
  "kind": "breakout",
  "name": "sensor_breakout",
  "board": { "width_mm": 40, "height_mm": 25 },
  "connectors": [
    { "ref": "J1", "pins": ["VCC", "GND", "SDA", "SCL"] },
    { "ref": "J2", "pins": ["VCC", "GND", "SDA", "SCL"] }
  ],
  "ground_zone": true
}
```

Run it:

```sh
kicadai \
  --json \
  --request ./request.json \
  --output ./out/sensor_breakout \
  --overwrite \
  generate breakout
```

Generated projects are written through safe directory handling. `--overwrite`
is required to replace an existing output directory.

`design create` requests can opt into bounded placement-routing retry with a
`routing_retry` object. Retry is disabled by default. When enabled, `max_attempts`
is the total number of placement/routing attempts, including the initial
attempt:

```json
{
  "routing_retry": {
    "enabled": true,
    "max_attempts": 2,
    "allowed_hint_categories": ["increase_spacing", "improve_fanout", "reduce_distance"]
  }
}
```

Retry summaries are returned in the routing stage. They include attempt count,
applied adjustment count, stop reason, selected attempt, selected reason, hint
categories, and compact attempt history.

Attempt history includes:

- the baseline attempt plus each retry attempt;
- routed, failed, and skipped net counts;
- route and placement scores;
- board validation counters when available;
- DRC evidence status and source.

Retry DRC evidence is skipped unless requested by `routing_retry.drc_policy`;
optional or required real KiCad evidence is wired through the same adapter used
by local smoke tests.

Runnable example:

```sh
kicadai \
  --json \
  --request ./examples/design_retry/placement_routing_retry.json \
  --output ./out/placement_routing_retry_demo \
  --overwrite \
  design create
```

Supported placement retry hint categories are `increase_spacing`,
`improve_fanout`, `reduce_distance`, and `move_from_edge`. Routing-rule and
unsupported-zone diagnostics are reported, but placement retry intentionally
ignores those zones during its optimization loop. Common stop reasons include
`disabled`, `routed`, `max_attempts`,
`no_eligible_hints`, `no_safe_adjustment`, `placement_blocked`,
`repeated_placement_state`, `non_improving_retry`, `drc_regression`,
`board_validation_regression`, and `context_canceled`.
Golden tests cover fixture loading, CLI retry summaries, supported category
adjustments, unsupported skip behavior, path normalization, and convergence
boundaries. Full-board retry tests also cover pad-backed seed boards where
spacing retry improves routing evidence, reduce-distance retry emits stable
proximity rules, and safe-stop retry preserves the best attempt. Generated
`design create` candidates now expose placement-stage `pad_hydration` evidence
and the generated LED fixture reaches real routing/connectivity diagnostics.
The expanded retry corpus now also covers generated no-eligible-hint
boundaries, generated multi-block pad-hydration and net-intent boundaries
before routing, hard-constraint preservation under retry adjustment, generated
mobility ownership, local-route mobility classification, and CLI selected-field
evidence for generated retry output. Larger generated convergence fixtures now
declare `multi-block-convergence`, `DRC-regression`, `no-convergence`,
`fixed-boundary`, and `local-route-boundary` families for continued ranking and
evidence hardening.

In practical terms, retry is now proven at three levels: focused category and
stop-condition unit coverage, pad-backed full-board seed coverage, and CLI
summary coverage for generated workflows with hydrated pad evidence.
Pad-backed full-board fixtures prove before/after improvement and safe stop.
Generated workflows now expose placement-stage `mobility` evidence and
routing-stage `local_route_mobility` evidence so AI callers can tell which
generated refs were eligible to move, which remained blocked, and whether local
routes used `transformable`, `rebuildable`, `preserved`, or `blocked`
handling.

Retry remains opt-in and is a layout-improvement/diagnostic mechanism, not a
fabrication-readiness claim. Current roadmap focus: strengthen BOM/CPL,
component identity, and manufacturer profile evidence for fabrication packages
now that retry convergence evidence and optional DRC evidence hooks are in
place.
