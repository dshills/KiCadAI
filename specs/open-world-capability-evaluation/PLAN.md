# Open-World Capability Evaluation And Gap-Driven Expansion Plan

## Objective

Build the deterministic evaluation and learning loop that turns unfamiliar
behavior-only requests into ranked reusable capability work, then close the
highest-value gaps and prove generalization on an untouched held-out corpus.

Status: In progress.

## Phase 1: Freeze Contract And Corpora

- Publish the outcome, clustering, ranking, improvement, and promotion
  contracts.
- Freeze separate discovery and held-out behavior-only corpora across analog,
  power, MCU, sensor, digital, and mixed-signal domains.
- Pin corpus bytes and membership with SHA-256.
- Add neutrality, leakage, duplicate, paraphrase, and reorder tests.

Acceptance: corpus identity is immutable, held-out membership cannot influence
ranking or implementation, and no source contains implementation detail.

## Phase 2: Production Evaluation Model

- Add a reusable package for terminal outcomes, observations, case results,
  aggregate reports, and validation.
- Reject invalid/tool outcomes rather than counting them as capability gaps.
- Canonicalize semantic capabilities, stages, codes, paths, and evidence.
- Add deterministic JSON and hash contracts.

Acceptance: all five required outcomes are represented, malformed evidence
fails closed, and report bytes are stable under input reorder.

## Phase 3: Generic Clustering And Ranking

- Move reusable clustering behavior out of the historical test-only held-out
  evaluator.
- Add reviewed capability-impact registry records and acyclic dependency
  validation.
- Calculate frequency, safety, reuse, and domain scores.
- Emit rank rationale, tie-break evidence, cases, domains, and required
  evidence.
- Add mutation and order-neutrality tests.

Acceptance: cluster identity is fixture-neutral, every score is derived from
trusted inputs, and exact ordering follows the versioned policy.

## Phase 4: Baseline Evaluation

- Compile and evaluate the frozen discovery corpus through normal production
  stages.
- Evaluate the held-out corpus without using it for ranking.
- Write checksum-pinned baseline reports.
- Confirm coverage of ready, clarification, unsupported, ambiguity, and
  budget-exhaustion outcomes.
- Select implementation work strictly from discovery ranks plus the four
  required family categories.

Acceptance: baseline bytes are frozen before capability implementation and
expose complete case-to-cluster traceability.

## Phase 5: Highest-Value Capability Families

- Implement clock fanout/loading evidence and reviewed buffering.
- Implement MCU programming/debug load and shared-pin constraints.
- Implement whole-bus buffering and level translation.
- Add reviewed converter and isolation primitives with dynamic and safety
  applicability.
- Preserve stable gaps outside each reviewed envelope.

Acceptance: implementations are generic, registry/catalog backed, identity
neutral, and improve discovery cases without special casing.

## Phase 6: Held-Out Generalization

- Re-evaluate the untouched held-out corpus.
- Require improvement for every implemented family on at least one held-out
  case.
- Prove no unsafe promotion, no ready-case regression, and no weakened safety
  evidence.
- Compare reports using identical corpus, policy, and registry contracts.

Acceptance: held-out readiness strictly improves and remaining gaps retain
stable normalized identities.

## Phase 7: Physical Promotion

- Lower every newly ready held-out case through the normal design workflow.
- Run simulation, routing, connectivity, writer, ERC, strict DRC, and
  zero-difference round-trip gates.
- Run every scenario twice in each of two clean local roots.
- Build and independently verify identical content-addressed bundles.

Acceptance: every promoted case satisfies the complete local KiCad contract
with deterministic replay and complete evidence traceability.

## Phase 8: Regression, Audit, And Release

- Run the full local short suite and all affected installed-KiCad matrices.
- Preserve protected USB-C, amplifier, MCU/ESP32, hierarchical, dynamic, and
  12/12 benchmark evidence.
- Publish final reports, checksums, capability deltas, and audit.
- Review the staged diff with Prism and resolve high/medium findings.
- Commit and push the clean result without starting or monitoring GitHub
  Actions.

Acceptance: the specification is proven requirement by requirement, local
evidence is reproducible, Prism is clear of high/medium findings, and the
pushed tree is clean.
