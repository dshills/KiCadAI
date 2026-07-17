# Adversarial Function-Level Corpus Implementation Plan

Date: 2026-07-17

## Phase 1: Freeze Before Implementation

1. Check in the specification and plan.
2. Add all 18 function-only inputs.
3. Record per-file SHA-256 values and an immutable manifest digest.
4. Add integrity tests for membership, hashes, semantic pressure coverage, and
   prohibited physical/provider-controlled fields.

Acceptance: corpus integrity passes without any production capability change.

## Phase 2: Untouched Baseline

1. Attempt strict decode, validation, synthesis, resolution, lowering, project
   creation, and applicable trusted simulation for every case.
2. Stop each case at its earliest blocking stage.
3. Publish the baseline pass rate and failure counts by stable category/code.
4. Preserve deterministic baseline bytes.

Acceptance: no failure is hidden, skipped, or classified from message text when
a structured stage/code exists.

## Phase 3: Frequency-Ordered Generic Closure

1. Rank failures by category and stable code.
2. Fix the highest-frequency root cause through generic vocabulary or reviewed
   catalog evidence.
3. Regenerate the report and repeat.
4. Record each category-count delta so a correction cannot silently trade one
   failure class for another.

Likely reusable capabilities include signed operating rails, semantic
multi-channel resolution, level-translation catalog families, adjustable-
regulator companion calculations, SPI sensor policies, transient operating
cases, and physical retry improvements. The baseline, not this list, decides
the order.

Acceptance: no production path references corpus identity, and every correction
improves or preserves authoritative gate evidence.

## Phase 4: Electrical Evidence

1. Run existing DC/AC/nonlinear models wherever graph completeness permits.
2. Derive bounded transient analyses only from trusted catalog models and
   function-intent operating parameters.
3. Add bounded tolerance/sensitivity evidence when nominal-only proof is the
   leading unresolved electrical category.
4. Fail closed on missing models, ranges, convergence, or operating limits.

Acceptance: providers supply conditions and assertions, never device equations,
models, matrices, or solver policy.

## Phase 5: Physical Promotion

1. Promote passing cases through schematic generation and readability.
2. Exercise generic placement, endpoint access, routing, and bounded repair.
3. Require connectivity, route completion, writer correctness, and strict
   round-trip gates.
4. Run real KiCad ERC/DRC for every promoted case.

Acceptance: every reported pass satisfies the complete gate profile.

## Phase 6: Closeout

1. Run all Go tests.
2. Run the original and adversarial KiCad-backed corpora.
3. Run both protected USB-C regressions.
4. Review staged changes with Prism and resolve all high/medium findings.
5. Commit, push, and verify GitHub Actions.
6. Audit every requirement in the specification against authoritative evidence.

Acceptance: the completion rule is proven without weakening the frozen corpus.
