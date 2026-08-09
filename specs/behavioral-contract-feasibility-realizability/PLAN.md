# Behavioral-Contract Feasibility and Realizability Gate Plan

Status: implementation plan

## Phase 0: Freeze the contract

- Commit `SPEC.md` and this plan before outcome-changing code.
- Preserve the existing V1 `CONTRACT.sha256`, corpus manifest, baseline and
  selection hashes, V2 corpus manifest/checksum, baseline/seal/selection
  hashes, and their frozen reproduction tests. The implementation must not
  modify any file covered by those commitments.

## Phase 1: Requirement classifier

- Add typed, deterministic realizability findings to open-topology synthesis.
- Implement conservative per-operating-case direct-supply envelopes.
- Implement the closed voltage-metric rules.
- Derive multi-output and converging multi-control obligation findings.
- Add boundary, adversarial, normalization, and replay tests using synthetic
  public requirements.

## Phase 2: Versioned feedback integration

- Preserve the v1 observation entrypoint and frozen evidence behavior.
- Add a v2 observation policy that refines only terminal topology failures.
- Map findings to `energy_domain_creation` and
  `multi_obligation_composition` with the original terminal failure retained as
  a downstream symptom.
- Add aggregation tests proving that previously conflated cases form distinct
  clusters while unclassified topology failures remain unchanged.

## Phase 3: Preservation and review

- Run focused open-topology feasibility, synthesis, capability-feedback, frozen
  V1/V2 evidence, and specification contract tests locally.
- Run broader local regression tiers in proportion to changed production
  behavior; do not start or inspect GitHub Actions.
- Review the staged diff with the repository's configured Prism external
  staged-diff review, remediate actionable findings, commit, and push.

## Phase 4: V3 experiment handoff

- Write a fresh V3 continuation addendum that binds the v2 classifier policy.
- Independently author and seal a new corpus only after the policy and its
  public tests are frozen.
- Baseline, cluster, select, implement, and validate under the unchanged
  closed-loop trust boundary.
