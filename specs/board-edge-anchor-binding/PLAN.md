# Board-Edge And Mechanical Anchor Binding Implementation Plan

Date: 2026-06-25

## Objective

Make block entry anchors bind to explicit board-edge and imported mechanical
interface points, not only connector/interface footprint pads, so generated
protection circuits can prove external-interface intent in more realistic board
layouts.

## Phase 1: Request Schema And Validation

Add request-level external endpoint declarations.

Tasks:

- Add `ExternalEndpointSpec` to `internal/designworkflow/request.go`.
- Add `Request.ExternalEndpoints []ExternalEndpointSpec` with JSON key
  `external_endpoints`.
- Define endpoint ID, kind, net name, edge, layers, roles, point
  (`x_mm`, `y_mm`), source, confidence, required, and description fields.
- Normalize endpoint IDs, kinds, roles, layers, source, confidence, and edge
  strings. IDs normalize by trimming whitespace, lowercasing, replacing runs of
  characters outside `[a-z0-9_]` with `_`, preserving existing underscores, and
  trimming leading/trailing `_`; add a
  missing endpoint ID validation issue when the normalized slug is empty, add a
  duplicate endpoint ID validation issue when distinct source IDs normalize to
  the same slug, include the original source IDs in the issue message, and
  suggest using unique alphanumeric or underscore-only IDs. Use a distinct
  issue message when a provided ID contains no characters that survive
  normalization. Reject request IDs
  whose normalized slug starts with system-reserved prefixes such as
  `board_edge_point_`, `footprint_pad_`, or `imported_mechanical_point_`.
  Layer names normalize by trimming whitespace and canonicalizing known copper
  names to KiCad casing, such as `f.cu` -> `F.Cu`, `b.cu` -> `B.Cu`, and
  `in2.cu` -> `In2.Cu`. Technical layers such as `Edge.Cuts` may be
  canonicalized for diagnostics, but anchor binding treats them as non-copper
  and does not use them for layer compatibility.
- Validate:
  - duplicate or missing endpoint IDs;
  - unsupported endpoint kinds;
  - required endpoint missing point;
  - required endpoint missing net name;
  - negative board width or height as an error when dimensions are explicitly
    provided;
  - zero or missing board width/height as not-yet-final dimensions; defer bounds
    checks until positive board dimensions are available;
  - endpoint X or Y coordinates less than `-anchorBindingGeometryEpsilonMM` as
    invalid even when board width/height are not finalized; values between
    `-anchorBindingGeometryEpsilonMM` and `0` are accepted and snapped to `0`;
  - endpoint X/Y values within `anchorBindingGeometryEpsilonMM` above positive
    board width/height are accepted and snapped to the maximum board boundary;
  - endpoint point outside board bounds using inclusive rectangular bounds with
    shared `anchorBindingGeometryEpsilonMM = 0.001`;
  - invalid edge values;
  - invalid non-empty layers, allowing known copper layers such as `F.Cu`,
    `B.Cu`, and inner layers parsed from `InN.Cu` where `N >= 1`,
    `N <= L-2`, and `L` is the declared board copper layer count. Omitted or
    empty-normalized layers mean "any copper layer" for explicit request
    endpoints, compatible with any anchor layer while still preferring
    endpoints with explicit matching layers when candidates are otherwise
    equivalent. Required endpoints or endpoints with `net_name` that declare
    only diagnostic technical layers must report a validation issue because
    they cannot satisfy electrical anchor binding;
  - defer `InN.Cu` range validation until a positive board copper layer count is
    available; once available, require `N >= 1` and `N <= L-2`;
  - required endpoints or endpoints with `net_name` on a board with no
    available copper layers;
  - roles/layers normalized by trimming whitespace and dropping empty values.
- Document in validation tests that `required: true` requires `point` and
  `net_name`, while `required: false` allows advisory endpoints that remain
  visible but cannot route without a point.
- Add request validation tests for valid and invalid endpoint declarations.

Review gate:

- `go test ./internal/designworkflow`
- Prism review staged changes.
- Commit: `Add external endpoint request schema`

## Phase 2: Explicit Endpoint Discovery

Convert request-declared endpoints into `PhysicalEndpoint` values.

Tasks:

- Extend endpoint discovery to accept the normalized `Request` or an endpoint
  discovery options struct containing external endpoints and board dimensions.
- Emit `PhysicalEndpointBoardEdgePoint` and
  `PhysicalEndpointImportedMechanicalPoint` records from request declarations.
- Preserve ID, kind, net, roles, layers, point, source, and confidence.
- Treat request points as relative to the current board bounding box and convert
  all discovered pad points into the same board-relative frame before matching.
  Record the board frame used for endpoint conversion, and recalculate
  board-relative endpoint coordinates from request-relative source data if
  later workflow stages change board bounds before matching or route evidence
  generation. Transform request-relative imported endpoint points into the
  board-relative frame before endpoint discovery, proximity matching, or route
  evidence generation. Phase 2 supports rectangular, axis-aligned board
  bounding boxes only. The
  current generated-board workflow uses a zero-offset board box, but Phase 2
  must still introduce a coordinate-frame helper so future non-zero
  axis-aligned board origins can be handled by subtracting the board
  bounding-box `min_x` and `min_y` without changing binding logic.
- Use deterministic endpoint IDs that keep request endpoint IDs readable and
  stable.
- Report endpoint declaration issues through existing discovery issue paths.
- Add tests for:
  - explicit board-edge endpoint discovery;
  - explicit imported mechanical endpoint discovery;
  - missing optional point does not route but remains visible;
  - required invalid endpoint surfaces a blocking or error issue consistently.

Review gate:

- `go test ./internal/designworkflow`
- Prism review staged changes.
- Commit: `Discover explicit external anchor endpoints`

## Phase 3: Derived Board-Edge Endpoint Discovery

Derive board-edge endpoints from edge-facing generated components.

Tasks:

- Identify edge-facing components using placement edge constraints and
  block-derived edge-facing constraints.
- For each placed pad near a board boundary, derive a
  `board_edge_point` endpoint in addition to the existing `footprint_pad`
  endpoint.
- Determine nearest board edge through the board coordinate-frame helper, using
  the current axis-aligned board bounds, `Request.Board.WidthMM`,
  `Request.Board.HeightMM`, and `Board.EdgeClearanceMM`.
- Use `max(Board.EdgeClearanceMM, defaultDerivedBoardEdgeEndpointThresholdMM)`
  as the derivation threshold so manufacturing clearance does not become a
  restrictive endpoint-discovery gate. The default threshold is
  `defaultDerivedBoardEdgeEndpointThresholdMM = 1.5`.
- Use a component's explicit edge placement constraint as the first edge hint
  when it is present and tied for nearest edge. Otherwise break equal-distance
  corner ties in stable order: `left`, `right`, `top`, then `bottom`, using
  `anchorBindingGeometryEpsilonMM = 0.001` for equality, including exact center
  points equidistant from all edges.
- Add role `edge` to derived endpoints and retain existing component/net roles.
- Use stable IDs based on endpoint kind, ref, pad name, and
  `pad_discriminator`. Leave `pad_discriminator` empty for unique pad names. If
  a footprint has duplicate pad names, use a durable persisted pad ID when
  available; otherwise use a canonical footprint-local geometry discriminator
  made from local pad center rounded to 0.001 mm, canonical layer set, pad
  shape, and pad size. Geometry-derived discriminators must emit informational
  warning evidence because library geometry changes can change identity. The
  hash seed must exclude coordinates and volatile session state and must use
  length-prefixed fields:
  `kind:<len>:<kind>\nref:<len>:<ref>\npad:<len>:<pad>\npad_discriminator:<len>:<stable-pad-uid-or-empty>\n`.
  Treat nearest edge and board-absolute placement coordinates as endpoint
  evidence, not endpoint identity, and do not include edge in the readable ID
  string.
- Add deterministic sorting across pad, board-edge, and imported endpoints.
- Add tests for:
  - USB/connector edge-facing pad produces a board-edge endpoint;
  - non-edge components do not produce board-edge endpoints;
  - pads far from the edge do not produce board-edge endpoints;
  - bottom-layer edge pads preserve layer evidence.

Review gate:

- `go test ./internal/designworkflow`
- Prism review staged changes.
- Commit: `Derive board edge anchor endpoints`

## Phase 4: Binding Resolution Policy

Teach anchor binding to treat board-edge and imported mechanical endpoints as
valid external endpoint candidates.

Tasks:

- Extend `endpointLooksExternal` so:
  - `board_edge_point` always satisfies external endpoint requirement;
  - `imported_mechanical_point` satisfies it when roles include `connector`,
    `edge`, `external`, `power`, `power_entry`, `mechanical_interface`,
    `signal`, `ground`, or `castellated`;
  - existing connector ref/role behavior remains unchanged.
- Apply deterministic precedence:
  - board-edge endpoint over its source footprint-pad endpoint for anchors that
    require an external endpoint when both share the same physical pad evidence;
  - explicit request endpoints over all inferred endpoints, including derived
    endpoints and footprint-pad endpoints, whenever both are compatible and
    inside the anchor search radius;
  - nearest compatible endpoint;
  - ambiguity when non-equivalent compatible endpoints remain.
- Preserve current hard net/layer/proximity filtering.
- Add issue tests for missing, ambiguous, net mismatch, and unsupported endpoint
  kind cases.
- Add binding success tests for:
  - ESD signal anchor to board-edge endpoint;
  - reverse-polarity raw VIN anchor to imported mechanical endpoint;
  - optional endpoint binding does not block when absent.

Review gate:

- `go test ./internal/designworkflow`
- Prism review staged changes.
- Commit: `Resolve anchors to external endpoint kinds`

## Phase 5: Route Evidence And Workflow Goldens

Prove routed endpoint-to-anchor evidence in `design create`.

Tasks:

- Ensure `AddAnchorBindingRoutes` routes board-edge and imported mechanical
  endpoints when endpoint and anchor coordinates plus net name are known.
- For layer-agnostic explicit endpoints, route on the anchor's preferred layer
  when present; otherwise use the first available copper layer from the board
  stackup, ordered by lowest stackup index/topmost copper first, as the
  deterministic default and include that selected layer in route evidence.
- Add route-status tests for:
  - routed board-edge endpoint;
  - routed imported mechanical endpoint;
  - missing endpoint point -> `not_routable`;
  - net mismatch refuses route.
- Add or extend `design create` fixture requests:
  - ESD block bound to explicit board-edge signal point;
  - reverse-polarity block bound to imported mechanical VIN point;
  - missing required endpoint negative fixture;
  - net mismatch negative fixture.
- Update CLI goldens for stable `anchor_bindings` summaries and issue paths.

Review gate:

- `go test ./internal/designworkflow`
- `go test ./cmd/kicadai`
- `go test ./...`
- Prism review staged changes.
- Commit: `Add external anchor binding workflow goldens`

## Phase 6: Documentation And Roadmap

Document the new external endpoint support.

Tasks:

- Update `README.md` design workflow and block verification sections.
- Update `specs/ROADMAP.md` Priority 2 current foundation and remaining work.
- Update `docs/circuit-block-verification.md` if endpoint examples there should
  include board-edge or imported mechanical declarations.
- Document that board-edge/imported-mechanical binding proves external
  interface intent, not KiCad DRC cleanliness or fabrication readiness.
- Document the JSON shape for `external_endpoints`.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Document external anchor endpoint binding`

## Final Completion Criteria

- Request JSON can declare board-edge and imported mechanical endpoints.
- Endpoint discovery emits pad, derived board-edge, explicit board-edge, and
  imported mechanical endpoint candidates deterministically.
- Required protection anchors can bind to non-pad endpoint kinds.
- Endpoint-to-anchor route evidence works for compatible endpoints.
- Missing, ambiguous, invalid, and not-routable states remain explicit.
- CLI goldens lock down AI-facing `anchor_bindings` evidence.
- Normal tests remain independent of local KiCad.
