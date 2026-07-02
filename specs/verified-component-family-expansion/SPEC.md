# Verified Component Family Expansion Specification

## Purpose

Expand KiCadAI component intelligence from a seed set of verified records into
larger, deterministic component families that AI design workflows can trust for
common generated boards.

The current catalog can select a small number of concrete passives,
connectors, LEDs, protection parts, regulators, and active parts. That is
enough for demos and selected block workflows, but not enough for broad
autonomous schematic and PCB generation. AI-generated designs need several
verified choices per common value/package/function, clear alternates, and
fail-closed behavior when evidence is missing.

This project closes the next roadmap gap under Priority 1: expand from the
first verified alternative slice to larger verified families, more
values/packages, and real second-source alternates.

## Goals

- Add verified family coverage for common design primitives used by simple
  digital boards, breakouts, power-entry boards, and amplifier circuits.
- Preserve deterministic selection and explainability when multiple concrete
  alternatives match.
- Require enough evidence for connectivity-level and fabrication-candidate
  workflows to reject placeholders and unsafe generic records.
- Improve component coverage reporting so missing alternates, missing MPNs,
  missing procurement snapshots, and weak equivalence groups are visible.
- Keep all supplier data local and snapshot-based until a trusted provider
  adapter is explicitly selected.

## Non-Goals

- No live supplier API calls.
- No claims of live stock, price, lead time, or manufacturer acceptance.
- No broad catalog scraping.
- No replacement of the KiCad library resolver.
- No final analog design validation for amplifier performance, stability, SOA,
  thermal design, or distortion.
- No automatic footprint choice for packages whose KiCad footprint semantics
  are ambiguous without reviewed evidence.

## Current Foundation

The project already has:

- `data/components` catalog files for families, passives, connectors,
  diodes/LEDs, active blocks, verified active parts, and alternatives.
- component selection by family, value, package, ratings, required functions,
  confidence, lifecycle, availability, and alternatives;
- equivalence-group metadata with preferred/alternate/fallback roles;
- local lifecycle/availability source snapshots through `--source-dir`;
- BOM and fabrication identity propagation from selected component metadata;
- coverage metrics for concrete records, generic fallback records, equivalence
  groups, missing MPNs, and family-level counts;
- KiCad library and pinmap evidence validation for covered records;
- workflow evidence for selected components and rejected candidates.

## Target Families

The first expansion should focus on high-leverage families that appear in
generated designs and amplifier examples.

### Passives

Required coverage:

- resistors:
  - 0603 and 0805;
  - values: 0R, 47R, 100R, 220R, 330R, 470R, 1k, 2.2k, 4.7k, 10k, 22k, 47k,
    100k;
  - at least one preferred concrete record and one alternate for common 0603
    and 0805 values where practical;
  - power ratings sufficient for ordinary signal/control use;
  - explicit derating/review notes when current or power requirements exceed
    ordinary assumptions.
- ceramic capacitors:
  - 0603 and 0805;
  - values: 10pF, 18pF, 22pF, 100pF, 1nF, 10nF, 100nF, 1uF, 4.7uF, 10uF;
  - voltage ratings: 16V, 25V, and 50V where package/value availability makes
    sense;
  - dielectric and derating notes for MLCC capacitance loss.
- electrolytic/polymer capacitors:
  - common through-hole or SMD footprints for 10uF, 47uF, 100uF, 220uF;
  - explicit polarity, voltage rating, package, and footprint evidence;
  - selection should require review if ripple current or ESR matters and the
    catalog does not model it.

### Connectors

Required coverage:

- 1x02, 1x03, 1x04, 1x05, 1x06 2.54 mm pin headers;
- common JST-style 2-pin and 3-pin low-voltage connectors if matching KiCad
  footprints are verified;
- audio-related connectors useful for amplifier examples:
  - 3.5 mm TRS jack candidates;
  - screw terminals or pluggable terminal blocks for speaker/power output.

Connector records must include pin functions for power, ground, signal, and
mechanical/shield pins where applicable.

### Diodes, LEDs, And Protection

Required coverage:

- small-signal diode second source for 1N4148-class use;
- Schottky diode options for reverse-polarity or low-current ORing;
- 0805 and 0603 LEDs in common colors;
- TVS/ESD protection for USB/power/signal-line use where footprint and pin
  mapping are verified.

### Amplifier-Relevant Seed Parts

Required coverage:

- small-signal BJT NPN/PNP pairs suitable for class AB examples;
- common op-amp packages already used in examples, with supply-range and
  output-current metadata;
- basic power transistor placeholders must remain blocked unless concrete
  pinout, package, thermal, and rating evidence is present.

This is not a complete audio design library. It is enough to prevent generated
amplifier examples from relying on unverified placeholders where concrete,
low-risk signal parts are available.

## Evidence Requirements

Every concrete record added for connectivity-level or higher selection must
include:

- family and package variant metadata;
- manufacturer and MPN;
- KiCad symbol ID;
- KiCad footprint ID for each selectable package;
- pinmap or rule-backed pin mapping evidence;
- confidence level:
  - `verified` for active, connector, polarized, and pinout-sensitive parts;
  - `rule_inferred` may be used only for symmetric passives with reviewed
    KiCad symbol/footprint pin assumptions;
- value and rating constraints appropriate to selection;
- lifecycle field, even if lifecycle source evidence is loaded separately;
- equivalence metadata when it participates in an alternate group.

Every equivalence group must:

- contain exactly one preferred member;
- contain zero or more alternates;
- not mix incompatible family, value, package, pinout, footprint, or rating
  semantics;
- use stable group IDs such as `resistor.10k.0805` or
  `capacitor.100n.0805.50v`.

Local source snapshots should be added for every newly introduced MPN when
available. Missing source snapshots are allowed at connectivity acceptance but
must be reported in coverage and must block fabrication-candidate workflows
when procurement evidence is required.

## Catalog Behavior

Selection must remain deterministic:

- preferred equivalence members win over alternates when both satisfy the
  query;
- alternates may be returned as candidate/rejection evidence;
- unresolved ambiguity between non-equivalent top candidates remains blocking
  unless the request explicitly allows alternatives;
- under-rated or under-evidenced records are rejected with stable issue codes;
- generic fallback records are allowed only at acceptance levels that permit
  their confidence.

Selection should prefer concrete parts over generic fallback records when both
match and the requested acceptance requires concrete evidence.

## Coverage Behavior

`component coverage` should make the expansion measurable:

- family-level concrete record count;
- generic fallback count;
- equivalence group count;
- groups missing preferred members;
- groups with duplicate preferred members;
- concrete records missing MPN;
- records missing local lifecycle/availability source evidence when
  `--source-dir` is supplied;
- optional target thresholds for required expansion families.

Golden coverage snapshots must be updated intentionally.

## CLI And Workflow Surface

Existing CLI commands should continue to work:

- `kicadai component list`
- `kicadai component show <id>`
- `kicadai component find ...`
- `kicadai component select`
- `kicadai component validate`
- `kicadai component coverage`

No new top-level command is required for the first implementation. New
selection and coverage fields are acceptable only if they are stable,
documented, and tested.

`design create` should benefit automatically when block components use
component queries rather than hard-coded placeholders.

## Data Layout

Preferred catalog organization:

- keep generic family records in existing family files where appropriate;
- add concrete passive alternates in `data/components/alternatives.json` or a
  new clearly named catalog file if the existing file becomes unwieldy;
- keep active/pinout-sensitive verified parts in `verified_active.json` or a
  family-specific file if needed;
- keep local source snapshots outside the core catalog in
  `data/component-sources` if checked in, or test fixtures if source data is
  intentionally synthetic.

All records must sort deterministically through existing catalog sort behavior.

## Testing Requirements

Unit and golden tests must cover:

- checked-in catalog loads and validates;
- coverage counts are deterministic;
- each target family has required concrete/equivalence coverage;
- representative concrete selections for resistor, capacitor, connector, LED,
  diode, BJT, and op-amp candidates;
- rating rejection for under-rated parts;
- lifecycle/availability source evidence behavior where snapshots exist;
- ambiguity behavior for non-equivalent candidates;
- equivalence preference behavior for preferred versus alternate members;
- workflow selected-component evidence for at least one generated design using
  expanded parts.

## Acceptance Criteria

- The checked-in catalog has deterministic verified coverage for the target
  first-slice families.
- `component validate` passes for the checked-in catalog.
- `component coverage` shows increased concrete records and equivalence groups
  without missing preferred or duplicate preferred groups.
- Representative selection tests prove concrete part selection and rejection
  behavior.
- Generated design workflows can select expanded concrete parts without
  relaxing acceptance gates.
- No local Atlas DB, generated cache, or live provider output is committed.

## Risks

- Adding real MPNs without source discipline can imply availability claims the
  project cannot support.
- Similar package names can map to subtly different KiCad footprints.
- Capacitor voltage/dielectric derating can produce false confidence if not
  modeled conservatively.
- Audio/amplifier parts need richer analog validation than the component
  catalog can currently provide.
- Large JSON catalog changes can become hard to review unless sliced by family
  and backed by golden diffs.

