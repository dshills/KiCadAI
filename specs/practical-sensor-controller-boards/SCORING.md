# Frozen adjudication procedure

Status: draft, to be sealed with the evaluator before the baseline.

The automated runner intentionally reports candidates, not complete passes.
The following read-only adjudication is part of the evaluator. It cannot be
changed after seeing corpus results. Review does not authorize editing the
generated proposal, requirement, circuit or project.

## Case audit record

Write a separate `<case-id>.audit.json` outside the worker's sealed case
directory. Bind it to the SHA-256 of `case.json`, `prompt.txt`, `result.json`,
`inventory.json`, and the supervisor's `<case-id>.resources.json`. Record:

- reviewer identity and independence (`self_review` for this task);
- review UTC timestamp and measured elapsed review time;
- one disposition for every numbered case acceptance clause and every common
  positive clause, preserving their original order and text;
- for every disposition: `pass`, `fail`, or `not_run`, a reason, and exact
  evidence file plus SHA-256 and JSON pointer, line, sheet or rendered region;
- for every electrical failure: assertion/observation, condition, actual,
  required bound, unit and provenance where present; otherwise explicit null
  with a reason why the quantity was not measured;
- the eight SPEC gates in their declared order for positives/paraphrases;
- manual component/net/placement/routing repairs (must be zero to pass), fixed
  clarification-answer turns and any unplanned assistance;
- the final case disposition and first failed gate, including a faithfulness
  failure that precedes later automated electrical/physical work.

Do not treat a provider's coverage claim as independent evidence. A material
requirement classified as mere context must still be traced to an enforceable
contract field and/or the independently applied physical acceptance check.
Missing or unverifiable evidence is `not_run`/failure, not implicit success.
No post-hoc topology answer may be supplied by the reviewer.

## Positive pass

All must hold:

1. The worker reports `complete_candidate`, two completed replays, a nonempty
   matching replay identity and no runtime error.
2. Every input and output file in the worker inventory authenticates. All native
   project files, including hierarchical sheets, participate in the identity.
3. Every material requirement and all eight acceptance gates pass the audit.
   The visual rubric is checked on the retained schematic renders.
4. The worker exits zero, is not killed, has no resource-monitor failures and
   stays within the frozen case/campaign/storage/provider limits. Missing RSS
   sampling is reported unavailable; it cannot support a claim of measured
   compliance. A machine candidate without verified budget compliance fails.
5. No manual implementation repair was used.

The audit may reject an automatic candidate. It may not promote an automatic
failure, skipped native check, failed replay or truncated campaign to a pass.

## Refusal and clarification

A refusal passes only when the initial workflow produces `refusal_candidate`,
the retained diagnostic matches the case's predetermined reason, and no
executable accepted requirement or project was emitted. A generic unrelated
error, provider transport failure or arbitrary refusal is not correct behavior.

A clarification passes only when the initial result is
`clarification_candidate`, the questions target the missing facts named in the
case, and no guessed executable requirement was emitted. The fixed answer turn
must use the existing cryptographic follow-up binding, preserve the supplied
facts and introduce no invented electrical assumptions. A remaining legitimate
capability gap after answering is reported separately; it is not a complete
board pass and does not expand the eight-positive denominator. A malformed,
misbound or fabricated follow-up fails the clarification-workflow criterion.

## Comparison and aggregate decision

Report all eight positives, all four refusals, both clarifications and both
paraphrases in original order, including failures and not-run cases.

- `baseline_passes`: complete primary-positive audits on merged main.
- `final_passes`: complete primary-positive audits on the final fresh live run.
- `preserved`: every baseline passing primary positive also passes the final
  fresh run AND the same-input paired control.
- `improved`: primary positives failing baseline but passing both the final
  fresh run AND the same-input paired control. Authenticate that paired input
  against the baseline's retained provider proposal/compiled requirement.
- `reserved_passes`: complete final audits for both public-frozen P07 and P08.
- Report first-attempt and corrected successes independently; corrections do
  not create new cases. Count clarification answers separately from diagnostic
  correction attempts. Paraphrase results never enter the primary denominator.

Milestone success is the conjunction of: `final_passes >= 6`, `improved >= 2`,
`preserved`, both reserved passes, four correct refusals, both correct
clarification workflows, complete budget/evidence compliance, zero manual
repair on counted designs and required regression suites passing.

A missing baseline/final/paired campaign means evaluation is incomplete, not a
zero-valued successful score. A completed negative experiment retains the same
denominator and publishes its blockers without another correction cycle.

## Baseline scope decision

Before production edits, list reproducible failure clusters from non-reserved
cases, the maximum three proposed generic changes, independent regressions,
resource costs and a credible path to two distinct complete uplifts. If the
observed gaps require a materially broader program, publish that finding and
request a new scope. Do not start implementing unrelated gaps merely because
the overall objective remains active.
