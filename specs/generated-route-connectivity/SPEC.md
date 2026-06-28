# Generated Route Endpoint Connectivity Specification

Date: 2026-06-28

## Summary

Generated design-level PCBs must connect routed copper to the physical pad
anchors that KiCad uses for board connectivity.

The previous generated-design net assignment work made footprints, pads,
tracks, vias, zones, and local-route copper carry consistent net names and net
codes. That closed the writer-correctness blocker where a generated board could
contain plausible objects without trustworthy net identity. The next blocker is
more physical: copper can now have the right net, but route endpoints may still
miss the same-net pads they are meant to connect.

This specification closes that gap by making generated local routes and routed
segments resolve their endpoints from placed, hydrated footprint pads. The goal
is to move KiCad-backed generated examples from "electrically named" toward
"electrically connected."

## Problem

The optional KiCad-backed design examples now expose a route-connectivity
failure after net assignment succeeds. Typical failures include:

- a footprint pad is assigned to the intended net but remains disconnected;
- a track has the intended net code but neither endpoint touches a same-net pad;
- a local route is generated from block intent but does not account for final
  footprint placement, rotation, pad offset, or side;
- a net appears partially routed or unrouted even though visual copper exists;
- KiCad ERC/DRC continues to report connectivity problems for generated
  fixtures that should be simple enough to connect.

This is a correctness boundary. A board with the right net table and right
copper net codes can still be electrically wrong if track endpoints do not
physically coincide with pad anchors, vias, or other connected copper on the
same net.

## Goals

- Resolve generated route endpoints from actual placed footprint pad anchors.
- Use hydrated footprint pad geometry, component placement, component rotation,
  board side, and layer information when computing endpoint coordinates.
- Bind block-local routes to their source and destination pads before PCB write.
- Emit tracks whose start and end points touch same-net pads, vias, or routed
  copper according to the existing board-validation tolerance.
- Preserve generated net names and net codes from the canonical net assignment
  path.
- Add pre-write route endpoint validation for generated local routes.
- Expose deterministic route-connectivity evidence in design workflow output.
- Promote the LED KiCad-backed generated fixture past the route-endpoint
  disconnected-pad blocker when the evidence supports it.
- Keep normal `go test ./...` independent of a local KiCad installation.

## Non-Goals

- Do not build a full autorouter in this project.
- Do not guarantee every generated board is DRC-clean after this work.
- Do not solve schematic ERC wiring issues unless they directly affect PCB
  route endpoint binding.
- Do not infer unsafe pin-to-pad mappings for unknown components.
- Do not weaken writer correctness, board validation, or optional KiCad checks.
- Do not mutate imported KiCad projects or preservation-sensitive content.
- Do not treat visual proximity as connectivity unless the physical endpoint
  model proves a same-net contact.

## Current Foundations

The project already has the foundations this work should extend:

- generated schematic and PCB writers;
- schematic-to-PCB transfer workflow;
- generated design net table and pad/copper net assignment;
- resolver-backed footprint graphics and pad hydration;
- component pinmaps and block component pin definitions;
- block PCB realization with required local routes;
- placement transaction and placement evidence models;
- routing engine and generated route operations;
- local-route mobility classification;
- writer correctness checks for PCB nets, pads, and copper;
- board validation for disconnected pads, route endpoints, outlines, zones, and
  DRC evidence hooks;
- optional KiCad-backed design fixtures under `examples/design/kicad-backed/`.

This work should connect those pieces rather than introduce a parallel routing
model.

## Required Model

### Physical Pad Endpoint

Generated routing must be able to ask for a physical pad endpoint by component
reference and pad identity.

Each resolved endpoint must include:

- component reference;
- footprint identity;
- pad number or name;
- assigned net name;
- assigned net code;
- absolute board coordinate;
- copper layer or side;
- component placement coordinate;
- component rotation;
- pad offset before placement transform;
- evidence source;
- confidence;
- issue path when unresolved.

Endpoint resolution must use hydrated footprint pad geometry when available.
If a selected footprint lacks pad geometry, generated routes that depend on
that pad must remain blocked with diagnostics instead of falling back to an
arbitrary component origin.

### Placement Transform

Endpoint coordinates must be derived from final component placement.

The transform must account for:

- footprint local pad offset;
- component position;
- component rotation;
- side/layer where the existing placement model supports it;
- units used by the PCB writer and board-validation layer;
- deterministic rounding compatible with existing writer behavior.

Top-side, zero-rotation components should be the first supported case.
Rotated top-side footprints must be covered by tests. Bottom-side behavior
should be supported where the existing placement and footprint models already
represent it; otherwise it must be diagnosed as an explicit unsupported route
binding case.

### Route Endpoint Binding

Generated local routes must bind to physical endpoints before final PCB write.

A route binding input should identify:

- route ID or operation ID;
- source block or design workflow stage;
- net name and net code;
- source endpoint by reference and pad/pin;
- destination endpoint by reference and pad/pin;
- layer and width policy;
- optional waypoints;
- route mobility or transformability classification.

A route binding output should contain:

- bound source endpoint;
- bound destination endpoint;
- generated track segments;
- assigned net name and net code;
- route width and layer;
- endpoint-contact evidence;
- warnings or blocking issues.

Routes must not silently remain at stale local coordinates when a physical pad
binding is available.

### Track Emission

The first supported track emission mode may be simple and conservative:

- direct one-segment routes between two same-net pad anchors when unobstructed
  enough for the current fixture;
- orthogonal two- or three-segment routes when existing route metadata provides
  waypoints or the routing layer already has a safe Manhattan path;
- no route emitted when either endpoint is unresolved or net identity conflicts.

When waypoints are preserved from block-local coordinates, endpoint coordinates
must still be replaced by the physical pad anchors. A route that cannot be
reconciled after final placement must produce a diagnostic instead of emitting
misleading copper.

### Connectivity Evidence

Design workflow output should include route-connectivity evidence with:

- number of route bindings attempted;
- number of routes bound to physical pads;
- number of generated track segments;
- number of endpoints resolved;
- number of endpoints unresolved;
- number of endpoint-to-pad contacts proven;
- number of endpoint net mismatches;
- issue paths for unresolved endpoints or mismatches.

The evidence should be compact in CLI JSON and detailed enough in persisted
artifacts to debug fixture promotion failures.

## Validation Requirements

### Pre-Write Validation

Generated route endpoint validation must fail before final write when:

- a route references an unknown component;
- a route references an unknown pad or pin;
- a route endpoint resolves to a pad with a different net;
- a route endpoint has no hydrated geometry;
- emitted copper does not touch its declared source or destination endpoint;
- emitted copper uses a net code that does not match the endpoint pads.

### Board Validation

Existing board validation should continue to detect:

- disconnected pads;
- route endpoints that do not touch same-net pads, vias, or copper;
- partially routed nets;
- missing outlines;
- clearance or zone issues.

This project should not hide board-validation failures. It should reduce the
specific disconnected-pad and route-endpoint failures caused by stale or
unbound generated local-route coordinates.

### Optional KiCad Validation

KiCad-backed tests must remain opt-in. When `KICADAI_KICAD_CLI` is configured,
generated examples should be able to capture:

- internal writer correctness result;
- internal board-validation result;
- KiCad ERC/DRC result where available;
- readiness transition evidence for each fixture.

Promotion from `expected_fail` to `candidate` or `pass` requires evidence that
the route-connectivity blocker is closed for that fixture.

## Fixture Targets

### LED Indicator

The LED KiCad-backed generated fixture is the first target. It should progress
past the known blocker where the series route has correct net assignment but
does not physically connect same-net pads.

Success for this fixture means:

- resistor and LED pads have assigned nets;
- the generated series route starts and ends on the intended same-net pad
  anchors;
- board validation no longer reports route endpoint misses for that route;
- any remaining failure is narrower and documented.

### Connector LED

The connector/LED fixture is the second target. It may require multiple block
components and connector pad anchors to bind correctly.

Success means local routes between connector, resistor, LED, and power/ground
anchors use physical pad endpoints instead of component origins or stale local
coordinates.

### I2C Sensor Breakout

The I2C sensor breakout may remain `expected_fail` if it exposes deeper routing,
placement, footprint, or ERC/DRC issues. This work should still narrow its
failure reason by eliminating endpoint-binding problems wherever the required
pad evidence exists.

## Diagnostics

Diagnostics must be stable and actionable. They should include:

- route ID or operation ID;
- source block ID or fixture ID;
- component reference;
- pad number/name;
- net name and net code;
- expected endpoint coordinate;
- emitted track endpoint coordinate;
- mismatch distance when relevant;
- category such as missing component, missing pad, missing pad geometry, net
  mismatch, stale local route, unsupported side, or no endpoint contact.

Example issue paths:

- `design.route_connectivity.routes[status_led_series].from`
- `design.route_connectivity.routes[status_led_series].to`
- `design.route_connectivity.refs[R1].pads[2]`
- `pcb.tracks[4].start`
- `pcb.tracks[4].end`

## Acceptance Criteria

- Generated route endpoints are resolved from placed hydrated footprint pads.
- Local-route copper for supported generated fixtures starts and ends on
  same-net physical pad anchors.
- Route endpoint validation catches stale or unbound generated routes before
  they are treated as successful output.
- The LED KiCad-backed generated fixture progresses past the existing
  route-endpoint disconnected-pad blocker, or any remaining blocker is proven
  to be outside endpoint binding.
- Connector/LED route-connectivity failures are narrowed with deterministic
  evidence.
- Default tests remain KiCad-independent.
- Optional KiCad-backed tests continue to skip cleanly when KiCad is not
  configured.
- README and ROADMAP describe the new generated route-connectivity guarantee
  and the remaining blockers honestly.

## Open Questions

- Does the current placement model fully represent bottom-side footprint
  mirroring, or should bottom-side local-route binding remain unsupported until
  a dedicated mirrored-transform project?
- Should route endpoint binding live in the routing package, design workflow
  package, or a shared generated-PCB evidence package?
- Can existing board-validation endpoint tolerance be reused directly, or does
  generated endpoint binding need a tighter pre-write tolerance?
- How much waypoint preservation is required for the first fixture promotions
  versus direct pad-to-pad segment generation?
