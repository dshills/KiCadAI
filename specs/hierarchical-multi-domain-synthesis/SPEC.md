# Hierarchical Multi-Domain Circuit Synthesis Specification

## 1. Purpose

This milestone closes the gap between flat multi-objective composition and
system-level circuit synthesis. A requirement continues to describe behavior,
interfaces, operating conditions, safety limits, and board limits. Production
code must derive the subsystem hierarchy, interface contracts, shared-resource
plan, implementation choices, and physical partitions.

Completion is measured against a frozen held-out corpus created before the
production implementation. Passing the corpus establishes a bounded
hierarchical synthesis envelope; it does not claim unrestricted arbitrary
circuit generation.

## 2. Required Outcomes

The implementation must:

- derive a canonical root/subsystem/block hierarchy from the behavioral
  requirement without receiving subsystem assignments from the fixture;
- give every derived block an independently verifiable functional contract;
- solve all inter-block voltage, current, impedance, bandwidth, logic-level,
  timing, protocol, noise, thermal, startup, and protection obligations;
- plan shared rails, references, clocks, resets, sequencing, decoupling,
  connectors, grounding, and protection resources at system scope;
- backtrack deterministically when individually valid block choices cannot
  satisfy the complete system;
- prove block-level and end-to-end behavior across every declared operating
  and tolerance corner;
- derive physical partitions and cross-partition routing constraints from
  electrical evidence;
- preserve traceability from requirements through hierarchy, contracts,
  choices, transactions, KiCad objects, and verification evidence;
- fail closed whenever any required proof is absent, contradictory, outside a
  bounded search budget, or below the required evidence confidence.

## 3. Requirement Contract

The public requirement schema is `kicadai.open-set-requirement.v4`. V4 is an
additive evolution of V3: it retains domains, external ports, abstract
participants, functional objectives, typed signals, operating cases,
behavioral requirements, system constraints, board limits, and all existing
promotion gates.

V4 does not accept fixture-authored subsystem membership, provider choices,
component identities, topology, nets, pins, coordinates, layers, tracks,
routes, vias, or expected output data. The hierarchy is generated evidence,
never an input hint.

The V4 acceptance contract adds mandatory gates for:

- hierarchical decomposition;
- independently checkable interface contracts;
- global shared-resource planning;
- deterministic architecture backtracking;
- physical partition evidence;
- end-to-end traceability.

Older schemas retain byte-identical canonical behavior and do not silently
acquire V4 claims.

## 4. Derived Hierarchy

The generated hierarchy has three canonical levels:

1. one system root representing the complete requirement;
2. subsystem nodes derived from electrical domains, connected behavioral
   cones, safety boundaries, and physical compatibility;
3. block nodes corresponding to selected functional obligations and their
   provider-generated child obligations.

Every non-root node records:

- stable identity derived from semantic inputs rather than traversal order;
- parent identity and canonical child order;
- covered requirement, participant, objective, and behavioral assertion IDs;
- domain and safety classifications;
- inbound and outbound contract IDs;
- selected expansion and component evidence after search;
- block-level analyses and assertions required for verification.

Every objective and participant must occur exactly once. Every selected
provider child must be owned by one block. Empty, overlapping, orphaned, or
cyclic hierarchy nodes are invalid.

## 5. Interface Contracts

Every boundary crossing becomes a typed contract. Contracts cover external
ports, internal signals, derived supplies, participant ports, and
provider-generated child boundaries. Each contract records applicable:

- source and sink identities and direction;
- signal kind, protocol, and reference domain;
- voltage and logic-level ranges;
- source current, sink demand, fanout, impedance, and loading;
- bandwidth, edge, frequency, and timing limits;
- startup/default state and sequencing dependencies;
- noise contribution or susceptibility;
- thermal or high-current classification;
- protection, isolation, and fault-containment obligations.

Compatibility must be proven from normalized bounds and selected evidence.
Unknown safety-relevant fields are blocking. A contract cannot be considered
proven merely because its endpoint blocks pass independently.

## 6. Shared-Resource Planning

The deterministic resource plan accounts for every consumer and records the
chosen source, capacity, demand, reserve, sequence, and affected hierarchy
nodes for:

- power rails and references;
- clocks and reset/control signals;
- connectors and externally exposed interfaces;
- decoupling and bulk-energy requirements;
- protection and isolation boundaries;
- grounding and return-path policy;
- thermal and high-current budgets.

Resource aliases are permitted only when all electrical and safety contracts
are compatible. Double counting, omitted consumers, contradictory sequencing,
and unproven capacity fail closed.

## 7. Search And Backtracking

Search remains bounded and deterministic. It evaluates complete hierarchical
candidates, not independent per-block winners. If the preferred candidate
fails contract, shared-resource, simulation, physical, or downstream
realization closure, the system records the structured rejection and evaluates
the next retained complete candidate.

The report must distinguish:

- provider expansion within a block;
- local block repair;
- global candidate backtracking;
- terminal unsupported, ambiguous, unsafe, unproven, and budget-exhausted
  outcomes.

Canonical inputs and policy must yield byte-identical hierarchy, contracts,
resource plan, candidate order, rejection evidence, and selected result.

## 8. Verification

Verification includes:

- block-scoped analyses for every derived contract;
- end-to-end analyses for every behavioral requirement;
- all declared supply, load, temperature, tolerance, startup, model, noise,
  thermal, timing, and fault corners;
- proof that every critical assertion is covered at every applicable corner;
- explicit model provenance and confidence;
- a system-level result only when every required block and end-to-end result
  passes.

No analytical estimate may be substituted for a required simulation without a
declared, policy-approved evidence class and explicit rationale.

## 9. Physical Co-Design

The physical plan derives partitions and boundary rules for analog, digital,
high-current, thermal, feedback, clock, protection, isolation, and
sensitive-node regions. It must express:

- compatible and incompatible co-location;
- return-path and reference requirements;
- separation, keepout, and crossing constraints;
- thermal coupling or separation;
- high-current width and transition requirements;
- feedback and sensitive-node locality;
- clock containment and fanout;
- protection placement relative to exposed interfaces.

The existing placement and routing systems consume these generic constraints.
Fixture coordinates, route allowlists, and benchmark-specific layout branches
are prohibited.

## 10. Frozen Held-Out Corpus

The corpus lives at
`internal/architecturesearch/testdata/hierarchical_multi_domain_corpus` and
contains six SHA-256-pinned V4 requirements:

1. a managed and protected Class-AB amplifier system;
2. a precision analog acquisition and decision system;
3. a regulated sensor, MCU, clock, and communications system;
4. an isolated mixed-voltage communications gateway;
5. a current-limited high-current switched-load controller;
6. a split-supply precision analog monitoring system.

Each contains at least four interacting functional obligations and at least
two electrical domains. The independent freeze test rejects implementation
details, subsystem hints, insufficient block/domain coverage, missing
environmental corners, missing acceptance gates, modified bytes, and
unmanifested fixtures.

The untouched baseline must show that production V3 rejects V4 and cannot emit
hierarchy, interface-contract, shared-resource, physical-partition, or
end-to-end traceability evidence.

## 11. Promotion Gates

Every held-out system must pass:

- deterministic decode, canonicalization, architecture selection, and replay;
- hierarchical, contract, shared-resource, backtracking, physical, and
  traceability evidence validation;
- block-level and end-to-end closed-loop verification across all corners;
- catalog identity, ratings, values, tolerances, and model provenance;
- complete lowering without lost semantic evidence;
- internal validation, connectivity, routing, and writer correctness;
- clean installed-KiCad ERC and strict DRC;
- zero normalized schematic and PCB round-trip differences;
- two clean-checkout promotion runs with identical normalized bundles.

The existing 12/12 held-out benchmark, amplifier lanes, ESP32/MCU/sensor
fixtures, protected USB-C fixtures, and installed-KiCad promotion matrix must
remain green.

Final closeout requires an evidence audit, Prism review with no unresolved
high or medium findings, a committed and pushed clean tree, and green GitHub
Actions for the exact pushed revision.
