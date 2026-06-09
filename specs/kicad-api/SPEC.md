# Full KiCad API Implementation Specification

## Purpose

Implement a complete, reliable Go API layer for the KiCad IPC API so KiCadAI can inspect, plan, and eventually mutate schematics and PCB layouts through a running KiCad instance. The implementation must move beyond ad hoc wrappers for `ping`, `version`, and LED-demo planning into a maintained API surface that tracks the pinned KiCad protobuf source.

The long-term product goal is AI-assisted KiCad automation: an AI system should be able to request safe, deterministic operations such as creating schematics, editing symbols and wires, placing PCB footprints, running checks, and reporting results. This specification focuses on the Go API foundation required for that goal.

## Background

KiCad exposes an IPC API using protobuf request and response envelopes over a local IPC endpoint. KiCadAI already has:

- Workspace-local Go client and CLI.
- IPC transport abstraction using `mangos`.
- Generated protobuf bindings under `internal/kiapi/gen`.
- Vendored KiCad API proto files under `third_party/kicad/api/proto`.
- API wrappers for basic connection, version, document discovery, and capability reporting.
- Deterministic workflow planning for a simple LED schematic.

Live probing against KiCad `10.0.3` confirmed:

- The client can connect and `ping` a running KiCad process.
- The current generated API surface reports `schematic.read`.
- The current generated bindings do not expose schematic mutation commands for placing symbols, wires, or labels.
- `GetOpenDocuments` may return `AS_UNHANDLED` against the current live server.

The full API implementation must therefore include both a wrapper layer and an explicit inventory/compatibility process. If a command is not exposed by the pinned KiCad proto source or is unhandled by the connected KiCad instance, the Go API must report that precisely instead of pretending support exists.

## Definition of Full API

For this project, "full API" means:

1. Every command message exposed by the pinned KiCad protobuf source can be sent through a typed Go wrapper or a documented generic command path.
2. Every response type exposed by the pinned KiCad protobuf source can be unpacked, validated, and returned in a typed Go result.
3. Unsupported, unimplemented, unhandled, or version-gated commands return structured errors.
4. The API surface is organized by KiCad domain:
   - Common API.
   - Project API.
   - Editor/document API.
   - Schematic API.
   - Board/PCB API.
   - Jobs and long-running operations.
5. Generated protobuf code remains mechanically generated and is not tested directly except through hand-written wrappers.
6. The implementation includes source inventory tooling so future KiCad releases can be adopted intentionally.

Full API does not mean every command will work against every KiCad version. The client must distinguish:

- `generated`: command exists in local generated bindings.
- `advertised`: command appears supported by local capability rules.
- `handled`: connected KiCad instance accepts the command.
- `safe`: command is allowed by KiCadAI's higher-level automation policy.

## Goals

1. Build a typed Go API wrapper for all pinned KiCad command messages.
2. Keep a generic escape hatch for commands not yet wrapped.
3. Add a command inventory that maps proto messages to wrapper functions, request payload types, response payload types, and capability requirements.
4. Support common, project, document/editor, schematic, board, and job workflows.
5. Add capability detection that is based on both generated bindings and live probing where possible.
6. Add safe mutation support when KiCad exposes write commands.
7. Preserve deterministic tests that do not require live KiCad.
8. Keep live integration tests opt-in behind `-tags=integration`.
9. Provide CLI commands that exercise the API surface for manual validation.
10. Make API changes auditable when KiCad proto files are refreshed.

## Non-Goals

- Reimplementing KiCad file parsing as a replacement for the IPC API.
- Autorouting or AI decision-making in the API layer.
- Bypassing KiCad capability or token checks.
- Pretending unsupported mutation commands exist.
- Directly editing generated `.pb.go` files by hand.
- Making normal tests depend on a running KiCad GUI.

## Compatibility Targets

### Required

- Go version pinned by `go.mod`.
- macOS and Linux IPC socket support.
- KiCad 9 or newer GUI IPC mode where commands are exposed.
- KiCad 10 compatibility, based on the current local live test target.

### Planned

- Windows named pipe support through the existing transport abstraction.
- KiCad 11 headless API server support when available.
- Multiple pinned KiCad API source versions if compatibility requires it.

## Source and Generation Requirements

The implementation must treat KiCad proto files as the source of truth.

Required source files:

```text
third_party/kicad/api/proto/**/*.proto
third_party/kicad/VERSION
third_party/kicad/README.md
third_party/kicad/LICENSE*
```

Generation requirements:

- `make proto` regenerates all protobuf bindings.
- `make proto-check` verifies generated files are current.
- `scripts/generate-proto.sh` maps every vendored proto file to a stable Go package.
- Generated files remain under `internal/kiapi/gen`.
- Generation output includes a generated README.
- A refresh command records the upstream KiCad version or commit.

Inventory requirements:

- Add a generated or maintained command inventory file under `internal/kiapi`.
- The inventory must include:
  - Domain.
  - Command name.
  - Request protobuf type.
  - Response protobuf type, if known.
  - Wrapper function name.
  - Capability name.
  - Minimum KiCad version, if known.
  - Live probe status, if probeable.

## API Architecture

### Package Layout

The full API should use this package structure:

```text
internal/kiapi
internal/kiapi/common
internal/kiapi/project
internal/kiapi/editor
internal/kiapi/schematic
internal/kiapi/board
internal/kiapi/jobs
internal/kiapi/gen
```

If public API exposure becomes necessary later, the hand-written packages can move to `pkg/kiapi` after the internal API stabilizes.

### Client Responsibilities

The core client must:

- Resolve connection configuration.
- Dial IPC transport.
- Pack typed command payloads into `google.protobuf.Any`.
- Attach request metadata and token when configured.
- Send requests serially through the request/reply transport.
- Unpack typed responses.
- Capture returned tokens in memory when KiCad provides them.
- Convert KiCad API statuses into structured Go errors.
- Close cleanly.

### Generic Command Path

The API must include a generic command path:

```go
func (c *Client) Send(ctx context.Context, payload proto.Message, response proto.Message) error
```

Generic behavior:

- Pack `payload` into an API request envelope.
- Send through transport.
- Validate response status.
- If `response` is non-nil, unpack response payload into it.
- Return `APIError`, `ClientError`, or `ConnectionError` with preserved causes.

Typed wrappers must use this generic path internally.

### Typed Wrapper Pattern

Each wrapper must:

- Accept `context.Context`.
- Accept a typed request or domain-specific input struct.
- Return a typed response or domain DTO.
- Avoid exposing generated protobuf internals when a cleaner domain type is practical.
- Preserve generated protobuf access for advanced callers when the wrapper is thin.

Example shape:

```go
func (c *Client) GetVersion(ctx context.Context) (Version, error)
func (c *Client) GetOpenDocuments(ctx context.Context, filter DocumentFilter) ([]Document, error)
func (c *Client) GetBoard(ctx context.Context, document DocumentRef) (board.Document, error)
```

### Error Model

The API must expose structured errors:

```go
APIError
ClientError
CapabilityError
UnsupportedCommandError
ConnectionError
```

Errors must include:

- Operation or command name.
- KiCad API status.
- Server message when available.
- Endpoint when connection-related.
- Wrapped cause for `errors.Is` and `errors.As`.

Known statuses such as `AS_UNHANDLED`, `AS_UNIMPLEMENTED`, `AS_TOKEN_MISMATCH`, `AS_BUSY`, and `AS_NOT_READY` must be represented clearly.

## Capability Model

Capabilities must be data-driven rather than hard-coded only in prose.

Required capability classes:

```text
common.ping
common.version
documents.read
project.read
project.write
schematic.read
schematic.write
schematic.symbol.place
schematic.symbol.update
schematic.symbol.delete
schematic.wire.place
schematic.label.place
board.read
board.write
board.footprint.place
board.track.place
board.zone.place
jobs.run
```

Capability detection must combine:

1. Generated binding inventory.
2. Static KiCad version rules where known.
3. Optional live probes for commands that can be tested safely.
4. Server response status from actual command attempts.

The API must never claim mutation support unless the command exists and is considered safe to call.

## Domain Requirements

### Common API

Required wrappers:

- Ping.
- Version.
- Token capture and redaction.
- Basic status/error translation.

### Document and Editor API

Required wrappers:

- List open documents when KiCad handles the command.
- Parse document types.
- Normalize project, board, and schematic references.
- Report `AS_UNHANDLED` as unsupported for versions or contexts that do not expose document discovery.

### Project API

Required wrappers:

- Project settings read commands exposed by the proto source.
- Project metadata and path handling where available.
- Safe write commands only when exposed and understood.

### Schematic API

Required wrappers:

- Schematic hierarchy and netlist read commands exposed by the proto source.
- Symbol, wire, label, sheet, and junction mutation wrappers when exposed by the proto source.
- Domain request validation before mutation.
- Stable coordinate units and conversion helpers.
- Safe operation planning that can be reviewed before execution.

If schematic mutation commands are absent from the pinned proto source, wrappers must remain planned but disabled with explicit unsupported errors.

### Board API

Required wrappers:

- Board document read commands exposed by the proto source.
- Footprint, track, via, zone, text, and drawing item mutation wrappers when exposed.
- Coordinate/unit helpers shared with schematic where practical.
- Preflight checks for board write capability.

### Jobs API

Required wrappers:

- Start jobs exposed by the proto source.
- Track job status if the API exposes job IDs or progress.
- Convert long-running operation responses into deterministic Go states.

## Mutation Safety Requirements

All write operations must pass through a safety layer before execution.

Safety checks:

- Capability exists.
- Connected KiCad version is compatible.
- Target document reference is explicit.
- Request validates locally.
- Operation is allowed by the workflow policy.
- Dry-run planning is available.

Write APIs must support:

- Plan-only mode.
- Execute mode.
- Structured result with completed operations and failed operation.
- Idempotency guidance where practical.
- Clear rollback limitations. The API should not claim rollback unless KiCad exposes a real transaction/undo mechanism that the wrapper uses.

## CLI Requirements

The CLI should grow alongside the API and provide manual validation commands:

```text
kicadai api inventory
kicadai api capabilities
kicadai api probe
kicadai api call <domain.command>
kicadai documents
kicadai project get
kicadai schematic read
kicadai schematic plan-led-demo
kicadai schematic draw-led-demo --execute
kicadai board read
```

CLI commands must:

- Support `--json`.
- Redact tokens.
- Return non-zero on failed live calls.
- Preserve structured error output.
- Avoid requiring live KiCad for plan-only commands.

## Testing Requirements

Normal tests must not require KiCad.

Required deterministic tests:

- Command inventory completeness.
- Generic request/response packing.
- Typed wrapper request construction.
- Typed response unpacking.
- API status to error translation.
- Capability mapping.
- Unsupported command handling.
- Mutation validation and preflight.
- CLI JSON output for command errors.

Integration tests:

- Must use `//go:build integration`.
- Must skip unless `KICAD_API_SOCKET` is set.
- May use `KICAD_API_TOKEN` and `KICAD_TIMEOUT_MS`.
- Must not be included in `make coverage-check`.
- Should cover ping, version, documents when supported, and safe read commands.
- Mutation integration tests must be opt-in and should require an explicit environment variable such as `KICAD_ALLOW_MUTATION_TESTS=1`.

Coverage:

- Generated code under `internal/kiapi/gen/**` is excluded from actionable thresholds.
- Hand-written wrappers and inventory must meet project coverage thresholds.
- Every bug found by live testing should get a deterministic regression test where possible.

## Documentation Requirements

Documentation must include:

- KiCad API source version.
- How to refresh proto files.
- How to regenerate Go bindings.
- Command inventory report.
- Supported capabilities by KiCad version.
- Live integration test instructions.
- Mutation safety model.
- Known unsupported commands.

## Completion Criteria

The full API foundation is complete when:

- Every generated command has either a typed wrapper or an explicit inventory entry explaining why it is generic-only or unsupported.
- `make proto-check` passes.
- `make test` passes without KiCad.
- `make coverage-check` passes.
- Live `ping` and `version` pass against a running KiCad instance.
- Capability reporting distinguishes generated, supported, handled, and safe commands.
- Schematic/board write paths execute only when KiCad exposes the required commands.
- CLI can print a complete API inventory in JSON.

## Risks

### Risk: KiCad Proto Source Does Not Expose Desired Mutation Commands

Mitigation: Inventory the exact proto source, document missing commands, and refresh against newer KiCad commits or releases before building wrappers that assume write support.

### Risk: Live Server Handles Fewer Commands Than Generated Bindings

Mitigation: Treat `AS_UNHANDLED` and `AS_UNIMPLEMENTED` as expected compatibility outcomes and feed them into live capability reporting.

### Risk: Typed Wrappers Become Boilerplate Heavy

Mitigation: Use a generic command path, generate wrapper scaffolds where practical, and keep domain DTOs only where they simplify caller behavior.

### Risk: Mutation Tests Damage User Projects

Mitigation: Keep mutation tests opt-in, require explicit mutation consent, and document use of throwaway projects.

### Risk: Version-Specific Behavior Becomes Confusing

Mitigation: Record KiCad version in every live probe result and keep capability rules data-driven.
