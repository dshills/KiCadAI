# Generic Circuit Graph Patch Implementation Plan

Date: 2026-07-15
Status: Complete

## Delivery Rules

- Maintain the existing `generic-circuit-v1`, provider, preflight, and direct
  creation behavior.
- Keep graph patching provider-free and project-write-free.
- Use Prism before every phase commit.
- Run focused tests per phase; run `make test`, `make lint`, and
  `make coverage-check` before completion.

## Phase 1: Contract And Patch Model

**Files:** `specs/generic-circuit-patch/*`, `internal/circuitgraph/patch.go`,
`internal/circuitgraph/patch_test.go`.

Define strict patch schema/types, operation limits, stable diagnostic codes, and
deterministic normalization. Add parse and schema-negative tests.

**Acceptance:** Unknown fields/operations and invalid typed selectors fail
closed with stable diagnostics. No graph mutation exists yet.

**Risk:** Low; isolated model and specification.

## Phase 2: In-Memory Fail-Closed Applier

**Files:** `internal/circuitgraph/patch_apply.go`, tests and fixtures under
`internal/circuitgraph` and `examples/circuit-graph`.

Implement clone-only application for supported component, endpoint, no-connect,
PCB placement/region, and policy operations. Strict-decode the corrected graph
after application and reject immutable/conflicting mutations.

**Acceptance:** Supported mutations normalize deterministically; rejected
operations leave the input document unchanged.

**Risk:** Medium; endpoint and multi-unit semantics must match existing graph
validation exactly.

## Phase 3: CLI Patch And Shared Re-Preflight

**Files:** `cmd/kicadai/circuit_preflight.go`, new CLI tests.

Add `circuit patch` argument handling, atomic corrected-graph output, critical
projection/diff response data, and reuse of the existing `evaluateCircuitPreflight`
path. No duplicate catalog/lowering/placement/routing implementation.

**Acceptance:** `circuit patch` never writes KiCad files; blocked patch or
blocked re-preflight creates no corrected graph. Ready corrected graph includes
structured evidence.

**Risk:** Medium; CLI output paths and write ordering must remain fail-closed.

## Phase 4: Acceptance Corpus And Direct Create Loop

**Files:** patch examples, `cmd/kicadai/circuit_preflight_test.go`, optional
KiCad test coverage.

Add repair fixtures for unknown component, invalid pin, invalid multi-unit, and
invalid placement graph; add negative conflict/immutable/unsafe tests. Prove
the repaired RC graph directly creates a deterministic project. Preserve
existing USB-C LED, BMP280, LMV321, dual-LMV321, and LM358 coverage.

**Acceptance:** Supported repair loop is reproducible and rejected cases write
neither graph nor project. Optional KiCad checks remain environment-gated.

**Risk:** Medium; fixture expectations may expose existing generic constraints.

## Phase 5: Documentation And Completion Audit

**Files:** `README.md`, `docs/cli-reference.md`, `docs/kicadai-agent-skill.md`,
`docs/ai-generation.md`, `specs/ROADMAP.md` as warranted.

Document the machine-readable agent loop and evidence boundaries. Run complete
offline quality gates and inspect existing provider/direct fixture regressions.

**Acceptance:** Docs do not claim external KiCad evidence from patch/preflight;
all quality gates and Prism pass, and the worktree is clean.

**Risk:** Low.

## Completion Evidence

- Phase 1: `6bb2c31` defined the contract and implementation plan.
- Phase 2: `dee8bba` and `004b794` added the strict model and clone-only
  applier.
- Phase 3: `fd65169` added the provider-free CLI and shared re-preflight path.
- Phase 4: `2f5cc19` added selector repair; the final completion commit adds
  semantic-repair decoding, projection evidence, the full repair corpus, and
  direct create-loop coverage.
- Phase 5: documentation and roadmap updates accompany complete offline test,
  lint, coverage, and staged Prism review evidence.
