# KiCadAI

KiCadAI is an early Go client for KiCad's IPC API. The first implementation phase establishes the Go project skeleton, CLI entrypoint, and shared configuration rules that later phases will use for protobuf generation, IPC transport, document discovery, and schematic automation.

## Current Phase

Phase 0 is implemented:

- Go module and package layout.
- CLI entrypoint at `cmd/kicadai`.
- Shared connection configuration package.
- Baseline tests and Make targets.

The client does not connect to KiCad yet. Connectivity starts in later phases after KiCad protobuf bindings and IPC transport are added.

## Requirements

- Go 1.22 or newer.
- KiCad 9.0 or newer will be required for future IPC work.

## Commands

```sh
go test ./...
go run ./cmd/kicadai --help
go run ./cmd/kicadai --json config
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
