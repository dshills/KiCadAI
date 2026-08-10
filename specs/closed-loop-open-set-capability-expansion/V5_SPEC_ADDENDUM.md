# Closed-Loop Open-Set Capability Expansion V5 Addendum

Status: freeze candidate; corpus authoring and synthesis prohibited

## 1. Objective and evidence boundary

V5 inherits `SPEC.md` and every unchanged safety, physical-promotion,
reproducibility, and fail-closed requirement from V1 through V4. V4 proved
strict public discovery uplift but failed its one-time blind final gate. The V4
held-out source, baseline, outcome, gap, diagnostic, and comparison evidence is
consumed and permanently unavailable to V5 authors, implementers, reviewers,
selection logic, prompts, and tests.

The only admissible V4 final fact is the public audit state: the one-time gate
failed and its updater is retired. V5 must not infer why it failed. It may use
public V4 discovery evidence and production code that was committed before the
blind run.

## 2. Immutable starting state

V5 begins at commit:

`d8e98b4dee3212823525c5955e8e025bd0039d03`

The commit includes the generic convergent-topology implementation, the
reviewed V4 evaluator lifecycle, and the non-revealing V4 failure audit. Before
V5 corpus authoring, the contract freeze binds:

- `internal/opentopologysynthesis/realizability.go` SHA-256
  `054b2c63f60f0f0c75fd3da72db062120d324317d7fab4825d5a66a09b4b32c9`;
- `internal/capabilityfeedback/observe.go` SHA-256
  `308e1194b5dd49f836d9c04b232e58cc44e83cf65354b2dbbaf16caf89903a0a`;
- `internal/capabilityfeedback/evaluate.go` SHA-256
  `e2c130f2f8ad54e727b9665c4642119fea8e72a9fc12f7551659d87b5708381c`;
- V4 impact-registry file SHA-256
  `c0229f216b3024627992327ddaa90f44df7f3f1f97412d05b22161284d15afa0`;
- V4 synthesis-policy file SHA-256
  `7e415c9a6b6d30142840c8bd56e598db70b1a2103bc663ccd73df762871cbb66`;
- V4 gap-transition-policy file SHA-256
  `ba73b2db190f48c70b31bc77b7689240df122f73b41e8b63624e540635139aa8`;
  and
- V4 retirement-audit SHA-256
  `ff3ba5f1d1895b7c733aee02f99b41f21af70316b01414f5e673919eba85e1e8`.

No outcome-affecting production change is allowed between this starting point
and the V5 discovery baseline except infrastructure expressly required to
encode and reproduce the frozen package-ranking protocol.

## 3. Fresh independently authored corpus

V5 contains 36 new behavior-only requirements: 18 discovery and 18 held-out.
Three context-isolated authors each contribute exactly one discovery and one
held-out case in each reporting domain: analog, power, digital/control,
MCU/interface, sensor, and mixed-signal. An author may not see another author's
requirements.

Authors receive only the committed V5 public authoring packet. They receive no
repository implementation, prior corpus bytes or semantic summaries, public
baseline outcomes, selected gaps, implementation history, or blind-validation
result beyond the rule that all earlier held-out evidence is prohibited.

Held-out source bytes are encrypted and removed from the implementation
workspace immediately after mechanical validation. A corpus custodian may
perform content-neutral schema, quota, provenance, and uniqueness checks but
may not synthesize or report expected feasibility. Historical non-overlap uses
only previously frozen semantic commitments; no retired source or key may be
opened for comparison.

## 4. Frozen evaluator and synthesis limits

V5 uses `closed-loop-capability-policy-v2-realizability`,
`ObserveRealizabilityAware`, and `EvaluateRealizabilityAware`. It reuses the
public V4 impact registry, synthesis ceilings, and monotonic gap identity
semantics byte-for-byte. Reuse of public policy is not reuse of corpus evidence.

The ceilings remain 4,000 expanded states, 8,000 generated graphs, 24 primitive
instances, 24 internal nodes, 128 candidate simulations, 2,048 corner
evaluations, 128 value trials, 16 topology repairs, 8 retained candidates, and
32 diagnostic samples.

Every pass requires the complete inherited 14-gate acceptance profile,
trusted multi-analysis simulation at all requested corners, model provenance,
closed-loop evidence, deterministic replay, complete physical realization,
clean ERC, strict DRC, connectivity, route completion, writer correctness,
zero normalized round-trip differences, and two clean-root installed-KiCad
promotions.

## 5. Capability-package ranking

V5 ranks reusable capability packages rather than isolated diagnostic codes. A
package identity is the length-delimited tuple `(scope, capability)`. Its
members are all discovery root-gap clusters with exact identities
`(stage, scope, capability, code)` that share that package identity.

Package evidence is computed only from discovery:

- affected cases are the unique union of member-cluster cases;
- reporting domains and safety impact are computed over that union;
- required evidence is the sorted unique union of member requirements;
- member identities remain explicit and sorted; and
- no held-out count, content, outcome, or gap enters eligibility or ranking.

A package is coverage-eligible only when it affects at least two discovery
cases across at least two reporting domains. Ranking then follows
`V5_SELECTION_POLICY.json`. After ranking, the frozen planner must produce one
executable generic expansion plan covering every member of the exact rank-one
package. If no package is eligible, or rank one lacks that complete plan, V5
closes fail-closed; it may not select an easier package or author more cases.

## 6. Implementation boundary

The selection artifact freezes the package identity, every exact member
identity, affected discovery cases and domains, complete ranking tuple,
required evidence, generic plan, corpus/baseline/policy hashes, and freeze
commits before any held-out baseline evidence can enter implementation context.

Only the selected package and inseparable prerequisites named by the plan may
change outcome-affecting code. Production code and tests must not contain V5
case identities, corpus paths or hashes, expected results, fixture coordinates,
allowlists, named circuit families, or block-family dispatch.

## 7. Final proof and retirement

Discovery final evaluation must first prove:

- strict total discovery pass-count improvement;
- strict pass-count improvement among selected-package-affected discovery
  cases;
- no baseline pass regression;
- no baseline unsafe case becoming pass;
- exact case-set preservation and monotonic preservation of every nonselected
  baseline gap identity; and
- complete deterministic and physical-promotion evidence for every pass.

Only then may the reviewed implementation open the held-out final boundary
once. V5 succeeds only if total held-out pass count strictly improves and at
least one newly passing held-out case carried a baseline root gap whose
`(scope, capability)` exactly matches the selected package. This causal check
is evaluated inside the sealed comparison and disclosed only as a boolean.

For still-nonpassing cases, only exact selected member identities may be
removed from the baseline gap set; every nonselected baseline identity must
remain in the final set. New final gaps are allowed. All inherited regression,
unsafe, replay, simulation, promotion, KiCad, and round-trip gates remain
mandatory.

Any held-out final reveal consumes V5. Success publishes encrypted evidence and
permitted aggregates; failure publishes only a non-revealing permanent audit.
No retry, tuning, corpus mutation, policy change, budget increase, or gate
relaxation can change that result.

GitHub Actions are not manually started or inspected. Local evidence remains
the authoritative development gate.
