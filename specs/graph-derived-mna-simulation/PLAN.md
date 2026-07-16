# Implementation Plan

Status: complete. Every phase below has current unit, repository-regression,
recorded-replay, and optional KiCad-backed evidence.

## Phase 1 — Contracts and Trust Boundary

- Extend trusted simulation intent with bounded analyses, source excitations,
  structured node assertions, resolved topology, and deterministic results.
- Register graph MNA and trusted primitive definitions without exposing
  provider-controlled equations, matrices, stamps, or topology labels.
- Add strict validation and canonical ordering for every new field.

## Phase 2 — Catalog and Graph Compilation

- Add primitive evidence to passive, source-port, and verified op-amp catalog
  records and validate all required model parameters.
- Compile terminals and nets from resolved circuit connectivity, choose ground
  generically from resolved electrical roles, and hash the canonical topology.
- Fail closed on missing claims, values, terminals, source evidence, or
  incompatible/ambiguous devices.

## Phase 3 — Deterministic MNA Execution

- Implement resistor, capacitor, voltage/current-source, and finite-gain
  single-pole op-amp stamps.
- Implement bounded complex Gaussian elimination with deterministic scaled
  pivoting, singular diagnostics, residual validation, and operating-limit
  checks.
- Execute DC operating points and deterministic logarithmic AC sweeps, evaluate
  structured assertions, and emit a provenance-complete report.

## Phase 4 — Held-Out Proof and Fail-Closed Tests

- Add a new automatic-hierarchy active analog fixture using no block family or
  recognized topology.
- Add unit tests for arbitrary graph compilation, both source kinds, DC/AC
  results, canonical replay, schema safety, singular systems, incompatible
  models, invalid limits, and assertion failures.
- Extend promotion tests to require multiple analyses, child sheets, strict
  round trips, and byte-identical simulation replay.

## Phase 5 — Completion Audit and Delivery

- Run focused tests, full tests, lint, diff checks, and required real-KiCad
  fixture and replay gates.
- Review staged changes with Prism, address all findings, commit, push, and
  verify GitHub Actions.
- Mark the specification implemented only after every requirement has direct
  current-state evidence.
