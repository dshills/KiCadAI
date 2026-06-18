# Repair Apply Hardening Specification

## 1. Objective

Make validation repair capable of applying safe repairs to real generated
KiCadAI project outputs, persisting the corrected project files, rerunning the
authoritative validation gates, and reporting the result without overstating
success.

The current closed-loop validation repair foundation can classify issues, build
repair plans, execute transaction-level repairs in memory, expose a repair CLI,
and surface an opt-in `validation_repair` workflow stage. The next step is to
connect that foundation to project-directory state so an agent can move from:

```text
validation failed -> repair plan exists
```

to:

```text
validation failed -> safe repair applied -> project rewritten -> validation passed
```

## 2. Current State

Implemented foundation:

- `internal/repair` classifier, planner, executor, runner, statuses, actions,
  and golden reports.
- Safe in-memory transaction executors for:
  - verified footprint assignment;
  - generated pad-net hints;
  - board outline generation/replacement;
  - targeted placement operation replacement;
  - targeted route operation replacement.
- `repair plan` / `repair apply --execute` CLI command that reports structured
  plans from stage-issue JSON.
- Opt-in `validation_repair` workflow stage for `designworkflow.CreateOptions`.
- Documentation in `docs/validation-repair.md`.

Known gaps:

- CLI `repair apply` does not yet load a KiCad project directory, mutate files,
  or persist output.
- Repair context must be reconstructed manually by callers; project hydration is
  not automatic.
- The repair runner is not yet wired into `design create` as an actual
  apply/rewrite/revalidate loop.
- Repair success can only be proven when an external caller provides a real
  validator.
- KiCad zone refill remains classified but not executable from the repair CLI.
- Imported project preservation rules are not strong enough to allow broad
  mutation.

## 3. Scope

In scope:

- Hydrate a repair context from a generated KiCadAI project directory.
- Reconstruct or load enough generated transaction state to apply supported
  repairs.
- Persist repaired files through existing writer/transaction paths.
- Rerun writer correctness, board validation, and optional KiCad checks.
- Add real `repair apply --execute --target <project>` behavior.
- Integrate repair apply into `design create` behind explicit options.
- Add golden and fixture coverage for broken-then-repaired generated projects.
- Keep all apply behavior deterministic and bounded by repair budgets.

Out of scope:

- Arbitrary direct text patching of KiCad S-expressions.
- Applying repairs to imported/user-authored projects that contain unsupported
  preserved nodes.
- AI-generated free-form repair edits.
- Full KiCad IPC mutation.
- KiCad zone fill emulation. Zone refill may only be delegated to configured
  KiCad CLI flows in a later follow-up.
- Manufacturing certification.

## 4. Design Principles

- Persisted repair success requires persisted output and revalidation.
- Do not infer ownership of user-authored content.
- Prefer generated transaction replay over direct file surgery.
- Refuse ambiguous repair context.
- Keep dry-run and apply behavior structurally identical except for mutation.
- Preserve existing project structure and output paths.
- Report original validation severity separately from repairability.
- Make every repair attempt traceable to the original issue and resulting file
  or operation change.

## 5. Repair Target Model

Add an explicit project repair target model under `internal/repair` or a small
adjacent package.

```go
type Target struct {
    ProjectRoot string
    ProjectName string
    Generated bool
    Transaction *transactions.Transaction
    Request *designworkflow.Request
    WriteResult *designworkflow.ProjectWriteResult
    WriterIssues []reports.Issue
    ValidationIssues []reports.Issue
    KiCadIssues []reports.Issue
    Artifacts []reports.Artifact
}
```

The target should distinguish:

- generated projects with known transaction/request provenance;
- generated projects where provenance is missing but files can be inspected;
- imported projects;
- mixed projects with preserved unsupported nodes.

Only the first category should be eligible for file mutation in the initial
implementation.

## 6. Provenance Requirements

Repair apply needs enough provenance to safely rewrite files.

Preferred sources:

- original `designworkflow.Request`;
- generated `transactions.Transaction`;
- project write inspection result;
- component selection evidence;
- placement stage result;
- routing stage result;
- writer correctness result;
- validation result.

If the generated transaction is not available, the first implementation should
block mutation and produce a clear issue:

```text
repair apply requires generated transaction provenance for safe persistence
```

Later work can add read-from-files reconstruction, but it must preserve unknown
nodes before enabling mutation.

## 7. Persistence Strategy

Supported initial persistence path:

1. Load the generated transaction/provenance.
2. Build repair plan from current validation issues.
3. Apply supported repair actions to the transaction in memory.
4. Re-apply the repaired transaction through `internal/transactions.Apply`.
5. Rewrite the same output directory only when:
   - `--execute` is present;
   - target is generated/owned;
   - overwrite policy is explicit;
   - no preservation conflicts exist.
6. Rerun validation gates.
7. Return final status.

This avoids direct S-expression mutation and keeps repair apply aligned with the
existing writer behavior.

## 8. CLI Behavior

Extend `repair` command:

```text
kicadai repair plan --target <project> [--request issues.json]
kicadai repair apply --target <project> --execute [--request issues.json]
```

Flags:

- `--target`: project directory, `.kicad_pro`, `.kicad_sch`, or `.kicad_pcb`
  target.
- `--request`: optional captured stage issues or repair bundle JSON.
- `--output`: optional output directory; defaults to target directory for apply
  only when `--execute --overwrite` is present.
- `--execute`: required for mutation.
- `--overwrite`: required when writing into an existing project directory.
- `--max-repair-attempts`: total budget.
- `--kicad-cli`: enable external KiCad checks when configured.
- `--keep-artifacts` / `--artifact-dir`: preserve validation artifacts.

Plan mode:

- read-only;
- reports eligible repairs and blockers;
- may inspect target files;
- must not write output.

Apply mode:

- requires `--execute`;
- requires generated provenance;
- persists repaired output;
- reruns validation;
- exits nonzero on blocked, worsened, or unrepaired results.

## 9. Repair Bundle Format

Add a repair bundle format so `design create`, CLI validation commands, and
agents can hand repair apply a complete context.

```json
{
  "project_root": "./out/demo",
  "project_name": "demo",
  "generated": true,
  "request": {},
  "transaction": {},
  "stage_issues": [
    { "stage": "validation", "issues": [] }
  ],
  "repair_options": {}
}
```

The bundle must be versioned:

```json
{ "schema": "kicadai.repair.bundle.v1" }
```

The loader must reject unknown future major versions.

## 10. Validation Strategy

Applied repair must rerun, at minimum:

- transaction validation after transaction mutation;
- project write result checks after rewrite;
- writer correctness;
- board validation;
- KiCad checks when configured and available.

Success rules:

- `repaired`: all blocking issues in targeted validation gates are gone.
- `partial`: some targeted issues are gone, but non-blocking or unrelated issues
  remain.
- `blocked`: repair could not be safely applied, validation worsened, required
  provenance is missing, or retry budget is exhausted.
- `skipped`: repair disabled or no repairable issues are present.
- `not_needed`: target validates before repair.

The runner must not infer success from operation application alone.

## 11. Workflow Integration

Extend `designworkflow.CreateOptions.Repair` with apply controls:

- enabled;
- apply;
- max attempts;
- keep artifacts;
- KiCad CLI policy;
- output overwrite policy.

When enabled in apply mode, `design create` should:

1. complete generation normally;
2. run writer correctness and validation;
3. if issues exist, build a repair bundle from in-memory request/transaction
   state;
4. apply safe repairs;
5. rewrite the project;
6. rerun validations;
7. append `validation_repair` stage with final result.

Default behavior must remain unchanged when repair is disabled.

## 12. Safety Gates

Repair apply must block when:

- target is not recognized as generated by KiCadAI;
- generated transaction provenance is missing;
- target contains preserved unsupported imported nodes;
- repair action is unsupported or unknown;
- repair would remove user-authored content;
- validation worsens after an attempt;
- the same issue repeats after an attempt;
- KiCad CLI is required but unavailable;
- overwrite is required but not authorized.

## 13. Testing Requirements

Unit tests:

- repair bundle load/save;
- target classification;
- provenance-required blocking;
- transaction mutation and re-apply;
- scoped placement and routing replacement;
- validation success/failure transitions;
- CLI guardrails for `--execute`, `--target`, and `--overwrite`.

Integration tests:

- generated project with missing footprint repair plan;
- generated project with missing outline repair apply;
- generated project with incorrect pad-net hints repaired and revalidated;
- route repair that remains blocked when routing evidence is absent;
- workflow repair stage that reports repaired only after validation passes.

Golden tests:

- plan-only report;
- missing provenance blocked report;
- successful persisted repair report;
- validation-worsened blocked report;
- no-repair-needed report.

External KiCad tests:

- optional and gated by environment variables;
- never required for default unit test pass.

## 14. Deliverables

- repair bundle model and loader;
- project repair target hydrator;
- persisted apply runner;
- extended repair CLI;
- workflow apply integration;
- fixture projects and golden reports;
- docs and README updates;
- roadmap gap update.

## 15. Acceptance Criteria

- `repair plan --target <generated-project>` is read-only and returns structured
  repair attempts.
- `repair apply --target <generated-project> --execute --overwrite` can persist
  at least one safe repair and rerun validation.
- `design create` can optionally run repair apply and only report `repaired`
  after revalidation passes.
- Missing provenance blocks with actionable diagnostics.
- Imported/preserved content is not mutated.
- Full `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` passes.
- Prism review has no unresolved high-severity findings.
