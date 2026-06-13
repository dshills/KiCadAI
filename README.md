# KiCadAI

KiCadAI is a Go toolkit for AI-assisted KiCad design work. It currently combines
three related capabilities:

- a Go client for probing KiCad's live IPC API;
- direct KiCad project, schematic, and PCB file writers;
- CLI tools for generation, inspection, evaluation, round-trip validation,
  transactions, and pinmap readiness checks.

The practical near-term goal is to let agents build and review KiCad-native
projects from structured intent while keeping the lower-level writer strict,
deterministic, and testable.

## Status

The direct-file workflow is the main functional path today. It can generate
KiCad project directories, write root schematics and PCBs, inspect existing
projects, evaluate common correctness issues, apply a conservative subset of
transactions to imported projects, and validate symbol-to-footprint pinmaps.

The live KiCad IPC client is useful for connection probes, version checks,
document discovery, and capability reporting. Live schematic/PCB mutation
through the IPC API is still limited by the write commands exposed by the
currently generated KiCad API surface, so actual design generation is done by
writing KiCad files directly.

## Requirements

- Go 1.22 or newer.
- KiCad 9 or newer is the target for current file output and IPC protobufs.
- `kicad-cli` is optional, but recommended for round-trip checks and external
  KiCad validation.
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
go run ./cmd/kicadai --json pinmap list
go run ./cmd/kicadai --json pinmap validate ./examples/01_led_indicator
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
- `generate breakout`: generate a connector breakout project from a structured
  JSON request.

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
target, or applied.

```sh
go run ./cmd/kicadai --json transaction validate ./tx.json
go run ./cmd/kicadai --json transaction plan ./out/project ./tx.json
go run ./cmd/kicadai --json --overwrite transaction apply ./out/project ./tx.json
```

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
| `blocks` | Circuit block library request files and generated schematic/project examples. |

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
- `internal/transactions`: structured transaction validation, planning, and apply.
- `internal/generate`: higher-level project generators.
- `internal/inspect`: inspection reports.
- `internal/evaluate`: readiness and correctness evaluation.
- `internal/kicadfiles/roundtrip`: KiCad CLI round-trip validation.
- `internal/pinmap`: symbol-footprint-pinmap registry and validation.
- `internal/workflows`: AI-facing named workflow registry.

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
GOCACHE=/tmp/kicadai-gocache go test ./...
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
- Imported-project mutation blocks unsupported raw content to avoid damaging
  user-authored KiCad features.
- Hierarchical pinmap validation is intentionally blocked until hierarchy
  flattening is implemented.
- Footprint geometry is still generated from transaction payloads and defaults;
  there is not yet a full footprint-library expander.
- Export/BOM/fabrication packaging commands are placeholders.
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
- `pinmap validate` blocked by hierarchy: validate child sheets directly or wait
  for hierarchy flattening support.
- Round-trip skipped: install `kicad-cli` or pass `--kicad-cli`.

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
