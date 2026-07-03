# Parallel AI Generation Workstreams Spec

## Purpose

KiCadAI has several roadmap tracks that can advance independently:

- KiCad-backed fixture promotion;
- component and circuit-block catalog expansion;
- routing/placement hardening;
- intent-planner and AI workflow polish.

The project needs machine-readable coordination so these tracks can be split
across contributors or agents without losing dependency information.

## Scope

- Extend the AI readiness matrix with parallel workstream metadata.
- Keep the existing readiness categories, task types, and amplifier matrix.
- Make dependencies between readiness records explicit and validated.
- Expose parallel workstream counts in coverage summaries.
- Document the intended parallel execution model.

## Non-Goals

- Do not implement a scheduler or multi-agent runtime.
- Do not auto-create git worktrees.
- Do not promote any `expected_fail` fixture without real evidence.
- Do not claim autonomous generation is complete.

## Data Model

Each AI readiness record may include:

- `parallel_group`: stable workstream ID for independent execution grouping;
- `depends_on`: optional list of readiness record IDs that must be advanced
  before this record can be considered independently promotable. It is a
  prerequisite relation, not a request for simultaneous work.

The `parallel_group` field is optional while older records are being migrated.
When present, it must match one of the values below; other values are invalid.
Records without the field are reported as `unassigned` in coverage summaries.
The Go constants added in `internal/aireadiness` are the implementation source
of truth:

- `fixture_promotion`;
- `catalog_block_expansion`;
- `engine_hardening`;
- `intent_ai_ux`;
- `documentation`.

Dependencies must:

- reference globally unique existing record IDs;
- use the existing `<domain>.<category>.<slug>` record ID format;
- not reference the same record;
- be lexicographically sorted by list element in checked-in JSON;
- form a directed acyclic graph. Validation must reject dependency cycles.
  The Go validator is responsible for enforcing ordering and cycle checks
  across the fully loaded matrix, because records may live in different domain
  files.

Record IDs must be unique across the fully loaded readiness dataset. The Go
validator must reject duplicate record definitions before dependency checks run,
because duplicates would make dependency references ambiguous.

Dependency satisfaction is readiness-level aware. A dependent record may advance
through `draft`, `connectivity`, or `candidate` while prerequisites are still in
progress, but promotion to `verified` requires every referenced dependency to be
`verified`. This avoids blocking useful work while keeping final completion
claims tied to completed prerequisites. The Go validator must continuously
enforce this invariant: any record whose readiness is `verified` is invalid if
any `depends_on` target is below `verified`.

`parallel_group` is a primary owner, not a tag list. If a record affects
multiple workstreams, assign it to the group that owns the next concrete task
and use `depends_on` to expose cross-group prerequisites.

Record IDs are stable public references once checked in. Renaming a record ID
is allowed only when the same change updates every inbound `depends_on`
reference. The Go validator must reject dangling references across the fully
loaded matrix so stale renames fail in tests. Documentation that names the old ID
must be updated as a manual checklist item because arbitrary Markdown references
are outside the validator's scope.

Existing readiness values remain unchanged:

- `missing`;
- `draft`;
- `connectivity`;
- `candidate`;
- `verified`.

`expected_fail` is not a readiness value in this matrix. It is fixture/test
metadata used elsewhere in the project for KiCad-backed generated designs.

Cross-group dependencies are allowed, but they mean the dependent record is not
fully independent from the referenced group. Coverage summaries and docs should
keep these dependencies visible so agents can split work around them instead of
assuming every group can advance in isolation.

## Coverage Summary

Domain coverage summaries must include total readiness record counts by
`parallel_group` so agents can answer "what can be worked on in parallel?"
without parsing all records manually. Records without a `parallel_group` must
be counted under `unassigned`, so group totals match the domain total.

## Acceptance Criteria

- AI readiness schema validates `parallel_group` and `depends_on`.
- Amplifier readiness records declare useful parallel groups and dependencies.
- Coverage tests assert group counts.
- Documentation tells agents how to split work safely.
- `go test ./internal/aireadiness` and `go test ./...` pass.
