# Entry Anchor Board Binding Implementation Plan

Date: 2026-06-24

## Objective

Bind block entry anchors to real PCB endpoints during board-level composition so
generated protection and power-path blocks are electrically meaningful at their
external interfaces, not merely parseable.

## Phase 1: Binding Data Model

Add the core data structures for physical endpoints, anchor bindings, binding
issues, and binding summaries.

Tasks:

- Add `PhysicalEndpoint`, `AnchorBinding`, `AnchorBindingIssue`, and
  `AnchorBindingSummary` models in the workflow or board-composition package
  that currently owns generated PCB evidence.
- Define endpoint kinds, binding statuses, route statuses, issue categories, and
  binding policy constants.
- Add JSON tags that match the existing `design create` evidence style.
- Add constructors or small helpers for common issue creation.
- Add unit tests for status aggregation, issue severity counting, and stable
  JSON field names.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Add entry anchor binding model`

## Phase 2: Endpoint Discovery

Discover candidate physical endpoints from generated PCB state.

Tasks:

- Collect footprint-pad endpoints from placed or hydrated footprint pad
  summaries.
- Preserve ref, pad, net, roles, point, source, and confidence where known.
- Add a deterministic endpoint ID format backed by a stable hash of the
  canonical tuple `(kind, ref, pad)`, with a short readable prefix such as
  `footprint_pad:J1:A6:<hash>` for diagnostics. Coordinates, nets, and source
  evidence are endpoint attributes and must not change the endpoint identity.
- Define the hash input string exactly as
  `kind=<kind>\nref=<ref>\npad=<pad>\n`, with raw field values and no coordinate
  or net fields.
- Build a simple spatial index for discovered endpoints before proximity
  matching. A grid index is sufficient initially; the API should allow an R-tree
  or similar index later if large-board fixtures show the need.
- Keep the spatial index behind a small interface so endpoint discovery and
  binding resolution do not depend on the concrete grid implementation.
- Add support stubs for `board_edge_point` and `imported_mechanical_point`
  without selecting them by default.
- Add tests that discover endpoints from a connector breakout and protection
  block fixture.
- Add tests for missing pad geometry and missing net metadata.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Discover physical endpoints for anchor binding`

## Phase 3: Anchor Collection And Binding Resolution

Collect anchors from block PCB realization evidence and resolve each required or
optional anchor to a physical endpoint.

Tasks:

- Extract entry anchors from block PCB fragments with block instance ID, anchor
  ID, port, net, coordinate, and policy.
- Resolve candidates using explicit connection intent first, then net, role, and
  deterministic proximity.
- Reject hard net mismatches.
- Mark ties as `ambiguous` rather than choosing arbitrarily.
- Emit `missing_endpoint`, `ambiguous_endpoint`, `net_mismatch`, and
  `role_mismatch` issues.
- Add tests for successful ESD signal binding, reverse-polarity power binding,
  missing endpoint, ambiguous endpoint, and net mismatch.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Resolve entry anchors to physical endpoints`

## Phase 4: Board-Level Route Evidence

Connect resolved physical endpoints to anchor coordinates using route operations
or route requests.

Tasks:

- For bound anchors with known endpoint and anchor points, create board-level
  route evidence between endpoint and anchor.
- Use existing net-class, route-width, and block route-width policies where
  available.
- Keep board-level endpoint-to-anchor routes separate from block-local
  anchor-to-component routes.
- Mark bindings as `routed`, `route_requested`, `not_routable`, or `skipped`.
- Emit `route_missing` or `missing_endpoint_point` issues where required.
- Add tests that assert route operations use the bound net and expected points.
- Add tests that missing coordinates do not produce fake route success.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Add endpoint to anchor route evidence`

## Phase 5: Design Workflow Integration

Integrate binding summaries into `design create`.

Tasks:

- Run endpoint discovery, anchor collection, binding resolution, and route
  evidence generation during generated-board composition.
- Add `anchor_bindings` to the workflow output JSON.
- Make required binding failures affect workflow success/readiness consistently
  with existing validation stages.
- Add CLI golden tests for successful and failing anchor binding summaries.
- Add fixtures or fixture adapters that cover representative KiCad 7, 8, and
  current-version hierarchical net naming when those project files are available
  locally, with skipped evidence when optional version fixtures are absent.
- Keep existing generated project outputs stable except for intentional evidence
  additions.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Expose anchor binding in design workflow`

## Phase 6: Protection Block Fixtures

Exercise the feature against concrete protection blocks.

Tasks:

- Add or extend fixtures for:
  - connector plus ESD protection;
  - power connector plus reverse-polarity protection;
  - downstream protected load net.
- Assert required entry anchors bind to connector pads.
- Assert protected output anchors bind to downstream endpoints when modeled.
- Assert generated routes are present for physical endpoint to anchor links.
- Add negative fixtures for missing connector and mismatched net cases.
- Add a negative geometry fixture for incompatible layer transitions or via
  policy so `route_status` becomes `not_routable` with a specific issue.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Add protection anchor binding fixtures`

## Phase 7: Documentation And Roadmap

Document the new evidence and update project status.

Tasks:

- Update `README.md` with the new anchor binding evidence in `design create`.
- Update `specs/ROADMAP.md` to move physical board-composition binding for
  anchors from remaining work into implemented foundation when complete.
- Add a short note that binding is not a substitute for KiCad DRC or fabrication
  readiness.
- Add examples of bound and unbound evidence if the README already contains
  workflow JSON examples.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Document entry anchor board binding`

## Final Completion Criteria

- All phases are committed independently.
- `go test ./...` passes after the final phase.
- Prism review has been run for each committed phase.
- Required generated ESD and reverse-polarity protection anchors bind to real
  connector pads in deterministic fixtures.
- Missing, ambiguous, invalid, and unsupported bindings are reported with
  actionable issues.
- `design create` includes stable anchor binding evidence for AI callers.
