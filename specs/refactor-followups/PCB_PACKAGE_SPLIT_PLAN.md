# PCB Package Reorganization Plan

## Goal

Reorganize the existing PCB Go package into smaller ownership files without
changing output bytes, parser behavior, or validation semantics. This is not a
sub-package split; all files remain in `internal/kicadfiles/pcb` unless a later
plan explicitly justifies a package boundary.

## Current Pain

The PCB package currently mixes data model structs, parser code, renderer code,
validation, preservation fields, generated fixtures, and connectivity helpers in
large files. That makes small writer changes harder to audit because unrelated
concepts appear in the same diff.

## Proposed Package Layout

- `model.go`: public PCB document structs, enums, typed aliases, and small value
  helpers.
- `read.go`: S-expression parser entry points and node-to-model conversion.
- `render.go`: writer entry points and model-to-S-expression conversion.
- `validate.go`: semantic validation and field-level validation helpers.
- `preserved.go`: raw/unknown node preservation and round-trip metadata.
- `connectivity.go`: net, pad, via, track, and zone connectivity helpers.
- `fixtures.go`: only constructors required by runtime CLI/library behavior.
  Test-only fixtures shared across packages move to `internal/testutil` or a
  build-tagged test helper so they do not inflate production binaries;
  package-private fixtures can move behind `_test.go`.

## Phase 1 - Baseline Guardrails

1. Capture current package tests, golden corpus tests, and round-trip tests.
2. Add package-level comments naming the intended file ownership boundaries.
3. Add a temporary checklist in the PR description for files moved and tests run.

Acceptance:

- `go test ./internal/kicadfiles/pcb ./internal/kicadfiles/roundtrip` passes
  before any moves.
- No behavior changes are included in the first commit.

## Phase 2 - Move Model Types

1. Move core structs and constants into `model.go`.
2. Keep exported identifiers unchanged.
3. Move only code needed by those types, such as simple enum string helpers.

Acceptance:

- No caller imports change.
- `git diff --word-diff` shows moved code only.

## Phase 3 - Move Parser Code

1. Move `Read`, parse helpers, and S-expression traversal helpers into
   `read.go`.
2. Keep preservation hooks colocated only until `preserved.go` is introduced.
3. Add no parser behavior in this phase.

Acceptance:

- Existing PCB reader tests pass unchanged.
- Round-trip tests show no new diffs.

## Phase 4 - Move Renderer Code

1. Move `Render`, writer helpers, and ordering-sensitive render sections into
   `render.go`.
2. Keep render ordering tests unchanged.
3. Add comments only where ordering is KiCad-sensitive.
4. Confirm the renderer sorts any map-backed collections before emission and
   emits LF line endings directly. Golden comparisons should also normalize LF
   to keep failures focused on semantic render changes.
5. If render-time sorting becomes expensive, introduce ordered slices or an
   ordered map wrapper so ordering cost is paid during mutation rather than on
   every render.
6. Add or confirm `.gitattributes` rules that force LF for golden fixtures and
   KiCad text outputs used in byte-identical tests.

Acceptance:

- Golden PCB render fixtures are byte-identical.
- No output formatting changes are accepted in this phase.

## Phase 5 - Move Connectivity Helpers

1. Move net lookup, pad lookup, track/via connectivity, and zone connectivity
   helpers into `connectivity.go`.
2. Add small unit tests around any helper that becomes exported inside the
   package.

Acceptance:

- PCB object correctness and DRC feedback-loop tests continue to pass.

## Phase 6 - Move Validation And Preservation

1. Move validation-only code into `validate.go`, using the connectivity helpers
   already isolated in Phase 5.
2. Move raw/unknown node preservation code into `preserved.go`.
3. Keep validation error text stable unless a separate correctness issue exists.

Acceptance:

- Existing validation tests pass.
- Unknown-node preservation tests pass.

## Non-Goals

- No new KiCad object support.
- No render ordering changes.
- No parser normalization changes.
- No package rename.
