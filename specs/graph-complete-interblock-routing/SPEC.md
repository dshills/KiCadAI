# Graph-Complete Inter-Block Routing Spec

Date: 2026-07-01

## Objective

Make generated inter-block routing complete every required endpoint in a
multi-endpoint net group before promotion gates consider the net routed.

The immediate target is `i2c_sensor_breakout_candidate`. It now has strong
evidence for:

- generated pad and copper net assignment;
- block-local VCC/GND/SDA/SCL alias propagation;
- inter-block route candidates;
- deterministic multi-endpoint route groups;
- deterministic route trees;
- branch routing primitives;
- endpoint contact proof;
- group-level completion summaries.

It still remains `expected_fail` because the generated routing workflow does
not yet produce graph-complete same-net copper for all VCC/GND/SDA/SCL route
groups. The latest observed blockers are:

- VCC: no legal two-layer path near `J28ebbed8001.2`, plus missing emitted
  contact evidence for required VCC endpoints;
- SDA: no legal two-layer path near `Rbc8617b0001.1`, plus missing emitted
  contact evidence for required SDA endpoints;
- GND: partial graph completion;
- SCL: one missed required sensor contact.

This project should close that gap by making route-tree execution the primary
inter-block route path and by giving the router enough same-net and layer-access
evidence to complete small generated boards.

## Non-Goals

- Do not attempt to guarantee KiCad ERC/DRC-clean fabrication readiness for all
  generated boards.
- Do not replace the raw routing engine with a production autorouter.
- Do not mutate imported/user-authored projects.
- Do not broaden component or block libraries.
- Do not make fixture promotion depend on an installed KiCad unless the fixture
  already declares optional/required KiCad evidence.

## Current Behavior

`RoutePlacement` still routes generated inter-block nets through the normal
`routing.RouteRequestContext` path. The multi-endpoint route-tree branch router
exists, but it is not yet the authoritative workflow path for generated
inter-block route groups.

As a result:

- route-tree summaries can report planned and attempted branches;
- contact proof can name missing or missed targets;
- `routes_completed` and group completion remain graph-based;
- richer multi-endpoint nets can still stall after partial net-level route
  operations.

The next step is to execute route trees directly for generated inter-block
nets, merge their operations with existing local/anchor routes, and only count
groups complete when every required endpoint is proven in one same-net graph.

## Required Capabilities

### 1. Workflow Route-Tree Execution

`RoutePlacement` must be able to:

- build inter-block route groups after PCB realization and placement;
- resolve physical contact targets for each group;
- plan route trees for each group;
- execute route-tree branches for eligible groups;
- emit route transactions from successful branches;
- preserve existing local-route and anchor-binding operations;
- avoid routing the same inter-block nets again through the old net-level path
  unless route-tree execution is disabled or inapplicable.

The existing net-level router should remain available for simple or non-grouped
route requests.

### 2. Branch Start And Target Semantics

Branch routing must support two kinds of branch starts:

- a resolved physical endpoint target;
- previously routed, same-net copper from earlier branches in the same tree.

Same-net copper reuse must remain conservative:

- only copper emitted by successful prior branches in the current route group
  can become reusable tree evidence;
- wrong-net copper remains an obstacle;
- existing board copper from the input request is treated according to its net
  and layer;
- branch results must carry operation IDs and branch IDs where practical.

### 3. Same-Net Graph Completion

Completion must be graph-based, not operation-count-based.

A group is complete only when:

- every required endpoint definition is resolved;
- every required target has contact proof;
- all required targets belong to one same-net contact graph;
- no blocking issue is attached to that net group.

Partial groups must distinguish:

- branches planned but not attempted;
- branches attempted but unrouted;
- branches routed but not contact-proven;
- endpoints proven but split across more than one graph component.

### 4. Layer Access And Through-Hole Semantics

The I2C fixture includes headers, pull-ups, decoupling, and sensor pads. The
route-tree executor must handle layer access conservatively:

- route requests should use the placed pad layer access already available in
  `routingadapters.RequestFromPlacement`;
- through-hole pads should allow same-net layer transitions without forcing an
  avoidable generated via when the pad itself provides copper access;
- SMD pads should stay constrained to their declared copper layers;
- branch route diagnostics must name layer-access blockers when transition
  policy prevents completion.

### 5. Search And Obstacle Diagnostics

Route-tree execution must surface enough diagnostic information to guide
placement retry and repair:

- net name;
- group ID/net;
- branch index;
- start/end endpoint IDs;
- nearest blocking obstacle kind/source where available;
- route status;
- contact proof status;
- suggested repair action.

The current VCC/SDA failures near other-net pads should remain visible, but the
diagnostic should be associated with the failed route-tree branch.

### 6. Integration With Retry And Promotion

Placement-routing retry should be able to use route-tree failures as routing
pressure:

- legal-path failures should produce spacing/distance/fanout hints when refs
  are known;
- graph-split/contact-miss failures should produce route/contact repair hints;
- retry summary should continue to report selected attempt evidence;
- promotion gates should use group completion semantics already exposed in
  `inter_block_routing`.

### 7. Backward Compatibility

Existing behavior must remain stable for:

- raw `route request`;
- local-route-only designs;
- LED design fixtures;
- connector/LED inter-block fixture;
- skipped routing;
- placement-blocked routing skips;
- optional KiCad-backed fixture metadata validation.

Existing summary fields must remain present. New fields may be added, but
existing JSON names and meanings should not be removed or weakened.

## Data Model

The existing types are the foundation:

- `InterBlockRouteGroup`;
- `InterBlockRouteTree`;
- `InterBlockBranchRoutingResult`;
- `InterBlockContactEvidence`;
- `InterBlockRouteCompletionSummary`.

Additional evidence may be added if needed:

```go
type InterBlockRouteTreeExecutionSummary struct {
    GroupsPlanned      int
    GroupsAttempted    int
    GroupsComplete     int
    GroupsPartial      int
    GroupsBlocked      int
    BranchesPlanned    int
    BranchesAttempted  int
    BranchesRouted     int
    BranchesBlocked    int
    ContactMisses      int
    GraphSplits        int
    IssueCount         int
}
```

If added, it should appear under routing stage summary as
`inter_block_route_trees` or be folded into `inter_block_routing` only if the
fields are stable and non-duplicative.

## Algorithm

For generated workflow routing:

1. Build local route operations and anchor route operations as today.
2. Build inter-block route candidates.
3. Build contact targets from placed pad evidence.
4. Build route groups and route trees.
5. Exclude route-tree-managed nets from the normal net-level routing request.
6. Route remaining non-managed nets through the existing router.
7. Execute route-tree branches for managed inter-block groups:
   - initialize same-net existing copper from request existing copper plus
     successful prior branch copper for the same group;
   - route one branch request at a time;
   - convert branch route results to transaction operations;
   - update same-net copper and branch evidence after each successful branch;
   - stop or continue according to branch failure policy.
8. Merge local, anchor, normal-route, and branch-route operations.
9. Snap inter-block endpoints where needed.
10. Rebuild contact evidence from merged operations.
11. Summarize graph completion and issue paths.
12. Feed diagnostics into placement retry and promotion gates.

## Failure Policy

Default policy should continue after a failed branch when doing so can produce
additional actionable evidence for other branches or other nets. It may stop a
single group early when:

- required endpoint definitions are unresolved;
- the root target is missing;
- context is canceled;
- branch execution would reuse unproven/wrong-net copper.

The route stage should remain blocked when any required group is incomplete.

## Acceptance Criteria

- `RoutePlacement` uses route-tree execution for generated inter-block groups.
- Connector/LED remains graph-complete.
- I2C fixture no longer reports stale generic multi-endpoint blockers.
- I2C blocker either:
  - moves to candidate with complete internal route evidence, or
  - narrows to concrete branch-level legal-path/contact blockers that are
    strictly more specific than the current VCC/SDA/GND/SCL group summary.
- `inter_block_routing` remains backward-compatible and includes group/tree
  completion evidence.
- Placement retry summaries continue to work and report selected attempts.
- `go test ./...` passes.
- Prism review has no unresolved high or medium findings before commit.

## Open Questions

- Should through-hole pad layer transitions be modeled in the raw router, the
  placement adapter, or the route-tree executor?
- Should route-tree branch results be written as separate route operations per
  branch, or coalesced per net after graph completion?
- Should incomplete groups keep all successful branch operations, or should a
  failed group suppress partial copper to avoid misleading generated boards?
- Should route-tree execution become default immediately, or be guarded by an
  internal option until the I2C fixture stabilizes?

