# Implementation Plan

Status: complete (2026-08-05). The completion evidence is recorded in
[AUDIT.md](AUDIT.md).

## Phase 0 — Baseline And Contract

- Record current package/corpus timing and identify repeated expensive work.
- Freeze the scheduler contract and preservation commands.

## Phase 1 — Evidence Model And Budgets

- Add canonical scheduler stages, per-attempt execution evidence, cache
  accounting, explicit per-candidate/per-analysis/overall budgets, and stable
  budget-exhaustion diagnostics.

## Phase 2 — Staged Trusted Evaluation

- Order complete resolved plans by the most expensive analysis they contain.
- Stop rejected attempts at the first failed stage while retaining actionable
  partial evidence.
- Require a complete final attempt for every passing candidate.

## Phase 3 — Content-Addressed Reuse

- Reuse byte-identical trusted plan reports through the bounded run-local
  SHA-256 cache.
- Record deterministic hits and misses without admitting persisted/provider
  evidence into the trusted cache.

## Phase 4 — Conservative Finalist Reduction

- Compute Pareto dominance only from complete evaluation and static ranking
  evidence.
- Retain an auditable decision for every eliminated finalist.

## Phase 5 — Physical Workflow Binding

- Bind exhaustive electrical selection to the existing ordered physical and
  installed-KiCad promotion stages.
- Validate that partial or budget-exhausted evidence cannot cross the boundary.

## Phase 6 — Performance And Preservation

- Optimize measured hot paths without weakening corner coverage.
- Pass the full local suite inside 20 minutes.
- Run all named preservation and installed-KiCad lanes twice where their
  contracts require deterministic replay.

## Phase 7 — Review And Closeout

- Update status/roadmap documentation and record exact commands and timings.
- Run Prism on the staged diff, remediate actionable findings, commit, and
  push.
