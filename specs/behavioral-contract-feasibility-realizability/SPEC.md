# Behavioral-Contract Feasibility and Realizability Gate

Status: frozen implementation contract

## 1. Purpose

Closed-loop open-set V2 proved that a terminal `complete_topology` failure can
hide materially different causes. A direct-domain circuit, a circuit that must
create a new energy domain, and a circuit that must compose several independent
control or output obligations must not be ranked as one reusable gap merely
because bounded search ended at the same stage.

This milestone adds a deterministic, identity-neutral classifier between
normalized behavioral requirements and capability-gap aggregation. It refines
causal evidence without selecting a circuit family or claiming that an
unclassified request is feasible.

## 2. Scope

The gate shall:

1. derive conservative direct voltage envelopes from declared external
   reference and supply domains at every named operating corner;
2. identify output requirements that can only be met by creating an additional
   voltage or energy domain;
3. identify independent multi-output and multi-control obligation graphs;
4. retain assertion IDs, operating-case IDs, and analysis kinds as auditable
   evidence;
5. refine only terminal topology-search gaps, leaving stronger model,
   simulation, component, physical, routing, and verification root causes
   authoritative; and
6. produce stable typed capability gaps for the next versioned closed-loop
   policy.

It shall not:

- infer a named topology, component, part, package, coordinate, or fixture;
- reject a request merely because it is complex;
- treat absence of a finding as proof of feasibility;
- mutate the frozen V1 or V2 corpus, reports, hashes, policy versions, or
  outcomes; or
- expose or reuse any retired held-out requirement or result.

## 3. Classification contract

The normalized requirement is classified into zero or more findings. Findings
are not mutually exclusive.

### 3.1 Direct-domain eligible

No typed finding is emitted when the declared behavior can fit inside the
conservative direct external-supply envelope and its obligation graph has one
observed output and no converging independent controls. This means only that no
covered realizability gap was proven.

### 3.2 Energy-domain creation required

For each assertion and operating case, the classifier computes both the outer
reachable voltage bounds and the voltage range guaranteed directly at the
least-favorable declared external-supply corner. It emits
`OPEN_TOPOLOGY_ENERGY_DOMAIN_CREATION_REQUIRED` only when a required interval
has no overlap with the outer bounds, an all-corner floor or ceiling exceeds
the guaranteed bounds, or a required peak/swing exceeds the applicable direct
magnitude/span.

The guaranteed comparison uses the least favorable supply value for each
polarity. A supply range that crosses zero contributes no guaranteed polarity,
but its finite endpoints still constrain the outer reachable bounds and
absolute magnitude. This is intentional under all-corners acceptance: a rail
that may be zero or negative cannot guarantee positive direct-domain output at
that corner. Non-finite values are rejected by schema validation; a missing
supply endpoint is uncertain and cannot prove a finding. Port electrical
ratings are limits, not energy sources.

The initial metric set is deliberately closed and hand-checkable:

- `output_high_voltage` and `startup_output_voltage` minimums;
- bounded `output_voltage` intervals;
- `output_low_voltage` maximums below the guaranteed lower rail;
- `peak_voltage` minimum absolute magnitude, following its versioned production
  metric mapping, compared with the larger absolute outer rail bound; and
- `output_swing` minimum peak-to-peak span.

Unknown metrics remain unclassified rather than guessed.

No generic component headroom is subtracted. The gate proves only energy-domain
bounds; primitive-specific saturation, dropout, switch loss, and output
headroom remain the responsibility of catalog selection and trusted corner
simulation. This intentional zero-headroom bound can produce false negatives,
but not an unsupported energy-domain finding.

### 3.3 Multi-output composition required

If behavioral assertions observe more than one distinct source port, the gate
emits `OPEN_TOPOLOGY_MULTI_OUTPUT_COMPOSITION_REQUIRED`. Circuit-level thermal
observations do not create an electrical output by themselves. Evidence names
only the assertions, cases, and analyses participating in the distinct output
obligations.

### 3.4 Multi-control composition required

If two or more distinct excitation ports drive assertions that converge on the
same observed source port, the gate emits
`OPEN_TOPOLOGY_MULTI_CONTROL_COMPOSITION_REQUIRED`. Repeated assertions from
one excitation do not qualify. Only external excitation-to-observation edges
declared by behavioral assertions participate; inferred internal feedback and
sense paths are excluded, so a single-input closed loop is not mislabeled as
multi-control. Convergence is evaluated across the complete requirement,
including assertions assigned to different operating cases, because one
physical design must implement every named mode.

## 4. Capability-feedback policy boundary

Legacy `closed-loop-capability-policy-v1` observation and aggregation remain
unchanged so committed V1/V2 evidence reproduces exactly. The realizability
gate is exposed through a new versioned policy entrypoint.

For a synthesis run whose terminal root is topology search exhaustion or no
complete graph:

- energy-domain findings map to topology capability
  `energy_domain_creation`;
- multi-output and multi-control findings map to topology capability
  `multi_obligation_composition`;
- the original terminal code is retained as a downstream symptom; and
- an unclassified failure remains `complete_topology`.

If universal candidate diagnoses provide a stronger causal root, they take
precedence and the classifier does not replace them.

## 5. Determinism and trust

- Normalize before classification.
- Sort and compact all finding evidence by code, path, assertion, operating
  case, and analysis.
- Use no wall clock, map iteration order, random input, host data, or network.
- Derive gap evidence hashes only from already hash-bound requirement and
  synthesis evidence.
- Reject requirements that fail `Validate` for
  `kicadai.open-topology-requirement.v1` before classification.
- Never promote, write, or mutate a project from classification evidence.

## 6. Acceptance

Completion requires:

1. public synthetic tests for positive, negative, bipolar, crossing-zero, exact
   boundary, and uncertain supply envelopes;
2. positive and negative tests for multi-output and multi-control obligation
   graphs;
3. two-run deterministic classification equality;
4. versioned capability-feedback tests proving typed clustering and legacy V1
   behavior preservation;
5. fail-closed invalid-input tests;
6. no corpus identity, file path, topology family, component, coordinate, or
   allowlist in production logic;
7. focused open-topology and capability-feedback suites passing locally; and
8. Prism review with all actionable findings remediated before commit.

Installed-KiCad promotion is not required for this evidence-only classifier
because it cannot produce or modify a design. Existing protected fixture and
promotion evidence must remain unchanged.
