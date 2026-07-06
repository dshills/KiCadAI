# Amplifier PCB Realization Closeout Specification

## Objective

Promote the protected Class AB headphone amplifier design workflow past PCB
realization and placement so routing, writer correctness, board validation, and
real KiCad ERC/DRC evidence can run.

The immediate target fixture is:

- `examples/design/kicad-backed/class_ab_headphone_protected.json`
- `examples/design/kicad-backed/class_ab_headphone_protected.metadata.json`

The fixture already proves the schematic side of the protected amplifier path:

- verified LMV321/op-amp selection;
- verified MMBT3904/MMBT3906 output transistor selection;
- `headphone_output_protection` block participation;
- composition alias cleanup;
- schematic electrical validation.

The remaining blocker is PCB realization:

- generated output-stage placements exceed the current board outline;
- fixed placements collide or land outside the usable board area;
- some inter-block routing endpoints still do not resolve to generated PCB
  pads or explicit board-edge endpoints.

## Scope

This project closes the first PCB realization gap for the protected amplifier
fixture without rewriting the placement engine.

In scope:

- identify and reproduce the exact placement and endpoint blockers reported by
  `design create`;
- make amplifier block-local PCB geometry fit within a deterministic board
  envelope;
- ensure output-stage, protection, load/reference, and gain-stage endpoints
  resolve to generated footprint pads or supported board-edge endpoints;
- preserve current block-local route evidence and schematic electrical
  behavior;
- update promotion metadata so the fixture advances to the next real blocker;
- add regression tests that fail if the amplifier fixture regresses to
  placement or endpoint-realization blockage.

Out of scope:

- full automatic analog PCB layout;
- production thermal/SOA proof;
- active short-circuit or DC fault protection design;
- speaker or bridged power-amplifier support;
- KiCad DRC-clean promotion if routing still exposes valid downstream issues;
- replacing the general placement engine.

## Current Failure Model

The current protected amplifier fixture is expected to stop before routing and
project write. The failure surface is useful and should be preserved until each
part is actually resolved:

- `pcb_realization.output` warning or blocking evidence when a block fragment
  cannot fit or cannot be translated to placement inputs;
- `placement` blocking evidence for generated component positions outside the
  board or within illegal spacing;
- endpoint diagnostics for route endpoints such as amplifier output,
  protection input/output, headphone output, load return/reference, and gain
  input that do not bind to a concrete generated pad.

After this project, those failure modes must not appear for
`class_ab_headphone_protected`. If a later stage blocks, metadata must describe
the new blocker accurately.

## Functional Requirements

### FR1: Deterministic Amplifier Board Envelope

The protected Class AB amplifier fixture must declare a board envelope large
enough for its fixed block-local PCB geometry.

Requirements:

- explicit board dimensions in the fixture or request are authoritative and
  used as-is; they override any derived dimensions, and undersized explicit
  dimensions must emit blocking placement diagnostics instead of being silently
  enlarged;
- automatic board-envelope derivation or growth is out of scope for this
  closeout and must not run for explicit dimensions;
- the solution must not globally enlarge unrelated small examples unless they
  request or require it.

### FR2: Amplifier Block Placement Legality

Generated Class AB output and protection placements must be legal inside the
board envelope.

Requirements:

- all generated footprint positions must be within the board outline after
  footprint extents and edge clearance are considered;
- every generated component footprint must be fully contained inside the board
  envelope using the footprint courtyard when available, otherwise the
  calculated physical/electrical bounding box from copper, pad, drill, hole,
  fabrication-body, and silkscreen geometry plus the active board-edge
  clearance in either case; enlarging the board must not disable
  component-level legality checks;
- edge-mounted components may have mechanical body or courtyard geometry extend
  past the board edge only when explicitly marked as edge-mounted; pads, copper
  features, and holes must remain inside the board envelope unless explicitly
  marked as edge-interface features, in which case the declared edge-interface
  clearance policy applies instead of the default board-edge clearance;
- fixed block-local placements must preserve intended local topology;
- output pair, bias network, DC-blocking capacitor, bleed/load/reference
  elements, and connector must remain ordered in signal-flow direction where
  the current block metadata declares local placement intent; for this closeout,
  signal-flow validation requires explicit block metadata such as
  `primary_axis`, `flow_direction`, or equivalent local placement intent, with
  rotation and mirroring transforms applied before checking order;
- placement diagnostics must identify the specific generated component and
  placement rule when a future regression occurs.

### FR3: Endpoint-to-Pad Realization

Inter-block routing endpoints emitted by the amplifier fixture must bind to
concrete routing anchors.

Requirements:

- every composed connection that reaches PCB realization must resolve both ends
  to one of:
  - a generated footprint pad with matching net assignment;
  - an explicit board-edge endpoint;
  - a supported imported mechanical or external endpoint;
  - a modeled net-tie/star-point component pad when the design explicitly
    declares that topology;
- endpoint binding evidence must include the originating block/port, resolved
  ref/pad, net, and failure reason when unresolved;
- resolved endpoint nets must match the schematic/composition alias net;
- no route endpoint may silently fall back to center-point or placeholder
  routing when a pad is expected; unresolved expected-pad endpoints must emit a
  blocking diagnostic that prevents routing or project writing.

### FR4: Net Continuity Preservation

Alias-cleaned schematic nets must survive PCB realization and placement.

Requirements:

- pad net assignments for functional nets must remain canonical where
  connected across blocks, including the current fixture names `AUDIO_IN`,
  `DRIVER_OUT`, `BIAS_P`, `BIAS_N`, `AMP_OUT_DC_BIASED`, `HP_OUT`, `HP_RET`,
  `LOAD_REF`, `VCC`, `GND`, `VEE`, and any local protection/output nets;
- block-local nets may remain local only when they do not represent an
  inter-block connection;
- route, zone, and pad operations must not reintroduce block-local aliases for
  exported composition nets.

### FR5: Evidence and Metadata

Promotion evidence must explain what changed and what still blocks.

Requirements:

- `class_ab_headphone_protected.metadata.json` expected stages must advance
  past `placement` when placement is fixed;
- if routing or writer validation becomes the next blocker, metadata must name
  that blocker exactly;
- tests must assert that old placement and endpoint blockers are absent;
- promotion reports must include enough evidence for an AI agent to decide the
  next repair action.

## Non-Functional Requirements

- Deterministic output across repeated test runs.
- No network dependency.
- No dependency on KiCad CLI for unit-level acceptance.
- Optional KiCad CLI checks remain opt-in and must not make ordinary tests
  flaky.
- Existing LED, connector, I2C, regulator, MCU, timing, routing, and placement
  tests must continue to pass.
- Changes should prefer existing block metadata, placement request, routing
  adapter, and design workflow structures over ad hoc fixture-only branches.

## Design Constraints

- Do not weaken placement validation to make the fixture pass.
- Do not mark endpoints as resolved unless there is a concrete pad or supported
  explicit external endpoint.
- Do not remove local-route evidence for amplifier blocks.
- Do not expand board size automatically in this closeout; use request-level
  board dimensions and block when they are insufficient.
- Do not bypass writer correctness or validation stages once placement succeeds.

## Acceptance Criteria

The project is complete when:

1. `design create` for `class_ab_headphone_protected` no longer blocks at
   `pcb_realization` or `placement`.
2. The fixture promotion test asserts the old placement and endpoint blocker
   paths are absent.
3. The fixture advances to routing, writer correctness, board validation, or
   KiCad evidence as the next accurately documented stage.
4. Focused tests pass for amplifier block realization, endpoint binding, design
   workflow promotion classification, and placement diagnostics.
5. `go test ./...` passes with the repository-local Go build cache.
6. Prism review has no unresolved high or medium correctness findings for each
   implementation phase.

## Risks

- Enlarging the board may hide genuine placement mistakes if not paired with
  component-level inside-board assertions.
- Moving generated placements may break block-local route evidence.
- Endpoint binding may expose missing footprint/pad metadata in amplifier
  blocks rather than a placement bug.
- KiCad ERC/DRC may reveal the next blocker after this project; that is
  expected and should be recorded as the new fixture status, not hidden.

## Closeout Decisions

- The protected amplifier fixture uses explicit board dimensions. Automatic
  board-envelope derivation is deferred to a future project.
- Amplifier block-local output/protection placements remain fixed relative
  groups for this closeout. A later placement-mobility project may make those
  groups movable only if it preserves local-route intent and validates
  signal-flow ordering constraints.
- Headphone connector and load-reference behavior must bind to generated
  connector/load/reference pads in this fixture. Board-edge endpoints are only
  acceptable for explicitly modeled external interface anchors that already
  exist in the request or block metadata.
