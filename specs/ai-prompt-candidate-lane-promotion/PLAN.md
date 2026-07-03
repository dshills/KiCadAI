# AI Prompt Candidate Lane Promotion Implementation Plan

Date: 2026-07-03

## Objective

Promote the first user-facing prompt-driven generation lane so the simple LED
indicator prompt produces a candidate-ready KiCad project with stable
AI-readable evidence.

Follow the project convention:

- implement one phase at a time;
- run focused tests for the phase;
- stage intended changes;
- run `prism review staged`;
- fix high and medium findings;
- commit before moving to the next phase.

## Phase 1: Baseline The Prompt Lane

### Goals

- Capture the exact current behavior of the primary LED prompt.
- Identify whether the stale blocker is still placement, routing, validation,
  promotion classification, or documentation drift.

### Tasks

- Run:

  ```sh
  kicadai --text "make a simple LED indicator board" --output examples/.generated/ai_led_prompt_candidate --overwrite intent create
  ```

- If `kicadai` is not built or current, build `./bin/kicadai` and rerun with
  the binary.
- Inspect generated artifacts:
  - `.kicadai/workflow-result.json`;
  - `.kicadai/validation-summary.json`;
  - `.kicadai/design-promotion.json`;
  - `.kicadai/retry-state.json`, if written;
  - generated `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb`.
- Record:
  - AI status;
  - workflow stage outcomes;
  - promotion readiness;
  - placement/routing summaries;
  - validation blockers;
  - whether output project files exist.
- Add or update a focused golden test that freezes the current prompt-lane
  status before fixes.

### Acceptance

- The current prompt lane has a precise baseline in tests or fixture evidence.
- The implementation target is known before any behavior changes.

### Review And Commit

- Run focused tests for touched packages.
- Run `prism review staged`.
- Commit message: `Baseline LED prompt candidate lane`.

## Phase 2: Remove The Blocking Gap

### Goals

- Fix the concrete blocker preventing the LED prompt lane from reaching
  candidate readiness.

### Tasks

- If the blocker is placement:
  - trace generated board dimensions, usable area, fixed placements, group
    transforms, and component mobility;
  - ensure generated LED/connector footprints are inside the usable board area;
  - add focused placement tests for generated prompt output.
- If the blocker is routing:
  - verify LED net intent reaches the routing/local-route layer;
  - prove required endpoint contacts against physical pads;
  - add focused routing/contact evidence tests.
- If the blocker is validation:
  - fix the narrow writer, transfer, schematic electrical, or board validation
    issue exposed by the prompt lane;
  - add a regression test for the exact issue.
- If the blocker is classification only:
  - adjust promotion or AI-status mapping so warning-only evidence is reported
    honestly without downgrading a candidate-ready project;
  - use a strict allow-list for candidate warnings, and keep unclassified
    warnings blocking until they are understood.

### Acceptance

- The primary LED prompt no longer stops at the old blocker.
- No validation gate is bypassed or weakened.
- Unsafe prompts still fail closed.

### Review And Commit

- Run focused package tests.
- Run `go test ./...` if the fix touches shared placement, routing, writer, or
  workflow code.
- Run `prism review staged`.
- Commit message: `Promote LED prompt generation blocker`.

## Phase 3: Promote The Golden Prompt Fixture

### Goals

- Convert the first prompt-lane golden from blocked to candidate or ready.
- Keep negative prompt behavior stable.

### Tasks

- Update CLI golden tests for:
  - LED prompt success;
  - unsafe mains/high-voltage prompt fail-closed;
  - ambiguous prompt clarification where currently supported.
- Add adversarial negative prompt coverage for boundary-crossing input, such as
  attempts to force unsafe high-voltage/current assumptions through prompt text.
- Assert the LED prompt output includes:
  - `data.ai_status.status` of `candidate` or `ready`;
  - generated project files;
  - validation summary;
  - promotion report;
  - no blocking writer correctness, schematic electrical, or board validation
    findings.
- Assert AI status does not include executable generated command arrays.
- Add fixture assertions for stable artifact paths and retry fields when
  present.

### Acceptance

- Default tests prove the prompt lane is candidate-ready without requiring
  KiCad.
- Negative prompt tests prove fail-closed behavior remains intact.

### Review And Commit

- Run:

  ```sh
  go test ./cmd/kicadai ./internal/intentdraft ./internal/intentplanner ./internal/designworkflow
  ```

- Run `prism review staged`.
- Commit message: `Promote LED prompt golden to candidate`.

## Phase 4: Optional KiCad-Backed Smoke

### Goals

- Verify the prompt lane against real KiCad when available without making KiCad
  mandatory for default tests.

### Tasks

- Add or update an optional test gated by `KICADAI_KICAD_CLI`.
- Run the LED prompt output through the existing KiCad ERC/DRC policy.
- Classify findings through promotion reports rather than ad hoc test logic.
- Store any generated optional artifacts under ignored generated-output
  directories.
- Ensure optional smoke tests use temporary or cleaned generated-output
  directories so stale KiCad artifacts do not affect later runs.

### Acceptance

- Default `go test ./...` still works without KiCad.
- When KiCad is configured, the prompt lane produces KiCad-backed promotion
  evidence or a precise blocker.

### Review And Commit

- Run focused optional tests if KiCad is configured locally.
- Run default focused tests without KiCad.
- Run `prism review staged`.
- Commit message: `Add KiCad smoke for LED prompt lane`.

## Phase 5: Documentation And Roadmap

### Goals

- Make docs match the new AI-generation status.

### Tasks

- Update README:
  - simple LED prompt is candidate-ready if Phase 3 succeeds;
  - first-lane status remains narrow and deterministic;
  - broader prompts may still block.
- Update `docs/intent-planning.md` with the promoted prompt workflow and
  artifact interpretation.
- Update `docs/kicadai-agent-skill.md` so agents know the supported prompt,
  status fields, and safe retry pattern.
- Update `specs/ROADMAP.md`:
  - move prompt-driven LED generation from blocker to implemented foundation;
  - keep I2C, amplifier, broader topology synthesis, and fabrication readiness
    gaps explicit.

### Acceptance

- Docs do not mention a stale LED placement blocker if it is resolved.
- Docs do not imply arbitrary AI board generation.
- Commands use `kicadai`, not `go run`.

### Review And Commit

- Run doc-relevant tests if any examples changed.
- Run `prism review staged`.
- Commit message: `Document promoted AI prompt lane`.

## Phase 6: Full Regression

### Goals

- Confirm the promoted prompt lane did not regress broader project behavior.

### Tasks

- Run:

  ```sh
  go test ./...
  ```

- Run the primary prompt command manually into a disposable generated directory.
- Inspect final JSON and generated `.kicadai/` artifacts.
- Confirm `git status --short` contains only intentional changes before final
  staging.

### Acceptance

- Full tests pass.
- The prompt command produces candidate-ready output.
- Prism has no unresolved high or medium findings on staged changes.
- All phase commits are present.

### Review And Commit

- If Phase 6 produces code or doc changes, run `prism review staged` and commit
  them.
- Otherwise record the test command/results in the final implementation summary.

## Done Criteria

This plan is complete when:

- the primary LED prompt lane is candidate-ready or the remaining blocker is
  narrower and explicitly documented;
- the README, roadmap, and agent docs match the actual result;
- unsafe prompts remain fail-closed;
- full tests pass;
- each implementation phase has been reviewed with Prism and committed.
