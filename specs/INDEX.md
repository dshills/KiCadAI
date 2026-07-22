# Specs Index

This directory contains active project specifications, implementation plans,
roadmaps, and dated review material.

## Active Roadmap

- [ROADMAP.md](ROADMAP.md) - current project roadmap.

## Latest Completed Milestone

- [Uncertainty-aware behavioral intent compilation](uncertainty-aware-behavioral-intent-compilation/SPEC.md)
  - completed behavior-first compiler specification.
- [Implementation plan](uncertainty-aware-behavioral-intent-compilation/PLAN.md)
  - completed implementation phases and acceptance gates.
- [Completion audit](uncertainty-aware-behavioral-intent-compilation/AUDIT.md)
  - frozen-corpus and installed-KiCad evidence.

## Active Spec Areas

Subdirectories group feature specs and plans by area. Prefer adding new work to
the relevant subdirectory instead of adding loose historical files at the root.

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
