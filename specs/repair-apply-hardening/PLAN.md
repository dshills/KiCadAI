# Repair Apply Hardening Implementation Plan

## 1. Objective

Implement `SPEC.md` in phases. The final result should make repair apply capable
of safely mutating generated KiCadAI projects, persisting output, rerunning
validation, and reporting repaired/partial/blocked status with evidence.

Use Prism review and a commit between phases.

## 2. Implementation Rules

- Do not mutate project files unless `--execute` and the required overwrite
  policy are present.
- Do not directly patch KiCad files when transaction replay can be used.
- Do not allow imported/user-authored projects to be mutated until preservation
  support is explicit.
- Preserve existing default `design create` behavior when repair is disabled.
- Keep tests deterministic and independent of KiCad CLI by default.
- Run `gofmt` on edited Go files.
- Run focused tests after each phase.
- Run `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` before the final
  phase commit.
- Run `prism review staged` before every phase commit.

## 3. Phase 1: Repair Bundle Model

### Goal

Create a versioned repair bundle that can carry generated project provenance,
stage issues, transaction data, and repair options.

### Work

- Add bundle model in `internal/repair`.
- Define schema name `kicadai.repair.bundle.v1`.
- Include:
  - project root;
  - project name;
  - generated flag;
  - optional design request;
  - transaction;
  - stage issues;
  - repair options.
- Add load/save helpers.
- Reject missing schema and unsupported schema versions.
- Normalize project paths to slash form in JSON output.

### Tests

- Load valid bundle.
- Save and reload bundle.
- Reject unsupported schema.
- Reject malformed transaction payload.
- Preserve stage issue refs/nets/severity.

### Acceptance Criteria

- CLI and workflow code can pass one typed bundle to repair apply.

### Commit Message

```text
Add repair bundle model
```

## 4. Phase 2: Target Hydration And Ownership Classification

### Goal

Classify repair targets and determine whether mutation is allowed.

### Work

- Add target hydrator for:
  - project directory;
  - `.kicad_pro`;
  - `.kicad_sch`;
  - `.kicad_pcb`.
- Detect generated KiCadAI projects using available project metadata and
  generated structure.
- Load optional repair bundle from `--request`.
- Identify imported/preserved-content blockers using existing inspection and
  writer-correctness signals.
- Return structured issues for missing target, unsupported target, missing
  provenance, and unsafe imported content.

### Tests

- Generated target with bundle is mutable.
- Missing bundle blocks mutation.
- Imported/preserved target blocks mutation.
- Schematic/PCB direct targets are inspectable but not initially mutable.
- Missing target returns structured issue.

### Acceptance Criteria

- `repair plan --target` can inspect ownership and report why apply is or is
  not allowed.

### Commit Message

```text
Hydrate repair targets
```

## 5. Phase 3: Persisted Transaction Apply Runner

### Goal

Apply safe transaction-level repairs, replay the transaction writer, and persist
the result to disk.

### Work

- Add persisted runner around existing `repair.Runner`.
- Convert bundle provenance into `repair.ExecutionContext`.
- Apply transaction mutations through existing repair executors.
- Replay repaired transaction with `transactions.Apply`.
- Require `--overwrite` when writing into an existing directory.
- Capture artifacts and write result.
- Keep plan mode read-only.

### Tests

- Missing outline repair rewrites generated project through transaction apply.
- Missing footprint repair updates transaction before replay.
- Apply without overwrite blocks.
- Apply without execute blocks at CLI layer.
- Transaction validation failure blocks before file write.

### Acceptance Criteria

- At least one safe repair can be persisted for a generated project fixture.

### Commit Message

```text
Persist generated repair apply
```

## 6. Phase 4: Post-Repair Validation Orchestration

### Goal

Make persisted repair success depend on real validation gates.

### Work

- Add validator adapter that reruns:
  - transaction validation;
  - writer correctness;
  - board validation;
  - optional KiCad checks.
- Compare before/after issues.
- Stop on:
  - pass;
  - worsened issue count;
  - same blocking issue repeated;
  - budget exhaustion.
- Preserve artifacts from validation runs.
- Report final issues from the latest validation run.

### Tests

- Repaired only when validators return no blocking issues.
- Partial when blocking issue clears but warning remains.
- Blocked when issue repeats.
- Blocked when issue count worsens.
- KiCad checks skipped cleanly when not configured.

### Acceptance Criteria

- Persisted repair cannot report success without post-write validation evidence.

### Commit Message

```text
Validate persisted repairs
```

## 7. Phase 5: Repair CLI Apply Target Support

### Goal

Extend `repair plan/apply` to operate on project targets, not only stage-issue
JSON.

### Work

- Add `--target` flag or parse target from command args consistently with
  existing CLI style.
- Support:
  - `repair plan --target <project>`;
  - `repair apply --target <project> --request bundle.json --execute
    --overwrite`.
- Return `reports.Result` JSON.
- Exit nonzero on blocked/failed apply.
- Include clear messages for:
  - missing target;
  - missing provenance;
  - imported project;
  - overwrite required;
  - validation failed after apply.

### Tests

- `repair plan --target` is read-only.
- `repair apply --target` requires `--execute`.
- Existing target requires `--overwrite`.
- Missing provenance returns blocked result.
- Successful fixture apply returns repaired result.

### Acceptance Criteria

- Agents can call repair apply on generated project output and get structured
  persisted repair evidence.

### Commit Message

```text
Apply repairs from CLI targets
```

## 8. Phase 6: Design Workflow Apply Integration

### Goal

Wire persisted repair apply into `design create` as an opt-in post-validation
loop.

### Work

- Extend `designworkflow.CreateOptions.Repair` or add wrapper options for:
  - apply;
  - overwrite;
  - max attempts;
  - KiCad CLI checks;
  - artifact handling.
- Build repair bundle from in-memory workflow state.
- If validation fails and repair apply is enabled:
  - apply repair;
  - rewrite project;
  - rerun validations;
  - append final `validation_repair` stage.
- Keep current plan-only repair stage for non-apply mode.
- Avoid upgrading acceptance unless validation evidence supports it.

### Tests

- Default workflow omits apply behavior.
- Plan mode adds warning/pending repair stage.
- Apply mode can repair a generated fixture.
- Apply mode remains blocked when repair validation fails.
- Fabrication readiness is not upgraded without KiCad evidence.

### Acceptance Criteria

- `design create` can optionally repair its own generated output in one run.

### Commit Message

```text
Apply repair in design workflow
```

## 9. Phase 7: Golden Fixtures And Regression Corpus

### Goal

Lock persisted repair behavior with fixtures and golden reports.

### Work

- Add generated broken fixtures:
  - missing outline;
  - bad pad-net hint;
  - missing footprint with verified evidence;
  - route repair blocked without route evidence;
  - missing provenance blocked.
- Add golden reports for:
  - plan-only;
  - apply repaired;
  - apply partial;
  - apply blocked;
  - no repair needed.
- Normalize paths and timestamps.
- Keep external KiCad checks optional.

### Tests

- Golden reports compare semantically as JSON.
- Fixture project files are valid after repair apply.
- Update helper exists if project convention supports it.

### Acceptance Criteria

- Repair apply behavior is stable enough for AI agents to depend on.

### Commit Message

```text
Add persisted repair fixtures
```

## 10. Phase 8: Documentation And Roadmap Update

### Goal

Document the persisted repair workflow and update roadmap status.

### Work

- Update `docs/validation-repair.md` with:
  - target apply examples;
  - bundle format;
  - safety gates;
  - persisted vs in-memory status semantics.
- Update README CLI overview.
- Update `specs/ROADMAP_GAP.md` with implemented status and remaining
  hardening.
- Include examples for agent usage.

### Tests

- Full `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` passes.
- Documentation examples use real command names and flags.

### Acceptance Criteria

- A developer or AI agent can understand how to run repair plan/apply safely.

### Commit Message

```text
Document persisted repair apply
```

## 11. Final Checklist

- Repair bundle exists and is versioned.
- Target hydration distinguishes generated, imported, and unsafe projects.
- Plan mode is read-only.
- Apply mode requires `--execute`.
- Existing output overwrite requires explicit permission.
- Safe repairs persist through transaction replay.
- Validation gates rerun after persisted apply.
- `design create` apply integration is opt-in.
- Golden fixtures cover repaired, partial, blocked, and no-op results.
- Full test suite passes.
- Prism has no unresolved high-severity findings.
