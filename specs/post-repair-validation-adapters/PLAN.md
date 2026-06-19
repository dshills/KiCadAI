# Post-Repair Validation Adapters Implementation Plan

## Phase 1: Adapter Option And Result Model

### Goal

Add the typed configuration and evidence result structures needed by all
post-repair validation adapters.

### Work

- Add `PostValidationOptions`.
- Add validation summary and delta models.
- Add stable issue-key helpers for before/after comparison.
- Add deterministic sorting for deltas and adapter summaries.
- Keep existing `PostApplyValidation` shape backward-compatible.

### Tests

- Issue keys are stable and include code, severity, path, refs, nets, message,
  and operation ID.
- Delta classification reports cleared, repeated, new, and worsened issues.
- Empty validation evidence produces a clean summary.
- Sorting is deterministic.

### Acceptance

- Persisted repair can represent validation evidence without changing current
  behavior.

### Commit

```text
Add post-repair validation evidence model
```

## Phase 2: Built-In Writer Correctness Adapter

### Goal

Run writer correctness automatically after persisted repair apply.

### Work

- Implement a writer correctness `PostApplyValidator`.
- Resolve the generated project target from output directory.
- Run existing writer correctness checks.
- Map results and artifacts into `repair.PostApplyValidation`.
- Add options for optional round-trip where supported.

### Tests

- Adapter passes on a valid generated project fixture.
- Adapter reports blocking issues on malformed output.
- Adapter skips or reports a clear issue when project target is missing.
- Round-trip option is off by default.

### Acceptance

- Persisted repair apply can use writer correctness without caller-injected
  validators.

### Commit

```text
Add repair writer correctness adapter
```

## Phase 3: Built-In Board Validation Adapter

### Goal

Run connectivity-first board validation after persisted repair apply.

### Work

- Implement a board validation `PostApplyValidator`.
- Locate `.kicad_pcb` from output directory or project metadata.
- Run board validation with strict-zone option support.
- Skip schematic-only outputs with explicit non-blocking evidence.
- Preserve artifacts and structured issues.

### Tests

- Adapter passes on a valid generated PCB fixture.
- Adapter reports missing outline or disconnected pads.
- Adapter skips schematic-only project with a clear skipped validation result.
- Strict-zone option changes zone severity as expected.

### Acceptance

- Persisted PCB repair apply runs board validation by default when a PCB exists.

### Commit

```text
Add repair board validation adapter
```

## Phase 4: Optional KiCad ERC, DRC, And Round-Trip Adapters

### Goal

Wire external KiCad-backed validation into repair apply policy without making
default tests depend on KiCad CLI.

### Work

- Implement KiCad ERC adapter.
- Implement KiCad DRC adapter.
- Implement or wrap optional round-trip validation adapter.
- Add options for required vs optional external checks.
- Preserve report artifacts when requested.
- Return explicit skipped evidence when KiCad CLI is unavailable.

### Tests

- Optional missing KiCad CLI produces skipped validation and non-blocking issue.
- Required missing KiCad CLI produces blocking issue.
- Fake runner returning ERC/DRC findings maps to structured issues.
- Artifact paths are preserved when keep-artifacts is enabled.

### Acceptance

- Repair apply can run or skip KiCad-backed validators according to explicit
  policy.

### Commit

```text
Add optional KiCad repair validators
```

## Phase 5: Persisted Apply Status From Validation Deltas

### Goal

Make persisted repair status depend on validation deltas, not only repair
executor status.

### Work

- Compute before/after validation summaries.
- Compare source stage issues to post-apply validation issues.
- Block on repeated blocking issue keys.
- Block on new blocking issues.
- Block on worsened issue count or severity.
- Report `partial` only when blocking issues are gone and warnings/skips remain.
- Report `repaired` only when required validation is clean.

### Tests

- Clean post-validation returns `repaired`.
- Repeated blocking issue returns `blocked`.
- New blocking issue returns `blocked`.
- More warnings can return `partial` only when no blockers remain.
- Required skipped validator returns `blocked`.

### Acceptance

- Persisted repair cannot overstate success.

### Commit

```text
Gate repair status on validation deltas
```

## Phase 6: Retry Budget Integration

### Goal

Bound repair and validation loops across persisted apply.

### Work

- Ensure total attempt and per-issue budgets are honored during persisted apply.
- Add cycle budget metadata to result summaries.
- Record budget exhaustion as blocked with latest validation evidence.
- Ensure workflow and CLI options map to the same budget fields.

### Tests

- Total budget exhaustion blocks.
- Per-issue budget exhaustion blocks repeated issues.
- Budget summary is deterministic.
- Zero/invalid budget normalizes to safe defaults.

### Acceptance

- Repair apply always terminates with actionable budget evidence.

### Commit

```text
Enforce persisted repair budgets
```

## Phase 7: Repair Bundle Export From Design Workflow

### Goal

Make target-based repair apply reproducible from `design create` output.

### Work

- Add design workflow option or CLI flag to persist repair bundle artifacts.
- Include normalized request, generated transaction, stage issues, repair
  options, project root/name, and ownership metadata.
- Add artifact entry to workflow result.
- Keep default behavior unchanged when bundle export is not requested.

### Tests

- `design create` can emit a valid bundle artifact.
- Bundle can be parsed by `repair.ParseBundle`.
- Bundle includes writer, validation, and KiCad stage issues.
- Disabled bundle export leaves existing output unchanged.

### Acceptance

- A generated design run can hand a bundle to `repair apply --target`.

### Commit

```text
Export design repair bundles
```

## Phase 8: CLI Target Apply Uses Built-In Validators

### Goal

Make `repair apply --target` useful without caller-supplied post validators.

### Work

- Wire built-in adapter factory into repair CLI target apply.
- Add flags for writer, board, ERC, DRC, round-trip, strict zones, artifacts,
  and KiCad CLI policy.
- Ensure `repair plan --target` remains read-only.
- Ensure `repair apply --target` requires `--execute`.
- Ensure existing output mutation still requires `--overwrite`.
- Return nonzero on blocked target apply.

### Tests

- CLI target apply runs writer and board validators by default.
- CLI plan does not write files.
- Missing provenance blocks mutation.
- Imported target remains blocked.
- Required DRC without KiCad CLI returns blocked.

### Acceptance

- Agents can call `repair apply --target ... --request bundle.json --execute
  --overwrite` and receive final validation evidence.

### Commit

```text
Use built-in validators for repair CLI apply
```

## Phase 9: Design Workflow Persisted Repair Evidence

### Goal

Expose validation deltas and artifacts in the `design create` repair stage.

### Work

- Attach adapter results to `validation_repair` stage summary.
- Attach before/after validation summaries.
- Attach issue delta counts.
- Include latest validation artifacts.
- Keep stage status conservative and aligned with persisted repair status.

### Tests

- Workflow repair stage reports repaired only with clean required validation.
- Workflow repair stage reports blocked on repeated issue.
- Workflow repair stage includes validation summaries and artifacts.
- Repair disabled path remains unchanged.

### Acceptance

- `design create` output explains exactly what repair changed and what
  validation proved afterward.

### Commit

```text
Expose persisted repair validation evidence
```

## Phase 10: Documentation And Roadmap Update

### Goal

Document the new repair validation behavior and move the roadmap forward.

### Work

- Update README repair section.
- Update repair docs with target apply examples and validator flags.
- Update `specs/ROADMAP.md` Priority 3 status.
- Document KiCad CLI skip/require behavior.
- Document status semantics for `repaired`, `partial`, `blocked`, and
  `not_needed`.

### Tests

- Docs-only phase does not require Go tests unless examples are generated.
- Run focused CLI help tests if command help changes.

### Acceptance

- Users can reproduce repair apply with built-in validation from docs.

### Commit

```text
Document post-repair validation adapters
```

## Final Verification

Before the final phase is considered complete:

- run focused tests after each phase;
- run `prism review staged` before each phase commit;
- run full `go test ./...` with a workspace-local `GOCACHE`;
- confirm `repair apply --target` result semantics match the spec;
- confirm `specs/ROADMAP.md` names the next priority after Priority 3.
