# Implementation Plan

Implementation status: complete. Phases 0 through 8 have local regression,
installed-KiCad promotion, completion-audit, and staged-review evidence; the
closing commit and push are recorded in repository history.

## Phase 0 — Freeze And Baseline

- Freeze five positive and three adversarial behavior-only requirements against
  the pre-implementation commit.
- Record checksums, coverage, required analyses, expected outcomes, and current
  deterministic failure evidence.

## Phase 1 — Requirement And Evidence Vocabulary

- Add bounded waveform metrics for oscillation frequency, duty cycle,
  peak-to-peak ripple, and conversion efficiency.
- Version reports where evidence shape changes and keep old recorded evidence
  fail-closed rather than silently reinterpreted.

## Phase 2 — Primitive Inventory And Provenance

- Expose only reviewed nonlinear/switching primitives already supported by the
  trusted registry.
- Validate analysis applicability, terminal roles, electrical ratings, dynamic
  parameters, thermal networks, and transient SOA envelopes.

## Phase 3 — Generic Architecture Operators

- Add relationship-derived candidates for bounded transfer, autonomous
  periodic feedback, controlled pulse power transfer, and regulated step-down
  energy transfer.
- Retain materially distinct alternatives and auditable rejection evidence.

## Phase 4 — Values And Trusted Dynamic Evaluation

- Derive component values from behavior and reviewed device limits.
- Add waveform measurements and deterministic periodic-window selection.
- Persist discontinuity, continuation, convergence, thermal, and SOA evidence.

## Phase 5 — Diagnosis And Repair

- Add generic diagnoses and bounded value/graph repairs for frequency, duty,
  ripple, regulation, efficiency, convergence, and stress failures.
- Re-run every affected corner and never repair around safety failures.

## Phase 6 — Readable And Switching-Aware Physical Synthesis

- Preserve conventional schematic flow and visible feedback/return paths.
- Derive switching-loop roles and enforce loop, keepout, return, and feedback
  constraints through generic physical evidence.

## Phase 7 — Promotion And Preservation

- Prove all five positive cases and all three adversarial outcomes.
- Prove deterministic replay, exhaustive corners, readability, ERC, strict DRC,
  connectivity, routing, writer correctness, and zero round-trip differences.
- Run the complete existing local preservation envelope.

## Phase 8 — Review And Closeout

- Record a requirement-by-requirement completion audit and update project docs.
- Run Prism on the staged diff, remediate actionable findings, commit, and push.
