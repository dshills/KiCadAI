# Closed-Loop Open-Set V4 Corpus Rules

Status: freeze candidate; authoring prohibited until the contract commit

## Neutrality

Each requirement describes externally observable electrical behavior only. It
may use public semantic domains, ports, conditions/events, analyses, metrics,
bounds, board limits, and acceptance gates.

It must not name or imply:

- a topology, block, component, part, package, model, footprint, symbol, net,
  layer, coordinate, route, placement, repair, or implementation algorithm;
- a synthesis status, gap, capability cluster, rank, selected behavior, or
  expected pass/failure;
- a project fixture or existing example; or
- any V1, V2, V3, or other V4 requirement by identity or text.

## Isolation and provenance

The author receives a clean public packet containing only the requirement
schema/vocabulary, this file, role/domain quotas, acceptance requirements, and
the namespace `v4_case_001` through `v4_case_024`. The packet contains no
repository production code, earlier corpus, synthesis result, diagnostic,
selection artifact, or expected outcome.

The author records exact inputs, tool/model identity if applicable, authoring
time, and a declaration that no synthesis or outcome inspection occurred. A
single author may write both roles only if the held-out bytes are quarantined
immediately and never enter the implementation context.

## Mechanical validation boundary

Before freeze, validation is limited to strict JSON/schema decoding, canonical
enums, reference integrity, finite and ordered bounds, all acceptance fields,
source hashes, normalized-text uniqueness, cross-version non-overlap,
role/domain quotas, and the diversity properties below. It must not synthesize
a circuit, query production feasibility, or infer an expected classification.

Held-out diagnostics must disclose only file validity, manifest position,
counts, aggregate quota/diversity results, and hashes—not electrical content.

## Required balance

- 12 discovery and 12 held-out cases.
- Exactly two cases per role in each reporting domain: analog, power,
  digital/control, MCU/interface, sensor, and mixed-signal.
- Stable manifest order and 24 unique neutral semantic IDs.
- All 14 acceptance booleans present and true.
- At least two operating cases and four behavioral assertions per requirement.
- At least two analysis kinds per requirement.

## Structural diversity

Within each role, the aggregate must include, without assigning outcomes:

- single-positive, bipolar, and multiple-supply declarations;
- voltage-, current-, and power-observed behavior;
- at least three requirements with multiple observed source ports;
- at least three requirements with distinct external excitations converging on
  one observed source port;
- DC, AC/noise/stability, transient/startup, and thermal/electrothermal
  analyses;
- load, tolerance/model, temperature, and supply variation;
- input/load/power steps, startup, rail-loss, and short-circuit events; and
- safety-critical assertions across at least three reporting domains.

These are behavioral coverage constraints, never requested classifier labels.

## Public electrical sanity review

Using only electrical/schema meaning, the author checks unit consistency,
finite physical environmental bounds, reference integrity, energy
conservation, and compatibility of simultaneous all-corner requirements.
Mathematical ideals such as a zero-ohm load are prohibited unless the bounded
behavior explicitly defines a short-circuit event. An adversarial requirement
must remain a bounded behavioral safety case and must not disclose an expected
classification.

## Freeze evidence

The manifest records provenance, every source SHA-256, role/domain/safety
metadata, stable order, normalized-content hashes, V1-V3 non-overlap results,
schema and policy versions, immutable starting commit, public packet hash, and
the exact acceptance/diversity validator hash. Corpus bytes cannot change after
the manifest commit.
