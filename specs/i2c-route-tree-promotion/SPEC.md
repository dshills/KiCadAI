# I2C Route-Tree Promotion Closeout Spec

Date: 2026-07-04

## Objective

Promote the first richer generated multi-block PCB fixture,
`examples/design/kicad-backed/i2c_sensor_breakout_candidate`, as far as the
evidence allows by closing its route-tree branch and same-net contact proof
gaps.

The simple LED prompt now has a strict structural candidate lane and a
KiCad-backed pass lane. The next project should prove that KiCadAI can move
beyond the LED-scale case toward a real multi-net generated board without
weakening validation. The immediate target is the I2C sensor breakout because
it already exercises:

- multiple generated blocks;
- VCC/GND/SDA/SCL multi-endpoint inter-block nets;
- local block routes that should act as same-net anchors;
- bounded placement-routing retry;
- promotion reports with structured route-tree evidence.

Success is not defined as "make all warnings disappear." Success is defined as
one of:

- promote the fixture from `expected_fail` to `candidate` or `pass` when the
  generated evidence actually supports that readiness; or
- leave it `expected_fail` with narrower, deterministic, repairable blockers
  that identify the next exact routing or validation project.

## Relationship To Existing Specs

This project builds on:

- `specs/route-tree-branch-path-completion/SPEC.md`;
- `specs/route-tree-branch-path-completion/PLAN.md`;
- `specs/route-tree-access-driven-routing/SPEC.md`;
- `specs/route-tree-vcc-proof-closeout/SPEC.md`;
- `specs/vcc-route-tree-path-completion/SPEC.md`.

Those specs describe the general route-tree machinery. This spec is the
promotion closeout that turns that machinery into a measurable fixture outcome.

## Current Problem

The roadmap currently describes the I2C fixture as progressing past several
older blockers:

- stale routing-skip evidence;
- local `sensor_*` net-alias blockers;
- missing pad/copper net-code blocker;
- generic endpoint-contact evidence gaps.

The remaining selected-attempt blockers are now narrower:

- VCC/SDA same-net graph splits;
- VCC/GND branch-scoped pathfinding blockers;
- route-tree branch/contact proof that does not yet connect every required
  endpoint in each managed net;
- KiCad ERC/DRC-clean proof after internal route completion.

The next implementation must make these blockers concrete and reduce them, not
hide them behind candidate metadata.

## Non-Goals

- Do not implement an unrestricted autorouter.
- Do not claim arbitrary generated boards are DRC-clean.
- Do not relax writer correctness, board validation, route completion, or
  promotion gates.
- Do not add new circuit block families.
- Do not add new component catalog families unless a fixture blocker proves a
  missing verified component is the actual cause.
- Do not mutate imported or user-authored projects.
- Do not require local KiCad for the default test suite.

## Required Capabilities

### 1. Fixture Baseline And Promotion Inventory

The fixture must have a deterministic baseline test that records:

- achieved readiness and promotion status;
- route-tree managed nets;
- required endpoints and proven endpoints;
- complete, partial, and blocked route-tree groups;
- branch attempts, branch completions, and branch blockers;
- graph component counts and missing endpoint lists;
- placement retry selected-attempt evidence;
- writer correctness and board validation statuses;
- KiCad gate status when KiCad is unavailable.

The baseline must fail if route-tree-managed nets silently fall back to generic
net-level routing or if contact proof disappears.

### 2. Same-Net Geometry Semantics

Route-tree branch search and contact proof must distinguish conductive same-net
geometry from blocking geometry.

Required behavior:

- same-net pads can be terminals;
- same-net pads can be legal pass-through or merge nodes when layer and pad
  geometry allow contact;
- generated same-net local routes can be merge anchors;
- successful earlier branch copper can become a legal merge target for later
  same-net branches;
- other-net pads, other-net copper, keepouts, unsupported zones, and board
  edges remain blockers;
- reports distinguish terminal contact, same-net merge, pass-through contact,
  and failure.

### 3. Endpoint Access Selection

Every route-tree branch endpoint should resolve to deterministic physical
access candidates before route search.

Candidate evidence should include:

- endpoint ID;
- ref/pad where known;
- net name and net-code evidence where available;
- layer;
- coordinate;
- source type: hydrated pad, local-route anchor, existing same-net copper, or
  external anchor;
- role: source, target, merge, pass-through, or fallback.

Ranking should prefer:

1. proven same-net local-route anchors;
2. legal pad access points with short route distance;
3. layer-compatible access with fewer immediate obstacles;
4. stable ref/pad/source ordering.

Missing or ambiguous access should produce structured route-tree issues.

### 4. Contact Graph Completion

Completion must be graph-derived.

For each managed net, the contact graph should include:

- required endpoints;
- selected endpoint access candidates;
- generated route segments;
- vias;
- generated local-route copper;
- legal same-net pad contacts;
- legal same-net merge points.

A route-tree group is complete only when every required endpoint belongs to one
same-net component. Partial groups must list split components and missing
endpoints. Contact proof should tolerate writer/readback rounding but still
require real geometric contact.

### 5. Branch Ordering And Clean Retry State

The route tree should avoid deterministic branch order that blocks harder
branches.

Required behavior:

- route access-constrained branches before easy branches when deterministic
  scoring supports it;
- allow later branches to merge into successful earlier same-net copper;
- discard failed partial branch operations;
- ensure retry attempts rebuild route-tree-managed copper from clean generated
  state rather than accumulating stale operations.

### 6. Promotion Metadata

The I2C fixture metadata must reflect the actual evidence:

- promote to `candidate` only when route-tree completion, writer correctness,
  board validation, and promotion gates support candidate readiness;
- promote to `pass` only when required KiCad ERC/DRC and writer round-trip
  evidence are clean;
- keep `expected_fail` only with explicit narrowed blockers;
- keep KiCad evidence separate from internal route-tree blockers.

## Test Requirements

Default tests must not require KiCad. They should use deterministic generated
workflow tests and fake-KiCad tests where external evidence is needed.

Required coverage:

- baseline I2C selected-attempt route-tree evidence;
- same-net pad merge/pass-through behavior;
- local-route anchor access candidates;
- same-net contact graph completion;
- failed branch operation discard;
- retry clean-state behavior;
- LED strict prompt and connector/LED candidate regression protection;
- promotion report consistency for I2C metadata.

Optional coverage:

- real KiCad smoke when `KICADAI_KICAD_CLI` is configured;
- expected-fail update if KiCad finds legitimate DRC issues after internal
  route-tree completion.

## Acceptance Criteria

- `i2c_sensor_breakout_candidate` has deterministic baseline and selected
  attempt tests.
- Route-tree-managed VCC/GND/SDA/SCL nets remain owned by route-tree execution.
- Same-net pads, local routes, and branch copper can serve as legal contact or
  merge evidence where safe.
- Contact graph summaries identify complete, partial, and blocked groups from
  physical graph connectivity.
- The fixture either promotes to `candidate`/`pass` or has narrower documented
  blockers than it has today.
- LED strict prompt candidate/pass tests remain green.
- `go test ./...` remains KiCad-independent and passes.

## Open Questions

- Should the first successful I2C result target `candidate` without requiring
  KiCad, or should the fixture stay `expected_fail` until a real KiCad DRC
  clean run exists?
- Should same-net pass-through through SMD pads be allowed only at terminal
  edges, or can centerline contact be accepted for generated evidence?
- Should contact graph evidence live in `designworkflow` summaries only, or be
  promoted into shared routing report types for reuse by lower-level routing
  tests?
