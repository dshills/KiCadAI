# Held-Out Capability Expansion Implementation Plan

Date: 2026-07-23

## Phase 1 — Specification And Pre-Implementation Freeze

- Check in this specification and plan.
- Freeze twelve behavior-only requirements across analog, power, digital, MCU,
  sensor, and mixed-signal domains.
- Pin prompt text and requirement bytes with SHA-256.
- Add an implementation-independent manifest integrity and neutrality test.
- Prove every requirement enables the complete acceptance profile.

Acceptance: the corpus and integrity tests are committed before any production
capability is added for held-out cases.

## Phase 2 — Stage-Coverage Evaluator And Untouched Baseline

- Add a deterministic evaluator with the fifteen specification stages.
- Record the earliest structured blocker and cumulative stage reach per case.
- Run all cases against the exact Phase 1 commit and installed KiCad.
- Freeze `BASELINE_REPORT.json` and its checksum.
- Rank failure clusters by frequency, domain breadth, safety criticality, and
  stable capability ID.

Acceptance: every case is a full pass or has one reproducible structured
blocker; the report cannot be regenerated to different bytes from unchanged
inputs and tools.

## Phase 3 — First Generic Unsupported Family

- Add or extend public semantic vocabulary only as required by the measured
  first cluster.
- Implement a generic provider using typed ports and behavioral constraints.
- Add reviewed catalog evidence and deterministic value/rating solvers.
- Add trusted simulation bindings and corner evaluation.
- Add generic lowering and physical-design semantics.
- Prove rejection at electrical, tolerance, thermal, and physical boundaries.
- Run the affected optional KiCad-backed cases after every correction.

Acceptance: every benchmark case in the first family passes the complete
promotion profile without identity-aware production code.

## Phase 4 — Second Electrically Distinct Family

- Repeat Phase 3 for the next ranked electrically distinct cluster.
- Prove that the second provider does not reuse a misleading evidence model
  from the first family.
- Exercise composition with an existing downstream capability where present.

Acceptance: every benchmark case in the second family passes the complete
promotion profile without weakening the first family or controls.

## Phase 5 — Physical And Writer Closure

- Resolve generic endpoint-access, placement, branch-order, layer-transition,
  and routing issues exposed by the new families.
- Preserve transaction operation provenance through generated tracks and vias.
- Require writer correctness, zero normalized round-trip differences, and
  byte-identical replay.
- Rerun each affected installed-KiCad case after every physical correction.

Acceptance: both new families have clean ERC, strict DRC, connectivity,
complete routing, correct writers, zero round-trip differences, and stable
replay.

## Phase 6 — Regression And Measured Final Report

- Regenerate the benchmark report with the frozen evaluator and gate profile.
- Prove increased full-pass count and cumulative stage reach.
- Run all Go tests and existing promotion corpora.
- Run both protected USB-C fixtures with installed KiCad.
- Search production code for corpus identities and prohibited shortcuts.

Acceptance: no control or existing promotion regresses and the final report
meets every measurable-improvement rule.

## Phase 7 — Clean-Checkout Promotion Bundles

- Add each new family to a versioned promotion matrix.
- Execute the matrix in a clean checkout using installed KiCad.
- Package and verify all required evidence and checksums.
- Re-run the verifier after copying the bundle outside the working tree.

Acceptance: every new family has a self-contained verified promotion bundle
whose manifest and receipt bind the exact source and tool identities.

## Phase 8 — Review And Delivery

- Audit every specification requirement against authoritative current-state
  evidence.
- Review staged changes with Prism and resolve all high/medium findings.
- Commit intentional phase boundaries, push the final exact commit, and verify
  GitHub Actions.
- Update project documentation with measured capability—not arbitrary-design
  claims.

Acceptance: the completion audit is fully evidenced, the worktree is clean,
and GitHub Actions is green for the pushed commit.
