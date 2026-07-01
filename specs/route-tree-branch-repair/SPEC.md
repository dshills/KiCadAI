# Route-Tree Branch Repair Spec

Date: 2026-07-01

## Objective

Turn graph-complete route-tree branch blockers into deterministic repair
actions that can improve generated multi-endpoint inter-block routing.

The immediate target is `i2c_sensor_breakout_candidate`. The current workflow
now executes route trees for VCC/GND/SDA/SCL, excludes those managed nets from
fallback net-level routing, and reports branch-scoped path/contact blockers.
The fixture still remains `expected_fail` because VCC and SDA branches cannot
find legal paths near other-net pads and contact proof remains incomplete.

This project should make those failures actionable:

- classify each failed route-tree branch by cause;
- map failures to concrete placement or routing repair hints;
- rerun bounded placement/routing retry using those hints;
- verify whether the selected retry attempt improves route-tree completion;
- keep explicit blockers when the fixture cannot yet promote.

## Non-Goals

- Do not replace the grid router with a production autorouter.
- Do not claim KiCad DRC-clean output solely from internal route-tree repair.
- Do not mutate imported/user-authored projects.
- Do not expand circuit block or component catalog coverage.
- Do not promote the I2C fixture unless writer, connectivity, and configured
  KiCad evidence gates actually support promotion.

## Current Behavior

`RoutePlacement` now emits `inter_block_route_trees` evidence. For I2C, the
latest known run reports:

- managed nets: `GND`, `SCL`, `SDA`, `VCC`;
- planned branches: 8;
- attempted branches: 8;
- route-tree branches routed: 0;
- internal inter-block contact evidence still proves some endpoints via local
  route operations;
- blocker issues include branch-scoped VCC/SDA no-legal-path failures near
  other-net pads;
- VCC has a contact miss;
- SDA has missing emitted contact evidence for all required endpoints.

Placement retry currently consumes general routing diagnostics, but route-tree
branch blockers are not yet converted into placement adjustments with enough
specificity to spread the actual conflicting refs, create fanout room, or
change route access.

## Required Capabilities

### 1. Branch Failure Classification

Every route-tree branch failure must be classified into one of these categories:

- `blocked_by_other_net_pad`;
- `blocked_by_keepout`;
- `blocked_by_board_edge`;
- `blocked_by_existing_copper`;
- `layer_access`;
- `via_policy`;
- `search_exhausted`;
- `contact_miss`;
- `missing_contact_target`;
- `graph_split`;
- `unsupported`;
- `unknown`.

The classifier must preserve:

- route net;
- route group net;
- branch path evidence until structured branch metadata is available;
- refs and pads involved in the branch;
- nearest blocker evidence when available from structured router issues;
- issue code and severity;
- original issue path;
- suggested repair action.

### 2. Branch Repair Hints

Classified branch failures must map to repair hints that existing or new retry
logic can consume:

- `blocked_by_other_net_pad` -> increase spacing between branch refs and
  blocker refs, or move a flexible group away from the obstacle;
- `blocked_by_keepout` / `blocked_by_board_edge` -> move branch refs inward or
  away from the blocked region;
- `blocked_by_existing_copper` -> increase spacing or reroute already-managed
  nets earlier/later;
- `layer_access` / `via_policy` -> allow eligible layers, add fanout room, or
  use a through-hole transition where legal;
- `search_exhausted` -> increase spacing and reduce branch length pressure;
- `contact_miss` -> snap/rebuild endpoint route contact;
- `missing_contact_target` -> re-evaluate placement pad hydration and endpoint
  resolution;
- `graph_split` -> connect same-net components or suppress partial copper.

Hints must be conservative. If a branch cannot name enough refs or locations,
it should produce a route-scope repair hint without moving unrelated
components.

### 3. Placement Retry Integration

Bounded placement-routing retry must use route-tree branch hints in addition to
normal router diagnostics.

The retry loop must:

- collect branch repair hints from the routing stage;
- include eligible refs from branch endpoints and nearest blockers;
- respect mobility policy (`fixed`, `group_transform`, `local_rebuild`,
  `soft_preferred`, `unowned`);
- preserve hard constraints and keepouts;
- generate deterministic candidate placements;
- select the best attempt using route-tree completion evidence when available.

Route-tree completion should influence best-attempt ranking through:

- fewer blocked groups;
- fewer partial groups;
- more proven required endpoints;
- more completed branches;
- fewer contact misses;
- fewer graph components;
- lower issue count.

### 4. Routing Retry Diagnostics

The route stage must expose a compact, AI-facing diagnostic summary for
route-tree repair:

```json
{
  "route_tree_repair": {
    "branch_failures": 3,
    "repairable_failures": 2,
    "unrepairable_failures": 1,
    "hint_count": 2,
    "nets": ["SDA", "VCC"],
    "refs": ["J1", "R1", "U1"]
  }
}
```

Detailed evidence may live in typed Go structs or stage issues, but the summary
must remain stable enough for CLI and AI consumers.

### 5. I2C Fixture Evidence

The I2C fixture should move as far as deterministic repair allows.

After implementation, one of these outcomes is acceptable:

- promotion to `candidate`, if route-tree completion, writer correctness,
  board validation, and configured KiCad evidence support it;
- continued `expected_fail`, but with selected retry evidence showing why VCC
  or SDA still cannot be repaired by the current placement/router model.

The fixture must not silently degrade. It must continue to emit explicit
promotion blockers.

## Data Model

New or extended model types may include:

```go
type InterBlockBranchFailureCategory string

type InterBlockBranchRepairHint struct {
    Category        InterBlockBranchFailureCategory `json:"category"`
    NetName         string                          `json:"net_name"`
    Refs            []string                        `json:"refs,omitempty"`
    Nets            []string                        `json:"nets,omitempty"`
    RetryScope      string                          `json:"retry_scope"`
    Action          string                          `json:"action"`
    Path            string                          `json:"path,omitempty"`
}

type InterBlockRouteTreeRepairSummary struct {
    BranchFailures       int      `json:"branch_failures"`
    RepairableFailures   int      `json:"repairable_failures"`
    UnrepairableFailures int      `json:"unrepairable_failures"`
    HintCount            int      `json:"hint_count"`
    Nets                 []string `json:"nets,omitempty"`
    Refs                 []string `json:"refs,omitempty"`
}
```

The final type names may differ if existing placement retry types can be
extended cleanly.

## Algorithm

For each `RoutePlacement` run:

1. Execute route trees as today.
2. Collect branch routing evidence and branch-scoped issues.
3. Classify branch failures by issue code, route status, contact proof, graph
   evidence, and conservative message categories where structured router
   fields are not yet available.
4. Convert repairable failures into retry hints.
5. Add a route-tree repair summary to routing stage summaries.
6. Feed hints into placement-routing retry.
7. Re-run placement and routing attempts within the retry budget.
8. Compare attempts using route-tree-aware ranking.
9. Preserve the selected attempt and stop reason in existing retry summaries.
10. Update fixture metadata and roadmap based on actual evidence.

## Compatibility

Existing behavior must remain stable for:

- simple routing requests;
- local-route-only generated designs;
- LED and connector/LED KiCad-backed fixtures;
- skipped routing;
- placement-blocked routing skips;
- raw `route request`;
- existing placement retry summaries.

New summaries may be added. Existing summary field names and meanings must not
be removed or weakened.

## Acceptance Criteria

- Branch-scoped route-tree failures are classified into stable categories.
- Branch repair hints include concrete refs/nets when evidence exists.
- Placement-routing retry consumes route-tree hints.
- Selected retry attempts consider route-tree completion evidence.
- Connector/LED remains candidate-level routed evidence.
- I2C either promotes or reports narrower selected-retry blockers than the
  current VCC/SDA branch failures.
- `go test ./...` passes.
- The I2C fixture command is run and documented.
- Prism review has no unresolved high or medium findings before commit.

## Open Questions

- Should route-tree hints directly mutate placement candidate generation, or
  should they first flow through existing routing diagnostic hint conversion?
- Should route-tree failed branch copper be suppressed until the whole group is
  graph-complete?
- Should branch ordering be retried when other-net copper from earlier managed
  groups blocks later nets?
- How much route-tree evidence should be exposed in the CLI summary versus
  detailed artifacts?
