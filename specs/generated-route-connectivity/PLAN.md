# Generated Route Endpoint Connectivity Implementation Plan

Date: 2026-06-28

## Objective

Close the generated route endpoint connectivity gap so generated local routes
and routed copper physically connect to same-net placed pads.

The immediate success target is the optional KiCad-backed LED fixture: after
generated net assignment, its next blocker is that local-route copper can have
the right net code while still missing the pads it is meant to connect. This
plan makes route endpoints resolve from hydrated placed pad anchors, then
extends the same path to connector/LED and richer generated examples.

## Phase 1: Capture Current Route Endpoint Failures

### Tasks

- Add or extend KiCad-independent tests that generate the optional
  KiCad-backed design fixtures without invoking KiCad.
- Capture current route-connectivity failures for:
  - LED indicator;
  - connector/LED;
  - I2C sensor breakout where relevant.
- Assert failure categories rather than brittle full error text:
  - disconnected same-net pad;
  - route endpoint not connected to a pad;
  - partially routed net;
  - stale local-route coordinate.
- Add a small negative board-validation fixture proving that a track with the
  correct net still fails when its endpoint misses the pad.

### Acceptance

- The current route endpoint blocker is reproducible without KiCad.
- Tests distinguish net-assignment success from physical connectivity failure.
- No generated routing behavior changes in this phase.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated route connectivity audit`

## Phase 2: Add Placed Pad Endpoint Resolver

### Tasks

- Add a resolver that returns physical pad endpoints for generated placed
  components.
- Resolve endpoints from:
  - component reference;
  - footprint identity;
  - hydrated pad number/name;
  - final component placement;
  - component rotation;
  - layer or side where modeled;
  - assigned pad net name and net code.
- Reuse existing footprint hydration and placement data structures.
- Emit deterministic diagnostics for missing components, missing pads, missing
  pad geometry, unsupported side transforms, and netless active pads.
- Add unit tests for:
  - zero-rotation top-side pad offsets;
  - 90-degree rotation;
  - 180-degree rotation;
  - unknown pad;
  - missing pad geometry;
  - net mismatch-ready endpoint records.

### Acceptance

- Supported placed pads resolve to deterministic board coordinates.
- Rotated pad coordinates are covered by tests.
- Missing endpoint evidence fails with actionable diagnostics.
- The resolver does not guess from component origin when pad geometry is
  unavailable.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated physical endpoint resolver`

## Phase 3: Bind Local Routes To Physical Endpoints

### Tasks

- Connect block-local route declarations to the placed pad endpoint resolver.
- Convert supported local routes into route operations whose start and end
  points are the physical source and destination pad anchors.
- Preserve:
  - route ID;
  - source block ID;
  - net name and net code;
  - route width;
  - layer;
  - route mobility classification;
  - operation correlation metadata.
- Replace stale local-route endpoint coordinates with resolved physical pad
  coordinates.
- Preserve intermediate waypoints only when they remain valid after endpoint
  rebinding; otherwise emit a diagnostic or generate the conservative supported
  route shape.
- Add tests for simple LED-style pad-to-pad binding.

### Acceptance

- Supported local routes no longer start or end at stale block-local
  coordinates.
- Local routes with unresolved endpoints are blocked with diagnostics instead
  of emitted as misleading copper.
- Generated route operations retain correct net identity.
- LED-style fixture generation produces route operations whose endpoints match
  same-net pad anchors in internal evidence.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Bind generated local routes to pad endpoints`

## Phase 4: Add Route Endpoint Preflight And Evidence

### Tasks

- Add generated route endpoint preflight validation before final PCB write.
- Validate that emitted route copper:
  - touches its declared source pad endpoint;
  - touches its declared destination pad endpoint;
  - uses a net code matching both endpoint pads;
  - references known board net table entries.
- Add design workflow route-connectivity evidence with counts for:
  - route bindings attempted;
  - routes bound;
  - endpoints resolved;
  - endpoints unresolved;
  - endpoint contacts proven;
  - endpoint net mismatches;
  - emitted track segments.
- Persist detailed evidence alongside existing generated workflow artifacts
  when artifact output is enabled.
- Surface compact CLI JSON fields in `design create`.

### Acceptance

- Generated routes that miss their endpoints fail before being reported as
  successful output.
- Route-connectivity evidence is deterministic and golden-testable.
- CLI JSON exposes enough summary data for AI agents to explain generated
  design quality.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Expose generated route connectivity evidence`

## Phase 5: Advance LED KiCad-Backed Fixture

### Tasks

- Apply the endpoint binding path to the LED KiCad-backed generated fixture.
- Update fixture metadata and expected failure categories.
- Promote the LED fixture to `candidate` or `pass` only if internal validation
  and optional KiCad evidence justify it.
- If the fixture remains `expected_fail`, narrow the documented blocker to the
  next actual issue.
- Add regression coverage ensuring the LED route endpoint touches the intended
  resistor and LED pads.
- Run optional KiCad validation when configured; otherwise verify skip behavior.

### Acceptance

- The LED fixture no longer fails because same-net route endpoints miss the
  intended pads, or the remaining issue is proven outside endpoint binding.
- Fixture metadata reflects the new blocker or readiness level.
- Optional KiCad-backed tests still skip cleanly without KiCad.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Advance LED route connectivity evidence`

## Phase 6: Extend Connector And Multi-Component Coverage

### Tasks

- Apply route endpoint binding to connector/LED generated fixtures.
- Cover connector pad endpoints, resistor endpoints, LED endpoints, and
  power/ground route endpoints where evidence exists.
- Add tests for multi-component local route binding across more than one block
  or component family.
- Ensure route evidence names unresolved endpoints and mismatches precisely.
- Update connector/LED fixture metadata to the new readiness or narrower
  expected-failure category.

### Acceptance

- Connector/LED route endpoint failures are either closed or narrowed to
  specific unresolved endpoint evidence.
- Multi-component route binding does not regress the LED fixture.
- Diagnostics identify exact route IDs, references, pads, and nets.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Extend generated route connectivity to connector fixtures`

## Phase 7: Golden Coverage, Documentation, And Roadmap Update

### Tasks

- Add golden snapshots for physical endpoint evidence.
- Add golden snapshots for route binding evidence.
- Add regression tests that fail if generated local-route endpoints drift away
  from same-net pad anchors.
- Update README with the generated route-connectivity guarantee and limits.
- Update `specs/ROADMAP.md` to reflect:
  - net assignment closed;
  - route endpoint connectivity status;
  - remaining blockers for KiCad-backed fixture promotion.
- Document any intentionally unsupported cases, such as bottom-side mirrored
  endpoint binding if not yet modeled.

### Acceptance

- Golden evidence is deterministic and reviewable.
- Documentation accurately describes what generated boards can and cannot yet
  prove.
- ROADMAP names the next blocker after this project.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Document generated route connectivity progress`

## Implementation Notes

- Prefer extending existing routing, placement, and workflow evidence models
  over introducing a parallel router.
- Keep the first routing shape conservative; direct or simple orthogonal
  tracks are acceptable if they physically connect the intended pads.
- Do not weaken validation to make fixtures pass.
- Do not emit route copper when endpoint evidence is missing or conflicting.
- Keep KiCad-backed validation optional and environment-gated.
- Preserve operation correlation so repair and validation reports remain useful
  to AI agents.

## Done Definition

- Generated route endpoints are bound to physical placed pad anchors where
  footprint pad geometry is available.
- Supported local routes emit copper that touches same-net endpoint pads.
- Route endpoint preflight catches stale or unresolved generated routes.
- LED KiCad-backed fixture evidence progresses past the route-endpoint blocker.
- Connector/LED evidence is improved or narrowed with deterministic blockers.
- Default tests remain KiCad-independent.
- Prism review has been run for each implementation phase.
- Each implementation phase is committed independently.
