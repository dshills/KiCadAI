# VCC Route-Tree Path Completion Spec

Date: 2026-07-02

## Objective

Complete the remaining VCC route-tree branch path gap in the KiCad-backed I2C
sensor breakout fixture without weakening validation or promotion gates.

The immediate target is:

```text
examples/design/kicad-backed/i2c_sensor_breakout_candidate
```

The current fixture is no longer blocked by broad GND/SDA route-tree failures.
The selected attempt now proves 11 of 12 required inter-block contacts, reports
3 complete route-tree contact-graph groups, 1 partial group, and narrows the
remaining repair hints to VCC:

- one `ROUTE_GRAPH_INCOMPLETE` contact proof;
- two branch-scoped `no legal two-layer path` blockers for VCC;
- fixed-net skip notices and missing-net-class warnings are separate
  non-repair evidence.

This project should make the final VCC endpoint join the same physical same-net
contact graph when a legal route exists, or else produce enough structured
evidence to drive the next placement/routing repair.

## Non-Goals

- Do not implement a production autorouter.
- Do not relax same-net contact proof, board validation, writer correctness, or
  KiCad ERC/DRC gates.
- Do not treat fixed-net skip notices or missing-net-class warnings as
  route-tree repair blockers.
- Do not promote the I2C fixture unless the promotion report gates support it.
- Do not mutate imported/user-authored KiCad projects.
- Do not broaden this work into amplifier layout, component catalog expansion,
  or unrelated routing features.

## Current Evidence Boundary

The current closeout work established these facts:

- route-tree execution owns VCC/GND/SDA/SCL and excludes those nets from
  fallback net-level routing;
- route-tree endpoint access includes pad and local-route anchor evidence;
- branch evidence records access pair attempts, selected roles, coordinates,
  layers, pair counts, and truncation;
- contact proof classifies same-net graph splits separately from generic
  contact misses;
- repair hints now report VCC-only blockers;
- generated fixed-net skip notices and missing-net-class warnings are counted
  separately from repairable route-tree blockers.

The remaining VCC failure appears after branch access selection, not before it.
The router has candidate access points, tries bounded pairs, and still fails to
produce a route/contact graph that connects every VCC endpoint.

## Required Capabilities

### 1. VCC Branch Failure Snapshot

The implementation must first preserve the current failure shape in tests and
diagnostics.

Required captured evidence:

- selected retry attempt;
- VCC route-tree branch indices;
- selected source/target access roles, refs/pads, layers, and coordinates;
- access pair counts and truncation;
- primary blocker codes/messages for each failed VCC attempt;
- nearest obstacle evidence where available;
- contact graph counts for VCC;
- route-tree repair hints narrowed to VCC.

Acceptance:

- tests fail if the blocker regresses to SDA/GND;
- tests fail if fixed-net skip notices inflate repair hint count;
- tests fail if contact graph evidence drops below 11/12 proven endpoints.

### 2. Same-Net Merge Legality Audit

The router must clearly distinguish legal same-net merge targets from blockers.

The audit must cover:

- VCC pads;
- VCC local-route anchors;
- earlier successful VCC branch copper;
- same-layer merge points;
- via/layer transitions where allowed by policy;
- other-net pads and copper near VCC;
- board edge and keepout constraints.

The output should answer:

- did a legal same-net merge candidate exist?
- was it rejected by occupancy, clearance, layer policy, via policy, grid
  quantization, or search limit?
- did another net’s pad/copper block the selected path?
- did the selected candidate pair require a path through a narrow channel that
  placement retry should widen?

### 3. Access Candidate Scoring Refinement

If VCC has legal access candidates that are not selected early enough, ranking
must be adjusted conservatively.

Allowed ranking refinements:

- prefer candidate pairs with lower immediate obstacle pressure;
- prefer same-layer or legal-via-compatible access pairs;
- prefer local-route anchor to pad only when the local anchor actually shortens
  or legalizes the path;
- avoid candidate pairs whose first/last grid step is already blocked;
- keep stable ref/pad/source tie-breakers.

Candidate limit changes are allowed only with evidence that a legal pair is
excluded by the current bound. If the legal pair is inside the limit, do not
raise the limit.

### 4. Same-Net Branch Merge Execution

If the branch can legally merge with existing same-net VCC copper, the route
executor should allow that merge and record it.

Required behavior:

- same-net VCC copper from earlier branches can be a terminal merge target;
- failed attempts leave no partial copper;
- successful merge evidence records the selected merge role/source;
- contact graph completion uses the merged same-net copper;
- other-net copper remains blocking.

### 5. VCC-Specific Repair Evidence

If VCC still cannot complete after the above improvements, the failure evidence
must be actionable.

Required diagnostics:

- branch index;
- source/target endpoint IDs;
- selected source/target roles and coordinates;
- pair rank and whether the pair list was truncated;
- legal same-net merge candidate count;
- nearest blocker kind/ref/net when available;
- repair category: placement spacing, fanout/layer access, via policy, search
  exhaustion, graph split, or unsupported geometry.

### 6. Promotion Decision

After routing changes, rerun the real KiCad-backed I2C fixture and update
tracked metadata honestly.

Allowed outcomes:

- promote to `candidate` only if route-tree completion, writer correctness,
  board validation, required artifacts, and configured KiCad evidence support
  promotion;
- remain `expected_fail` if any gate remains blocked, but document the exact
  blocker and updated evidence.

## Data And API Expectations

Prefer extending existing internal structs over introducing parallel models.

Likely affected areas:

- `internal/designworkflow/interblock_route_branches.go`
- `internal/designworkflow/route_tree_endpoint_access.go`
- `internal/designworkflow/interblock_contact.go`
- `internal/designworkflow/routing.go`
- `internal/routing/occupancy.go`
- `internal/routing/search.go`
- `internal/routing/route.go`
- `internal/routing/diagnostics.go`

New fields should be compact and JSON-stable. If a richer diagnostic object is
needed, expose it in route-tree branch evidence rather than embedding opaque
text in messages.

## Validation Strategy

Use layered validation:

- small routing unit tests for same-net merge legality;
- route-tree branch tests for candidate ranking and failed-attempt cleanup;
- I2C workflow tests for VCC-only blocker reduction;
- optional KiCad-backed fixture run for promotion evidence;
- full `go test ./...` before final documentation.

## Success Criteria

Minimum success:

- VCC failure evidence becomes more specific than the current graph-split plus
  two generic no-legal-path blockers.
- Fixed-net skip notices and missing-net-class warnings stay out of repair
  failure counts.
- No regression in LED or connector/LED candidate fixtures.

Preferred success:

- the I2C fixture reaches 12/12 route-tree endpoint proof;
- all VCC/GND/SDA/SCL contact graph groups are complete;
- route-tree repair hints drop to zero;
- the promotion report moves past route completion and reaches the next real
  gate.

Promotion success:

- the fixture moves from `expected_fail` to `candidate` only if every declared
  candidate gate passes or is allowed by metadata policy.

