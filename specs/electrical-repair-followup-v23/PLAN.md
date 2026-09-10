# V23: residual electrical diagnosis and bounded repair

Authorized follow-up to the negative V22 result. Base: `39aff2fbff00f64c65cc568f4333f9591bd28824`.
V22 remains immutable experimental evidence: 1 pass, 6 unsupported, 1 unsafe,
16 exhausted; no additional complete passes. This is a separate iteration,
not a retry, replacement, or extension of the V22 evaluation.

## Phase 1: frozen residual diagnosis

Before numerical execution, commit the diagnostic implementation and its input
seal. Authenticate V22's published evidence and the retained V21 diagnostic
graphs. Use only the four previously selected public cases. Cases 004 and 021
have no repair trials: retain their critical-failure refusals without numerical
execution. For 017 and 018, replay every recorded repair trial once, in recorded
order (114 and 4 respectively), without rerunning synthesis or proposing edits.

Reconstruct each graph from its recorded parent and single terminal change.
Require an exact graph hash, use the unchanged V22 evaluator and V18 environment,
and reproduce the remaining simulation/corner budget before each trial. Require
the exact recorded evaluation hash, status, passing-attempt count, and aggregate
simulation/corner consumption. A mismatch terminates diagnosis; never substitute
new results for V22. Do not retry successful trials or silently rerun a failed
diagnostic invocation. Any instrumentation recovery must have a separately
committed, outcome-neutral correction and an explicit execution-count record.

Retain a compact first-failed-gate projection for every authenticated trial,
including assertion, analysis, operating case, corner, actual, bounds, and
plan/report/model identities. Keep timing outside deterministic trace identities.
Use serial execution, Go 1.26.8 darwin/arm64 CGO=1, and a 30-minute infrastructure
timeout. These are diagnostic replays, not corpus evaluations or pass evidence.
Publish a source-bound residual report distinguishing observations from hypotheses.

## Phase 2: select and implement one generic correction

Only after the residual report, freeze the highest-impact justified root cause,
operator/solver/model scope, count limits, and independent regression fixtures.
Preserve critical-failure guards, previously passing corners, all structural and
model-admission gates, numerical bounds, provenance, and physical acceptance.
Do not assume that greater search budgets or a higher passing-attempt count solve
an electrical requirement. No performance or concurrency changes without a
separate profiling justification. No circuit templates, component allowlists,
fixture coordinates, held-out-derived shortcuts, or changed corpus requirements.

## Phase 3: freeze and evaluate the successor

Commit the exact implementation, environment, selection, resource limits,
24-public-case/two-replay protocol, output path, preservation rules, and source
seals before evaluation. Preserve all historical evidence, including V22.
No held-out keys/plaintext; no outcome-driven retries or budget tuning. Require
additional complete simulation-and-installed-KiCad passes, unchanged historical
passes and safety outcomes, and deterministic evidence. Publish negative results
honestly; no implicit v1 admission or further capability iteration.

## Phase 4: verify and hand off

Run affected local tests throughout, then the required local quality, coverage,
race, replay, seal, release, educational, writer, routing, simulation, round-trip,
and installed-KiCad preservation gates. Prism-review completed staged phases,
remediate valid findings, commit, push, and open a follow-up PR. Do not merge PR
#9 or this follow-up without merge authorization. Do not manually dispatch Actions.
