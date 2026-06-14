# CLI Package Split Plan

## Goal

Split the CLI implementation into command parsing, handlers, DTOs, and shared
response/error plumbing while preserving command names, flags, JSON shapes, and
exit codes.

## Current Pain

`cmd/kicadai/main.go` contains flag parsing, command dispatch, JSON DTOs,
project workflows, KiCad IPC calls, library loading, validation output, and
round-trip orchestration. This slows reviews and makes small command additions
riskier than necessary.

## Proposed Layout

- `cmd/kicadai/main.go`: process entry point, app construction, signal context.
- `internal/cli/parser`: command and flag parsing.
- `internal/cli/handlers`: one file per command family.
- `internal/cli/dto`: JSON request/response structs used by CLI boundaries.
- `internal/cli/output`: JSON/text rendering and shared error envelope.
- `internal/cli/runtime`: shared app dependencies, config resolution, and KiCad
  connection setup.

## Phase 1 - Command Inventory

1. Generate a list of current commands, subcommands, flags, JSON outputs, and
   exit code behavior.
2. Add table-driven tests for command dispatch on representative commands.
3. Add golden JSON tests for responses that external agents consume.
4. Add parser error tests for unknown commands, missing args, and invalid flag
   values before moving parser code.

Acceptance:

- No code is moved before command behavior is captured.
- Existing CLI tests pass.
- Confirm DTOs are CLI-only before placing them under `internal/cli/dto`; shared
  in-module DTOs should move to `internal/dto` instead.
- If DTOs need to be imported by Go code outside this module later, write a
  separate public `pkg/cli/dto` proposal before moving them out of `internal`.

## Phase 2 - Extract DTOs

1. Move request/response structs out of `main.go` into `internal/cli/dto`.
2. Keep JSON tags and field names byte-for-byte compatible.
3. Update tests to import DTOs where needed.

Acceptance:

- Golden JSON output remains unchanged.

## Phase 3 - Extract Output And Errors

1. Move `writeJSON`, text summary helpers, and shared error envelopes into
   `internal/cli/output`.
2. Ensure non-JSON output remains stable for human-facing commands.
3. Keep command handlers returning typed results plus issues/errors.
4. Introduce a small command error interface or wrapper that carries
   `ExitCode() int`, so the dispatcher preserves validation/system error exit
   code distinctions.
5. Define a central exit-code registry, starting with success `0`, input error
   `1`, usage/flag error `2`, and runtime/system error `3`, and require
   handlers to use it.

Acceptance:

- Error status codes and JSON error shapes remain unchanged.

## Phase 4 - Extract Runtime And Config

1. Move config resolution, API client connection, and signal-aware runtime setup
   into `internal/cli/runtime`.
2. Keep CLI flags mapped directly to config fields.
3. Avoid global mutable config.

Acceptance:

- IPC probe and KiCad API tests continue to pass.

## Phase 5 - Extract Handlers

1. Move command families one at a time:
   - KiCad API commands;
   - file inspection and validation;
   - project generation;
   - block and composition commands;
   - round-trip commands;
   - library resolver commands.
2. Keep dispatch table behavior unchanged.
3. Run focused tests after each family.

Acceptance:

- `kicadai --help` and command-specific help are unchanged unless explicitly
  updated in a separate UX commit.

## Phase 6 - Parser Cleanup

1. Move flag set construction and command routing into `internal/cli/parser`.
2. Keep `main.go` as orchestration only.
3. Keep the Phase 1 parser error tests passing unchanged.

Acceptance:

- `main.go` contains no command-specific business logic.

## Non-Goals

- No command renames.
- No JSON response redesign.
- No MCP server work.
- No hidden behavior changes to exit codes.
