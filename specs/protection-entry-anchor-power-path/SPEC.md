# Protection Entry Anchor And Power-Path Realization Spec

## Summary

Extend circuit block PCB realization so protection blocks can model the layout
intent they currently describe only as unsupported behavior:

- ESD protection should know where the protected signal enters the board and
  should realize a short connector-to-TVS-to-signal local path plus a short TVS
  ground return.
- Reverse-polarity protection should realize the raw-input-to-diode and
  diode-to-protected-output power path with width evidence.

The immediate target blocks are:

- `esd_protection`
- `reverse_polarity_protection`

This project closes the next Priority 2 roadmap gap: route-through entry
anchors and power-path local route realization for protection blocks. It should
not claim complete fabrication readiness or KiCad DRC cleanliness by itself.

## Goals

- Add a first-class way for PCB realization metadata to describe external or
  block-boundary route endpoints.
- Realize protection block local routes that are not solely component-pin to
  component-pin routes.
- Keep the existing component-role route endpoint behavior backwards
  compatible.
- Add deterministic validation for route endpoint shape, route IDs, route
  lengths, route widths, and conditional metadata.
- Upgrade ESD and reverse-polarity block realization metadata with truthful
  local-route IDs.
- Upgrade ESD and reverse-polarity verification manifests to assert the new
  local-route evidence.
- Surface AI-readable evidence in `block show`, `block realize-pcb`,
  `block verify`, and `design create` block-planning output without requiring
  KiCad CLI.

## Non-Goals

- Do not implement full surge, ESD, EMI, or transient simulation.
- Do not select exact TVS capacitance, clamping voltage, or surge-current
  variants beyond the existing verified seed part.
- Do not implement ideal-diode MOSFET reverse-polarity topologies.
- Do not guarantee thermal performance or voltage-drop budget for the
  Schottky diode.
- Do not require `kicad-cli` in unit tests.
- Do not claim DRC-clean or fabrication-ready protection layouts until stable
  KiCad-backed evidence exists.
- Do not build a general hierarchical connector-placement solver in this pass.

## Current State

`esd_protection` currently declares:

- one `tvs` component;
- exported `SIGNAL` and `GND` ports;
- placement group `esd_shunt`;
- max-length constraint `esd_ground_short`;
- unsupported behavior notes for route-through ordering and external
  entry-point proximity.

`reverse_polarity_protection` currently declares:

- one `series_diode` component;
- exported `VIN_RAW`, `VIN_PROTECTED`, and `GND` ports;
- placement group `input_protection`;
- min-width constraint `input_diode_current_width`;
- unsupported behavior notes for raw connector entry anchors and thermal/power
  analysis.

Both blocks now require PCB realization evidence in their manifests, but they do
not assert protection-specific local routes because the realization model only
supports route endpoints with `component_role` and `pin`.

## Required Model Additions

### Route Endpoint Roles

Extend `blocks.RouteEndpoint` to support block-boundary endpoints while keeping
existing fields valid:

```go
type RouteEndpoint struct {
    ComponentRole string `json:"component_role,omitempty"`
    Pin           string `json:"pin,omitempty"`
    AnchorID      string `json:"anchor_id,omitempty"`
    Port          string `json:"port,omitempty"`
}
```

Endpoint modes:

- Component endpoint: `component_role` plus `pin`.
- Anchor endpoint: `anchor_id`, referring to a `PCBEntryAnchor` that carries
  the port binding.
- Port endpoint: `port` when the caller only needs a named block boundary
  point and does not need a separate anchor ID.

Exactly one endpoint mode must be set. Component endpoints preserve the current
validation rule requiring a known role and non-empty pin. Anchor endpoints and
port endpoints are mutually exclusive in a route endpoint even though entry
anchors themselves are bound to block ports.

### Entry Anchors

Add PCB realization metadata for block-boundary route anchor points:

```go
type PCBEntryAnchor struct {
    ID          string            `json:"id"`
    Port        string            `json:"port"`
    NetTemplate string            `json:"net_template,omitempty"`
    Placement   RelativePlacement `json:"placement"`
    Side        string            `json:"side,omitempty"`
    Description string            `json:"description,omitempty"`
    When        RealizationWhen   `json:"when,omitempty"`
}
```

Add to `PCBRealization`:

```go
EntryAnchors []PCBEntryAnchor `json:"entry_anchors,omitempty"`
```

Anchor semantics:

- `id` must be unique within a realization.
- `port` must refer to a known block port.
- `net_template`, when present, must map to an emitted block net.
- `placement` is relative to the block origin, like component realization
  placement.
- anchors may be conditional with `When`.
- anchors are evidence and route endpoints; they do not create physical
  footprint pads by themselves.

### Realized Anchor Output

Extend `BlockPCBRealizationResult` with:

```go
EntryAnchors []RealizedPCBEntryAnchor `json:"entry_anchors,omitempty"`
```

where:

```go
type RealizedPCBEntryAnchor struct {
    ID        string            `json:"id"`
    Port      string            `json:"port"`
    NetName   string            `json:"net_name,omitempty"`
    Placement RelativePlacement `json:"placement"`
}
```

`block realize-pcb` must include anchors so AI callers can see where protected
signals or power inputs are expected to enter or leave a block fragment.

## Required Realization Behavior

### Route Endpoint Resolution

`RealizeBlockPCB` must resolve local routes from:

- component endpoint to component endpoint;
- anchor endpoint to component endpoint;
- component endpoint to anchor endpoint;
- anchor endpoint to anchor endpoint.

For anchor endpoints, the realized route endpoint should use:

- `Ref`: a stable synthetic reference such as `@anchor:<id>` or a dedicated
  field if transaction endpoint types already support it;
- `Pin`: the anchor ID or port name;
- coordinates derived from the anchor placement when route operations need
  geometric points.

Route length must include:

- anchor-to-component distance;
- component-to-anchor distance;
- anchor-to-anchor distance;
- waypoint length in sequence.

Length calculation must remain deterministic and match current component-route
length behavior.

### Transaction Output

Generated `RouteOperation` entries should be emitted for local routes when
enough geometry exists. If the transaction route endpoint cannot represent an
anchor cleanly, emit a route operation with stable synthetic endpoint metadata
and retain the full anchor evidence in `realization.entry_anchors`.

The writer and board validation stages do not need to fabricate physical anchor
pads in this project. Anchor routes may remain evidence-only until board-level
composition maps anchors to connector pads or board-edge features.

## Block Metadata Requirements

### ESD Protection

Add entry anchors:

- `signal_entry`
  - port: `SIGNAL`
  - net template: `signal`
  - placement: just before the TVS on the protected line
- optional `ground_return`
  - port: `GND`
  - net template: `gnd`
  - placement: near TVS ground

Add local routes:

- `esd_signal_entry_to_tvs`
  - net: `signal`
  - from: `signal_entry`
  - to: `tvs` pin `1`
  - max intended length: short, about 2 mm
  - required: true
- `esd_tvs_to_ground`
  - net: `gnd`
  - from: `tvs` pin `2`
  - to: `ground_return` or port `GND`
  - max intended length: about 3 mm
  - required: true

Update unsupported behaviors by removing or narrowing:

- "route-through connector ordering is advisory until ordered net segments are
  modeled"
- "external connector entry-point proximity is advisory until entry anchors are
  modeled"

Remaining unsupported behavior should still mention that exact surge/capacitance
selection and KiCad DRC-backed layout proof are not complete.

### Reverse-Polarity Protection

Add entry anchors:

- `vin_raw_entry`
  - port: `VIN_RAW`
  - net template: `vin_raw`
  - placement: just before diode input
- `vin_protected_exit`
  - port: `VIN_PROTECTED`
  - net template: `vin_protected`
  - placement: just after diode output

Add local routes:

- `raw_input_to_diode`
  - net: `vin_raw`
  - from: `vin_raw_entry`
  - to: `series_diode` pin `2`
  - width: at least 0.6 mm for the current seed
  - required: true
- `diode_to_protected_output`
  - net: `vin_protected`
  - from: `series_diode` pin `1`
  - to: `vin_protected_exit`
  - width: at least 0.6 mm
  - required: true

Update unsupported behavior to keep:

- thermal dissipation and forward-voltage budget not calculated;
- MOSFET/ideal-diode topologies not generated.

The raw connector anchor unsupported note should be removed once anchors exist.

## Verification Manifest Requirements

Upgrade:

- `internal/blocks/testdata/verification/esd_protection_5v/manifest.json`
- `internal/blocks/testdata/verification/reverse_polarity_schottky/manifest.json`

Expected ESD manifest additions:

```json
"required_local_routes": [
  "esd_signal_entry_to_tvs",
  "esd_tvs_to_ground"
]
```

Expected reverse-polarity manifest additions:

```json
"required_local_routes": [
  "raw_input_to_diode",
  "diode_to_protected_output"
]
```

Keep `require_realization: true`.

Do not add `require_board_validation` until anchor evidence can map to physical
connector or board-edge features without causing false pad-net failures.

## CLI And AI Evidence Requirements

`kicadai --json --builtins block show esd_protection` should expose:

- `pcb_realization.entry_anchors`;
- local-route IDs;
- remaining unsupported behaviors.

`kicadai --json --builtins block realize-pcb esd_protection` should expose:

- realized anchor positions;
- realized local route endpoints and lengths;
- route operations where possible;
- no blocking realization issues for default parameters.

`kicadai --json --builtins block verify` should show `pcb_realization` passing
for ESD and reverse-polarity manifests and should fail if the expected local
route IDs disappear.

`design create` should carry these local-route IDs through block planning and
placement/routing evidence where current workflow summaries already expose
block realization evidence.

## Tests

Add or update focused tests for:

- `ValidatePCBRealization` accepts valid entry anchors.
- `ValidatePCBRealization` rejects:
  - duplicate anchor IDs;
  - unknown anchor ports;
  - invalid anchor route endpoint combinations;
  - route endpoints with neither component nor anchor data;
  - route endpoints with conflicting component and anchor data.
- `RealizeBlockPCB` emits entry anchors and resolves anchor-to-component route
  lengths.
- ESD default realization includes both required local routes.
- Reverse-polarity default realization includes both required local routes and
  width evidence.
- Block verification manifests fail if new route IDs are removed.
- CLI golden summaries include `pcb_realization` for the upgraded manifests.

Recommended commands:

```sh
go test ./internal/blocks ./internal/blocks/verification ./cmd/kicadai
go test ./cmd/kicadai -run TestRunBlockVerificationGoldens -update-block-verification-goldens
```

## Acceptance Criteria

- ESD and reverse-polarity blocks have first-class entry anchors.
- Their PCB realization metadata declares route-through or power-path local
  routes with stable IDs.
- Realization output includes anchor evidence and realized local routes for the
  default block parameters.
- Verification manifests assert the new route IDs.
- Existing timing, placement, and block verification tests keep passing.
- README, `docs/circuit-block-verification.md`, and `specs/ROADMAP.md` are
  updated after implementation.

## Open Questions

- Should synthetic anchor endpoints be represented in transaction operations as
  pseudo refs, or should transaction endpoints grow explicit anchor fields?
- Should entry anchors eventually become board-composition features that bind to
  connector pads, board-edge points, or imported mechanical constraints?
- Should ESD protection become a multi-component route-through block with an
  explicit connector role in a later phase?
- Should power-path width policy derive from `input_current` instead of the
  current fixed 0.6 mm seed constraint?
