# Specs Index

This directory contains active project specifications, implementation plans,
roadmaps, and dated review material.

## Active Roadmap

- [ROADMAP.md](ROADMAP.md) - current project roadmap.

## Latest Completed Milestone

- [Held-out capability expansion](held-out-capability-expansion/SPEC.md)
  - frozen behavior-only benchmark and generic capability-expansion contract.
- [Implementation plan](held-out-capability-expansion/PLAN.md)
  - ranked failure closure, regression, and clean-checkout promotion phases.
- [Completion audit](held-out-capability-expansion/AUDIT.md)
  - records the measured 5/12 to 11/12 improvement, verification, and receipts.
- [Completion audit](clean-checkout-kicad-promotion/AUDIT.md)
  - exact reproduction, corpus, bundle, review, commit, and Actions evidence.

## Active Spec Areas

Subdirectories group feature specs and plans by area. Prefer adding new work to
the relevant subdirectory instead of adding loose historical files at the root.

- [Held-out capability expansion specification](held-out-capability-expansion/SPEC.md)
  - freezes a twelve-case behavior-only benchmark across six domains and the
    fifteen-stage promotion contract.
- [Held-out capability expansion implementation plan](held-out-capability-expansion/PLAN.md)
  - ranks the untouched baseline and promotes reusable support for two
    electrically distinct families.
- [Held-out baseline](held-out-capability-expansion/BASELINE_REPORT.json) and
  [final report](held-out-capability-expansion/FINAL_REPORT.json)
  - record the measured improvement from 5/12 to 11/12 complete passes with
    standalone clock generation remaining fail-closed.
- [Held-out promotion matrix](held-out-capability-expansion/PROMOTION_MATRIX.json)
  - binds the five newly promoted cases to clean-checkout installed-KiCad
    evidence.
- [Held-out capability expansion completion audit](held-out-capability-expansion/AUDIT.md)
  - binds the generic implementation, final report, regression gates,
    clean-checkout receipts, review, and delivery evidence.
- [External review mitigation specification](external-review-mitigation/SPEC.md)
  - closes the confirmed placement, stock-library, CLI, discoverability, and
    evidence-artifact findings from the 2026-07-21 independent review.
- [External review mitigation implementation plan](external-review-mitigation/PLAN.md)
  - phases the generic fixes, KiCad-backed regression ladder, Prism review,
    commits, push, and CI verification.
- [External review mitigation baseline](external-review-mitigation/BASELINE.md)
  - freezes the reproduced findings, durable fixtures, known-failure tests, and
    initial test evidence before implementation.
- [Independent test-session feedback](FEEDBACK.md)
  - source review and reproduction context for the mitigation milestone.

## Archive

- [archive/README.md](archive/README.md) - superseded reviews, older fix plans,
  and retired roadmap snapshots.
- [July 2026 code review](archive/CODE_REVIEW_07_02_2026.md) and
  [remediation plan](archive/CODE_REVIEW_FIX_PLAN_07_02_2026.md) - historical
  review material; the tracked findings have been closed or superseded.
