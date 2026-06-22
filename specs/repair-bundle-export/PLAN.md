# Repair Bundle Export Command Implementation Plan

## Objective

Add a dedicated `repair export-bundle` CLI flow that turns generated-project
targets plus structured stage issues into parseable repair bundles consumable by
`repair apply --target`.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests independent of real KiCad CLI, network access, and global
  KiCad library roots.
- Reuse existing repair bundle, target hydration, stage issue loading, and
  report envelope code before adding new models.
- Do not loosen imported-project mutation safety.
- Do not mutate project, schematic, PCB, or library files during export.
- Normalize temp paths in tests and assert selected stable fields instead of
  full JSON snapshots.

## Phase 1: Export Model And Service

### Goal

Create a small internal export service that can build a repair bundle and
structured export result without CLI concerns.

### Work

- Add an internal repair export model, likely in `internal/repair`, for:
  - export options;
  - export result;
  - summary fields;
  - bundle artifact metadata.
- Implement an export function that accepts:
  - target path;
  - stage issue groups;
  - output path;
  - execute flag;
  - overwrite flag;
  - repair options.
- Reuse `repair.HydrateTarget` to inspect target ownership.
- Reuse `repair.SaveBundle` for write execution.
- Default output path to `<target-root>/.kicadai/repair-bundle.json`.
- Compute summary fields:
  - stage count;
  - issue count;
  - blocking count;
  - generated;
  - has transaction;
  - dry run.
- Return a `reports.Artifact` when the bundle is or would be written.

### Tests

- Generated target dry-run returns default path and writes no file.
- Execute writes a parseable bundle.
- Summary counts match supplied stage issues.
- Existing bundle blocks without overwrite.
- Overwrite replaces an existing bundle.

### Acceptance

- Bundle export behavior is testable without invoking the CLI.

### Commit

```text
Add repair bundle export service
```

## Phase 2: Target Safety And Path Policy

### Goal

Enforce generated-target-only export and safe output paths.

### Work

- Block missing targets with structured issues.
- Block imported or non-generated targets.
- Block targets where generated ownership cannot be established.
- Ensure output path stays inside target root.
- Ensure parent directories are created only under target root.
- Reject empty stage issue groups.
- Reject malformed stage issue data at service boundary if applicable.

### Tests

- Missing target blocks.
- Imported target blocks.
- Output path outside target root blocks.
- Empty stage issues block.
- Parent `.kicadai` directory creation works for safe paths.

### Acceptance

- Export cannot create repair bundles for unsafe targets or outside the target
  root.

### Commit

```text
Harden repair bundle export safety
```

## Phase 3: CLI Command

### Goal

Expose `repair export-bundle` through `cmd/kicadai`.

### Work

- Add `repair export-bundle` subcommand handling.
- Require `--json`.
- Require `--target`.
- Require `--request`.
- Use existing `loadRepairStageIssues` for stage issue input.
- Map CLI flags into repair export options:
  - `--output`;
  - `--execute`;
  - `--overwrite`;
  - `--max-repair-attempts`;
  - `--seed` if useful in the export model.
- Return standard `reports.Result` JSON.
- Keep all flags before subcommand words because the current CLI uses a single
  global flag set.
- Update usage text to list `repair export-bundle`.

### Tests

- CLI dry-run returns bundle path and does not write.
- CLI execute writes parseable bundle.
- CLI missing request returns JSON issue.
- CLI missing target returns JSON issue.
- CLI malformed request returns JSON issue.

### Acceptance

- Users can run `kicadai --json --target ... --request ... repair
  export-bundle` and receive a stable structured result.

### Commit

```text
Add repair export-bundle CLI
```

## Phase 4: Apply From Export Golden

### Goal

Prove exported bundles are immediately consumable by `repair apply --target`.

### Work

- Add CLI golden fixture for:
  - generated target;
  - stage issue JSON;
  - export command;
  - apply command using the exported bundle.
- Reuse existing post-repair CLI helpers where possible.
- Assert exported bundle parseability.
- Assert `repair apply --target` reads the exported bundle and reaches the
  expected status.
- Assert overwrite behavior remains enforced by apply.

### Tests

- Exported bundle parses with `repair.LoadBundle`.
- Apply from exported bundle returns structured JSON.
- Apply without overwrite blocks on existing generated target.
- Apply with overwrite follows existing generated-project safety rules.

### Acceptance

- The core handoff chain is covered end to end:
  `stage issues -> export bundle -> apply target`.

### Commit

```text
Add apply-from-export repair golden
```

## Phase 5: Transaction Evidence Hook

### Goal

Prepare export for transaction-aware bundles without overbuilding the first CLI
surface.

### Work

- Inspect existing generated manifests and transaction persistence paths.
- If a generated transaction is already discoverable from target metadata,
  hydrate it into the bundle.
- If not discoverable, keep `transaction` absent and make
  `summary.has_transaction` false.
- Add internal seam for future explicit `--transaction` support.
- Ensure exported bundles without transactions remain valid but clearly
  signaled.

### Tests

- Export summary reports `has_transaction=false` when no transaction evidence
  exists.
- If discoverable transaction evidence exists, export includes it and
  `has_transaction=true`.
- Invalid transaction evidence blocks bundle export.

### Acceptance

- AI callers can tell whether an exported bundle is likely sufficient for
  mutation or only useful as repair evidence.

### Commit

```text
Add repair export transaction evidence hook
```

## Phase 6: CLI Goldens And Regression Coverage

### Goal

Lock down the stable external JSON contract for the export command.

### Work

- Add selected-field CLI assertions for:
  - `data.target`;
  - `data.bundle_path`;
  - `data.dry_run`;
  - `data.summary.stage_count`;
  - `data.summary.issue_count`;
  - `data.summary.blocking_count`;
  - `data.summary.generated`;
  - `data.summary.has_transaction`;
  - `artifacts`.
- Add path normalization helpers if existing post-repair helpers are not
  reusable.
- Add tests for explicit output path and default output path.
- Add tests for overwrite gate and outside-root path rejection.

### Tests

- `go test ./cmd/kicadai ./internal/repair`
- The new command tests must pass without KiCad installed.

### Acceptance

- Future CLI changes cannot silently break the bundle export contract.

### Commit

```text
Add repair export CLI goldens
```

## Phase 7: Documentation And Roadmap

### Goal

Document the command and move the roadmap to the next priority.

### Work

- Update README with:
  - export command usage;
  - dry-run and execute behavior;
  - generated-target-only safety policy;
  - default bundle path;
  - apply-from-export sequence.
- Update `specs/ROADMAP.md`:
  - mark dedicated repair bundle export as implemented;
  - make the next recommended item generated full-board retry fixtures unless
    implementation findings change that order.
- Add notes to this plan if transaction hydration remains a known gap.

### Tests

- `go test ./cmd/kicadai ./internal/repair`
- `go test ./...`

### Acceptance

- User-facing docs match implemented behavior and the roadmap points to the
  next project.

### Commit

```text
Document repair bundle export
```

## Implementation Notes

- `repair export-bundle` is implemented as a generated-target-only evidence
  export path. It defaults to dry-run behavior and writes only when `--execute`
  is provided.
- Output paths are constrained to the generated target root. Existing bundle
  files require `--overwrite`.
- The internal export service accepts optional transaction evidence and reports
  `summary.has_transaction=true` when a transaction is present.
- Current CLI exports from stage issue JSON do not yet discover full generated
  transaction provenance from target metadata, so CLI-generated bundles can
  report `summary.has_transaction=false`. Those bundles remain useful for
  reproducible evidence and diagnostics. Mutation through `repair apply
  --target` still requires transaction provenance and blocks safely when the
  transaction is absent.
- The next follow-up is to persist or hydrate transaction provenance for
  generated targets outside the `design create --repair-apply` path.
