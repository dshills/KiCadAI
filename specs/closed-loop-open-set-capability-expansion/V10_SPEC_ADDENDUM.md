# Closed-Loop Open-Set Capability Expansion V10 Addendum

Status: corrective contract candidate; no V10 author packet, corpus, key,
evaluation, outcome, frontier, selection, or implementation exists.

## 1. Predecessor and scope

V10 succeeds the permanently retired V9 experiment. V9 stopped during
aggregate corpus validation because its frozen assignment lacked high-safety
coverage for one circuit role in each partition. No V9 corpus was published,
no V9 source key was created, no V9 requirement was synthesized, and no V9
baseline began. V10 may reuse the digest-only V1–V8 historical commitment but
must not reuse V9 authored requirement bytes.

Except where this addendum explicitly strengthens preparation, V10 preserves
V9's behavior-only corpus contract, four-outcome baseline, typed gap/path
identity, complete effect-exposure selection, sibling and non-exposed-case
preservation, two-round budgets, installed-KiCad promotion, deterministic
replay, fail-closed behavior, and three-key blind boundary.

## 2. Assignment feasibility before authoring

Before an author packet is checksum-frozen, a production preflight must accept
the complete assignment metadata. It uses no authored requirement, synthesis,
simulation, feasibility, outcome, ranking, or implementation information.

The preflight proves at minimum:

- exact author and partition totals;
- unique identities and unique partition/domain/circuit-role triples;
- exact domain, circuit-role, and safety counts in each partition;
- per-author static, dynamic, multi-output, and off-nominal allocations;
- mathematical consistency of every configured quota;
- at least one assigned high-safety case for every reporting domain in each
  partition; and
- at least one assigned high-safety case for every circuit role in each
  partition.

The packet generator must call this production preflight on its candidate
assignments before writing any packet byte. The frozen packet contract test
must independently re-run the same preflight on the committed assignments.
Failure retires the packet candidate before author authorization or dispatch.

## 3. Freshness and correction boundary

V10 uses six fresh isolated authors and 48 fresh requirements, 24 discovery
and 24 held-out. Authors may see only their V10 packet. V9 quarantines, V9
requirements, correction history, validation errors, and retirement details
are prohibited author inputs.

Outcome-neutral corrections remain bounded to public electrical adjudication,
schema/reference repair, assignment-bound safety metadata, or validator-
reported aggregate canonical-analysis coverage. No correction may target
synthesis feasibility or expected outcome.

## 4. Evaluation and success

V10 starts evaluation only after authenticated publication and a separately
frozen evaluator, evidence validator, public baseline publisher, exposure
engine, and blind baseline publisher. Generation zero evaluates all 24 public
cases exactly twice and promotes every pass twice from distinct clean roots.

The complete active frontier is clustered into topology, component, model,
simulation, physical-design, and verification gaps. Selection ranks generic
capability closures by unlocked cases, domain and role diversity, safety
weight, exposed noncovered cases, sibling burden, atom count, and member count.
No fixture-specific template, coordinates, family allowlist, or manual rank
override is permitted.

Success requires measurable public and held-out pass uplift attributable to
the selected generic capability chain, deterministic replay, full installed-
KiCad promotion, no baseline-pass regression, no unsafe-to-pass transition,
exact sibling/non-exposed preservation, and all existing local regressions.

## 5. Terminal behavior

Any frozen-boundary inconsistency, incomplete corpus, key/publication failure,
environment drift, nondeterminism, exposure omission, sibling drift,
non-exposed drift, regression, or budget violation publishes a no-replace
terminal retirement. A retired version is never repaired or retried; the next
attempt receives a new version and a new freeze.
