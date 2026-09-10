# V23 admitted execution and electrical certificates

This checkpoint integrates the solver layer from `83259ecb` with electrical
evaluation. Repair-search integration, the final successor source/protocol
freeze, corpus execution, installed-KiCad preservation, and the follow-up PR
remain pending. No corpus outcome, release support, or physical readiness claim
is made here. V22 and all earlier production/evidence files remain unchanged.

## Explicit solver selection and prerequisites

`EvaluateElectricalCandidateV23` is the only new public numerical entry point.
Calling it explicitly selects the sealed V23 execution policy; no default API,
primitive model, source registry, or historical solver registry is changed.

The V22 model/backend admission decisions remain exact **prerequisites**: each
assertion/case/corner must have its original trusted model and enabled compatible
backend before numerical work. They do not claim which engine executed V23.
An unavailable historical backend still refuses, even when the V23 API is called.
The enclosing V23 policy explicitly selects bounded linear-sweep recovery; other
analyses execute the historical delegate. This is not an implicit substitution
or an extension of the environment's enabled-solver list.

Version-isolated adapters preserve the historical ordering, early refusals,
metric translation, critical-assertion handling, exact admission keys, and atomic
worst-case budget accounting. Only numerical dispatch and its execution sidecar
are changed. Full source copies are necessary to leave frozen historical entry
points byte-for-byte unchanged; no new topology, model, or analysis is introduced.

## Evidence and verification

`ElectricalEvaluationV23` contains the historical-shaped numerical evaluation,
the exact V23 solver-policy ID/digest, and ordered `SolverAttemptV23` records.
Every numerical report has exactly one record identifying its assertion,
analysis, operating case, and corner. The record carries the solver execution
binding and unmodified numerical diagnostics, including on a numerical refusal.
Pre-solver refusals have no execution record. Plans and waveforms are not copied
into the sidecar.

The envelope hashes a projection of its schema, policy ID/digest, already-bound
evaluation hash, and ordered execution records. Verification separately checks
the full evaluation hash. The projection avoids serializing the waveform payload
again merely to name its enclosing policy; it does not skip content verification.

Verification reconstructs each numerical plan from the requirement, graph,
catalog, models, and corner. It checks report/plan identity, exact execution
policy/work bounds, diagnostics, and fresh prerequisite admission. Passing
evaluations additionally require every requested corner, every electrical pass,
and exact bounded simulation/corner accounting. Missing, reordered, duplicate,
unassociated, cross-plan, or rehashed policy evidence is refused.
`VerifyPassingCornersV23` also checks every internal worst-case corner against
the unchanged nominal, deterministic, and directed corner schedule. Assertions,
assignments, bounds, pass flags, and nominal-report correspondence must match;
rehashing a report after deleting an internal corner cannot certify it.

These are provenance-binding verifiers, not numerical reruns or signatures from
an external authority. The committed evaluator/build freeze and deterministic
replay must still establish that the recorded numerical execution occurred.

`ElectricalCertificateV23` wraps the unchanged V22 structural/control-path,
model-admission, numerical-bound, and complete-corner checks as a prerequisite
proof, and binds the verified V23 evaluation identity. Any numerical diagnostic
prevents certification. The nested prerequisite proof does not assert execution
by the historical solver. A certificate still makes no KiCad/physical claim.

## Independent regressions

- The existing hand-authored 12 V monitor remains byte-identical numerically for
  its non-sweep analyses and receives explicit historical-delegate records.
- An independent extension traverses a 0–12 V input range under the existing
  12 V model, allowing its 0.3 V rail margins. Its full-span slope is
  `(11.7 - 0.3) / 12 = 0.95`. The original engine refuses at the rail exit; V23
  produces the numerical pass and an authenticated electrical certificate.
  This fixture uses no corpus request or graph.
- A requirement for slope 0.98–0.99 remains a genuine electrical refusal; the
  correction cannot certify it. Unavailable backend, insufficient atomic corner
  budget, and cancellation perform no numerical work.
- Tamper tests cover policy and solver identities, solver work bounds, omitted or
  duplicate execution/attempt records, ordering, corner identity, hidden numerical
  diagnostics, plan/model provenance, missing reports, omitted complete corners,
  changed assertion bounds, false success, and changed accounting.
- One-sided-bound regressions preserve maximum-only and minimum-only passes and
  the correct direction for each real refusal. Internal-corner tests cover the
  exhaustive and directed schedules and rehashed report/corner omissions.

All-corner simulation certificates here are independent test results. Additional
complete simulation-and-installed-KiCad **corpus** passes are still required by
the active goal; these tests do not satisfy that final outcome.
