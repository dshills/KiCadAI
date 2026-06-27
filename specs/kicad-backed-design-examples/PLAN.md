# KiCad-Backed Design Examples Implementation Plan

Date: 2026-06-27

## Phase 1: Define Optional Fixture Metadata

Goal: make KiCad-backed design example expectations machine-readable before
adding richer fixtures.

Tasks:

- Add a small metadata model for files named
  `examples/design/kicad-backed/*.metadata.json`.
- Validate required fields:
  - `id`;
  - `request`;
  - `tier`;
  - `readiness`;
  - `acceptance`;
  - `require_erc`;
  - `require_drc`;
  - `expected_stages`;
  - `known_gaps`.
- Reject metadata whose `request` escapes the fixture directory or does not
  match an existing request file.
- Reject unknown tier/readiness values.
- Add unit tests for valid metadata, missing required fields, bad enum values,
  and path traversal.

Review:

- `go test ./internal/designworkflow -run 'TestDesignExample.*Metadata|TestDesignExamplesOptional'`
- `prism review staged`

Commit:

- `Add KiCad-backed design example metadata`

## Phase 2: Wire Metadata Into Optional Example Tests

Goal: make the optional tier enforce declared fixture expectations.

Tasks:

- Update `TestDesignExamplesOptionalKiCadBackedTier` to enumerate metadata
  files rather than raw request files.
- Load each request through the metadata's `request` field.
- Apply `require_erc` and `require_drc` to `KiCadCheckOptions`.
- Skip `readiness: blocked` fixtures with their known gaps.
- For `candidate`, `pass`, and `expected_fail` fixtures:
  - run `designworkflow.Create`;
  - assert expected stages exist;
  - assert pass fixtures have successful KiCad checks;
  - assert expected-fail fixtures produce blocked evidence rather than skip
    evidence;
  - preserve candidate failures as test failures unless explicitly allowlisted.
- Improve optional-tier failure output with fixture ID, readiness, output dir,
  stage statuses, issue paths, and artifacts.

Review:

- `go test ./internal/designworkflow -run 'TestDesignExamplesOptional|TestDesignExample.*Metadata'`
- `go test ./internal/designworkflow -run 'TestDesignExamples'`
- `prism review staged`

Commit:

- `Enforce KiCad-backed design example expectations`

## Phase 3: Add Initial Optional Smoke Fixture

Goal: add a conservative first fixture that exercises the optional tier without
overstating multi-block readiness.

Tasks:

- Create `examples/design/kicad-backed/led_indicator_kicad_smoke.json`.
- Create matching `led_indicator_kicad_smoke.metadata.json`.
- Start with the smallest truthfully supportable workflow:
  - structural or ERC/DRC acceptance depending on current behavior;
  - routing enabled only if it completes deterministically;
  - explicit `known_gaps` if a check is optional or candidate-only.
- Run the fixture manually with local `KICADAI_KICAD_CLI` if available.
- Ensure default `go test ./...` skips cleanly when KiCad is unavailable.

Review:

- `go test ./internal/designworkflow -run 'TestDesignExamples'`
- Optional local run:
  - `KICADAI_KICAD_CLI=/path/to/kicad-cli go test ./internal/designworkflow -run TestDesignExamplesOptionalKiCadBackedTier`
- `prism review staged`

Commit:

- `Add KiCad-backed LED design smoke fixture`

## Phase 4: Add Multi-Block Candidate Fixtures

Goal: begin tracking richer generated board readiness without blocking the
default test suite.

Tasks:

- Add one connector-plus-LED fixture if current connector/LED realization is
  stable enough.
- Add one I2C sensor breakout candidate or expected-fail fixture that captures
  current generic sensor/connector PCB realization gaps.
- Add one protected power-entry candidate only if its block-level evidence is
  ready enough for design-level generation.
- Use `readiness: expected_fail` when the fixture is intended to document a
  known gap rather than pass.
- Keep allowlists narrow and require a `known_gaps` entry for every allowed or
  expected issue.
- Add tests that verify expected-fail candidates are not silently skipped.

Review:

- `go test ./internal/designworkflow -run 'TestDesignExamples'`
- Optional local KiCad run when available.
- `prism review staged`

Commit:

- `Add multi-block KiCad-backed design candidates`

## Phase 5: Persist Optional KiCad Evidence Artifacts

Goal: make optional fixture output useful for humans and AI agents.

Tasks:

- Ensure optional runs write ERC/DRC reports under
  `.kicadai/checks/` inside the generated output.
- Verify `.kicadai/workflow-result.json` includes the KiCad check stage,
  artifacts, and issue summaries.
- Add assertions that passing optional fixtures have report artifact paths when
  checks are required.
- Add candidate/expected-fail assertions that blocked evidence is visible and
  tied to the correct stage.
- If workflow artifacts are missing useful fields, extend existing result
  serialization rather than adding fixture-specific files.

Review:

- `go test ./internal/designworkflow -run 'TestDesignExamplesOptional|TestDesignExamplesGenerateReadableProjectArtifacts'`
- `go test ./...`
- `prism review staged`

Commit:

- `Persist KiCad-backed design example evidence`

## Phase 6: Documentation And Agent Guidance

Goal: make the optional tier discoverable and hard to misuse.

Tasks:

- Add `examples/design/kicad-backed/README.md`.
- Update `examples/design/README.md` to link to optional KiCad-backed examples.
- Update `docs/intent-planning.md` with when to use default examples versus
  optional KiCad-backed examples.
- Update `docs/kicadai-agent-skill.md` so agents do not claim optional
  fixtures passed unless `KICADAI_KICAD_CLI` was configured and evidence
  exists.
- Ensure docs use the compiled `kicadai` binary, not `go run`.

Review:

- `rg "go run ./cmd/kicadai|go run" README.md docs examples -n`
- `go test ./...`
- `prism review staged`

Commit:

- `Document KiCad-backed design examples`

## Phase 7: Roadmap Update

Goal: record what was promoted and what remains blocked.

Tasks:

- Update `specs/ROADMAP.md` under Priority 2 and Priority 9.
- List optional fixtures by readiness:
  - pass;
  - candidate;
  - expected fail;
  - blocked.
- Record remaining implementation gaps discovered while running the optional
  examples, especially placement/routing/connector/sensor/protection issues.
- Keep the definition of autonomous readiness conservative.

Review:

- `go test ./...`
- `prism review staged`

Commit:

- `Update roadmap for KiCad-backed design examples`

## Acceptance Checklist

- Metadata validation exists and rejects unsafe or ambiguous fixtures.
- Optional example tests are skipped by default without KiCad.
- At least one optional KiCad-backed fixture exists.
- Optional fixture runs use `KICADAI_KICAD_CLI` and real `KiCadCheckOptions`.
- Required KiCad check artifacts are asserted when fixtures pass.
- Expected-fail fixtures produce blocked evidence rather than silent skips.
- Docs clearly separate default examples from optional KiCad-backed examples.
- `go test ./...` passes without requiring KiCad.
