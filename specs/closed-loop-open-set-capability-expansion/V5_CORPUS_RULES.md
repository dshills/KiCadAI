# Closed-Loop Open-Set V5 Corpus Rules

Status: freeze candidate; authoring prohibited until the contract commit

## Neutral behavior only

Every requirement describes externally observable electrical behavior using
only the public requirement schema: semantic domains, ports, operating cases,
events, analyses, metrics, bounds, board limits, and the inherited acceptance
profile.

A requirement must not name or imply a topology, component, part, package,
model, footprint, symbol, net, layer, coordinate, route, placement, repair,
provider, algorithm, outcome, diagnostic, gap, capability, package rank,
expected implementation, fixture, example, or earlier requirement.

## Independent authorship

Three isolated authors each produce 12 requirements: one discovery and one
held-out case in each of the six reporting domains. Each author receives a
disjoint 12-ID allocation from `v5_case_001` through `v5_case_036` and may not
see another allocation's content.

Author inputs are limited to the frozen V5 packet, public schema/vocabulary,
role/domain assignments, acceptance requirements, and these rules. Authors may
not access repository implementation, V1-V4 corpus bytes or semantic summaries,
any baseline/final result, selected capability, development conversation, or
expected classification. Each author records exact inputs, tool/model identity
if used, time, file hashes, and a no-synthesis/no-outcome-inspection declaration.

Held-out files enter author-specific quarantine immediately. The implementation
context receives no held-out plaintext, filename-to-behavior summary, or
diagnostic before the one-time final boundary.

## Mechanical and public sanity validation

Before freeze, validation is limited to strict JSON/schema decoding, canonical
enums, reference integrity, finite ordered bounds, complete acceptance fields,
source hashes, exact and normalized uniqueness, quotas, and the diversity rules
below. Historical non-overlap is performed by a corpus custodian without
disclosing retired content.

Historical non-overlap compares new semantic hashes only with previously frozen
commitments. It must not open, decrypt, reconstruct, or summarize any retired
held-out source.

Public electrical sanity review may check units, physical environmental bounds,
reference integrity, energy consistency, and simultaneous-corner coherence. It
must not run synthesis, query production feasibility, predict classification,
or recommend implementation changes.

## Required balance

- Exactly 18 discovery and 18 held-out cases.
- Exactly three cases per role in each reporting domain: analog, power,
  digital/control, MCU/interface, sensor, and mixed-signal.
- Exactly one case per author, role, and reporting domain.
- Thirty-six unique neutral semantic IDs and stable manifest order.
- All inherited 14 acceptance booleans explicitly present and true.
- At least two operating cases, four behavioral assertions, and two analysis
  kinds per requirement.

## Diversity within each role

The aggregate must include:

- single-positive, bipolar, and multiple-supply declarations;
- voltage-, current-, and power-observed behavior;
- at least five requirements with multiple observed source ports;
- at least five requirements with distinct excitations converging on one
  observed source port;
- DC, AC/noise/stability, transient/startup, and thermal/electrothermal work;
- load, tolerance/model, temperature, and supply variation;
- input, load, and power steps, startup, rail-loss, and bounded short-circuit
  events; and
- safety-critical assertions in at least four reporting domains.

Within each author contribution, at least four analysis kinds and three event
kinds must appear across both roles. Discovery and held-out cases from the same
author/domain must not be normalized paraphrases or share identical port,
assertion, and analysis signatures.

## Freeze evidence

The manifest records author identity/provenance commitments, source SHA-256,
role/domain/safety metadata, stable order, semantic and normalized hashes,
cross-author and cross-version non-overlap results, packet/validator hashes,
schema/policy versions, and starting/contract-freeze commits.

Any requirement, manifest, assignment, validator, quota, or authorship change
after the corpus commit creates a new experiment version and requires a fresh
baseline. Invalid or uncertain corpus evidence fails closed.
