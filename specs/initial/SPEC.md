# KiCad Go IPC Connection Specification

## Purpose

Build a Go client that connects to KiCad's IPC API and provides a small, reliable automation layer for schematic drawing. The long-term goal is an AI-assisted system that can create and modify schematics and PCBs through KiCad, but this initial phase focuses on connection, version probing, document discovery, and simple schematic operations.

## Background

KiCad 9 introduced an IPC API that communicates with a running KiCad process over a local IPC endpoint. The API is language-agnostic and uses Protocol Buffers wrapped in request and response envelope messages. Official Python bindings exist, but this project will implement the client in Go.

Relevant KiCad API properties:

- KiCad 9 and 10 require a running GUI process with the API server enabled.
- KiCad 11 adds headless `kicad-cli api-server` support.
- macOS and Linux use Unix domain sockets.
- Windows uses named pipes.
- The default endpoint is usually `api.sock` in KiCad's temp directory.
- KiCad-launched plugins receive `KICAD_API_SOCKET` and `KICAD_API_TOKEN`.
- External tools may need the socket path and token supplied explicitly.
- Requests and responses use protobuf envelopes:
  - `kiapi.common.ApiRequest`
  - `kiapi.common.ApiResponse`
  - command payloads packed into `google.protobuf.Any`

## Goals

1. Connect to a running KiCad IPC API server from Go.
2. Send a `Ping` command and confirm a valid response.
3. Query KiCad version and API compatibility metadata.
4. Discover open KiCad documents, especially schematic documents.
5. Provide a small Go API for schematic automation.
6. Automate one simple schematic drawing workflow.
7. Establish enough structure that future AI planning code can call deterministic Go operations.

## Non-Goals

- Full KiCad API coverage.
- PCB layout generation.
- Autorouting.
- AI prompt orchestration.
- Standalone parsing or editing of KiCad files without KiCad.
- Replacing KiCad's UI.
- Supporting every KiCad version in the first milestone.

## Initial Compatibility Target

The first implementation should target:

- Go 1.22 or newer.
- KiCad 9.0 or newer for GUI IPC.
- KiCad 11 support may be added later for headless mode.
- macOS first, because the current development environment is macOS.
- Linux second.
- Windows named pipe support can be deferred until the IPC transport abstraction is stable.

## Connection Requirements

The Go client must support this configuration:

```text
KICAD_API_SOCKET
KICAD_API_TOKEN
KICAD_CLIENT_NAME
KICAD_TIMEOUT_MS
```

Defaults:

- `KICAD_API_SOCKET`: if unset, use platform defaults.
  - macOS/Linux: `ipc:///tmp/kicad/api.sock`
  - Flatpak Linux can be added later as an alternate default.
- `KICAD_API_TOKEN`: optional. Send it when available.
- `KICAD_CLIENT_NAME`: default to `kicadai-go-<pid>`.
- `KICAD_TIMEOUT_MS`: default to `2000`.

The client must also allow these values to be passed explicitly in Go code so tests and tools are not forced to depend on process environment.

## Transport

Use an NNG-compatible request/reply transport.

Candidate Go libraries:

- `go.nanomsg.org/mangos/v3`
- Another maintained NNG-compatible library if `mangos` cannot connect to KiCad's IPC endpoint correctly.

The transport interface should be narrow:

```go
type Transport interface {
    Dial(endpoint string) error
    Send([]byte) error
    Recv() ([]byte, error)
    Close() error
}
```

This allows tests to use an in-memory fake transport without requiring KiCad.

## Protobuf Generation

The project must generate Go types from KiCad's `.proto` files.

Expected source:

```text
api/proto/common/envelope.proto
api/proto/common/commands/*.proto
api/proto/common/types/*.proto
api/proto/schematic/**/*.proto
api/proto/board/**/*.proto
```

Initial generation can include all KiCad API proto files to avoid repeated generation work as capabilities expand.

Use:

```text
protoc
protoc-gen-go
google.golang.org/protobuf
```

Generated files should live under a stable package path, for example:

```text
internal/kiapi/gen
```

If the upstream KiCad proto files do not include Go package options, the build should provide a checked-in `buf` or `protoc` mapping configuration rather than editing vendored KiCad files.

## Client API

Create a low-level client responsible only for envelope handling.

```go
type ClientConfig struct {
    SocketPath  string
    Token       string
    ClientName  string
    Timeout     time.Duration
}

type Client struct {
    // owns transport, token, client name, timeout
}

func NewClient(config ClientConfig) (*Client, error)
func (c *Client) Close() error
func (c *Client) Ping(ctx context.Context) error
func (c *Client) GetVersion(ctx context.Context) (*KiCadVersion, error)
func (c *Client) Send(ctx context.Context, command proto.Message, response proto.Message) error
```

Behavior:

- `Send` packs the command into `Any`.
- `Send` wraps the command in `ApiRequest`.
- `Send` includes `client_name`.
- `Send` includes `kicad_token` if known.
- `Send` updates the stored token from `ApiResponse.header.kicad_token` when no token was previously configured.
- `Send` checks `ApiResponse.status`.
- `Send` unpacks the response `Any` into the caller-provided response message.
- All transport errors should be returned as connection errors with endpoint context.
- All KiCad API status errors should preserve the raw status code and message.

## Error Model

Define project-specific errors:

```go
type ConnectionError struct {
    Endpoint string
    Cause    error
}

type APIError struct {
    Code    int32
    Message string
}

type TimeoutError struct {
    Operation string
    Timeout   time.Duration
}
```

The error model must distinguish:

- Cannot dial KiCad.
- Request send failed.
- Response receive failed.
- Response protobuf parse failed.
- KiCad returned non-OK API status.
- Response payload could not be unpacked.

## Schematic Automation Scope

After connectivity is proven, add a schematic-focused service on top of the low-level client.

```go
type SchematicService struct {
    client *Client
}

func (s *SchematicService) GetOpenSchematics(ctx context.Context) ([]Document, error)
func (s *SchematicService) GetActiveSchematic(ctx context.Context) (*Document, error)
func (s *SchematicService) AddSymbol(ctx context.Context, req AddSymbolRequest) (*SymbolRef, error)
func (s *SchematicService) AddWire(ctx context.Context, req AddWireRequest) (*WireRef, error)
func (s *SchematicService) AddLabel(ctx context.Context, req AddLabelRequest) (*LabelRef, error)
```

The exact command messages must be mapped from KiCad's schematic proto definitions during implementation. If KiCad 9 or 10 schematic write support is incomplete, the first automation milestone should target the earliest KiCad version that exposes the required schematic commands, likely KiCad 11.

## First Automation Workflow

The first successful schematic automation should create or modify a simple circuit:

```text
VCC -> resistor -> LED -> GND
```

Required operations:

1. Connect to KiCad.
2. Verify API availability with `Ping`.
3. Query KiCad version.
4. Find an open schematic document.
5. Place one resistor symbol.
6. Place one LED symbol.
7. Place power symbols for VCC and GND, if supported by the schematic API.
8. Draw wires between the symbols.
9. Add net labels where supported.
10. Return a structured summary of created objects.

If symbol placement is not available in the connected KiCad version, the workflow should fail with a clear capability error rather than silently doing partial work.

## AI Integration Boundary

The AI layer should not call raw KiCad protobuf commands directly.

Instead, future AI logic should call stable Go operations such as:

```go
CreateSimpleLEDIndicator(ctx, SchematicIntent) (*AutomationResult, error)
PlaceDecouplingCapacitor(ctx, DecouplingIntent) (*AutomationResult, error)
CreateConnectorBlock(ctx, ConnectorIntent) (*AutomationResult, error)
```

The Go layer is responsible for:

- Coordinate normalization.
- KiCad command selection.
- API version checks.
- Validation before mutation.
- Returning structured errors.
- Returning a machine-readable summary of created or modified objects.

This keeps AI-generated plans separate from direct schematic mutation.

## CLI Tool

Provide a small CLI for manual testing:

```text
kicadai ping
kicadai version
kicadai documents
kicadai draw-led-demo
```

Expected behavior:

- `ping`: exits 0 if KiCad is reachable.
- `version`: prints KiCad version and API binding version if available.
- `documents`: prints open document IDs, types, and paths.
- `draw-led-demo`: runs the first automation workflow.

The CLI should support:

```text
--socket
--token
--client-name
--timeout-ms
--json
```

## Test Plan

Unit tests:

- Envelope packing.
- Envelope parsing.
- Token update behavior.
- API status error mapping.
- Response unpacking failure.
- Default config resolution.
- Fake transport send/receive behavior.

Integration tests:

- Optional tests gated by `KICAD_API_SOCKET`.
- `Ping` against a real KiCad instance.
- `GetVersion` against a real KiCad instance.
- Document discovery against a real KiCad instance.

Manual tests:

- Start KiCad.
- Enable API in `Preferences -> Plugins`.
- Open a project and schematic.
- Run `kicadai ping`.
- Run `kicadai version`.
- Run `kicadai documents`.
- Run `kicadai draw-led-demo`.

## Implementation Milestones

### Milestone 1: Go Project Skeleton

- Create `go.mod`.
- Add protobuf and transport dependencies.
- Add package layout.
- Add CLI entrypoint.

### Milestone 2: Protobuf Generation

- Vendor or reference KiCad proto files.
- Add reproducible generation command.
- Generate Go bindings.
- Add a check that generated files are current.

### Milestone 3: Low-Level IPC Client

- Implement config resolution.
- Implement NNG transport.
- Implement request/response envelope handling.
- Implement `Ping`.
- Implement `GetVersion`.
- Add fake transport tests.

### Milestone 4: Document Discovery

- Implement open document listing.
- Identify schematic documents.
- Add CLI output for documents.

### Milestone 5: Schematic Capability Mapping

- Inspect schematic proto commands.
- Determine minimum KiCad version for schematic write operations.
- Implement capability checks.
- Define typed Go requests for symbols, wires, and labels.

### Milestone 6: LED Demo Automation

- Implement simple schematic placement workflow.
- Add dry-run planning output.
- Add mutation execution.
- Return structured result.

## Open Questions

- Which KiCad version should be the first hard target: 9, 10, or 11?
- Does the installed KiCad build expose schematic mutation commands, or only board/project commands?
- Should this project vendor KiCad proto files or fetch them during code generation?
- Should generated Go protobuf files be committed?
- Which Go NNG library works most reliably with KiCad's IPC endpoint on macOS?
- Should the first AI workflow operate only on an already-open schematic, or should it create/open projects too?

## References

- KiCad IPC API developer docs: https://dev-docs.kicad.org/en/apis-and-binding/ipc-api/for-addon-developers/
- KiCad API source tree: https://gitlab.com/kicad/code/kicad/-/tree/master/api
- KiCad API server implementation: https://gitlab.com/kicad/code/kicad/-/blob/master/common/api/api_server.cpp
- KiCad API envelope proto: https://gitlab.com/kicad/code/kicad/-/raw/master/api/proto/common/envelope.proto
- Official Python bindings: https://gitlab.com/kicad/code/kicad-python
