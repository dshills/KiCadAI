# KiCadAI

KiCadAI is an early Go client for KiCad's IPC API. The first implementation establishes the Go project skeleton, CLI entrypoint, shared configuration rules, protobuf envelope client, and connection probes that later phases will use for document discovery and schematic automation.

## Current Phase

Phase 0 through Phase 11 are implemented:

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

Schematic automation is planned next.

## Requirements

- Go 1.22 or newer.
- KiCad 9.0 or newer will be required for future IPC work.

## Commands

```sh
go test ./...
go run ./cmd/kicadai --help
go run ./cmd/kicadai --json config
go run ./cmd/kicadai --json ping
go run ./cmd/kicadai --json version
go run ./cmd/kicadai --json documents
go run ./cmd/kicadai --json capabilities
go run ./cmd/kicadai --document / --json plan-led-demo
make proto
make proto-check
```

## Live KiCad Integration Tests

Normal tests do not require KiCad:

```sh
make test
```

To run live tests, start KiCad with the API enabled, set the socket endpoint, and use the `integration` build tag:

```sh
KICAD_API_SOCKET=ipc:///tmp/kicad/api.sock go test -tags=integration ./...
```

Common live-test failures are an API-disabled KiCad instance, a stale or wrong socket path, a token mismatch, multiple KiCad instances, or running against an endpoint without an open editor document.

## Vendored KiCad API Protos

KiCad's IPC API protobuf definitions are vendored under `third_party/kicad/api/proto` so Go code generation will be reproducible. The pinned upstream commit is recorded in `third_party/kicad/VERSION`.

To refresh the vendored proto files intentionally:

```sh
make refresh-kicad-proto
```

Set `KICAD_REF=<commit-or-tag>` to refresh from a different KiCad ref.

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
