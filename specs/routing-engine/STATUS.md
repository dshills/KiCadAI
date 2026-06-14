# Routing Engine Status

Date: 2026-06-14

## Implemented

- Deterministic routing request/result model in `internal/routing`.
- Single-layer Manhattan grid routing.
- Two-layer routing with vias.
- Keepout, pad, board-edge, and existing-copper occupancy.
- Deterministic net and endpoint-pair planning.
- Route simplification into KiCad-style segments.
- Via generation and validation.
- Whole-request routing with routed, partial, and blocked statuses.
- In-process routed connectivity and clearance validation.
- Route operation emission for transaction-style consumers.
- Placement-to-routing adapter in `internal/routingadapters`.
- Design API integration via `designapi.Builder.RouteBoard`.
- CLI entry point: `kicadai --json --request <file> route request`.
- KiCad ERC/DRC feedback mapping into routing issues.
- AI-facing repair diagnostics with categories and suggested actions.
- Golden routed examples for straight, detour, and endpoint-via routes.
- Stress tests for deterministic repeated routing and search-limit failure.
- README routing documentation and a runnable JSON request fixture.

## Verified

- `go test ./...`
- `go run ./cmd/kicadai --json --request ./examples/routing/simple_request.json --mode single_layer --grid 1 --trace-width 0.1 --clearance 0.2 route request`
- Prism staged reviews were run for each implementation phase. Actionable high
  and medium findings were addressed unless they required a deliberate API
  change outside the phase scope.

## Current Limits

- The router targets small deterministic boards, not dense production
  autorouting.
- Routing is orthogonal and grid-based; there is no diagonal, curved,
  differential-pair, length-tuned, or impedance-aware routing yet.
- Placement quality strongly affects route success.
- Routing and placement currently use different JSON names for board-edge
  clearance: `rules.edge_clearance_mm` for routing and
  `rules.board_edge_clearance_mm` for placement.
- The route CLI does not yet expose an edge-clearance override.
- Placement pad summaries do not yet preserve complete footprint pad shape and
  through-hole metadata.
- KiCad DRC feedback is wired through the checks package, but stable real-CLI
  DRC fixtures remain environment-dependent.

## Suggested Next Work

- Add `--edge-clearance` to `route request`.
- Decide whether routing should accept `board_edge_clearance_mm` as an alias or
  replace `edge_clearance_mm`.
- Expand footprint-library pad geometry hydration before placement-to-routing
  conversion.
- Add routed PCB writer examples that open in KiCad and optionally run local
  DRC when `kicad-cli` is available.
- Add AI repair loop commands that consume `RepairDiagnostic` output and propose
  placement/routing changes.
