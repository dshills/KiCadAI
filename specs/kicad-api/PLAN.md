# Full KiCad API Implementation Plan

## Overview

This plan implements the full KiCad API specification in phases. The work starts with source inventory and command mapping because the current live test showed that KiCadAI can connect to KiCad `10.0.3`, but the pinned generated API surface does not expose schematic mutation commands. The plan therefore avoids building speculative write wrappers until the command inventory proves those commands exist.

Each phase should be reviewed independently, tested, and committed before moving to the next phase.

## Phase 1: API Source Inventory

### Objective

Create a reliable inventory of every protobuf file, command message, response message, and generated Go package from the pinned KiCad API source.

### Tasks

1. Add an inventory generator or script that scans:

```text
third_party/kicad/api/proto/**/*.proto
internal/kiapi/gen/**/*.pb.go
```

2. Record for each command:
   - Domain.
   - Proto file.
   - Proto message name.
   - Generated Go package.
   - Generated Go type.
   - Expected response type when discoverable.
   - Whether a hand-written wrapper exists.
3. Generate a machine-readable inventory:

```text
internal/kiapi/api_inventory.json
```

4. Add a readable inventory report command:

```text
kicadai --json api-inventory
```

5. Add tests that fail if command messages are added without inventory coverage.

### Deliverables

- Inventory generator or maintained inventory file.
- CLI inventory output.
- Tests for inventory completeness.

### Done When

- `make test` passes.
- Inventory includes common, project, schematic, board, and jobs domains.
- The inventory explicitly shows whether schematic mutation commands exist in the pinned proto source.

## Phase 2: Generic Command Engine Hardening

### Objective

Make the generic command path strong enough to support every wrapper.

### Tasks

1. Audit `Client.Send` for:
   - Request packing.
   - Response unpacking.
   - Token propagation.
   - API status conversion.
   - Context cancellation.
   - Transport close behavior.
2. Add a public internal helper for typed commands:

```go
func SendCommand[T proto.Message](ctx context.Context, c *Client, request proto.Message, response T) (T, error)
```

or use an equivalent non-generic helper if simpler.

3. Ensure errors include:
   - Command type.
   - KiCad status.
   - Server message.
   - Wrapped cause.
4. Add deterministic fake transport tests for:
   - Successful response unpacking.
   - Empty response.
   - Wrong response type.
   - API error response.
   - Unhandled command response.
   - Token capture.

### Deliverables

- Hardened generic send path.
- Expanded client tests.
- Clear command error types.

### Done When

- All existing wrappers use the hardened path.
- `make test` and `make coverage-check` pass.

## Phase 3: Capability Model v2

### Objective

Replace narrow static capability reporting with data-driven capability state.

### Tasks

1. Define capability state:

```text
generated
wrapped
version_supported
live_handled
safe
```

2. Add capability records for each inventory command.
3. Map records to high-level capabilities:
   - `schematic.read`
   - `schematic.write`
   - `board.read`
   - `board.write`
   - `jobs.run`
4. Add live probe support for safe read-only commands.
5. Keep mutation probes disabled unless explicitly allowed.
6. Update CLI:

```text
kicadai capabilities
kicadai api probe
```

7. Preserve current behavior that write capabilities are missing when commands are absent.

### Deliverables

- Data-driven capability package.
- JSON capability output with generated/wrapped/handled/safe fields.
- Live probe command.

### Done When

- Capability output explains why schematic drawing is blocked on the current KiCad/proto combination.
- Unit tests cover generated-only, wrapped, unhandled, and safe states.

## Phase 4: Common and Editor API Wrappers

### Objective

Complete typed wrappers for common, editor, and document-level commands exposed by the pinned API.

### Tasks

1. Wrap common commands:
   - Ping.
   - Version.
   - Any other common command in inventory.
2. Wrap editor/document commands:
   - Open documents when handled.
   - Document type conversion.
   - Active editor or document commands where exposed.
3. Normalize DTOs for:
   - Version.
   - Document references.
   - Document type.
   - Command status.
4. Add graceful unsupported handling for commands that KiCad returns as `AS_UNHANDLED`.
5. Add CLI coverage for wrappers.

### Deliverables

- Complete common/editor wrapper package.
- CLI smoke commands.
- Fake transport tests.

### Done When

- Inventory marks common/editor command wrappers as complete or explicitly generic-only.
- Live `ping` and `version` still pass.

## Phase 5: Project API Wrappers

### Objective

Implement typed wrappers for project-level API commands exposed by the pinned source.

### Tasks

1. Inventory all project commands and types.
2. Add project wrapper package:

```text
internal/kiapi/project
```

3. Add DTOs for project settings and paths where practical.
4. Implement read wrappers first.
5. Implement write wrappers only when the proto source exposes them and safety requirements are understood.
6. Add CLI commands:

```text
kicadai project get
kicadai project settings
```

### Deliverables

- Project API wrappers.
- Tests for request/response conversion.
- CLI commands for safe project reads.

### Done When

- All project commands in the inventory are wrapped, generic-only, or explicitly unsupported.
- Normal tests do not require a live project.

## Phase 6: Schematic Read API Wrappers

### Objective

Complete read-only schematic wrappers before adding mutation.

### Tasks

1. Inventory schematic read commands.
2. Add schematic API package:

```text
internal/kiapi/schematic
```

3. Wrap exposed read commands for:
   - Hierarchy.
   - Netlist.
   - Schematic metadata.
   - Symbol/wire/label read models if exposed.
4. Add conversions to domain-level DTOs.
5. Update workflow planning to optionally use read data when available.
6. Add CLI commands:

```text
kicadai schematic read
kicadai schematic netlist
```

### Deliverables

- Schematic read wrappers.
- DTO conversion tests.
- Optional live integration tests for read commands.

### Done When

- `schematic.read` is backed by concrete wrappers, not only version rules.
- Live read commands skip or report unsupported cleanly when KiCad returns `AS_UNHANDLED`.

## Phase 7: Schematic Mutation API Wrappers

### Objective

Implement schematic write commands only if the pinned or refreshed KiCad API source exposes them.

### Tasks

1. Confirm inventory entries for mutation commands:
   - Place symbol.
   - Update symbol.
   - Delete symbol.
   - Place wire.
   - Place label.
   - Add junction.
   - Sheet operations.
2. If commands are absent:
   - Document absence in inventory and capability output.
   - Refresh KiCad proto source from a newer release or commit.
   - Regenerate bindings.
   - Re-run inventory.
3. If commands are present:
   - Add typed wrappers.
   - Add domain validation.
   - Add dry-run planning.
   - Add execution result tracking.
4. Wire `draw-led-demo --execute` to real mutation wrappers.
5. Add mutation integration tests behind:

```text
-tags=integration
KICAD_ALLOW_MUTATION_TESTS=1
```

### Deliverables

- Schematic write wrappers or explicit unsupported report.
- Executable LED demo when supported.
- Safety-gated mutation tests.

### Done When

- The CLI either draws the LED demo through real API commands or reports the exact missing command/capability.
- No mutation can run accidentally in normal tests.

## Phase 8: Board Read API Wrappers

### Objective

Implement board/PCB read wrappers exposed by the pinned source.

### Tasks

1. Inventory board read commands.
2. Add board API package:

```text
internal/kiapi/board
```

3. Wrap read commands for board metadata, footprints, tracks, zones, nets, and design settings where exposed.
4. Add coordinate and unit conversion helpers.
5. Add CLI commands:

```text
kicadai board read
kicadai board nets
```

### Deliverables

- Board read wrappers.
- Board DTO tests.
- Optional live read tests.

### Done When

- `board.read` capability is backed by concrete inventory and wrappers where exposed.

## Phase 9: Board Mutation API Wrappers

### Objective

Implement PCB write operations when KiCad exposes them through the pinned source.

### Tasks

1. Confirm inventory entries for:
   - Place footprint.
   - Move footprint.
   - Place track.
   - Place via.
   - Place zone.
   - Add board text/drawings.
2. Add typed wrappers and validation.
3. Add dry-run planning for PCB operations.
4. Add safety preflight for target board document.
5. Keep integration mutation tests opt-in.

### Deliverables

- Board write wrappers or explicit unsupported report.
- Board mutation planning DTOs.
- Safety-gated integration tests.

### Done When

- `board.write` reports accurate support.
- Supported operations can be executed against a throwaway board only when explicitly enabled.

## Phase 10: Jobs and Long-Running Operations

### Objective

Support KiCad job commands exposed by the API.

### Tasks

1. Inventory jobs commands and response types.
2. Add jobs package:

```text
internal/kiapi/jobs
```

3. Implement job start/status wrappers.
4. Normalize job result states.
5. Add timeout and polling controls.
6. Add CLI commands:

```text
kicadai jobs run
kicadai jobs status
```

### Deliverables

- Jobs wrappers.
- Polling tests with fake transport.
- Optional live tests for safe read-only jobs.

### Done When

- Jobs commands are wrapped, generic-only, or explicitly unsupported in inventory.

## Phase 11: CLI API Explorer

### Objective

Expose enough CLI tooling to manually test and debug the full API without writing one-off Go programs.

### Tasks

1. Add:

```text
kicadai api inventory
kicadai api capabilities
kicadai api probe
kicadai api call
```

2. Support JSON input for `api call` where safe.
3. Redact tokens in all output.
4. Print KiCad version and endpoint in live probe reports.
5. Add examples to README.

### Deliverables

- API explorer CLI.
- JSON schemas or documented payload examples.
- CLI tests.

### Done When

- A developer can inspect which commands are generated, wrapped, handled live, and safe from the CLI.

## Phase 12: Documentation and Compatibility Matrix

### Objective

Document the implemented API surface and known KiCad compatibility behavior.

### Tasks

1. Add generated or maintained API inventory docs:

```text
docs/kicad-api-inventory.md
```

2. Add compatibility matrix by KiCad version.
3. Document unsupported commands and why they are unsupported.
4. Document mutation safety requirements.
5. Document live test setup.
6. Document proto refresh workflow.

### Deliverables

- API inventory documentation.
- Compatibility matrix.
- Mutation safety documentation.

### Done When

- README links to API inventory and live-test docs.
- KiCad `10.0.3` behavior observed in local testing is documented.

## Phase 13: Coverage and Regression Hardening

### Objective

Ensure the expanded API remains testable and regression-resistant.

### Tasks

1. Add package-level coverage checks for new hand-written API packages if useful.
2. Add regression tests for every live API issue discovered.
3. Keep generated code excluded from the actionable threshold.
4. Add fixture-based protobuf response tests for representative commands.
5. Ensure `make coverage-check` remains stable without KiCad.

### Deliverables

- Expanded tests.
- Updated coverage docs.
- Regression fixtures.

### Done When

- `make test` passes.
- `make coverage-check` passes.
- Prism review has no material findings.

## Phase 14: Release Readiness

### Objective

Prepare the full API layer for use by AI workflow code.

### Tasks

1. Stabilize internal API names.
2. Decide whether any wrappers should move from `internal` to `pkg`.
3. Add examples for Go callers.
4. Add end-to-end dry-run workflow tests.
5. Add live smoke-test checklist.
6. Tag a known-good KiCad API source version.

### Deliverables

- Go examples.
- Live smoke-test checklist.
- Stable API package decision.

### Done When

- AI workflow code can call typed wrappers or safe operations without depending on generated protobuf details.
- The project can explain exactly what KiCad automation is supported by the pinned API source and connected KiCad version.

## Quality Gates for Every Phase

Each phase must satisfy:

- `gofmt` for Go changes.
- `make test`.
- `make coverage-check` when tests or hand-written code change.
- `make proto-check` when proto generation changes.
- Prism review of staged changes.
- No normal test requires live KiCad.
- No mutation integration test runs without explicit opt-in.
- No token values appear in logs, snapshots, or test output.

## Suggested Commit Sequence

1. Add API inventory tooling.
2. Harden generic command engine.
3. Add capability model v2.
4. Complete common/editor wrappers.
5. Add project wrappers.
6. Add schematic read wrappers.
7. Add schematic mutation support or explicit missing-command report.
8. Add board read wrappers.
9. Add board mutation support or explicit missing-command report.
10. Add jobs wrappers.
11. Add CLI API explorer.
12. Add compatibility docs.
13. Harden coverage/regression tests.
14. Prepare release/API examples.

## Initial Open Questions

1. Which KiCad upstream source should be the next pinned proto refresh target: installed `10.0.3`, latest stable release, or current upstream master?
2. Are schematic mutation commands available in a newer KiCad API source but absent from the current vendored proto files?
3. Should the command inventory be generated during `make proto` or maintained as a checked-in derived artifact with a separate `make api-inventory` target?
4. Should the first public caller-facing package remain `internal` until mutation is proven, or move read-only wrappers to `pkg` earlier?
5. What level of AI operation policy belongs in the API layer versus a higher workflow/orchestration layer?

## Risk Management

### Missing Mutation Commands

Do not implement fake mutation wrappers. Inventory first, refresh proto source if needed, and report missing commands as unsupported.

### Broad Wrapper Boilerplate

Use a generic command engine and consider generating wrapper scaffolds from inventory metadata.

### Live API Drift

Record live probe results with KiCad version, endpoint, and command status. Treat drift as compatibility data, not as a generic failure.

### Project Damage From Mutation Tests

Require explicit environment consent and document throwaway-project usage for all mutation integration tests.
