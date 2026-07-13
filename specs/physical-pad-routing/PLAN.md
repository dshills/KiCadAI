# Physical Pad Routing Correctness Plan

## Phase 1: Fail-First Regressions

Files:

- `internal/designworkflow/interblock_candidates_test.go`
- `internal/kicadfiles/designapi/builder_test.go`

Work:

- Add candidate-expansion coverage for uniquely hydrated duplicate-pad aliases.
- Replace the writer auto-tie expectation with a no-implicit-copper contract.

Acceptance:

- Tests demonstrate the missing physical endpoints and unsafe writer behavior.

Risk: low; tests only.

## Phase 2: Physical Pad Hydration and Aliases

Files:

- `internal/designworkflow/placement.go`
- `internal/designworkflow/placement_test.go`
- `internal/designworkflow/pad_hydration.go`
- `internal/designworkflow/pad_hydration_test.go`
- `internal/designworkflow/route_endpoint_resolver.go`
- `internal/designworkflow/route_endpoint_resolver_test.go`

Work:

- Prefer complete resolver-backed or verified footprint pads over incomplete
  logical request pads.
- Overlay graph net assignments onto every matching physical pad.
- Generate deterministic routing-only aliases without changing writer pad names.

Acceptance:

- Four duplicate library shield pads remain four physical routing endpoints.
- Written footprint pads retain their original library names.

Risk: medium; resolver geometry becomes authoritative for routable footprints.

## Phase 3: Route-Path Endpoint Expansion

Files:

- `internal/designworkflow/interblock_candidates.go`
- `internal/designworkflow/interblock_candidates_test.go`
- `internal/designworkflow/explicit_pcb.go`
- `internal/designworkflow/explicit_pcb_test.go`

Work:

- Expand participating route-tree candidates from hydrated placement pads.
- Expand generic explicit-circuit direct-routing requests from hydrated physical
  pads without mutating the placement/writer model.
- Preserve provenance, deterministic ordering, deduplication, and local-island
  pruning.

Acceptance:

- Every matching physical pad alias becomes a required endpoint.
- Unrelated and mismatched pads remain excluded.
- Focused design-workflow tests pass.

Risk: medium; endpoint counts change for both shared routing paths.

## Phase 4: Remove Writer Copper Synthesis

Files:

- `internal/kicadfiles/designapi/builder.go`
- `internal/kicadfiles/designapi/builder_test.go`

Work:

- Remove automatic same-net duplicate-pad track insertion and dead helpers.
- Confirm explicit transaction routes remain the sole source of board tracks.

Acceptance:

- Calling `Design` never adds an unrequested track.
- Design API tests pass.

Risk: medium; hidden unrouted duplicate pads will now surface correctly.

## Phase 5: Promotion Evidence

Files:

- No production files unless current evidence reveals another generic defect.

Work:

- Rebuild `bin/kicadai`.
- Replay the captured live generic BMP280 response.
- Run strict KiCad ERC/DRC and inspect physical-pad contacts.
- Run focused tests, `go test ./...`, and the optional KiCad-backed suite.
- Review staged changes with Prism before commit.

Acceptance:

- Unsafe cross-connector writer ties are absent.
- All required shield and duplicated power/ground pads have routed contact.
- Existing pass fixtures remain clean.
- The next blocker is based on current KiCad evidence.

Risk: medium; DRC may expose later geometry blockers previously masked by shorts.
