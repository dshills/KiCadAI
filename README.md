# KiCadAI

KiCadAI is an early Go client for KiCad's IPC API. The first implementation establishes the Go project skeleton, CLI entrypoint, shared configuration rules, protobuf envelope client, and connection probes that later phases will use for document discovery and schematic automation.

## Current Status

Implementation phases are complete through Phase 14, with Phase 15 documented as a roadmap. The current functional surface is:

- Go module and package layout.
- CLI entrypoint at `cmd/kicadai`.
- Shared connection configuration package.
- Baseline tests and Make targets.
- Vendored KiCad API protobuf definitions pinned to an upstream commit.
- Generated Go protobuf bindings under `internal/kiapi/gen`.
- IPC transport abstraction with fake and Mangos-backed request/reply implementations.
- Low-level KiCad protobuf envelope client with token capture and API status errors.
- CLI `ping` and `version` probes.
- Open document discovery and CLI `documents` listing.
- Optional live KiCad integration test harness.
- Capability detection for schematic read versus missing schematic write commands.
- Schematic domain request types and validation for planned symbol, wire, and label operations.
- Deterministic LED demo planning and `plan-led-demo` CLI output.
- LED demo execution boundary and `draw-led-demo --execute`, currently blocked by missing schematic write capability.
- AI-ready workflow registry with safe named operations and structured validation issues.
- Developer setup, troubleshooting, integration-test, and protobuf regeneration docs.

Actual schematic mutation remains gated until KiCad exposes compatible schematic write commands in the generated API surface.

## Roadmap

Phase 15 records follow-up work in the [Phase 15 Roadmap](specs/initial/FUTURE.md). Those items are outside the current implementation scope.

## AI Workflow Boundary

Future AI-generated design logic should call named workflow operations in `internal/workflows`, not generated protobuf packages or transport clients directly. The safe registry currently exposes:

- `create_led_indicator` implemented as a deterministic LED schematic plan.
- `place_decoupling_capacitor` reserved for a future workflow.
- `create_connector_block` reserved for a future workflow.

Use `workflows.PlanOperation` with a structured `{operation, payload}` request envelope and inspect `OperationResult.Issues` before attempting execution. Go callers can use `workflows.NewCreateLEDIndicatorRequest` to build the initial implemented payload.

## Requirements

- Go 1.22 or newer.
- A KiCad build exposing the vendored IPC API shape; the current generated bindings target KiCad 9+ API definitions.
- A running KiCad instance with the IPC API enabled for live probes.

The default Unix IPC endpoint is `ipc:///tmp/kicad/api.sock`. Windows named pipes are not implemented yet, so Windows users must pass an explicit endpoint once support is added.

## KiCad API Setup

Enable KiCad's API in KiCad, then open a project and the editor you want to inspect. For schematic automation work, open the schematic editor before running `documents`, `capabilities`, or LED demo commands.

Start with the resolved config:

```sh
go run ./cmd/kicadai --json config
```

Pass connection settings with flags:

```sh
go run ./cmd/kicadai --socket ipc:///tmp/kicad/api.sock --token "$KICAD_API_TOKEN" --json ping
```

Or with environment variables:

```sh
export KICAD_API_SOCKET=ipc:///tmp/kicad/api.sock
export KICAD_API_TOKEN=your-token-if-required
export KICAD_CLIENT_NAME=kicadai-dev
export KICAD_TIMEOUT_MS=5000
```

The client captures KiCad's returned token in memory when no token is configured. It does not persist tokens to disk; set `KICAD_API_TOKEN` yourself if a later process needs to reuse a token. Tokens are redacted from `config` output.

## Commands

```sh
go run ./cmd/kicadai --help
go run ./cmd/kicadai --json config
go run ./cmd/kicadai --json ping
go run ./cmd/kicadai --json version
go run ./cmd/kicadai --json documents
go run ./cmd/kicadai --json capabilities
go run ./cmd/kicadai --document / --json plan-led-demo
go run ./cmd/kicadai --document / --execute --json draw-led-demo
```

`plan-led-demo` is deterministic and does not mutate KiCad. `draw-led-demo --execute` currently performs capability preflight and returns a structured failure when schematic write commands are unavailable in the generated API.

## Testing

Normal tests do not require KiCad:

```sh
make test
```

`make test` wraps `go test ./...` with workspace-local Go cache paths.

Generated protobuf output can be checked with:

```sh
make proto
make proto-check
```

## Live KiCad Integration Tests

To run live tests, start KiCad with the API enabled, set the socket endpoint, and use the `integration` build tag:

```sh
KICAD_API_SOCKET=ipc:///tmp/kicad/api.sock go test -tags=integration ./...
```

Live tests are skipped unless `KICAD_API_SOCKET` is set.

## Troubleshooting

- `cannot dial` or connection timeout: KiCad is not running, the API is disabled, or `KICAD_API_SOCKET` points at the wrong endpoint. Verify with `go run ./cmd/kicadai --json config`, then pass `--socket` explicitly.
- `AS_NOT_READY`: KiCad has started but is not ready to service API requests. Wait a moment, make sure the project/editor has finished loading, then retry.
- `AS_TOKEN_MISMATCH`: The configured token does not match the KiCad instance. For repeated CLI commands, get the token for the running KiCad instance and set `KICAD_API_TOKEN`; in-memory capture only helps within a single long-lived Go client process.
- Multiple KiCad instances: each instance may use a different socket and token. Use explicit `KICAD_API_SOCKET` and avoid relying on defaults while more than one instance is open.
- Wrong endpoint or no open editor: `documents` should show the expected schematic or PCB document. Open the schematic editor and rerun `go run ./cmd/kicadai --json documents`.
- Schematic commands unavailable: `capabilities` may report schematic read support while `schematic.write` and symbol placement remain missing. In that state, `plan-led-demo` works and `draw-led-demo --execute` returns a structured preflight failure instead of mutating KiCad.
- `AS_UNIMPLEMENTED` or `AS_UNHANDLED`: the running KiCad version does not implement the requested command. Check `version` and `capabilities`, then keep the workflow in planning mode.
- `AS_BUSY` or timeout: KiCad is doing another operation. Retry after the editor is idle or increase `KICAD_TIMEOUT_MS`.

## Vendored KiCad API Protos

KiCad's IPC API protobuf definitions are vendored under `third_party/kicad/api/proto` so Go code generation will be reproducible. The pinned upstream commit is recorded in `third_party/kicad/VERSION`.

To refresh the vendored proto files intentionally:

```sh
make refresh-kicad-proto
```

Set `KICAD_REF=<commit-or-tag>` to refresh from a different KiCad ref.

After refreshing, run:

```sh
make proto
make proto-check
make test
```

## Protobuf Generation

Generated Go bindings are committed under `internal/kiapi/gen`. The generator uses explicit `protoc` mappings because KiCad's upstream proto files do not declare Go package options.

Install the `protoc` compiler first. On macOS:

```sh
brew install protobuf
```

Install the pinned generator plugin into the workspace-local `bin` directory:

```sh
GOBIN="$PWD/bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
```

Then regenerate:

```sh
make proto
```

## KiCad Configuration

The CLI uses these environment variables:

```text
KICAD_API_SOCKET
KICAD_API_TOKEN
KICAD_CLIENT_NAME
KICAD_TIMEOUT_MS
```

Tokens are redacted from CLI output.

Connection precedence is flag first, environment second, platform default last. Any socket string without a scheme receives the `ipc://` prefix. Use absolute socket paths for reliable behavior from any working directory; if you must use a relative socket, prefer an explicit form such as `./api.sock`, which normalizes to `ipc://./api.sock`.
