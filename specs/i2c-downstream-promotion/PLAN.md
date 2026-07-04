# I2C Downstream Promotion Implementation Plan

Date: 2026-07-04

## Objective

Carry `i2c_sensor_breakout_candidate` from route-tree-complete
`expected_fail` toward structural `candidate` by proving downstream
project-write, writer-correctness, validation, and KiCad evidence behavior.

Implement in phases. After each implementation phase:

1. run the focused tests for that phase;
2. run `go test ./...` when production behavior changes;
3. stage only that phase;
4. run `prism review staged`;
5. fix high/medium findings;
6. commit before moving to the next phase.

Use:

```sh
GOCACHE=.cache/go-build go test ...
```

## Phase 1: Downstream Stop Diagnosis

### Goals

- Identify the exact reason the I2C fixture remains `expected_fail` after route
  completion now passes.
- Capture the current stage status, issue, and gate evidence before changing
  behavior.

### Tasks

- Add or extend a focused test that runs `i2c_sensor_breakout_candidate` and
  records:
  - routing status;
  - route-completion gate;
  - project-write status;
  - writer-correctness status;
  - validation status;
  - KiCad-check status;
  - promotion achieved readiness;
  - blocking issue codes and paths.
- Determine whether downstream stages stop because of:
  - routing stage `warning` versus `ok`;
  - fixed-net or missing-net-class notices treated as blockers;
  - expected-fail metadata policy;
  - artifact path/write setup;
  - writer-correctness parse/readback failure;
  - validation failure;
  - missing optional KiCad evidence.
- Add a concise diagnostic helper only if existing promotion formatting hides
  the blocker.

### Acceptance

- A failing or passing test names the exact downstream blocker.
- Route completion remains locked at 12/12 proven endpoints and 4 complete
  groups.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'I2C|Promotion|ProjectWrite|WriterCorrectness|Validation' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Diagnose I2C downstream promotion blockers`.

## Phase 2: Stage Continuation After Route Completion

### Goals

- Ensure complete route-tree proof allows the workflow to continue into
  project-write.
- Prevent informational routing notices from blocking downstream stages.

### Tasks

- Inspect routing-to-project-write gating.
- If fixed-net skip notices or missing-net-class warnings still stop workflow
  continuation, classify them as non-blocking for stage continuation while
  preserving them as diagnostic evidence.
- If `StageRouting` status remains warning due only to non-blocking notices,
  ensure downstream continuation uses severity/gate semantics rather than raw
  warning status.
- Add tests proving:
  - complete route-tree contact proof permits project-write;
  - true route contact misses still block project-write;
  - missing required endpoints still block project-write.

### Acceptance

- I2C reaches `project_write` after routing.
- Route-contact regressions still stop downstream stages.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'I2C|ProjectWrite|RouteCompletion|RouteTree' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Continue I2C workflow after route-tree proof`.

## Phase 3: Project Artifact Proof

### Goals

- Prove the generated I2C project writes KiCad-native artifacts.
- Make artifact evidence replayable and promotion-visible.

### Tasks

- Run the I2C fixture through project-write into a deterministic output
  directory.
- Assert expected artifacts exist:
  - `.kicad_pro`;
  - `.kicad_sch`;
  - `.kicad_pcb`;
  - `.kicadai/design-promotion.json`;
  - any writer/validation artifacts currently expected by metadata.
- Ensure artifact paths in stage summaries and promotion reports are relative
  to the project output root.
- If output path handling uses temp directories that break replay, adjust the
  test harness or artifact reporting.

### Acceptance

- Project-write stage is `ok` for the I2C fixture in default local mode.
- Required artifacts exist and are listed in promotion evidence.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|ProjectWrite|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Prove I2C project write artifacts`.

## Phase 4: Writer Correctness Closeout For I2C

### Goals

- Prove generated I2C schematic and PCB files round-trip through the KiCadAI
  readers with stable net and component identity evidence.

### Tasks

- Add writer-correctness assertions for the generated I2C project:
  - schematic readback succeeds;
  - PCB readback succeeds;
  - component identity properties exist;
  - footprint refs and pads read back;
  - pad net names match VCC/GND/SDA/SCL expectations;
  - route copper net names are non-empty and expected;
  - net codes resolve through current KiCad 10 name-only behavior.
- If readback fails, compare generated files against known-good writer patterns
  and fix the writer rather than weakening the fixture.
- Keep writer-correctness issues actionable with file/path/ref/net evidence.

### Acceptance

- Writer-correctness stage is `ok` or only warning-level for explicitly
  accepted non-blocking evidence.
- Pad and copper net assignment regressions fail tests.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./internal/kicadfiles/... -run 'I2C|WriterCorrectness|NetAssignment|Readback' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Close I2C writer correctness evidence`.

## Phase 5: Board Validation Evidence

### Goals

- Prove the written I2C PCB is electrically meaningful after readback, not just
  parseable.

### Tasks

- Assert board validation reports:
  - required endpoint nets are present;
  - no required VCC/GND/SDA/SCL pads are disconnected;
  - route completion remains complete after write/readback;
  - board outline exists;
  - zones are either valid or explicitly not required by the fixture.
- Add regression tests for a deliberate I2C-like disconnected pad if existing
  validation tests do not cover the failure mode.
- Ensure validation artifacts are referenced in promotion output.

### Acceptance

- Validation stage is `ok` for the I2C fixture in default local mode.
- A disconnected I2C pad fixture fails validation.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./internal/boardvalidation -run 'I2C|BoardValidation|Connectivity|RouteCompletion' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Validate I2C written board connectivity`.

## Phase 6: KiCad Evidence Policy

### Goals

- Keep default tests KiCad-independent while making local KiCad ERC/DRC evidence
  explicit and promotion-visible.

### Tasks

- Verify default mode without `KICADAI_KICAD_CLI`:
  - KiCad checks are skipped/not-run with explicit external evidence;
  - no silent pass is reported.
- Verify configured mode when local KiCad is available:
  - ERC/DRC commands run;
  - report artifact paths are recorded;
  - failures and warnings map to promotion gates.
- Add or update fake-runner tests if real KiCad is not available in CI.
- Decide whether I2C can become structural `candidate` with optional KiCad
  checks skipped, or must remain `expected_fail` until real local KiCad evidence
  is clean.

### Acceptance

- Optional KiCad absence is explicit.
- Required KiCad policy blocks promotion when evidence is missing.
- Fake-runner or local KiCad tests cover ERC/DRC artifact reporting.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|KiCad|ERC|DRC|Promotion' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Record I2C KiCad evidence policy`.

## Phase 7: Metadata Promotion Decision

### Goals

- Align fixture metadata with achieved readiness.
- Avoid both stale `expected_fail` and premature `pass`.

### Tasks

- If internal downstream gates pass and KiCad is optional for `candidate`,
  update `i2c_sensor_breakout_candidate.metadata.json` readiness to
  `candidate`.
- If KiCad required evidence still blocks candidate readiness, keep
  `expected_fail` and update `known_gaps` to the exact remaining KiCad blocker.
- Update:
  - `examples/design/kicad-backed/README.md`;
  - `README.md`;
  - `docs/layout-routing.md`;
  - `specs/ROADMAP.md`.
- Ensure no current doc says route-tree contact proof is the active I2C blocker.

### Acceptance

- Promotion report matches metadata.
- Docs identify the next real blocker.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Update I2C downstream promotion status`.

## Phase 8: Full Regression

### Goals

- Confirm the repo is stable after I2C downstream promotion work.

### Tasks

- Run:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|ProjectWrite|WriterCorrectness|Validation|KiCad|Promotion|DesignExamples' -count=1
  go test ./...
  ```

- Check `git status --short`.

### Acceptance

- Focused and full tests pass.
- Worktree is clean except intentional follow-up specs.
