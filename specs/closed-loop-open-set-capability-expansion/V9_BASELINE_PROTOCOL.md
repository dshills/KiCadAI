# V9 Baseline, Exposure, and Adaptive Evaluation Protocol

## 1. Preconditions

Evaluation starts only from a clean committed tree containing the frozen V9
contract/verifier, authenticated 24/24 corpus publication, absent V9 output
paths, frozen evaluator and toolchain manifests, and the committed V8 retirement
with reason `causal_lineage_invalid` and `held_out_opened=false`.

The evaluator freezes inventory, catalog, model registry, synthesis policy,
installed KiCad and libraries, seeds, concurrency one for outcome-affecting
work, locale, timezone, floating-point mode, and resource ceilings. Any drift,
extra file, mismatched commitment, or preexisting terminal path fails before
synthesis. V8 held-out material is never opened.

## 2. Generation-zero discovery baseline

All 24 discovery cases run in manifest order twice in independent clean roots.
Complete exports, normalized diagnostics, candidate inventory, observation,
frontier, and hashes must match byte for byte. Every pass receives two further
clean-root installed-KiCad promotions.

Each case is exactly `pass`, `unsupported`, `unsafe`, or `exhausted`. A pass
requires all 14 corpus gates and both promotions. Timeout-as-outcome, unknown,
partial, skipped, or conflicting classification invalidates the baseline.

The atomic public baseline contains per-case evidence, aggregate counts,
immutable obligation commitments, complete root frontiers, environment
commitments, and checksums. No held-out input is opened during this phase.

## 3. Frontier and identity validation

Every eligible nonpass has at least one root gap in one canonical category:
`topology`, `component`, `model`, `simulation`, `physical_design`, or
`verification`. Exact member identity is `(stage, category, scope, capability,
code)`; atom identity omits stage/code. Every path binds one immutable obligation
anchor, begins with exactly one root leaf, has sorted unique evidence, and has a
canonical hash. Duplicate, contradictory, missing, or unanchored paths fail.

Gap ordering is obligation anchor, path hash, member key, then evidence. Case,
domain, role, safety, and normalized diagnostics never participate in member or
atom identity.

## 4. Complete candidate and exposure construction

Candidate enumeration is complete and fail-closed on overflow. It uses eligible
discovery cases only; unsafe cases remain regression sentinels. A candidate
covers a case only when its direct plus statically proven closure members cover
the complete active frontier.

For every eligible candidate the evaluator also scans every active path in all
24 discovery cases. A case enters `effect_exposure_case_ids` when any current
member or atom is direct or closure. The selection commits, in canonical order:

- fully covered case IDs;
- exposure case IDs;
- selected/closure path hashes per exposed case;
- all nonselected sibling path hashes per exposed case; and
- all non-exposed case hashes.

The exposure set must contain every fully covered case. Recomputing it from the
frontier must reproduce exact bytes. Omissions, additions, ambiguous identity,
or a selected path classified as a sibling invalidate selection.

## 5. Ranking and implementation seal

Candidates must fully cover at least two active cases, two domains, and two
roles within per-round budgets. Ranking follows the exact tuple in the V9
specification: unlock and diversity descending, then exposed noncovered cases,
sibling burden, atoms, and members ascending. The complete semantic co-rank-one
set is public; canonical key order breaks only a complete semantic tie.

Before implementation, the effect plan freezes direct/closure identities,
exposure and sibling commitments, required evidence, production/verification
paths, prerequisite consumers, reverse call graph, bounded runtime traces, and
before hashes. Every outcome-affecting consumer must map to the selected closure.
No V9 corpus synthesis or outcome inspection may construct or test the plan.

The implementation seal adds after hashes, exact changed paths, generic tests,
local regression results, installed-KiCad evidence, and Prism disposition. Any
unmapped change, nonselected sibling mutation capability, fixture dispatch, or
environment drift rejects the seal.

## 6. Exactly-once public round

Each committed round is authorized and consumed exactly once:

1. verify clean tree, absent output, runner, selection, plan, seal, and
   environment commitments;
2. evaluate all 24 discovery cases twice;
3. promote every pass twice;
4. reconstruct complete current frontiers from immutable obligations;
5. require exact case metadata and case set;
6. for each selected/closure path, require disappearance only on satisfied
   obligation or one to four same-anchor successors with exact prior prefix,
   one appended different same/higher-stage member, and nonweaker evidence;
7. require every committed nonselected sibling path byte-identical;
8. require every non-exposed case byte-identical, including diagnostics,
   frontier, satisfied obligations, and outcome;
9. require at least two advanced active cases across two domains and two roles;
10. reject pass regression and unsafe-to-pass transition; and
11. enforce round and cumulative budgets.

Stage order is topology, component, model, simulation, physical_design,
verification. Unknown stage, anchor change, path rewrite/truncation, retained
selected leaf, zero successor for a failing selected obligation, successor
overflow, sibling drift, exposure omission, or non-exposed drift retires V9.

Strict total pass uplift plus at least one new active-cohort pass is public
admission. Otherwise an admissible advance may continue only after the next
ranking, exposure set, plan, and selection are separately frozen. At most two
rounds, six atoms, and 18 members are permitted.

## 7. Retirement evidence

Retirement is atomically published before the test fails. It binds generation,
input frontier/selection/plan/seal/runner/environment hashes, a frozen reason,
`held_out_opened=false`, and aggregate drift counts. Drift classes are exactly:
`selected_path`, `nonselected_sibling`, `non_exposed_case`, `regression`,
`environment`, and `budget`.

Because discovery is public, an optional public detail file may list case IDs,
obligation anchors, prior/current path hashes, and drift class. It may not
contain requirement text, expected implementation, coordinates, or any
held-out-derived value. Its schema and inclusion choice are frozen before the
round. A retirement artifact cannot be overwritten or retried.

## 8. Blind held-out baseline and final

After public selection and before implementation, an isolated custodian uses
the V9 source key and a new distinct 0600 baseline key to evaluate exactly 24
held-out cases under generation zero. It publishes encrypted per-case evidence
and only non-revealing aggregate commitments. The implementation context cannot
access keys, plaintext, decrypted records, outcomes, frontiers, or timing.

Only public admission permits one isolated final using a third distinct 0600
key. Success requires strict held-out pass uplift, at least one newly passing
case covered by the public selected chain, exact case preservation, no baseline
pass regression, no unsafe-to-pass transition, deterministic replay, and full
physical promotion. Publication contains encrypted records and frozen aggregate
counts only.

## 9. Atomicity and terminal behavior

Every baseline, selection, round, retirement, and final is written to a fresh
sibling temporary directory, fsynced where supported, checksum-verified,
renamed without replacement, and independently verified read-only. Success and
retirement roots are mutually exclusive.

There is no retry after consumption, corpus mutation, selection override,
expected-result edit, manual rank choice, budget increase, gate relaxation,
fixture-specific implementation, held-out disclosure, or manual GitHub Actions
run. Any ambiguity fails closed into the frozen terminal artifact.
