# Closed-Loop Open-Set Capability Expansion V4 Addendum

Status: freeze candidate; corpus authoring and synthesis prohibited

## 1. Inherited objective and retired evidence

V4 inherits `SPEC.md`, `CORPUS_RULES.md`, and every unchanged trust, safety,
promotion, and reproducibility criterion from V1 through V3. V1, V2, and V3
remain failed experiments. Their held-out sets are permanently retired and may
not be inspected, copied, transformed, relabeled, or reused as blind evidence.

V3 demonstrated strict discovery, held-out, and rank-one-affected uplift, but
failed its exact remaining-gap equality gate after a selected capability
exposed later diagnostic stages. V4 does not reinterpret that result as a pass.
It replaces exact equality prospectively with the public monotonic relation in
`V4_GAP_TRANSITION_PROTOCOL.md`. This rule is frozen before any V4 requirement
is authored or synthesized.

## 2. Immutable starting state

The outcome-changing production starting point is commit:

`3d2d9bb0e8ff3e68ae6a160c136030b5a3b6d7db`

That checkpoint includes the generic authored-active-control correction and a
passing unfiltered open-topology synthesis suite, including the historical
windowed-heating test. The V4 corpus, baseline, selection, implementation seal,
and final evidence must bind the full hash.

The frozen classifier commitments are:

- `internal/opentopologysynthesis/realizability.go` SHA-256:
  `054b2c63f60f0f0c75fd3da72db062120d324317d7fab4825d5a66a09b4b32c9`
- `internal/capabilityfeedback/observe.go` SHA-256:
  `308e1194b5dd49f836d9c04b232e58cc44e83cf65354b2dbbaf16caf89903a0a`
- `internal/capabilityfeedback/evaluate.go` SHA-256:
  `e2c130f2f8ad54e727b9665c4642119fea8e72a9fc12f7551659d87b5708381c`
- realizability contract manifest SHA-256:
  `7df4d9e7a342939200625a7ab637909d52f3fa09ddb24ed16cedfc102cf5897d`

The local prerequisite evidence at that checkpoint is:

- `go test ./internal/opentopologysynthesis -count=1 -timeout=12m`: pass;
- exact windowed-heating regression: pass;
- capability-feedback, simulation-model, and V3 specification packages: pass;
- installed-KiCad protected LED fixture: pass; and
- installed-KiCad protected I2C fixture: pass.

The fixture passes include clean ERC, strict DRC, connectivity, route
completion, writer correctness, deterministic replay, and zero round-trip
differences.

## 3. Frozen policies

V4 uses `closed-loop-capability-policy-v2-realizability` exclusively.
Observation uses `ObserveRealizabilityAware`; aggregation uses
`EvaluateRealizabilityAware`. Evidence from another policy version cannot be
mixed into a V4 aggregate.

V4 freezes its own byte-identical copies of the V3 impact registry and
synthesis ceilings. Reuse of the public policy is not reuse of prior corpus or
outcome evidence.

- normalized V4 impact-registry hash:
  `64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377`
- V4 synthesis-policy hash:
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`

## 4. Fresh isolated corpus

V4 contains 24 newly authored behavior-only requirements: 12 discovery and 12
held-out, with exactly two cases in each role for analog, power,
digital/control, MCU/interface, sensor, and mixed-signal reporting domains.

The isolated author receives only the committed V4 public authoring packet:
the public requirement schema and vocabulary, `V4_CORPUS_RULES.md`, quotas,
acceptance requirements, and a fresh V4 semantic-ID namespace. The author must
not receive repository implementation, earlier corpus bytes or outcomes, V4
synthesis behavior, or an expected rank or result.

Held-out source bytes remain quarantined from the implementation context after
mechanical validation. Only commitments, counts, quota evidence, and encrypted
baseline material cross the blind boundary before the implementation seal.

## 5. Budgets and acceptance gates

V4 retains these ceilings:

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

Every passing case requires primitive-only open topology, trusted multi-analysis
simulation, all corners, model provenance, closed-loop evidence, complete
routing/connectivity, writer correctness, zero round-trip difference,
installed-KiCad ERC and strict DRC, deterministic replay, and fail-closed
behavior.

## 6. Selection and implementation boundary

Discovery alone determines clustering, deterministic ranking, and rank-one
selection. The selection artifact freezes the full selected cluster identity
`(stage, scope, capability, code)`, its required evidence, affected discovery
cases, complete ranking tuple, policy hashes, corpus/baseline hashes, and the
generic expansion plan before held-out outcomes can be revealed.

Only the selected reusable capability and inseparable generic prerequisites may
change outcome-affecting production code. Production code and tests must not
contain V4 identities, paths, hashes, expected outcomes, coordinates,
allowlists, or named fixture/circuit families.

## 7. Success and failure

V4 completes only if frozen bytes and unchanged policies prove all of the
following:

- strict total discovery pass-count improvement;
- strict held-out pass-count improvement;
- strict pass-count improvement among rank-one-affected discovery cases;
- no baseline-pass regression;
- no baseline-unsafe case becomes pass;
- the V4 monotonic nonselected-gap relation for every case that remains
  nonpassing;
- byte-identical two-run synthesis evidence;
- two clean-root promotions for every new pass; and
- all local installed-KiCad and regression gates.

The monotonic gap relation removes only gaps matching the exact selected
cluster identity and requires the remaining baseline identity set to be a
subset of the final identity set. New gaps may be reported; an unrelated old
gap may not disappear, be renamed, or be reclassified. Required-evidence
changes create a different identity and do not satisfy preservation.

Discovery uplift must pass before held-out final validation begins. Any
revealed held-out final result consumes the V4 held-out set. No retry, corpus
mutation, budget drift, gate relaxation, or post-revelation tuning can convert
a failed run into a pass. Completion or failure writes a permanent audit marker
that blocks every V4 update mode.

GitHub Actions are not manually started or inspected. Local evidence is the
authoritative development gate under the standing project instruction.
