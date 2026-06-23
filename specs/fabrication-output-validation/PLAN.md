# Fabrication Output Validation Implementation Plan

## Objective

Implement KiCad CLI-backed Gerber and drill artifact generation, validation,
manifest integration, and readiness gating for fabrication packages.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests hermetic; do not require a local KiCad installation.
- Keep export dry-run by default.
- Require `--execute` before writing fabrication artifacts.
- Require `--overwrite` before replacing existing generated or external
  artifact paths.
- Use explicit argv execution for KiCad CLI; do not build shell command
  strings.
- Keep all generated package paths inside the project/package output directory.
- Do not claim manufacturer acceptance or full DFM coverage.

## Phase 1: Plotting Model And Runner Interface

### Goal

Introduce a testable fabrication plotting abstraction without changing export
behavior.

### Work

- Add fabrication plotting request/result models:
  - project root;
  - project name;
  - PCB path;
  - package output directory;
  - Gerber output directory;
  - drill output directory;
  - execute/dry-run;
  - overwrite;
  - KiCad CLI path;
  - CLI policy.
- Add command evidence models for:
  - command kind;
  - argv;
  - output directory;
  - exit status;
  - stdout/stderr snippets;
  - generated paths;
  - skipped reason.
- Add a runner interface for KiCad CLI calls.
- Add a default runner using `exec.CommandContext`.
- Add fake runner support for tests.
- Add path helpers that normalize package-relative artifact paths and reject
  path escapes.

### Tests

- Plot request path normalization.
- Output directory containment checks.
- Fake runner captures command evidence deterministically.
- Context cancellation is surfaced as a structured issue.
- No export behavior changes yet.

### Acceptance

- Fabrication plotting can be exercised in unit tests without invoking KiCad.
- Existing export tests continue passing.

### Commit

```text
Add fabrication plotting runner model
```

## Phase 2: KiCad CLI Gerber And Drill Generation

### Goal

Wire the plotting runner into `export fabrication` while preserving dry-run and
overwrite safety.

### Work

- Implement KiCad CLI command construction for:
  - Gerber plotting from the project PCB;
  - Excellon drill export from the project PCB.
- Resolve the project PCB path from the existing evaluation target.
- In dry-run:
  - report intended Gerber/drill output directories;
  - do not create directories or files;
  - mark artifacts as expected or skipped according to policy.
- In execute mode:
  - require `--kicad-cli`;
  - create package output directories only after path safety checks;
  - refuse to overwrite existing Gerber/drill directories without
    `--overwrite`;
  - invoke the runner;
  - collect generated files after each command.
- Attach command issues to fabrication result artifacts.
- Preserve existing BOM/CPL/readiness/manifest writes.

### Tests

- Dry-run does not write Gerber/drill directories.
- Execute with fake runner writes deterministic Gerber/drill files.
- Missing KiCad CLI blocks generation with stable issue.
- Existing output directory requires overwrite.
- Fake runner failure blocks readiness and records command evidence.
- CLI JSON exposes Gerber/drill artifact paths.

### Acceptance

- `ExportPackage` can generate Gerber/drill files through a fake runner.
- Real KiCad CLI integration is isolated to the default runner and command
  builder.

### Commit

```text
Generate fabrication Gerber and drill artifacts
```

## Phase 3: Artifact Validation And Evidence Gates

### Goal

Validate generated or existing Gerber/drill outputs before allowing readiness
to improve.

### Work

- Add Gerber validation:
  - required copper layers;
  - `Edge.Cuts`;
  - non-empty files;
  - package-relative paths.
- Add drill validation:
  - detect through-hole pads, vias, and drilled mounting holes from PCB data;
  - require non-empty drill output when drilled features exist;
  - allow skipped/pass-like evidence when no drilled features exist.
- Add artifact-level issues for:
  - missing layer output;
  - missing edge cuts;
  - missing drill output;
  - empty file;
  - path escape;
  - failed KiCad CLI command.
- Update readiness summary:
  - `Summary.Gerber`;
  - `Summary.Drill`;
  - evidence map entries.
- Ensure failed validation blocks `ready`.

### Tests

- Missing copper Gerber fails.
- Missing `Edge.Cuts` fails.
- Empty Gerber file fails.
- Missing drill output fails when drilled features exist.
- Drill output is skipped/pass-like when there are no drilled features.
- Existing external Gerber/drill files can be evaluated without execution.
- Validation issues are stable and sorted.

### Acceptance

- Readiness is based on validated artifacts, not only command success or file
  existence.

### Commit

```text
Validate fabrication output artifacts
```

## Phase 4: Manifest And Report Integration

### Goal

Make generated fabrication artifacts first-class package manifest and report
entries.

### Work

- Add generated Gerber/drill file lists to artifact entries or related evidence
  structures without breaking manifest validation.
- Mark artifact generator:
  - `kicad-cli` for files generated during export;
  - `external` for pre-existing evidence not created by the current run;
  - `kicadai` for readiness/BOM/CPL/manifest metadata.
- Include artifact-level issues in manifest output.
- Ensure `fabrication.ReportArtifacts` maps generated Gerber/drill evidence to
  report artifacts.
- Keep manifest ordering deterministic.
- Add result-level summaries useful to AI callers:
  - generated file counts;
  - missing required output categories;
  - KiCad CLI attempted/skipped status.

### Tests

- Manifest includes Gerber/drill artifacts with relative package paths.
- Manifest includes generator values.
- Artifact issues serialize and validate.
- Report artifact mapping includes Gerber and drill entries.
- JSON snapshots or selected-field tests cover stable CLI output.

### Acceptance

- `package-manifest.json` is sufficient for downstream tooling to find
  fabrication outputs and understand why readiness passed or failed.

### Commit

```text
Record fabrication output evidence in manifests
```

## Phase 5: CLI And Workflow Policy Refinement

### Goal

Expose generated fabrication output behavior through CLI flags and
fabrication-candidate workflows without surprising writes.

### Work

- Keep `export fabrication` as the primary executable fabrication output path.
- Confirm `--execute`, `--overwrite`, `--output`, `--kicad-cli`,
  `--allow-missing-drc`, and `--require-drc` interactions.
- Decide whether `design create` remains dry-run-only for fabrication preview
  or gains an explicit export execution option.
- If design workflow execution is added:
  - require explicit opt-in;
  - keep generated outputs inside the generated project;
  - preserve dry-run default.
- Add CLI help text for generated Gerber/drill behavior.
- Add selected-field CLI tests for:
  - dry-run expected artifacts;
  - execute generated artifacts;
  - missing KiCad CLI;
  - overwrite refusal.

### Tests

- CLI dry-run output remains non-mutating.
- CLI execute output writes artifacts with fake runner.
- CLI missing KiCad path produces stable blocked issue.
- CLI overwrite policy is enforced.
- Existing design-create fabrication-candidate tests remain deterministic.

### Acceptance

- Users and AI callers can request fabrication artifact generation explicitly
  and inspect stable JSON evidence.

### Commit

```text
Expose fabrication output generation in CLI evidence
```

## Phase 6: Optional Real KiCad Smoke Coverage

### Goal

Add non-default smoke coverage for real KiCad CLI behavior without making CI or
local default tests flaky.

### Work

- Add an integration test helper gated by an environment variable such as
  `KICADAI_REAL_KICAD_CLI`.
- Use a small checked-in or generated board fixture.
- Run Gerber/drill generation into a temporary or example-local output path
  that respects existing working-directory constraints.
- Validate non-empty generated outputs.
- Capture KiCad CLI version when available.
- Skip cleanly when the environment variable is missing.

### Tests

- Default `go test ./...` skips real KiCad smoke tests.
- With the environment variable set, smoke test runs and validates generated
  files.
- Failure output includes command evidence.

### Acceptance

- Developers with KiCad installed can verify real plotting behavior locally.
- Default automated tests remain hermetic.

### Commit

```text
Add optional KiCad fabrication smoke test
```

## Phase 7: Documentation And Roadmap Closeout

### Goal

Document the new fabrication output behavior and update the roadmap.

### Work

- Update README fabrication export section:
  - KiCad CLI-backed Gerber/drill generation;
  - dry-run default;
  - execute/overwrite requirements;
  - output directories;
  - readiness semantics;
  - remaining DFM/manufacturer limitations.
- Update `specs/ROADMAP.md`:
  - mark Gerber/drill generation and validation foundation as implemented;
  - move next near-term priority to larger-board placement/routing convergence
    with KiCad DRC-backed layout evidence.
- Add any needed example request notes.
- Run full test suite.

### Tests

- `go test ./...`
- Documentation references current command names and fields.

### Acceptance

- Documentation and roadmap match implemented behavior.

### Commit

```text
Document fabrication output validation
```

## Final Acceptance Checklist

- Gerber and drill plotting can be requested through `export fabrication`.
- Dry-run remains non-mutating.
- Execute mode writes package artifacts only with explicit permission.
- Generated artifacts are validated and surfaced in readiness evidence.
- Missing or failed Gerber/drill output blocks `ready`.
- Manifest and report artifacts include fabrication outputs.
- Default tests pass without KiCad.
- Optional real KiCad smoke test is available for local verification.
- README and roadmap describe the current state accurately.
