# Routing Connectivity Junctions Plan

## Phase 1: Regression Tests

Files:

- `internal/routing/validation_test.go`

Work:

- Add focused T-junction, crossing, via-interior, different-layer, near-miss, and
  ordering tests against route connectivity validation.
- Add a routing regression where one duplicate-pad endpoint participates in two
  branches and must retain one physical access point.

Acceptance:

- Tests demonstrate the current false-disconnect behavior while retaining negative
  fail-closed cases.

Risk:

- Low; test-only.

## Phase 2: Connectivity Graph Correction

Files:

- `internal/routing/validation.go`
- `internal/routing/validation_test.go`

Work:

- Track segment graph components by normalized layer.
- Union components for exact same-layer segment intersections.
- Union via layer points with segments that contain the via center.
- Cache the first successful physical access point and layer selected for each
  endpoint in a net, then restrict subsequent branch searches to that access.
- Keep canonical coordinate rounding and deterministic graph construction.

Acceptance:

- All focused positive and negative tests pass.
- Existing routing tests pass.

Risk:

- Medium; this changes shared electrical-connectivity classification.

## Phase 3: Milestone and Regression Evidence

Files:

- No production files unless evidence identifies another shared defect.

Work:

- Rebuild `bin/kicadai` from current source.
- Replay the captured live generic BMP280 graph and run the recorded target lane.
- Run the optional KiCad-backed fixture suite.
- Run broader Go tests and lint if the blocker is cleared.
- Review staged changes with Prism before commit.

Acceptance:

- The captured live route no longer has a false `DISCONNECTED_PAD` finding.
- Existing pass fixtures remain clean.
- Any next blocker is reported from current code and current CLI evidence.

Risk:

- Medium; downstream KiCad DRC may reveal real geometry that was previously masked.
