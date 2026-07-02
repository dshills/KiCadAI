# Component Hint Enforcement Specification

Date: 2026-07-02

## Summary

KiCadAI component records already carry `placement_hints` and `routing_hints`,
but most downstream systems still treat them as catalog metadata rather than
enforceable design constraints. The next project converts supported component
hints into placement, routing, validation, workflow, and AI-facing evidence.

The goal is not to build a general constraint solver. The goal is to make the
first supported hint kinds real: nearby companion placement, edge/access
placement intent, net-class width hints, and role-specific routing hints should
affect generated PCB transactions and produce deterministic diagnostics when
they cannot be satisfied.

## Problem Statement

Catalog records can describe local layout expectations such as:

- regulator input/output capacitors near the regulator;
- power nets needing wider traces;
- connectors preferring edge-facing placement;
- protection parts near external entry points;
- timing/decoupling parts near the active component they support.

Today those hints are useful for humans and future code, but they are not
consistently projected into:

- placement request constraints;
- route width or net-class choices;
- block-local or inter-block route planning;
- post-placement quality checks;
- workflow summaries and rationale output.

As a result, AI-generated designs can select a correct component but fail to
honor the selected component's own layout intent. That gap matters especially
for regulators, protection networks, oscillators, connectors, and amplifier
blocks where placement and routing are part of correctness.

## Goals

- Define a supported subset of component hint kinds that downstream code must
  consume.
- Translate selected component `placement_hints` into placement constraints or
  quality checks where enough references exist.
- Translate selected component `routing_hints` into route/net-class
  requirements where enough net-role evidence exists.
- Preserve unsupported hint kinds as explicit warnings rather than silently
  ignoring them.
- Surface enforced, skipped, and unsupported hint evidence in:
  - component-selection workflow summaries;
  - placement/routing stage summaries;
  - validation issues;
  - rationale evidence for AI callers.
- Keep behavior deterministic and hermetic.
- Avoid claiming fabrication readiness from hints alone. Hints improve layout
  intent enforcement; they do not replace ERC, DRC, thermal, SI, or fabrication
  evidence.

## Non-Goals

- Do not implement a full constraint solver.
- Do not infer undocumented component requirements from package names or free
  text.
- Do not make unsupported hints mutate placement or routing.
- Do not require KiCad CLI for default tests.
- Do not treat catalog hints as stronger than explicit request overrides,
  locked placement, fixed board-edge bindings, or block-required routes.
- Do not solve high-speed differential routing, impedance control, or thermal
  copper synthesis in this slice.

## Supported Hint Subset

### Placement Hints

The initial enforceable placement hint kinds are:

- `near`
  - Meaning: place the selected component near a target component role, pin
    role, or selected companion.
  - Typical use: regulator capacitors near regulator pins; decoupling
    capacitors near MCU/regulator/oscillator supply pins.
  - Required fields: `target`; optional numeric `value` plus `unit`.
  - Behavior: translate to a proximity constraint when both selected refs are
    known; otherwise emit a warning with a skipped reason.

- `edge`
  - Meaning: prefer or require placement close to the board edge or an external
    entry anchor.
  - Typical use: connectors, USB-C power input, user-facing headers.
  - Required fields: `target` or an implied selected component role.
  - Behavior: translate to an edge/accessibility placement preference where the
    placement model supports it; otherwise emit advisory evidence.

- `keepout`
  - Meaning: selected component should avoid a named area or other component
    role.
  - Typical use: antenna/protection/interface spacing.
  - Behavior: first implementation may validate and report the hint without
    moving components unless the placement model already supports equivalent
    keepout constraints.

### Routing Hints

The initial enforceable routing hint kinds are:

- `net_class`
  - Meaning: nets matching the hint's `net_role` should use at least the
    requested width.
  - Required fields: `net_role`, `value`, and `unit`.
  - Behavior: translate to route width requirements or net-class evidence when
    net-role mapping is known; warn when no matching net role is known.

- `tie`
  - Meaning: route or net assignment should tie a role-specific control net to
    another role, such as AP2112K `EN` to `VIN`.
  - Behavior: this remains block-owned when the block already emits the
    connection; component hint enforcement should record evidence that the tie
    was satisfied, not duplicate the operation.

- `no_connect`
  - Meaning: a pin or net role must be explicitly no-connected.
  - Behavior: this remains block-owned when the block emits a KiCad no-connect;
    component hint enforcement records evidence or reports missing evidence.

Unsupported hint kinds must be preserved in evidence as unsupported. They must
not silently affect placement or routing.

## Data Flow

1. Component selection chooses concrete records and includes selected
   `placement_hints` and `routing_hints` in `ComponentSelectionEntry`.
2. The workflow maps selected component roles to schematic refs, PCB refs,
   block component roles, and known net roles.
3. A hint normalization stage converts raw catalog hints into workflow-local
   `ComponentHintEvidence` records:
   - selected component ID;
   - instance ID;
   - component role;
   - hint kind;
   - target role/net role;
   - requested value/unit;
   - enforcement status.
4. Placement consumes eligible placement hints and records whether each hint was
   enforced, skipped, unsupported, or contradicted by fixed placement.
5. Routing consumes eligible routing hints and records whether each hint was
   enforced, skipped, unsupported, or contradicted by route constraints.
6. Validation checks measurable hints after placement/routing:
   - proximity distances;
   - edge/access hints when measurable;
   - route width/net-role hints when route evidence exists;
   - explicit tie/no-connect evidence where available.

## Enforcement Status

Every selected hint should end with one of these statuses:

- `enforced`
- `satisfied_by_block`
- `validated`
- `skipped_missing_ref`
- `skipped_missing_net_role`
- `skipped_locked_placement`
- `unsupported`
- `conflict`
- `failed`

Connectivity-oriented workflows may warn on `skipped_*`, `unsupported`, or
`failed` statuses unless the hint is required by a block rule. ERC/DRC and
fabrication-candidate workflows should block only when the hint is explicitly
required for the selected block/component or when violating it would invalidate
already-declared readiness.

## Diagnostics

Diagnostics must include:

- component ID;
- selected ref when known;
- component role;
- hint kind;
- target role or net role;
- requested value/unit;
- observed value when measurable;
- status;
- actionable suggestion.

Examples:

- `component hint skipped: regulator output_capacitor near regulator has no PCB ref`
- `component hint failed: output_capacitor distance 12.4mm exceeds 3mm near hint`
- `component routing hint enforced: power net width >= 0.5mm`
- `component routing hint unsupported: net_role differential_pair`

## AI-Agent Expectations

After this project, an AI agent should be able to answer:

- which selected components contributed placement or routing hints;
- which hints changed placement or routing;
- which hints were satisfied by existing block operations;
- which hints were skipped and why;
- whether unsatisfied hints are warnings or blockers;
- what request, catalog, or block changes would satisfy the remaining hints.

Agents must not claim a board is fabrication-ready only because hints were
enforced. Hints are supporting layout evidence.

## Acceptance Criteria

- Selected component entries expose placement and routing hints.
- Supported `near` placement hints are converted into measurable proximity
  constraints or validation checks.
- Supported `net_class` routing hints are converted into route-width/net-role
  evidence where net roles are known.
- AP2112K `tie` and `no_connect` routing hints are reported as
  `satisfied_by_block` when the voltage regulator block emits EN-to-VIN and NC
  no-connect operations.
- Unsupported hints are visible in workflow/rationale evidence and do not
  mutate placement/routing.
- Existing generated design workflows remain deterministic.
- `go test ./...` passes without requiring KiCad CLI.

## Risks

- Hints may be too vague to enforce without role/ref mapping. The
  implementation must prefer explicit skipped evidence over guessing.
- Placement and routing models already have block-local constraints. Component
  hints must not duplicate or fight those constraints.
- A warning-only first pass may feel weak, but it is safer than silently moving
  components or overclaiming route quality.
- Later fabrication readiness will need stronger proof than hint satisfaction.
