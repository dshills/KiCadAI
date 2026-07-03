# Parallel AI Generation Workstreams Implementation Plan

## Phase 1: Spec And Plan

Goal: capture the parallel execution model.

Tasks:

- Add `SPEC.md` defining the parallel readiness metadata.
- Add this `PLAN.md`.
- Review with Prism and commit.

Validation:

- `git diff --check`
- `prism review staged`
- New `parallel_group` and `depends_on` validation is implemented in Phase 2.
  Before committing later phases, run the Go validator against the fully loaded
  checked-in readiness dataset so global references and cycles are checked even
  when only a subset of files is staged.

Commit:

- `Spec parallel AI generation workstreams`

## Phase 2: Readiness Schema And Coverage

Goal: make parallel workstream metadata first-class in code.

Tasks:

- Add `ParallelGroup` constants to `internal/aireadiness`.
- Add `parallel_group` and `depends_on` fields to readiness records.
- Update the Go-backed readiness schema model; there is no separate checked-in
  JSON Schema file for `data/ai-readiness` today.
- Validate allowed groups and dependency references.
- Implement DAG cycle detection and lexicographical `depends_on` element sort
  validation in `internal/aireadiness`.
- Add `ByParallelGroup` to domain coverage summaries.
- Add unit tests for validation and summary behavior.

Validation:

- `go test ./internal/aireadiness`
- `prism review staged`

Commit:

- `Track parallel AI readiness workstreams`

## Phase 3: Matrix And Documentation

Goal: apply the model to the current parallel tracks.

Tasks:

- Annotate amplifier readiness records with parallel groups and dependencies.
- Update `docs/ai-readiness.md`.
- Update `docs/kicadai-agent-skill.md`.
- Update `specs/ROADMAP.md` near-term sequence.

Validation:

- `go test ./internal/aireadiness`
- `go test ./...`
- `git diff --check`
- `prism review staged`

Commit:

- `Document parallel AI generation tracks`

## Completion Criteria

- The readiness matrix can answer which records belong to each parallel track.
- Invalid dependencies fail fast in tests.
- Agents have explicit guidance for splitting `fixture_promotion`,
  `catalog_block_expansion`, `engine_hardening`, `intent_ai_ux`, and
  `documentation` work.
