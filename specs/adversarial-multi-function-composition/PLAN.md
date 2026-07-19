# Adversarial Multi-Function Circuit Composition Plan

## Objective

Prove that open-set search can build and globally validate unfamiliar circuits
containing multiple interacting functional objectives, without fixture-aware
production logic or topology-specific request mappers.

Status: Complete (2026-07-19).

## Phase 1: Specification And Pre-Implementation Freeze

- Publish the v2 typed-signal and system-constraint contract.
- Freeze ten behavior-only requirements with two to four objectives each.
- Pin manifest membership and fixture bytes with SHA-256.
- Add strict schema, representative-coverage, neutrality, and membership tests.

Acceptance: the freeze test passes before production v2 implementation begins.

## Phase 2: V2 Requirement Model

- Add strict v2 decoding while preserving v1 compatibility.
- Normalize and validate signals, signal bindings, derived-domain sources, and
  system constraints.
- Preserve order-neutral canonical hashes and stable diagnostics.

Acceptance: malformed, contradictory, unresolved, and ambiguous contracts fail
closed; v1 replay bytes do not change.

## Phase 3: Typed Multi-Objective Search

- Convert signal bindings into shared typed anchors.
- Validate producer/consumer cardinality and end-to-end contracts.
- Add deterministic per-obligation coverage accounting.
- Preserve bounded alternatives and selection rationale.

Acceptance: generic synthetic tests prove composition, rejection,
unsupported/ambiguous/exhausted states, and order-independent replay.

## Phase 4: Global Candidate Reasoning

- Prove derived-domain voltage provenance and aggregate current budgets.
- Validate startup state, loading/drive, stability, tolerances, and board
  limits across complete candidates before scoring.
- Emit structured global calculation and rejection evidence.

Acceptance: boundary mutations fail closed and no global failure reaches
selection.

## Phase 5: Generic Provider Coverage

- Extend reusable providers and catalog-backed expansions for the frozen
  capabilities.
- Keep provider inputs capability-neutral and corpus-identity blind.
- Exercise providers with synthetic voltage, current, frequency, loading, and
  tolerance mutations independent of corpus files.

Acceptance: all required capabilities are selected through the registry; code
search finds no fixture IDs, paths, expected parts, or private schemas.

## Phase 6: Lowering And Offline Promotion

- Lower shared signals and derived supplies into normal function-level graph
  anchors without topology-specific mappers.
- Recheck component/rating/value/tolerance evidence after catalog resolution.
- Run deterministic replay, writer, zero-diff round-trip, connectivity, and
  routing gates for all ten held-out requirements.

Acceptance: every corpus item passes every offline gate without corpus edits.

## Phase 7: KiCad Promotion And Regression Closeout

- Run clean ERC and strict DRC for every promoted held-out circuit.
- Run all existing applicable regression and optional KiCad-backed suites.
- Publish a requirement-by-requirement audit and measured coverage report.
- Update the roadmap and AI-readiness matrix without claiming arbitrary
  generation.
- Review staged changes with Prism, commit, push, and verify GitHub Actions for
  the exact commit.

Acceptance: the complete audit has authoritative evidence for every goal item,
the worktree is clean, and GitHub Actions is green.

## Closeout Evidence

- Phases 1-5: strict v2 schema, typed signals/domains, global proof rules,
  coverage accounting, deterministic alternatives, catalog providers, and
  fail-closed mutation tests pass in `internal/architecturesearch` and
  `internal/circuitgraph`.
- Phase 6: all ten frozen requirements pass the offline resolver, lowering,
  writer, normalized round-trip, connectivity, routing, and deterministic
  replay workflow.
- Phase 7: all ten pass the installed KiCad promotion with clean ERC and
  strict DRC; the preserved `usb_c_i2c_sensor_3v3_protected` installed-KiCad
  regression also passes.
- The requirement-by-requirement and measured-coverage record is
  [AUDIT.md](AUDIT.md).
