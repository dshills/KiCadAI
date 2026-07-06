# Amplifier VCC Route Completion Spec

Date: 2026-07-06

## Objective

Complete the remaining VCC route-tree/contact-graph blocker in the protected
Class AB headphone amplifier KiCad-backed fixture so it can advance from
`routing` to `project_write`, writer correctness, structural validation, and
eventually real KiCad ERC/DRC evidence.

Immediate target:

```text
examples/design/kicad-backed/class_ab_headphone_protected.json
```

Current state:

- schematic electrical validation passes after amplifier label cleanup;
- PCB realization and placement pass;
- routing is enabled and no longer skipped by fixture policy;
- all 20 block-local route operations bind to physical same-net pad anchors;
- route connectivity resolves 40 of 40 local route endpoints with no net
  mismatches;
- six required inter-block nets are graph-complete;
- VCC remains partial with 4 of 5 required endpoints proven;
- required VCC endpoint `output.3` is not in the same proven contact graph;
- project write, writer correctness, structural validation, and KiCad checks are
  skipped with reason `routing did not complete`.

The goal is to either prove the final VCC endpoint through legal same-net route
geometry, or narrow the blocker into a deterministic, actionable placement or
routing repair that can be applied automatically.

## Non-Goals

- Do not relax clearance, board-edge, keepout, via, net-class, same-net contact,
  writer-correctness, or KiCad evidence rules.
- Do not mark the amplifier fixture as `candidate` or `pass` unless the
  promotion gates actually support that readiness.
- Do not hide VCC route failures behind allowlists or fixture-specific skips.
- Do not broaden this work into speaker-load, bridge, high-voltage, or
  power-amplifier support.
- Do not implement a production autorouter.
- Do not change unrelated I2C route-tree behavior except through shared routing
  improvements that remain covered by existing tests.

## Current Evidence Boundary

The protected amplifier fixture currently reports:

```text
required_net_classification.required_inter_block = 7
required_net_classification.complete = 6
required_net_classification.partial = 1
required_net_classification.missing_endpoints = 1
route_tree_contact_graph.required_endpoints = 24
route_tree_contact_graph.proven_endpoints = 23
route_tree_contact_graph.partial_groups = 1
route_tree_repair.nets = ["VCC"]
```

The blocking VCC evidence includes:

- `ROUTE_GRAPH_INCOMPLETE` at
  `design.inter_block_contact.nets[5].endpoints[1].segment`;
- `VALIDATION_FAILED` at
  `design.inter_block_route_groups["VCC"].branches[1].nets.VCC`;
- missing endpoint ID `output.3`;
- repair refs including `QCCDE149E001`, `__KICADAI_RT_SRC_7`, and
  `__KICADAI_RT_DST_7`.

This boundary is important: the next implementation must not regress to broad
schematic, placement, endpoint-binding, or fixed-net skip blockers. It must keep
the first blocker concentrated on the final VCC contact graph unless it fully
solves that blocker.

## Required Capabilities

### 1. Amplifier VCC Failure Snapshot

The workflow must preserve the current VCC failure shape in a deterministic
test before attempting route changes.

Required assertions:

- routing runs and blocks;
- `skip_routing` is false;
- local route connectivity remains clean;
- required-net classification reports six complete nets and one partial VCC net;
- missing endpoint IDs include only `output.3`;
- route-tree repair hints name VCC only;
- project write and downstream gates remain skipped only because routing did not
  complete;
- promotion `route_completion` fails while `writer_correctness`,
  `connectivity`, and `kicad_checks` stay skipped.

### 2. VCC Endpoint and Access Traceability

The implementation must make it easy to determine why `output.3` is not proven.

Required trace data:

- exact component/ref/pad represented by `output.3`;
- selected footprint pad coordinates and layer;
- source/target access candidates for the failed VCC branch;
- whether each candidate is a pad, local-route anchor, same-net copper, or
  synthetic access point;
- branch index and selected access pair rank;
- route status and operation count for each attempted VCC pair;
- primary blocker code/message and nearest obstacle metadata;
- same-net contact graph component membership for `output.3` and the emitted
  VCC route copper.

Trace data should live in existing routing stage summaries or compact new
summaries, not only in verbose CLI output.

### 3. Legal Same-Net VCC Completion

If a legal route exists, route-tree execution must connect `output.3` to the VCC
contact graph.

Allowed strategies:

- improve candidate ranking so the router tries a legal access pair earlier;
- route from a pad to the nearest legal same-net local-route anchor when that
  anchor is proven to merge into the required graph;
- split a hard branch into deterministic sub-branches through legal same-net
  copper when contact proof can verify the merge;
- use layer/via policy already allowed by the request and board rules;
- feed targeted placement retry hints when spacing, fanout, or boundary pressure
  prevents a legal route.

Disallowed strategies:

- counting bounding-box overlap as contact;
- snapping route endpoints to the wrong pad or wrong net;
- ignoring fixed copper obstacles;
- changing net names to avoid the missing endpoint;
- allowing partial VCC to pass as complete.

### 4. Fail-Closed Repair Classification

If VCC cannot legally route with the current placement, the workflow must report
a compact repair classification that an AI caller can act on.

Required repair categories:

- `increase_spacing`: nearby component or local-route congestion blocks VCC;
- `move_component`: a specific ref, likely `Qccde149e001` or its neighbor,
  must move to create VCC access;
- `allow_layer_or_via`: routing fails because available layer/via policy is too
  constrained;
- `relax_rule`: routing fails because a named design rule blocks an otherwise
  sensible path;
- `manual_review`: the route is electrically/thermally ambiguous and should not
  be auto-fixed.

Repair evidence must include affected nets, refs, candidate pair, and the
original blocking issue code.

### 5. Promotion Handoff

When VCC completes:

- `routing` should become `ok` or non-blocking warning;
- `project_write` should run;
- writer correctness should run;
- structural validation should run;
- `kicad_checks` should either run when KiCad CLI is configured or remain
  explicitly skipped due to missing/disabled KiCad CLI, not because routing
  failed;
- promotion gates should move the first blocker from `route_completion` to the
  next real evidence stage.

When VCC does not complete:

- `route_completion` remains failed;
- downstream gates remain skipped because routing did not complete;
- the first blocker stays VCC-specific and repairable when possible.

## Acceptance Criteria

1. The protected amplifier fixture no longer reports stale routing-skip,
   placement, endpoint-binding, schematic-label, project-write, or generic
   validation blockers before the VCC route blocker.
2. VCC route-tree diagnostics identify `output.3`, branch evidence, access
   roles, issue code, and repair category.
3. If a legal path exists, VCC reaches 5 of 5 proven endpoints and all seven
   required inter-block nets are graph-complete.
4. If no legal path exists, the fixture remains `expected_fail` with a precise
   VCC repair hint and no false promotion.
5. Existing I2C route-tree candidate/pass behavior does not regress.
6. Full `go test ./...` passes with the repository-local Go build cache.

## Risks

- Improving amplifier VCC routing may change shared route-tree access ranking
  and affect I2C fixtures.
- A local path fix may only work for current placement and fail when placement
  retry adjusts component coordinates.
- Completing VCC could reveal the next blocker in writer correctness,
  structural validation, or real KiCad ERC/DRC.
- Overfitting to `output.3` could hide a general route-tree endpoint proof issue
  if the same pattern appears on other amplifier or power nets.

## Deliverables

- deterministic amplifier VCC route snapshot tests;
- route-tree branch/contact diagnostics for the missing VCC endpoint;
- routing or placement-repair implementation that completes VCC when legal or
  reports precise repair evidence when not;
- promotion handoff tests for downstream stages;
- README, roadmap, and fixture metadata updates after the new status is known.
