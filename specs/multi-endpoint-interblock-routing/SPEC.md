# Multi-Endpoint Inter-Block Routing Specification

Date: 2026-07-01

## Summary

Generated design-level PCB routing must support nets with more than two
physical endpoints. The current inter-block routing foundation can discover
route candidates, emit route operations, bind block-local routes to physical
same-net pads, and prove endpoint-contact evidence. It still struggles when a
logical net spans a sensor pin, connector pin, pull-up resistor, decoupling
capacitor, or other branch endpoint.

The immediate target is `i2c_sensor_breakout_candidate`. That fixture now
reaches routing with clean block-local route aliasing and local contact proof,
but it remains `expected_fail` because VCC/SDA/SCL multi-endpoint route groups
are only partially routed or have missing contact targets.

This project makes multi-endpoint generated nets explicit, deterministic, and
graph-proven. A net is complete only when every required endpoint is connected
to the same same-net copper graph, not merely when one pairwise segment is
emitted.

## Problem

The router currently handles many simple two-endpoint cases and can report
physical endpoint-contact evidence. Richer generated blocks naturally create
multi-terminal nets:

- VCC connects the sensor, connector, and decoupling capacitor.
- GND connects the sensor, connector, and decoupling capacitor.
- SDA connects the sensor, connector, and SDA pull-up resistor.
- SCL connects the sensor, connector, and SCL pull-up resistor.

Treating these as independent point-to-point routes is not enough:

- route ordering can block later branches with earlier copper;
- a route can connect two endpoints while leaving a third pad disconnected;
- duplicate pairwise route attempts can emit unnecessary or conflicting
  geometry;
- route completion counts can be misleading unless they evaluate the full
  endpoint group;
- retry cannot make good placement decisions without knowing which endpoint
  group and branch failed.

The workflow must route or explicitly block the whole required endpoint group.

## Goals

- Build a deterministic multi-endpoint net model for generated inter-block
  routing.
- Route all required endpoints for a logical net using a graph-based strategy.
- Prefer compact trunk-and-branch or minimum-spanning-tree style routing over
  unrelated pairwise routes.
- Reuse existing routing search, pad/contact targets, contact proof, same-net
  graph validation, placement retry, diagnostics, and writer pipelines.
- Count a multi-endpoint net complete only when all required endpoints belong
  to one same-net connected component.
- Produce per-net branch evidence for attempted, connected, partial, blocked,
  and skipped branches.
- Narrow `i2c_sensor_breakout_candidate` from generic multi-endpoint blockers
  to either candidate-level internal routing evidence or exact remaining DRC,
  placement, or KiCad evidence blockers.
- Keep default tests KiCad-independent.

## Non-Goals

- Do not build a dense production autorouter.
- Do not require zones or copper pours to satisfy power/ground routing in this
  phase.
- Do not promote the I2C fixture to `pass` without optional KiCad evidence.
- Do not weaken board validation, writer correctness, route-contact proof, or
  optional KiCad ERC/DRC gates.
- Do not route imported user-authored projects.
- Do not implement advanced fanout, differential-pair, impedance, or
  length-matching routing as part of this project.
- Do not solve all amplifier or MCU layouts unless they share the same
  multi-endpoint routing primitive.

## Current Foundations

This project must extend existing code rather than creating a parallel routing
path:

- generated `design create` workflow;
- request connection aliases and block composition;
- PCB fragment realization and block-local route evidence;
- generated placement and placement retry;
- route endpoint resolver and physical contact targets;
- generated inter-block routing candidates;
- routing engine search and route operation emission;
- route-contact proof and same-net graph completion evidence;
- board validation and writer correctness;
- KiCad-backed design fixture metadata and promotion reports;
- repair diagnostics and retry hint mapping.

## Required Model

### Multi-Endpoint Net Group

A multi-endpoint net group represents one canonical net and all physical
targets that must be connected.

Each group must include:

- canonical net name;
- net code when assigned;
- net role and net class evidence where available;
- source request connections and aliases;
- all required endpoint targets;
- optional endpoint targets that may improve routing but do not gate
  completion;
- block instance, block ID, ref, and pad identity for each target when known;
- target coordinate, layer, contact tolerance, and geometry source;
- routing policy for vias, layers, width, clearance, partial routing, and
  global-net behavior;
- deterministic endpoint ordering used for route planning;
- evidence of route tree construction and branch attempts.

The group must preserve alias provenance. Multiple request connections that
collapse into one canonical net should create one group, not multiple
competing pairwise nets.

### Required And Optional Endpoints

Required endpoints gate completion. Optional endpoints are allowed as access
points or intermediate anchors but must not cause a route to fail unless a
request or block contract marks them required.

Required endpoint sources include:

- physical pads for all connected block ports in the request;
- block-owned pull-up, decoupling, bias, protection, or load components that
  are part of the generated same-net circuit contract;
- explicit board/interface anchors declared as required.

Optional endpoint sources include:

- validated block-local access points that duplicate a pad endpoint;
- same-net copper endpoints that can shorten a route;
- future zone access points when zone-sufficient proof exists.

### Route Tree

The route planner must produce a route tree for each multi-endpoint group.

The first implementation should support:

- deterministic root selection;
- nearest-neighbor or minimum-spanning-tree branch ordering;
- reuse of already emitted same-net copper as valid branch start points when
  contact proof exists;
- branch-level route requests emitted through the existing routing engine;
- branch evidence tied back to the net group and target identities.

The tree does not need to be globally optimal. It must be deterministic,
explainable, and validation-backed.

### Root Selection

Root selection must be deterministic and stable:

1. Prefer connector or explicit board-interface endpoints for external nets.
2. Prefer regulator/source endpoints for supply nets when known.
3. Prefer the lowest-cost central endpoint by Manhattan distance when there is
   no semantic source.
4. Break ties by stable ref/pad identity.

The chosen root must be included in route evidence.

### Branch Completion

A branch is complete when:

- it emits route operations or reuses existing same-net copper;
- its start and end contact targets are proven;
- no rule, clearance, layer, or net mismatch issue blocks it;
- the same-net graph joins the branch target to the route tree component.

A net group is complete when all required endpoints are in the same same-net
connected component. Pairwise branch success alone is not sufficient if graph
validation cannot prove the final component.

### Same-Net Copper Reuse

Same-net copper may be reused as a branch start or target only when:

- it has the same canonical net;
- it is on a reachable layer;
- contact proof or graph evidence already validates it;
- using it does not cross keepouts or other-net clearance constraints;
- the route operation provenance remains traceable.

Other-net copper remains an obstacle. Same-net visual proximity without contact
proof must not satisfy completion.

### Power And Ground Policy

Power and ground nets may be high-fanout. This phase should route small
generated power/ground groups conservatively, but it must not pretend to have
plane or zone connectivity.

Allowed behavior:

- attempt trace-tree routing for small VCC/GND groups;
- report explicit blockers when trace routing cannot satisfy the group;
- carry net-class warnings for missing power/ground class policy;
- keep zone-sufficient proof disabled unless a later project implements it.

Disallowed behavior:

- treating all same-named power symbols as physically connected without copper;
- counting zones as connected without explicit zone proof;
- suppressing disconnected pad findings for power or ground.

## Required Workflow Behavior

### Candidate Building

The workflow must group inter-block route candidates by canonical net before
route search. For each net:

- resolve all required endpoint targets;
- deduplicate duplicate pad/access targets;
- classify unresolved targets as blocking or warning according to endpoint
  requirement;
- emit a stable summary of endpoint count, target count, route tree count, and
  unresolved count.

### Route Planning

The planner must:

- choose a deterministic root;
- compute branch order;
- route each branch from the existing same-net tree to the next target;
- retry branch start selection from validated same-net copper when direct
  root-to-target routing fails;
- preserve emitted route operation IDs and target identities;
- stop or continue according to the active partial-routing policy.

### Validation And Evidence

Routing summaries must expose:

- multi-endpoint nets considered;
- net groups attempted;
- endpoint targets required and proven;
- branch routes attempted, connected, partial, and blocked;
- same-net graph component count per net;
- required endpoints missing from the final component;
- route operations emitted;
- route-contact issue count;
- DRC/rule blocker count where available.

Existing `inter_block_routing` and `inter_block_contacts` summaries may be
extended, but should remain backward-compatible for current tests and CLI
consumers where possible.

### Error Codes And Diagnostics

Existing route-contact and graph codes remain valid. New or more specific
diagnostic paths should identify:

- net group;
- endpoint target;
- branch index;
- route operation ID when available;
- blocker category.

Suggested path shape:

```text
design.inter_block_route_groups.<net>.branches[<n>]
design.inter_block_route_groups.<net>.endpoints[<n>]
```

Diagnostics should map to repair hints:

- `increase_spacing`;
- `move_component`;
- `route_from_access_point`;
- `snap_to_target`;
- `allow_via`;
- `assign_net_class`;
- `manual_route_required`.

## Fixture Targets

### I2C Sensor Breakout

`examples/design/kicad-backed/i2c_sensor_breakout_candidate.json` is the
primary target.

Expected improvement:

- VCC/GND/SDA/SCL route groups are built from all required endpoints.
- Local block route net aliases remain clean.
- Endpoint-contact proof remains clean for block-local routes.
- Inter-block route summaries name branch-level blockers or prove full
  same-net graph connectivity.
- If internal routing completes, metadata may move from `expected_fail` to
  `candidate` only if writer correctness, validation, artifact checks, and
  configured KiCad evidence policy also allow it.

### Connector LED

The existing connector/LED candidate fixture must not regress. It should remain
a small two-endpoint or low-fanout route proof case.

### Future Fixtures

The model should be reusable by:

- MCU plus I2C sensor plus connector;
- regulator feeding downstream loads;
- USB-C power feeding regulator/protection/load blocks;
- amplifier rails and output/load networks.

## Acceptance Criteria

- Multi-endpoint route groups are explicit in workflow evidence.
- I2C VCC/GND/SDA/SCL groups include all required sensor, connector, pull-up,
  and decoupling endpoints.
- Route tree planning is deterministic for identical requests.
- Branch route attempts reuse existing same-net copper only with contact or
  graph proof.
- A net group is counted complete only when every required endpoint is in one
  same-net connected component.
- Partial routing reports exact missing endpoints and branch blockers.
- Board validation and writer correctness remain strict.
- Default `go test ./...` remains KiCad-independent.
- Optional KiCad-backed fixture behavior remains metadata-driven.

## Test Requirements

Add KiCad-independent tests for:

- grouping a three-endpoint net into one route group;
- deduplicating duplicate endpoint/access targets;
- root selection tie-breaking;
- deterministic branch order;
- branch routing that connects three pads through a same-net tree;
- branch routing that partially connects two of three pads and reports the
  missing endpoint;
- same-net copper reuse as a branch start;
- wrong-net copper not joining the graph;
- I2C fixture route summary shape and expected current/promotion status.

Optional KiCad-backed tests may be added only behind existing environment
gates.

## Documentation Requirements

Update:

- `specs/ROADMAP.md` with achieved status and remaining blocker;
- `examples/design/kicad-backed/README.md` if fixture readiness or blockers
  change;
- `README.md` or focused routing docs only if user-facing commands or output
  fields change.

## Risks

- A naive route-tree strategy may overfit I2C and fail on other topologies.
- Branch routing may make later branches harder if same-net copper is not
  chosen carefully.
- Power/ground nets can become too complex without zone support.
- More detailed route summaries may break brittle tests if added carelessly.
- Promoting fixtures prematurely would hide real DRC or KiCad evidence gaps.

## Open Questions

- Should small power/ground groups use trace-tree routing by default, or should
  they require explicit net class policy before route attempts?
- What fanout threshold should trigger a "manual route or zone required"
  blocker?
- Should the planner insert same-net junction stubs, or only route directly to
  pads/access points in this phase?
- Should branch retry be integrated immediately with placement retry, or first
  exposed as route evidence only?
