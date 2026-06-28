# Inter-Block Endpoint Contact Completion Specification

Date: 2026-06-28

## Summary

Generated inter-block routes must be judged complete only when emitted copper
physically contacts the intended same-net pads or validated access points.

The generated inter-block routing work now discovers design-level route
candidates, promotes request connections into placement nets, attempts routes,
and reports route evidence. The remaining blocker is stricter and physical:
some generated routes are attempted and emitted, but board validation still
reports disconnected pads because the track endpoint does not create a proven
same-net contact with the target pad or access point.

This specification closes that gap. It defines the model, validation, evidence,
and fixture promotion requirements for endpoint-contact-correct inter-block
routing.

## Problem

The current workflow can produce inter-block route evidence that says a route
was attempted, and it can emit route operations for nets such as `LED_EN`.
However, generated boards may still fail connectivity-first validation with
issues such as `DISCONNECTED_PAD`.

That means the generated PCB has crossed an important threshold but is not yet
electrically trustworthy:

- route candidates have enough endpoint evidence to attempt routing;
- route operations can be emitted with the intended net;
- the board may visually contain plausible copper;
- validation still cannot prove every intended pad is connected by same-net
  copper.

For AI-generated boards, this is not acceptable. The workflow must not equate
"route emitted" with "route complete." It must prove physical contact or report
the exact contact gap.

## Goals

- Make inter-block route completion depend on physical same-net endpoint
  contact, not only route operation emission.
- Reuse the existing placed pad endpoint resolver, block-local access point
  evidence, routing engine, net assignment, and board validation foundations.
- Add a deterministic contact proof model for route endpoints, pads, vias,
  access points, and same-net copper.
- Feed contact-proof failures back into inter-block route evidence and repair
  diagnostics.
- Narrow connector/LED failures from generic partial routing to exact
  contact-miss causes, or promote the fixture when all supported contacts are
  proven.
- Add regression coverage for direct pad-to-pad, pad-to-access-point, and
  contact-miss cases.
- Keep default `go test ./...` independent of a local KiCad installation.

## Non-Goals

- Do not build a production autorouter.
- Do not guarantee all generated boards are DRC-clean.
- Do not solve global power routing, zones, or plane connectivity unless they
  directly affect endpoint contact classification.
- Do not weaken disconnected-pad, unrouted-net, route-completion, writer
  correctness, or optional KiCad checks.
- Do not route imported user projects.
- Do not infer unsafe contacts from visual proximity when pad geometry or net
  identity is missing.
- Do not promote KiCad-backed fixtures to `pass` without optional KiCad
  evidence where fixture policy requires it.

## Current Foundations

This project should extend existing code rather than introduce a parallel
connectivity system:

- generated design workflow orchestration;
- request-level inter-block connection promotion into placement nets;
- inter-block route candidate discovery;
- placed pad endpoint resolution;
- block fragment port endpoint evidence;
- block-local route binding to physical same-net pad anchors;
- PCB transaction route operations;
- writer net assignment for pads, tracks, vias, and zones;
- board validation for disconnected pads and route completion;
- routing stage inter-block summaries;
- optional KiCad-backed fixture metadata under `examples/design/kicad-backed/`;
- operation-correlated diagnostics and repair planning.

## Required Model

### Contact Target

A contact target is a physical location and copper object that an inter-block
route endpoint is allowed to touch.

Each target must include:

- canonical net name and net code;
- target kind: `pad`, `access_point`, `via`, `track_endpoint`, or
  `same_net_copper`;
- component reference and pad number/name when target kind is `pad`;
- block ID and instance ID when known;
- coordinate and copper layer;
- contact radius or tolerance source;
- geometry source: hydrated footprint pad, declared access point, generated
  route endpoint, or validated same-net copper;
- confidence: `high`, `medium`, or `blocked`;
- diagnostic path when unresolved.

Pad targets should use hydrated footprint pad geometry. Access point targets
must be traceable to block-local route evidence, entry-anchor evidence, or a
declared safe connection point. Component origins must not be used as contact
targets unless a future model explicitly proves that the origin is a copper
contact, which is not part of this project.

### Contact Proof

A contact proof records whether an emitted route endpoint physically contacts a
target.

Each proof must include:

- route operation ID;
- route class, initially `inter_block`;
- net name and net code;
- endpoint side: `start`, `end`, or `intermediate`;
- emitted coordinate and layer;
- target coordinate and layer;
- target identity;
- distance or overlap metric;
- tolerance used;
- status: `proven`, `miss`, `net_mismatch`, `layer_mismatch`,
  `missing_target`, `unsupported_geometry`, or `ambiguous`;
- blocking flag;
- suggested repair action when contact is not proven.

A route is complete only when all required endpoint contacts are `proven` and
the resulting same-net connectivity graph connects the intended endpoint group.

### Same-Net Connectivity Graph

Inter-block route validation must build a graph that includes:

- same-net pads;
- emitted inter-block tracks;
- emitted block-local tracks;
- vias when supported by the current layer stack and writer;
- validated access points;
- zones only when the current validation policy has explicit zone-sufficient
  proof.

The graph must not join objects across different nets. It must not join objects
on different layers unless a valid via or same-layer policy proves the
transition. It must not let a visually close but non-touching segment satisfy
connectivity.

### Completion Status

Inter-block route summaries must distinguish:

- `candidate`: route has enough endpoint evidence to attempt;
- `attempted`: routing emitted at least one operation;
- `contact_proven`: all required route endpoint contacts are proven;
- `connected`: graph connectivity proves all required endpoints are connected;
- `partial`: some endpoints or contacts are proven, but not all required
  endpoints connect;
- `blocked`: missing data, unsafe geometry, obstacle, rule, or validation
  failure prevents completion;
- `skipped`: fixture or request policy intentionally disabled routing.

Existing `routes_completed` counts should mean `connected`, not merely
`attempted`.

## Required Behavior

### Route Emission

Before a route operation is counted as complete:

- start and end coordinates must be snapped or generated from contact targets;
- track net name and net code must match the target net;
- the layer must match the target layer or include a valid via transition;
- the route must not terminate at a component origin when a pad/access target is
  available;
- route geometry must preserve board outline and obstacle constraints;
- endpoint contact proofs must be generated immediately after route emission.

If the router cannot terminate exactly on the target, it may emit a partial
route only when evidence marks it as partial and validation does not treat it as
complete.

### Access Points

Block-local routes may expose access points for inter-block routing. Access
points are valid only when they are:

- on the same canonical net as the inter-block route;
- on a layer that the route can reach;
- traceable to a physical pad, validated same-net copper endpoint, or declared
  block interface anchor;
- not inside a keepout or ambiguous overlap region;
- reported in workflow evidence.

### Validation

Pre-write and post-write validation must report contact issues with stable
codes. Required codes include:

- `ROUTE_CONTACT_MISSING_TARGET`;
- `ROUTE_CONTACT_NET_MISMATCH`;
- `ROUTE_CONTACT_LAYER_MISMATCH`;
- `ROUTE_CONTACT_MISS`;
- `ROUTE_CONTACT_AMBIGUOUS`;
- `ROUTE_CONTACT_UNSUPPORTED_GEOMETRY`;
- `ROUTE_GRAPH_INCOMPLETE`;
- `ROUTE_COMPLETION_PARTIAL`.

Existing board validation issue codes can remain, but inter-block routing
evidence should map them back to route operations and contact targets where
possible.

### Repair Hints

Contact failures should produce deterministic repair hints:

- snap route endpoint to resolved pad center;
- regenerate route from nearest valid access point;
- increase endpoint tolerance only when writer and validation tolerances prove
  the current threshold is too strict;
- move placement to reduce route distance or avoid obstacle;
- insert via only when via policy and validation support it;
- block and request manual routing when geometry is unsupported.

The first implementation may only produce hints. It does not need to execute a
new repair pass unless existing generated-repair infrastructure can safely
apply the fix.

## Fixture Targets

### Connector LED

Connector/LED is the primary target. The workflow should prove whether the
promoted `LED_EN` inter-block route physically contacts both required endpoint
targets.

Success means:

- route candidates still resolve the connector and LED-side endpoints;
- routing emits same-net copper for the intended connection;
- contact proofs are `proven` for the route start and end;
- graph connectivity marks the net connected when all required endpoints are
  joined;
- board validation no longer reports `DISCONNECTED_PAD` for the supported
  `LED_EN` path, or the remaining issue is mapped to a narrower non-contact
  blocker.

### I2C Sensor Breakout

I2C sensor breakout remains a broader generated-board fixture. This project
should add contact proof evidence for VCC, GND, SDA, and SCL candidates where
routes are attempted. It may remain `expected_fail` if global power routing,
multi-net routing, placement, or KiCad DRC issues remain.

### LED Indicator

LED indicator should remain covered as a block-local route endpoint regression.
This project must not regress the already-proven local endpoint binding path.

## Evidence Output

`design create` routing output should include a compact inter-block contact
summary:

- `contacts_required`;
- `contacts_proven`;
- `contacts_failed`;
- `contact_misses`;
- `net_mismatches`;
- `layer_mismatches`;
- `missing_targets`;
- `routes_connected`;
- `routes_partial`;
- `routes_blocked`;
- issue count;
- selected issue paths.

Persisted artifacts may include detailed per-contact proofs for debugging and
fixture promotion.

## Acceptance Criteria

- Inter-block route completion means same-net graph connectivity, not just
  route operation emission.
- Contact targets are resolved from physical pads or validated access points.
- Contact proofs are generated for attempted inter-block routes.
- `routes_completed` or equivalent completion counts only include graph-proven
  routes.
- Connector/LED has deterministic regression coverage for the current
  `LED_EN` contact blocker and its resolution or narrowed failure category.
- I2C sensor breakout reports route-contact evidence for attempted candidates
  without requiring KiCad.
- Default tests remain KiCad-independent.
- Optional KiCad-backed fixtures skip cleanly when `KICADAI_KICAD_CLI` is not
  configured and provide promotion evidence when it is.
- README and ROADMAP describe the new route-contact guarantee and remaining
  blockers honestly.

## Open Questions

- Should the contact graph live in board validation, routing evidence, or a
  shared package consumed by both?
- Should route endpoints snap to pad centers, pad edges, or declared access
  points when all are available?
- What tolerance should be canonical for writer/reader round-trip coordinates:
  board-validation tolerance, routing-grid tolerance, or a stricter
  endpoint-contact tolerance?
- Should same-net copper intersections count as contact for generated
  inter-block routes before full DRC overlap checks are KiCad-backed?
- How much of the contact repair hint model should be executable in this phase
  versus evidence-only?
