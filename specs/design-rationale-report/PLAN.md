# Design Rationale And Known-Limit Report Implementation Plan

Date: 2026-06-26

## Phase 1: Rationale Model And Builder Skeleton

Create the internal report model and a minimal builder.

Tasks:

1. Add `internal/rationale`.
2. Define:
   - `Report`;
   - `SourceSummary`;
   - `IntentSummary`;
   - `Decision`;
   - `EvidenceRecord`;
   - `KnownLimit`;
   - `ValidationSummary`;
   - `NextAction`.
3. Add schema constant `kicadai.design.rationale.v1`.
4. Add `BuildFromPlan` for `intentplanner.PlanResult`.
5. Add deterministic normalization and sorting helpers.
6. Add JSON marshal helper.
7. Add unit tests for empty/minimal plan reports.

Acceptance:

- Package compiles without CLI dependencies.
- Minimal plan produces deterministic JSON.
- Status is derived conservatively.

Commit message:

`Add design rationale report model`

## Phase 2: Draft And Source Evidence Integration

Connect natural-language draft evidence to rationale reports.

Tasks:

1. Add `BuildFromDraftAndPlan`.
2. Map `intentdraft.ExtractionReport.Fields` to `EvidenceRecord` values.
3. Map draft clarifications to known limits and decisions.
4. Preserve source hash, source type, and summary.
5. Link planner decisions to source evidence where paths match.
6. Add tests for:
   - supported text draft;
   - blocked battery clarification;
   - unsupported interface clarification.

Acceptance:

- Natural-language source attribution appears in the report.
- Blocking clarifications produce `needs_clarification`.

Commit message:

`Link draft evidence into rationale reports`

## Phase 3: Planner Decision Mapping

Convert planner output into explicit decisions and known limits.

Tasks:

1. Map `SelectedBlockRecord` to `block_selection` decisions.
2. Map `ConnectionRecord` to `connection` decisions.
3. Map planner assumptions to report assumptions.
4. Map planner clarifications and known gaps to known limits.
5. Map planner issues to known limits with stable categories.
6. Add requirement IDs and evidence IDs where available.
7. Add tests using existing intent fixtures.

Acceptance:

- `intent-plan.json` information can be understood from the rationale report
  without reading the raw plan.
- Known gaps are visible and categorized.

Commit message:

`Map planner rationale decisions`

## Phase 4: Workflow And Validation Summary Mapping

Fold design workflow status into the report.

Tasks:

1. Add optional workflow input to the builder.
2. Summarize requested and achieved acceptance.
3. Count completed, blocked, warning, and skipped stages.
4. Convert workflow issues into known limits.
5. Map stage summaries to evidence records.
6. Add next-action suggestions for:
   - missing component evidence;
   - placement blocked;
   - routing skipped;
   - missing KiCad CLI evidence;
   - fabrication not proven.
7. Add tests using generated workflow fixture results.

Acceptance:

- A blocked workflow produces actionable next actions.
- A warning-only workflow reports `partial`, not `ready`.

Commit message:

`Summarize workflow validation rationale`

## Phase 5: `intent rationale` CLI

Expose rationale reports through the CLI.

Tasks:

1. Add `intent rationale` subcommand.
2. Support source modes:
   - `--request`;
   - `--text`;
   - `--file`;
   - `--target`.
3. Enforce exactly one source mode.
4. For `--request`, load and plan the structured request.
5. For `--text/--file`, run draft, then plan when safe.
6. For blocked draft, build a clarification-only report.
7. Write `design-rationale.json` when `--output` is provided.
8. Add CLI tests for success, clarification, conflict, and output writing.

Acceptance:

- `kicadai --json --text "..." intent rationale` returns a report.
- Unsafe prose never reaches planner generation.
- Existing intent subcommands remain compatible.

Commit message:

`Add intent rationale CLI`

## Phase 6: Target Artifact Loading

Build reports from generated project `.kicadai/` metadata.

Tasks:

1. Add target loader for `.kicadai/` artifacts.
2. Load:
   - `intent-draft.json`;
   - `intent-extraction.json`;
   - `intent-clarifications.json`;
   - `intent-plan.json`;
   - `generated-request.json`.
3. Tolerate missing optional artifacts as known limits.
4. Block when no supported rationale source exists.
5. Write `.kicadai/design-rationale.json` for target mode.
6. Add CLI tests with generated target fixtures.

Acceptance:

- Existing generated projects can produce rationale reports.
- Imported or unsupported targets fail closed with actionable issues.

Commit message:

`Load rationale reports from generated targets`

## Phase 7: Persist Rationale From Intent Create

Write rationale automatically from generation workflows.

Tasks:

1. Update `intent create` to build rationale after planning and workflow
   execution.
2. Persist `.kicadai/design-rationale.json`.
3. Include rationale artifact reference in command output.
4. Persist report even when workflow finishes `partial` or `blocked`, as long as
   managed metadata exists.
5. Add tests for generated text and structured request paths.

Acceptance:

- Every generated project from `intent create` has rationale metadata.
- Blocked workflow reports still explain why they stopped.

Commit message:

`Persist rationale artifacts from intent create`

## Phase 8: Documentation And Roadmap Update

Document the new rationale workflow.

Tasks:

1. Update `README.md` with:
   - `intent rationale` examples;
   - source mode rules;
   - report fields;
   - known limitations.
2. Update `specs/ROADMAP.md` to mark rationale foundation implemented.
3. Add example rationale command snippets for text and target modes.
4. Confirm documentation uses compiled `kicadai` command style.

Acceptance:

- Docs explain how AI callers should use the rationale report.
- Roadmap reflects the new foundation and remaining synthesis gaps.

Commit message:

`Document design rationale reports`

## Phase 9: Review And Compatibility Sweep

Perform final hardening.

Tasks:

1. Run `go test ./...`.
2. Run `prism review staged`.
3. Fix all high/medium findings.
4. Confirm no normal test requires KiCad CLI or network access.
5. Confirm existing `intent plan`, `intent explain`, `intent create`, and
   `intent draft` tests still pass.
6. Check working tree and commit final cleanup if needed.

Acceptance:

- Full tests pass.
- Prism has no unresolved high/medium findings.
- Work is committed phase by phase.

Commit message:

`Harden design rationale reports`

