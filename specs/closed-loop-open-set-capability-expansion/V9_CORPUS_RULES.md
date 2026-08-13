# V9 Fresh Behavior-Only Corpus Rules

## 1. Shape and independence

V9 has exactly 48 requirements: 24 discovery and 24 held-out. Six isolated
authors each receive eight fixed paths, four per partition. Requirements are
freshly authored from the V9 packet and may not derive from V1–V8 corpus text,
examples, fixtures, synthesis outputs, rankings, or other authors.

Each author context verifies its packet checksum before reading the assignment,
reads only packet-manifest inputs, writes only its designated empty quarantine,
and uses no synthesis, simulation, feasibility, classification, KiCad, network,
or repository inspection tool. Any isolation violation invalidates the complete
author bundle and requires a fresh replacement context.

## 2. Behavior-only boundary

A requirement may state externally observable behavior, ports, supply/reference
domains, operating cases, events, bounded assertions, environmental limits,
board envelope, safety impact, and acceptance gates.

It may not prescribe or hint at parts, values as implementation choices,
topology, known circuit-family realization, block names, symbols, footprints,
packages, placement, coordinates, routes, layers, templates, fixture IDs,
corpus IDs, expected outcomes, or KiCadAI internals. Electrical quantities
needed to define observable behavior are not implementation hints.

## 3. Frozen assignment dimensions

Each packet fixes requirement path, partition, reporting domain, circuit role,
safety impact, primary static/dynamic class, required analysis diversity,
output multiplicity, and off-nominal obligation. Authors must satisfy but not
change those assignments.

Across the corpus:

- every reporting domain and circuit role appears in both partitions;
- every author has at least two static-primary and two dynamic-primary cases;
- every author has meaningful multi-output behavior in both partitions;
- every author uses at least four distinct canonical analysis kinds;
- every author has at least two bounded off-nominal or event-driven cases; and
- safety-critical and safety-relevant cases are distributed across authors,
  roles, domains, and partitions.

Primary classification is derived from the lexicographically first primary
assertion under the frozen rule; cosmetic naming cannot alter it.

## 4. Stable semantics and references

Case, domain, port, operating-case, event, assertion, observation, and output
IDs are unique canonical tokens. Every reference resolves locally. Supply and
reference domains are explicit; every ground-referenced signal has a bounded
external return capacity consistent with its declared supply/interface limit.

Bounds are finite, ordered, dimensionally valid, physically coherent, and use
canonical units and analysis names. Each assertion is externally testable and
observes an existing port, output, or neutral whole-circuit state allowed by the
public schema. Events, conditions, observations, and assertions are coherent
within the same operating case.

## 5. Required behavioral diversity

The 48-case corpus must include meaningful coverage of static transfer,
transient response, frequency response, distortion/noise, stability, thermal or
electrothermal behavior, startup/shutdown, protection/fault response,
power/efficiency, digital timing or thresholds, sensing accuracy, and
bidirectional or multi-output interaction. Coverage is behavioral, not a list
of preferred circuit families.

Normalized semantic signatures must be unique across the complete corpus. An
author must not create paired discovery/held-out paraphrases. Aggregate
diversity is validated before publication without disclosing held-out content.

## 6. Acceptance profile

Every requirement enables all 14 mandatory gates:

1. contract validity;
2. deterministic planning;
3. topology/electrical rule validity;
4. component and model availability;
5. simulation/assertion evidence;
6. safety evidence;
7. schematic export correctness;
8. PCB export correctness;
9. clean ERC;
10. strict DRC;
11. exact connectivity;
12. route completion;
13. writer correctness; and
14. zero round-trip diffs.

Omission, disabling, `unknown`, or conditional bypass of a mandatory gate
invalidates the requirement rather than defining an outcome.

## 7. Validation and corrections

Packet-local validation checks exact object shape, canonical enums, bounds,
references, quotas, diversity, prohibited identities, exact file set,
assignment-ordered provenance, and full-byte hashes. Authors report only counts,
status, blockers, and disclosed uncertainties; held-out content is never sent
to the parent context.

Public adjudication may correct objective electrical or schema errors while
preserving assigned behavior. A correction changes only named requirements,
their source hashes, and authorship end timestamp. Blind aggregate correction
may add one canonical bounded assertion to one compatible held-out case only
when the frozen validator reports a missing aggregate category. Corrections stop
as soon as validation passes. No correction may target synthesis feasibility or
expected outcome.

## 8. Publication and secrecy

The publisher accepts only validated complete bundles and deterministic
assignment order. It publishes public discovery plaintext, record-encrypted
held-out data, manifest, aggregate obligation commitments, authorship
attestations, and checksums atomically without replacement.

External source, baseline, and final keys are distinct 32-byte 0600 regular
files under `~/.config/kicadai/closed-loop-open-set/v9/`. Keys and plaintext are
never committed, logged, printed, passed by argument/environment, or exposed to
implementation/Prism contexts. Quarantine plaintext is removed only after an
independent verifier authenticates every published record and manifest.
