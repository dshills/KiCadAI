# Route-Tree Access-Driven Routing Spec

Date: 2026-07-02

## Objective

Use route-tree endpoint access and contact-graph evidence to choose better
physical branch terminals, snap branch routes to valid same-net contact points,
and eliminate the remaining I2C selected-attempt GND/SDA branch/contact
blockers where the current router can do so safely.

The immediate target remains
`examples/design/kicad-backed/i2c_sensor_breakout_candidate`. The latest
selected attempt now proves 11 of 12 required inter-block contacts, reports
three complete contact-graph groups and one partial contact-graph group, and
exposes local-route merge anchors. The branch executor still reports:

- a GND no-legal-two-layer-path blocker near the connector ground pad;
- an SDA no-legal-two-layer-path blocker;
- one SDA contact miss.

The current branch executor still routes branch requests primarily from
abstract ref/pin endpoints. This project must make branch routing access-aware:
it should pick physical pad/local-route/contact-graph access candidates, route
to the best legal terminal, and prove contact against the graph.

## Non-Goals

- Do not implement a production autorouter.
- Do not bypass clearance, keepout, layer, via, or board-edge rules.
- Do not claim KiCad ERC/DRC-clean output solely from internal graph proof.
- Do not mutate imported/user-authored projects.
- Do not introduce new circuit block families or component catalog records.
- Do not promote the I2C fixture unless all declared gates support promotion.

## Current Behavior

Implemented foundations:

- route-tree-managed VCC/GND/SDA/SCL nets are excluded from fallback routing;
- route-tree endpoint access summary exposes pad access and local-route anchors;
- route-tree contact graph summary exposes required/proven endpoints, graph
  group completion, and local-route merge evidence;
- same-net pads and same-net existing copper are not occupancy blockers;
- route-tree repair classifies branch/contact failures and feeds bounded retry;
- selected retry attempt improves from 10 to 11 proven endpoints.

Remaining limitations:

- route-tree branches do not yet select among multiple access candidates for a
  logical endpoint;
- local-route anchors are visible in summaries but are not first-class branch
  terminals;
- branch routing does not try alternate source/target access pairs when the
  first pair is blocked;
- branch operations can miss a resolved contact target even when nearby
  same-net graph evidence exists;
- issue evidence identifies branch blockers, but does not yet say which access
  candidates were tried and rejected.

## Required Capabilities

### 1. Branch Access Candidate Selection

For every route-tree branch endpoint, build candidate access points from:

- resolved physical pads;
- generated block-local route anchors;
- existing same-net route-tree branch copper;
- same-net contact-graph merge points;
- external anchors where already supported.

Each candidate must include:

- endpoint ID;
- role (`source_pad`, `target_pad`, `local_route_anchor`,
  `same_net_copper`, `external_anchor`);
- ref/pad when known;
- net;
- layer;
- coordinates;
- source/provenance;
- deterministic score components.

Selection must prefer:

1. already-proven local-route anchors on the same net;
2. pad access points nearest to the opposite branch endpoint;
3. candidates with legal routing layer access;
4. candidates farther from immediate other-net obstacles;
5. stable endpoint/ref/pad/layer/source ordering.

### 2. Multi-Candidate Branch Attempts

For each branch, the executor must try a bounded deterministic set of access
pairs rather than a single abstract ref/pin pair.

Rules:

- cap candidate pairs per branch to avoid search explosion;
- try higher-ranked candidate pairs first;
- stop after the first route that produces valid same-net contact graph proof;
- discard failed partial attempts;
- preserve branch indices in issues and evidence;
- record tried candidate count and selected candidate roles.

### 3. Access-Point Route Requests

The lower-level router currently accepts net endpoints as ref/pin pairs. This
project may either:

- add a route request mechanism for explicit point/layer access terminals; or
- introduce temporary synthetic route components/pads for access candidates.

The chosen approach must:

- keep generated operations KiCad-native route operations;
- keep net identity stable;
- respect all occupancy and routing rules;
- avoid leaking synthetic refs into project files;
- preserve existing ref/pin routing behavior for ordinary nets.

### 4. Endpoint Snapping And Contact Proof

Branch routes must terminate at or merge into valid same-net contact graph
nodes.

Required behavior:

- route endpoints should snap to the selected access candidate when within
  tolerance;
- branch routes may end at same-net local-route anchors or same-net branch
  copper merge points;
- contact proof must accept valid graph connectivity through generated
  same-net local routes;
- contact proof must continue rejecting wrong-net and wrong-layer contacts;
- failures must distinguish route path failure from post-route contact miss.

### 5. Structured Branch Evidence

Branch evidence must expose enough detail for AI repair decisions:

- branch index;
- start/end endpoint IDs;
- status;
- attempted access pair count;
- selected source/target roles;
- selected source/target coordinates;
- merge kind when applicable;
- operation count;
- issue count.

Branch issues should include:

- branch path;
- net;
- refs/pads where known;
- candidate roles tried;
- nearest obstacle where available;
- whether local-route anchors existed but could not be reached.

### 6. I2C Fixture Promotion Decision

After implementation, run the I2C fixture and update evidence.

Allowed outcomes:

- promote to `candidate` or `pass` only if routing, writer correctness,
  validation, and configured KiCad evidence support it;
- remain `expected_fail` only if remaining blockers are narrower and clearly
  documented.

Desired improvement:

- eliminate the SDA contact miss;
- increase branch executor routed branch count;
- reduce GND/SDA pathfinding blockers;
- reach four complete contact-graph groups if branch routing can legally do so.

## Data Model

Potential extensions:

```go
type RouteTreeBranchAccessAttempt struct {
    BranchIndex       int    `json:"branch_index"`
    SourceEndpointID  string `json:"source_endpoint_id"`
    TargetEndpointID  string `json:"target_endpoint_id"`
    SourceRole        string `json:"source_role,omitempty"`
    TargetRole        string `json:"target_role,omitempty"`
    SourceXMM         float64 `json:"source_x_mm,omitempty"`
    SourceYMM         float64 `json:"source_y_mm,omitempty"`
    TargetXMM         float64 `json:"target_x_mm,omitempty"`
    TargetYMM         float64 `json:"target_y_mm,omitempty"`
    CandidatePairRank int    `json:"candidate_pair_rank"`
    Status            string `json:"status"`
    MergeKind         string `json:"merge_kind,omitempty"`
}

type RouteTreeAccessDrivenSummary struct {
    BranchesConsidered       int `json:"branches_considered"`
    CandidatePairsConsidered int `json:"candidate_pairs_considered"`
    CandidatePairsAttempted  int `json:"candidate_pairs_attempted"`
    AccessRoutedBranches     int `json:"access_routed_branches"`
    AccessContactMisses      int `json:"access_contact_misses"`
    LocalRouteAnchorUses     int `json:"local_route_anchor_uses"`
    SameNetMergeUses         int `json:"same_net_merge_uses"`
}
```

Final names may differ if existing branch evidence structs can be extended
cleanly.

## Compatibility

Must remain stable for:

- ordinary two-endpoint routing;
- route-tree branch issue paths;
- local-route-only designs;
- LED and connector/LED KiCad-backed candidate fixtures;
- generated project transaction shape;
- writer correctness and board validation checks;
- optional KiCad CLI behavior.

Synthetic helper objects, if used, must never be serialized into KiCad project
files.

## Testing Requirements

Minimum tests:

- access candidates are ranked deterministically;
- local-route anchor is preferred over pad center when it provides proven
  same-net access;
- blocked first access pair falls back to a second legal pair;
- failed access-pair attempts emit no partial copper;
- selected branch route snaps to the selected target access point;
- wrong-net/wrong-layer contact remains rejected;
- branch evidence records candidate pair attempts and selected roles;
- I2C route-tree summaries show improvement or narrower blockers;
- LED and connector/LED fixtures do not regress.

Optional KiCad-backed tests:

- I2C fixture promotion report updates actual blocker state;
- candidate/pass promotion only occurs when KiCad-required evidence passes.

## Acceptance Gates

- `go test ./internal/designworkflow -run 'AccessDriven|RouteTree|InterBlock|I2C|ConnectorLED' -count=1`
  passes.
- `go test ./internal/routing -run 'PointAccess|SameNet|Route' -count=1`
  passes if routing internals are changed.
- `go test ./...` passes.
- I2C fixture metadata reflects actual evidence from a fresh run.
- Prism review has no unresolved high or medium findings.

