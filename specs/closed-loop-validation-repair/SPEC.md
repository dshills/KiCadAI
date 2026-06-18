# Closed-Loop Validation Repair Specification

## 1. Objective

Build a deterministic repair loop that can consume validation failures from the
existing KiCadAI generation pipeline, propose or apply bounded repairs, rerun
the affected validation stages, and return a clear final status.

The goal is to move from "detect generated-design failures" to "repair common
generated-design failures automatically or explain why the design is blocked."

## 2. Background

KiCadAI now has several strong validation gates:

- writer correctness checks for project structure, schematic connectivity,
  schematic-to-PCB transfer, PCB pad nets, copper nets, zones, and optional
  round-trip evidence;
- board validation checks for net-to-pad correctness, route completion,
  unrouted nets, zones, outlines, and optional KiCad DRC;
- KiCad ERC/DRC feedback parsing and allowlisting;
- placement diagnostics;
- routing diagnostics;
- circuit block verification and block evidence in the design workflow.

These systems currently produce issues and repair suggestions, but the design
workflow does not yet own a retry loop that can apply repairs and revalidate.

## 3. Scope

This project adds a repair orchestration layer.

In scope:

- classify existing `reports.Issue` values into repair categories;
- map repair categories to deterministic repair strategies;
- apply safe transaction-level repairs where the existing data model supports
  them;
- rerun the minimum affected validation stage after each repair attempt;
- track repair attempts, applied changes, skipped repairs, and final status;
- integrate with `design create` as an optional post-validation repair loop;
- expose a CLI command for repair dry-runs and applied repairs;
- keep every repair bounded by retry budgets and explicit safety checks.

Out of scope:

- arbitrary AI-generated patching of KiCad files;
- replacing placement or routing engines;
- full KiCad interactive editing;
- manufacturing certification;
- hidden mutation of user-authored KiCad projects without `--execute` or an
  equivalent explicit apply flag.

## 4. Design Principles

- Deterministic first: the same inputs, seed, and repair policy should produce
  the same attempts.
- Safe by default: dry-run is the default CLI behavior unless the command is
  explicitly asked to write.
- Preserve context: every repair attempt keeps the original issue, target stage,
  operation IDs where available, changed transaction operations, and
  revalidation result.
- Prefer narrow repairs: rerun the smallest affected stage instead of the whole
  workflow when possible.
- Stop early on unsafe repairs: if a repair would remove user-authored content,
  rewrite unknown objects, or exceed budget, return blocked.
- Existing validators remain authoritative: repair success is defined by
  revalidation, not by the repair action claiming success.

## 5. Inputs

The repair layer must accept:

- a design workflow request and prior `designworkflow.WorkflowResult`;
- generated project output directory;
- transaction plans or generated operations when available;
- writer correctness results;
- board validation results;
- KiCad check results;
- routing and placement results;
- repair policy options.

The first implementation should primarily target generated projects where
KiCadAI owns the transaction plan. Imported/user-authored project repair can be
added later with stricter preservation rules.

## 6. Output Model

Add a package such as `internal/repair` with a result model:

```go
type Result struct {
    Status      Status
    Attempts    []Attempt
    FinalIssues []reports.Issue
    Artifacts   []reports.Artifact
    Summary     Summary
}
```

Expected statuses:

- `not_needed`: validation already passes;
- `repaired`: all targeted blocking issues were repaired and revalidated;
- `partial`: some issues were repaired, but non-blocking issues remain;
- `blocked`: repair is unsafe, unsupported, or exhausted retry budget;
- `skipped`: repair disabled or no repairable issue exists.

Each attempt should include:

- attempt number;
- source stage;
- source issue code and path;
- classifier category;
- chosen action;
- dry-run or applied mode;
- transaction operations or file paths affected;
- validation command/stage rerun;
- before/after issue counts;
- result status;
- blocking reason if any.

## 7. Issue Classification

Classify issues by `reports.Code`, stage, message, refs, nets, operation ID,
and known suggestions.

Initial categories:

- `missing_footprint`
- `unknown_symbol`
- `invalid_net_assignment`
- `disconnected_pad`
- `unrouted_net`
- `route_clearance`
- `missing_board_outline`
- `zone_unfilled`
- `zone_wrong_net`
- `placement_collision`
- `placement_outside_board`
- `roundtrip_diff`
- `kicad_cli_unavailable`
- `unsupported_object`
- `unsafe_user_content`
- `unknown`

Classification must be tested independently from repair execution.

## 8. Repair Actions

Initial repair actions should be conservative and aligned with current engine
capabilities.

### 8.1 Footprint Repair

For `missing_footprint`:

- use component selection and library resolver evidence when available;
- apply `assign_footprint` operations for generated symbols;
- block if no verified symbol-footprint-pinmap evidence exists at the requested
  acceptance level.

### 8.2 Net Assignment Repair

For `invalid_net_assignment` and `disconnected_pad`:

- rebuild pad net hints from schematic-to-PCB transfer data where available;
- regenerate affected place-footprint operations when the writer owns the
  footprint placement;
- rerun writer correctness and board validation.

### 8.3 Route Repair

For `unrouted_net`, `route_clearance`, or routing-stage issues:

- rerun routing for affected nets with the existing placement result;
- optionally widen search budgets or switch from single-layer to two-layer mode
  if policy allows;
- block if placement constraints make a route impossible.

### 8.4 Placement Repair

For `placement_collision` or `placement_outside_board`:

- rerun placement with adjusted board margin or group spread policy only when
  the original request allows it;
- preserve fixed components and connector edge constraints;
- rerun routing after any placement repair.

### 8.5 Outline Repair

For `missing_board_outline`:

- add or regenerate a board outline from request board dimensions when the
  project is generated by KiCadAI;
- block for imported projects unless the user explicitly opts into outline
  generation.

### 8.6 Zone Repair

For `zone_unfilled`:

- do not pretend to fill zones in-process;
- request KiCad refill/DRC when CLI is available;
- otherwise return a clear blocked or warning status depending on policy.

For `zone_wrong_net`:

- repair only generated zone operations whose intended net is present in the
  design model;
- rerun board validation.

### 8.7 Unsupported Repairs

For `roundtrip_diff`, `unsupported_object`, `unsafe_user_content`, and unknown
KiCad features:

- do not rewrite;
- return blocked with actionable diagnostics.

## 9. Repair Policy

Define options:

```go
type Options struct {
    Enabled bool
    Apply bool
    MaxAttempts int
    MaxAttemptsPerIssue int
    AllowPlacementRetry bool
    AllowRoutingRetry bool
    AllowFootprintAssignment bool
    AllowOutlineGeneration bool
    AllowKiCadCLI bool
    Acceptance designworkflow.AcceptanceLevel
}
```

Defaults:

- repair disabled unless called explicitly or design workflow option enables it;
- dry-run in CLI unless `--execute` is provided;
- maximum 3 total attempts;
- maximum 1 attempt per source issue;
- no unsafe imported-project mutation.

## 10. Workflow Integration

Add an optional `repair` stage after validation/KiCad checks in `design create`.

The stage should:

1. collect blocking issues from prior stages;
2. classify repairable issues;
3. create a repair plan;
4. apply allowed repairs if enabled;
5. rerun affected validation stages;
6. update final workflow result with repair stage and feedback.

If repair succeeds, the workflow should report improved achieved acceptance.
If repair is skipped or blocked, existing validation feedback remains intact.

## 11. CLI

Add a command such as:

```sh
kicadai --json repair plan --target ./out/project
kicadai --json repair apply --target ./out/project --execute
kicadai --json design create --request request.json --output ./out --repair
```

Initial CLI can support generated-project repair only. It should return JSON
with:

- classified issues;
- proposed attempts;
- dry-run changes;
- final or projected status;
- artifacts.

## 12. Safety Rules

Repair must block when:

- the target project contains unsupported imported objects and the repair would
  rewrite them;
- the issue lacks enough structured context to target a repair;
- a repair would remove components, nets, zones, or tracks not generated by
  KiCadAI;
- retry budget is exhausted;
- validation gets worse after an attempted repair;
- the requested acceptance level requires evidence the project cannot provide.

## 13. Testing Strategy

Required tests:

- classifier maps known issue codes to categories;
- unsupported issues are skipped or blocked with clear reasons;
- missing footprint repair produces expected transaction operations;
- disconnected pad repair updates pad net hints and revalidates;
- missing outline repair creates an outline for generated projects;
- unrouted net repair invokes routing and updates route operations;
- placement collision repair reruns placement only when allowed;
- repair loop stops after configured budgets;
- repair loop never reports success without revalidation;
- design workflow includes repair stage when enabled;
- CLI dry-run returns proposed repairs without writing.

Use fake validators/runners for deterministic tests. KiCad CLI-backed repair
tests must be optional.

## 14. Acceptance Criteria

The project is complete when:

- repair classification covers the issue codes emitted by current writer,
  board, placement, routing, and KiCad-check paths;
- at least three safe repair actions are implemented end to end;
- repair attempts are recorded with stable JSON output;
- design workflow can optionally run the repair loop;
- CLI dry-run and apply modes are available;
- failed repairs return blocked reports with actionable suggestions;
- `go test ./...` passes without KiCad installed.

## 15. Follow-Ups

Future work after this project:

- AI-assisted repair strategy selection above deterministic repair actions;
- imported-project preservation-aware repairs;
- KiCad GUI/API-backed interactive repair;
- richer DRC-driven placement and routing optimization;
- learned repair prioritization from historical failures.
