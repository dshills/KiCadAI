# Closed-Loop Open-Set V3 Corpus Rules

Status: freeze candidate

## Neutrality

Every requirement describes externally observable electrical behavior only.
It may name semantic domains, ports, operating conditions/events, analyses,
metrics, bounds, board limits, and acceptance gates from the public schema.

It must not name or imply:

- a topology, block, component, part, package, model, footprint, symbol, net,
  layer, coordinate, route, placement, repair, or implementation algorithm;
- an expected synthesis status, failure code, capability cluster, rank, or
  expected pass;
- a project fixture or an existing example; or
- V1, V2, or another V3 requirement by identity or text.

## Isolation

The corpus author receives no repository access beyond a clean copy of the
public requirement schema/vocabulary and these rules. The author records the
exact provided inputs, tool/model identity if applicable, authoring time, and a
statement that no synthesis or outcome inspection occurred.

The implementation agent may run only mechanical validation over held-out
bytes: strict JSON/schema decoding, canonical enums, reference integrity,
bounds, acceptance completeness, source hashes, normalized-text uniqueness,
cross-version non-overlap, role/domain quotas, and the diversity properties
below. Mechanical diagnostics must not include held-out electrical content.

## Required balance

- 12 discovery and 12 held-out cases.
- Exactly two cases per role in each reporting domain: analog, power,
  digital/control, MCU/interface, sensor, and mixed-signal.
- Stable manifest order and unique neutral semantic IDs.
- All 14 acceptance booleans present and true.
- At least two operating cases and four behavioral assertions per case.
- At least two analysis kinds per case.

## Structural diversity

Within each role, the aggregate corpus must include, without assigning expected
outcomes:

- single-positive, bipolar, and multiple-supply declarations;
- voltage-, current-, and power-observed behavior;
- at least three requirements with multiple observed source ports;
- at least three requirements with distinct external excitations converging on
  one observed source port;
- DC, AC/noise/stability, transient/startup, and thermal/electrothermal
  analyses;
- load, tolerance/model, temperature, and supply variation;
- input/load/power steps, startup, rail-loss, and short-circuit events in the
  aggregate; and
- safety-critical assertions distributed across at least three reporting
  domains.

These are behavioral diversity constraints, not requested classifier labels.

## Feasibility neutrality

The author must avoid mathematical ideals such as zero-ohm environmental loads
unless the public behavior explicitly studies a short-circuit event. The author
must perform a public electrical sanity review for unit consistency, finite
bounds, reference use, energy conservation, and mutually compatible
all-corner requirements. This review may not use KiCadAI synthesis or infer an
implementation.

An intentionally adversarial requirement may be included only as a bounded
behavioral safety case and must not disclose its expected classification.

## Freeze evidence

The committed manifest records authoring provenance, source SHA-256 for every
request, role/domain/safety metadata, stable order, normalized-content hashes,
cross-version non-overlap results, schema/policy versions, the immutable
starting commit, and the exact acceptance/diversity check hash.
