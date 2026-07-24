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
DRC gates. This is measured readiness inside the installed 111-component
generic graph contract as of 2026-07-21;
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

## Behavioral Intent Compilation

The uncertainty-aware compiler adds a language-facing readiness gate above the
matrix. Its 24-prompt, 12-paraphrase-group corpus spans amplifiers, filters,
power, protection, sensors, and MCU interfaces. Supported prompts must compile
to strict behavior, pass architecture search and trusted closed-loop evidence,
and then pass the complete physical/KiCad promotion lane. Ambiguous prompts ask
one minimal blocking question; unavailable behavior produces a stable
capability-gap record. This evidence measures the registered envelope and does
not promote unsupported matrix records implicitly.

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
promotes the modeled slice only; unreviewed converter/control-loop behavior,
isolation, RF/high-speed interfaces, arbitrary parts, and high-energy thermal
or protection claims remain unsupported.

## Held-Out Capability Expansion

The versioned behavior-only capability benchmark supplies two requirements in
each of six domains: analog, power, digital, MCU, sensor, and mixed-signal.
Neither prompts nor normalized requirements prescribe topology, components,
nets, pins, coordinates, layers, routes, providers, or expected evidence.

The frozen baseline passed 5 of 12 complete installed-KiCad workflows. The
final report passes 11 of 12 with the original evaluator and unchanged gate
profile. Reusable support now covers three distinct constant-current contexts
and two precision-rectification contexts. All control rows remain passing.
Every promoted row reaches trusted simulation, physical realization,
connectivity and route completion, writer correctness, clean ERC, strict DRC,
zero-difference round trip, and deterministic replay.

Standalone `clock_generation` is the sole remaining benchmark gap and remains
fail-closed at architecture selection. This result materially broadens the
measured envelope, but twelve cases cannot establish arbitrary circuit
generation, unrestricted part qualification, dense-board routing,
RF/high-speed behavior, mains safety, or fabrication release.

Authoritative evidence:

- [benchmark specification](../specs/held-out-capability-expansion/SPEC.md);
- [5/12 frozen baseline](../specs/held-out-capability-expansion/BASELINE_REPORT.json);
- [11/12 final report](../specs/held-out-capability-expansion/FINAL_REPORT.json);
  and
- [five-scenario promotion matrix](../specs/held-out-capability-expansion/PROMOTION_MATRIX.json).

## Reproducible Promotion Evidence

Run `make promotion-bundle` from an unmodified checkout to reproduce the
supported release evidence without hand-configured KiCad or library paths. The
toolchain lock fixes KiCad 10.0.3 and its stock libraries; each required design
runs twice, and a passing bundle proves the declared ERC, strict DRC,
connectivity, route, writer, round-trip, and deterministic-comparison gates.
The content-addressed bundle can be verified later without KiCad or network
access.

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
