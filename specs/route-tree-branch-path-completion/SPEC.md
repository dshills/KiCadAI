# Route-Tree Branch Path Completion Spec

Date: 2026-07-02

## Objective

Make generated multi-endpoint inter-block route trees complete real same-net
branches instead of only classifying why they failed.

The immediate target is `examples/design/kicad-backed/i2c_sensor_breakout_candidate`.
The workflow now owns the fixture's VCC/GND/SDA/SCL inter-block nets through
route-tree execution, classifies branch/contact failures, feeds route-tree
repair hints into bounded placement retry, and selects attempts using
route-tree completion evidence. The latest selected attempt still remains
`expected_fail` because GND/SDA branch paths and same-net endpoint contact proof
do not fully complete.

This project must close the next routing gap:

- route branches must be able to legally connect through same-net pads and
  existing same-net local-route anchors;
- contact proof must recognize those completed branches as one same-net graph;
- failed branches must emit richer endpoint/blocker evidence when completion
  is still impossible;
- the I2C fixture should be promoted as far as evidence allows.

## Non-Goals

- Do not implement a full production autorouter.
- Do not route arbitrary dense boards or claim broad DRC-clean layout quality.
- Do not relax board validation, writer correctness, or KiCad evidence gates.
- Do not mutate imported/user-authored projects.
- Do not add new circuit-block families or component-catalog parts.
- Do not promote a fixture unless its metadata-declared gates are actually met.

## Current Behavior

The current route-tree pipeline provides these foundations:

- inter-block route candidates are grouped by multi-endpoint net;
- route trees choose deterministic branch requests;
- route-tree-managed nets are excluded from fallback net-level routing;
- route branches emit same-net copper evidence where they can route;
- endpoint-contact proof checks whether emitted route operations physically
  touch required same-net targets;
- route-tree repair classifies branch/contact failures and feeds bounded retry;
- retry summaries record route-tree complete/partial/blocked groups, routed
  branches, contact misses, and repair issue counts.

The remaining failures are not merely missing diagnostics. The selected retry
attempt still does not produce enough copper/contact proof for every required
GND/SDA endpoint. Likely contributing gaps include:

- same-net pads may still be treated as obstacles rather than valid pass-through
  or terminal graph nodes in some branch searches;
- local-route copper anchors may not be available as first-class route-tree
  graph nodes for inter-block branches;
- branch route requests may terminate at pad centers or abstract endpoints
  without selecting the best physical pad access point;
- branch ordering may create earlier copper that blocks later branch search
  instead of allowing same-net merge;
- contact proof may require exact route-to-target contact and miss valid
  same-net graph connectivity through pad/local-route geometry;
- route-tree issues still do not always carry enough structured endpoint and
  obstacle context for deterministic repair.

## Required Capabilities

### 1. Same-Net Pass-Through And Merge Semantics

Route search and occupancy must distinguish blocking obstacles from same-net
conductive geometry.

Required behavior:

- same-net pads are legal route terminals;
- same-net pads may be legal pass-through nodes when the pad geometry supports
  it and the layer/access policy allows contact;
- same-net generated copper from earlier branches may be a legal merge target;
- same-net local-route segments may be legal merge targets when they are owned
  generated geometry and their net identity is proven;
- other-net pads, other-net copper, keepouts, and board edges remain blockers;
- the route report must state whether a branch ended at its target pad, merged
  into same-net copper, or failed before contact.

This must not make shortcuts through unrelated nets or through unsupported
zone/copper geometry.

### 2. Branch Endpoint Access Selection

Route-tree branch planning must resolve physical access points for every branch
endpoint before route search.

Endpoint access should include:

- ref/pad identity where known;
- net name and net-code evidence;
- layer access candidates;
- pad access point coordinates;
- local-route anchor coordinates where a block-local route already proves the
  same net;
- endpoint role, such as `source_pad`, `target_pad`, `local_route_anchor`,
  `existing_same_net_copper`, or `external_anchor`.

When multiple access points exist, selection must be deterministic and prefer:

1. already proven same-net local-route anchors;
2. pad access points with shortest legal branch distance;
3. layer-compatible points that avoid immediate collision;
4. stable ref/pad ordering as a tie-breaker.

### 3. Contact Graph Completion

Route-tree completion must be measured by a same-net contact graph, not by
raw route-operation counts alone.

For each managed net:

- graph nodes should include required endpoints, routed branch segments, vias,
  generated same-net local routes, valid pad contacts, and legal same-net merge
  points;
- graph edges should represent physical contact on compatible layers or legal
  through-hole/via transitions;
- a route group is complete only when every required endpoint is connected in
  the same contact graph component;
- partial and blocked groups must report missing endpoints and split
  components;
- contact proof must tolerate writer/readback coordinate rounding while still
  requiring actual geometric contact.

### 4. Branch Ordering And Reroute Policy

The route tree must avoid deterministic ordering that traps later branches.

Required behavior:

- branch planning should route hard/short/access-constrained branches before
  easy branches when that improves completion;
- branch search should allow legal same-net merge into previously routed branch
  copper;
- failed branch attempts should not leave partial copper that can poison later
  branches;
- retry attempts must rebuild route-tree-managed copper cleanly instead of
  accumulating stale branch operations.

### 5. Structured Failure Evidence

When a branch still cannot complete, issues must carry enough evidence for the
next repair project.

Route-tree branch issues should include, through structured fields where the
existing report model supports them and through stable path/ref/net fields
otherwise:

- managed net;
- branch index or deterministic branch ID;
- source endpoint role/ref/pad/net;
- target endpoint role/ref/pad/net;
- selected source/target access coordinates;
- nearest blocking obstacle kind and source when available;
- whether same-net merge candidates existed;
- whether the failure was search exhaustion, access failure, collision,
  layer/via policy, or graph split.

Until the shared report model supports arbitrary structured metadata, the path,
refs, nets, and deterministic issue messages must remain stable and covered by
tests.

### 6. I2C Fixture Promotion Path

The implementation must run the I2C fixture and update metadata based on the
actual result.

Allowed outcomes:

- `candidate` or `pass` if route-tree completion, writer correctness, board
  validation, promotion, and configured KiCad evidence support promotion;
- continued `expected_fail` only if the remaining blockers are narrower,
  structured, and explicitly documented.

The desired near-term target is at least:

- VCC/GND/SDA/SCL route-tree groups are attempted;
- completed/proven branch count increases versus the current baseline;
- selected attempt reports fewer branch/contact proof blockers;
- promotion report identifies any remaining KiCad/ERC/DRC blockers separately
  from internal route-tree completion blockers.

## Data Model

New or extended types may include:

```go
type RouteTreeEndpointAccessRole string

const (
    RouteTreeAccessSourcePad       RouteTreeEndpointAccessRole = "source_pad"
    RouteTreeAccessTargetPad       RouteTreeEndpointAccessRole = "target_pad"
    RouteTreeAccessLocalRouteAnchor RouteTreeEndpointAccessRole = "local_route_anchor"
    RouteTreeAccessSameNetCopper   RouteTreeEndpointAccessRole = "same_net_copper"
    RouteTreeAccessExternalAnchor  RouteTreeEndpointAccessRole = "external_anchor"
)

type RouteTreeEndpointAccess struct {
    EndpointID string                      `json:"endpoint_id"`
    Role       RouteTreeEndpointAccessRole `json:"role"`
    Ref        string                      `json:"ref,omitempty"`
    Pad        string                      `json:"pad,omitempty"`
    Net        string                      `json:"net"`
    Layer      string                      `json:"layer,omitempty"`
    XMM        float64                     `json:"x_mm"`
    YMM        float64                     `json:"y_mm"`
    Source     string                      `json:"source"`
}

type RouteTreeBranchCompletion struct {
    Net              string   `json:"net"`
    BranchID         string   `json:"branch_id"`
    SourceEndpointID string   `json:"source_endpoint_id"`
    TargetEndpointID string   `json:"target_endpoint_id"`
    Status           string   `json:"status"`
    MergeKind        string   `json:"merge_kind,omitempty"`
    MergeSource      string   `json:"merge_source,omitempty"`
    SegmentCount     int      `json:"segment_count"`
    ContactProven    bool     `json:"contact_proven"`
    Issues           []string `json:"issues,omitempty"`
}

type RouteTreeContactGraphSummary struct {
    Nets               []string `json:"nets,omitempty"`
    RequiredEndpoints  int      `json:"required_endpoints"`
    ProvenEndpoints    int      `json:"proven_endpoints"`
    Components         int      `json:"components"`
    CompleteGroups     int      `json:"complete_groups"`
    PartialGroups      int      `json:"partial_groups"`
    BlockedGroups      int      `json:"blocked_groups"`
    SameNetMerges      int      `json:"same_net_merges"`
    LocalRouteMerges   int      `json:"local_route_merges"`
}
```

The final implementation may use existing routing and designworkflow structs if
that keeps the model smaller. The important contract is stable evidence for
endpoint access, same-net merge, branch completion, and contact graph status.

## Algorithm

For each route-tree-managed net:

1. Resolve required endpoints from promoted inter-block route candidates.
2. Build endpoint access candidates from hydrated pads, local routes,
   generated same-net copper, and external anchors.
3. Build a contact graph seed from already proven same-net local routes and
   endpoint pads.
4. Choose deterministic branch requests using route-tree topology plus access
   quality.
5. Route each branch with occupancy that treats same-net pads/copper as legal
   merge/contact geometry.
6. On success, add branch copper and update the contact graph.
7. On failure, discard partial branch operations and emit structured branch
   failure evidence.
8. After all branches, compute group completion from the contact graph.
9. Feed any remaining structured failures into existing route-tree repair and
   placement retry.
10. Write promotion evidence and fixture metadata from the selected attempt.

## Compatibility

Existing behavior must remain stable for:

- single-endpoint or two-endpoint routing;
- local-route-only block verification;
- LED and connector/LED KiCad-backed candidate fixtures;
- board-validation pad/copper net checks;
- optional KiCad CLI behavior;
- generated-project ownership and repair bundle behavior.

The new route-tree completion logic must be deterministic across runs.

## Testing Requirements

Minimum tests:

- same-net pad is a legal terminal and other-net pad remains an obstacle;
- same-net existing branch copper is a legal merge target;
- same-net local-route anchor can satisfy a branch endpoint;
- partial failed branch copper is not emitted;
- route-tree contact graph marks a net complete only when all required
  endpoints are in one component;
- route-tree summaries expose same-net/local-route merge counts;
- retry selection still prefers fewer blocked route-tree groups;
- LED and connector/LED fixture behavior does not regress;
- I2C fixture baseline shows either promotion or narrower blockers.

Optional tests when local KiCad is configured:

- generated I2C fixture can run through KiCad-backed promotion evidence;
- DRC findings are reported separately from internal contact graph blockers;
- promoted candidate metadata matches actual generated evidence.

## Acceptance Gates

- `go test ./internal/routing -run 'SameNet|Occupancy|Contact|Route' -count=1`
  passes.
- `go test ./internal/designworkflow -run 'RouteTree|InterBlock|I2C|Retry' -count=1`
  passes.
- `go test ./...` passes.
- The I2C fixture run records updated route-tree/contact evidence.
- Metadata and roadmap reflect whether the fixture promoted or what exact
  blockers remain.
- Prism review has no unresolved high or medium findings.

