# V7 Phase 0 Prism Review Dispositions

Date: 2026-08-12

Scope: V7 adaptive selector, round evaluator, executable contract bindings,
historical-manifest transition, and regenerated catalog-provenance evidence.

Provider: configured external Gemini provider through Prism staged review.

## Remediated findings

Prism reported no high-severity findings. All applicable medium and low
findings were remediated, including collision-checked compact closure keys,
error propagation instead of panic, canonical sorted prior-atom validation,
linear evidence-subset comparison, insertion-ordered closure expansion without
repeated full-set materialization, and linear duplicate-safe merging of prior
and selected atom keys. Round evaluation also independently recomputes the
complete active cohort covered by the selected members, preventing callers
from omitting covered cases to manipulate aggregate evidence. Candidate
semantic comparison is allocation-free in the bounded 262,143-candidate sort.

## Rejected finding: malformed test gap

The reported malformed `Causal[REDACTED]` test literal has disposition
`rejected_with_reproducible_evidence`. The staged source contains the valid Go
field assignment `CausalToken: capability + "_" + code`; the redacted token
appeared only in Prism's rendered review context. Both the focused package test
and the complete repository test suite compile and pass with the staged source.

## Rejected finding: allow frontiers on passing cases

The suggestion to allow a passing case to retain a frontier has disposition
`rejected_contract_conflict`. `V7_SPEC_ADDENDUM.md` section 6 requires every
still-nonpassing case to expose a nonempty next frontier; a frontier is the
causal explanation of why a case is not passing. Accepting one on a passing
case would make the outcome and causal evidence contradictory. Selection uses
`ErrInvalidInput` for this malformed input, while round evaluation uses
`ErrRoundGate` for the same contradiction at the admission boundary.

## Rejected finding: add a closure bound

The suggestion to add a bound to the exponential subset-union closure has
disposition `rejected_already_implemented`. The frozen cohort has exactly 18
discovery cases, the policy limits candidate bundles to 262,143 (`2^18 - 1`),
and `buildClosure` returns `ErrCandidateOverflow` as soon as the insertion-
ordered compact set exceeds that limit. Focused tests cover both the complete
18-case bound and forced overflow with a lower policy limit.

## Rejected finding: nondeterministic atom-key ordering

The claim that `sortedSetKeys(selectedAtoms)` may preserve randomized Go map
iteration has disposition `rejected_with_direct_source_evidence`.
`sortedSetKeys` delegates to `sortedMapKeys`, which collects every key and then
calls `slices.Sort` before returning. The resulting prior-atom state is
therefore canonical and independent of map iteration order.

## Rejected finding: remove strict total pass uplift

The suggestion to remove `passAfter > passBefore` when a new active-cohort case
passes has disposition `rejected_with_reproducible_evidence`. The independent
condition is required by:

- `V7_SPEC_ADDENDUM.md` section 4: stop only after strict total discovery pass
  uplift and at least one newly passing active-cohort case;
- `V7_BASELINE_PROTOCOL.md` Final public admission: strict total discovery
  pass-count improvement and an active causal-cohort pass are separate proofs;
  and
- `V7_SELECTION_POLICY.json` `public_admission.require_total_pass_uplift=true`
  plus `minimum_new_active_cohort_passes=1`.

Reproducible counterexample: if future regression-accounting logic is changed
or bypassed while a cohort case becomes pass, checking only the cohort count
could admit a round whose total pass count is flat or lower. Keeping both
comparisons at the admission boundary makes the frozen dual-gate contract
explicit and fail closed even if an upstream invariant regresses. The check is
therefore intentional defense in depth, not redundant protocol behavior.

## Residual severity

There are no undisposed high- or medium-severity findings.
