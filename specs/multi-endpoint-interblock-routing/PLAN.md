# Multi-Endpoint Inter-Block Routing Implementation Plan

Date: 2026-07-01

## Objective

Make generated inter-block routing handle nets with three or more required
endpoints, starting with the I2C sensor breakout fixture. The goal is to move
from pairwise route attempts toward deterministic route-tree planning and
same-net graph completion for the whole net group.

## Phase 1: Baseline And Route Group Audit

### Objectives

- Capture the exact current I2C multi-endpoint routing failure.
- Prove which endpoints exist, which branches are attempted, and which targets
  are missing.

### Tasks

- Add focused tests around the current I2C route summary:
  - `VCC`;
  - `GND`;
  - `SDA`;
  - `SCL`.
- Assert local route aliasing stays clean:
  - local route contacts are bound;
  - endpoint net mismatches remain zero.
- Add or extend test helpers that extract route groups, endpoint targets,
  branch/contact issues, and route operations by net.
- Record current blockers with net, ref, pad, branch, and issue code where
  possible.

### Acceptance

- The I2C fixture expected-fail state is reproduced deterministically without
  KiCad.
- Tests distinguish local route success from multi-endpoint inter-block
  failure.
- The failing boundary is named precisely enough to guide implementation.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Audit I2C multi-endpoint routing groups`.

## Phase 2: Multi-Endpoint Route Group Model

### Objectives

- Represent one canonical net and all required physical endpoints as a single
  route group.

### Tasks

- Add route group structures in the existing design workflow or routing adapter
  path.
- Group inter-block candidates by canonical net before route planning.
- Deduplicate repeated endpoint/access targets.
- Preserve endpoint provenance:
  - request connection;
  - block instance;
  - block ID;
  - ref;
  - pad;
  - source role;
  - target requirement.
- Classify endpoints as required or optional.
- Surface unresolved required endpoints as structured issues.
- Add unit tests for:
  - three-endpoint grouping;
  - duplicate target deduplication;
  - unresolved required endpoint diagnostics;
  - alias provenance preservation.

### Acceptance

- Multi-endpoint route groups are deterministic.
- I2C VCC/GND/SDA/SCL groups include expected sensor, connector, pull-up, and
  decoupling targets.
- No existing two-endpoint connector/LED route behavior regresses.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Model multi-endpoint inter-block route groups`.

## Phase 3: Deterministic Route Tree Planning

### Objectives

- Plan branch order for multi-endpoint groups before route search.

### Tasks

- Implement deterministic root selection:
  - board interface or connector first when available;
  - supply/source endpoint when known;
  - central endpoint by Manhattan distance otherwise;
  - stable ref/pad tie-breaker.
- Implement initial branch ordering with nearest-neighbor or MST-style
  behavior.
- Emit route tree evidence:
  - root target;
  - branch index;
  - branch start target;
  - branch end target;
  - planned distance;
  - source endpoint IDs.
- Add tests for root selection, tie-breaking, branch order, and deterministic
  repeatability.

### Acceptance

- Identical inputs produce identical route trees.
- Route tree evidence is compact and suitable for CLI JSON summaries.
- Existing pairwise route requests can still be represented as two-endpoint
  route trees.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Plan deterministic multi-endpoint route trees`.

## Phase 4: Branch Routing And Same-Net Copper Reuse

### Objectives

- Route multi-endpoint branches through the existing routing engine while
  allowing proven same-net copper to become part of the route tree.

### Tasks

- Convert each route tree branch into an existing routing request.
- Route from the current same-net tree to the next required endpoint.
- After each branch, update the same-net graph/contact evidence.
- Allow branch starts from existing same-net copper only when contact proof or
  graph evidence validates the copper.
- Preserve operation IDs and branch IDs.
- Block wrong-net or unproven copper reuse.
- Add tests for:
  - three-pad route completion;
  - same-net copper branch start reuse;
  - wrong-net copper rejection;
  - branch partial route with exact missing endpoint.

### Acceptance

- A small three-endpoint generated route can complete through a route tree.
- Partial branch failures do not inflate completed route counts.
- Route operations remain writer-compatible.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Route multi-endpoint branches through same-net trees`.

## Phase 5: Completion Semantics And Workflow Summaries

### Objectives

- Count route completion at the net-group level and expose actionable
  per-branch evidence.

### Tasks

- Update inter-block route completion semantics:
  - branch attempted;
  - branch contact-proven;
  - branch graph-connected;
  - net group complete;
  - net group partial;
  - net group blocked.
- Extend routing summaries with:
  - multi-endpoint net count;
  - required endpoints;
  - proven endpoints;
  - branch attempts;
  - branch completions;
  - graph component count;
  - missing required endpoints.
- Keep existing summary fields backward-compatible where practical.
- Map branch failures to repair diagnostics and placement retry hints.
- Add workflow tests for summary shape and issue paths.

### Acceptance

- `routes_completed` reflects graph-proven complete net groups.
- I2C summaries name exact missing endpoints and branch blockers.
- Existing connector/LED candidate fixture behavior does not regress.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Summarize multi-endpoint route completion`.

## Phase 6: I2C Fixture Narrowing Or Promotion

### Objectives

- Apply the route-tree implementation to `i2c_sensor_breakout_candidate`.

### Tasks

- Run the checked-in I2C fixture through `design create`.
- Inspect VCC/GND/SDA/SCL route group outcomes.
- If internal route completion succeeds:
  - allow project write;
  - verify writer correctness;
  - verify board validation;
  - verify generated artifacts;
  - evaluate optional KiCad policy if configured.
- If internal route completion still blocks:
  - keep metadata `expected_fail`;
  - update known gaps with exact route tree branch blockers.
- Update `examples/design/kicad-backed/README.md` if fixture status changes.
- Update `specs/ROADMAP.md`.

### Acceptance

- I2C either moves to `candidate` with complete internal evidence, or its
  `expected_fail` blocker is narrower than the current generic multi-endpoint
  route completion failure.
- Metadata matches the actual workflow artifacts and stages.
- The fixture run is reproducible.
- `go test ./...` passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Advance I2C multi-endpoint route completion`.

## Phase 7: Documentation And Regression Closeout

### Objectives

- Make the new routing capability understandable and stable.

### Tasks

- Update user-facing docs only if CLI output or commands changed.
- Update focused routing documentation if available.
- Add regression notes to the roadmap:
  - what multi-endpoint routing now proves;
  - what remains blocked;
  - whether I2C changed readiness.
- Run:
  - focused design workflow tests;
  - routing package tests;
  - `go test ./...`;
  - optional checked-in fixture command.
- Confirm ignored generated outputs are not staged.

### Acceptance

- Documentation does not overclaim autonomous fabrication readiness.
- Tests cover route grouping, tree planning, branch routing, completion
  semantics, and I2C summary behavior.
- Working tree contains only intended source/docs changes before commit.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document multi-endpoint routing support`.

## Implementation Notes

- Prefer extending the existing generated inter-block routing path over adding
  a separate router.
- Keep group and branch IDs deterministic for stable tests.
- Treat power/ground zone completion as unsupported unless a later project adds
  zone-sufficient proof.
- Keep KiCad-backed checks optional and metadata-driven.
- Do not promote I2C merely because route operations are emitted; require
  same-net graph proof for every required endpoint.
