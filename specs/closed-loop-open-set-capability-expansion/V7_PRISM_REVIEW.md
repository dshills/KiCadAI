# V7 Freeze-Candidate Prism Review Dispositions

Date: 2026-08-11

Scope: the V7 specification addendum, plan, corpus rules, baseline protocol,
and selection policy staged for the freeze-candidate commit.

Provider: configured external Gemini provider through Prism staged review.

## Disposition summary

All reported high-severity findings were remediated. Material remediations
include single-threaded outcome-affecting subprocesses, a separate audited
custodian security principal, exact identity encoding, bounded correction and
replacement attempts, machine-readable corpus quotas, a sole authoritative
starting-commit field, mechanical prerequisite-consumer evidence, an explicit
gap-stage order, and an exact blind-final disclosure list.

All medium-severity findings were remediated except suggestions to allow an
unrelated beneficial side effect to remove a nonselected gap and to relax the
generic-reuse floor for one remaining case. Both findings have disposition
`rejected_with_reproducible_evidence` below.

## Rejected finding: unrelated beneficial side effects

The suggestion conflicts with the causal-learning purpose of V7 and with:

- `V7_SPEC_ADDENDUM.md` section 4, which requires admitted lineage for a
  disappearing nonselected gap;
- `V7_BASELINE_PROTOCOL.md` Single round evaluation steps 5 and 6, which bind
  selected-member removal and successor identity; and
- `V7_SELECTION_POLICY.json` `gap_lineage_successor`, which rejects an
  unrelated effect as ambiguous selected-bundle causality.

Reproducible counterexample:

1. A case has complete frontier `{selected_A, nonselected_B}`.
2. The sealed selected capability removes `selected_A`.
3. An unrelated library or tool change removes necessary blocker
   `nonselected_B`, so the case passes.
4. Replaying only the selected capability while removing the unrelated change
   leaves `nonselected_B` and the case does not pass.

Allowing step 3 would attribute the pass to `selected_A` even though the
selected implementation alone cannot reproduce it. A statistical-independence
test cannot repair that causal mismatch in a deterministic 18-case frozen
sample, and would introduce a new unfrozen policy and significance threshold.
Retirement is therefore the required fail-closed result, not a false negative:
it prevents an unselected intervention from contaminating learned capability
impact.

## Rejected finding: relaxing reuse for one remaining case

The suggestion to reduce `minimum_advanced_active_cases` from two to one when
only one active case remains has disposition
`rejected_with_reproducible_evidence`. It conflicts with:

- `V7_SPEC_ADDENDUM.md` section 5, which requires every atom and bundle to have
  support across at least two active cases and two reporting domains;
- `V7_BASELINE_PROTOCOL.md` Single round evaluation step 8; and
- `V7_SELECTION_POLICY.json` `eligibility.final_round_relaxation=false` and
  its frozen `generic_reuse_evidence` rationale.

Reproducible counterexample: with one active case, both a genuinely reusable
capability and a case-shaped capability have the same observed support count of
one and reporting-domain count of one. The frozen corpus supplies no second
observation that can distinguish them. Relaxing the gate would therefore admit
the exact evidence pattern that the no-fixture and generic-reuse requirements
exclude. V7 optimizes measurable generic coverage improvement, not 100% corpus
completion; it intentionally retires when only an uncorroborated single-case
intervention remains.

## Residual severity

After the recorded dispositions, there are no undisposed high- or
medium-severity findings. Low-severity findings were remediated where they
affected determinism, canonical representation, or cryptographic parameters.
