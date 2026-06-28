# Generated Inter-Block Routing Specification

Date: 2026-06-28

## Summary

Generated design-level PCBs must route electrical connections between blocks,
not only inside individual block realizations.

The generated route-connectivity project proved that block-local routes can
bind to physical same-net pad anchors after placement. That closes a major
"looks routed but misses the pad" failure mode. The next blocker is larger:
multi-block generated designs still need automatic inter-block route planning,
route-completion evidence, and validation strong enough to promote
KiCad-backed examples from `expected_fail` toward `candidate`.

This specification focuses on the generated-design workflow path. It connects
schematic/design-level nets across placed block endpoints, emits PCB route
operations for those inter-block nets, and reports deterministic evidence about
what was routed, partially routed, or left unrouted.

## Problem

Current generated PCB routing has three different levels of evidence:

- block-local route evidence, now bound to physical pad anchors;
- placement-to-routing adapters that can build route requests from placed pads;
- board validation that can identify disconnected pads and incomplete nets.

These pieces do not yet form a reliable generated-design inter-block routing
contract. In a multi-block design such as connector/LED, the LED block may have
valid local copper while the connection from connector pads to LED block ports
remains intentionally skipped or insufficiently proven. That prevents optional
KiCad-backed design examples from being promoted.

The specific gaps are:

- inter-block design connections are not consistently transformed into routed
  PCB endpoint pairs after placement;
- generated workflow evidence does not clearly distinguish block-local route
  completion from inter-block route completion;
- route-completion validation does not yet provide a promotion-ready summary
  for expected-fail fixtures;
- skipped routing remains acceptable for some fixtures even when enough pad
  endpoint evidence exists to attempt a conservative route;
- KiCad-backed examples cannot graduate until internal route completion and
  optional ERC/DRC evidence agree.

## Goals

- Route generated inter-block nets between placed physical pad endpoints.
- Reuse existing placement, routing, pad hydration, net assignment, and route
  endpoint resolver foundations.
- Distinguish block-local routing evidence from inter-block routing evidence.
- Emit deterministic route-completion summaries for generated design workflows.
- Attempt conservative routing for connector/LED style multi-block examples
  when endpoints, layers, and board rules are available.
- Keep normal `go test ./...` KiCad-independent.
- Keep optional KiCad-backed fixture promotion evidence explicit and
  metadata-driven.
- Preserve existing validation strictness; do not hide unrouted nets or KiCad
  failures.

## Non-Goals

- Do not build a production autorouter for arbitrary boards.
- Do not guarantee every generated design is DRC-clean after this project.
- Do not weaken board validation, writer correctness, or KiCad ERC/DRC checks.
- Do not solve schematic ERC issues unrelated to PCB route completion.
- Do not route imported user projects.
- Do not implement advanced routing features such as impedance control,
  differential pairs, tuned length, or via optimization.
- Do not promote fixtures to `pass` without optional KiCad evidence.

## Current Foundations

The project should extend these existing pieces:

- generated design workflow orchestration;
- block planning and composition connections;
- schematic-to-PCB transfer and net assignment;
- resolver-backed footprint pad hydration;
- placed pad endpoint resolver;
- block-local route binding to physical pad anchors;
- placement-to-routing adapter;
- routing engine and routing diagnostics;
- board validation for disconnected pads, unrouted nets, route endpoints, and
  DRC hooks;
- writer correctness checks for PCB nets, pads, copper, and zones;
- optional KiCad-backed design examples and metadata gates.

## Required Model

### Inter-Block Route Candidate

The workflow must derive route candidates for generated design-level nets that
cross block boundaries.

Each candidate should include:

- net name and net code;
- source design connection or schematic-to-PCB transfer source;
- endpoint references and pads;
- endpoint physical coordinates and layers;
- block IDs and instance IDs for all endpoints when available;
- endpoint confidence, using `high`, `medium`, or `low`; high-confidence
  endpoints come from hydrated placed pads or declared block access points,
  medium-confidence endpoints come from validated schematic-to-PCB bindings, and
  low-confidence endpoints may only produce warnings or blocked candidates, not
  emitted route copper;
- route width/layer policy;
- routing status;
- issues when blocked.

### Access Point

An access point is a concrete same-net coordinate where an inter-block route is
allowed to meet block-local copper, a pad center, or an anchor without causing
DRC overlap ambiguity. Access points may be declared by block PCB realization
metadata, derived from placed pad endpoints, or derived from validated
block-local route endpoints. Every access point must include a ref/pad or
anchor ID, canonical net name, coordinate, layer, source, and confidence.

### Global Net Reservation

A global net reservation is a layer-aware pre-routing keepout or preferred
corridor for power/ground delivery. Reservations are derived before signal
routing from global nets, zones, anchors, and high-current constraints. They do
not require full power routing, but they give the inter-block router concrete
constraints instead of vague future corridors.

### Route Classes

Generated routing evidence must distinguish:

- `local`: block-local routes declared by block PCB realization metadata;
- `inter_block`: routes between two or more block instances;
- `anchor`: routes between a block entry anchor and an explicitly modeled
  top-level board/interface endpoint; parent/child or peer block-to-block
  connections remain `inter_block`; a net that includes both peer blocks and a
  top-level interface reports both `inter_block` and `anchor` evidence, while
  route-completion status is still judged on the unified net and summary counts
  must separate unique nets from route-class segments to avoid double counting;
- `global`: generated power/ground nets that span many pads;
- `external`: request-provided physical endpoints.

This project focuses on `inter_block`. It must not regress `local` or `anchor`
routes.

### Endpoint Selection

Inter-block routing may use:

- placed pad endpoints from the route endpoint resolver;
- placement request nets and endpoints;
- block output port-to-pin evidence;
- schematic-to-PCB transfer bindings;
- explicit request connections and aliases.

Connection aliases must be normalized to one canonical net name before
placement, routing, or writer net-code assignment. Multiple aliases for the
same connected port group collapse to a stable canonical name using explicit
request alias priority first, then the highest-priority port name when
available, then a stable identifier derived from source connection topology.
Lexical ordering is allowed only for diagnostic display, not for writer-facing
canonical net names.
Alias collisions block candidate generation only when two previously distinct
logical nets would be merged without an explicit connection; that condition is
a hard validation failure because it would create an unintended short. The
candidate builder must retain an original port-to-net identity map and alias
provenance through canonicalization so these collisions remain detectable.
Intentional alias merges require explicit request alias-map provenance or an
explicit connection joining the affected ports; matching text names alone are
not enough to merge two previously distinct nets.

If a design-level connection cannot be traced to physical pad endpoints, the
workflow must emit a deterministic blocked issue rather than fabricating a
route from component origins.

### Routing Strategy

The first implementation should be conservative:

- use same-layer direct or Manhattan routing where the existing routing engine
  can prove a path;
- prefer the placement-to-routing adapter for route search instead of adding a
  separate router;
- keep grid, width, clearance, and allow-partial behavior controlled by
  existing routing options;
- honor board outlines and keepouts during route search, and emit early
  diagnostics when constraints make a route physically impossible;
- block cross-layer inter-block routes unless the existing routing engine can
  emit vias that satisfy the active board design rules for drill diameter,
  copper diameter, annular ring, net assignment, allowed layer pair, and the
  active board layer stack, and validation can prove connectivity; blocked
  diagnostics must suggest placement changes, same-layer escapes, or manual
  routing when vias are unavailable, and may emit `needs_manual_via` partial
  evidence for visualization without writing unvalidated via copper;
- keep block-local route operations separate from inter-block generated route
  operations, with explicit same-net access points where inter-block routes may
  touch block-local copper without creating DRC overlap or stub ambiguity.

### Route Completion Evidence

`design create` routing output should expose:

- total inter-block nets considered;
- inter-block nets with enough endpoints to route;
- inter-block routes attempted;
- inter-block routes completed;
- inter-block endpoints resolved;
- inter-block endpoints unresolved;
- unrouted or partially routed inter-block nets;
- emitted track segments;
- route diagnostics count;
- issue paths for blockers.

Evidence should be stable enough for tests and concise enough for CLI JSON.

## Validation Requirements

Validation must build a unified connectivity graph across `local`,
`inter_block`, `anchor`, `global`, and `external` route evidence before judging
route completion. A route class may be reported separately, but net continuity
must be proven end-to-end across all emitted copper touching the same canonical
net.

The candidate builder must also emit global net reservations early enough to
avoid routing signal traces through expected power paths or zone corridors. Full
global routing may remain deferred, but the inter-block router must consume
these reservations and report when signal routing would make later power
delivery structurally impossible.

### Internal Validation

Generated inter-block routes must fail or warn deterministically when:

- an endpoint is missing a physical pad;
- endpoint pad net does not match the route net;
- a route cannot connect all required endpoints;
- emitted copper has missing or mismatched net codes;
- a route endpoint fails same-net contact checks;
- a board outline or keepout prevents route completion;
- route completion is partial while strict unrouted validation is requested.

### Fixture Promotion

The `connector_led_kicad_smoke` fixture is the first promotion target after
LED local-route binding. It may remain `expected_fail`, but its known gap must
narrow from local endpoint binding to inter-block route completion or KiCad
ERC/DRC evidence.

The `i2c_sensor_breakout_candidate` fixture may remain expected-fail if richer
multi-net routing, pull-ups, connector mapping, or ERC/DRC issues remain. This
project should still improve its evidence by reporting which inter-block nets
are routable, attempted, completed, or blocked.

## Diagnostics

Diagnostics should identify:

- route class;
- net name;
- block instance IDs;
- component references;
- pads/pins;
- endpoint coordinates and layers where available;
- routing status;
- route operation IDs where available;
- validation issue code and suggested next action.

Example paths:

- `design.interblock_routing.nets.LED_EN`
- `design.interblock_routing.routes.header.SIG.to.status.IN`
- `design.interblock_routing.endpoints.J1.1`
- `design.interblock_routing.nets.GND.partial`

## Acceptance Criteria

- Generated workflow derives inter-block route candidates from design
  connections and placed physical pad endpoints.
- Connector/LED fixture attempts inter-block routing when routing is not
  skipped and endpoint evidence exists.
- Routing stage output includes deterministic inter-block route completion
  evidence.
- Board validation failures for inter-block nets become narrower and more
  actionable.
- Existing block-local endpoint binding tests continue to pass.
- Default tests remain KiCad-independent.
- Optional KiCad-backed tests continue to skip cleanly unless configured.
- ROADMAP and README clearly state whether connector/LED has progressed to
  candidate or remains expected-fail with a narrower blocker.

## Open Questions

- Should `skip_routing` in existing expected-fail fixtures remain a fixture
  policy until promotion, or should tests add a second non-skipped request path
  for inter-block routing proof?
- Should global power/ground nets be fully routed in this project or should this
  project only reserve and diagnose likely power/ground delivery corridors until
  a dedicated power-routing/zone project?
- Should the first inter-block route-completion summary count nets or endpoint
  pairs as the primary unit?
- How much KiCad DRC evidence is required before promoting connector/LED from
  `expected_fail` to `candidate`?
