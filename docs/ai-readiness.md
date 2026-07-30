# AI Readiness Matrix

The AI readiness matrix tracks the remaining verified-design knowledge needed
before an AI agent can generate broader schematics and PCBs without constant
human review.

The matrix is intentionally machine-readable. Records live under:

```text
data/ai-readiness/matrix/*.json
data/ai-readiness/requirements/*.json
```

The Go validator lives in `internal/aireadiness`.

## Record Shape

Each matrix record describes one gap or evidence target:

- `id`: stable `domain.category.slug` identifier. Each ID segment
  (`domain`, `category`, and `slug`) must be dot-free; use underscores for
  word separation within a segment. The validator enforces that the first two
  ID segments match the explicit `domain` and `category` fields.
- `category`: `component`, `block`, `layout`, `validation`, or
  `documentation`.
- `domain`: design domain, such as `amplifier`.
- `readiness`: enum validated by `internal/aireadiness`.
- `blocker`: why the item is not ready.
- `evidence_needed`: concrete evidence required before promotion.
- `next_task`: enum validated by `internal/aireadiness`.
- `evidence`: required when a record is marked `verified`.
- `parallel_group`: optional workstream owner for parallel execution planning.
  Missing values and explicit `unassigned` values are counted as `unassigned`
  in coverage summaries.
- `depends_on`: optional sorted list of readiness record IDs that must exist and
  must be `verified` before this record may be marked `verified`.

Verified records must carry supporting evidence. Evidence that references a
checked-in artifact must include either a semantic hash or git object ID.
Evidence may also include documented source references when the evidence kind
is not a generated artifact.

`internal/aireadiness` is the source of truth for enum validation. The docs
list current values for convenience.

Current `readiness` values:

- `missing`
- `draft`
- `connectivity`
- `candidate`
- `verified`

Current `next_task` values:

- `add_component`
- `add_block`
- `verify_pinmap`
- `verify_layout`
- `capture_kicad_evidence`
- `write_docs`

Current `parallel_group` values:

- `unassigned`
- `fixture_promotion`
- `catalog_block_expansion`
- `engine_hardening`
- `intent_ai_ux`
- `documentation`

`depends_on` references must use stable record IDs, must be sorted
alphabetically by full record ID string, must not reference the current record,
and must form a directed acyclic graph across the fully loaded matrix.
Cross-group dependencies are allowed, but they mean a workstream is not fully
independent from the referenced group.

Semantic hashes are intended to be hashes over canonicalized, non-volatile
representations of generated artifacts. Until a dedicated hash command exists,
prefer git object IDs for checked-in artifacts or keep the record below
`verified`.

## Requirement Shape

Requirement files under `data/ai-readiness/requirements/*.json` describe the
minimum matrix coverage expected for a domain.

- `version`: requirements schema version.
- `domain`: domain the requirements apply to.
- `required_categories`: matrix categories that must have at least one record
  for the domain.
- `required_record_ids`: specific record IDs that must exist for the domain.

## Amplifier Coverage

The `amplifier` matrix covers:

- family-level block contracts for input buffer, gain stage, bias network,
  Class AB output pair, output protection, supply decoupling, and load
  connectors;
- verified op-amp drive and stability choices;
- Class A/Class AB output devices;
- Class AB bias networks;
- headphone DC blocking and output protection;
- thermal and high-current layout constraints;
- feedback, decoupling, and stability layout;
- simulation-backed Class AB headphone promotion evidence;
- KiCad-backed amplifier promotion evidence;
- AI-facing amplifier design-limit documentation.

The bounded amplifier matrix is now `verified`. In addition to the headphone
slice, the protected 10 W RMS/8 ohm speaker lane selects a reviewed OPA134,
complementary driver and power BJT pairs, emitter/current-sense resistors,
stability parts, comparator, relay driver, and normally-open speaker relay. Its
reusable blocks provide load-side feedback, local current limiting, Zobel
damping, bipolar DC-fault detection, tolerance-bounded mute timing, supply-loss
release, relay clamping, star/Kelvin return intent, high-current net classes,
thermal coupling, device symmetry, and heatsink/mechanical constraints.

`class_ab_speaker_10w_protected` is a checked-in fabrication-candidate `pass`
fixture with no allowlist or known gap. Its declared lane requires clean real
KiCad ERC and strict DRC, complete route/contact evidence, writer correctness,
zero-difference round trip, and fabrication-package gates. The matrix evidence
is pinned to Git blob IDs so documentation cannot silently outrun the reviewed
implementation.

This verification is deliberately bounded. Bridge-tied outputs, substantially
higher power, mains-connected supplies, arbitrary output architectures,
unreviewed substitutions, and heatsinks outside the modeled envelope remain
unsupported and must fail closed.

## Open-Set Composition

The first frozen open-set corpus proves five single-function requirements. A
second adversarial corpus proves 10 behavior-only multi-function requirements
containing 35 objectives and 3 abstract participants. Search composes 18 typed,
registered capabilities, validates 23 whole-circuit constraints before scoring,
and reports selected, rejected, unsupported, ambiguous, and budget-exhausted
obligations with deterministic alternatives and rationale.

All 10 adversarial circuits pass component, rating, value, tolerance, lowering,
writer, round-trip, connectivity, routing, clean installed-KiCad ERC, and strict
DRC gates. The current catalog contains 171 records across 33 families as of
2026-07-28, including 156 verified records. The generic graph contract exposes
the supported subset of those records;
unknown capabilities, insufficient evidence, incompatible domains, unsafe
startup, and exceeded budgets fail closed. It does not establish unrestricted
natural-language intent, arbitrary topology or parts, RF/high-speed design,
mains safety, or general dense-board autorouting.

## Simulation-Grounded Synthesis

A SHA-256-pinned ten-circuit v3 corpus now proves bounded behavioral
requirements, trusted model decisions, registered analyses, named operating
corners, deterministic diagnosis/repair, and replay through the physical
workflow. It includes Class-A and Class-AB amplification and preserves clean
protected USB-C LED and I2C evidence. This moves readiness from structural
composition to measured closed-loop selection inside the checked-in catalog;
unknown topologies, arbitrary parts/models, RF, mains, and general dense-board
routing remain fail-closed boundaries.

## Dynamic Electrothermal And Control-Loop Synthesis

The V5 behavior-only corpus adds six previously unsupported dynamic systems:
reactive-load feedback amplification, analog servo regulation, switching power
conversion, protected inductive switching, Class-AB thermal/load-fault
behavior, and multi-rail sequencing. Requirements provide behavior, operating
corners, events, thermal environment, and safety margins without prescribing
implementation details.

Reviewed model provenance, connectivity-derived loop identity, deterministic
return-ratio evidence, declared-corner stability, coupled electrothermal
transients, transient SOA, protection response, candidate alternatives, and
bounded repairs are hash-bound through lowering and promotion. Two cases reject
a statically acceptable candidate and select a dynamically safe alternative.
All six cases pass two executions in each of two independent local
clean-checkout roots, with identical 418-file bundles, clean ERC, strict DRC,
complete routing/connectivity, writer correctness, zero round-trip
differences, and deterministic replay.

This expands measured readiness beyond static bounds, but only for reviewed
primitives, events, models, and work budgets. Unknown control ICs, arbitrary
converter topologies, unreviewed thermal assemblies, RF/high-speed behavior,
mains safety, and high-energy protection outside the modeled envelope remain
unsupported.

Authoritative evidence:

- [specification](../specs/dynamic-electrothermal-control-loop-synthesis/SPEC.md);
- [completion audit](../specs/dynamic-electrothermal-control-loop-synthesis/AUDIT.md);
  and
- [capability report](../specs/dynamic-electrothermal-control-loop-synthesis/CAPABILITY_REPORT.json).

## Behavioral Intent Compilation

The uncertainty-aware compiler adds a language-facing readiness gate above the
matrix. Its 24-prompt, 12-paraphrase-group corpus spans amplifiers, filters,
power, protection, sensors, and MCU interfaces. Supported prompts must compile
to strict behavior, pass architecture search and trusted closed-loop evidence,
and then pass the complete physical/KiCad promotion lane. Ambiguous prompts ask
one minimal blocking question; unavailable behavior produces a stable
capability-gap record. This evidence measures the registered envelope and does
not promote unsupported matrix records implicitly.

## Evidence-Driven Capability Expansion

An unsupported assessment can now produce a deterministic expansion plan
without being reclassified. Gap kinds are mapped to reusable architecture,
component, model, physical-rule, routing, and verification needs. Candidate
registries accept only bounded content-addressed sources whose explicit claims
and source kinds satisfy those needs; component and simulation-model proposals
remain quarantined alongside reviewed declarative provider contracts.

Each need automatically receives a representative case plus missing,
conflicting, irrelevant, and fabricated-evidence adversarial cases. A promotion
bundle stays `experimental` until source integrity, deterministic replay,
simulation, workflow, installed-KiCad ERC/strict DRC, connectivity, route
completion, writer correctness, and zero-diff round trip all pass. Passing
changes the bundle only to `review_ready`. A hash-bound review approval and
explicit registry mutation are still required for support, and fresh held-out
requests must select the promoted capability through the normal typed provider
interface.

This closes the process gap between “we know this is unsupported” and “we can
propose the generic evidence needed to support it.” It does not make arbitrary
unknown circuits safe or allow an AI to invent component ratings, models,
physical rules, or validation results.

## Generic MCU Subsystems

The behavior-driven catalog lane now selects among three verified controller
families without target names in the request. Typed MCU evidence covers
physical pins, alternate functions, supplies, programming, clocks, boot state,
and current budgets; deterministic constraint search binds compatible
peripheral bundles before graph lowering. A neutral ATmega328P-A,
ESP32-WROOM-32E, and STM32G031K8T6 corpus passes replay and installed-KiCad ERC,
strict DRC, routing/connectivity, writer, and zero-diff round-trip gates.

This is a deeper generic capability inside the checked-in catalog, not an
arbitrary-MCU claim. Unverified targets and unmodeled bus loading, power
transients, RF, thermal, or high-speed requirements continue to fail closed or
remain explicit review boundaries.

## Power-Tree And Interface Synthesis

A neutral four-design corpus now proves the bounded catalog-backed power and
mixed-signal interface slice without naming selected parts or physical
coordinates. It covers a regulated MCU/sensor subsystem, protected MOSFET load,
buffered ADC acquisition path, and Class-AB amplifier power interface. The
shared lane derives or validates rail source topology, current and transient
budgets, regulator dynamics and thermal margin, sequencing, pull-ups, level
translation, source termination, clock conditioning, and passive or buffered
ADC settling.

All four designs pass deterministic offline replay and the installed-KiCad
ERC, strict DRC, route/connectivity, writer, and zero-difference round-trip
gates. Ten reordered negative cases retain stable typed failure codes. This
promotes the modeled slice only; converter/control-loop behavior outside the
reviewed V5 model and event envelope, isolation, RF/high-speed interfaces,
arbitrary parts, and high-energy thermal or protection claims remain
unsupported.

## Held-Out Capability Expansion

The versioned behavior-only capability benchmark supplies two requirements in
each of six domains: analog, power, digital, MCU, sensor, and mixed-signal.
Neither prompts nor normalized requirements prescribe topology, components,
nets, pins, coordinates, layers, routes, providers, or expected evidence.

The frozen baseline passed 5 of 12 complete installed-KiCad workflows. The
original expansion report advanced to 11 of 12 with the unchanged evaluator
and gate profile by covering three constant-current contexts and two
precision-rectification contexts. Generic fixed packaged and
resistor-programmed clock generation subsequently closes the unchanged
benchmark at 12 of 12. Every row reaches trusted simulation, physical realization,
connectivity and route completion, writer correctness, clean ERC, strict DRC,
zero-difference round trip, and deterministic replay.

This result materially broadens the measured envelope, but twelve cases cannot
establish arbitrary circuit generation, unrestricted part qualification,
dense-board routing, RF/high-speed behavior, mains safety, or fabrication
release.

Authoritative evidence:

- [benchmark specification](../specs/held-out-capability-expansion/SPEC.md);
- [5/12 frozen baseline](../specs/held-out-capability-expansion/BASELINE_REPORT.json);
- [11/12 expansion report](../specs/held-out-capability-expansion/FINAL_REPORT.json);
- [12/12 clock closeout audit](../specs/standalone-clock-generation/AUDIT.md);
  and
- [five-scenario promotion matrix](../specs/held-out-capability-expansion/PROMOTION_MATRIX.json).

## Hierarchical Multi-Domain Synthesis

The V4 lane now derives system/subsystem/block hierarchy, independently
checkable boundary contracts, shared-resource accounting, global candidate
backtracking, physical partitions, and end-to-end traceability for a frozen
six-system corpus. The systems span protected Class-AB, precision analog,
regulated MCU/sensor, isolation, high-current switching, and split-supply
monitoring. Each passes two-run offline and installed-KiCad promotion.

This is the strongest current evidence toward AI-directed whole-system design,
but it remains limited to registered capabilities and verified catalog/model
evidence. See the
[hierarchical specification](../specs/hierarchical-multi-domain-synthesis/SPEC.md)
and [promotion matrix](../specs/hierarchical-multi-domain-synthesis/PROMOTION_MATRIX.json).

## Protocol-Aware Bus Synthesis

The registered architecture lane now handles behavior-only I2C, SMBus, SPI,
and UART buffering and level translation. It covers solved open-drain pull-ups,
whole-bus and per-segment loading, independent branch translation,
mixed-direction push-pull channel groups, reversed voltage domains, inactive
startup, partial-power isolation, hot-plug containment, and explicit
contention policy.

Four frozen requirements pass deterministic architecture search, trusted-model
closed-loop evaluation, lowering, routing/connectivity, writer and round-trip
checks, and local installed-KiCad ERC/strict DRC twice. Eleven negative
conditions fail closed. This closes a measured architecture cluster but does
not add USB, Ethernet, LVDS, CAN/RS-485, RF, or transmission-line signoff.
See the
[specification](../specs/protocol-aware-bus-synthesis/SPEC.md) and
[promotion matrix](../specs/protocol-aware-bus-synthesis/PROMOTION_MATRIX.json).

## Evidence-Backed Component Onboarding

Unfamiliar parts no longer require an immediate hand edit to the checked-in
catalog. Behavior-only requirements and immutable manufacturer documents can
produce identity-neutral, content-addressed candidate artifacts. Provider
extraction remains untrusted: exact source excerpts, values and units,
conflicts, ratings, temperature, derating, package, pin-to-pad mappings,
licenses, and registered model provenance are all revalidated
deterministically.

The candidate is quarantined and can run only in an isolated evaluation
catalog. It becomes a supported overlay after independent exact-hash approval
and two identical passing runs of executable simulation, connectivity, route
completion, writer correctness, round trip, ERC, and strict DRC. The frozen
corpus contains seven unfamiliar identities across op-amp, transistor,
regulator, converter, sensor, logic, and interface families; no corpus MPN or
component ID appears in production Go.

This advances component readiness from “catalog additions are entirely
manual” to “source-backed proposals can be qualified automatically under
registered family and model semantics.” It does not infer new simulation
physics, create unknown package geometry, waive missing manufacturer data, or
approve a part for fabrication. See the
[specification](../specs/evidence-backed-component-onboarding/SPEC.md),
[plan](../specs/evidence-backed-component-onboarding/PLAN.md), and
[promotion matrix](../specs/evidence-backed-component-onboarding/PROMOTION_MATRIX.json).

## Reproducible Promotion Evidence

Run `make promotion-bundle` from an unmodified checkout to reproduce the
supported release evidence without hand-configured KiCad or library paths. The
toolchain lock fixes KiCad 10.0.3 and its stock libraries; each required design
runs twice, and a passing bundle proves the declared ERC, strict DRC,
connectivity, route, writer, round-trip, and deterministic-comparison gates.
The content-addressed bundle can be verified later without KiCad or network
access.

Run `make hierarchical-promotion-bundle` to reproduce the six-system
hierarchical matrix through the same locked toolchain, two-run comparison, and
standalone verifier.

Run `make dynamic-electrothermal-promotion-bundle` to reproduce the six-circuit
V5 matrix, including two executions per scenario, installed-KiCad gates,
normalized comparison, and the content-addressed capability evidence.

Run `make component-onboarding-promotion-bundle` with
`KICADAI_KICAD_CLI`, `KICADAI_SYMBOLS_ROOT`, and
`KICADAI_FOOTPRINTS_ROOT` set to reproduce the seven unfamiliar-part cases in
two clean source snapshots and compare 56 canonical KiCad/simulation
artifacts.

This strengthens provenance and reproducibility for records already inside the
verified capability envelope. It does not promote an unverified matrix record,
qualify an unknown part or model, prove arbitrary-circuit generation, or turn a
design-validation pass into fabrication approval.

## Validation

Run:

```sh
go test ./internal/aireadiness
```

The tests validate:

- schema and enum values;
- stable ID format;
- ID/domain/category consistency;
- duplicate IDs;
- valid parallel groups;
- dependency references, ordering, duplicate entries, cycles, and verified
  dependency readiness;
- verified records requiring evidence;
- amplifier requirement coverage;
- domain coverage summaries, including `by_parallel_group`.

## Contributor Flow

When adding a component, block, or layout capability for AI generation:

1. Add or update a matrix record.
2. Keep non-verified records explicit about blockers and required evidence.
3. Add evidence only when it is backed by checked-in artifacts, semantic hashes,
   git object IDs, or documented source references.
4. Promote readiness in small steps: `missing` -> `draft` -> `connectivity` ->
   `candidate` -> `verified`.
5. Keep `requirements/*.json` updated when a domain has mandatory coverage.
6. Assign `parallel_group` when the record belongs to a known workstream, and
   use `depends_on` for real prerequisites instead of burying ordering in prose.
