# Imported Project Preservation Implementation Plan

## Phase 1: Preservation Report Model

Objective: create the shared data model and status logic without changing
existing command behavior.

Deliverables:

- Add `internal/preservation` package.
- Define report, status, scope, file report, object report, operation review,
  ownership, mutability, and preservation status types.
- Add normalization and summary helpers.
- Add conversion helpers from preservation blockers to `reports.Issue`.
- Keep the package independent from KiCad readers initially to avoid cycles.

Tests:

- status aggregation;
- summary counts;
- issue conversion;
- deterministic ordering;
- JSON shape.

Verification:

- `go test ./internal/preservation ./internal/reports`

Commit: `Add imported preservation report model`

## Phase 2: Project Inspection Preservation Summary

Objective: surface preservation evidence during read-only inspection.

Deliverables:

- Build a preservation report from existing `inspect.ProjectSummary`,
  `inspect.SchematicSummary`, and `inspect.PCBSummary` data.
- Classify project scope as generated, imported, or unknown using manifest
  status.
- Add preservation summary fields to inspect output where compatible with the
  existing JSON shape.
- Map schematic `RawItems`, PCB `Preserved`, unsupported nodes, and
  preservation-only nodes into file/object reports.
- Preserve current read-only inspect behavior for imported projects.

Tests:

- clean imported minimal project reports warning or clean status, not blocked;
- schematic raw items appear as preservation-only or unsupported according to
  classifier;
- PCB preserved nodes appear in preservation summary;
- generated manifest changes scope classification;
- inspect JSON remains deterministic.

Verification:

- `go test ./internal/inspect ./internal/preservation ./cmd/kicadai`

Commit: `Expose imported preservation in inspection`

## Phase 3: Evaluation Preservation Check

Objective: make read-only evaluation report imported preservation state.

Deliverables:

- Add an `imported_preservation` or `preservation` check to `evaluate project`
  when the target is imported or contains preservation-sensitive content.
- Keep read-only evaluation non-mutating and non-blocking for preservation-only
  content unless parsing fails.
- Mark unsupported-blocking content as failed for mutation readiness while still
  allowing evaluation to complete.
- Add suggestions that tell agents to avoid imported mutation and use generated
  project workflows for repairs.

Tests:

- `evaluate project` includes preservation check for imported target.
- preservation-only content is visible as a warning.
- unsupported-blocking content is visible as failed/blocked evidence.
- generated targets do not get noisy imported warnings unless preservation
  issues exist.
- CLI JSON includes check and issues.

Verification:

- `go test ./internal/evaluate ./internal/preservation ./cmd/kicadai`

Commit: `Add imported preservation evaluation check`

## Phase 4: Transaction Plan Operation Reviews

Objective: make imported edit planning explain exactly why operations are safe or
blocked.

Deliverables:

- Build a preservation report during `transactions.PlanTransaction` for existing
  project targets.
- Add operation reviews for mutation operations.
- Preserve existing blockers:
  - `PRESERVATION_CONFLICT` for unsupported imported content;
  - `UNSAFE_REMOVE` for imported symbol removal;
  - ambiguous/missing reference blockers;
  - unverified pinmap blockers;
  - write ordering blockers.
- Attach operation IDs where available.
- Ensure plan output carries enough preservation evidence for AI repair loops.

Tests:

- unsupported imported content blocks design-touching operations.
- safe read-only or non-mutating operations do not produce mutation blockers.
- remove symbol remains blocked.
- connect imported symbols remains blocked on pinmap evidence.
- operation review paths and operation IDs are deterministic.

Verification:

- `go test ./internal/transactions ./internal/preservation`

Commit: `Add imported preservation operation reviews`

## Phase 5: Imported Apply Fail-Closed Gate

Objective: ensure apply cannot bypass planning preservation evidence.

Deliverables:

- Recompute preservation report at imported apply start.
- Block imported apply unless:
  - target preservation status allows the operation class;
  - operation reviews are present or can be recomputed;
  - write_project ordering is valid;
  - hashes are fresh where plan evidence includes hashes.
- Keep all currently unsafe imported mutation paths blocked.
- Improve errors for missing preservation evidence.

Tests:

- imported apply without preservation approval is blocked.
- imported apply with unsupported raw nodes is blocked before write.
- missing/stale plan evidence blocks.
- generated apply path is unchanged.
- no files are modified in blocked imported apply tests.

Verification:

- `go test ./internal/transactions ./internal/repair ./cmd/kicadai`

Commit: `Gate imported apply on preservation evidence`

## Phase 6: Fixture Corpus For Imported Preservation

Objective: add durable fixtures that prove read-only and blocked imported
behavior.

Deliverables:

- Add fixtures under `examples/imported/` or `internal/preservation/testdata/`:
  - clean minimal imported project;
  - schematic with unsupported/preservation-only raw item;
  - PCB with preserved raw node;
  - local library table fixture;
  - duplicate reference fixture;
  - unsafe remove transaction;
  - safe-add candidate transaction.
- Add fixture metadata that states expected preservation status.
- Keep fixtures small and deterministic.

Tests:

- fixture loader validates metadata.
- inspect/evaluate/transaction plan expectations match metadata.
- blocked apply fixtures do not modify files.

Verification:

- `go test ./internal/preservation ./internal/inspect ./internal/evaluate ./internal/transactions`

Commit: `Add imported preservation fixtures`

## Phase 7: First Safe Add Planning Policy

Objective: define and test a narrow safe imported add class, initially as
plan-safe and apply-blocked unless all gates pass.

Deliverables:

- Classify isolated `add_symbol` plus optional `assign_footprint` and
  `add_no_connect` as `safe_add` only when:
  - root schematic parses cleanly;
  - reference is new;
  - library and pins resolve;
  - operation does not modify existing user-authored objects;
  - no unsupported/preservation-only content overlaps the target file;
  - `write_project` is final.
- Keep unsafe operations blocked.
- Expose why a candidate safe add is still apply-blocked if writer preservation
  is incomplete.

Tests:

- safe-add candidate produces `safe_add` operation review.
- duplicate reference blocks.
- missing symbol library blocks.
- presence of unsupported schematic content downgrades to blocked.
- assign footprint to imported existing ref remains blocked unless generated by
  the same transaction.

Verification:

- `go test ./internal/transactions ./internal/preservation ./internal/libraryresolver`

Commit: `Classify safe imported add plans`

## Phase 8: Optional Safe Add Apply Prototype

Objective: only if Phase 7 evidence is strong, allow the narrow safe-add class
to write imported root schematics.

This phase may be skipped or kept plan-only if implementation evidence shows
preservation remains insufficient.

Deliverables:

- Add explicit option gate for imported safe add apply.
- Require fresh preservation report immediately before write.
- Write root schematic only; do not mutate PCB, project settings, library tables,
  or local libraries.
- Re-read the schematic after write.
- Verify previous preservation-only/unsupported classification did not worsen.
- Return artifacts and preservation report.

Tests:

- safe add writes one new symbol to a clean imported root schematic when enabled.
- default apply still blocks without explicit option.
- unsupported raw item blocks even with option.
- post-write readback succeeds.
- failed apply leaves original file unchanged.

Verification:

- `go test ./internal/transactions ./internal/preservation ./cmd/kicadai`

Commit: `Prototype safe imported schematic add apply`

## Phase 9: CLI And Agent Documentation

Objective: make imported preservation behavior clear to users and AI agents.

Deliverables:

- Update README imported-project status.
- Update `docs/validation-and-analysis.md` with preservation check/report shape.
- Update `docs/kicadai-agent-skill.md` imported-project stop conditions.
- Update `docs/development.md` with fixture guidance.
- Update `specs/ROADMAP.md` Priority 7 current foundation and remaining work.
- Add command examples using compiled `kicadai`.

Tests:

- Docs use `kicadai`, not `go run ./cmd/kicadai`.
- CLI examples match implemented commands.

Verification:

- `go test ./...`

Commit: `Document imported project preservation`

## Prism And Commit Process

For each phase:

1. Implement the phase.
2. Run the targeted tests listed for the phase.
3. Stage only phase-relevant files.
4. Run `prism review staged`.
5. Fix actionable Prism findings.
6. Commit the phase.
7. Move to the next phase only after the worktree is clean or unrelated changes
   are explicitly ignored.

## Success Criteria

The implementation is complete when:

- imported project inspection and evaluation expose preservation evidence;
- transaction planning returns operation-level preservation reviews;
- imported apply is fail-closed without preservation approval;
- fixture coverage proves clean, warning, blocked, and unsafe imported cases;
- optional safe-add behavior is either implemented behind an explicit gate or
  explicitly deferred with plan-safe evidence only;
- docs tell agents when to stop instead of mutating imported projects.
