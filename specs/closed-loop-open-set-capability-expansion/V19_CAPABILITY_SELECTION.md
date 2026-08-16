# V19 Capability Selection

Status: selected from V18 public generation-one evidence; no V19 implementation
is authorized by this report.

## Decision

Select **generic causal topology repair** as the single V19 capability target.

The exact typed capability atom `causal_topology_repair` is the strongest
bounded target in the V18 report:

- 5 affected nonpassing cases;
- 4 reporting domains;
- 4 circuit roles;
- all 5 cases have `causal_topology_repair` as their only frontier capability;
- all 5 fail with `OPEN_TOPOLOGY_REPAIR_EXHAUSTED`; and
- the set spans non-safety, review-required, and safety-relevant requirements.

This is a stronger selection than choosing a broad failure family. The larger
analysis/model/solver family affects more cases, but it contains six distinct
typed capabilities and several analysis kinds. Treating that family as one
implementation would hide scope and make an unlock claim that the V18 evidence
does not support.

## Evidence boundary

The ranking and selection use only the committed V18 public generation-one
report:

- path:
  `internal/capabilityfeedback/testdata/closed_loop_open_set_v18_generation_one/report.json`
- report hash:
  `82cb1847bd6b90511aea06b4034dfe1d60c5968f8c33fa9a2e3258e3e9ff5794`
- file SHA-256:
  `332983874f65d84099f5f7a8740b9dd815aa6e892358f784be24a6c043f8edad`
- evaluated implementation commit:
  `e7d5e96c9cd6f5ee573b4cbdbe7ac0b042cbd111`
- publication commit:
  `f414f2fc560047e95dbc83928334564c8368a3e6`
- population: 24 public cases, comprising 1 pass and 23 nonpassing cases.

No corpus was authored or changed. No held-out key, record, result, or other
non-public evaluation evidence was accessed. No synthesis or KiCad evaluation
was run for this selection.

The proposed quantitative acceptance limits are copied from the public V18
production policy and causal-repair constants at the evaluated source commit;
they do not add evidence to the ranking.

## Method

For every nonpassing case, the terminal capability and code from every typed
frontier path were extracted. Related atoms were grouped into root-cause
families for coverage accounting. Cases may appear in multiple families when
their frontier identifies more than one independent blocker.

Two counts prevent overclaiming:

- **affected cases**: the family appears anywhere in the case frontier;
- **single-family cases**: every frontier atom in the case belongs to that
  family, so removing the family would expose either a pass or a later typed
  blocker.
- **single-capability cases**: every frontier atom is the same exact capability
  atom; this stricter count is used only for the exact-atom ranking.

Implementable atoms were ranked independently by this deterministic tuple:

1. affected case count, descending;
2. reporting-domain count, descending;
3. single-capability case count, descending; and
4. canonical capability name, ascending.

The word “unlock” below means a conservative candidate unlock, not a promised
pass: the selected capability is the case's only currently reported frontier
atom. Physical pass status still requires every existing gate.

## Root-cause clusters

| Rank | Root-cause family | Affected cases | Domains | Single-family cases | Typed atoms |
|---:|---|---:|---:|---:|---|
| 1 | Analysis/model/solver availability | 9 | 5 | 7 | `dc_operating_point_model`, `dc_operating_point_solver`, `ac_sweep_solver`, `dc_sweep_solver`, `transient_solver`, `stability_solver` |
| 2 | Topology construction and repair | 6 | 4 | 6 | `causal_topology_repair`, `complete_topology` |
| 3 | Assertion-directed performance repair | 6 | 5 | 3 | output/startup voltage, peak current, settling time, bandwidth, transconductance, and transimpedance evidence |
| 4 | Thermal/electrothermal evidence | 5 | 3 | 2 | `thermal_model`, `electrothermal_model`, `electrothermal_solver`, `junction_temperature_evidence` |
| 5 | Catalog value coverage | 1 | 1 | 1 | `catalog_value_domain` |

The five families cover all 23 nonpassing cases. Their affected-case counts
sum to more than 23 because four cases contain blockers from multiple families.

Family membership by public case:

- Analysis/model/solver availability: cases 001, 002, 008, 010, 011, 014,
  015, 020, and 023.
- Topology construction and repair: cases 009, 012, 016, 017, 018, and 021.
- Assertion-directed performance repair: cases 003, 006, 019, 020, 022, and
  024.
- Thermal/electrothermal evidence: cases 003, 007, 013, 019, and 023.
- Catalog value coverage: case 004.

## Exact capability ranking

The leading exact atoms are:

| Rank | Capability | Affected cases | Domains | Single-capability cases |
|---:|---|---:|---:|---:|
| 1 | `causal_topology_repair` | 5 | 4 | 5 |
| 2 | `electrothermal_model` | 4 | 3 | 1 |
| 3 | `dc_operating_point_solver` | 3 | 2 | 2 |
| 4 | `output_voltage_evidence` | 2 | 2 | 1 |
| 5 | `transient_solver` | 2 | 2 | 1 |

Five other atoms affect two cases but rank below the displayed set because
they cover fewer domains or have no single-capability case. Every remaining
atom affects one case.

The selected atom appears in cases 009, 016, 017, 018, and 021. Those cases
span `digital_control`, `mixed_signal_data_conversion`,
`protection_power_integrity`, and `sensing_instrumentation`, and the roles
`sensing_measurement`, `protection_supervision`, `source_bias`, and
`amplification_conditioning`.

Case 012 is adjacent topology evidence but is not claimed as a selected-atom
unlock because its frontier is `complete_topology`, not
`causal_topology_repair`.

## V19 capability boundary

V19 should add a bounded, deterministic repair operator that derives candidate
graph changes from unresolved behavioral obligations, electrical-domain
compatibility, port direction, and causal reachability. It may repair an
incomplete primitive graph only through reusable graph operations and must
retain canonical ordering and the V18 evaluator's frozen policy limits.

The V19 freeze may tighten but must not raise the current bounds:

- 20,000 expanded states and 50,000 generated graphs;
- 32 primitive instances and 32 internal nodes per graph;
- 50,000 candidate simulations, 65,536 corner evaluations, and 4,096 value
  trials;
- 128 topology repairs and 16 retained candidates; and
- for causal repair specifically, 48 single trials, beam width 8, and at most
  2 graph changes per proposal. The two-change bound is proposal width, not
  total repair depth: successive accepted proposals may compose within the
  overall 128-topology-repair budget.

Tests must assert reported consumption against these numeric limits rather
than depending on wall-clock time.

These are independent total-consumption ceilings, not a target ratio between
simulations and corners. A rejected candidate may consume a simulation without
reaching corner expansion; every authored corner remains mandatory before a
candidate can pass. The 65,536 value is a fail-closed ceiling, never permission
to sample or omit corners: reaching it before all required corners are evaluated
must produce exhaustion, not a pass. Raising either frozen limit is outside this
selection goal and would require separate evidence and authorization.

The capability must not contain corpus IDs, requirement IDs, fixture names,
coordinates, expected outcomes, circuit-family templates, block allowlists, or
special-case schemas. It must not broaden component/model evidence, change
simulation semantics, or weaken safety, fail-closed, or physical-promotion
gates.

Primitive eligibility must come from the existing environment-supplied,
catalog-backed primitive inventory and reviewed model-provenance registry. The
repair operator may not maintain a private primitive list.

## Proposed acceptance tests

### Generic unit and property tests

1. Given incomplete synthetic graphs across at least four electrical domains
   and four circuit roles, repairs are derived only from public behavioral
   obligations and graph semantics.
2. Repeated execution produces byte-identical ordered repair candidates and
   canonical graph hashes.
3. A repair may not introduce a causal cycle, active-output contention,
   incompatible domain crossing, floating external reference, unbound port, or
   unsupported primitive.
4. Complete graphs and V18-ineligible requirements delegate byte-for-byte to
   the V18 path with the unchanged legacy inventory and simulation environment.
5. Exhausted search returns the existing typed
   `OPEN_TOPOLOGY_REPAIR_EXHAUSTED` evidence; it never manufactures a pass.
6. A synthetic maximum-size stress graph contains 32 primitive instances and
   32 internal nodes, high fan-out at every eligible branch point, and a deep
   feedback back-edge. The test uses the original order, reverse order, every
   cyclic rotation, and 256 Fisher-Yates permutations from fixed seed `19019`;
   it rejects the causal cycle and proves that every reported consumption
   field remains within the numeric V18 bounds above.
7. A separate beam stress fixture must create more than 48 otherwise eligible
   single-change proposals and prove deterministic truncation to 48 trials,
   beam width 8, no proposal with more than 2 changes, and identical hashes
   across input-order permutations.
8. A synthetic causal graph that requires three independent graph changes
   must resolve through at least two successive proposals. No proposal may
   exceed two changes, total topology repairs may not exceed 128, and replay
   hashes must remain identical. This proves bounded compositional depth
   without speculatively increasing the V18 proposal-width limit.

### Public V18 replay acceptance

1. Authenticate and reuse the exact 24-case V18 public report/corpus boundary;
   do not author a new corpus and do not access held-out material.
2. Evaluate every case exactly twice under one committed V19 environment.
3. All five selected cases must either pass or advance to a strictly later,
   non-`causal_topology_repair` typed frontier. A remaining
   `causal_topology_repair` frontier in any selected case fails V19 admission.
4. In addition to the mandatory five-of-five advancement gate in item 3, at
   least two selected cases must become complete passes, and those passes must
   cover at least two reporting domains. Fewer than two passes fails admission
   even if all five cases advance. The other selected cases may reveal later
   typed blockers, but those blockers must be deterministic and must not be
   hidden by a generic exhaustion code.
5. Preserve every V18 pass, unsafe outcome, and satisfied typed obligation.
6. Every pass must have two identical synthesis replays and two installed-KiCad
   promotions with clean ERC, strict DRC, connectivity, route completion,
   writer correctness, and zero round-trip differences.
7. The complete local Go suite, historical replay seals, deterministic replay,
   and fail-closed tests must remain green without changing historical hashes.

## Estimated execution cost

This estimate is for a later, separately authorized V19 implementation goal:

| Work | Estimate |
|---|---:|
| Freeze V19 boundary and focused synthetic fixtures | 3–4 hours |
| Generic repair operator and deterministic ranking | 16–24 hours |
| Unit/property tests and historical delegation proofs | 6–8 hours |
| Local regression and installed-KiCad promotion | 2–3 hours |
| One 24-case, two-replay public run | 2–3 hours wall time |
| Prism remediation and evidence publication | 1–2 hours |
| **Total** | **30–44 engineering hours; about 4–6 working days** |

Execution should have a hard stop after one implementation run and one bounded
correction run. If the selected cases do not meet the advancement gate, V19
should publish a fail-closed retirement report instead of starting another
corpus or protocol cycle.

## Next authorized action

None. This report intentionally stops at selection. A separate goal must freeze
the V19 specification and implementation plan before production code changes.
