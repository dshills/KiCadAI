# Entry Anchor Board Binding Specification

Date: 2026-06-24

## Summary

KiCadAI block realization can now declare entry anchors for protection and
power-path circuits, and can emit block-local routes to those anchors. That is
necessary but not sufficient for autonomous PCB generation. An entry anchor is
currently evidence of where a signal or power path should enter a block; it is
not yet guaranteed to be bound to a real connector pad, board-edge feature, test
point, or imported mechanical constraint.

This specification defines board-level binding for block entry anchors. The
goal is to turn anchor intent into physical PCB endpoints, validate that the
endpoint and anchor agree on net and role, and expose deterministic evidence to
the CLI and future AI workflow layers.

## Problem

The current block realization model can describe routes such as:

- connector-side signal entry into an ESD block;
- raw input power entry into a reverse-polarity block;
- protected output power path from a protection block into the rest of the
  board.

However, those routes can terminate at synthetic anchor identifiers. The PCB may
look structurally plausible, but the anchor does not prove that copper reaches a
real pad or board interface. This leaves several gaps:

- an ESD input anchor might not be connected to the USB or external connector
  pad it is intended to protect;
- a reverse-polarity input anchor might be route evidence only, without a
  physical power-entry endpoint;
- net names can drift between connector pads, block ports, and anchor-local
  route definitions;
- placement can satisfy local proximity rules while still leaving board-level
  entry geometry unproven;
- AI-facing workflow output cannot distinguish "anchor declared" from "anchor
  physically bound and routed."

## Goals

- Add a board-level binding model that associates block entry anchors with
  physical endpoints.
- Support deterministic binding to:
  - generated or existing footprint pads;
  - board-edge interface points represented by explicit coordinates;
  - imported mechanical constraints when a future importer provides them.
- Preserve block-local anchor evidence while adding separate board-level
  binding evidence.
- Validate that bound anchors and physical endpoints share the expected net,
  port role, and known coordinates.
- Generate or request board-level route segments between physical endpoints and
  anchor coordinates where enough geometry is available.
- Surface binding evidence in `design create` output so AI callers can explain
  which block interfaces are physically connected.
- Fail or warn deterministically when anchors are missing, ambiguous, or
  physically unbound according to policy.

## Non-Goals

- Do not implement arbitrary mechanical CAD import in this project.
- Do not replace the block-local route model. Anchor binding complements that
  model.
- Do not require every optional anchor to be bound; binding policy must be
  explicit per anchor or block family.
- Do not claim fabrication readiness from binding alone. Writer correctness,
  board validation, and configured KiCad ERC/DRC checks remain separate gates.
- Do not mutate imported KiCad projects until preservation-safe imported-project
  mutation is implemented.

## Terminology

- **Entry Anchor**: A named block-local PCB coordinate where an external signal,
  power path, or protected path should enter or leave a block.
- **Physical Endpoint**: A concrete PCB object that can electrically connect to
  copper, usually a footprint pad. Future endpoint types may include board-edge
  contact geometry or imported mechanical anchor points.
- **Anchor Binding**: The board-level association between one entry anchor and
  one physical endpoint, with net, role, coordinate, and validation evidence.
- **Binding Policy**: Rules that define whether a binding is required,
  optional, advisory, or unsupported for a given anchor.
- **Binding Evidence**: Workflow output that records binding status, target
  identity, net agreement, coordinate source, route evidence, and issues.

## Current Capabilities To Reuse

- Circuit blocks can emit PCB realization metadata, placement groups, local
  route definitions, route-width constraints, keepouts, proximity constraints,
  and entry-anchor evidence.
- Placement already emits component location evidence and supports block-derived
  placement intent.
- Footprint hydration can expose pad geometry for resolved footprints.
- Routing can produce net-aware route operations from known points.
- Connectivity-first board validation can detect pad-net mismatches, unrouted
  nets, route completion issues, missing outlines, zone issues, and DRC evidence
  hooks.
- `design create` already emits block planning, placement, routing, validation,
  repair, and fabrication-readiness evidence.

## Required Model Additions

### Physical Endpoint

Add a model that can represent a routable board interface endpoint:

```text
PhysicalEndpoint
  id
  kind: footprint_pad | board_edge_point | imported_mechanical_point
  ref
  pad
  net_name
  layers
  roles
  point
  source
  confidence
  issues
```

For phase-one implementation, `footprint_pad` is the only required endpoint
kind. `board_edge_point` and `imported_mechanical_point` can be modeled without
full generation support so the API does not need to be redesigned later.

### Anchor Binding

Add a model that records the selected binding:

```text
AnchorBinding
  id
  block_instance_id
  anchor_id
  anchor_port
  anchor_net_name
  anchor_point
  endpoint_id
  endpoint_kind
  endpoint_ref
  endpoint_pad
  endpoint_net_name
  endpoint_layers
  endpoint_point
  status: bound | unbound | ambiguous | invalid | unsupported
  required: true | false
  route_status: routed | route_requested | not_routable | skipped
  distance_mm
  issue_ids
```

All points in anchor binding evidence are board-absolute coordinates in
millimeters. Component-relative coordinates must be converted before entering
the binding model. Endpoint layers are a set, not a scalar: SMD pads normally
advertise one copper layer, through-hole pads advertise all connected copper
layers, and future board-edge endpoints advertise the copper layers they expose.

The binding must be separate from the existing block-local route evidence. A
route to `@anchor:<id>` is not proof that the anchor is bound to a connector pad.

Endpoint confidence is an evidence score, not an identity field. It starts from
the endpoint source quality: explicit generated pad metadata is high confidence,
hydrated footprint geometry with net metadata is medium-to-high confidence, and
inferred or imported metadata is lower confidence. Resolution uses confidence
only after hard net compatibility and role compatibility; it must not override a
better explicit connection or interface-group match.

### Binding Issue

Binding issues should be normalized so the repair loop and AI caller can reason
about them:

```text
AnchorBindingIssue
  id
  severity: info | warning | blocking
  category:
    missing_anchor
    missing_endpoint
    ambiguous_endpoint
    missing_endpoint_point
    net_mismatch
    role_mismatch
    route_missing
    unsupported_endpoint_kind
  block_instance_id
  anchor_id
  endpoint_id
  message
  repair_hint
```

## Binding Inputs

Binding should use available deterministic evidence in this order:

1. Explicit connection intent from the design request or block composition.
2. Block exported ports and required anchor metadata.
3. Assigned schematic nets transferred to PCB nets.
4. Placed footprint pad summaries from hydration.
5. Endpoint roles from component metadata, block roles, or connector pin roles.
6. Interface or pin-group affinity, preferring endpoints on the same connector,
   external interface, or declared board-interface group before generic
   same-net candidates such as `GND`.
7. Geometric proximity when several otherwise valid candidates remain.

The engine must not choose a candidate that fails hard net compatibility. If
multiple compatible candidates remain and no deterministic rule breaks the tie,
the binding is `ambiguous`.

Multiple same-net candidates on the same physical interface group are not
automatically ambiguous. For example, several GND or VBUS pads on the same
connector may be equivalent return or power-entry points. When candidates share
the same component/interface group, net, role family, and layer compatibility,
the resolver may select the nearest candidate deterministically and record the
equivalent candidates in evidence with an informational issue. Current path,
thermal, or return-path constraints can still force ambiguity when declared by
the block or board-interface rule.

Net compatibility must use a KiCad-aware canonical net comparison helper. The
helper must trim surrounding whitespace, preserve case-sensitive KiCad net
identity, understand hierarchical path forms where schematic transfer can prove
their canonical net code, and use only explicit aliases from schematic transfer,
design request constraints, or project net metadata. The helper must not invent
a canonical case map for well-known nets; `GND` and `gnd` remain different
unless transferred project data proves they are the same net code or the request
declares an alias. Common power names such as `VCC`, `+5V`, and `VIN` are not
implicitly equivalent, but the engine must support declared aliases so power
paths do not fail when the schematic intentionally maps them to one physical
net. For known all-caps power net families, case-only matches may produce a
lower-confidence suggested binding with a warning, but fabrication/readiness
gates still require an explicit alias or matching KiCad net code before treating
the binding as clean.
An opt-in relaxed matching policy may accept case-only matches for common
power/ground nets in early intent-planning modes, but it must emit a warning and
must not satisfy fabrication-candidate readiness without explicit alias evidence.

For user-request mismatch diagnostics, the helper may emit soft alias
suggestions for common case variants such as `GND`/`gnd` or `VCC`/`vcc`. Soft
suggestions are repair hints only; they are not accepted as clean bindings until
the request, schematic transfer, or project metadata declares the alias.

Proximity can break ties only within a bounded search radius. The default
maximum proximity-only binding distance is configurable in board composition
settings and starts at 10 mm when no project or block-specific value is
provided. Blocks and board interfaces can declare tighter constraints.
Candidates outside the active radius remain unbound or ambiguous rather than
being selected only because they are the nearest available pad.
Distance is 2D Euclidean board distance in millimeters. Candidate scoring should
apply a deterministic layer-transition penalty before proximity tie-breaking so
same-layer candidates win over otherwise equivalent cross-layer candidates.
The default layer-transition penalty should be 2 mm to 3 mm equivalent distance,
so a very local via transition can still beat a much longer same-layer route.
The penalty is configurable through board stackup/routing settings and should be
derived from via geometry or minimum trace width when those settings are
available.

Role compatibility uses conservative exact and family matches:

- signal anchors can bind to signal, bidirectional, connector-signal, and
  passive endpoint roles;
- power-input anchors can bind to power-source, connector-power, power-input,
  and passive endpoint roles when the net matches;
- power-output anchors can bind to power-input, load-power, connector-power,
  and passive endpoint roles when the net matches;
- ground anchors can bind only to ground or passive endpoint roles on a ground
  net;
- unknown roles can bind only when net and explicit connection intent match,
  and should emit an informational role-confidence issue.

## Binding Policies

Each anchor must resolve to one of these policies:

- `required`: unbound, ambiguous, invalid, or unrouted status is blocking.
- `optional`: missing binding is a warning when the block is otherwise valid.
- `advisory`: evidence is reported but does not affect workflow success.
- `unsupported`: the system acknowledges the anchor kind but cannot bind it yet.

Default policies:

- ESD connector-side entry anchors are `required` when the protected signal is
  declared as an external interface, regardless of whether the physical
  connector is generated in the same pass or supplied by an imported endpoint.
- Reverse-polarity raw input power anchors are `required` when the design has a
  declared external power input.
- Protected output anchors are `required` when the downstream load net is
  generated in the same design.
- Anchors on unsupported endpoint kinds are `unsupported` and should not be
  silently treated as clean.

## Routing Behavior

When both anchor and endpoint coordinates are known, board-level routing should
add a short route request or route operation from the physical endpoint to the
anchor point. This route is distinct from block-local routes between the anchor
and protected components.

The generated route must:

- use the anchor net name after confirming it matches the endpoint net;
- use the configured route width policy with this precedence:
  block-specific route constraint may add a stronger requirement, but explicit
  net-class width acts as the minimum floor, and the router default is used only
  when neither block-specific nor net-class constraints exist. The generated
  width is the larger of block-specific width and net-class minimum when either
  exists; otherwise it is the router default. The binding should emit an
  informational issue when a block-specific narrower width is raised to satisfy
  the net-class floor;
- report a width-to-pad incompatibility issue when the selected width exceeds
  the physical landing area or escape capability of the endpoint instead of
  silently producing likely DRC-violating copper;
- validate layer context and request a simple via transition where supported;
  report `requires_layer_transition` separately from mechanically impossible or
  unsupported transitions;
- preserve placement mobility policy and local-route mobility constraints;
- be reported as `routed` only when the route operation is actually present in
  the generated board transaction;
- be reported as `route_requested` when a downstream router request is created
  but not yet materialized;
- be reported as `not_routable` when coordinates or supported route policy are
  missing, or when layer/via compatibility cannot be satisfied;
- be reported as `route_requested` with a layer-transition issue when a via is
  needed and a downstream router can satisfy it.

## Workflow Output

`design create` should expose anchor binding summaries with enough detail for AI
callers to reason about generated boards:

```json
{
  "anchor_bindings": {
    "total": 3,
    "bound": 2,
    "blocking_issues": 1,
    "bindings": [
      {
        "block_instance_id": "esd_usb_1",
        "anchor_id": "usb_d_plus_entry",
        "anchor_port": "D+",
        "anchor_net_name": "USB_D+",
        "endpoint_id": "footprint_pad:J1:A6:9f3a12c0",
        "endpoint_ref": "J1",
        "endpoint_pad": "A6",
        "endpoint_net_name": "USB_D+",
        "status": "bound",
        "route_status": "routed",
        "distance_mm": 3.2
      }
    ],
    "issues": []
  }
}
```

The output should make clear when a generated board is parseable but not yet
electrically proven at its external interfaces.

## Validation Requirements

The binding validator must check:

- every required anchor exists;
- every required anchor has exactly one selected endpoint;
- selected endpoints have known coordinates;
- anchor and endpoint nets match after normalization;
- anchor and endpoint layer context is compatible or an explicit layer
  transition is planned;
- endpoint role is compatible with the anchor port role where roles are known;
- route evidence exists when routing is required;
- route evidence uses the same net as the binding;
- unsupported endpoint types are explicit findings, not silent passes.

## Tests And Fixtures

Required tests:

- a generated ESD protection board with connector pads binds signal entry
  anchors to the connector pads;
- a reverse-polarity power-entry board binds raw power input anchors to the
  power connector pads;
- a missing connector pad produces a blocking `missing_endpoint` issue for a
  required anchor;
- a net mismatch produces a blocking `net_mismatch` issue;
- two equally valid connector pads produce `ambiguous_endpoint`;
- known endpoint and anchor coordinates produce board-level route evidence;
- missing coordinates produce `not_routable`;
- `design create` emits stable JSON summaries for successful and failed
  bindings.

## Acceptance Criteria

- Required ESD and reverse-polarity anchors in generated block-based boards can
  be traced to real physical endpoints when those endpoints exist.
- Generated board route evidence distinguishes physical-endpoint-to-anchor
  routes from block-local anchor-to-component routes.
- `design create` reports bound, unbound, invalid, ambiguous, and unsupported
  anchors deterministically.
- Net mismatches and missing required endpoints block workflow success or
  readiness claims.
- Existing generated examples and tests keep passing.

## Open Questions

- Should board-edge-only anchors create explicit KiCad copper pads, test pads,
  or keep purely synthetic coordinate evidence until a footprint exists?
- Should connector pin role mapping live in component metadata, block metadata,
  or a dedicated board-interface model?
- Should a missing required anchor be repairable by adding a connector block, or
  should repair initially only report the missing dependency?
- Should anchor binding feed placement retry directly when an endpoint is too
  far away from its protection block?
