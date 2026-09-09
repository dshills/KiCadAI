# Post-topology electrical blockers

Status: diagnostic implementation in progress; no successor evaluation frozen or run.
Base: merged V21 maintenance evaluation, `abe617d182369352df8fe0e16f9071a539881b21`.

## Objective and boundary

Explain the actual electrical failures of the four publicly advanced V21 cases,
select the highest-impact reusable correction from that evidence, and measure
additional complete simulation-and-KiCad passes in a separately frozen successor.
V18, V20, both V21 reports and all historical seals remain unchanged. V21 and the
successor remain experimental; the v1 support envelope is not expanded implicitly.
No held-out keys/content, circuit templates, component allowlists, fixture
coordinates, outcome-driven budget changes, or manual Actions dispatch are allowed.

## 1. Diagnostic instrumentation

Add an additive, read-only projection of existing synthesis evidence. Do not
modify the sealed V21 execution or its selection. For a retained evaluation:

- Authenticate its existing hash before projection.
- Preserve requirement, inventory, graph, value-trial, evaluation, plan, report,
  and model-evidence identities.
- Identify the first failed attempt in the evaluator's recorded order, with its
  assertion, analysis, operating case, corner, diagnostic, actual and bounds.
- Distinguish admission, pre-simulation preparation, solver, assertion, resource,
  cancellation, and insufficient-evidence states. Never infer successful solver
  execution from a generic nonpassing label, a model name, or a plan hash alone.
- Include compact counts of all attempt outcomes and diagnostic categories; retain
  the selected post-topology graph and structural-certificate binding so diagnosis
  can be independently reproduced. Do not duplicate waveform arrays.
- Keep wall-clock timing outside deterministic trace hashes. Measure synthesis
  and projection duration and actual serialized trace bytes separately. Report
  historical multi-GB spool sizes only as historical spot observations.
- Add deterministic replay, tamper, ordering, admission, solver, assertion,
  missing-evidence, cancellation, and bounded-output tests using independent data.

## 2. First deliverable: root-cause report

Authenticate the committed public corpus and maintenance assessment. The selected
diagnostic population is its four qualifying public cases (004, 017, 018, 021),
not a new selection by observed success. This list belongs only to evaluation
configuration, never production code.

Run each selected case once through the unchanged V21 synthesis boundary in a
fresh diagnostic root, with the exact legacy/V18 catalogs and models and unchanged
default policy. These are explicitly diagnostic executions, not repetitions or
replacement of the frozen V21 evaluation. Commit the diagnostic source and run
configuration before execution. Match the resulting synthesis identity to retained
historical evidence where that identity is available; disclose any mismatch.

Publish ROOT_CAUSE.md with source-bound traces, candidate-specific first failures,
cross-case cause counts, limitations, and timing/size observations. Rank shared
causes by distinct affected cases, then domains, with a stable textual tie-break.
Different diagnostic codes are not automatically different root causes, and a
shared generic code is not automatically one cause. Explicitly distinguish facts,
source-supported explanations, and hypotheses. No capability correction is selected
before this report exists.

## 3. Generic correction

Freeze the chosen root cause and scope before implementation. Add one reusable
correction with independent public positive/negative fixtures across applicable
domains. Preserve early fail-closed admission, model/solver provenance, numerical
and safety checks, deterministic behavior, and historical implementation paths.
Keep performance changes separate unless measurements establish a bottleneck.

## 4. Separately frozen successor evaluation

Before running the successor, commit its exact code/input/environment seals,
selection and resource limits, 24-case/two-replay public protocol, output paths,
and preservation/advancement rules. Reuse immutable public requirements and compare
against the corrected V21 maintenance report; do not touch held-out data.
An increase in complete simulation-and-installed-KiCad passes is the success
measure. Structural advancement or renamed diagnostics alone is insufficient.
Publish a negative result honestly if the bounded correction does not achieve it;
do not retune/repeat the frozen evaluation in response to outcomes.

## 5. Verification and handoff

Run local unit/integration, race, deterministic, seal, external-review, coverage,
educational, and installed-KiCad preservation gates. Review staged changes through
authorized Prism, remediate valid findings, then commit/push/open a PR. Do not merge
or begin another capability phase as part of this milestone.
