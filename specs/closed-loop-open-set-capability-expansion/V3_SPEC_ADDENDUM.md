# Closed-Loop Open-Set Capability Expansion V3 Addendum

Status: freeze candidate; corpus not yet authored

## 1. Inherited objective

V3 inherits `SPEC.md`, `CORPUS_RULES.md`, and every unchanged trust and success
criterion from V1/V2. It does not reinterpret the failed V1 or V2 experiments
as passes. Their held-out sets remain retired and may not be inspected, copied,
transformed, relabeled, or reused as blind evidence.

## 2. Immutable starting state

The outcome-changing production starting point is commit:

`859c8df068db8254b715042b691c441a0d135fab`

That checkpoint includes the completed behavioral-contract feasibility and
realizability gate and its audit. The V3 corpus, baseline, selection, final
comparison, and promotion evidence must bind this full commit hash.

The classifier implementation commitments are:

- `internal/opentopologysynthesis/realizability.go` SHA-256:
  `054b2c63f60f0f0c75fd3da72db062120d324317d7fab4825d5a66a09b4b32c9`
- `internal/capabilityfeedback/observe.go` SHA-256:
  `308e1194b5dd49f836d9c04b232e58cc44e83cf65354b2dbbaf16caf89903a0a`
- `internal/capabilityfeedback/evaluate.go` SHA-256:
  `e2c130f2f8ad54e727b9665c4642119fea8e72a9fc12f7551659d87b5708381c`
- realizability contract manifest SHA-256:
  `7df4d9e7a342939200625a7ab637909d52f3fa09ddb24ed16cedfc102cf5897d`
- V3 implementation manifest SHA-256:
  `d06997de15c5afe71853058124f9b30a6afdd018fcf09d3a6da2e7df57d88b28`

## 3. Versioned evaluation policy

V3 uses `closed-loop-capability-policy-v2-realizability` exclusively.
Observation must use `ObserveRealizabilityAware`; aggregation must use
`EvaluateRealizabilityAware`. Mixing policy-v1 and policy-v2 case evidence in
one aggregate is invalid.

The V3 impact registry is versioned independently and must contain these
identity-neutral relationships in addition to the inherited graph:

- `energy_domain_creation` feeds `complete_topology`,
  `passing_behavioral_evidence`, and `physical_realization`;
- `multi_obligation_composition` feeds `complete_topology`,
  `passing_behavioral_evidence`, and `physical_realization`.

The exact normalized registry bytes and hash must be committed before baseline
execution.

- normalized V3 impact-registry hash:
  `64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377`
- frozen synthesis-policy hash:
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`

## 4. Fresh isolated corpus

V3 contains 24 new behavior-only requirements: 12 discovery and 12 held-out,
with exactly two cases in each role for analog, power, digital/control,
MCU/interface, sensor, and mixed-signal reporting domains.

The corpus author must work in a context isolated from:

- V1/V2 requirement bytes, identities, outcomes, diagnostics, reports, and
  selection artifacts;
- all V1/V2 held-out material;
- production topology, simulation, repair, and realizability implementation;
- the V3 implementation conversation and any expected outcome; and
- existing corpus examples or fixture schematics.

The author may receive only the public requirement schema/vocabulary, the
normative neutral authoring rules, role/domain quotas, acceptance requirements,
and a fresh semantic-ID namespace. No author may synthesize a candidate or
inspect baseline behavior while writing the corpus.

Held-out source bytes must be quarantined from the implementation agent after
mechanical validation. Only hashes, counts, aggregate quota evidence, and
encrypted baseline commitments may cross the blind boundary.

## 5. Frozen budgets and gates

V3 retains the V2 synthesis ceilings:

- 4,000 expanded states;
- 8,000 generated graphs;
- 24 primitive instances;
- 24 internal nodes;
- 128 candidate simulations;
- 2,048 corner evaluations;
- 128 value trials;
- 16 topology repairs;
- 8 retained candidates; and
- 32 diagnostic samples.

It retains all acceptance gates: primitive-only open topology, trusted
multi-analysis simulation, all corners, model provenance, closed-loop evidence,
complete routing/connectivity, writer correctness, zero round-trip difference,
installed-KiCad ERC and strict DRC, deterministic replay, and fail-closed
behavior.

## 6. Selection and implementation boundary

Discovery alone determines clustering, ranking, and rank-one selection. The
selection artifact seals the complete ranking tuple, affected discovery cases,
required evidence, policy/registry hashes, corpus/baseline hashes, and expansion
plan before any held-out result can be revealed.

Only the selected reusable capability and inseparable generic prerequisites may
change production behavior. Production code and tests must not contain V3
identities, paths, hashes, expected outcomes, coordinates, allowlists, or named
fixture/circuit families.

## 7. Success and failure

V3 completes only if frozen bytes and unchanged policy prove:

- strict total discovery pass-count improvement;
- strict held-out pass-count improvement;
- strict pass-count improvement among rank-one-affected discovery cases;
- no baseline-pass regression;
- preserved unsafe evidence and stable remaining gaps;
- byte-identical two-run synthesis evidence;
- two clean-root promotions for every new pass; and
- local installed-KiCad ERC, strict DRC, connectivity, route completion,
  writer correctness, zero round-trip differences, and replay.

Discovery uplift must pass before held-out final validation starts. Any revealed
held-out final result consumes the V3 held-out set. No retry, corpus mutation,
budget drift, gate relaxation, or post-revelation tuning may convert a failed
run into a pass.

GitHub Actions are not manually started or inspected; local evidence is the
authoritative development gate under the standing project instruction.
