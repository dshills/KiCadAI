# Closed-Loop Open-Set V8 Author Corpus Rules

## Fixed allocation

The complete corpus contains exactly 36 fresh behavior-only requirements: 18
discovery and 18 held-out. Exactly six independent authors each write six:
three discovery and three held-out. Each author has one assignment in every
reporting domain and every circuit role. Assigned identity, corpus role,
reporting domain, circuit role, safety impact, source ID, and path are immutable.
Manifest identities occur only in assignment and authorship metadata, never in
a requirement body.

## Isolation and behavior-only scope

Use only the files frozen by the per-author manifest and an empty quarantine.
Do not inspect repository code, prior corpus source, retired held-out plaintext,
another packet or bundle, synthesis results, baselines, frontiers, rankings,
plans, diagnostics, capabilities, or expected outcomes. Do not run synthesis,
simulation, feasibility, classification, ranking, or outcome tools.

Requirements specify externally observable behavior and physical constraints.
They must not prescribe manufacturer or orderable parts, symbols, footprints,
packages, pins, component values other than external contracts, topologies,
named circuit families, stage counts, coordinates, routes, layers, templates,
block families, algorithms, expected diagnostics, capabilities, or known
KiCadAI limitations.

## Assignment semantics

Use the assigned reporting domain and circuit role to conceive behavior without
copying their literal metadata into the requirement. Circuit roles mean:

- `source_bias`: externally bounded source or bias behavior;
- `amplification_conditioning`: an observable input-to-output transfer;
- `conversion_regulation`: conversion or regulation across external envelopes;
- `sensing_measurement`: a measured stimulus mapped to an observable output;
- `interface_control`: a control/data input governing observable outputs; and
- `protection_supervision`: externally testable supervision, limiting, or fault
  response.

Safety impact controls critical evidence without appearing as prose metadata:

- `non_safety`: no assertion is marked critical;
- `review_required`: include meaningful off-nominal behavior, with critical
  assertions optional;
- `safety_relevant`: at least one assertion is critical; and
- `safety_critical`: at least two assertions are critical, including one tied
  to a bounded fault or off-nominal case.

## Structural and electrical validity

Every file strictly follows `PUBLIC_REQUIREMENT_CONTRACT.md`, uses only its
canonical vocabulary, resolves every reference, includes all 14 acceptance
gates, and uses finite, ordered, bounded, physically meaningful values. Each
analysis and assertion must be testable from its conditions and observations.
Interfaces, external energy, loads, events, and assertions must be mutually
coherent. Disclose uncertainty rather than choosing an implementation or
predicting an outcome.

## Diversity and non-duplication

Within the six-file bundle:

- at least two requirements have multiple meaningful outputs, with at least one
  discovery and one held-out;
- at least four distinct analysis kinds occur;
- at least two requirements have a static primary behavior and at least two
  have a dynamic primary behavior;
- at least two include a bounded fault or off-nominal operating case;
- at least one includes noise, thermal, stability, or tolerance behavior; and
- no normalized electrical behavior signature repeats.

Independently vary port quantity/direction/dependency, supplies, polarity,
sequence, range, analyses, events, sweeps, corners, loads, environment, metrics,
observation scopes, behavior shape, board limits, and safety context. A
discovery and held-out requirement must not be paired variants. Cosmetic
renaming, reordered arrays, unused ports, duplicated assertions, and ideal
unbounded sources do not count toward diversity.

## Return and correction boundary

Return exactly six assigned JSON files plus `AUTHORSHIP.json`, with source
hashes in assignment order. The validator may issue only outcome-neutral
correction packets naming public rule IDs and assigned paths. A correction may
alter only those assigned files plus `authoring_ended_utc` and corresponding
source hashes. It may not expose another author, historical plaintext,
synthesis behavior, expected outcomes, diagnostics tied to capabilities, or
implementation advice. A behavior/domain/role/safety/partition change requires
a fresh replacement rather than correction.
