# V22 implementation review

Prism review `1525b34320db77f4ed46c3e718a021d5` reviewed the complete staged
implementation, independent public fixtures and follow-up documentation through
the authorized configured `gemini-3-flash-preview` provider at parent `8e5f0bc5`.

## Findings

- `70d85132ef39c29d` (medium, maintainability): not actionable as reported.
  The cited lines 54–74 contain result/state fields, a one-line public wrapper,
  and a function declaration, not the alleged nested conditional block. The
  admission path already uses guard clauses. No correctness or failing path
  was identified by this finding.
- `16fa642180778fb5` (low, repeated sort calculation): addressed. Store the
  derived passing count and penalty once per beam state and compare those
  immutable ranking fields. This does not change ranking or search limits;
  deterministic replay and compound-repair tests still pass.
- `82339aa585e7e08f` (low, missing loop break): its location is outside the
  cited file and duplicate terminals are already invalid, so the asserted
  correctness issue does not apply. The actual replay helper now stops after
  the exact terminal and instance match, consistent with its single-edit
  contract. The exact-edit validator still rejects mismatched or extra edits.

Manual review also tightened selected-result identity/count-bound verification
and numerical report-to-plan metadata validation. Rehashed reports with changed
registry, catalog or device evidence are rejected even when the numerical actual
and claimed plan hash remain unchanged. Historical model/evaluator sources are
not altered.

## Validation status

Focused deterministic, tamper, budget, critical/prior-pass and synthesis tests
pass. The affected race suite and full-project lint pass. Both independent
monitors passed installed-KiCad promotion again after review, with unchanged
two-root project hashes. `V22_VALIDATION.md` records the subsequently completed
bounded, coverage, race and preservation checks. The separately frozen public
successor evaluation is not declared complete by this document.

## Follow-up review

Prism run `8fd3a4e69f928a18325caaa1ffb4dea7` reviewed the remediated staged diff.

- `7830f8586a726d78` (medium, alleged shared-slice mutation): not valid.
  `ApplyValueTrial` begins with `CloneGraph`; `NormalizeGraph` also creates an
  owned graph before sorting or normalizing slices. The V22 evaluator does not
  mutate `graphForAdmission` before those calls. Integration tests additionally
  compare complete predecessor bytes before and after continuation.
- `3caf3c82ceb99ae8` (low, double break): not valid. The operation contract is
  exactly one terminal on exactly one uniquely identified instance. After the
  loop, `checkControlChangeV22` requires the claimed source terminal/node pair
  and the exact reconstructed after graph; absent matches return an error.
  Continuing into another identically named instance would not be a valid fix.

Further manual review added cancellation-aware streaming authentication of the
entire eligible predecessor, before search. A matching selected graph/evaluation
alone cannot authenticate unrelated source metadata or a substituted run hash.
This adds no serialized replay buffer and leaves ineligible runs untouched.

## Final staged implementation review

Prism run `f4abd5731ea73b1e1838e4bc55311239` included predecessor authentication.

- `93bb2d97ee77a644` (medium, float equality): not valid for this check. This
  is exact evidence reconstruction, not comparison of independent numerical
  approximations. `evaluateAssertionCorner` records the report actual divided
  by the same deterministic scale; the certificate repeats that exact operation
  using the same authenticated operands. Nonfinite values are already refused.
  An epsilon here would weaken tamper detection. Electrical acceptance itself
  uses the requirement's declared bounds, not this integrity comparison.
- `b7f25409b653dff5` (low, incremental graph-analysis suggestion): no demonstrated
  defect or redundant identical-input computation. Each candidate changes a
  control binding and must revalidate its complete graph and feedback cycles.
  Work is explicitly bounded. Incremental validation/caching is a separate
  profiling-driven optimization, as required by the goal; it is not grounds to
  skip current fail-closed checks or expand this correction's scope.

All reported correctness findings are either remediated or dispositioned against
source and regression evidence. No unresolved valid implementation finding is
being accepted as a release or capability exception.
