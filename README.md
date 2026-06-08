# KiCadAI

KiCadAI is an early Go client for KiCad's IPC API. The first implementation phase establishes the Go project skeleton, CLI entrypoint, and shared configuration rules that later phases will use for protobuf generation, IPC transport, document discovery, and schematic automation.

## Current Phase

Phase 0 through Phase 2 are implemented:

- Go module and package layout.
- CLI entrypoint at `cmd/kicadai`.
- Shared connection configuration package.
- Baseline tests and Make targets.
- Vendored KiCad API protobuf definitions pinned to an upstream commit.
- Generated Go protobuf bindings under `internal/kiapi/gen`.

The client does not connect to KiCad yet. Active KiCad connectivity starts after IPC transport and envelope client work are added in later phases.

## Requirements

- Go 1.22 or newer.
- KiCad 9.0 or newer will be required for future IPC work.

## Commands

```sh
go test ./...
go run ./cmd/kicadai --help
go run ./cmd/kicadai --json config
make proto
make proto-check
```

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

Future phases will use these environment variables:

```text
KICAD_API_SOCKET
KICAD_API_TOKEN
KICAD_CLIENT_NAME
KICAD_TIMEOUT_MS
```

Tokens are redacted from CLI output.
