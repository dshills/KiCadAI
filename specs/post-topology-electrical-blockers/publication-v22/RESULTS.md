# V22 electrical-repair results

Status: **evaluation complete; preservation passed; electrical improvement
failed**. The original process and frozen post-run assessment exited successfully
on 2026-09-09. This is a negative capability result, not completion of the goal.

## Current evidence boundary

The evaluator is the original clean freeze at
`d3b6088fbaa4f318b5685d62c4fc83eeba3eb4a2`. It ran the unchanged 24 public
requirements twice each, with four preselected cases entering V22 and twenty
using their exact V21/V20 predecessor path. No held-out data is involved.
The supported v1 surface is unchanged.

All 24 cases completed both replays and passed the independent retained-file
audit. Their full replay hashes, case decisions, and gate records all match the
corrected V21 report. Outcomes are **1 pass, 6 unsupported, 1 unsafe, and 16
exhausted**. There are **zero additional complete passes** and no regressions.
All twenty unselected predecessor paths satisfy the frozen preservation gate.

The preserved passing case completed both installed-KiCad promotions, each with
two clean project roots, all required acceptance gates, and exact project hash
`749e28df8589469077f4242a65c804d02a74f0068a6b883aa39002d8d2cc7af6`.
Its promotion hash remains
`b57b61e0ea85d20b0b8bb5fc0b024fa9c221d19cb4e47ed7396051a11a38227e`.

The [report](../../../internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/report.json)
has canonical identity
`1d811e1f50a95291891f7468b81ee7654d0d0a84576612336de491f9cdc36db9`
and file SHA-256
`ae119e22679d94e8a1e618f6fdc8896b4b6a3480262cb4f5de23cc3834af7455`.
The [frozen assessment](../../../internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/ASSESSMENT.json)
records `preservation_passed: true`, `electrical_improvement_passed: false`, and
unchanged typed frontier counts. Its `terminal_frontier_before/after` maps count
all recorded frontier-path nodes, not distinct cases or unique root causes.

## Selected repair observations

| Public case | Observed V22 repair result | Measured search work | Complete new pass |
| --- | --- | --- | --- |
| 004 | Refused by the existing critical-failure guard | No binding work or candidate simulations | No |
| 017 | Evaluation budget exhausted | 168 considered bindings; 128 candidate simulation invocations; 114 recorded trials | No |
| 018 | Repair frontier exhausted, without exhausting its budget | 30 considered bindings; 6 candidate simulation invocations; 4 recorded trials | No |
| 021 | Refused by the critical-failure guard | No binding work or candidate simulations | No |

The invocation and trial counts are different measures: numerical evaluation
consumption includes retries. They must not be presented as interchangeable.
The field named `passing_assertions` counts passed evaluation attempts, each
bound to an assertion, operating case, and corner; it is not necessarily a count
of distinct requirements.

Both electrical sidecar files match exactly for each completed selected case;
their repair and wrapper identities, requirement bindings, and frozen limits
pass the independent publication audit.

Both replays of case 021 retained the exact predecessor synthesis identity and
returned `critical_failure` without generating repair trials. Its completed
case and both retained sidecars pass the independent authentication checks.

Case 017 recorded 111 failed trials and three unsupported trials. The latter
were rejected as `non_numerical_result`. The best recorded candidate had two
passing evaluation attempts, and the search reached depth two before its
invocation budget was consumed. This does **not** prove that increasing a budget
would solve the case. No budget increase or outcome-driven retry was performed.

Case 018 explicitly tried redirecting the amplifier's `IN_MINUS` terminal from
the signal input to the conditioned output. That direct negative-feedback
candidate remained electrically failed, although its trace records two passing
assertion/corner attempts. The other three evaluated candidates also failed.
The frontier exhausted without hitting the invocation budget; increasing that
budget alone does not address the observed exhaustion of available admissible
rebindings.

These observations establish that control-terminal rebinding alone did not
solve cases 017 or 018. They do not establish the precise residual electrical
cause of every rejected trial. The compact trial records retain evaluation
identities, changes, status, and counts, but not each trial's first failed
assertion identity or actual/bound pair. It would be unsupported to infer which
specific assertion improved, or to choose a new model, value, or graph operation
solely from the count of passing attempts. The original first-failed-gate
diagnoses remain separately available in the immutable root-cause evidence.

## Interpretation and remaining gates

Failed V22 repair leaves the shared synthesis run and capability outcome exactly
unchanged; new repair work is recorded in its separate authenticated sidecar
and elapsed-time measurements. No failure has been renamed into advancement.
The independent single- and dual-monitor development fixtures passed complete
simulation and installed-KiCad checks, but those are not new corpus passes.

The selected repair iteration did **not** meet the goal's improvement criterion.
The completed aggregate, all 48 retained replay records, and the exact publication
inventory are authenticated. The original evaluator was also reproduced
byte-for-byte from a clean clone of the frozen commit. Local quality and
installed-KiCad preservation evidence is recorded in
[LOCAL_VALIDATION.md](../public-evaluation-v22/LOCAL_VALIDATION.md).
Final staged [Prism review](REVIEW.md) has no unresolved valid findings. Human PR
review and integration remain separate gates. Passing these gates cannot turn
the zero selected-case improvement into success.

The next technical decision needs the specific failed electrical assertion and
actual/bound values for rejected repair candidates. The retained trial counts
alone do not justify another operator, larger budgets, relaxed critical guards,
or any v1 admission. No outcome-driven repeat of this frozen evaluation occurred.

Timing and memory observations are documented separately in
`PERFORMANCE_OBSERVATIONS.md`. They are not controlled comparative benchmarks.
