# Simulation-Guided Open-Topology Synthesis Plan

## Objective

Add a deterministic, bounded, primitive-component topology search and
simulation-refinement lane, prove it against a frozen behavior-only corpus,
and preserve every existing supported regression.

Status: implemented and locally verified on 2026-07-30. The
[completion audit](AUDIT.md) and [promotion matrix](PROMOTION_MATRIX.json)
record phase-by-phase evidence, the 6/8 measured envelope, stable bounded
exhaustions, installed-KiCad receipts, and preserved regression lanes.

## Working Rules

- Freeze the held-out corpus and untouched baseline before production search
  code.
- Reuse catalog and model evidence; do not copy component facts into search.
- Use only terminal-level generic graph operators in production.
- Keep budgets count-based and output ordering canonical.
- Fail closed on missing primitive, model, rating, package, or physical proof.
- Run focused tests after each change and the optional installed-KiCad held-out
  lane after every fix that affects generation, lowering, routing, or writing.
- Run all verification locally. Do not wait for GitHub Actions.
- Review the final staged diff with Prism before committing.

## Phase 0 — Capability Audit And Frozen Benchmark

- Record the exact current boundary between provider expansion, primitive
  simulation, value repair, and physical promotion.
- Add eight topology-neutral held-out requirements and a SHA-256-pinned
  manifest.
- Add freeze tests that reject identity, component, topology, provider, model,
  value, net, geometry, route, and repair leakage.
- Capture the untouched baseline with stage-by-stage failure codes and hashes.

Exit: the corpus is immutable, the baseline is reproducible, and no production
topology synthesis exists.

## Phase 1 — Requirement And Evidence IR

- Add strict `kicadai.open-topology-requirement.v1` decode, normalization,
  validation, and canonical hashing.
- Add semantic excitation-to-observation assertions without topology fields.
- Add search policy, consumption, graph, candidate, diagnosis, repair, and
  final-report types.
- Add stable failure codes and deterministic JSON marshaling tests.

Exit: valid requirements normalize identically under reordered input, invalid
or implementation-bearing requirements fail closed, and reports are replay
stable.

## Phase 2 — Primitive Inventory

- Derive primitive terminal contracts from the accepted catalog and registered
  model claims.
- Restrict the initial lane to the primitive families in the specification.
- Bind values, tolerances, ratings, package evidence, and allowed analyses.
- Record exclusions for functional compact models.
- Add deterministic inventory and missing-evidence tests.

Exit: the same catalog and model registry always produce the same primitive
inventory hash and candidate order.

## Phase 3 — Canonical Graph Kernel

- Implement immutable candidate graphs, typed external/internal nodes, and
  terminal attachments.
- Implement graph validation, source/observation reachability, active-device
  supply/reference checks, and static rating rejection.
- Implement isomorphism-resistant canonical topology fingerprints.
- Add mutation, permutation, symmetry, and adversarial invalid-graph tests.

Exit: semantically identical graphs share one fingerprint and invalid graphs
cannot reach simulation.

## Phase 4 — Bounded Topology Search

- Implement generic add/connect/join/feedback/substitute/remove operations.
- Add deterministic best-first frontier ordering, dominance pruning, and
  complete budget accounting.
- Generate multiple materially distinct candidates where available.
- Add cancellation and every budget-exhaustion path.

Exit: search produces deterministic complete primitive graphs without
consulting named capability providers or fixture-specific branches.

## Phase 5 — Value Domains And Analytic Seeding

- Derive bounded preferred-value and catalog-variant domains.
- Add topology-neutral analytic scale estimates for resistance, capacitance,
  bias, threshold, gain, and timing.
- Keep estimates advisory; only trusted simulation can pass an assertion.
- Add tolerance, rating, domain-exhaustion, and ordering tests.

Exit: each value trial is catalog-valid, reproducible, and provenance-complete.

## Phase 6 — Simulation Evaluation And Diagnosis

- Lower candidate graphs to the existing circuit graph and reviewed primitive
  simulation registry.
- Bind semantic supplies, excitations, loads, events, and observations.
- Evaluate all required cases and generate stable normalized diagnoses.
- Reject functional compact models and untrusted or inapplicable claims.

Exit: complete graphs receive trusted assertion evidence or a stable
fail-closed model/evaluation result.

## Phase 7 — Graph-Changing Repair

- Map diagnosis classes to bounded generic graph/value repair operators.
- Record preconditions, graph deltas, before/after hashes, and work.
- Rerun full resolution and all analyses after every repair.
- Add successful repair, non-improvement, repeated-state, unsupported,
  cancellation, and exhausted-budget tests.

Exit: at least one held-out result passes only after a generic topology change,
with byte-identical replay.

## Phase 8 — Architecture And Physical Integration

- Adapt passing primitive graphs to the normal architecture selection,
  lowering, design workflow, and evidence contracts.
- Derive schematic grouping and placement hints from graph roles, never fixture
  identity.
- Route using existing generic policy and preserve all writer/round-trip gates.
- Add CLI/API entry points and retained artifact references.

Exit: a primitive-generated candidate can traverse the same production path as
provider-backed candidates without weakening either path.

## Phase 9 — Held-Out Promotion And Regression

- Run all eight cases twice locally.
- Require at least six complete passes and stable fail-closed outcomes for the
  remainder.
- Run installed-KiCad ERC/strict DRC, connectivity, route completion, writer,
  normalized round trip, and replay gates for every passing case.
- Run existing USB-C, ESP32/MCU, amplifier, component-onboarding, architecture,
  simulation, routing, writer, round-trip, and promotion suites.
- Compare clean-root artifacts and produce a promotion matrix.

Exit: acceptance thresholds and all preserved regressions pass locally.

## Phase 10 — Audit, Documentation, Review, And Delivery

- Update roadmap, project status, AI readiness, AI generation, CLI reference,
  and reproduction instructions.
- Write a clause-by-clause completion audit and final benchmark report.
- Scan production code for held-out leakage.
- Review the complete staged diff with Prism and resolve actionable findings.
- Commit and push the reviewed change.

Exit: local evidence, documentation, review, commit, and push are complete.
