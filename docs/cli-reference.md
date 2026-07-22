# CLI Reference

Command examples and live KiCad IPC setup details for the compiled `kicadai` binary.

## KiCad IPC Setup

Live IPC commands require KiCad to be running with the API enabled. Open the
project/editor you want to inspect, then run:

```sh
kicadai config
kicadai ping
kicadai version
kicadai documents
kicadai capabilities
kicadai capability generation --json
```

Connection flags:

```sh
kicadai \
  --socket ipc:///tmp/kicad/api.sock \
  --token "$KICAD_API_TOKEN" \
  --timeout-ms 5000 \
  ping
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

Structured project-analysis and generation commands return JSON by default.
Use `--format text` only for commands that expose a human-readable summary.
The legacy `--json` flag remains accepted as a compatibility alias for
`--format json`.

JSON mode writes exactly one JSON document to stdout on both success and
failure. Logs and tool diagnostics use stderr, so stdout can be passed directly
to a JSON decoder. When a command has more than 128 issues, the envelope emits
a deterministic 128-issue sample and a `diagnostics` summary with exact total,
emitted, omitted, and group counts. A circuit creation attempt whose output
directory already exists also retains the complete list in
`.kicadai/diagnostics.json` and reports that file as a `diagnostics_report`
artifact.

```sh
kicadai config
kicadai ping
kicadai version
kicadai documents
kicadai capabilities
kicadai inspect project ./examples/07_generated_pcb
kicadai evaluate project ./examples/07_generated_pcb
kicadai evaluate schematic ./examples/01_led_indicator/led_indicator.kicad_sch
kicadai writer check ./examples/07_generated_pcb
kicadai validate board ./examples/07_generated_pcb
kicadai check erc ./examples/checks/erc_fail/erc_fail.kicad_sch
kicadai check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
kicadai fabrication profile list
kicadai fabrication profile show generic_assembly
kicadai fabrication profile validate ./profiles/my-board-house.json
kicadai component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai pinmap list
kicadai pinmap validate ./examples/01_led_indicator
kicadai --request ./examples/placement/simple_request.json place request
kicadai --request ./examples/routing/simple_request.json route request
kicadai --request ./examples/repair/missing_footprint_stage_issues.json repair plan
kicadai --request ./examples/intent/sensor_breakout.json --output ./out/intent_plan --overwrite intent plan
kicadai --request ./examples/intent/sensor_breakout.json intent explain
kicadai --text "make a 3.3V I2C temperature sensor breakout" intent rationale
kicadai --file ./behavioral-request.txt \
  --provider openai --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-request intent compile
kicadai --request ./examples/intent/sensor_breakout.json --output ./out/intent_sensor --overwrite intent create
kicadai --prompt-file ./examples/ai/usb_c_bmp280_breakout/prompt.txt \
  --provider recorded \
  --provider-record ./examples/ai/usb_c_bmp280_breakout/recorded-response.json \
  --output ./out/ai_usb_c_bmp280 --overwrite design create
kicadai --prompt-file ./examples/ai/usb_c_led_indicator_protected/prompt.txt \
  --provider recorded \
  --provider-record ./examples/ai/usb_c_led_indicator_protected/recorded-response.json \
  --output ./out/ai_usb_c_led_protected --overwrite design create
kicadai --prompt-file ./examples/ai/generic_rc_filter/prompt.txt \
  --provider recorded \
  --provider-record ./examples/ai/generic_rc_filter/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  --output ./out/ai_generic_rc --overwrite design create
kicadai --request ./examples/schematic-ir/led_indicator.json schematic-ir validate
kicadai --request ./examples/schematic-ir/led_indicator.json schematic-ir normalize
kicadai --request ./examples/schematic-ir/led_indicator.json schematic-ir transaction
kicadai --request ./examples/schematic-ir/led_indicator.json --output ./out/ir_led --overwrite schematic-ir write
kicadai --target ./out/project --request ./examples/repair/missing_footprint_stage_issues.json repair export-bundle
kicadai --execute --overwrite --target ./out/project --request ./examples/repair/missing_footprint_stage_issues.json repair export-bundle
# For integrations that already produce a generated repair bundle:
kicadai --execute --overwrite --target ./out/project --request ./path/to/generated-repair-bundle.json repair apply
# Generate/apply a repair bundle during design create, then replay that saved
# bundle later for reproducible target-apply validation:
kicadai --request ./examples/design/led_indicator.json --output ./out/led_indicator --overwrite --repair-apply --skip-routing design create
kicadai --execute --overwrite --target ./out/led_indicator --request ./out/led_indicator/.kicadai/repair-bundle.json repair apply
kicadai --feedback transaction validate ./examples/transactions/invalid_feedback.json
```

### Provider-Backed Design Creation

`design create` accepts either a deterministic `--request` or one provider
input from `--prompt`/`--prompt-file`. Recorded mode is hermetic and suitable
for CI; OpenAI mode requires `OPENAI_API_KEY` and uses strict Structured
Outputs. Bounded semantic dispatch supports the promoted BMP280 and protected
LED profiles. Explicit `--ai-profile generic-circuit-v1` instead requests the
strict catalog-resolved graph contract; it never falls back to a bounded
profile or accepts provider-defined libraries and geometry. See
[AI Generation](ai-generation.md) for strict KiCad-backed commands and the
artifact contract.

### Behavioral Intent Compilation

`intent compile` is the provider-backed, behavior-first trust boundary. Its
input may specify observable behavior, interfaces, operating cases, ranges,
tolerances, safety limits, and manufacturing-neutral board bounds. Provider
output may not select topology, parts, pins, nets, coordinates, layers, routes,
models, solver controls, or validation evidence.

```sh
kicadai \
  --file ./behavioral-request.txt \
  --provider openai \
  --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-request \
  intent compile
```

The command returns one terminal compilation status:

- `ready`: deterministic architecture search and hash-bound trusted closed-loop
  evidence passed; an executable strict v3 requirement and
  `.kicadai/behavioral-design-request.json` are persisted.
- `needs_clarification`: the smallest blocking question is persisted in
  `.kicadai/behavioral-follow-up-template.json`; no executable design request
  is released.
- `unsupported`: stable semantic capability-gap evidence is persisted; no
  guessed design is emitted.
- `invalid`: strict schema, source coverage, uncertainty, hash binding, or
  provider-boundary validation failed.

To answer a clarification, edit only the template's `answer` fields and rerun
against the complete original source and same output directory:

```sh
kicadai \
  --file ./behavioral-request.txt \
  --provider openai \
  --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-request \
  --follow-up ./out/behavioral-request/.kicadai/behavioral-follow-up-template.json \
  --overwrite \
  intent compile
```

After a `ready` result, create and validate the selected design through the
normal deterministic workflow:

```sh
kicadai \
  --request ./out/behavioral-request/.kicadai/behavioral-design-request.json \
  --output ./out/behavioral-project --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

The compilation directory also retains the original source, installed semantic
capabilities, provider proposal and attempts, compilation, architecture search,
and full closed-loop report for replay and audit.

Promotion harnesses may pass `--promotion-readiness expected_fail|candidate|pass|blocked`
to declare the expected readiness recorded in the promotion report. This flag
does not change achieved readiness or bypass any workflow, ERC, DRC, routing,
writer, simulation, or round-trip gate. Recorded replay commands preserve it.

Before asking an agent for a generic circuit, query its installed contract:

```sh
kicadai capability generation --json
```

Use `data.capabilities` for generic versus bounded profile distinctions,
limitations, and required evidence. Use `data.generic_graph_contract` as the
catalog-resolved component/function vocabulary for `generic-circuit-v1`.
Use `data.function_level_contract` for the registry-backed function-level
operation names, parameters, endpoint roles, unit conventions, readiness
limits, and unsupported claims. The complete provider-free workflow is in
[Function-Level Circuit Workflow](function-level-circuits.md).
Unsupported graph data must be treated as a fail-closed preflight result; no
KiCad project should be written after a blocking diagnostic.

### Generic Circuit Preflight

Agents can validate a `generic-circuit-v1` graph before invoking a provider or
writing a KiCad project:

```sh
kicadai circuit --help
kicadai circuit preflight --help
kicadai circuit create --help
kicadai capability generation --json
kicadai --request ./graph.json circuit preflight
kicadai --request ./corrected-graph.json circuit preflight
kicadai --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  circuit create --request ./corrected-graph.json --output ./out/project --overwrite
```

The three help forms exit successfully, describe their actual registered
flags, and end with a concrete next command. They do not parse or validate a
circuit request.

`circuit preflight` is read-only. It returns normalized graph and catalog
resolution evidence, the lowered request, schematic validation, deterministic
placement/routing plans, stage-tagged diagnostics, and `ready_for_write`.
`kicad_erc` and `kicad_drc` gates are reported as external evidence rather than
claimed by preflight.

`circuit create` consumes the same strict decode, catalog resolution, lowering,
placement, routing, and diagnostics used by preflight. It refuses to create an
output directory for a graph that is not `ready_for_write`. Successful output
includes writer-correctness and internal-connectivity evidence. To require
authoritative KiCad evidence, add `--kicad-cli`, `--require-erc`,
`--require-drc`, `--require-kicad-roundtrip`, and `--strict-diffs` as needed.
Those checks remain environment-gated and are never inferred from preflight.

### Generic Circuit Patch

Agents can repair a rejected graph with the bounded, provider-free patch
contract, then preflight and create the corrected graph:

```sh
kicadai circuit patch --request ./broken-graph.json \
  --patch ./changes.json --output ./corrected-graph.json
kicadai --request ./corrected-graph.json circuit preflight
kicadai --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  circuit create --request ./corrected-graph.json --output ./out/project --overwrite
```

`circuit patch` accepts only typed component selector, endpoint, explicit
no-connect, PCB placement/region, and policy corrections. It fails closed on
unknown operations or immutable graph changes, writes no KiCad files, and only
writes the corrected graph after the shared preflight gates pass. Its JSON
result includes the input graph hash, normalized operations, before/after
critical graph projection, changed paths, and re-preflight evidence. KiCad
ERC/DRC and normalized KiCad round-trip remain external requirements until
explicitly invoked during creation or promotion.

When a failed preflight has an unambiguous safe correction, its JSON
`data.repair_options` contains the candidates. Each option identifies the
source diagnostic, provides a partial `operation_template`, lists
`required_values` and `allowed_values`, and includes the rationale, stage, and
retry scope. Construct a standard
`kicadai.circuit-patch.v1` document from one option, then let `circuit patch`
perform final validation. Absence of an option is intentional: ambiguous,
electrical, thermal, fabrication, ERC/DRC, and immutable-identity findings
require review rather than an executable repair.

### Generic Circuit Repair Plan

```sh
kicadai circuit repair-plan --request ./broken-graph.json
```

This read-only command returns preflight evidence plus `data.plan`. `ready`
means the graph already passes preflight. `repair_available` contains one
validated patch for the caller to save and pass to `circuit patch`.
`needs_review` means zero or competing candidates exist. `blocked` reports
malformed input, a repeated normalized graph hash, or the attempt ceiling.
Supply prior hashes through repeatable `--previous-hash` for multi-step agent
loops. The command never writes a corrected graph or KiCad project. Routing,
electrical, thermal, safety, fabrication, and external KiCad findings are
review boundaries, not executable repairs.

Provider output budgets are profile-aware: 8,192 tokens for bounded reference
profiles and 32,768 for `generic-circuit-v1`. Override them with
`--ai-max-output-tokens N` or `KICADAI_AI_MAX_OUTPUT_TOKENS`; the CLI flag wins
and the accepted range is 1,024 through 65,536. Token exhaustion is reported as
`AI_PROVIDER_INCOMPLETE` at `provider.max_output_tokens` with usage and manual
retry guidance. It never causes an automatic paid retry.

After envelope decode, provider-backed runs write the strict, secret-free
`.kicadai/ai-provider-replay.json` artifact and return an exact replay command
in `data.provider.replay_command`. The replay artifact supplies its profile, so
the emitted recorded-provider command needs no prompt and makes no network
request. Plain fixture envelopes remain backward compatible.

### Schematic IR Commands

`schematic-ir` is the first AI-facing schematic design/layout IR entry point.
It accepts strict JSON via `--request` and returns structured JSON by default.
The IR separates component/net intent from layout intent and repair policy, then
lets KiCadAI validate and normalize the result before translation to schematic
transactions.

```sh
kicadai --request ./examples/schematic-ir/led_indicator.json schematic-ir validate
kicadai --request ./examples/schematic-ir/led_indicator.json schematic-ir normalize
kicadai --request ./examples/schematic-ir/led_indicator.json schematic-ir transaction
kicadai --request ./examples/schematic-ir/led_indicator.json --output ./out/ir_led --overwrite schematic-ir write
```

Subcommands:

- `validate`: strict-decode and validate the IR, returning summary and issues.
- `normalize`: return normalized layout groups and placements for readable
  schematic generation.
- `transaction`: return the existing KiCadAI transaction stream generated from
  normalized IR.
- `write`: write a schematic-only KiCad project directory from normalized IR.
  This emits `.kicad_pro` and `.kicad_sch` files, preserving schematic
  readability checks while leaving PCB routing to later workflows.

Checked-in examples currently cover an LED indicator, USB-C powered LED
indicator, and I2C sensor with 3.3 V regulator breakout. The v1 IR is intentionally
bounded: it is not a free-form language parser and does not yet guarantee
arbitrary schematic perfection.

### Evaluation Commands

Use evaluation commands for read-only readiness checks. Schematic and project
evaluation include `schematic_validation` and `schematic_electrical` checks when
a root schematic is available.

```sh
kicadai evaluate project ./examples/07_generated_pcb
kicadai evaluate schematic ./examples/01_led_indicator/led_indicator.kicad_sch
kicadai evaluate pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
```

Use `evaluate pcb` for read-only PCB file readiness checks. Use
`validate board` for generated-board electrical connectivity validation on a
project directory.

### Fabrication Profile Commands

Fabrication profile commands return JSON by default and are intended for
agent/tooling discovery:

```sh
kicadai fabrication profile list
kicadai fabrication profile show generic_assembly
kicadai fabrication profile validate ./profiles/my-board-house.json
```

Built-in physical-rule profiles are `generic_assembly`,
`generic_2layer_economy`, `generic_2layer_standard`,
`generic_4layer_standard`, and `generic_castellated_review`. Load trusted
local JSON profiles with:

```sh
kicadai --manufacturer-profile-dir ./profiles fabrication profile list
KICADAI_FABRICATION_PROFILE_DIR=./profiles kicadai fabrication profile list
```

Use `--manufacturer-profile <id>` with `export preview` or
`export fabrication` to select the active physical-rule profile. Exported
readiness reports, `physical-rules.json`, and `package-manifest.json` record
the resolved profile ID, version, source, and hash.

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

Example:

```sh
kicadai --request ./examples/design/led_indicator.json --output ./out/led_indicator --overwrite design create
```

The resulting workflow JSON includes a `schematic_electrical` stage before PCB
realization.

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
  --target ./out/led_indicator \
  --request ./examples/repair/missing_footprint_stage_issues.json \
  repair export-bundle

kicadai \
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
