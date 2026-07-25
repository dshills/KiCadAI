# Hierarchical Multi-Domain Circuit Synthesis Plan

## Objective

Generate, verify, and physically realize previously unseen multi-block,
multi-domain circuits through a generic deterministic hierarchy rather than a
flat collection of functional selections.

Status: Complete.

## Phase 1: Specification, Corpus Freeze, And Baseline

- Publish the V4 additive contract and generated-evidence invariants.
- Freeze six behavior-only requirements with at least four interacting
  functional obligations and at least two electrical domains each.
- Pin manifest membership and fixture bytes with SHA-256.
- Add an independent strict mirror decoder and neutrality/freeze tests.
- Record the untouched V3 baseline before production V4 support.

Acceptance: the frozen-corpus test passes, the corpus has no implementation or
hierarchy hints, and the baseline proves the new evidence is unavailable.

## Phase 2: V4 Model And Hierarchical Decomposition

- Add strict V4 decoding, normalization, limits, and canonical hashing while
  preserving V1-V3 replay.
- Derive stable system, subsystem, and block nodes from typed behavioral
  connectivity and electrical domains.
- Account for every participant, objective, provider child, assertion, and
  hierarchy edge exactly once.
- Validate ownership, acyclicity, canonical ordering, and behavior under input
  reordering.

Acceptance: synthetic positive and mutation tests prove complete deterministic
decomposition and fail closed on missing, duplicate, cyclic, or ambiguous
ownership.

## Phase 3: Contracts And Shared Resources

- Generate typed contracts for every hierarchy boundary.
- Solve electrical, protocol, timing, loading, noise, thermal, startup,
  protection, and isolation compatibility.
- Build globally accounted resource plans for rails, references, clocks,
  resets, sequencing, decoupling, connectors, grounding, thermal capacity, and
  protection.
- Emit bounded calculations and structured diagnostics.

Acceptance: boundary and budget mutations fail before selection; every
resource consumer is accounted exactly once with proven capacity and margin.

## Phase 4: Deterministic Global Backtracking

- Retain canonical complete hierarchical candidates.
- Feed contract, resource, simulation, physical, and downstream closure
  failures back into candidate selection.
- Distinguish local repairs from global candidate backtracking.
- Preserve bounded execution and explicit unsupported, ambiguous, unsafe,
  unproven, and exhausted terminal states.

Acceptance: generic synthetic cases prove that a locally preferred but
globally invalid architecture is rejected and the next safe complete
architecture is selected byte-identically under reordered input.

## Phase 5: Verification, Physical Plan, And Traceability

- Derive block-scoped and end-to-end analysis plans across all operating
  corners.
- Require model provenance and complete critical-assertion coverage.
- Generate generic physical partitions and crossing/locality constraints.
- Propagate hierarchy, contracts, resources, decisions, and requirement IDs
  through lowering, transactions, KiCad objects, and verification reports.

Acceptance: every selected object and proof is traceable; missing simulation,
physical, or traceability evidence fails closed.

## Phase 6: Held-Out Promotion And Regression

- Promote all six frozen systems through closed-loop synthesis and lowering.
- Run connectivity, routing, writer correctness, deterministic replay, and
  zero-difference round trips.
- Run clean installed-KiCad ERC and strict DRC for every system.
- Run the 12/12 held-out benchmark and all amplifier, MCU/sensor, ESP32,
  protected USB-C, writer, routing, fabrication, and promotion regressions.
- Produce two identical clean-checkout promotion bundles and publish the audit.

Acceptance: all held-out and preserved gates pass with authoritative evidence
and no fixture-specific production logic.

## Phase 7: Review And Closeout

- Review the complete staged diff with Prism.
- Resolve every high and medium finding.
- Re-run affected and full gates after review fixes.
- Commit, push, and verify GitHub Actions for the exact revision.

Acceptance: the worktree is clean, the exact pushed SHA is green, and the
requirement-by-requirement audit proves every specification item.
