# Inter-Block Endpoint Contact Completion Implementation Plan

Date: 2026-06-28

## Objective

Make generated inter-block routing completion depend on physical same-net
contact and same-net graph connectivity, so the workflow can distinguish
"route emitted" from "route electrically connected."

The immediate target is the connector/LED generated fixture, where an
inter-block route can be attempted for `LED_EN` but validation may still report
disconnected pads.

## Phase 1: Contact Gap Audit

### Tasks

- Add or tighten KiCad-independent regression coverage for connector/LED with
  routing enabled.
- Capture the current route evidence for `LED_EN`:
  - candidate endpoints;
  - emitted route operations;
  - pad targets;
  - board-validation disconnected/unrouted findings.
- Assert that the failing boundary is endpoint contact or graph connectivity,
  not candidate discovery or local-route endpoint binding.
- Add test helpers that can locate route operations and validation issues by
  net name and operation ID.
- Document the observed current blocker in the spec audit notes or test
  failure messages.

### Acceptance

- The current connector/LED contact gap is reproduced deterministically without
  KiCad.
- Tests prove the LED local-route endpoint binding path still passes.
- Failure output names the exact net, refs/pads, and contact boundary.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Audit inter-block route contact gap`

## Phase 2: Contact Target And Proof Model

### Tasks

- Add a contact target model for physical pads, access points, vias, and
  same-net copper.
- Add a contact proof model with stable status values:
  - `proven`;
  - `miss`;
  - `net_mismatch`;
  - `layer_mismatch`;
  - `missing_target`;
  - `unsupported_geometry`;
  - `ambiguous`.
- Resolve contact targets from existing placed pad endpoints and block-local
  access point evidence.
- Preserve net name, net code, layer, component reference, pad identity, block
  ID, instance ID, coordinate, tolerance, and evidence source.
- Add unit tests for resolved pads, missing pads, net mismatches, layer
  mismatches, and unsupported geometry.

### Acceptance

- Contact targets are deterministic and traceable to physical endpoint
  evidence.
- Missing or unsafe targets produce structured diagnostics instead of fallback
  coordinates.
- Contact proof JSON is stable enough for workflow artifacts.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add inter-block route contact proofs`

## Phase 3: Route Endpoint Contact Validation

### Tasks

- Add contact validation for emitted inter-block route operations.
- Compare emitted track start/end coordinates against required contact targets.
- Use a single explicit contact tolerance compatible with existing
  board-validation and writer rounding behavior.
- Validate net name, net code, and layer before accepting coordinate contact.
- Emit stable issue codes for:
  - missing target;
  - net mismatch;
  - layer mismatch;
  - coordinate miss;
  - ambiguous contact;
  - unsupported geometry.
- Correlate contact issues with route operation IDs and existing workflow issue
  paths.
- Add regression tests for direct hit, near miss, wrong net, wrong layer, and
  missing target.

### Acceptance

- Attempted routes receive contact proofs before they are counted as complete.
- Contact misses are visible as route-stage diagnostics and board-validation
  evidence.
- Existing board validation behavior is not weakened.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Validate inter-block route endpoint contacts`

## Phase 4: Same-Net Connectivity Graph Completion

### Tasks

- Build or extend a same-net graph for generated route completion.
- Include same-net pads, inter-block tracks, block-local tracks, validated
  access points, and supported vias.
- Exclude zones unless a zone-sufficient policy explicitly proves
  connectivity.
- Mark a route `connected` only when all required endpoint targets belong to
  the same same-net connected component.
- Change inter-block `routes_completed` semantics to count graph-proven routes,
  not only emitted route operations.
- Preserve separate counts for candidates, attempted routes, contact-proven
  routes, connected routes, partial routes, and blocked routes.
- Add tests for:
  - direct pad-to-pad connected route;
  - emitted route that misses one pad;
  - same-net track chain through a validated access point;
  - wrong-net copper that must not connect graph components.

### Acceptance

- Inter-block completion evidence matches graph connectivity.
- Partial routes remain visible and do not inflate completed counts.
- Existing local, anchor, and block-local route evidence does not regress.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Complete inter-block routes by contact graph`

## Phase 5: Connector LED Route Completion

### Tasks

- Apply contact proof and graph completion to the connector/LED routing path.
- Ensure the `LED_EN` route starts and ends on same-net physical pad or access
  targets where the current router can prove a route.
- If necessary, adjust route endpoint snapping so emitted copper terminates at
  the resolved target coordinate rather than stale or approximate coordinates.
- Update connector/LED assertions to require contact proof and connected status
  for the supported signal path, or to assert a narrower blocker if a different
  routing limitation remains.
- Update fixture metadata known gaps only when the evidence genuinely narrows.
- Preserve skip behavior for optional KiCad-backed checks when KiCad is not
  configured.

### Acceptance

- Connector/LED no longer treats route emission alone as completion.
- The supported `LED_EN` path is either graph-connected or blocked with an
  exact contact/geometry issue.
- The old generic disconnected-pad failure is resolved or mapped to a narrower
  proven blocker.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Prove connector LED inter-block contact`

## Phase 6: I2C Contact Evidence Expansion

### Tasks

- Add inter-block contact proof summaries for I2C sensor breakout candidates.
- Report contact state for VCC, GND, SDA, and SCL when route attempts exist.
- Distinguish global-net or deferred power routing blockers from signal contact
  blockers.
- Add deterministic tests for candidate evidence and contact summary shape.
- Avoid promoting fixture readiness unless internal validation and optional
  KiCad-backed evidence justify it.

### Acceptance

- I2C evidence names which nets have contact targets, attempted routes,
  contact proofs, partial graph connectivity, or blockers.
- Remaining `expected_fail` status is narrower and more actionable.
- Connector/LED behavior does not regress.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Report I2C inter-block contact evidence`

## Phase 7: Repair Diagnostics And Documentation

### Tasks

- Map contact failures into repairable diagnostics where existing repair
  infrastructure can consume them.
- Add suggested actions for:
  - snap endpoint to pad/access point;
  - regenerate route from access point;
  - move placement;
  - block unsupported via or geometry;
  - request manual routing.
- Update README with current inter-block contact guarantee and limits.
- Update `docs/layout-routing.md` with local, inter-block, contact-proof, and
  graph-completion terminology.
- Update `examples/design/kicad-backed/README.md` with fixture status.
- Update `specs/ROADMAP.md` to mark this item complete and name the next
  blocker.

### Acceptance

- Route-contact failures produce operation-correlated, repairable diagnostics.
- Documentation distinguishes route candidates, route attempts, contact proof,
  and graph completion.
- ROADMAP accurately reports whether connector/LED is still `expected_fail`,
  `candidate`, or `pass`.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Document inter-block route contact completion`
