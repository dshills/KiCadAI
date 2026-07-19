# Adversarial Multi-Function Circuit Composition Specification

## 1. Purpose

This milestone extends open-set architecture search from one independently
selected function to circuits whose analog, digital, power, protection, and
interface objectives interact. The search must prove the compatibility of the
whole architecture, not merely concatenate individually valid fragments.

Completion is demonstrated against a frozen behavior-only corpus selected
before production implementation. Passing the corpus establishes a measured
multi-function envelope; it does not establish arbitrary circuit generation.

## 2. Required Outcomes

The implementation must:

- compose two to four objective providers through public typed contracts;
- represent internal behavioral signals without naming nets or prescribing a
  topology;
- propagate power-domain sources and loads across selected fragments;
- prove voltage, current, loading, startup, stability, tolerance, and board
  constraints for each complete candidate;
- retain materially distinct deterministic alternatives and selection
  rationale;
- report selected, rejected, unsupported, ambiguous, and budget-exhausted
  capability requests;
- fail closed when any safety-relevant global obligation is unknown or fails;
- lower a selected architecture through the existing resolver, writer,
  round-trip, connectivity, routing, ERC, and strict-DRC pipeline.

## 3. Non-Goals And Prohibitions

This milestone does not permit:

- fixture IDs, project names, corpus paths, or corpus hashes in production;
- expected component, provider, fragment, symbol, footprint, pin, pad, net,
  coordinate, layer, track, route, or via data in a requirement;
- topology-specific request schemas or capability-specific mapper branches;
- implicit voltage conversion, protection, isolation, buffering, or power
  sourcing;
- treating independent fragment validity as proof of system validity;
- selecting an unsafe or unproven candidate because it scores well;
- weakening existing acceptance gates or editing a frozen input to make it
  pass.

## 4. Public Behavioral Contract

The public schema is `kicadai.open-set-requirement.v2`. It retains v1 domains,
external ports, abstract participants, generic objectives, constraints, board
limits, and acceptance gates. It adds two capability-neutral constructs.

### 4.1 Internal signals

`requirements.signals` declares behavioral interfaces between objectives. A
signal has an identity, semantic kind, domain, electrical bounds, and optional
protocol. It does not declare a circuit net, topology, implementation, or
component. Each signal binding declares the endpoint direction (`source`,
`sink`, or `bidirectional`), allowing one producer and one or more compatible
consumers to share a typed search anchor.

Every non-reference signal requires exactly one source in a complete
candidate. Multiple direct sources are invalid. A sink-only signal, an
unconsumed safety-control signal, and direction or contract incompatibility are
blocking diagnostics.

### 4.2 System constraints

`requirements.system_constraints` expresses capability-neutral whole-circuit
requirements using the existing name/relation/value/unit/tolerance contract.
Examples include total quiescent power, startup output state, supply-current
headroom, ambient range, stability margin, and aggregate board limits.
Providers cannot define private extensions to this contract.

Domain `source` is either `external` or the identity of a power signal. A
derived domain is valid only when the referenced signal has one compatible
source and the global current and voltage proofs pass.

## 5. Composition Semantics

Search converts participants and objectives into obligations exactly as in
v1. External-port, participant-port, and signal bindings all become typed
anchors. Providers receive only normalized capability, port contracts,
generic constraints, and public board limits. They never receive fixture
identity.

A complete candidate is globally valid only when:

1. every obligation has one selected provider expansion;
2. every binding is preserved and every signal has valid source/sink
   cardinality;
3. all connected port contracts agree on kind, direction, domain, voltage,
   current, impedance, frequency, protocol, default state, and required traits;
4. every derived power domain has a proven source and its worst-case demand is
   within source capacity with required margin;
5. startup defaults cannot unintentionally enable a protected or power output;
6. amplifier/filter loading and stability requirements have evidence at
   nominal and tolerance corners;
7. selected component count, estimated area, and stated system budgets pass;
8. all safety-relevant evidence meets the declared minimum confidence.

Validation runs for every complete candidate before scoring. Global failures
are structured candidate rejections, never post-selection warnings.

## 6. Coverage Accounting

The deterministic result includes one coverage record for every participant,
objective, and provider-generated child obligation. Each record contains its
path, capability, terminal state, and bounded evidence. Terminal states are:

- `selected`: present in the selected complete architecture;
- `rejected`: providers were considered but every expansion violated a hard
  contract;
- `unsupported`: no registered provider supplies the capability;
- `ambiguous`: multiple policy-equivalent safety-relevant choices require user
  input;
- `budget_exhausted`: bounded search ended before a conclusion.

Aggregate counts must equal the number of terminal coverage records. Ordering
is canonical and replay must be byte-identical.

## 7. Alternatives And Rationale

Complete candidates remain subject to the v1 fail-first lexicographic score.
Alternatives must have distinct canonical architecture fingerprints. The
result explains the first substantive score field that separates each retained
alternative. A fingerprint tie-break is permitted only after all substantive
fields tie. Safety failures are rejected before this comparison.

## 8. Fail-Closed Rules

No lowered intent may be emitted when any of the following is true:

- a capability is unsupported, ambiguous, or unresolved;
- search or tolerance budgets are exhausted;
- a signal has invalid direction or source cardinality;
- voltage-domain provenance is missing or contradictory;
- aggregate worst-case current exceeds a source rating or required margin;
- startup state can enable an unsafe output;
- loading, drive, stability, or tolerance evidence is missing or fails;
- protection/isolation traits are required but unproven;
- board or component-count limits fail;
- lowering loses a binding, calculation, component, or evidence record;
- any required downstream validation gate fails.

## 9. Frozen Held-Out Corpus

The corpus lives at
`internal/circuitgraph/testdata/adversarial_multi_function_composition_corpus`.
It contains ten requirements, each with two to four interacting objectives:

1. regulated, translated, and transient-protected sensor controller;
2. hysteretic comparator, protected MOSFET load control, and fault indication;
3. split-supply filtered analog front end;
4. active filter, amplifier, and protected output;
5. MCU-controlled Class-AB bias, mute, and output protection;
6. dual-threshold battery-window load disconnect;
7. regulated, isolated, translated, and protected digital gateway;
8. regulated Class-A line driver with output protection;
9. precision sensor amplifier, filtering, thresholding, and indication chain;
10. current-sensed protected load driver with fault indication.

The manifest and every fixture are SHA-256 pinned. The freeze test strictly
decodes the independent pre-implementation schema mirror, proves objective
counts and required representative coverage, rejects implementation details,
and detects unmanifested files. Any corpus change requires a new specification
revision and an explicit re-baseline.

## 10. Promotion Gates

Every held-out requirement must provide authoritative evidence for:

- component identity, rating, value, and tolerance resolution;
- nominal and worst-case global electrical reasoning;
- complete lowering and byte-identical deterministic replay;
- writer correctness and zero semantic round-trip differences;
- complete connectivity and routing;
- clean KiCad ERC and strict DRC.

All existing open-set, amplifier, ESP32, MCU, analog, sensor, USB-C,
fabrication, writer, routing, and round-trip suites must remain green. Final
closeout requires Prism review, a committed and pushed tree, and green GitHub
Actions for that exact commit.
