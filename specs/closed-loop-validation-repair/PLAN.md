# Closed-Loop Validation Repair Implementation Plan

## 1. Objective

Implement the Closed-Loop Validation Repair system described in `SPEC.md`.

The end state is a deterministic repair layer that can classify validation
failures, propose or apply safe repairs, rerun validation, and report whether a
generated KiCad project was repaired, partially repaired, skipped, or blocked.

Implement this in small phases with Prism review and a commit between phases.

## 2. Implementation Rules

- Reuse existing packages:
  - `internal/reports`
  - `internal/designworkflow`
  - `internal/writercorrectness`
  - `internal/boardvalidation`
  - `internal/kicadfiles/checks`
  - `internal/placement`
  - `internal/routing`
  - `internal/transactions`
- Do not introduce a second validation framework.
- Do not mutate project files by default.
- Keep deterministic tests independent of KiCad CLI.
- Repairs must never claim success without revalidation.
- Prefer transaction-level repairs over direct file edits.
- Run `gofmt` on edited Go files.
- Run focused tests after each phase.
- Run `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` before the final
  phase commit.
- Run `prism review staged` before every phase commit.

## 3. Phase 1: Repair Model And Classifier

### Goal

Add the repair package, result model, policy model, and issue classifier.

### Work

- Add `internal/repair`.
- Define:
  - `Status`;
  - `Category`;
  - `Action`;
  - `Options`;
  - `Result`;
  - `Attempt`;
  - `Summary`;
  - `Plan`.
- Implement classifier from `reports.Issue` to repair categories.
- Classify using:
  - `reports.Code`;
  - issue path;
  - issue message;
  - refs/nets;
  - stage name when available.
- Add helper to build a repair plan from grouped stage issues.

### Tests

- Missing footprint issue maps to `missing_footprint`.
- Disconnected pad maps to `disconnected_pad`.
- Invalid net assignment maps to `invalid_net_assignment`.
- Missing board outline maps to `missing_board_outline`.
- Placement collision maps to `placement_collision`.
- KiCad CLI unavailable maps to `kicad_cli_unavailable`.
- Unknown issue maps to `unknown`.
- Unsupported/unsafe categories are not marked repairable.

### Acceptance Criteria

- Repair classification compiles independently.
- No existing workflow behavior changes.

### Commit Message

```text
Add repair model and issue classifier
```

## 4. Phase 2: Repair Planning Engine

### Goal

Turn classified issues into deterministic repair plans without applying changes.

### Work

- Add `Planner`.
- Add policy defaults:
  - disabled unless explicitly called;
  - dry-run by default;
  - max 3 total attempts;
  - max 1 attempt per source issue.
- Implement action selection:
  - missing footprint -> assign footprint action;
  - invalid/disconnected pad -> regenerate pad net hints action;
  - unrouted/clearance -> reroute action;
  - placement collision/outside board -> replace placement action;
  - missing outline -> generate outline action;
  - zone unfilled -> KiCad refill required action;
  - unsupported/unknown -> blocked action.
- Include issue context and reason for every skipped or blocked repair.

### Tests

- Plan chooses expected actions for known categories.
- Plan respects `MaxAttempts`.
- Plan respects `MaxAttemptsPerIssue`.
- Dry-run plan does not produce file mutations.
- Unsupported issues produce blocked attempts with suggestions.

### Acceptance Criteria

- CLI/workflow can ask "what would you repair?" without changing files.

### Commit Message

```text
Plan validation repairs
```

## 5. Phase 3: Footprint And Net-Hint Repairs

### Goal

Implement the first safe transaction-level repairs.

### Work

- Add repair executors for generated transaction outputs.
- Implement missing footprint repair:
  - use existing component/resolver evidence when available;
  - emit `assign_footprint` operations;
  - block if evidence is missing at required acceptance.
- Implement pad net hint repair:
  - derive pad nets from schematic-to-PCB transfer or generated connect ops;
  - update generated `place_footprint` payloads where safe;
  - preserve existing placement coordinates.
- Add revalidation hook interface.

### Tests

- Missing footprint dry-run shows assignment operation.
- Missing footprint apply adds assignment operation in memory.
- Missing footprint blocks when no verified footprint exists.
- Pad net hint repair updates generated footprint pads.
- Net repair refuses unknown/user-authored targets.

### Acceptance Criteria

- At least two repair actions can produce concrete transaction changes.

### Commit Message

```text
Repair footprints and pad net hints
```

## 6. Phase 4: Outline, Placement, And Routing Repairs

### Goal

Add repair executors for the most common board-shape and layout failures.

### Work

- Implement missing board outline repair for generated projects:
  - use request board dimensions;
  - add `set_board_outline` transaction operation;
  - block if board dimensions are unavailable.
- Implement placement retry action:
  - invoke placement engine with adjusted policy only when allowed;
  - preserve fixed components.
- Implement routing retry action:
  - invoke routing engine for affected nets;
  - support bounded policy escalation such as two-layer mode when allowed.

### Tests

- Missing outline repair emits expected outline operation.
- Missing outline blocks without board dimensions.
- Placement repair is skipped unless policy allows it.
- Routing repair reruns only when placement exists.
- Routing repair reports blocked when route remains impossible.

### Acceptance Criteria

- Repair engine can address outline, placement, and routing classes without
  direct file editing.

### Commit Message

```text
Repair outline placement and routing issues
```

## 7. Phase 5: Repair Loop And Revalidation

### Goal

Apply planned repairs in bounded attempts and rerun validators.

### Work

- Add `Runner`.
- Execute attempts in deterministic order.
- Rerun minimum affected validation:
  - writer correctness after transaction/file repair;
  - board validation after PCB/net/route/zone repair;
  - placement/routing validation after layout repair;
  - KiCad checks only when configured.
- Compare before/after issue sets.
- Stop if:
  - validation passes;
  - issue count worsens;
  - same issue repeats;
  - retry budget is exhausted;
  - unsafe repair is required.
- Record every attempt.

### Tests

- Not-needed result when initial validation passes.
- Repaired result after fake validator clears issue.
- Blocked result after validator keeps same issue.
- Blocked result when issue count worsens.
- Retry budget is honored.
- Final issues are the latest validation issues.

### Acceptance Criteria

- The repair loop cannot report success without a passing revalidation result.

### Commit Message

```text
Run bounded validation repair loop
```

## 8. Phase 6: Design Workflow Integration

### Goal

Expose repair as an optional `design create` stage.

### Work

- Add repair options to `designworkflow.CreateOptions`.
- Add `ValidationRepair` stage after validation/KiCad checks.
- Collect prior blocking issues from workflow stages.
- Run repair loop only when enabled.
- Add repair summary to workflow result.
- Update feedback suggestions when repair succeeds or blocks.
- Keep existing behavior unchanged when repair is disabled.

### Tests

- Workflow omits repair stage by default.
- Workflow includes skipped repair stage when enabled but no issues exist.
- Workflow includes repaired stage when fake repair succeeds.
- Workflow remains blocked when repair fails.
- Fabrication acceptance is not upgraded without revalidation evidence.

### Acceptance Criteria

- AI design workflow can opt into closed-loop repair without destabilizing the
  default path.

### Commit Message

```text
Integrate repair loop into design workflow
```

## 9. Phase 7: Repair CLI

### Goal

Expose repair planning and application to agents and humans.

### Work

- Add `repair` command:
  - `repair plan --target <project>`;
  - `repair apply --target <project> --execute`.
- Support flags:
  - `--request`;
  - `--output`;
  - `--execute`;
  - `--max-repair-attempts`;
  - `--allow-placement-retry`;
  - `--allow-routing-retry`;
  - `--allow-outline-generation`;
  - `--kicad-cli`;
  - `--keep-artifacts`;
  - `--artifact-dir`.
- Return `reports.Result` JSON.
- Exit nonzero on blocked repair.
- Keep plan mode read-only.

### Tests

- `repair plan` returns proposed attempts.
- `repair apply` requires `--execute`.
- Missing target returns structured issue.
- Blocked repair exits nonzero.
- JSON output includes attempts and final status.

### Acceptance Criteria

- Agents can run repair dry-runs and applied repairs from the CLI.

### Commit Message

```text
Expose validation repair CLI
```

## 10. Phase 8: Golden Reports And Examples

### Goal

Lock stable repair report behavior and provide examples.

### Work

- Add golden JSON snapshots for:
  - no repair needed;
  - planned missing footprint repair;
  - successful fake repair;
  - blocked unsupported repair;
  - retry budget exhausted.
- Add examples under `examples/repair/`.
- Add update flag for repair goldens.
- Normalize volatile paths and timestamps.

### Tests

- Golden reports match.
- Update flag refreshes goldens.
- Examples validate with deterministic tests.

### Acceptance Criteria

- Repair output contract is stable enough for AI agents.

### Commit Message

```text
Add validation repair golden reports
```

## 11. Phase 9: Documentation And Roadmap Update

### Goal

Document repair behavior, safety limits, and workflow usage.

### Work

- Add `docs/validation-repair.md`.
- Update README:
  - CLI examples;
  - design workflow repair flag;
  - repair statuses;
  - safety limits.
- Update `specs/ROADMAP_GAP.md`:
  - mark Closed-Loop Validation Repair implemented or in progress;
  - identify next recommended gap.

### Tests

- Documentation examples use real command names and flags.
- Full `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` passes.

### Acceptance Criteria

- A developer or AI agent can understand when repair is safe, when it is
  skipped, and how to inspect attempts.

### Commit Message

```text
Document validation repair loop
```

## 12. Final Completion Checklist

- Classifier covers current report issue codes.
- Repair plans are deterministic.
- Dry-run mode is default for CLI repair.
- At least three concrete safe repair actions exist.
- Revalidation is mandatory before reporting repair success.
- Design workflow integration is opt-in.
- Repair CLI is available.
- Golden report tests pass.
- Documentation explains safety limits.
- `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` passes.
- Prism has no unresolved high-severity findings.
