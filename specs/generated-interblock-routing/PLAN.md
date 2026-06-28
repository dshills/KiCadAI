# Generated Inter-Block Routing Implementation Plan

Date: 2026-06-28

## Objective

Close the next generated-board blocker after block-local route endpoint
binding: route generated electrical connections between blocks and expose
deterministic route-completion evidence.

The immediate target is connector/LED. Its block-local LED route now binds to
physical pads, but connector-to-LED inter-block connections still need routed
same-net copper and validation evidence before fixture promotion.

## Phase 1: Audit Inter-Block Routing Gaps

### Tasks

- Add KiCad-independent tests that generate connector/LED with routing enabled
  and capture current inter-block routing status.
- Distinguish:
  - block-local routes;
  - placement/routing engine routes;
  - skipped routing fixture policy;
  - board-validation disconnected/unrouted net issues.
- Add assertions that current failures are inter-block route-completion
  blockers, not local route endpoint blockers.
- Document which connector/LED nets have physical endpoints and which are
  missing endpoint evidence.

### Acceptance

- Current connector/LED inter-block routing gap is reproduced without KiCad.
- Tests prove local route endpoint binding remains intact.
- Failure categories are deterministic.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated inter-block routing audit`

## Phase 2: Inter-Block Route Candidate Builder

### Tasks

- Add a generated inter-block route candidate builder in the design workflow.
- Promote request-level block connections into placement request nets before
  placement so component placement and routing optimize for the same
  inter-block connectivity.
- Derive candidates from:
  - request connections;
  - normalized block composition;
  - placement request nets;
  - schematic-to-PCB transfer evidence where available.
- Resolve connection aliases to canonical net names before route candidate
  derivation while retaining source alias provenance, and reject alias
  collisions before distinct logical net identities are discarded.
- Add regression tests for alias collisions where two distinct logical nets
  would be merged by conflicting aliases.
- Resolve endpoints through the placed pad endpoint resolver.
- Classify route candidates as local, inter-block, anchor, global, or
  external.
- Emit structured issues for missing refs, missing pads, net mismatches, and
  insufficient endpoints.

### Acceptance

- Unit tests cover connector/LED route candidate derivation.
- Candidate output includes net name, endpoints, block IDs/instance IDs, pad
  coordinates, layers, and status.
- Missing endpoint evidence is deterministic and actionable.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated inter-block route candidates`

## Phase 3: Route Inter-Block Candidates

### Tasks

- Feed routable inter-block candidates into the existing routing engine or
  placement-to-routing adapter.
- Keep block-local route operations separate from inter-block route operations.
- Treat existing block-local copper on the target layers as routing obstacles
  unless it belongs to the same canonical net and is exposed through an access
  point.
- Preprocess block-local copper obstacles into simplified geometry before
  routing so dense generated blocks do not explode pathfinding cost.
- Preserve net names, net codes, route widths, layers, and operation
  correlation metadata.
- Block cross-layer routes unless the existing engine emits valid vias and
  validation can prove them.
- Add tests for direct or Manhattan connector-to-LED routing.

### Acceptance

- Connector/LED with routing enabled emits inter-block route operations for at
  least the supported signal path when endpoints are available.
- Route operations start/end on same-net physical pad anchors or validated
  routing access points.
- Unsupported or blocked nets produce actionable diagnostics.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Route generated inter-block candidates`

## Phase 4: Route Completion Evidence

### Tasks

- Add inter-block route-completion summary fields to the routing stage.
- Include:
  - inter-block nets considered;
  - route candidates;
  - routes attempted;
  - routes completed;
  - endpoints resolved/unresolved;
  - partial/unrouted nets;
  - emitted segments;
  - issue count.
- Add stable JSON/golden-style regression coverage.
- Use deterministic placement and routing seeds in regression tests.
- Record the placement and routing seeds in route-completion evidence for
  reproduction of CI failures.
- Canonicalize evidence in golden-style tests by comparing net names,
  endpoint refs/pads, route status, and completion counts, and assert route
  geometry with tolerance-based matchers rather than string-rounded
  coordinates.
- Surface concise evidence in `design create` output.

### Acceptance

- Routing stage exposes deterministic inter-block routing evidence.
- Evidence distinguishes local route completion from inter-block route
  completion.
- CLI JSON remains compact and stable.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Expose generated inter-block route evidence`

## Phase 5: Connector/LED Fixture Progression

### Tasks

- Add or update connector/LED tests to assert inter-block route evidence.
- Decide whether to:
  - keep the metadata fixture `skip_routing` policy and add a separate
    routing-enabled regression request; or
  - enable routing in the fixture if internal and optional KiCad evidence
    justify it.
- Update metadata known gaps to the new blocker:
  - inter-block route completion;
  - KiCad ERC/DRC proof;
  - schematic ERC if still present.
- Run optional KiCad checks when configured; otherwise verify skip behavior.

### Acceptance

- Connector/LED no longer reports generic local route endpoint blockers.
- If routing succeeds internally, fixture metadata is narrowed or promoted
  according to evidence.
- If routing remains blocked, the blocker names exact nets/endpoints.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Advance connector LED inter-block routing evidence`

## Phase 6: I2C Sensor Breakout Evidence

### Tasks

- Run the same inter-block candidate builder and evidence summary against the
  I2C sensor breakout candidate.
- Identify which nets are routable, blocked, global, or deferred.
- Add tests that assert deterministic evidence without requiring KiCad.
- Update I2C metadata only when the failure reason narrows.

### Acceptance

- I2C fixture route evidence is more specific than generic expected-fail.
- Unsupported global or richer bus routing cases are explicit.
- Connector/LED behavior does not regress.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Report I2C inter-block routing evidence`

## Phase 7: Documentation And Roadmap Update

### Tasks

- Update README with generated inter-block routing status.
- Update `examples/design/kicad-backed/README.md` fixture table and readiness
  notes.
- Update `specs/ROADMAP.md` with completed and remaining routing blockers.
- Add a short documentation note about local vs inter-block route evidence.
- Ensure all tests pass.

### Acceptance

- Documentation distinguishes local route endpoint binding from inter-block
  route completion.
- ROADMAP names the next blocker after this project.
- `go test ./...` passes.
- Prism review has been run for each implementation phase.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Document generated inter-block routing progress`

## Implementation Notes

- Prefer extending existing routing adapter and validation paths.
- Keep routing attempts conservative and deterministic.
- Do not treat a visually plausible route as complete unless validation can
  prove same-net connectivity.
- Keep KiCad-backed validation optional and environment-gated.
- Do not promote fixtures without evidence.

## Done Definition

- Generated workflow can derive inter-block route candidates from placed design
  connections.
- Supported inter-block candidates can be routed through existing routing
  infrastructure.
- Routing stage evidence separates local and inter-block completion.
- Connector/LED fixture evidence progresses or narrows to exact blockers.
- I2C fixture evidence becomes more specific.
- Default tests remain KiCad-independent.
- Each phase is reviewed with Prism and committed independently.
