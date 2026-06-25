# Board-Edge And Mechanical Anchor Binding Specification

Date: 2026-06-25

## Summary

KiCadAI now binds block entry anchors to physical connector/interface pads during
`design create`, and protection blocks can expose board-level `anchor_bindings`
evidence. The next gap is external interfaces that are not naturally represented
as connector pads:

- board-edge contact points;
- castellated or card-edge entry points;
- mechanical/user-authored external interface points from imported layouts;
- explicit design-intent endpoints that should bind protection anchors before
  complete component placement exists.

This specification extends anchor binding so `board_edge_point` and
`imported_mechanical_point` become first-class endpoint sources rather than
placeholder enum values.

## Problem

Current anchor binding discovers endpoints from placed footprint pads. That is
enough for connector-backed examples, but many practical boards expose signals
or power through geometry that is not a conventional connector footprint:

- a protected signal may enter through an edge contact or castellated pad;
- a reverse-polarity block may protect a raw VIN point represented by an
  imported mechanical datum;
- a user may want to reserve a board-edge service point before choosing the
  final connector;
- imported project review may expose mechanical/interface constraints without a
  writer-owned footprint.

Without these endpoint kinds, AI output can only say "no physical endpoint
found" even when the design intent has enough geometry to bind the anchor.
That blocks useful board-level protection evidence and makes repair suggestions
less precise.

## Terminology

- An `anchor` is a circuit-block entry point that needs board-level physical
  evidence, such as a protected signal input or raw VIN input.
- An `endpoint` is a physical board point that can satisfy an anchor, such as a
  footprint pad, board-edge point, or imported mechanical point.
- A `binding` is the resolved relationship between one anchor and one endpoint,
  including diagnostics and route evidence.

## Goals

- Add request-level external endpoint declarations for board-edge and imported
  mechanical points.
- Derive board-edge endpoint candidates from edge-facing generated components
  when a pad lies near the board boundary.
- Preserve current footprint-pad endpoint discovery and binding behavior.
- Resolve anchor bindings against pad, board-edge, and imported mechanical
  endpoint candidates with deterministic precedence and diagnostics.
- Expose endpoint source, confidence, and binding evidence in `design create`
  output.
- Keep missing or unsupported endpoint evidence explicit for AI callers.
- Add golden and unit coverage for successful, missing, ambiguous, and
  net-mismatch binding cases.

## Non-Goals

- Do not implement full mechanical CAD import.
- Do not mutate imported user projects.
- Do not infer hidden electrical connectivity from arbitrary outline geometry.
- Do not treat board-edge or mechanical endpoint binding as fabrication
  readiness by itself.
- Do not replace KiCad ERC/DRC, writer correctness, or connectivity-first board
  validation.

## Current Implementation To Reuse

Existing anchor binding foundations already include:

- `PhysicalEndpointKind` values:
  - `footprint_pad`;
  - `board_edge_point`;
  - `imported_mechanical_point`.
- `PhysicalEndpoint`, `AnchorBinding`, `AnchorBindingIssue`, and
  `AnchorBindingSummary` models.
- placed footprint pad endpoint discovery in
  `internal/designworkflow/anchor_endpoint_discovery.go`;
- grid-backed proximity matching in
  `internal/designworkflow/anchor_binding_resolution.go`;
- endpoint-to-anchor route generation in
  `internal/designworkflow/anchor_binding_routes.go`;
- `design create` integration through routing-stage `anchor_bindings` evidence.

The new work should extend these surfaces instead of creating a parallel
binding model.

## Proposed Request Model

Add explicit external endpoint declarations to `designworkflow.Request`.

```text
Request
  external_endpoints []ExternalEndpointSpec

ExternalEndpointSpec
  id
  kind: board_edge_point | imported_mechanical_point
  net_name
  roles []
  layers []
  point
  edge
  source
  confidence
  required
  description
```

Field rules:

- `id` is required and must be stable within a request.
- `kind` is required and initially accepts only `board_edge_point` and
  `imported_mechanical_point`.
- `net_name` is optional for advisory endpoints but required for endpoints that
  participate in required anchor binding. Net-name matching follows KiCad net
  semantics and is case-sensitive.
- `roles` should include values such as `connector`, `edge`, `external`,
  `power`, `power_entry`, `signal`, `ground`, `castellated`, or
  `mechanical_interface`.
- `layers` is optional for explicit request endpoints. Omitted layers, or a
  layer list that normalizes to empty, mean "any copper layer" for binding.
  Derived endpoints always inherit the source pad layers and must not use an
  explicit-endpoint default. A missing field and `[]` are both canonical
  wildcard-copper declarations; non-empty strings that cannot be canonicalized
  to a supported copper layer or known diagnostic-only technical layer are
  validation errors. Technical layers such as `Edge.Cuts` may be preserved for
  diagnostics but are ignored for binding layer compatibility. For omitted
  layers on explicit `board_edge_point` endpoints, binding prefers accessible
  surface copper layers (`F.Cu`, `B.Cu`) before internal copper layers.
- `point` is board-absolute millimeters and is required for routing. The
  initial coordinate system is relative to the rectangular,
  axis-aligned board bounding box: `(0,0)` is the top-left of that bounding
  box, X increases right, and Y increases downward. It is not a global
  CAD-origin coordinate. Imported mechanical points from Y-up systems must be
  converted before request submission. Discovery records the board frame used
  for endpoint conversion; if a later workflow stage changes the board bounds
  before matching or routing, endpoint board-relative coordinates must be
  recalculated from the request-relative source data. For simple Y-up imported
  coordinates already relative to the same board rectangle, convert with
  import helpers must first transform source coordinates into the request's
  board bounding-box frame. After that transform, simple Y-up coordinates
  relative to the bottom-left corner of the same bounding box convert with
  `y_down = board_height - y_up`.
  Discovery must convert placed pad coordinates into this same board-relative
  frame before proximity matching. The current generated-board workflow has a
  zero board origin, but the conversion helper should still exist so non-zero
  imported axis-aligned origins can be supported later by subtracting the board
  bounding box `min_x` and `min_y`. Imported endpoint declarations are parsed
  in the request-relative frame first, then transformed into the snapshotted
  board-relative frame before endpoint discovery, proximity matching, or route
  evidence generation. Rotated board frames and non-rectangular outlines are
  out of scope for this phase; future polygon/transform support should replace
  width/height edge checks where those outlines are modeled.
- `edge` is optional descriptive evidence and can be one of `left`, `right`,
  `top`, `bottom`. Derived edge selection must break corner-distance ties in
  stable order: `left`, `right`, `top`, then `bottom`; for example, if
  distance to `left` and `top` are equal within epsilon, select `left`.
- `source` defaults to `request.external_endpoints`.
- `confidence` defaults to `high` for request-declared board-edge endpoints
  and `medium` for request-declared imported mechanical endpoints. Imported
  endpoints may explicitly set `low` when the source is advisory or inferred.
- `required` controls validation of the endpoint declaration itself, not anchor
  binding policy. Anchor binding policy remains derived from block policy until
  a later spec adds per-anchor override declarations.

JSON example:

```json
{
  "external_endpoints": [
    {
      "id": "edge_vin",
      "kind": "board_edge_point",
      "net_name": "power_vin_raw",
      "roles": ["power", "edge", "connector"],
      "layers": ["F.Cu"],
      "point": {"x_mm": 2.0, "y_mm": 15.0},
      "edge": "left",
      "description": "Raw VIN enters from a castellated left-edge contact"
    }
  ]
}
```

## Derived Board-Edge Endpoints

Endpoint discovery should also derive board-edge endpoints from generated
component pads when all of these are true:

- the component has a hard edge constraint or edge-facing PCB realization
  constraint;
- the pad has known absolute coordinates;
- the pad is within the configured board edge clearance or a small default
  threshold from the nearest board boundary. The default threshold is
  `defaultDerivedBoardEdgeEndpointThresholdMM = 1.5`;
- the pad has a net name or a role that can be matched to an anchor.

Endpoint IDs from requests are normalized by trimming whitespace, converting to
lowercase, replacing runs of non-alphanumeric characters with `_`, and trimming
leading/trailing `_`. Duplicate detection runs after normalization.

Derived endpoint identity should be stable and separate from the pad endpoint:

```text
board_edge_point:<ref>:<pad>:<hash>
```

`ref` and `pad` may be normalized for readable ID text by lowercasing and
replacing non-alphanumeric separator characters with `_`, but identity must
come from the hash. `edge` must not appear in the ID string because it is
mutable evidence, not identity. The hash seed must be length-prefixed and
include stable logical fields so separator collisions are impossible:

```text
kind:<len>:<kind>\nref:<len>:<ref>\npad:<len>:<pad>\npad_discriminator:<len>:<stable-pad-uid-or-empty>\n
```

Board-relative and board-absolute coordinates must not participate in endpoint
identity. They are endpoint attributes and may change when placement improves.
`edge` is also evidence, not identity, because a corner or board-size adjustment
can change nearest-edge classification. Pad name is the identity field.
`pad_discriminator` is empty
for unique pad names. If a footprint has duplicate pad names, use a durable
persisted pad ID when the footprint source exposes one. If no durable pad ID
exists, use a canonical footprint-local geometry discriminator made from local
pad center rounded to 0.001 mm, canonical layer set, pad shape, and pad size.
Geometry-derived discriminators are stable across placement moves but can change
when the footprint library geometry changes; derived endpoints that rely on them
must include informational warning evidence. Never use source order, internal
array index, regenerated UUIDs, placement, rotation, or board-size-derived
coordinates for `pad_discriminator`.

The derived endpoint should include:

- `kind=board_edge_point`;
- the original `ref` and `pad`;
- the same net and layers as the pad;
- roles from the component plus `edge`;
- the pad point;
- `source=placement.edge_pad`;
- `confidence=high` when net and role are known, otherwise `medium`.

This preserves the pad endpoint while giving external-interface matching a
clear endpoint kind that satisfies protection-block external endpoint policy.

## Imported Mechanical Endpoints

Imported mechanical endpoints are request-declared for this phase. They model
known board coordinates that come from a user, importer, or external process.

Rules:

- They can bind anchors when net/layer compatibility succeeds.
- They default to `confidence=medium`; callers may explicitly set `low` when
  the source is advisory or inferred.
- They do not create KiCad mechanical geometry by themselves.
- They may create route operations if both endpoint and anchor points are
  known and a net name is available.
- If they are required but missing point or net metadata, validation should
  report an actionable issue before routing.

## Binding Resolution Policy

Candidate filtering should stay hard-gated by:

- point availability;
- maximum proximity;
- net compatibility;
- layer compatibility;
- external endpoint requirement for protection blocks.

Endpoint kinds should participate as follows:

1. For anchors that require an external endpoint, a board-edge endpoint derived
   from a footprint pad wins over the source footprint-pad endpoint when both
   represent the same ref/pad/location/net/layers. This prevents the derived
   external evidence from making every edge-facing connector ambiguous.
2. Explicit request endpoints win over all inferred endpoints, including
   derived board-edge endpoints and footprint-pad endpoints, whenever both are
   compatible and inside the anchor search radius, because explicit declarations
   are user or AI intent rather than inferred evidence.
3. Multiple compatible explicit request endpoints for the same anchor remain
   ambiguous unless they are equivalent by policy. Document order must not be
   used to hide conflicting explicit intent.
4. Layer precedence runs before generic ambiguity handling:
   exact explicit endpoint-layer and anchor-layer match first;
   layer-agnostic endpoint resolved to the anchor preferred layer second;
   layer-agnostic endpoint resolved to accessible surface copper (`F.Cu`,
   `B.Cu` in board stackup order) third;
   layer-agnostic endpoint resolved to internal copper in stackup order last.
   If this ranking still leaves multiple non-equivalent candidates at the same
   rank, report `ambiguous_endpoint`.
5. Apply explicit-vs-inferred and layer precedence before evaluating endpoint
   equivalence or ambiguity.
6. When compatible equivalent endpoints are tied within
   `anchorBindingGeometryEpsilonMM = 0.001` after precedence rules, choose
   the lexicographically smallest `endpoint_id` as the final deterministic
   tie-breaker. This mainly applies to multiple explicit declarations at the
   same location with equivalent metadata.
7. Footprint pads remain valid candidates.
8. Board-edge endpoints satisfy `ExternalEndpointRequired` for ESD and
   reverse-polarity protection.
9. Imported mechanical endpoints satisfy `ExternalEndpointRequired` only when
   they include role `connector`, `edge`, `external`, `power`, `power_entry`,
   `mechanical_interface`, `signal`, `ground`, or `castellated`.
10. Multiple non-equivalent compatible endpoints remain `ambiguous`.
11. Equivalent endpoints may be selected deterministically with info evidence,
   preserving existing equivalent-endpoint behavior.

The existing `endpointLooksExternal` helper should be extended rather than
duplicated.

The cross-kind board-edge-over-source-pad precedence rule is applied before
checking ambiguous non-equivalent endpoints. Endpoints are equivalent for
deterministic tie-breaking only when they share
the same net name, endpoint kind, normalized layer set, board-relative point
within `anchorBindingGeometryEpsilonMM`, source ref, and source pad. Compatible
non-equivalent endpoints tied within the same epsilon remain ambiguous. The
derived board-edge endpoint/source footprint-pad exception above is the only
cross-kind equivalence rule in this phase.

## Validation And Issues

Add request validation for external endpoints:

- missing endpoint ID;
- duplicate endpoint ID;
- unsupported endpoint kind;
- missing point on required endpoint;
- invalid edge value;
- invalid non-empty layer strings, with inner copper layers accepted by parsing
  `InN.Cu`, requiring `N` to be between 1 and 30 inclusive, and requiring the
  layer to exist within the declared board stackup. For a board with `L` copper
  layers, internal copper layers are valid only when `N` is in `[1, L-2]`; for
  boards with fewer than two copper layers, no `InN.Cu` layer is valid. Odd
  copper layer counts are accepted when the board stackup declares them; the
  same `[1, L-2]` internal-layer range applies;
- required endpoints or endpoints with `net_name` on a board with no available
  copper layers, because electrical anchor binding and route evidence need at
  least one copper layer;
- required endpoint missing `net_name`;
- point outside the declared board area, using inclusive bounds with
  shared `anchorBindingGeometryEpsilonMM = 0.001` when positive board width and
  height are available. Out-of-bounds issues should suggest checking whether
  imported points were provided in a different coordinate frame or Y-up
  convention;
- required endpoints with a point while board width or height is explicitly
  non-positive. Missing or not-yet-final dimensions defer bounds validation
  until positive dimensions are available;
- endpoint role strings with leading/trailing whitespace.

Add or reuse binding issues:

- `missing_endpoint`;
- `ambiguous_endpoint`;
- `missing_endpoint_point`;
- `net_mismatch`;
- `role_mismatch`;
- `unsupported_endpoint_kind`.

When possible, issue repair hints should tell the AI caller whether to add an
external endpoint, move the endpoint near the anchor, fix the net name, or add
explicit connector/interface intent.

`not_routable` on a required anchor binding is blocking. `not_routable` on an
optional or advisory binding is non-blocking evidence unless another validation
policy explicitly requires route completion.

## Workflow Output

`design create` should continue to expose `anchor_bindings` in the routing
stage. New endpoint kinds must appear in existing evidence fields:

- `endpoint_kind`;
- `endpoint_id`;
- `endpoint_net_name`;
- `endpoint_layers`;
- `endpoint_point`;
- `distance_mm`;
- `route_status`;
- `issue_ids`.

No new top-level workflow stage is required. Endpoint discovery details may be
added to routing-stage summary if useful, such as:

```text
anchor_binding_endpoint_counts:
  footprint_pad
  board_edge_point
  imported_mechanical_point
```

## Testing Requirements

Unit tests:

- request validation accepts valid board-edge and imported mechanical endpoints;
- request validation rejects duplicate IDs, unsupported kinds, missing required
  points, missing required nets, invalid edge values, and outside-board points;
- endpoint discovery includes explicit request endpoints;
- endpoint discovery derives board-edge points from edge-facing placed pads;
- board-edge endpoints satisfy external endpoint requirements;
- imported mechanical endpoints bind only when role/net/layer compatible;
- ambiguous endpoint handling remains deterministic;
- endpoint-to-anchor route generation works for board-edge and imported
  mechanical endpoints.

Golden tests:

- `design create` with ESD bound to a board-edge point;
- `design create` with reverse-polarity VIN bound to an imported mechanical
  point;
- missing board-edge endpoint produces a stable required issue;
- net mismatch produces a stable invalid binding issue.

## Documentation Requirements

Update:

- `README.md` block verification/design workflow section;
- `specs/ROADMAP.md` Priority 2;
- optionally `docs/circuit-block-verification.md` if endpoint examples are
  expanded there.

Docs must state that board-edge/mechanical binding is stronger external
interface evidence, not KiCad DRC or fabrication readiness.

## Acceptance Criteria

- `designworkflow.Request` can declare board-edge and imported mechanical
  endpoints.
- Anchor binding can select these endpoints and report their kinds in workflow
  evidence.
- Protection anchors can bind to board-edge points without requiring a
  connector footprint.
- Imported mechanical endpoints can bind and route when they carry compatible
  net/layer/point metadata.
- Required bad bindings block deterministically with actionable issues.
- Normal `go test ./...` remains KiCad-independent.
- Prism review passes before each implementation commit.
