# KiCad Go IPC Implementation Plan

## Overview

This plan implements the initial KiCad Go IPC specification in phases. The sequence is intentionally front-loaded with connectivity, generated API bindings, and test seams before adding schematic mutation. The project should have a working `ping` and `version` command before any schematic automation is attempted.

The first stable outcome is a Go CLI and library that can connect to a running KiCad API server, send protobuf commands, receive protobuf responses, and report open documents. The second outcome is a small schematic automation surface that can draw a simple LED indicator circuit when the connected KiCad version exposes the required schematic write commands.

## Phase 0: Repository Foundation

### Objective

Create a minimal Go project structure that can support generated protobuf code, a reusable client library, CLI commands, and tests.

### Tasks

1. Create `go.mod`.
2. Choose module path.
3. Add standard package layout:

```text
cmd/kicadai/main.go
internal/config
internal/ipc
internal/kiapi
internal/kiapi/gen
internal/schematic
internal/workflows
```

4. Add initial development files:

```text
README.md
Makefile
.gitignore
```

5. Add baseline commands:

```text
go test ./...
kicadai --help
```

### Deliverables

- Go module builds.
- CLI entrypoint exists.
- Empty package tests pass.
- Project README explains the current phase and KiCad dependency.

### Done When

- `go test ./...` succeeds.
- `kicadai --help` prints usable command help.

## Phase 1: KiCad API Source Strategy

### Objective

Make protobuf generation reproducible without relying on ad hoc local paths.

### Tasks

1. Decide whether to vendor KiCad API proto files or fetch them during setup.
2. Prefer vendoring for the initial project because it makes generation deterministic.
3. Add vendored proto source under:

```text
third_party/kicad/api/proto
```

4. Record the KiCad source commit or release tag used.
5. Add a script or make target to refresh the vendored proto files.
6. Include the relevant upstream license and attribution.

### Recommended Initial Choice

Vendor KiCad proto files from a specific KiCad release tag or commit. Start with the newest stable release installed locally, then update intentionally.

### Deliverables

- `third_party/kicad/api/proto` contains KiCad proto definitions.
- `third_party/kicad/VERSION` or equivalent records upstream source.
- Make target exists for proto refresh or documents the manual refresh command.

### Done When

- The repo contains all proto files needed for common, schematic, and board API commands.
- The vendored source version is unambiguous.

## Phase 2: Protobuf Generation

### Objective

Generate Go bindings for KiCad's protobuf API in a repeatable way.

### Tasks

1. Install and document required tools:

```text
protoc
protoc-gen-go
```

2. Inspect KiCad proto files for `go_package` options.
3. If missing, create explicit mapping configuration.
4. Prefer a checked-in generation script or `Makefile` target:

```text
make proto
```

5. Generate Go bindings under:

```text
internal/kiapi/gen
```

6. Add a generated-code freshness check if practical:

```text
make proto-check
```

7. Verify that `ApiRequest`, `ApiResponse`, `Ping`, `GetVersion`, and document commands are available in Go.

### Implementation Notes

- Do not edit vendored KiCad `.proto` files just to add Go package options.
- Use `Mfile.proto=module/path/...` mappings if the upstream protos lack Go package metadata.
- Keep generated packages internal until there is a reason to expose them.

### Deliverables

- Generated Go protobuf files.
- `make proto` regenerates them.
- Build imports generated API packages successfully.

### Done When

- `go test ./...` succeeds after deleting and regenerating generated files.
- A small compile-only test can instantiate `ApiRequest`, `ApiResponse`, and `Ping`.

## Phase 3: Configuration Resolution

### Objective

Centralize connection configuration so CLI, tests, and future AI workflows all use the same rules.

### Tasks

1. Implement `ClientConfig`:

```go
type ClientConfig struct {
    SocketPath string
    Token      string
    ClientName string
    Timeout    time.Duration
}
```

2. Implement config resolution from explicit values and environment variables.
3. Supported environment variables:

```text
KICAD_API_SOCKET
KICAD_API_TOKEN
KICAD_CLIENT_NAME
KICAD_TIMEOUT_MS
```

4. Add default endpoint logic:

```text
macOS/Linux: ipc:///tmp/kicad/api.sock
Windows: defer or return unsupported until named pipe transport is implemented
```

5. Generate default client name:

```text
kicadai-go-<pid>
```

6. Validate timeout parsing.
7. Normalize socket values so both raw paths and `ipc://` URLs can be accepted if practical.

### Deliverables

- `internal/config` package.
- Unit tests for explicit config, environment config, defaults, invalid timeouts, and platform behavior.

### Done When

- Config resolution is deterministic and covered by tests.
- CLI can print effective config with sensitive token redacted.

## Phase 4: Transport Abstraction

### Objective

Create a narrow request/reply transport interface and a real NNG implementation.

### Tasks

1. Define the transport interface:

```go
type Transport interface {
    Dial(endpoint string) error
    Send([]byte) error
    Recv() ([]byte, error)
    Close() error
}
```

2. Implement fake transport for unit tests.
3. Implement NNG-compatible transport using `go.nanomsg.org/mangos/v3` or selected alternative.
4. Add timeout handling.
5. Add endpoint context to connection errors.
6. Add platform guards for unsupported named pipe cases if necessary.

### Transport Selection Gate

Before committing deeply to a library, run a tiny manual probe against KiCad:

```text
connect -> send Ping -> receive response
```

If `mangos` cannot speak to KiCad's endpoint reliably, evaluate another NNG-compatible Go library before continuing.

### Deliverables

- `internal/ipc` package.
- Fake transport tests.
- Real transport implementation.
- Manual probe notes in README or docs.

### Done When

- Fake transport can simulate request/reply.
- Real transport can dial a KiCad socket in a manual test environment.

## Phase 5: Low-Level KiCad Client

### Objective

Implement protobuf envelope handling and a small public client API.

### Tasks

1. Implement `Client`.
2. Implement `NewClient`.
3. Implement `Close`.
4. Implement generic command send:

```go
func (c *Client) Send(ctx context.Context, command proto.Message, response proto.Message) error
```

5. Pack command into `google.protobuf.Any`.
6. Wrap command in `ApiRequest`.
7. Include `client_name`.
8. Include `kicad_token` when known.
9. Serialize request.
10. Send request.
11. Receive response.
12. Parse `ApiResponse`.
13. Check status.
14. Capture response token if initial token was empty.
15. Unpack response payload into expected response type.
16. Return structured errors.

### Error Types

Implement errors that distinguish:

- Dial failure.
- Send failure.
- Receive failure.
- Request marshal failure.
- Response unmarshal failure.
- KiCad API status failure.
- Response `Any` unpack failure.
- Context cancellation or timeout.

### Deliverables

- `internal/kiapi/client.go`.
- Unit tests using fake transport.
- No live KiCad required for unit test coverage.

### Done When

- Tests prove envelope formation.
- Tests prove token update behavior.
- Tests prove non-OK KiCad statuses become API errors.
- Tests prove wrong response type fails cleanly.

## Phase 6: Connectivity Commands

### Objective

Expose connection checks through a CLI that is useful during development.

### Tasks

1. Add CLI command parser.
2. Implement shared flags:

```text
--socket
--token
--client-name
--timeout-ms
--json
```

3. Implement:

```text
kicadai ping
kicadai version
```

4. `ping` sends KiCad `Ping`.
5. `version` sends KiCad `GetVersion`.
6. JSON output should include:

```text
socket_path
client_name
reachable
kicad_version
api_status
error
```

7. Redact token in all logs and output.

### Deliverables

- CLI `ping` command.
- CLI `version` command.
- Manual connection instructions.

### Done When

- `kicadai ping` exits 0 against a reachable KiCad instance.
- `kicadai ping` exits non-zero with a clear error when KiCad is not reachable.
- `kicadai version --json` prints machine-readable version data.

## Phase 7: Document Discovery

### Objective

List open KiCad documents and identify schematic documents.

### Tasks

1. Map KiCad document type protos into Go domain structs.
2. Implement:

```go
func (c *Client) GetOpenDocuments(ctx context.Context, types ...DocumentType) ([]Document, error)
```

3. Implement CLI:

```text
kicadai documents
```

4. Support optional document type filtering.
5. Add JSON output.
6. Handle no open documents clearly.
7. Handle project manager endpoint versus editor endpoint differences.

### Deliverables

- Document domain types.
- Open document client method.
- CLI document listing.

### Done When

- `kicadai documents` lists open project, board, or schematic documents when available.
- No-document and unsupported-endpoint cases produce clear diagnostics.

## Phase 8: Integration Test Harness

### Objective

Add optional real-KiCad tests without making normal CI depend on KiCad.

### Tasks

1. Add integration test build tag:

```text
//go:build integration
```

2. Gate live tests on `KICAD_API_SOCKET`.
3. Add tests for:

```text
Ping
GetVersion
GetOpenDocuments
```

4. Document how to run:

```text
KICAD_API_SOCKET=ipc:///tmp/kicad/api.sock go test -tags=integration ./...
```

5. Capture common failure modes:

- API disabled.
- Wrong socket.
- Multiple KiCad instances.
- No PCB/schematic editor open.
- Token mismatch.

### Deliverables

- Integration test files.
- README section for manual integration testing.

### Done When

- Normal `go test ./...` passes without KiCad.
- Integration tests pass when KiCad is running and configured.

## Phase 9: Schematic API Capability Mapping

### Objective

Determine which KiCad versions expose the schematic write operations required for the LED demo.

### Tasks

1. Inspect generated schematic command packages.
2. Identify commands for:

```text
List/get open schematics
Add symbol
Add wire
Add label
Add power symbol
Update schematic items
Commit or refresh document, if required
```

3. Record command names, request messages, response messages, and required fields.
4. Identify minimum KiCad version for each command.
5. Implement capability model:

```go
type Capability string

const (
    CapabilitySchematicRead  Capability = "schematic.read"
    CapabilitySchematicWrite Capability = "schematic.write"
    CapabilitySymbolPlace    Capability = "schematic.symbol.place"
    CapabilityWirePlace      Capability = "schematic.wire.place"
    CapabilityLabelPlace     Capability = "schematic.label.place"
)
```

6. Implement capability check:

```go
func DetectCapabilities(ctx context.Context, client *Client) (*Capabilities, error)
```

7. Ensure missing schematic write support fails before mutation attempts.

### Deliverables

- Capability matrix document or code comments.
- Capability detection package.
- Tests for capability decisions using mocked version/API data.

### Done When

- The project can state the minimum KiCad version for the LED demo.
- CLI can report whether the connected KiCad instance supports schematic writes.

## Phase 10: Schematic Service Domain Layer

### Objective

Build typed schematic operations that hide protobuf details from workflow and AI layers.

### Tasks

1. Create `internal/schematic`.
2. Define domain request/response structs:

```go
type Point struct {
    X int64
    Y int64
}

type AddSymbolRequest struct {
    Document DocumentRef
    LibraryID string
    Reference string
    Value string
    Position Point
    RotationDegrees float64
}

type AddWireRequest struct {
    Document DocumentRef
    Points []Point
}

type AddLabelRequest struct {
    Document DocumentRef
    Text string
    Position Point
}
```

3. Implement coordinate conventions.
4. Implement request validation.
5. Implement conversion to KiCad protobuf commands.
6. Implement response conversion back to domain refs.
7. Add dry-run planning structs without mutation.

### Coordinate Decision

Use KiCad internal API units directly in the low-level schematic service. Add convenience constructors later for millimeters/inches once verified against actual KiCad schematic coordinate behavior.

### Deliverables

- Schematic domain package.
- Validation tests.
- Proto conversion tests.

### Done When

- Schematic operations can be expressed without importing generated protobuf packages outside the schematic/client internals.
- Invalid requests fail before calling KiCad.

## Phase 11: LED Demo Planning

### Objective

Create a deterministic plan for the first schematic automation workflow before enabling mutation.

### Tasks

1. Define workflow input:

```go
type LEDDemoIntent struct {
    Document DocumentRef
    Origin Point
    Prefix string
}
```

2. Define workflow output:

```go
type AutomationPlan struct {
    Operations []PlannedOperation
}
```

3. Create operation list:

```text
Place VCC power symbol
Place resistor
Place LED
Place GND power symbol
Wire VCC to resistor
Wire resistor to LED
Wire LED to GND
Add labels if supported
```

4. Assign stable coordinates.
5. Assign references and values:

```text
R?: 1k
D?: LED
```

6. Add CLI:

```text
kicadai plan-led-demo --json
```

7. Validate capabilities before returning executable plan.

### Deliverables

- `internal/workflows` LED demo planner.
- Plan-only CLI command.
- Snapshot-style tests for generated plan.

### Done When

- The LED demo plan is deterministic.
- The plan can be printed without connecting to KiCad if a document ref is supplied.
- The plan reports missing capabilities when connected to an insufficient KiCad version.

## Phase 12: LED Demo Execution

### Objective

Execute the LED demo plan against KiCad and return a structured mutation summary.

### Tasks

1. Implement executor that runs planned operations in order.
2. Add rollback strategy decision:

- Initial implementation does not rollback.
- It reports completed operations and failed operation index.
- Future implementation may use KiCad undo groups if exposed by API.

3. Implement:

```go
func CreateSimpleLEDIndicator(ctx context.Context, svc *SchematicService, intent LEDDemoIntent) (*AutomationResult, error)
```

4. Add CLI:

```text
kicadai draw-led-demo
```

5. Require explicit mutation confirmation flag if desired:

```text
--execute
```

6. Return JSON summary:

```text
document
created_symbols
created_wires
created_labels
operations_completed
failed_operation
```

7. Add manual verification checklist.

### Deliverables

- Executable LED demo workflow.
- CLI command.
- Manual test instructions.

### Done When

- Running the command against a supported KiCad instance creates the LED demo schematic elements.
- Failure after partial execution is reported accurately.
- The result summary is machine-readable.

## Phase 13: AI-Ready Operation Boundary

### Objective

Prepare the codebase for future AI-generated design intents without exposing raw API calls to AI orchestration.

### Tasks

1. Define intent structs for future operations.
2. Define validation results:

```go
type ValidationIssue struct {
    Severity string
    Code string
    Message string
}
```

3. Define structured operation results.
4. Add a registry of safe workflow operations:

```text
create_led_indicator
place_decoupling_capacitor
create_connector_block
```

5. Keep only `create_led_indicator` implemented initially.
6. Document that future AI code must call workflow operations, not generated protobuf APIs.

### Deliverables

- Intent/result types.
- Workflow registry.
- Documentation for future AI integration.

### Done When

- There is a clear API boundary for future AI orchestration.
- The LED workflow can be called by name with structured input.

## Phase 14: Documentation and Developer Experience

### Objective

Make the first implementation usable by a developer setting up KiCad and the Go client.

### Tasks

1. Update README with:

- KiCad version requirements.
- How to enable KiCad API.
- How to find socket path.
- How to pass token/socket.
- How to run CLI probes.
- How to run integration tests.
- Known limitations.

2. Add troubleshooting guide:

- `AS_NOT_READY`
- `AS_TOKEN_MISMATCH`
- Cannot dial socket.
- Multiple KiCad instances.
- Wrong endpoint.
- Schematic commands unavailable.

3. Add examples:

```text
kicadai ping
kicadai version --json
kicadai documents
kicadai plan-led-demo --json
kicadai draw-led-demo --execute
```

4. Add contributor notes for regenerating protobuf files.

### Deliverables

- README.
- Troubleshooting section.
- Proto generation docs.

### Done When

- A developer can follow the docs from a clean checkout to a successful `ping`.
- A developer with a supported KiCad version can run the LED demo.

## Phase 15: Future Expansion Plan

### Objective

Record the next logical increments after the initial connection and schematic demo work.

### Candidate Follow-Up Phases

1. Headless KiCad 11 support using `kicad-cli api-server`.
2. Linux Flatpak socket autodetection.
3. Windows named pipe support.
4. Board document discovery.
5. PCB item placement.
6. Netlist transfer from schematic to board.
7. AI-generated schematic intent planning.
8. Design rule and ERC/DRC integration.
9. Component library search and symbol resolution.
10. Undo grouping or transactional mutation support.

### Done When

- Follow-up work is documented but not allowed to expand the scope of the initial implementation.

## Cross-Phase Quality Gates

Each implementation phase should preserve these gates:

- `go test ./...` passes.
- No live KiCad dependency in unit tests.
- Generated protobuf code is reproducible.
- CLI errors are clear and actionable.
- Tokens are not printed in logs.
- Domain/workflow packages do not expose generated protobuf types unless explicitly required.
- Schematic mutations are gated by capability checks.
- Mutation commands return structured summaries.

## Risk Register

### Risk: Go NNG Library Incompatibility

KiCad uses an NNG-style IPC request/reply endpoint. The selected Go transport may not be fully compatible.

Mitigation:

- Validate transport early with a real `Ping`.
- Keep transport behind a narrow interface.
- Be prepared to switch libraries before building higher layers.

### Risk: Missing Go Package Options in KiCad Protos

KiCad's proto files may not be configured for direct Go generation.

Mitigation:

- Use explicit `protoc` mapping options.
- Keep mappings checked in.
- Do not patch vendored upstream proto files unless unavoidable.

### Risk: Schematic Write API Version Gap

KiCad 9 or 10 may not expose enough schematic write operations for the LED demo.

Mitigation:

- Add capability detection before mutation.
- Make the LED demo minimum version explicit.
- Allow connectivity and document discovery to succeed even if schematic writes are unsupported.

### Risk: Wrong KiCad Endpoint

The user may connect to a project manager endpoint or stale socket rather than the schematic editor endpoint.

Mitigation:

- Document endpoint behavior.
- Surface open document types in `documents`.
- Include socket path and client name in diagnostics.

### Risk: Multiple KiCad Instances

Multiple instances can create different socket names and tokens.

Mitigation:

- Prefer explicit `KICAD_API_SOCKET`.
- Preserve and send KiCad token once discovered.
- Report token mismatch clearly.

### Risk: Partial Schematic Mutation

The LED workflow may fail after creating some items.

Mitigation:

- Start with plan/dry-run mode.
- Report completed operation count and failed operation.
- Investigate KiCad undo group support later.

## Suggested Initial Work Order

1. Phase 0: Repository Foundation.
2. Phase 1: KiCad API Source Strategy.
3. Phase 2: Protobuf Generation.
4. Phase 3: Configuration Resolution.
5. Phase 4: Transport Abstraction.
6. Phase 5: Low-Level KiCad Client.
7. Phase 6: Connectivity Commands.
8. Phase 7: Document Discovery.
9. Phase 8: Integration Test Harness.
10. Phase 9: Schematic API Capability Mapping.
11. Phase 10: Schematic Service Domain Layer.
12. Phase 11: LED Demo Planning.
13. Phase 12: LED Demo Execution.

Phases 13 through 15 should remain design and documentation work until the first LED demo has run successfully against a supported KiCad instance.
