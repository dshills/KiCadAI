# AI-Controlled Generation Lane Implementation Plan

## Phase 1: Baseline Existing Prompt And Intent Behavior

Goal: establish what already works before adding new orchestration.

Tasks:

- Run existing `intent draft`, `intent explain`, and `intent create` examples
  for LED, connector/LED, and I2C-style prompts.
- Capture current outputs, missing artifacts, status shapes, and blockers.
- Identify whether `intent create --text` already writes all needed `.kicadai`
  prompt, draft, plan, design, and validation artifacts.
- Identify which existing fixture should become the first AI-controlled lane
  golden: LED, connector/LED, or I2C breakout.
- Record the current blocker list in tests or a short baseline note if useful.

Acceptance:

- The first implementation target is chosen from actual behavior.
- No new behavior is added without knowing the current failure point.
- Existing command behavior remains unchanged.

Validation:

```sh
kicadai --text "make a simple LED indicator board" intent draft
kicadai --text "make a simple LED indicator board" --output examples/.generated/ai_lane_led_baseline --overwrite intent create
go test ./internal/intentdraft ./internal/intentplanner ./internal/designworkflow
```

Prism:

```sh
prism review staged
```

Commit:

```sh
Baseline AI controlled generation lane
```

## Phase 2: Define AI Lane Status Summary

Goal: add or normalize a compact final status object that AI callers can use.

Tasks:

- Add an internal status model for prompt-driven generation if no suitable
  existing model already exists.
- Normalize statuses to:
  - `ready`;
  - `candidate`;
  - `blocked`;
  - `needs_clarification`;
  - `unsupported`;
  - `tool_error`.
- Include stage, issue code, message, artifact paths, suggested next action,
  retryability, and clarification requirement.
- Reuse existing report, promotion, validation, and issue structures where
  possible.
- Add golden tests for status mapping from:
  - successful draft;
  - blocking clarification;
  - unsupported prompt;
  - design workflow blocker;
  - optional KiCad skip/tool error.

Acceptance:

- AI callers can determine the next action from one JSON object.
- Status does not hide validation failures.
- Existing CLI output remains backward compatible unless intentionally updated.

Validation:

```sh
go test ./internal/intentdraft ./internal/intentplanner ./internal/designworkflow ./internal/reports
```

Prism:

```sh
prism review staged
```

Commit:

```sh
Add AI lane status summary
```

## Phase 3: Persist Prompt-Driven Artifact Manifest

Goal: make `intent create --text` produce a durable AI-consumable output bundle.

Tasks:

- Ensure prompt-driven generation persists:
  - `.kicadai/intent-source.txt`;
  - `.kicadai/intent-draft.json`;
  - `.kicadai/intent-extraction.json`;
  - `.kicadai/intent-clarifications.json`;
  - `.kicadai/intent-plan.json`;
  - `.kicadai/design-request.json`;
  - `.kicadai/design-promotion.json`;
  - `.kicadai/validation-summary.json` or equivalent validation artifact;
  - `.kicadai/rationale.json` where available.
- Add a manifest or summary listing which artifacts exist, which were skipped,
  and why.
- Define the repository policy for generated `.kicadai/` artifacts, including
  whether project-local outputs remain ignored and which checked-in examples may
  intentionally commit evidence.
- Add or document recommended ignore rules for transient `.kicadai/` output
  when generated projects are created inside a repository. The default should
  preserve `.kicadai/` inside the generated project output, while repository
  templates may ignore generated output directories such as `out/` or
  `examples/.generated/`.
- Preserve partial artifacts for blocked runs when they are syntactically valid
  JSON/text or parseable KiCad files.
- Ensure output paths are project-relative in JSON.
- Add tests that assert artifact presence for the first supported lane.

Acceptance:

- A generated project directory contains enough evidence for an external AI to
  inspect without re-running commands.
- Blocked runs preserve diagnosis artifacts.
- Artifact paths are deterministic and safe for golden tests.

Validation:

```sh
go test ./internal/intentplanner ./internal/designworkflow
kicadai --text "make a simple LED indicator board" --output examples/.generated/ai_lane_led --overwrite intent create
```

Prism:

```sh
prism review staged
```

Commit:

```sh
Persist AI lane generation artifacts
```

## Phase 4: Add First-Lane Golden Prompt Fixtures

Goal: lock down supported prompt behavior with regression tests.

Tasks:

- Add prompt fixtures for:
  - simple LED indicator;
  - connector breakout with power LED;
  - 3.3V I2C sensor breakout.
- Add negative prompt fixtures with explicit expected statuses:
  - ambiguous voltage -> `needs_clarification`;
  - unsupported high-voltage or mains request -> `unsupported`;
  - amplifier/power-amplifier request outside first-lane automation ->
    `unsupported`;
  - fabrication-ready request without required KiCad evidence -> `unsupported`
    when the lane lacks the capability, or `blocked` when a supported lane is
    missing required local evidence.
- Add a fabrication-readiness prompt fixture that requests generated files with
  missing local KiCad evidence and asserts `unsupported` or `blocked` with a
  clear KiCad evidence issue.
- Store expected draft/status summaries, not brittle full project output.
- Ensure I2C may remain `blocked` or `candidate` depending on current routing
  evidence, but it must report a specific blocker.

Acceptance:

- Supported prompts produce structured intent and either project output or a
  precise current blocker.
- Unsafe or unsupported prompts fail closed.
- Tests do not require network access or a live LLM.

Validation:

```sh
go test ./internal/intentdraft ./internal/intentplanner ./internal/designworkflow
go test ./...
```

Prism:

```sh
prism review staged
```

Commit:

```sh
Add AI lane prompt goldens
```

## Phase 5: Wire Optional Repair Guidance Into The Lane

Goal: make blocked output actionable for AI retry loops without enabling unsafe
free-form edits.

Tasks:

- Surface existing repair bundle or repair guidance paths in the AI lane status
  when available.
- Mark whether retry is allowed based on issue category and generated-project
  ownership.
- Document when the AI should:
  - retry with repair apply;
  - revise structured intent;
  - ask the user a clarification;
  - stop as unsupported.
- Add tests for blocker-to-next-action mapping.
- Do not add arbitrary AI patching or direct KiCad file edits.

Acceptance:

- Blocked prompt-driven runs tell an AI what the next safe action is.
- Retry recommendations are bounded and deterministic.
- Revalidation remains required after repair.

Validation:

```sh
go test ./internal/repair ./internal/designworkflow ./internal/intentplanner
```

Prism:

```sh
prism review staged
```

Commit:

```sh
Expose AI lane repair guidance
```

## Phase 6: Document Agent Usage And Update Roadmap

Goal: make the new lane the documented shortest path to AI-generated schematics
and PCBs.

Tasks:

- Update README with a short "AI-controlled generation" section.
- Update `docs/kicadai-agent-skill.md` with the first-lane command sequence and
  status interpretation.
- Update `docs/intent-planning.md` or CLI docs with prompt-driven examples.
- Update `specs/ROADMAP.md` to mark this lane as the next practical AI
  generation milestone.
- Document unsupported scopes and why amplifiers are not the first automation
  lane.

Acceptance:

- An external AI agent can follow docs without reading source code.
- Docs use the compiled `kicadai` binary, not `go run`.
- Roadmap clearly distinguishes first-lane AI generation from general
  autonomous board design.

Validation:

```sh
go test ./...
git diff --check
```

Prism:

```sh
prism review staged
```

Commit:

```sh
Document AI controlled generation lane
```

## Overall Completion Criteria

The project is complete when:

- at least one prompt-driven lane generates a KiCad schematic and PCB or a
  precise current blocker through one documented CLI path;
- all artifacts needed by an external AI are persisted under `.kicadai/`;
- supported/unsupported prompt behavior is locked down by tests;
- final status is compact enough for AI prompt context;
- Prism has reviewed each phase;
- each phase is committed independently.
