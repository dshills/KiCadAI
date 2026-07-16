# Generic Circuit Agent Repair Harness Plan

Date: 2026-07-15
Status: Complete

## Phase 1: Contract

Define repair-plan states, stop reasons, patch derivation rules, input-hash
history, and machine-readable result types.

**Files:** `specs/generic-circuit-repair-harness/*`, `internal/circuitgraph/*`.

**Acceptance:** safe candidates can be proven valid without applying them.

**Completed:** `90786c6`.

## Phase 2: Deterministic Planner

Implement a pure planner over preflight repair options. It selects only one
fully-derived option, validates its patch, and fail-closes otherwise.

**Files:** `internal/circuitgraph/repair_plan.go`, tests.

**Acceptance:** stable hash/state/stop reason and no fixture-specific logic.

**Completed:** `c166358`.

## Phase 3: CLI Integration

Add `circuit repair-plan`, reuse shared preflight, and enforce no-write
behavior and repeatable hash flags.

**Files:** `cmd/kicadai/circuit_preflight.go`, tests.

**Acceptance:** JSON output contains preflight, selected option/patch where
safe, external evidence, and structured diagnostics.

**Completed:** `c166358`.

## Phase 4: Convergence Corpus

Prove RC and LM358 repair loops, valid ready paths, ambiguous/review/repeated
negative paths, and preserve direct/provider fixtures.

**Acceptance:** emitted patch is accepted by `circuit patch`; the harness never
applies it itself.

**Completed:** `e32b46a`, `e269a76`, and `245d9cb`.

## Phase 5: Documentation and Closeout

Document the agent loop, resolve the CI baseline, run full quality gates,
Prism-review staged work, and commit each phase.

**Completed:** documentation and final quality/CI evidence are recorded with
the closeout commit.
