# Routing Engine Hardening Specification

## Purpose

Harden the existing deterministic routing engine so generated PCB routes are
electrically meaningful, DRC-aware, repairable, and suitable for AI-generated
design workflows.

The current router can build requests from placed boards, plan deterministic
net order, resolve pad access, route small single-layer and two-layer nets,
validate connectivity and clearance, and emit route transaction operations.
This project raises that foundation from "can route simple nets" to "can
explain routing quality, enforce richer electrical rules, and drive repair
loops when routing fails."

## Goals

1. Add explicit routing rule intent for net classes, net roles, layer policy,
   via policy, and high-current/analog/signal behavior.
2. Make route quality inspectable through per-net reports, route metrics,
   failure categories, and structured diagnostics.
3. Improve obstacle and clearance handling around footprints, keepouts, board
   edges, existing copper, vias, and zones.
4. Strengthen two-layer routing with via limits, layer preferences, and
   return-path awareness where possible.
5. Add first-class handling for routed power, ground, analog, clock, and
   high-current nets.
6. Add the foundation for differential-pair and length-matching constraints,
   even if full tuning remains a later capability.
7. Feed DRC, connectivity, and routing failures back into repair workflows with
   specific actions.
8. Add golden route corpus coverage for representative circuit blocks.
9. Preserve deterministic output and compatibility with existing transaction,
   PCB writer, validation, and design workflow code.

Shared routing and PCB writing constraints must have one source of truth in
`internal/pcbrules`. The router may keep adapter-local helpers, but net class,
layer, trace, via, clearance, zone, and length policy definitions must not fork
between route search and PCB writer output.

## Non-Goals

- Replacing KiCad's interactive push-and-shove router.
- Dense BGA breakout or advanced escape routing.
- RF layout synthesis.
- Full impedance calculation.
- Full differential-pair tuning in this hardening pass.
- Thermal simulation.
- Arbitrary polygonal zone refill.
- Mutating imported/user-authored copper without explicit preservation support.

## Existing Foundation

The implementation already includes:

- `internal/routing` request/result models.
- Board, layer, component, pad, net, existing copper, obstacle, rule, and
  strategy models.
- Single-layer and two-layer routing modes.
- Grid conversion and geometry helpers.
- Pad access and route planning.
- Obstacle occupancy from board geometry, pads, keepouts, existing copper, and
  vias.
- A deterministic grid search router.
- Segment and via construction.
- Route validation for endpoint connectivity and clearance.
- Route transaction operation output.
- Route diagnostics and design workflow route integration.
- Routing adapter from placement output.
- Golden and performance tests for simple route examples.

This hardening work must extend that package rather than introduce a parallel
router or bypass the transaction/writer pipeline.

## Design Principles

- Keep routing deterministic for identical requests.
- Prefer explicit blocked/warning issues over silent fallback behavior.
- Treat connectivity and DRC-relevant geometry as hard gates.
- Keep route reports compact but actionable for AI agents.
- Separate route planning quality from final KiCad DRC evidence.
- Preserve existing public behavior unless a phase explicitly changes it.
- Keep all generated copper attributable to route operations and nets.

## Routing Rule Model

Routing rules must support both global defaults and per-net overrides.

Required model concepts:

- Net class:
  - trace width;
  - clearance;
  - class-to-class clearance matrix entries;
  - via diameter;
  - via drill;
  - via clearance;
  - neckdown width and neckdown length near constrained pads;
  - allowed layers;
  - preferred layer;
  - max vias;
  - max length or warning length;
  - role/policy tag.
- Net role:
  - power;
  - ground;
  - signal;
  - clock;
  - analog;
  - high-current;
  - differential;
  - unknown.
- Route policy:
  - allow or forbid vias;
  - preserve existing copper;
  - allow partial routes;
  - treat zones as ignored, obstacles, or unsupported blockers;
  - allow rerouting of generated copper only when requested.

Rule resolution order:

1. Explicit net rule overrides.
2. Net class rules.
3. Role-derived policy.
4. Global rules.
5. System defaults.

Rules resolve per attribute. If a higher-precedence level specifies trace
width but leaves max vias unset, max vias must continue falling through role,
global, and system defaults rather than treating the whole higher-precedence
rule object as complete.

Rule structs in `internal/pcbrules` must represent unset values explicitly, for
example with pointer fields or an equivalent optional type. Numeric zero must
never be overloaded to mean both "unset" and "intentionally set to zero."

Physical footprint and pad constraints are hard gates rather than ordinary
defaults. They must be checked after logical rule resolution and may force a
neckdown, route failure, or repair diagnostic when the resolved trace/via rules
cannot physically enter a pad or escape a footprint.

`System defaults` means software fallback values shipped by this project, not
footprint/package-specific constraints.

Invalid or inconsistent rules must produce structured issues before route
search begins.

## DRC-Aware Routing Quality

The router must report why a route is acceptable, weak, or failed.

Per-net route reports should include:

- net name;
- role;
- class;
- endpoint count;
- routed endpoint count;
- route status;
- segment count;
- via count;
- total length;
- layer usage;
- max clearance pressure;
- search nodes;
- search limit hit;
- failure category;
- suggested repair action.

Route quality dimensions should include:

- route completion;
- endpoint connectivity;
- clearance;
- via policy;
- layer policy;
- route length;
- search pressure;
- obstacle pressure;
- DRC evidence when available.

Search pressure is measured as visited search nodes divided by available grid
nodes for the active layer set, with an optional secondary nodes-per-millimeter
value for route reports where routed length is known.

The route result should keep existing `Metrics` stable while adding richer
quality/report fields where needed.

## Obstacle And Clearance Hardening

Obstacle handling must become explicit enough for DRC-oriented diagnostics.

Required obstacle classes:

- board edge and margin;
- other-net pads;
- same-net pad body with access exceptions;
- keepouts;
- mechanical holes;
- other-net existing copper;
- same-net existing copper as a connection target or valid traversal area when
  continuity evidence exists;
- generated previous routes;
- vias and via keepouts;
- zones according to configured zone policy.

Clearance behavior:

- Resolve clearance from the class-to-class clearance matrix when present,
  otherwise use the stricter value from the interacting net rules.
- Inflate zero-width or boundary obstacles by
  `(current_trace_width / 2) + resolved_inter_net_clearance`.
- Inflate trace-like obstacles by
  `(current_trace_width / 2) + (obstacle_trace_width / 2) +
  resolved_inter_net_clearance`.
- Apply per-net-class clearance overrides.
- Apply pad-specific clearance overrides when present.
- Permit rule-bounded neckdowns near pads when the resolved trace width cannot
  physically enter the pad area.
- Define `near pad` neckdown geometry as the bounded segment from the pad
  access point through the configured `neckdown_length`, unless full trace
  width is validated earlier against all nearby obstacle clearances.
- Allow the shoulder from neckdown width back to full width only after
  validating the wider geometry against nearby obstacle clearance.
- Track the nearest or blocking obstacle when search fails.
- Emit diagnostics naming the obstacle kind/source when available.

## Net Roles And Policies

Routing behavior must be role-aware.

Initial role policies:

- Power/high-current:
  - wider trace defaults or required net class;
  - lower via preference;
  - shorter route warning thresholds;
  - stronger failure diagnostics when not routed.
- Ground:
  - permit zone/plane assumptions only when explicit;
  - otherwise route like power with clear warnings.
- Analog:
  - prefer analog region/layer policy when provided;
  - avoid noisy/clock nets where obstacle metadata exists.
- Clock:
  - warn on long detours or excessive vias.
- Signal:
  - use default routing rules.
- Differential:
  - validate pair metadata and report unsupported tuning until implemented.

## Two-Layer And Via Policy

Two-layer routing must be more explainable.

Requirements:

- Respect `AllowVias`, `AllowBackLayer`, `MaxViasPerNet`, and allowed layers.
- Report via count per net.
- Report when a route only succeeds by violating preferred layer policy.
- Prefer fewer vias when path cost ties.
- Treat through-hole pads as valid cross-layer access.
- Fail with actionable diagnostics when vias are forbidden but required.

Blind and buried vias are out of scope for this hardening pass. The rule model
may reserve fields for future layer-pair transitions, but implemented routing
must treat supported vias as through-vias only and reject unsupported transition
pairs with structured diagnostics.

## Differential Pair And Length Matching Foundation

Full differential routing is out of scope, but the model must stop treating
differential or coupled-net intent as ordinary anonymous nets.

Requirements:

- Add a coupled-net constraint model or reserved rule extension.
- Default `DifferentialPair` groups must have exactly two member nets.
- Reserve a separate coupled-group mode for future N-member interfaces rather
  than overloading differential-pair semantics.
- Validate pair polarity through explicit metadata or complementary naming such
  as `_P`/`_N`, `+`/`-`, or another configured suffix pair.
- Validate that pair members exist and have comparable endpoint counts.
- Report unsupported pair routing as a structured blocked or warning issue
  depending on policy.
- Add route report fields for length and pair group IDs.
- Do not falsely claim pair tuning is complete.

Length matching foundation:

- Add optional target/max length fields.
- Report measured route length.
- Warn or fail when length exceeds the configured tolerance.
- Defer meander generation.

## Zone And Copper Pour Policy

Zones currently remain under-modeled. Hardening must make this explicit.

Required behavior:

- `ZoneIgnore`: route without considering zones and report that zones were
  ignored when zones exist.
- `ZoneObstacle`: treat zones as obstacles.
- `ZoneUnsupported`: block routing when zones are present.
- Ground/power zone assumptions must be explicit; the router must not assume a
  copper pour electrically completes a net unless a validation adapter proves
  it.
- `ZoneSufficient`: when validation evidence proves a zone already completes a
  specific net segment, skip redundant trace generation for that satisfied
  segment and report the evidence. Without that evidence, continue treating the
  segment as unrouted.

Zone connectivity evidence must be stronger than overlap and must be strictly
conservative. `ZoneSufficient` is optional and must stay disabled unless the
proof engine can prove the specific connection points involved. A proof may
skip trace generation only when cached same-net connectivity evidence accounts
for minimum copper width along the whole path, class clearances, islands,
fragmentation, pad connection style, and point-specific thermal-relief or solid
connection geometry. Any unsupported constraint or simplification must be
pessimistic; uncertainty must produce `unrouted` or `blocked`, never
`satisfied`. Raw flood fill or A* can be used to build the cache, but route
checks should query cached evidence rather than repeating full geometric search
for every segment.

## DRC And Validation Feedback Integration

Routing hardening must feed repair and validation loops.

Required diagnostics:

- disconnected route;
- failed endpoint access;
- no path found;
- search budget exhausted;
- clearance violation;
- via policy violation;
- layer policy violation;
- excessive length;
- too many vias;
- zone policy blocker;
- unsupported differential pair;
- DRC finding mapped to route, net, or operation where possible.

Each diagnostic should include:

- category;
- action;
- severity;
- message;
- net names;
- refs when available;
- operation IDs when available;
- suggested repair.

Repair actions should include:

- move components closer;
- rerun placement with routing-readiness feedback;
- widen board or relax keepouts;
- increase route search budget;
- allow vias or back layer;
- assign a net class;
- widen trace or increase clearance;
- split route into generated reroute transaction;
- run KiCad DRC after route changes.

## Golden Corpus

Add route hardening golden coverage for:

- LED indicator;
- voltage regulator;
- MCU minimal power/reset/programming local nets;
- USB-C power entry;
- I2C sensor SDA/SCL/power;
- op-amp feedback network;
- connector breakout;
- forced single-layer failure;
- two-layer via success;
- keepout detour;
- zone policy behavior.

Golden tests should compare compact summaries:

- route status;
- routed/failed/skipped net counts;
- route report statuses;
- segment/via counts;
- route lengths where deterministic;
- diagnostics categories/actions;
- transaction operation counts.

Avoid brittle full JSON route comparisons unless the shape is intentionally
locked.

## CLI And Workflow Integration

The existing `route request` CLI should expose new rule and reporting fields
through JSON requests/results without adding a second command.

Design workflow integration should:

- pass placement quality/routing-readiness context into routing where useful;
- return routing quality reports in the routing stage summary or artifacts;
- include repairable routing diagnostics in `design create` feedback;
- keep route operations operation-correlated for writer and repair loops.

Any route-triggered placement retry must be bounded by an explicit attempt
limit and convergence check. If route quality or blocking diagnostics do not
improve after a retry, the workflow must stop with a structured blocked result
rather than cycling between placement and routing.

## Acceptance Criteria

- Existing routing tests continue to pass.
- New rule models validate deterministically.
- Per-net route reports identify weak and failed routes.
- Role and net class policies affect routing rules.
- Two-layer via policy failures are explainable.
- Zone policy is explicit and tested.
- Differential-pair intent is validated and reported even before full pair
  routing exists.
- Route diagnostics are repairable and include nets/refs where available.
- Golden routing corpus covers representative block-shaped boards.
- Full `go test ./...` passes.

## Open Questions

- Should route quality reports live directly on `routing.Result` or under a
  nested `QualityReport`?
- What threshold should distinguish warning versus failure for route length and
  via count when no explicit policy is provided?
- How much KiCad DRC parser mapping should this project include before routing
  repair is considered complete?
