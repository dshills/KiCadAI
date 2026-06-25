# KiCadAI

KiCadAI is a Go toolkit for AI-assisted KiCad design work. It currently combines
three related capabilities:

- a Go client for probing KiCad's live IPC API;
- direct KiCad project, schematic, and PCB file writers;
- CLI tools for generation, inspection, evaluation, connectivity-first board
  validation, ERC/DRC feedback, validation repair planning, round-trip validation, transactions,
  operation-correlated feedback, component intelligence, and pinmap readiness
  checks, plus placement congestion/fanout/advanced-rule scoring and bounded
  placement-routing retry evidence, fabrication readiness previews, and
  deterministic intent planning.

The practical near-term goal is to let agents build and review KiCad-native
projects from structured intent while keeping the lower-level writer strict,
deterministic, and testable.

## Status

The direct-file workflow is the main functional path today. It can generate
KiCad project directories, write root schematics and PCBs, inspect existing
projects, evaluate common correctness issues, apply a conservative subset of
transactions to imported projects, place components with block-aware quality
reports, congestion/fanout readiness evidence, and repair diagnostics, route
small PCB nets, validate
symbol-to-footprint pinmaps, select catalog-backed components, run
connectivity-first PCB validation, and run KiCad-backed ERC/DRC checks through
`kicad-cli`. Transaction validation, planning, and apply results carry stable
operation IDs where possible so AI agents can connect issues back to the source
operation they need to repair.
Placement/routing feedback now has an opt-in bounded retry foundation:
placement quality reports include coarse congestion cells and component fanout
pressure, routing diagnostics map to placement retry hints, retry policies are
explicit in design requests, and `design create` can rerun placement/routing
within a capped budget while returning best-attempt evidence. Generated
placements carry mobility classes (`fixed`, `group_transform`, `local_rebuild`,
`soft_preferred`, and `unowned`) plus route-handling policy so retry can move
eligible generated block-local refs while preserving hard constraints and
local-route intent. Placement candidate scoring now includes advanced PCB rule
families for thermal placement, high-current paths, creepage/clearance domains,
differential-pair placement readiness, and controlled-impedance corridor
evidence. These rules support hard rejection where placement-time proof is
possible and workflow summaries for AI callers. Golden tests now cover retry
behavior across fixtures, categories, stop conditions, unsupported skips, CLI
summaries, pad-backed full-board seed fixtures for spacing improvement,
reduce-distance rule evidence, safe non-improvement stops, generated mobility
ownership, and local-route mobility evidence. The generated `design create` workflow now
hydrates footprint pad summaries through resolver-backed records or verified
built-in seed templates, so generated boards reach real routing/connectivity
evidence instead of stopping at missing pads.
Routing engine hardening now has an implemented foundation: shared PCB rule
resolution, route quality reports, net-class and role-aware routing, length
policy, search-pressure quality scoring, explicit zone policy behavior,
via/layer diagnostics, coupled-net intent reporting, workflow integration, and
golden route coverage.
Circuit-block entry anchors now have board-level binding evidence in
`design create`: placed connector/interface pads, derived board-edge pad
points, explicit `board_edge_point` endpoints, and `imported_mechanical_point`
endpoints can bind to
ESD and reverse-polarity protection anchors. Endpoint-to-anchor route
operations are emitted when geometry is known, and routing summaries report
bound, missing, ambiguous, invalid, unsupported, net-mismatched, routed, and
not-routable anchor states.
Closed-loop validation repair now has a deterministic foundation: issue
classification, repair planning, safe transaction-level executors, a bounded
runner that requires revalidation before reporting success, persisted repairs
for generated projects, post-repair validation adapters, an opt-in
`validation_repair` workflow stage, and a `repair` CLI for structured plans and
target-based apply. Persisted repair results include validation summaries,
before/after issue deltas, retry budget evidence, and artifacts such as repair
bundles or KiCad reports when available.

The live KiCad IPC client is useful for connection probes, version checks,
document discovery, and capability reporting. Live schematic/PCB mutation
through the IPC API is still limited by the write commands exposed by the
currently generated KiCad API surface, so actual design generation is done by
writing KiCad files directly.

The intent planner now provides the first higher-level AI orchestration layer.
It accepts structured intent requests, derives requirements and constraints,
maps supported goals to circuit blocks, emits assumptions and known gaps, and
can hand the generated request to `design create` for project generation.

## Requirements

- Go 1.22 or newer.
- KiCad 9 or newer is the target for current file output and IPC protobufs.
- `kicad-cli` is optional, but recommended for round-trip checks and external
  KiCad ERC/DRC validation.
- `protoc` is only needed when regenerating vendored protobuf bindings.

The repository vendors KiCad API protobuf definitions under
`third_party/kicad/api/proto` and commits generated Go bindings under
`internal/kiapi/gen`.

## Quick Start

Run tests:

```sh
make test
```

Run the compiled CLI:

```sh
kicadai --help
```

Build a local binary:

```sh
go build -o bin/kicadai ./cmd/kicadai
```

Generate a simple LED project without contacting KiCad:

```sh
kicadai \
  --json \
  --output /tmp/led_indicator \
  --name led_indicator \
  --seed demo \
  --with-pcb \
  generate-led-demo
```

Open `/tmp/led_indicator/led_indicator.kicad_pro` in KiCad.

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

### Component Intelligence

Component intelligence provides a deterministic catalog and selection layer for
AI-facing generation. The default catalog lives in `data/components/` and can be
overridden with `--catalog-dir`. Records include KiCad symbol IDs, package
variants, footprint IDs, function pins, pad functions, ratings, values, and
verification confidence.

Confidence levels are:

- `verified`: explicit evidence is available.
- `library_derived`: derived from KiCad library metadata.
- `rule_inferred`: limited safe inference, mainly symmetric passives.
- `placeholder`: draft-only structural stand-in.
- `blocked`: known unsafe or incomplete.

Selection is acceptance-gated. Draft requests may use placeholders with
warnings. Connectivity, ERC/DRC, and fabrication-candidate requests reject
placeholder active components and require verified evidence except for narrowly
allowed passive rule-inferred records.

Examples:

```sh
kicadai --json component list
kicadai --json component show resistor.generic.0805
kicadai --json component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai --json --request ./examples/components/select_resistor.json component select
kicadai --json component validate
```

`design create` includes a `component_selection` stage after block planning and
before schematic or PCB writes. Request JSON can include `component_policy` to
set a catalog directory, minimum confidence, package preferences, per-role
component overrides, and component-specific acceptance. See
`docs/component-intelligence.md` and `examples/components/`.

### AI Design Workflow

`design create` is the first deterministic AI-facing workflow. It accepts an
explicit request JSON, selects built-in circuit blocks by ID, generates a
schematic transaction, realizes PCB fragments, places footprints, optionally
routes, writes the KiCad project, runs structural/connectivity validation, and
returns stage-by-stage feedback.

```sh
kicadai \
  --json \
  --request ./examples/design/led_indicator.json \
  --output ./out/led_indicator \
  --overwrite \
  design create
```

Useful flags:

- `--output`: project output directory for `design create` and generation
  commands; for `repair export-bundle`, an optional bundle output path inside
  the target root.
- `--target`: existing generated project directory or file for
  target-oriented repair commands.
- `--execute`: perform filesystem writes for repair export/apply commands that
  otherwise default to dry-run behavior.
- `--overwrite`: allow replacing generated output directories or existing
  repair bundle files.
- `--skip-routing`: skip board routing while still writing realized local PCB
  fragments.
- `--route-mode`: routing mode, one of `single_layer`, `two_layer`, or
  `validate_only`; `--mode` remains as a shorter alias.
- `--grid`, `--trace-width`, and `--clearance`: route planner controls in
  millimeters.
- `--allow-partial`: allow partial route results.
- `--placement-estimated-width`, `--placement-estimated-height`,
  `--placement-board-margin`: fallback placement geometry controls.
- `--strict-unrouted`, `--strict-zones`: make validation stricter.
- `--require-drc`, `--kicad-cli`, `--keep-artifacts`, `--artifact-dir`: require
  KiCad-backed DRC evidence.
- `--allow-missing-drc`: request optional DRC evidence without blocking when
  `kicad-cli` is unavailable.
- `--repair`: include the plan-only `validation_repair` stage in
  `design create`; it does not persist the replay bundle.
- `--repair-apply`: implies `--repair`, applies generated-project validation
  repair during `design create`, and writes `.kicadai/repair-bundle.json`.
- `--max-repair-attempts`: bound validation repair attempts when repair is
  enabled by the workflow or a repair command.

Examples live in `examples/design/`:

- `led_indicator.json`: structural acceptance, routing skipped.
- `sensor_breakout.json`: connectivity-oriented request that may produce
  actionable routing/validation feedback as the workflow matures.

The top-level JSON result uses requested acceptance as the success contract. A
request for `structural` acceptance can return `ok: true` while still including
connectivity or KiCad-check issues for higher levels. Those issues are retained
in `issues[]` and grouped under `data.feedback.repairs[]` so an agent can decide
whether to revise the request, adjust placement/routing, or ask for external
tooling.

### Intent Planner

The `intent` command family sits above `design create`. It is intended for AI
callers that can produce structured intent but should not need to know the
exact block IDs, validation defaults, or generated workflow request shape.

```sh
kicadai \
  --json \
  --request ./examples/intent/sensor_breakout.json \
  --output ./out/intent_plan \
  --overwrite \
  intent plan

kicadai \
  --json \
  --request ./examples/intent/sensor_breakout.json \
  intent explain

kicadai \
  --json \
  --request ./examples/intent/sensor_breakout.json \
  --output ./out/intent_sensor \
  --overwrite \
  intent create
```

Commands:

- `intent plan`: validates the intent request, derives requirements,
  assumptions, known gaps, selected blocks, component policy, validation
  policy, and a generated `design create` request.
- `intent explain`: returns a compact AI-facing summary of requirements,
  selected blocks, assumptions, gaps, and blocking issues to stdout.
- `intent create`: plans intent, refuses ambiguous or blocking plans, runs
  `design create`, and writes planner artifacts under the generated project's
  `.kicadai/` directory.

When `--output` is passed to `intent plan`, the planner writes
standalone preview artifacts, `intent-plan.json` and
`generated-request.json`, directly in that output directory so the plan can be
inspected without creating project metadata. When
`intent create` succeeds, it writes the generated KiCad project plus
`.kicadai/intent-plan.json` and `.kicadai/generated-request.json` inside the
project output directory. The `.kicadai/` paths are the persistent
project-relative metadata locations once a KiCad project exists.

`--output` is used for new generated content, including intent planning and
project creation. `--target` is reserved for commands that inspect or repair an
existing generated project; after `intent create` or `design create`, pass that
generated project directory as `--target` to later target-oriented repair
commands. Commands that write to an existing output directory require
`--overwrite`.

Examples live in `examples/intent/` and cover sensor breakout, MCU programmer,
power module, amplifier module, and fabrication-oriented sensor requests.

Current intent-planner gaps:

- input is structured JSON, not free-form natural language;
- MCU peripheral, clock, programming, and I2C support remain conservative known
  gaps unless represented by verified block metadata;
- some block supply-voltage metadata is still partial, so unsupported voltage
  compatibility evidence is reported instead of guessed;
- fabrication-focused intent maps to stricter validation/component/routing
  policy, but external manufacturer acceptance remains a downstream
  fabrication-readiness concern.

Current gaps for autonomous one-shot schematic + PCB generation:

- end-to-end autonomous design still depends on the planner's structured intent
  request and verified block/component coverage;
- placement is deterministic and conservative, not a full board-layout
  optimizer;
- routing is suitable for small known-good nets and local fragment routes, not
  dense production boards;
- writer correctness now reports generated-file gaps explicitly; remaining
  failures mostly reflect missing footprint assignments, resolver-backed
  pinmaps, and strict PCB reader/writer coverage;
- KiCad ERC/DRC requires `kicad-cli` from KiCad 7 or newer and is optional
  unless requested.

### Circuit Blocks

The block library contains reusable schematic templates for LED indicators,
connector breakouts, voltage regulators, I2C sensors, op-amp gain stages,
USB-C power input, a minimal ATmega328P-A system, crystal and canned
oscillators, reset/programming headers, 5 V ESD shunts, and series-diode
reverse-polarity input protection. Blocks can be listed, inspected, validated,
instantiated, composed, and now realized as PCB fragments.

Built-in blocks now declare machine-readable electrical rules, PCB rules, local
route requirements, placement/proximity constraints, verification evidence, and
readiness gaps. `design create` includes that block readiness in its
`block_planning` stage so agents can explain selected blocks, missing evidence,
required local routes, and known limitations before writing a project.

```sh
kicadai --json block list
kicadai --json block show led_indicator
kicadai --json --request ./examples/blocks/requests/led_indicator.json block instantiate led_indicator
kicadai --json --request ./examples/blocks/requests/led_indicator.json block realize-pcb led_indicator
```

`block realize-pcb` returns:

- the normal block instantiation output with schematic transactions;
- `realization.components[]` with refs, footprints, role names, and relative
  PCB placements;
- `realization.entry_anchors[]` with `id`, `port` (the block-level logical
  port associated with the anchor), `net_name`, `description`,
  block-origin-relative `placement.x_mm` and `placement.y_mm`, and string
  `placement.layer` values such as `"F.Cu"` for block-boundary entry/exit
  points such as connector-adjacent ESD inputs and protected power-path
  endpoints;
- `realization.local_routes[]` with `id`, `net_name`, endpoint refs/pins, and
  endpoint objects that use either component refs/pins or anchor references
  through `anchor_id`, plus route operations for verified local nets;
- `placement_request`, a ready input for the placement engine.

This is the first PCB-fragment layer for the circuit block library. It does not
yet claim fabrication readiness for complete boards; global block composition,
board outline selection, route conflict resolution, zone refill, and KiCad DRC
evidence are still required before generated block PCBs should be treated as
manufacturing candidates.

Circuit blocks also have a verification harness with checked-in manifests for
all built-in blocks. It verifies schematic semantics, PCB placement/pad/route
expectations where declared, PCB realization metadata, internal board
validation, writer correctness when requested, and optional KiCad ERC/DRC
evidence. The oscillator and reset/programming manifests now
assert realized local routes and timing fixtures for local decoupling,
reset/programming route length, enable/control presence, and ground-reference
checks where the current realization model can prove them. ESD and
reverse-polarity protection manifests now require modeled entry-anchor route
and power-path local-route evidence via `expected.pcb.required_local_routes`.
`design create` now adds board-level `anchor_bindings` evidence in the routing
stage when realized blocks expose entry anchors. The workflow discovers placed
physical pad endpoints, derives `board_edge_point` endpoints from edge-constrained
pads, accepts request-declared `external_endpoints`, resolves required
protection anchors to connector, `board_edge_point`, or
`imported_mechanical_point` endpoints,
emits endpoint-to-anchor route operations when both coordinates are known, and
reports bound, missing, ambiguous, invalid, unsupported, net-mismatched, routed,
or not-routable bindings as structured evidence. This binding evidence prevents
synthetic anchors from being mistaken for proven external interfaces, but it is
not a substitute for KiCad DRC, surge/thermal analysis, DFM checks, or
fabrication readiness gates.

In the root object of a `design create` request JSON, request-declared external
endpoints use this shape:

```json
{
  "external_endpoints": [
    {
      "id": "edge_vin",
      "kind": "board_edge_point",
      "net_name": "VIN_RAW",
      "roles": ["power_entry", "edge"],
      "layers": ["F.Cu"],
      "point": {"x_mm": 0, "y_mm": 12.5},
      "edge": "left",
      "required": true
    }
  ]
}
```

Use `kind: "imported_mechanical_point"` for user/importer-supplied interface
coordinates. `id` is required for every external endpoint, normalized by
trimming whitespace, lowercasing, replacing every run of non-`[a-z0-9_]`
characters with `_`, trimming leading/trailing `_`, and must be unique within
the `external_endpoints` array after normalization; missing or duplicate IDs
fail request validation. IDs are used as stable diagnostic and evidence
identifiers, not as references from other request fields yet. The optional
`edge` field is descriptive evidence for
`board_edge_point` endpoints; values normalize to lowercase and the valid values
are `left`, `right`, `top`, and `bottom` in the rectangular board coordinate
space, where `top` is the minimum Y edge and `bottom` is the maximum Y edge.
Non-rectangular boards should use the nearest cardinal direction until
polygon-edge endpoint support exists. `point.x_mm` and `point.y_mm` are
millimeters in the board coordinate frame used by the generated PCB. This
follows KiCad's positive-down Y convention, so smaller Y values are visually
above larger Y values. `roles` describe external-interface intent; useful values
include `connector`, `edge`, `external`, `power_entry`, `mechanical_interface`,
and `castellated`. Layer names are normalized to KiCad canonical names, so
`f.cu` becomes `F.Cu`; unsupported copper or technical layers fail validation.
For physically required endpoints, prefer an explicit copper layer list such as
`["F.Cu"]` or `["B.Cu"]` that matches the real interface copper. Omitted
`layers`, `null`, or `[]` is permissive fallback behavior: it acts as a binding
wildcard that can match an anchor on any available copper layer, but it does not
prove the physical endpoint exists on every copper layer. Optional endpoints can
omit `point` or `net_name` and remain visible as advisory evidence, but they
cannot produce route evidence until both are known. Endpoints marked
`"required": true` fail request validation unless `point` is a non-null object
with finite numeric `x_mm` and `y_mm` values and `net_name` is a non-empty
string.

```sh
kicadai --json --builtins block verify
kicadai --json --case ./internal/blocks/testdata/verification/led_indicator_default/manifest.json block verify
kicadai --json --suite ./internal/blocks/testdata/verification --output ./out/block-verification --overwrite block verify
kicadai --json --builtins --kicad-corpus --kicad-corpus-tier smoke block verify
```

KiCad-backed checks are skipped unless a manifest or flag requires them. Use
`--kicad-cli`, `--require-erc`, `--require-drc`, `--keep-artifacts`, and
`--artifact-dir` when external ERC/DRC evidence is required. Optional ERC/DRC
expectations are visible as skipped when no output directory or KiCad CLI is
available; required ERC/DRC fails verification with an explicit reason. A
skipped optional ERC/DRC stage means the block remains structurally verified by
the built-in harness, but it is not KiCad-clean or fabrication-ready evidence.
The opt-in KiCad corpus currently seeds `led_indicator_default` and
`connector_breakout_4pin` as smoke-tier candidates. Corpus mode emits a
`kicad_corpus` summary and, with `--output`, writes `corpus-summary.json` plus
per-case `reports/corpus-result.json` files. Normal `go test ./...` remains
KiCad-independent by default; real KiCad smoke tests require
`KICADAI_RUN_KICAD_CLI=1` and can use
`KICADAI_KICAD_CLI=/path/to/kicad-cli` for non-default installs.
When checks run, generated project freshness records the project signature and
a separate ERC/DRC check-context signature for the resolved `kicad-cli` path
and version, measurement units, and allowlist contents. Golden report snapshots
live under `cmd/kicadai/testdata/golden/block_verification` and can be
refreshed with:

```sh
go test ./cmd/kicadai -run TestRunBlockVerificationGoldens -update-block-verification-goldens
```

The `./internal/...` paths above are source-tree paths for repository
development. For general use, prefer `--builtins` or pass your own manifest
path through `--case` or `--suite`.

See [docs/circuit-block-verification.md](docs/circuit-block-verification.md)
for evidence levels, manifest structure, and extension guidance.

### Placement

The placement engine accepts a structured board placement request and returns
placed components, geometry issues, metrics, and `place_footprint` transaction
operations for successful placements.

```sh
kicadai \
  --json \
  --request ./examples/placement/simple_request.json \
  place request
```

Current placement support includes:

- fixed components and top/bottom side constraints;
- connector edge intent;
- hard and optional keepouts plus mechanical constraints;
- component spacing, group anchors, and group spread checks;
- proximity rules and region preferences;
- semantic candidate scoring for component role, group cohesion, electrical
  proximity, route length, congestion, fanout, edge, region, and mobility;
- advanced placement rules for thermal spacing/edge preference, high-current
  source-load proximity, creepage/clearance domain spacing, differential-pair
  placement readiness, and controlled-impedance corridor/reference-plane
  evidence;
- timing-sensitive placement scoring for clock-source proximity rules, used by
  crystal, canned oscillator, and reset/programming realization evidence;
- hard candidate rejection for checkable advanced-rule violations;
- per-net HPWL and routing-readiness reports;
- footprint-derived bounds helpers;
- transaction operation output.

Requests can use explicit component bounds or hydrate bounds from the library
resolver in Go before calling `placement.Place`. The usable board area uses the
larger of the JSON `board.margin_mm` and `rules.board_edge_clearance_mm` values
(`Board.MarginMM` and `Rules.BoardEdgeClearanceMM` in Go). Board coordinates
follow KiCad's schematic/PCB file convention: X increases to the right and Y
increases downward.

`placement.BuildQualityReport` returns AI-facing evidence for repair loops:

- `group_reports`, `proximity_reports`, `region_reports`, `net_reports`, and
  `keepout_reports` describe why a placement is good or poor; edge constraint
  evidence is exposed through edge counters and score dimensions.
- `score.dimensions` currently covers group cohesion, edge constraints,
  mechanical keepouts, region satisfaction, routing readiness, and proximity.
- `diagnostics` maps placement quality issues to repairable actions for
  missing placements, keepouts, regions, proximity, routing readiness,
  estimated footprint geometry, grouping, advanced placement rules, and
  validation issues.
- Design workflow placement summaries include advanced-rule dimension counts,
  worst scores, hard violations, warning counts, and unsupported/missing proof
  evidence for AI callers.
- Design workflow PCB realization summaries include `timing_results`, and
  timing evidence findings are surfaced as stage issues with refs, nets, and
  repair suggestions when thresholds are violated.

The placement hardening golden corpus covers representative LED, regulator,
MCU minimal, USB-C power, I2C sensor, op-amp gain-stage, and connector-breakout
layouts. Placement-routing retry coverage now also includes deterministic
goldens for retry summary shape, spacing/fanout/distance adjustments,
unsupported skip behavior, selected stop conditions, CLI output, and pad-backed
full-board seed evidence. Generated full-board boundary coverage now includes
hydrated-pad retry evidence for LED and multi-block sensor/header workflows,
generated placement mobility summaries, local-route mobility summaries, and
hard-constraint preservation under retry adjustment.
Placement is still a deterministic heuristic, not a production-grade constraint
solver. Advanced placement rules are placement-level heuristics and evidence,
not thermal simulation, impedance calculation, or routed length matching.
Larger-board convergence, spatial acceleration for large hard-rule sets, and
final KiCad DRC-backed layout proof remain future work.

### Routing

The routing engine is a deterministic grid router for small placed boards. It
accepts a structured `routing.Request`, routes intentional nets, validates
connectivity and clearance in-process, and returns route segments, vias,
metrics, issues, AI-facing repair diagnostics, and route-shaped operations.

Run routing from JSON:

```sh
kicadai \
  --json \
  --request ./examples/routing/simple_request.json \
  --mode single_layer \
  --grid 1 \
  --trace-width 0.1 \
  --clearance 0.2 \
  route request
```

For `route request`, CLI flags override the matching values from the JSON
request. Omit the flags when you want to run the fixture exactly as written.

Useful route flags:

- `--mode single_layer|two_layer|validate_only`
- `--grid <mm>`
- `--trace-width <mm>`
- `--clearance <mm>`
- `--allow-partial`

Routing JSON uses `rules.edge_clearance_mm` for board-edge clearance. Placement
JSON uses `rules.board_edge_clearance_mm`; the two request schemas are related
but intentionally separate. There is not currently a route CLI override for
edge clearance, so set this value in the JSON request.

The Go API entry points are:

- `routing.RouteRequest(request)` for raw route planning.
- `routing.ValidateResult(request, result)` for in-process route checks.
- `routing.DiagnosticsForResult(result)` for repair categories/actions.
- `routingadapters.RequestFromPlacement(...)` to route placement output.
- `designapi.Builder.RouteBoard(...)` to apply routed tracks/vias to a design.

### Connectivity-First Board Validation

`validate board` is the current board-readiness gate for generated PCBs. It
combines file parsing, structural PCB validation, net-to-pad checks, generated
connectivity checks, unrouted-net summaries, route endpoint checks, zone
evidence, and optional KiCad DRC evidence into one JSON result.

```sh
kicadai \
  --json \
  validate board ./examples/07_generated_pcb
```

`validate board` accepts either a `.kicad_pcb` file, a `.kicad_pro` file, or a
project directory. When given a project directory, it chooses the board matching
the project name and reports an error for ambiguous board files.

Useful flags:

- `--strict-zones`: make zones without fill evidence blocking.
- `--require-drc`: require KiCad DRC evidence.
- `--allow-missing-drc`: reserved for future workflows; it is currently mutually
  exclusive with `--require-drc`, and missing DRC evidence is already
  non-blocking by default.
- `--kicad-cli /path/to/kicad-cli`: run KiCad DRC and include parsed findings.
- `--allowlist ./allowlist.json`: suppress known KiCad DRC findings through the
  existing check allowlist format.
- `--keep-artifacts --artifact-dir ./reports`: retain KiCad DRC reports.

The result includes stable check names:

- `pcb_structural_validation`
- `net_to_pad_validation`
- `generated_connectivity`
- `unrouted_net_validation`
- `route_completion`
- `zone_validation`
- `kicad_drc`

Missing DRC evidence is explicit but non-blocking by default. DRC findings are
blocking when DRC runs and returns unsuppressed findings. Zone fills are not
refilled in-process; a zone without filled polygons is a warning by default and
becomes blocking with `--strict-zones`.

Known limitations:

- The validator does not replace KiCad DRC or zone refill.
- File-backed validation internally normalizes a small set of fields the current
  PCB reader does not fully hydrate yet, such as footprint paths and property
  layers. This is read-only and does not modify project files on disk.
- In-process route completion uses deterministic geometric evidence and is meant
  to catch generated-board mistakes early, not to certify fabrication outputs.

Current routing support includes deterministic net ordering, single-layer
Manhattan routing, two-layer routing with vias, keepout and existing-copper
obstacles, route simplification, connectivity validation, clearance validation,
operation emission, KiCad check feedback mapping, golden routed examples,
bounded stress tests, shared `internal/pcbrules` resolution, per-net route
quality reports, net class and role-aware trace/via/layer rules, length policy
warnings/failures, explicit zone policies, coupled-net intent reporting,
repairable route diagnostics, workflow route-quality summaries, and a routing
hardening golden corpus.
Detailed design rationale and remaining hardening requirements are documented in
`specs/routing-engine-hardening/`.

Current routing limitations:

- The router is intended for simple boards and early AI workflow validation, not
  dense BGA or production autorouting.
- Routes are orthogonal grid paths. Length policies can warn or fail routes,
  but automatic length tuning/meanders, diagonal routing, curved routing,
  differential-pair routing, and impedance-aware routing are not supported yet.
- The shared rule model includes clearance matrices and neckdown constraints,
  but full DRC-grade neckdown geometry and clearance-matrix enforcement remain
  limited.
- Placement quality strongly affects routing success.
- Copper zones are treated as obstacles or unsupported policy inputs; zone-fill
  aware routing and conservative zone-sufficient proof remain intentionally
  blocked until proof evidence exists.
- Placement can hydrate bounds and compact pad summaries from resolver records.
  New-project and imported-project transaction apply can hydrate pads, pad
  shapes, through-hole metadata, text, graphics, and models. Routing still
  consumes compact pad summaries rather than full pad-stack data.
- KiCad DRC execution is integrated through the checks package, but tests still
  rely primarily on deterministic parser/fake-runner paths unless a local
  stable KiCad fixture is available.

### Writer Correctness Checks

`writer check` is the generated-file correctness gate. It is stricter than a
plain parser and more writer-focused than board readiness. It answers whether
files emitted by KiCadAI preserve project structure, schematic connectivity,
schematic-to-PCB transfer, footprint pad net assignments, copper net
references, zone references, and optional KiCad round-trip stability.

```sh
kicadai --json writer check ./examples/07_generated_pcb/generated_pcb.kicad_pcb
kicadai --json writer check --strict-diffs --allow-unrouted ./examples/07_generated_pcb
```

The command accepts a project directory, `.kicad_pro`, `.kicad_sch`, or
`.kicad_pcb` target. It returns nonzero when blocking writer issues are present,
which makes it suitable for CI and AI workflow gating. Project and `.kicad_pro`
targets can run cross-file checks. Single-file targets run the checks supported
by that file and skip checks that require a missing sibling schematic or PCB.

Stable checks:

- `project_structure`
- `schematic_parse`
- `schematic_connectivity`
- `schematic_pcb_transfer`
- `pcb_parse`
- `pcb_net_table`
- `footprint_pad_nets`
- `copper_net_references`
- `zone_net_references`
- `kicad_round_trip`

Useful flags:

- `--require-kicad-roundtrip`
- `--kicad-cli /path/to/kicad-cli`
- `--strict-diffs`
- `--allow-unrouted`
- `--keep-artifacts --artifact-dir ./reports`

Current limits:

- Some older generated examples intentionally fail writer checks because they
  lack footprint assignments or resolver-backed pinmaps.
- KiCad round-trip evidence is skipped unless a KiCad CLI path is available, or
  blocking when `--require-kicad-roundtrip` is set.
- Check names and flags use their stable CLI/API identifiers, so separators
  differ between JSON check IDs such as `kicad_round_trip` and flags such as
  `--require-kicad-roundtrip`.
- The writer gate is not a fabrication package validator.

### Inspection

Inspect KiCad projects and files:

```sh
kicadai --json inspect project ./examples/07_generated_pcb
kicadai --json inspect schematic ./examples/01_led_indicator/led_indicator.kicad_sch
kicadai --json inspect pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
```

Inspection reports summarize discovered files, symbol counts, footprint counts,
and reader issues.

### Evaluation

Evaluate projects and files for generated-output readiness:

```sh
kicadai --json evaluate project ./examples/07_generated_pcb
kicadai --json evaluate schematic ./examples/01_led_indicator/led_indicator.kicad_sch
kicadai --json evaluate pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
```

Current evaluation includes checks for missing files, duplicate references,
missing footprints, missing board outlines, disconnected pads, invalid net
assignments, and preservation conflicts. Reports include a
`fabrication_ready` field when applicable.

### Transactions

Transactions are structured edit plans. They can be validated, planned against a
target, or applied. Validation and planning include generated operation
summaries with stable IDs, and apply annotates operation-scoped failures with
the same IDs where attribution is safe.

```sh
kicadai --json transaction validate ./tx.json
kicadai --json transaction plan ./out/project ./tx.json
kicadai --json --overwrite transaction apply ./out/project ./tx.json
```

For AI repair loops, add `--feedback` to validation or planning. The command
keeps the raw issue list and also returns a grouped `feedback` object keyed by
stable operation IDs.

```sh
kicadai --json --feedback transaction validate ./examples/transactions/invalid_feedback.json
kicadai --json --feedback transaction plan ./out/invalid_feedback ./examples/transactions/invalid_feedback.json
```

Each transaction operation summary includes an `id`, and operation-scoped
issues include `operation_id` when KiCadAI can safely link the issue back to a
source operation. A repair agent should use that ID to find the matching
operation, edit or replace it, and rerun validation. Some issues intentionally
remain unlinked when attribution would be ambiguous, such as shared refs,
shared nets, or generic KiCad CLI findings without trace data.

Feedback summaries include:

- `operations[]`: grouped issues by operation ID, including refs, nets,
  artifacts, severity, and suggestions;
- `issues[]`: the original flat issue list;
- `summary`: operation count, issue count, blocking/error/warning counts, and
  unlinked issue count.

Transaction inputs do not need to contain operation IDs. KiCadAI derives IDs
from operation content and disambiguates identical operations. If an operation
changes, its ID is expected to change.

Supported operation kinds:

- `create_project`
- `set_board_outline`
- `add_symbol`
- `connect`
- `assign_footprint`
- `place_footprint`
- `route`
- `add_zone`
- `write_project`

`remove_symbol` is modeled but intentionally unsafe for imported apply and is
blocked by planning.

Minimal generated-project transaction:

```json
{
  "name": "demo",
  "operations": [
    { "op": "create_project", "name": "demo" },
    { "op": "set_board_outline", "board": { "width_mm": 30, "height_mm": 20 } },
    {
      "op": "add_symbol",
      "ref": "R1",
      "value": "10k",
      "library_id": "Device:R",
      "at": { "x_mm": 25, "y_mm": 25 },
      "pins": [
        { "number": "1", "x_mm": -2.54, "y_mm": 0 },
        { "number": "2", "x_mm": 2.54, "y_mm": 0 }
      ]
    },
    { "op": "assign_footprint", "ref": "R1", "footprint_id": "Resistor_SMD:R_0805_2012Metric" },
    {
      "op": "place_footprint",
      "ref": "R1",
      "at": { "x_mm": 15, "y_mm": 10 },
      "pads": [
        { "name": "1", "type": "smd", "width_mm": 1.2, "height_mm": 1.4, "net": "NET_A" },
        { "name": "2", "type": "smd", "width_mm": 1.2, "height_mm": 1.4, "net": "NET_B" }
      ]
    },
    { "op": "write_project" }
  ]
}
```

Imported-project apply is deliberately conservative. It supports adding symbols,
assigning footprints, placing or moving footprints, adding simple routes and
zones, and writing the project. It blocks unsupported raw content, unsafe
removals, arbitrary hierarchy refactors, and operations that could damage
unknown KiCad constructs. Imported writes use a project lock, atomic file
replacement, permission preservation, and fsync before rename.

### Pinmaps

Pinmap validation checks whether schematic symbol-to-footprint assignments have
human-verified pin mappings before fabrication readiness is claimed.

List built-in pinmaps:

```sh
kicadai --json pinmap list
```

Validate a project:

```sh
kicadai --json pinmap validate ./examples/01_led_indicator
```

Current built-in mappings include common resistors, capacitors, LEDs, simple
headers, and `Device:Q_NPN_BEC` to a TO-92 inline footprint. Missing mappings,
pin-count mismatches, pin-name mismatches, and unflattened hierarchical sheets
block pinmap fabrication readiness.

### Library Resolver

The `library` command indexes local KiCad symbol, footprint, and template
repositories so generators and transactions can use real library IDs.

```sh
export KICADAI_KLC_ROOT=/path/to/klc
export KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols
export KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints
export KICADAI_TEMPLATES_ROOT=/path/to/kicad-templates

kicadai --json library symbol Device:R
kicadai --json library footprint Resistor_SMD:R_0805_2012Metric
kicadai --json library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric
kicadai --json library pinmap-candidate Device:R Resistor_SMD:R_0805_2012Metric
kicadai --json library templates
```

Hardened symbol inspection commands expose resolver evidence without requiring
agents to read raw `.kicad_sym` files:

```sh
kicadai --json library symbols list
kicadai --json library symbols show Device:R
kicadai --json library symbols pins Device:R
kicadai --json library symbols validate Device:R
```

These commands report parsed units, common pins, electrical types, power-symbol
flags, inherited metadata, rough graphics bounds, and resolver diagnostics.
`writer check` can use the same resolver evidence when symbol roots are
configured, and reports a `library_resolver` check when resolution is attempted.

Use `--library-cache .kicadai/library-index.json` for faster repeated loads and
`--refresh-library-cache` to rebuild it. See
[docs/library-resolver.md](docs/library-resolver.md) for setup, command
examples, cache behavior, compatibility statuses, and opt-in integration tests.

### Circuit Block Library

The `block` command exposes reusable circuit primitives for AI-assisted
schematic generation. Blocks declare parameters, ports, required libraries, and
verification levels.

```sh
kicadai --json block list
kicadai --json block show led_indicator
kicadai \
  --json \
  --request examples/blocks/requests/led_indicator.json \
  --output ./out/led_indicator \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
kicadai \
  --json \
  --request examples/blocks/requests/composed_sensor_breakout.json \
  --output ./out/composed_sensor_breakout \
  --name composed_sensor_breakout \
  --overwrite \
  block compose
```

Current built-in blocks include LED indicator, connector breakout, voltage
regulator, I2C sensor, op-amp gain stage, USB-C power input, MCU minimal
system, crystal oscillator, canned oscillator, reset/programming header, 5 V
ESD protection, and reverse-polarity input protection. These blocks now carry
electrical and PCB rule metadata for required companions, decoupling, pull-ups,
rail compatibility, enable/reset/programming handling, edge constraints,
keepouts, proximity constraints, route priorities, conditional realization, and
required local routes where the current realization model supports them. The
newer protection and timing blocks remain structural/partial: they use verified
seed records and checked-in verification manifests, but need more variants and
stronger KiCad-backed layout evidence before fabrication-readiness claims.

The generated block examples are structural schematic/project outputs; they are
not yet fabrication-ready PCB designs. See
[docs/circuit-block-library.md](docs/circuit-block-library.md) for request
formats, verification levels, resolver requirements, examples, AI usage, and
known limitations. The current release-readiness gap matrix is in
[docs/circuit-block-readiness.md](docs/circuit-block-readiness.md).

### Round-Trip Validation

Round-trip commands use `kicad-cli` to save or normalize files and compare the
result:

```sh
kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  roundtrip schematic ./examples/01_led_indicator/led_indicator.kicad_sch

kicadai --json roundtrip pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
kicadai --json roundtrip project ./examples/07_generated_pcb
```

Useful flags:

- `--keep-artifacts`
- `--artifact-dir ./examples/roundtrip_artifacts`
- `--timeout 30s`
- `--allowlist ./allowlist.json`

If `kicad-cli` is not found, round-trip checks return a structured skipped
result rather than failing the deterministic unit suite.

### ERC/DRC Checks

KiCad-backed ERC/DRC checks run through `kicad-cli`, preserve the raw JSON
report, and return structured findings for AI repair loops:

```sh
kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  check erc ./examples/checks/erc_fail/erc_fail.kicad_sch

kicadai --json check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
kicadai --json check project ./examples/checks/drc_pass
```

Useful flags:

- `--keep-artifacts`
- `--artifact-dir ./examples/check_artifacts`
- `--timeout 30s`
- `--allowlist ./check_allowlist.json`

`evaluate` reports now include skipped ERC/DRC evidence placeholders when
schematic or PCB files are present. Run `check project` when you need actual
KiCad ERC/DRC evidence. A design can be parseable and structurally evaluated
without being ERC/DRC clean or fabrication-ready.

Check output includes:

- stable finding IDs;
- KiCad rule/code, severity, references, nets, layers, locations, and raw report
  snippets when available;
- repair categories such as `connectivity`, `power`, `clearance`, `outline`,
  `footprint`, and `net_assignment`;
- an AI-friendly summary prompt grouped by category, reference, net, and layer.

Current caveat: real ERC smoke testing works with local KiCad 10.0.3. The DRC
runner and parser are implemented and covered with deterministic tests, but the
current local KiCad CLI exits before writing DRC JSON for the generated PCB
fixtures, so a stable real DRC fixture remains a follow-up.

### Fabrication Export And Readiness

The `export` command family evaluates whether a project has enough evidence to
claim fabrication readiness and can produce deterministic package metadata,
BOM, CPL, Gerber, and drill reports. These commands are intended for
machine-to-machine workflows today, so they are dry-run by default and require
`--json`. If `--json` is omitted, the CLI returns the standard
structured-command usage error instead of a human summary.

```sh
kicadai --json export preview ./project
kicadai --json export bom ./project
kicadai --json export fabrication ./project
```

Fabrication reports now include explicit assembly evidence:

- BOM rows carry component identity status, source, package, component class,
  lifecycle, confidence, and issue/blocking counts.
- CPL rows carry BOM linkage, component identity, normalized side, raw layer,
  raw rotation, normalized rotation, and placement readiness notes.
- BOM/CPL consistency checks block mismatched references, duplicate
  references, missing placements, extra placements, footprint mismatches,
  missing coordinates, and unknown assembly sides.
- Optional manufacturer profiles add local assembly policy checks. The built-in
  `generic_assembly` profile requires exact manufacturer/MPN evidence for
  assembly-critical rows while allowing generic passives.

Use `--execute` to write files and `--overwrite` to replace existing package
files. KiCad CLI is required for Gerber and drill generation:

```sh
kicadai \
  --json \
  --execute \
  --overwrite \
  --manufacturer-profile generic_assembly \
  --kicad-cli /path/to/kicad-cli \
  export fabrication ./project
```

Default package paths are under `<project>/fabrication/`:

- `readiness.json`
- `package-manifest.json`
- `physical-rules.json`
- `bom.csv`
- `cpl.csv`
- `gerbers/`
- `drill/`

Readiness statuses are intentionally conservative:

- `blocked`: required project files, writer/board validation, report data, or
  configured external evidence is missing or failing.
- `candidate`: the project has partial evidence, but not enough to claim ready.
- `ready`: all modeled required evidence passes. KiCadAI can now generate and
  validate Gerber/drill evidence through `kicad-cli`, but readiness remains
  blocked or candidate when any modeled evidence is missing or failing.

KiCad CLI evidence is policy-driven. Without `--kicad-cli`, preview and export
stay deterministic and do not invoke external tools. With `--kicad-cli` and
`--execute`, `export fabrication` invokes KiCad CLI to generate Gerber and drill
outputs, validates required copper, mask, silkscreen, Edge.Cuts, and drill
files, and records generated file lists in `package-manifest.json`. Missing
`ready`-level evidence keeps the status at `candidate` or `blocked`, never
`ready`. Physical fabrication checks now run during `export preview`,
`export fabrication` without `--execute`, and `export fabrication` execution.
The generated
`physical-rules.json` report covers stackup, net classes, solder mask/paste pad
policy, Edge.Cuts containment, courtyard overlap/presence, silkscreen board
clearance, and mounting-hole geometry/edge clearance. Physical-rule blockers are
included in readiness status and package manifests. With `--require-drc`,
missing or failing external fabrication evidence is blocking. `design create`
now runs a dry-run fabrication preview only when the input request JSON sets
`validation.acceptance` to
`fabrication-candidate`, which is the highest current design acceptance level
and functions as a request to prove fabrication readiness. That input value is
an enum value; the output field `acceptance.fabrication_ready` is a JSON field
name and boolean. In the output workflow result, partial readiness status
(`candidate` or `blocked`) downgrades the achieved acceptance and leaves
`acceptance.fabrication_ready` false. The `fabrication_readiness` workflow stage
also exposes a compact `physical_rules` summary with status, blocker count,
warning count, active physical-rule/manufacturer profile, and report path
relative to the project root when available.

This is still not a manufacturer acceptance guarantee. KiCadAI validates the
presence, identity consistency, and local profile compatibility of modeled
fabrication outputs, but broader DFM checks such as manufacturer-specific
annular ring policy, solder mask slivers, impedance, panelization, assembly notes,
live part availability, and procurement readiness remain separate gates.

## Examples

Checked-in examples live under `examples/`:

| Example | Focus |
|---|---|
| `01_led_indicator` | Single resistor and LED from VCC to GND. |
| `02_button_pullup` | Pull-up resistor, push button, and output label. |
| `03_rc_filter` | Passive RC low-pass filter with input/output labels. |
| `04_555_timer` | Medium-complexity 555 oscillator schematic. |
| `05_sensor_node` | Hierarchical project with power, MCU, and sensor sheets. |
| `06_class_ab_headphone_amp` | Op-amp gain stage with diode-biased class AB headphone output. |
| `07_generated_pcb` | Generated schematic and PCB fixture. |
| `08_pcb_object_correctness` | PCB object correctness fixture. |
| `checks` | ERC/DRC fixture projects and report samples for KiCad-backed validation. |
| `blocks` | Circuit block library request files and generated schematic/project examples. |
| `transactions` | Transaction fixtures, including an invalid feedback example for AI repair loops. |

Open each KiCad project by opening the `.kicad_pro` file in its directory.

## Go Packages

Key packages:

- `cmd/kicadai`: CLI entrypoint.
- `internal/kiapi`: live KiCad IPC client and transport boundary.
- `internal/kicadfiles/project`: `.kicad_pro` reader/writer.
- `internal/kicadfiles/schematic`: `.kicad_sch` reader/writer and validation.
- `internal/kicadfiles/pcb`: `.kicad_pcb` reader/writer and validation.
- `internal/kicadfiles/design`: project-directory read/write orchestration.
- `internal/kicadfiles/designapi`: higher-level Go builder API.
- `internal/transactions`: structured transaction validation, planning, apply,
  operation IDs, feedback summaries, and operation trace maps.
- `internal/generate`: higher-level project generators.
- `internal/inspect`: inspection reports.
- `internal/evaluate`: readiness and correctness evaluation.
- `internal/kicadfiles/checks`: KiCad CLI ERC/DRC execution, report parsing, and AI-facing findings.
- `internal/kicadfiles/roundtrip`: KiCad CLI round-trip validation.
- `internal/pinmap`: symbol-footprint-pinmap registry and validation.
- `internal/workflows`: AI-facing named workflow registry.
- `internal/writercorrectness`: generated writer correctness gate used by the
  CLI and AI design workflow.

Generated protobuf packages under `internal/kiapi/gen/**` should not be used as
the AI workflow boundary. Prefer `internal/workflows`, `internal/transactions`,
or `internal/kicadfiles/designapi`.

## Testing

Normal tests do not require KiCad:

```sh
make test
```

Equivalent direct command:

```sh
go test ./...
```

If your environment blocks the default Go build cache location, use a writable
temporary cache:

```sh
GOCACHE="$(mktemp -d)" go test ./...
```

Coverage:

```sh
make coverage
make coverage-check
```

`make coverage` prints both raw coverage and coverage excluding generated
protobuf code under `internal/kiapi/gen/**`. `make coverage-check` fails if the
generated-excluded total drops below `COVERAGE_THRESHOLD`, defaulting to `75.0`.

Live integration tests are opt-in:

```sh
KICAD_API_SOCKET=ipc:///tmp/kicad/api.sock go test -tags=integration ./...
```

Generated-file validation through KiCad CLI is also opt-in:

```sh
KICAD_VALIDATE_GENERATED_FILES=1 \
KICAD_CLI=/path/to/kicad-cli \
go test -tags=integration ./internal/kicadfiles/design
```

Round-trip fixture validation:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1 \
KICADAI_ROUNDTRIP_ARTIFACT_DIR="$(pwd)/examples/roundtrip_artifacts" \
go test ./internal/kicadfiles/roundtrip
```

Set `KICADAI_KICAD_CLI` if `kicad-cli` is not on `PATH`.

ERC/DRC parser and fake-runner tests:

```sh
go test ./internal/kicadfiles/checks
go test ./internal/blocks/verification
```

Opt-in block ERC/DRC smoke tests with local KiCad:

```sh
KICADAI_RUN_KICAD_CLI=1 go test ./internal/blocks/verification -run TestOptionalKiCadBlockSmoke
```

The block smoke test is skipped by default and is intended for local proof, not
as a required CI dependency. It currently exercises selected protection and
oscillator block manifests and requires an explicit pass when enabled. Set
`KICADAI_KICAD_CLI` to a full path only when `kicad-cli` is not discoverable on
`PATH`.

Direct real ERC/DRC CLI smoke checks:

```sh
kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  --keep-artifacts \
  --artifact-dir ./examples/check_artifacts \
  check erc ./examples/checks/erc_fail/erc_fail.kicad_sch
```

## Protobuf Maintenance

Refresh vendored KiCad protobuf definitions intentionally:

```sh
make refresh-kicad-proto
```

Set `KICAD_REF=<commit-or-tag>` to refresh from a specific KiCad ref.

Install `protoc` and the Go generator before regenerating bindings:

```sh
brew install protobuf
GOBIN="$PWD/bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
make proto
make proto-check
make test
```

## Current Limitations

- Live KiCad IPC write support is not the primary mutation path yet.
- Direct readers/writers model a growing but incomplete subset of KiCad
  schematic and PCB syntax.
- The routing engine handles small deterministic grid-routing cases, not full
  production autorouting. It now reports route quality, rules, diagnostics, and
  length/search-pressure evidence, but it is still not a KiCad push-and-shove or
  dense-board router.
- Imported-project mutation blocks unsupported raw content to avoid damaging
  user-authored KiCad features.
- Operation feedback is strongest for transaction-derived issues. Generic KiCad
  CLI findings remain unlinked unless a unique operation trace exists.
- Generated-target repair mutation requires fresh `.kicadai/transaction.json`
  provenance referenced by `.kicadai/manifest.json`, or an explicit repair
  bundle transaction. Legacy generated projects without that file are treated as
  evidence-only until regenerated.
- Hierarchical pinmap validation is intentionally blocked until hierarchy
  flattening is implemented.
- Footprint-library expansion covers resolver-backed pads, text, graphics,
  attributes, metadata properties, and model references for generated and
  imported-project transaction apply. It does not yet preserve every advanced
  KiCad footprint node or pad-stack option.
- Export/BOM/fabrication packaging commands now produce readiness previews,
  deterministic BOM/CPL reports where evidence exists, and dry-run package
  manifests. They are readiness gates, not a complete manufacturer-release
  package yet.
- Real DRC execution still needs a stable known-good/known-bad fixture on the
  local KiCad CLI; parser and command paths are implemented.
- Windows named-pipe IPC support is not implemented.

## Troubleshooting

- `cannot dial` or timeout: KiCad is not running, the API is disabled, or
  `KICAD_API_SOCKET` points at the wrong endpoint.
- `AS_NOT_READY`: KiCad has started but is not ready. Wait for the editor to
  finish loading and retry.
- `AS_TOKEN_MISMATCH`: set the correct `KICAD_API_TOKEN` for the running KiCad
  instance.
- `AS_UNIMPLEMENTED` or `AS_UNHANDLED`: the running KiCad version does not
  implement the requested API command. Use direct-file generation for mutation.
- `draw-led-demo --execute` blocked by capabilities: expected when schematic
  write commands are unavailable in the generated API.
- `transaction apply` blocked by preservation conflict: the imported file
  contains KiCad constructs the writer does not model safely yet.
- `transaction validate --feedback` returns a nonzero exit for invalid example
  transactions by design. Inspect `data.feedback.operations[]` and
  `issues[].operation_id` to identify the operation to edit.
- `pinmap validate` blocked by hierarchy: validate child sheets directly or wait
  for hierarchy flattening support.
- Round-trip skipped: install `kicad-cli` or pass `--kicad-cli`.
- ERC/DRC check skipped: install `kicad-cli`, set `KICADAI_KICAD_CLI`, or pass
  `--kicad-cli`.
- ERC/DRC check returns findings: this is a design validation failure, not a
  tool failure. The JSON `data.checks[].summary.prompt` field is intended for
  compact AI repair context.
- `route request` returns blocked or partial: inspect `data.issues` and, from
  Go, `routing.DiagnosticsForResult`. Common fixes are moving components,
  reducing clearance/trace width, enabling a second layer or vias, and verifying
  pad layers/geometry.

## Design Direction

The project is moving toward an AI design loop:

1. Convert intent into structured transactions or higher-level generator
   requests.
2. Write KiCad-native project files deterministically.
3. Inspect and evaluate the result.
4. Run pinmap, round-trip, and KiCad CLI checks.
5. Produce review and fabrication-readiness reports.

The CLI is the current integration surface. MCP or other agent protocols can be
layered on later once the core tools are complete and reliable.
