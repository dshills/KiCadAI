# KiCadAI

KiCadAI is a Go toolkit for AI-assisted KiCad design work. It currently combines
three related capabilities:

- a Go client for probing KiCad's live IPC API;
- direct KiCad project, schematic, and PCB file writers;
- CLI tools for generation, inspection, evaluation, connectivity-first board
  validation, ERC/DRC feedback, round-trip validation, transactions,
  operation-correlated feedback, component intelligence, and pinmap readiness
  checks.

The practical near-term goal is to let agents build and review KiCad-native
projects from structured intent while keeping the lower-level writer strict,
deterministic, and testable.

## Status

The direct-file workflow is the main functional path today. It can generate
KiCad project directories, write root schematics and PCBs, inspect existing
projects, evaluate common correctness issues, apply a conservative subset of
transactions to imported projects, place components, route small PCB nets,
validate symbol-to-footprint pinmaps, select catalog-backed components, run
connectivity-first PCB validation, and run KiCad-backed ERC/DRC checks through
`kicad-cli`. Transaction validation, planning, and apply results carry stable
operation IDs where possible so AI agents can connect issues back to the source
operation they need to repair.

The live KiCad IPC client is useful for connection probes, version checks,
document discovery, and capability reporting. Live schematic/PCB mutation
through the IPC API is still limited by the write commands exposed by the
currently generated KiCad API surface, so actual design generation is done by
writing KiCad files directly.

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

Run the CLI from source:

```sh
go run ./cmd/kicadai --help
```

Build a local binary:

```sh
go build -o bin/kicadai ./cmd/kicadai
```

Generate a simple LED project without contacting KiCad:

```sh
go run ./cmd/kicadai \
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
go run ./cmd/kicadai --json config
go run ./cmd/kicadai --json ping
go run ./cmd/kicadai --json version
go run ./cmd/kicadai --json documents
go run ./cmd/kicadai --json capabilities
```

Connection flags:

```sh
go run ./cmd/kicadai \
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
go run ./cmd/kicadai --json config
go run ./cmd/kicadai --json ping
go run ./cmd/kicadai --json version
go run ./cmd/kicadai --json documents
go run ./cmd/kicadai --json capabilities
go run ./cmd/kicadai --json inspect project ./examples/07_generated_pcb
go run ./cmd/kicadai --json evaluate project ./examples/07_generated_pcb
go run ./cmd/kicadai --json writer check ./examples/07_generated_pcb
go run ./cmd/kicadai --json validate board ./examples/07_generated_pcb
go run ./cmd/kicadai --json check erc ./examples/checks/erc_fail/erc_fail.kicad_sch
go run ./cmd/kicadai --json check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
go run ./cmd/kicadai --json component find --family resistor --package 0805 --value-kind resistance --value 10k
go run ./cmd/kicadai --json pinmap list
go run ./cmd/kicadai --json pinmap validate ./examples/01_led_indicator
go run ./cmd/kicadai --json --request ./examples/placement/simple_request.json place request
go run ./cmd/kicadai --json --request ./examples/routing/simple_request.json route request
go run ./cmd/kicadai --json --feedback transaction validate ./examples/transactions/invalid_feedback.json
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
  intent through schematic/PCB write, validation, and feedback.
- `writer check`: verify generated project, schematic, PCB, net, footprint,
  pad, copper, zone, and optional KiCad round-trip writer correctness.
- `generate breakout`: generate a connector breakout project from a structured
  JSON request.
- `block realize-pcb <block_id>`: instantiate a circuit block and return its
  PCB realization fragment plus the placement request that can feed the
  placement engine.
- `component list|show|find|select|validate`: inspect the curated component
  catalog, choose symbol/footprint bindings, and enforce confidence gates.

LED generation:

```sh
go run ./cmd/kicadai \
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
go run ./cmd/kicadai \
  --json \
  --request ./request.json \
  --output ./out/sensor_breakout \
  --overwrite \
  generate breakout
```

Generated projects are written through safe directory handling. `--overwrite`
is required to replace an existing output directory.

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
go run ./cmd/kicadai --json component list
go run ./cmd/kicadai --json component show resistor.generic.0805
go run ./cmd/kicadai --json component find --family resistor --package 0805 --value-kind resistance --value 10k
go run ./cmd/kicadai --json --request ./examples/components/select_resistor.json component select
go run ./cmd/kicadai --json component validate
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
go run ./cmd/kicadai \
  --json \
  --request ./examples/design/led_indicator.json \
  --output ./out/led_indicator \
  --overwrite \
  design create
```

Useful flags:

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

Current gaps for autonomous one-shot schematic + PCB generation:

- request planning is explicit block composition, not natural-language block
  selection;
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
USB-C power input, and a minimal ATmega328P-A system. Blocks can be listed,
inspected, validated, instantiated, composed, and now realized as PCB
fragments.

```sh
go run ./cmd/kicadai --json block list
go run ./cmd/kicadai --json block show led_indicator
go run ./cmd/kicadai --json --request ./examples/blocks/requests/led_indicator.json block instantiate led_indicator
go run ./cmd/kicadai --json --request ./examples/blocks/requests/led_indicator.json block realize-pcb led_indicator
```

`block realize-pcb` returns:

- the normal block instantiation output with schematic transactions;
- `realization.components[]` with refs, footprints, role names, and relative
  PCB placements;
- `realization.local_routes[]` and route operations for verified local nets;
- `placement_request`, a ready input for the placement engine.

This is the first PCB-fragment layer for the circuit block library. It does not
yet claim fabrication readiness for complete boards; global block composition,
board outline selection, route conflict resolution, zone refill, and KiCad DRC
evidence are still required before generated block PCBs should be treated as
manufacturing candidates.

### Placement

The placement engine accepts a structured board placement request and returns
placed components, geometry issues, metrics, and `place_footprint` transaction
operations for successful placements.

```sh
go run ./cmd/kicadai \
  --json \
  --request ./examples/placement/simple_request.json \
  place request
```

Current placement support includes fixed components, top/bottom side
constraints, edge preferences, keepouts, component spacing, group anchors,
group spread checks, HPWL metrics, footprint-derived bounds helpers, and
transaction operation output. Requests can use explicit component bounds or
hydrate bounds from the library resolver in Go before calling
`placement.Place`. The usable board area uses the larger of
the JSON `board.margin_mm` and `rules.board_edge_clearance_mm` values
(`Board.MarginMM` and `Rules.BoardEdgeClearanceMM` in Go).

### Routing

The routing engine is a deterministic grid router for small placed boards. It
accepts a structured `routing.Request`, routes intentional nets, validates
connectivity and clearance in-process, and returns route segments, vias,
metrics, issues, AI-facing repair diagnostics, and route-shaped operations.

Run routing from JSON:

```sh
go run ./cmd/kicadai \
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
go run ./cmd/kicadai \
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
operation emission, KiCad check feedback mapping, golden routed examples, and
bounded stress tests.

Current routing limitations:

- The router is intended for simple boards and early AI workflow validation, not
  dense BGA or production autorouting.
- Routes are orthogonal grid paths; there is no diagonal, curved, length-tuned,
  differential-pair, or impedance-aware routing yet.
- Placement quality strongly affects routing success.
- Copper zones are treated as obstacles or unsupported policy inputs; zone-fill
  aware routing is not implemented.
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
go run ./cmd/kicadai --json writer check ./examples/07_generated_pcb/generated_pcb.kicad_pcb
go run ./cmd/kicadai --json writer check --strict-diffs --allow-unrouted ./examples/07_generated_pcb
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
go run ./cmd/kicadai --json inspect project ./examples/07_generated_pcb
go run ./cmd/kicadai --json inspect schematic ./examples/01_led_indicator/led_indicator.kicad_sch
go run ./cmd/kicadai --json inspect pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
```

Inspection reports summarize discovered files, symbol counts, footprint counts,
and reader issues.

### Evaluation

Evaluate projects and files for generated-output readiness:

```sh
go run ./cmd/kicadai --json evaluate project ./examples/07_generated_pcb
go run ./cmd/kicadai --json evaluate schematic ./examples/01_led_indicator/led_indicator.kicad_sch
go run ./cmd/kicadai --json evaluate pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
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
go run ./cmd/kicadai --json transaction validate ./tx.json
go run ./cmd/kicadai --json transaction plan ./out/project ./tx.json
go run ./cmd/kicadai --json --overwrite transaction apply ./out/project ./tx.json
```

For AI repair loops, add `--feedback` to validation or planning. The command
keeps the raw issue list and also returns a grouped `feedback` object keyed by
stable operation IDs.

```sh
go run ./cmd/kicadai --json --feedback transaction validate ./examples/transactions/invalid_feedback.json
go run ./cmd/kicadai --json --feedback transaction plan ./out/invalid_feedback ./examples/transactions/invalid_feedback.json
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
go run ./cmd/kicadai --json pinmap list
```

Validate a project:

```sh
go run ./cmd/kicadai --json pinmap validate ./examples/01_led_indicator
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

go run ./cmd/kicadai --json library symbol Device:R
go run ./cmd/kicadai --json library footprint Resistor_SMD:R_0805_2012Metric
go run ./cmd/kicadai --json library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric
go run ./cmd/kicadai --json library pinmap-candidate Device:R Resistor_SMD:R_0805_2012Metric
go run ./cmd/kicadai --json library templates
```

Hardened symbol inspection commands expose resolver evidence without requiring
agents to read raw `.kicad_sym` files:

```sh
go run ./cmd/kicadai --json library symbols list
go run ./cmd/kicadai --json library symbols show Device:R
go run ./cmd/kicadai --json library symbols pins Device:R
go run ./cmd/kicadai --json library symbols validate Device:R
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
go run ./cmd/kicadai --json block list
go run ./cmd/kicadai --json block show led_indicator
go run ./cmd/kicadai \
  --json \
  --request examples/blocks/requests/led_indicator.json \
  --output ./out/led_indicator \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
go run ./cmd/kicadai \
  --json \
  --request examples/blocks/requests/composed_sensor_breakout.json \
  --output ./out/composed_sensor_breakout \
  --name composed_sensor_breakout \
  --overwrite \
  block compose
```

Current built-in blocks include LED indicator, connector breakout, voltage
regulator, I2C sensor, op-amp gain stage, USB-C power input, and MCU minimal
system. The generated block examples are structural schematic/project outputs;
they are not yet fabrication-ready PCB designs. See
[docs/circuit-block-library.md](docs/circuit-block-library.md) for request
formats, verification levels, resolver requirements, examples, AI usage, and
known limitations. The current release-readiness gap matrix is in
[docs/circuit-block-readiness.md](docs/circuit-block-readiness.md).

### Round-Trip Validation

Round-trip commands use `kicad-cli` to save or normalize files and compare the
result:

```sh
go run ./cmd/kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  roundtrip schematic ./examples/01_led_indicator/led_indicator.kicad_sch

go run ./cmd/kicadai --json roundtrip pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
go run ./cmd/kicadai --json roundtrip project ./examples/07_generated_pcb
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
go run ./cmd/kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  check erc ./examples/checks/erc_fail/erc_fail.kicad_sch

go run ./cmd/kicadai --json check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
go run ./cmd/kicadai --json check project ./examples/checks/drc_pass
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

### Export Skeleton

The `export` command family exists as a structured CLI placeholder for future
review, BOM, and fabrication-package outputs:

```sh
go run ./cmd/kicadai --json export preview ./project
go run ./cmd/kicadai --json export bom ./project
go run ./cmd/kicadai --json export fabrication ./project
```

These subcommands currently return `UNSUPPORTED_OPERATION`.

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
```

Real ERC/DRC CLI smoke checks:

```sh
go run ./cmd/kicadai \
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
  production autorouting.
- Imported-project mutation blocks unsupported raw content to avoid damaging
  user-authored KiCad features.
- Operation feedback is strongest for transaction-derived issues. Generic KiCad
  CLI findings remain unlinked unless a unique operation trace exists.
- Hierarchical pinmap validation is intentionally blocked until hierarchy
  flattening is implemented.
- Footprint-library expansion covers resolver-backed pads, text, graphics,
  attributes, metadata properties, and model references for generated and
  imported-project transaction apply. It does not yet preserve every advanced
  KiCad footprint node or pad-stack option.
- Export/BOM/fabrication packaging commands are placeholders.
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
