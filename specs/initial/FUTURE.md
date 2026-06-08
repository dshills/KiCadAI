# Phase 15 Roadmap: Future Expansion Plan

This document records follow-up work after the initial KiCad connection and schematic planning implementation. These items are intentionally outside the current implementation scope.

## Scope Boundary

The current implementation stops at:

- KiCad IPC connection, token handling, and connection probes.
- Version, document, and capability discovery.
- Deterministic schematic operation planning for a simple LED indicator.
- A guarded execution boundary that refuses mutation when schematic write support is unavailable.
- An AI-safe workflow registry for named operations.

Future work must preserve the workflow boundary: AI-driven code should create structured workflow requests and must not call generated protobuf clients or raw transport APIs directly.

## Candidate Follow-Up Items

1. Headless KiCad API server support.
   - Detect and connect to future `kicad-cli api-server` style workflows when available.
   - Keep GUI KiCad and headless KiCad configuration paths separate.

2. Linux Flatpak socket autodetection.
   - Detect Flatpak-specific runtime socket locations.
   - Prefer explicit `KICAD_API_SOCKET` when multiple candidates exist.

3. Windows named pipe support.
   - Add a transport implementation for Windows IPC once the KiCad endpoint format is confirmed.
   - Extend config defaults and integration tests without changing Unix behavior.

4. Board document discovery.
   - Expand document filtering and capability reporting around PCB editor documents.
   - Keep board APIs behind domain types rather than exposing generated protobuf types.

5. PCB item placement.
   - Add domain requests for footprints, tracks, vias, zones, and graphics.
   - Require capability checks before any board mutation.

6. Netlist transfer from schematic to board.
   - Define a workflow that translates validated schematic plans into PCB connectivity intent.
   - Keep synchronization explicit and auditable.

7. AI-generated schematic intent planning.
   - Add natural-language-to-workflow adapters that produce `workflows.OperationRequest` values.
   - Validate every generated intent before planning or execution.

8. ERC/DRC integration.
   - Add commands to run and parse electrical and design-rule checks when KiCad exposes suitable API calls.
   - Return machine-readable issue summaries for AI feedback loops.

9. Component library search and symbol resolution.
   - Add symbol and footprint lookup workflows.
   - Resolve ambiguous library IDs before generating schematic plans.

10. Undo grouping or transactional mutation support.
    - Group related schematic or board mutations when KiCad exposes undo/transaction APIs.
    - Report partial progress and failed operation indices for non-transactional execution.

## Quality Gates For Future Work

- `make test` passes without requiring live KiCad.
- Live KiCad behavior stays behind optional integration tests.
- Generated protobuf output remains reproducible.
- Tokens are never printed in logs or command output.
- Mutation commands require explicit `--execute` style confirmation.
- Capability checks gate every mutation path.
- Workflow/domain packages remain the AI-facing boundary.
