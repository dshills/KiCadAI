# Generic Circuit Repair Guidance Plan

Date: 2026-07-15
Status: Complete

## Phase 1: Contract and Data Model

**Files:** `specs/generic-circuit-repair-guidance/*`,
`internal/generationcapability/*`, `internal/circuitgraph/*`.

Define the repair capability contract, stable guidance types, dispositions,
operation-template invariants, and deterministic serialization.

**Tests:** model validation and capability-document equality.

**Acceptance:** capability output describes only existing patch operations and
does not imply automatic correction.

**Risk:** Low; additive machine-readable fields.

## Phase 2: Graph-Derived Repair Planner

**Files:** `internal/circuitgraph/repair_guidance.go`, focused tests.

Map only the four supported diagnostic families to evidence-backed templates
and alternatives. Omit ambiguous or non-graph/electrical cases.

**Tests:** component selector, endpoint selector, unit, region, ambiguity, and
determinism cases.

**Acceptance:** every agent-selectable template is compatible with the patch
schema after required values are supplied.

**Risk:** Medium; diagnostics must preserve stable source paths.

## Phase 3: Preflight and Capability Integration

**Files:** `cmd/kicadai/circuit_preflight.go`,
`cmd/kicadai/generation_capability*.go`, tests.

Attach guidance to preflight results and expose the same patch contract in CLI
and provider capability documents.

**Tests:** JSON output, deterministic repeated runs, no output writes.

**Acceptance:** provider and CLI capability paths share one source of truth.

**Risk:** Medium; preflight returns at several early gates.

## Phase 4: Repair Corpus

**Files:** `cmd/kicadai/circuit_preflight_test.go`, graph fixtures as needed.

Exercise unknown selector, invalid pin/function, invalid multi-unit, and
invalid region repairs end to end. Add negative omission cases.

**Tests:** graph -> preflight -> option -> patch -> ready preflight; direct
create for one corrected RC graph; existing fixture regressions.

**Acceptance:** no fixture-specific production logic or executable unsafe
options.

**Risk:** Medium; catalog alternatives and stage diagnostics may reveal gaps.

## Phase 5: Documentation and Completion

**Files:** `README.md`, `docs/cli-reference.md`,
`docs/kicadai-agent-skill.md`, `specs/ROADMAP.md`.

Document the autonomous repair loop and evidence boundary. Run focused tests,
`make test`, `make lint`, `make coverage-check`, staged Prism review, and
commit each completed phase.

**Acceptance:** full offline suite passes; optional KiCad checks remain gated;
worktree is clean.

**Risk:** Low.

## Completion Evidence

- Phase 1: `1f82e3a` defined the public repair contract and delivery plan.
- Phases 2-3: `bd85fe8` added deterministic graph-derived options, capability
  metadata, and preflight integration.
- Phase 4: `f0db449` added repairable preflight corpus coverage.
- Phase 5: documentation, full offline verification, and staged Prism review
  accompany the final completion commit.
