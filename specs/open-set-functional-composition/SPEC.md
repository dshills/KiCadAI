# Open-Set Functional Circuit Composition Specification

## 1. Purpose

KiCadAI can synthesize verified function-level intents, but those intents still
name most functions and connections before synthesis begins. This project adds
a deterministic, bounded architecture-search layer that starts from electrical
behavior and composes catalog-backed components with reusable typed circuit
fragments. Its result lowers into the existing function-level circuit graph and
therefore retains the current writer, routing, validation, and evidence gates.

The milestone is complete only when unfamiliar held-out requirements can be
realized without fixture-aware production logic and all existing regressions
remain green.

## 2. Goals

The implementation must:

- accept behavioral electrical requirements without preselecting a topology;
- model typed ports, direction, electrical limits, protocols, and power domains;
- compose catalog components and reusable fragments through compatible ports;
- generate, reject, score, and select candidates within explicit budgets;
- calculate component values and publish nominal and tolerance evidence;
- retain distinct viable alternatives and explain the selected candidate;
- fail closed when a requirement is ambiguous, unsafe, unsupported, or lacks
  sufficient evidence;
- produce a normal function-level intent only after architecture selection;
- preserve deterministic replay under request, catalog, and provider reordering;
- pass writer correctness, zero-diff round trips, connectivity, route
  completion, and applicable KiCad ERC and strict DRC gates.

## 3. Non-Goals

This project does not:

- claim unbounded or universally optimal circuit synthesis;
- infer unstated safety-critical requirements;
- use online component searches or mutable external services;
- bypass catalog identity, rating, simulation, ERC, DRC, routing, or
  fabrication evidence;
- encode held-out fixture names, fixture coordinates, expected parts, expected
  fragment IDs, or expected topology in production code;
- add one request schema per topology or provider;
- accept a candidate merely because it can be serialized to KiCad.

## 4. Input Contract

The architecture input schema is `kicadai.open-set-requirement.v1`. It is a
behavioral contract separate from the selected circuit graph and contains:

- project identity and requested acceptance level;
- named power/reference domains and their operating ranges;
- external typed ports and electrical contracts;
- optional abstract participants identified only by required capability;
- objectives identified by capability and bound to ports;
- generic constraints expressed as name, relation, value, unit, and tolerance;
- board-level component-count and size budgets;
- explicit acceptance gates.

The input must not contain component IDs, variants, manufacturer part numbers,
references, symbols, footprints, pins, pads, fragment IDs, circuit topology,
nets, placement, layers, routes, or simulation implementations. Those are
search results or later lowering decisions.

The decoder is strict. Unknown fields, duplicate identities, invalid units,
unknown relations, unresolved bindings, contradictory ranges, and non-finite
quantities are blocking diagnostics.

## 5. Typed Port And Domain Model

Every component and fragment boundary exposes typed ports. A port contract
contains, where applicable:

- semantic kind, such as power, reference, analog voltage, digital logic,
  open-drain bus, switched load, or protected output;
- direction: source, sink, or bidirectional;
- domain identity;
- minimum, nominal, and maximum voltage;
- continuous and peak current limits;
- source/sink drive limits and input leakage;
- input/output impedance bounds;
- frequency or edge-rate bounds;
- logic thresholds and output levels;
- protocol and mode, including open-drain versus push-pull;
- required default or unpowered state;
- isolation and protection traits when required;
- evidence confidence and source identifiers.

Compatibility is a proof obligation. Two ports unify only when direction,
kind, domain, ranges, drive/load limits, protocol, default state, and required
traits are compatible. Unknown data cannot satisfy a safety-critical bound.
Conversions such as level translation, buffering, protection, or power
regulation require an explicit fragment; the unifier must not silently coerce
electrical domains.

## 6. Reusable Fragment Contract

A fragment is a topology-capable provider registered independently of any
fixture. It declares:

- stable provider and revision identity;
- one or more supplied capabilities;
- typed external ports;
- required child capabilities or catalog component predicates;
- generic applicability constraints;
- deterministic expansion into functions and connections;
- a bounded parameter/value solver;
- validation predicates and rejection codes;
- cost/evidence metrics used by scoring;
- provenance for formulas and design rules.

Provider input is the same normalized requirement/port model for every
fragment. Providers may not receive fixture IDs or arbitrary fixture extension
objects. A provider may implement a reusable electrical family, but it may not
define a private request schema that predetermines its topology.

Initial providers may cover threshold detection, protected load switching,
adjustable regulation, active-filter stages, and logic translation, but must be
selected only through their declared capabilities and contracts.

## 7. Bounded Deterministic Search

Search proceeds in this fixed order:

1. Strictly decode and normalize units, names, lists, and ranges.
2. Convert objectives into unresolved capability and port obligations.
3. Enumerate matching catalog components and fragment providers in stable
   identity order.
4. Expand the lexicographically earliest unresolved obligation.
5. Unify typed ports and domains.
6. Run parameter/value solvers and validate nominal and tolerance corners.
7. Reject candidates with structured reasons as soon as a hard obligation
   fails.
8. Continue until candidates are complete or a declared budget is exhausted.
9. Score complete candidates and retain distinct alternatives.
10. Lower the selected candidate to a function-level intent and revalidate all
    contracts against the resolved catalog records.

Default hard budgets are:

- 256 expanded candidate states;
- depth 12;
- 64 selected components;
- 32 unresolved obligations per state;
- 16 candidates per provider expansion;
- three complete architecture alternatives;
- 10,000 deterministic tolerance-corner evaluations.

Requests may lower these budgets but may not exceed policy maxima. Budget
exhaustion is a fail-closed result containing the explored count, frontier
count, limiting budget, and unresolved obligations. Parallel evaluation is
permitted only if the merged result is byte-for-byte identical to serial
evaluation.

## 8. Candidate Rejection

Every rejected candidate records a stable code, obligation path, provider or
component identity, and evidence. Rejection categories include:

- unsupported capability;
- incompatible port kind or direction;
- voltage, current, impedance, frequency, or logic-level mismatch;
- missing protection or unsafe default state;
- unavailable or insufficient-confidence catalog evidence;
- unsatisfied participant capability;
- unsolved or out-of-range value;
- failed nominal constraint;
- failed tolerance corner or inadequate margin;
- component-count, board-size, or search-budget excess;
- invalid expanded graph;
- unresolved ambiguity where policy requires user choice.

The report includes aggregate rejection counts and a bounded deterministic
sample per category. Diagnostics must not depend on map iteration or goroutine
completion order.

## 9. Value And Tolerance Evidence

Value synthesis must use deterministic formulas and preferred-value series.
Each derived value records:

- formula/revision identity;
- normalized inputs and units;
- ideal value;
- permitted series and candidate values considered;
- selected nominal value;
- catalog tolerance and rating;
- evaluated nominal result;
- worst-case corners and result bounds;
- required bound and remaining margin;
- rejection evidence for discarded values.

Discrete selection uses numeric error followed by rating margin, evidence
confidence, package preference, and canonical value identity. Floating-point
comparisons use documented quantization before ordering. Missing tolerance or
rating evidence cannot be treated as zero tolerance or unlimited rating.

## 10. Scoring And Alternatives

Hard-invalid candidates are never scored as viable. Viable candidates use this
lexicographic tuple:

1. count of unproven non-safety obligations;
2. negative worst normalized constraint margin;
3. negative aggregate evidence confidence;
4. component count;
5. fragment count;
6. estimated quiescent power where comparable evidence exists;
7. board-area estimate;
8. canonical architecture fingerprint.

No weighted sum may allow a safety failure to trade against convenience.
Unknown optional metrics sort after known metrics. The report retains up to
three candidates with distinct architecture fingerprints and explains the
first score field that separated each alternative from the selected candidate.

## 11. Search Result And Rationale

The deterministic result contains:

- policy and schema versions;
- normalized requirement hash;
- catalog, provider-registry, and formula-library hashes;
- applied budgets and consumption;
- selected architecture fingerprint;
- selected components and fragments with evidence;
- port/domain bindings;
- calculated values and tolerance evidence;
- satisfied and unproven constraints;
- rejected-candidate summary;
- ranked distinct alternatives;
- selection rationale;
- fail-closed diagnostics when no candidate is selectable;
- hash of the lowered function-level intent when successful.

Successful replay against the same immutable inputs must produce identical
result and lowered-intent bytes.

## 12. Held-Out Corpus

The frozen corpus is stored in
`internal/circuitgraph/testdata/open_set_composition_corpus`. It contains five
behavior-only requirements selected before implementation:

- a threshold detector with controlled hysteresis;
- a protected inductive-load switch driven by low-voltage logic;
- an adjustable regulated supply requiring passive set-point programming;
- a fourth-order active low-pass response;
- a controller/sensor interface whose open-drain bus crosses logic domains.

The manifest and each requirement are SHA-256 pinned. Corpus membership and
content changes require a specification revision; silently editing an input to
match an implementation is forbidden. Production packages may not reference
corpus paths, hashes, fixture IDs, project names, or descriptions.

## 13. Fail-Closed Rules

No function-level intent is emitted when:

- the request is invalid, contradictory, or materially ambiguous;
- no complete candidate satisfies every safety-critical constraint;
- ratings, tolerances, logic thresholds, or protection evidence are missing;
- the search budget is exhausted before proving a selectable result;
- two candidates remain tied across the complete score tuple but imply a
  user-visible safety or functional choice;
- lowering changes or loses a proven electrical contract;
- post-lowering graph validation fails.

The failure remains structured and actionable. It must never fall back to a
fixture, a nearest topology, or an unverified default.

## 14. Verification And Completion

For each held-out requirement, evidence must prove:

- deterministic strict decoding and normalization;
- independence from request, catalog, and provider registration order;
- bounded search with recorded rejections and scoring;
- typed-port and domain compatibility;
- component identity and rating resolution;
- value and worst-case tolerance calculations where applicable;
- at least one ranked alternative where more than one viable architecture is
  available, or explicit evidence that no alternative is viable;
- complete lowering and connection coverage;
- deterministic function-level synthesis and replay;
- writer correctness and zero round-trip differences;
- complete routing/connectivity;
- clean KiCad ERC and strict DRC under the optional configured lane.

Repository-wide tests must also prove that existing amplifier, ESP32, MCU,
analog, sensor, USB-C, writer, routing, round-trip, and fabrication fixtures are
unchanged or improved. Completion requires Prism review, committed changes,
push, and passing GitHub Actions.
