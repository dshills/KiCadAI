# Development Reference

Examples, package map, testing, protobuf maintenance, current limits, troubleshooting, and design direction.

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
| `09_class_a_headphone_amp` | Class A headphone amplifier schematic fixture. |
| `10_opamp_buffer_headphone_amp` | Op-amp headphone-buffer schematic fixture. |
| `checks` | ERC/DRC fixture projects and report samples for KiCad-backed validation. |
| `blocks` | Circuit block library request files and generated schematic/project examples. |
| `transactions` | Transaction fixtures, including an invalid feedback example for AI repair loops. |

Open each KiCad project by opening the `.kicad_pro` file in its directory.

## Schematic Readability Gates

`internal/schematiclayout` provides deterministic readability checks for
generated and checked-in schematic examples.

- Standard example gates cover `01_led_indicator` through `05_sensor_node`.
  They require orthogonal wires, usable sheet placement, and no blocking
  standard readability diagnostics. Current parser geometry has known
  conservative symbol-body approximations, so tests keep the unavoidable
  pin-entry overlap exceptions scoped to individual examples.
- Strict amplifier gates cover `06_class_ab_headphone_amp`,
  `09_class_a_headphone_amp`, and `10_opamp_buffer_headphone_amp`. They enforce
  left-to-right signal flow, feedback above the active stage, positive rails
  above the signal lane, return/load symbols lower on the page, and no diagonal
  schematic wires.
- The schematic IR arbitrary-topology corpus writes and re-parses a
  deterministic suite across dense labeled fabric, feedback-ring, and
  fanout/branch graph families. Every fixture must pass strict post-write
  readability validation, with dense fabric selecting A3 and the more compact
  graph families selecting A4.
- The dense multi-pin corpus adds twelve deterministic `Sensor:Generic_I2C_8P`
  meshes with two to four endpoints per labeled net. It verifies that
  calibrated connection anchors and visual pin-facing directions remain
  distinct through layout, write, parse, electrical validation, and zero-warning
  readability checks. Its 96-symbol hierarchy companion forces the same mesh
  through child-sheet relayout and validates every emitted sheet independently.
- Oversized IR documents emit deterministic hierarchy child sheets. Explicit
  groups remain together when possible; a group that cannot fit a child page
  is split deliberately, with `partition_split_group_count` reported as layout
  evidence. Cross-sheet endpoints use collision-aware global-label stubs, and
  hierarchy tests cover transformed resolver-backed symbols and pin-owned
  no-connect markers.
- Direct local wires are emitted only for calibrated or explicitly requested
  endpoint contracts. Automatic nets that touch transformed symbols use the
  label path by default; all supported symbol rotation/mirror combinations for
  that path have KiCad ERC and zero-diff round-trip evidence.
- Workflow readability summaries are emitted by `design create` planning paths
  and include profile, pass/fail, component/routed-net counts, diagonal-wire
  count, stage-order violations, power-placement violations, diagnostic counts,
  decode errors, generated role evidence, `rule_profile`, `rule_count`,
  `repair_guidance_available`, and `repair_guidance_count`.
- Readability diagnostics now carry `repair` guidance when the diagnostic code
  maps to a known rule. The stable rule inventory is exposed by
  `internal/schematiclayout.RuleProfiles`, `ReadabilityRules`, `RuleByID`, and
  `RuleForDiagnostic`.

Useful focused commands:

```sh
go test ./internal/schematiclayout
go test ./internal/designworkflow -run Readability
go test ./internal/schematicir -run GeneratedArbitraryTopologyCorpus
go test ./internal/schematicir -run DenseMultiPinTopologyCorpus
go test ./internal/schematicir -run DenseMultiPinHierarchy
```

When a readability test fails, inspect the diagnostic code first. `diagonal_wire`
and amplifier-specific codes are layout failures. `symbol_overlap` and
`wire_symbol_overlap` can still be parser-geometry artifacts for imported
fixtures until exact KiCad text and symbol extents are modeled. Use the
diagnostic `repair` text as the first repair hint before changing schematic
writer geometry.


## Go Packages

Key packages:

- `cmd/kicadai`: CLI entrypoint.
- `internal/designworkflow`: deterministic project-generation orchestration.
  Its block-planned creation path has an explicit stage contract from planning
  through KiCad checks; a blocked stage marks only its declared downstream
  stages as skipped.
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
- `internal/aireadiness`: machine-readable AI generation readiness matrix
  loader, validator, and coverage summaries.

Generated protobuf packages under `internal/kiapi/gen/**` should not be used as
the AI workflow boundary. Prefer `internal/workflows`, `internal/transactions`,
or `internal/kicadfiles/designapi`.

AI generation coverage gaps are tracked in `data/ai-readiness/`; see
[AI Readiness Matrix](ai-readiness.md).


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

The required GitHub Actions workflow runs formatting, `go vet`, the declared
golangci-lint policy, `make test`, and `make coverage-check` for pull requests
and pushes to `main`. It is intentionally offline: KiCad-backed checks and
OpenAI provider execution remain opt-in local or release evidence, not required
CI dependencies.

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

## Error Boundaries

Use `%w` when a package-boundary error has a cause that a caller may classify
with `errors.Is` or `errors.As`, such as filesystem, parser, external-tool, or
typed validation failures. Keep independently formatted user-facing validation
diagnostics unwrapped when their structured `reports.Issue` code/path/severity
is the classification contract. Do not perform mechanical wrapping changes that
hide those diagnostics or alter a stable CLI message solely to increase a
wrapping metric.

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
  contains KiCad constructs the writer does not model safely yet, or the target
  is an existing imported project and `--allow-imported-apply` was not supplied.
  Always run `transaction plan` first and inspect `data.preservation` before
  opting into imported writes.
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
