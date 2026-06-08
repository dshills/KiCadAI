# KiCadAI

KiCadAI is an early Go client for KiCad's IPC API. The first implementation phase establishes the Go project skeleton, CLI entrypoint, and shared configuration rules that later phases will use for protobuf generation, IPC transport, document discovery, and schematic automation.

## Current Phase

Phase 0 and Phase 1 are implemented:

- Go module and package layout.
- CLI entrypoint at `cmd/kicadai`.
- Shared connection configuration package.
- Baseline tests and Make targets.
- Vendored KiCad API protobuf definitions pinned to an upstream commit.

The client does not connect to KiCad yet. Phase 1 only pins the upstream API definitions; active KiCad connectivity starts after protobuf generation and IPC transport are added in later phases.

## Requirements

- Go 1.22 or newer.
- KiCad 9.0 or newer will be required for future IPC work.

## Commands

```sh
go test ./...
go run ./cmd/kicadai --help
go run ./cmd/kicadai --json config
```

## Vendored KiCad API Protos

KiCad's IPC API protobuf definitions are vendored under `third_party/kicad/api/proto` so Go code generation will be reproducible. The pinned upstream commit is recorded in `third_party/kicad/VERSION`.

To refresh the vendored proto files intentionally:

```sh
make refresh-kicad-proto
```

Set `KICAD_REF=<commit-or-tag>` to refresh from a different KiCad ref.

## KiCad Configuration

Future phases will use these environment variables:

```text
KICAD_API_SOCKET
KICAD_API_TOKEN
KICAD_CLIENT_NAME
KICAD_TIMEOUT_MS
```

Tokens are redacted from CLI output.
