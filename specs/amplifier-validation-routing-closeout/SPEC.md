# Amplifier Validation and Routing Policy Closeout Specification

## Objective

Promote the protected Class AB headphone amplifier fixture past writer-level
structural validation blockers that are caused by generated schematic labels,
route policy, or incomplete generated PCB connectivity, so real KiCad ERC/DRC
evidence can run.

The immediate target fixture is:

- `examples/design/kicad-backed/class_ab_headphone_protected.json`
- `examples/design/kicad-backed/class_ab_headphone_protected.metadata.json`

This fixture already proves:

- verified LMV321 op-amp selection;
- verified MMBT3904/MMBT3906 complementary output-device selection;
- diode-string Class AB biasing;
- `headphone_output_protection` participation;
- schematic electrical validation after alias cleanup;
- PCB realization;
- placement;
- endpoint binding;
- project writing;
- writer correctness.

The current blocker is structural validation before KiCad ERC/DRC. The known
failure family is:

- generated schematic label/connectivity evidence that does not represent the
  intended electrical topology cleanly;
- unrouted or partially routed PCB nets that prevent the board from proving
  same-net pad connectivity;
- route-policy metadata that may intentionally skip routing even though the
  fixture now has enough placement and endpoint evidence to attempt routing.

## Scope

In scope:

- capture the current protected amplifier structural validation baseline;
- classify schematic label/connectivity failures into writer bugs, fixture
  topology gaps, generated decorative-label issues, or valid ERC-level
  problems;
- ensure generated schematic labels attach to real wire endpoints and do not
  create false net aliases, floating labels, or misleading label/segment
  conflicts;
- enable routing for the protected amplifier fixture only when route requests
  include concrete pads, endpoints, and required-net classification;
- classify every required amplifier PCB net as routed, locally connected,
  intentionally external, explicitly allowed unrouted, or blocking;
- preserve the existing fail-closed behavior for unsupported amplifier loads,
  unsafe power requests, unknown output devices, and missing protection;
- update metadata and docs so the fixture's first blocker is current and
  actionable.

Out of scope:

- production-ready analog layout;
- full automatic amplifier board optimization;
- active output short-circuit or DC-fault protection;
- speaker, bridged, high-voltage, or power-amplifier support;
- thermal, SOA, EMC, crosstalk, or stability signoff;
- guaranteeing clean KiCad ERC/DRC after structural blockers are removed;
- replacing the general autorouter.

## Current Failure Model

The protected amplifier fixture must remain useful while it is still
`expected_fail`. A useful failure is one that reaches the deepest supported
stage and reports the next real engineering blocker.

Before this project, the fixture reaches structural validation and reports a
mix of schematic label/connectivity and PCB route-completion blockers. That is
too broad for the next AI-controlled repair loop because it is not clear
whether the next action should be schematic cleanup, route enablement, route
repair, or KiCad ERC/DRC inspection.

After this project, structural validation must separate these cases:

- **Generated schematic artifact**: labels, wire endpoints, or aliases emitted
  by KiCadAI are malformed or misleading. This is a writer/generator bug and
  must block before KiCad.
- **Unsupported topology**: the design asks for a net relationship the current
  amplifier blocks cannot model safely. This must block with a precise design
  limitation.
- **Route-policy skipped**: routing was intentionally disabled or gated. This
  must be explicit metadata, not an unlabeled structural connectivity failure.
- **Required net unrouted**: routing ran or was required, but same-net pads are
  not connected by copper. This must identify the net, pads, and route evidence.
- **KiCad-ready candidate**: writer-level structural evidence is clean enough
  that optional KiCad ERC/DRC can be the next authority.

## Functional Requirements

### FR1: Baseline and Classification

The protected amplifier fixture must have a reproducible baseline that captures
the current structural blockers.

Requirements:

- baseline capture must run without requiring KiCad CLI;
- issue extraction must normalize timestamps and absolute output paths;
- evidence must include stage status, issue code/path, affected net, affected
  symbol or footprint when available, and whether the issue is schematic, PCB,
  routing, writer, or KiCad-backed;
- tests must fail if the fixture regresses to older placement, endpoint,
  project-write, or writer-correctness blockers.

### FR2: Schematic Label and Connectivity Cleanup

Generated amplifier schematics must use labels only when they improve
readability and preserve KiCad-native connectivity.

Requirements:

- labels must attach to a wire endpoint or wire segment using coordinates that
  KiCad resolves as connected;
- labels must not be emitted as decorative text or near-net annotations when
  they are intended to create connectivity;
- a label must not bridge two canonical nets unless the design explicitly
  models a net-tie, star point, or alias-equivalent net;
- generated port labels for amplifier nets must preserve canonical names such
  as `AUDIO_IN`, `DRIVER_OUT`, `BIAS_P`, `BIAS_N`, `AMP_OUT_DC_BIASED`,
  `HP_OUT`, `HP_RET`, `LOAD_REF`, `VCC`, `GND`, and `VEE`;
- local helper nets may remain local only when they do not cross a block
  boundary;
- validation diagnostics must identify the label text, coordinate, nearest
  wire endpoint, and expected net when a future label fails to attach.

### FR3: Route Policy Enablement

The protected amplifier fixture must no longer skip routing by stale metadata
once placement and endpoint evidence are available.

Requirements:

- fixture metadata must distinguish:
  - routing intentionally disabled for a fixture;
  - routing unsupported for a known topology;
  - routing enabled but failed;
  - routing enabled and generated complete required-net evidence;
- `skip_routing` may remain only if there is a specific, current blocker that
  explains why route generation must not run;
- route enablement must be fixture-scoped or capability-scoped, not a global
  relaxation for all expected-fail designs;
- route requests must include concrete endpoint anchors for required amplifier
  nets before routing starts;
- routing must fail closed if an endpoint is missing, ambiguous, or attached to
  the wrong canonical net.

### FR4: Required Amplifier Net Coverage

PCB validation must know which amplifier nets are required to be connected by
copper and which are intentionally external or local.

Requirements:

- required inter-block nets must include, when present:
  - `AUDIO_IN`;
  - `DRIVER_OUT`;
  - `BIAS_P`;
  - `BIAS_N`;
  - `AMP_OUT_DC_BIASED`;
  - `HP_OUT`;
  - `HP_RET`;
  - `LOAD_REF`;
  - `VCC`;
  - `GND`;
  - `VEE`;
  - output-device base/emitter/collector nets that cross block boundaries;
- single-pad or external-interface nets must be classified explicitly so they
  are not mistaken for missing routes;
- local-route evidence from verified amplifier blocks must count toward same-net
  connectivity only when route geometry contacts the assigned pads on the
  correct net;
- partial routes must report the connected component set and the missing
  component set;
- same-net route completion must not be claimed from bounding-box overlap,
  center-point placeholders, or labels alone.

### FR5: Writer-to-KiCad Evidence Handoff

When writer-level structural blockers are resolved, the promotion report must
make optional KiCad ERC/DRC the next authority.

Requirements:

- if KiCad CLI is unavailable, the fixture may remain `candidate` or
  `expected_fail` depending on metadata, but the report must state that KiCad
  evidence was not run;
- if KiCad CLI is available, the fixture must run ERC/DRC according to the
  existing optional KiCad-backed workflow and record results;
- KiCad ERC/DRC failures must not be hidden by broad structural-validation
  buckets;
- metadata must distinguish writer-clean but KiCad-failing designs from
  writer-blocked designs.

### FR6: Regression and Documentation

All status text must match the new behavior.

Requirements:

- update fixture metadata known gaps after each blocker class is resolved;
- update README, roadmap, and KiCad-backed example docs only where status
  changes;
- add regression tests for schematic label attachment, route-policy gating,
  required-net classification, and protected fixture promotion status;
- existing LED, connector, I2C, regulator, MCU, placement, routing, writer, and
  fabrication-readiness tests must continue to pass.

## Non-Functional Requirements

- Deterministic output across repeated test runs.
- No network dependency.
- Ordinary tests must not require KiCad CLI.
- Optional KiCad checks must be opt-in and clearly reported.
- Changes should use existing design workflow, block realization, route-tree,
  structural validation, promotion, and report models.
- Diagnostics should be specific enough for an AI controller to decide the next
  repair action without reading generated KiCad files by hand.

## Design Constraints

- Do not weaken structural validation to promote the fixture.
- Do not mark unrouted required nets as accepted unless metadata explicitly
  classifies them as allowed external or intentionally deferred.
- Do not hide schematic label bugs by suppressing validation globally.
- Do not route through unsupported amplifier topology or unsafe output-load
  assumptions.
- Do not remove existing amplifier block verification or simulation gates.
- Do not promote from `expected_fail` to `candidate` or `pass` without matching
  evidence.

## Acceptance Criteria

The project is complete when:

1. The protected Class AB fixture no longer reports stale placement, endpoint,
   project-write, or writer-correctness blockers.
2. Generated amplifier schematic labels either attach correctly or produce
   precise writer-level diagnostics.
3. Route policy for the protected fixture is explicit: routing runs when the
   route request is complete, or a current route-policy blocker is reported.
4. Required amplifier nets are classified as routed, locally connected,
   external, allowed unrouted, partially routed, or blocking.
5. The first remaining blocker is either a precise required-net route failure,
   a precise generated schematic issue, or optional KiCad ERC/DRC evidence.
6. Metadata, README, and roadmap text agree on the fixture status.
7. Focused tests and `go test ./...` pass with the repository-local Go build
   cache.
8. Prism review has no unresolved high or medium correctness findings for each
   implementation phase.

## Risks

- Label cleanup may expose real schematic topology gaps that were previously
  hidden by alias names.
- Enabling routing can reveal route-tree limitations for dense amplifier
  topologies.
- Required-net classification can accidentally bless incomplete connectivity if
  external and local nets are not modeled carefully.
- KiCad ERC/DRC may reveal new legitimate blockers after writer-level cleanup;
  that is expected and should become the next documented status rather than
  being bypassed.
