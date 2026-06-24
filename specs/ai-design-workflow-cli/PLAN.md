# AI Design Workflow CLI Implementation Plan

## 1. Objective

Implement the `kicadai design create` workflow described in `SPEC.md`.

The implementation should proceed in reviewable phases. The first milestone is
not arbitrary natural-language design generation. The first milestone is a
deterministic CLI that accepts explicit block-composition requests and
orchestrates schematic generation, PCB realization, placement, routing,
validation, and feedback.

## 2. Implementation Rules

- Reuse existing packages before adding new abstractions.
- Keep the workflow deterministic and testable without KiCad GUI.
- Keep LLM planning outside the deterministic core for the first pass.
- Use existing `reports.Result` envelopes and `reports.Issue` codes.
- Preserve operation IDs and stage attribution where available.
- Run `gofmt` on edited Go files.
- Run focused tests for every phase.
- Run `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` before major
  commits where practical.
- Run `prism review staged` before each phase commit and address material
  correctness findings.
- Commit between phases.
- Do not stage unrelated changes such as an existing modified
  `specs/ROADMAP.md` unless explicitly requested.

## 3. Phase 1: Request Schema And Model

### Goal

Add a typed model for design workflow requests and validation.

### Work

- Add `internal/designworkflow` package.
- Define:
  - `Request`;
  - `Intent`;
  - `BoardSpec`;
  - `LibrarySpec`;
  - `BlockInstanceSpec`;
  - `ConnectionSpec`;
  - `ConstraintSpec`;
  - `ValidationSpec`;
  - `AcceptanceLevel`.
- Add JSON decoding with unknown-field rejection if local patterns support it.
- Add `ValidateRequest`.
- Normalize project names, block instance IDs, and acceptance defaults.
- Convert explicit block specs to `blocks.CompositionRequest`.

### Tests

- Valid explicit block-composition request passes.
- Duplicate block IDs fail.
- Unknown acceptance level fails.
- Invalid connection endpoint syntax fails.
- Missing board dimensions fail.

### Acceptance Criteria

- No CLI behavior changes yet.
- Package tests pass.

### Commit Message

```text
Add design workflow request model
```

## 4. Phase 2: Stage Result And Feedback Model

### Goal

Create the workflow reporting contract before orchestration logic grows.

### Work

- Define:
  - `StageName`;
  - `StageStatus`;
  - `StageResult`;
  - `WorkflowResult`;
  - `AcceptanceResult`;
  - `Feedback`;
  - `RepairSuggestion`.
- Add helpers for:
  - appending stage issues;
  - computing overall `ok`;
  - computing achieved acceptance level;
  - grouping issues by stage/ref/net/block/operation.
- Map existing issues to workflow repair scopes.

### Tests

- Blocking stage marks workflow blocked.
- Warning-only stages keep workflow ok.
- Acceptance level downgrades are explicit.
- Feedback groups refs and nets deterministically.

### Acceptance Criteria

- Reports can represent a failed workflow before the workflow exists.

### Commit Message

```text
Add design workflow result model
```

## 5. Phase 3: Explicit Block Planning

### Goal

Turn explicit `blocks[]` and `connections[]` request data into block
composition output.

### Work

- Add planner function:
  - `PlanBlocks(ctx, registry, request)`.
- Validate block IDs against `blocks.Registry`.
- Validate block params through existing block request validation.
- Convert connection endpoint strings to `blocks.CompositionConnection`.
- Call `blocks.ComposeBlocks`.
- Return stage result with block summaries and issues.

### Tests

- LED plus connector request composes.
- Unknown block returns structured issue.
- Unknown port returns structured issue.
- Conflicting voltage domains propagate as workflow feedback.

### Acceptance Criteria

- Explicit composition can produce schematic operations through the new package.

### Commit Message

```text
Plan explicit design workflow blocks
```

## 6. Phase 4: Schematic Transaction And Project Apply

### Goal

Generate and apply the schematic-side project transaction.

### Work

- Convert composition output to project transaction with existing block helpers.
- Validate transaction before apply.
- Apply transaction to `--output`.
- Emit project/schematic artifacts.
- Respect `--overwrite`.
- Keep stage summaries for operation counts and artifact paths.

### Tests

- Simple block design writes `.kicad_pro` and `.kicad_sch`.
- Existing output without overwrite fails.
- Transaction validation issue blocks apply.

### Acceptance Criteria

- `designworkflow.Create` can write a schematic-only project for explicit block
  requests.

### Commit Message

```text
Apply design workflow schematic transactions
```

## 7. Phase 5: PCB Fragment Realization

### Goal

Realize each composed block into PCB fragment data.

### Work

- For every block instance:
  - fetch block definition;
  - instantiate block with normalized params;
  - call `blocks.RealizeBlockPCB`;
  - apply deterministic block-local transform.
- Add a simple fragment layout strategy:
  - row/grid placement within board bounds;
  - deterministic spacing;
  - warning when estimated extent exceeds board.
- Merge realized components, local routes, constraints, and unsupported
  behaviors into workflow state.

### Tests

- LED request produces two PCB components and one local route.
- Connector breakout produces connector placement.
- Unsupported realization behavior is warning-level unless acceptance requires
  it.
- Board too small produces placement/floorplan issue.

### Acceptance Criteria

- Workflow state contains PCB fragments ready for board-level placement.

### Commit Message

```text
Realize design workflow PCB fragments
```

## 8. Phase 6: Board-Level Placement Integration

### Goal

Build and execute one placement request for the whole design.

### Work

- Convert realized fragments to one `placement.Request`.
- Include board size and constraints from request.
- Hydrate library geometry when resolver is configured.
- Preserve fixed local placements where required.
- Run `placement.PlaceContext`.
- Emit placement operations.
- Add placement quality feedback to workflow report.

### Tests

- LED plus connector placement succeeds.
- Small board produces blocked placement feedback.
- Resolver-unavailable path uses estimated bounds with warning.

### Acceptance Criteria

- Workflow emits deterministic `place_footprint` operations for complete
  design placement.

### Commit Message

```text
Place design workflow PCB fragments
```

## 9. Phase 7: Routing Integration

### Goal

Route simple generated boards and preserve partial-route feedback.

### Work

- Convert placement output to `routing.Request`.
- Include board constraints:
  - layers;
  - grid;
  - trace width;
  - clearance;
  - allow partial.
- Seed routing with required local routes where possible.
- Run router unless `--skip-routing`.
- Emit route operations.
- Add route diagnostics to workflow feedback.

### Tests

- LED local route appears in transaction output.
- Two-connector simple signal route succeeds.
- Unroutable fixture returns routing repair suggestion.
- `--skip-routing` marks routing stage skipped.

### Acceptance Criteria

- Connectivity acceptance can be attempted for small known-good boards.

### Commit Message

```text
Route design workflow boards
```

## 10. Phase 8: PCB Write And Schematic-To-PCB Transfer

### Goal

Produce `.kicad_pcb` output that matches the schematic refs and nets.

### Work

- Use existing schematic-to-PCB transfer and design API/writer helpers.
- Add board outline from request.
- Apply placement and route operations.
- Write final project files.
- Ensure project-local library tables are written when required.
- Preserve stage artifact paths.

### Tests

- Generated project includes `.kicad_pcb`.
- PCB refs match schematic refs.
- Board outline exists.
- Inspect project succeeds.

### Acceptance Criteria

- `design create` writes a parseable KiCad project with schematic and PCB.

### Commit Message

```text
Write design workflow PCB projects
```

## 11. Phase 9: Validation Pipeline

### Goal

Run deterministic validation gates and compute achieved acceptance.

### Work

- Run:
  - inspect project;
  - evaluate project;
  - transaction validation;
  - board validation.
- Map validation outputs into stage results.
- Enforce acceptance levels:
  - draft;
  - structural;
  - connectivity.
- Wire `--strict-unrouted` and `--strict-zones`.

### Tests

- Structural request reaches structural acceptance.
- Missing footprint fails structural acceptance.
- Unrouted net fails connectivity when strict.
- Warning-only validation does not block draft acceptance.

### Acceptance Criteria

- Output clearly states requested and achieved acceptance levels.

### Commit Message

```text
Validate design workflow outputs
```

## 12. Phase 10: KiCad ERC/DRC Integration

### Goal

Add optional KiCad-backed validation evidence.

### Work

- Reuse existing check runners.
- Run ERC against generated schematic when requested.
- Run DRC against generated PCB when requested.
- Respect:
  - `--kicad-cli`;
  - `--timeout`;
  - `--allowlist`;
  - `--require-drc`;
  - request validation policy.
- Mark missing KiCad CLI as skipped or blocking depending on acceptance.

### Tests

- Fake KiCad runner success reaches `erc-drc`.
- Fake ERC finding becomes feedback.
- Missing KiCad CLI blocks only when required.
- Allowlisted finding is suppressed.

### Acceptance Criteria

- `erc-drc` acceptance has explicit KiCad evidence.

### Commit Message

```text
Add KiCad checks to design workflow
```

## 13. Phase 11: CLI Command

### Goal

Expose the workflow through `kicadai design create`.

### Work

- Add `design` command family.
- Add `create` subcommand.
- Require `--json`, `--request`, and `--output`.
- Parse workflow-specific flags.
- Call `designworkflow.Create`.
- Return `reports.Result`.
- Add usage text.

### Tests

- Missing subcommand returns structured failure.
- Missing request returns structured failure.
- Simple LED request returns workflow result.
- Existing output without overwrite fails.

### Acceptance Criteria

- User can run:

```sh
kicadai --json design create --request examples/design/led.json --output /tmp/kicadai-led --overwrite
```

### Commit Message

```text
Expose design create CLI
```

## 14. Phase 12: Example Requests And Golden Fixtures

### Goal

Add small stable examples that define the supported first milestone.

### Work

- Add `examples/design/led_indicator.json`.
- Add `examples/design/sensor_breakout.json`.
- Add `examples/design/README.md`.
- Add golden JSON snapshots for selected stage summaries.
- Avoid committing generated output directories unless intentionally small and
  stable.

### Tests

- Example requests parse and validate.
- LED example reaches at least structural acceptance.
- Sensor breakout example produces actionable feedback if full routing is not
  ready.

### Acceptance Criteria

- Docs and examples demonstrate the intended workflow without hidden setup.

### Commit Message

```text
Add design workflow examples
```

## 15. Phase 13: Repair Feedback Refinement

### Goal

Make failures actionable for AI loops.

### Work

- Add repair suggestion builders for:
  - unknown block;
  - invalid parameter;
  - missing footprint;
  - placement outside board;
  - route blocked;
  - unrouted required net;
  - ERC finding;
  - DRC finding.
- Include `retry_scope`.
- Include `stage`.
- Include block instance and operation IDs when available.

### Tests

- Each major failure type has a repair suggestion.
- Suggestions are deterministic.
- No suggestion leaks local secrets or unrelated paths.

### Acceptance Criteria

- An AI agent can tell whether to modify request JSON, rerun placement, rerun
  routing, provide KiCad libraries, or escalate to user review.

### Commit Message

```text
Refine design workflow repair feedback
```

## 16. Phase 14: Documentation And Status

### Goal

Document the workflow, current capabilities, and remaining limits.

### Work

- Update `README.md`.
- Add or update `examples/design/README.md`.
- Document request schema.
- Document acceptance levels.
- Document command examples.
- Document known limitations:
  - explicit block request first;
  - limited global floorplanning;
  - limited routing complexity;
  - KiCad CLI required for ERC/DRC evidence;
  - no fabrication-readiness claim without validation evidence.

### Tests

- Run final `go test ./...`.
- Run example command at least once.

### Acceptance Criteria

- README accurately reflects implemented state.

### Commit Message

```text
Document design workflow CLI
```

## 17. Initial Vertical Slice Definition

The first end-to-end vertical slice should be:

- request: LED indicator plus two-pin connector breakout;
- output: KiCad project with schematic and PCB;
- placement: all components placed;
- routing: LED series net routed, connector nets represented;
- validation: structural and connectivity validation run;
- feedback: no blocking issues for `structural`, clear caveats for
  `connectivity` if any route is skipped.

This slice proves the orchestration path without pretending the system can
already synthesize arbitrary boards.

