# Closed-Loop Open-Set Corpus Rules

Status: normative for corpus version 1

## Purpose

These rules keep the expansion benchmark independent of its implementation.
They apply to discovery and held-out authors, reviewers, manifests,
requirements, tests, reports, and all later production work.

## Membership

- Version 1 contains exactly 24 cases: 12 discovery and 12 held-out.
- Each role contains exactly two cases in each required reporting domain:
  analog, power, digital/control, MCU/interface, sensor, and mixed-signal.
- Case IDs are opaque sequential identities and carry no circuit-family,
  expected-outcome, topology, part, or capability meaning.
- Manifest order is authoritative and may not change after freeze.
- Discovery and held-out cases may exercise related observable needs, but may
  not duplicate or paraphrase one another.

## Independent Authoring

- Requirements are authored without reading production provider selection,
  topology operators, candidate ordering, component inventory contents,
  simulation implementation, physical fallback code, or known diagnostic text.
- Authors receive only the public behavior vocabulary and these rules.
- Held-out requirements are sealed before the discovery baseline is evaluated.
- Held-out source bytes and cluster membership are not used for capability
  selection, implementation, component/model choices, value tuning, repair
  policy, or test thresholds.
- A reviewer records the authorship process and confirms that no expected
  implementation or outcome was supplied.

## Allowed Requirement Content

A case may declare:

- external power, reference, signal, control, load, and fault interfaces;
- voltage, current, impedance, frequency, timing, accuracy, ripple, noise,
  distortion, efficiency, isolation, and environmental envelopes;
- startup, shutdown, enable, default-state, fault, recovery, and sequencing
  behavior;
- DC, AC, transient, distortion, noise, startup, fault, thermal,
  electrothermal, SOA, isolation, and tolerance assertions;
- safety relevance and externally owned bounds; and
- maximum board dimensions and component count without geometry prescriptions.

## Prohibited Requirement Content

A case may not state or imply:

- topology or named circuit arrangement;
- component class when behavior alone is sufficient, manufacturer, part number,
  package, symbol, footprint, unit, pin, pad, or model identity;
- internal net, node, reference designator, block, provider, architecture,
  primitive, operator, or repair identity;
- schematic grouping, hierarchy, coordinates, regions, layers, tracks, vias,
  zones, keepouts, route ordering, or clearances beyond external safety bounds;
- source code path, test name, corpus role, fixture identity, hash, diagnostic,
  issue code, gap code, expected outcome, expected stage, or ranking hint; or
- an implementation value chosen only to match an existing catalog entry.

Protocol names are allowed only when they are externally observable interface
requirements and must not prescribe a particular translator, buffer, isolator,
or controller.

## Diversity Rules

Within each role:

- no two cases may have the same set of ports, behavioral metrics, analyses,
  operating conditions, and fault requirements;
- at least four cases require multiple interacting behavioral functions;
- at least four cases require a dynamic analysis;
- at least four cases require fault or startup evidence;
- at least four cases require thermal, SOA, isolation, or other safety evidence;
- at least two cases exercise bipolar or below-reference behavior;
- at least two cases exercise switching or time-varying control; and
- at least two cases require mixed signal/power physical behavior.

These are authorship constraints, not production cluster labels.

## Safety Declarations

Each manifest row declares one reporting-only safety level:

- `non_safety`;
- `review_required`;
- `safety_relevant`; or
- `safety_critical`.

The declaration affects ranking evidence but never changes pass criteria.
Critical cases must state the observable bounds that make them critical.

## Freeze Artifacts

The freeze contains:

- strict manifest schema and version;
- opaque case ID, role, reporting domain, safety level, requirement filename,
  and requirement SHA-256;
- canonical manifest SHA-256;
- immutable requirement bytes;
- authorship/review record;
- evaluator-policy version;
- impact-registry hash;
- work-budget profile; and
- prohibited-token and semantic-neutrality test results.

Any membership, byte, hash, role, domain, safety, policy, registry, or budget
change creates version 2 and requires a new untouched baseline.

## Automated Freeze Tests

Tests must reject:

- missing, duplicate, reordered, or non-opaque IDs;
- source or manifest hash drift;
- missing domain/role quotas;
- discovery/held-out duplicate or normalized-text collision;
- expected outcome, stage, issue, capability, fixture, source-path, part,
  topology, model, coordinate, or routing vocabulary;
- invalid strict requirement documents;
- requirements without complete acceptance gates;
- reporting metadata leaking into requirement bytes; and
- production code containing any frozen case ID or corpus path outside the
  corpus loader and test harness.

Automated token checks supplement semantic review; they do not replace it.
