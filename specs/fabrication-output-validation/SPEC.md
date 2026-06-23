# Fabrication Output Validation Specification

## Objective

Make fabrication package readiness depend on generated and validated
manufacturing artifacts, not only project parseability, board validation, and
metadata previews.

The next concrete milestone is:

```text
generated KiCad project
  -> writer and board validation
  -> optional KiCad ERC/DRC evidence
  -> Gerber generation
  -> drill generation
  -> artifact validation
  -> package manifest
  -> readiness status
```

KiCadAI should be able to run `export fabrication` against a generated project
and, when a configured KiCad CLI is available and the project passes required
checks, produce deterministic Gerber and drill artifact evidence under the
fabrication package directory.

This project does not make arbitrary boards fabrication-ready. It closes the
current packaging gap: KiCadAI already evaluates fabrication readiness and emits
BOM/CPL/readiness/manifest files, but it does not yet generate or validate the
Gerber and drill files required for a complete manufacturer-release package.

## Current Foundation

The repository already has:

- `internal/fabrication` readiness models:
  - `ReadinessStatus`: `blocked`, `candidate`, `ready`;
  - artifact kinds for `bom`, `cpl`, `gerber`, `drill`, `erc`, `drc`,
    `manifest`, and `readiness_report`;
  - evidence statuses for pass, warning, missing, skipped, and fail;
  - package manifest serialization and validation.
- `export preview`, `export bom`, and `export fabrication` CLI paths.
- Safe dry-run, execute, overwrite, and output-directory handling.
- Deterministic BOM/CPL report generation where source data is available.
- Writer correctness and board validation gates.
- Optional or required KiCad CLI evidence policy through `--kicad-cli`,
  `--allow-missing-drc`, and `--require-drc`.
- `design create` fabrication-candidate integration that downgrades achieved
  acceptance when fabrication readiness is partial or blocked.
- Artifact report plumbing through `internal/reports`.

The main missing piece is an executable fabrication-artifact generator and
validator for Gerber and drill outputs.

## Scope

### In Scope

- Add a fabrication plotting service that can invoke KiCad CLI to generate:
  - Gerber plots;
  - Excellon drill files;
  - optional KiCad-generated job/report files when KiCad emits them.
- Keep plotting behind explicit execution:
  - dry-run reports expected actions and paths;
  - `--execute` is required to write generated artifacts;
  - `--overwrite` is required to replace existing generated artifact
    directories or files.
- Support deterministic output directories under the package root:
  - `fabrication/gerbers/`;
  - `fabrication/drill/`.
- Validate generated fabrication artifacts:
  - required directories exist;
  - at least one copper Gerber exists for each PCB copper layer present;
  - `Edge.Cuts` output exists;
  - solder mask and paste outputs are checked when expected for the board;
  - drill output exists when the board has through-hole pads or vias;
  - files are non-empty;
  - file names are normalized relative paths inside the package;
  - artifact manifests include generated paths and validation issues.
- Integrate Gerber/drill evidence into:
  - `fabrication.Result.Summary`;
  - `fabrication.Manifest`;
  - `fabrication.Result.Artifacts`;
  - readiness scoring and status.
- Make required-fabrication-output failures blocking when a caller asks for a
  fabrication package:
  - missing Gerber output blocks `ready`;
  - missing drill output blocks `ready` when drilled features exist;
  - failed KiCad plotting blocks `ready`;
  - unsafe output path or overwrite conflicts block export.
- Keep default tests hermetic:
  - no real KiCad install required;
  - no network access;
  - fake KiCad CLI runner with deterministic files and failures.
- Add optional real KiCad smoke test support gated by environment/configuration
  or existing KiCad CLI flags, not required in CI.
- Update README and roadmap to describe the new generated artifact behavior and
  the remaining limits.

### Out Of Scope

- Manufacturer-specific fabrication profile support.
- Zip/package upload generation.
- Panelization.
- Pick-and-place rotation normalization beyond existing CPL output.
- IPC-based plotting.
- Guaranteeing any manufacturer will accept the output.
- Full DFM analysis for annular ring, solder slivers, impedance, creepage,
  copper balance, silkscreen over pads, paste reductions, or assembly notes.
- Making imported-project mutation safe beyond existing preservation rules.

## User-Facing Behavior

### Dry Run

`export fabrication` remains dry-run by default:

```sh
go run ./cmd/kicadai --json --kicad-cli /path/to/kicad-cli export fabrication ./project
```

Expected behavior:

- no files are written;
- result includes expected package artifacts;
- Gerber/drill artifacts are reported as expected or missing depending on
  existing evidence;
- if `--kicad-cli` is configured, the result describes plotting actions that
  would run;
- readiness remains `candidate` or `blocked` unless all required evidence
  already exists.

### Execute

Writing fabrication outputs requires `--execute`:

```sh
go run ./cmd/kicadai \
  --json \
  --execute \
  --overwrite \
  --kicad-cli /path/to/kicad-cli \
  export fabrication ./project
```

Expected outputs under `<project>/fabrication/`:

```text
readiness.json
package-manifest.json
bom.csv
cpl.csv
gerbers/
drill/
```

The command should:

- run KiCad CLI plotting commands;
- validate generated outputs;
- write BOM/CPL/readiness/manifest;
- mark Gerber/drill artifacts `generated` when created and validated;
- attach artifact-level issues when output exists but fails validation;
- return `ready` only when all modeled required evidence passes.

### Existing Artifact Reuse

If Gerber/drill artifacts already exist:

- dry-run may use them as evidence;
- execute without `--overwrite` must not clobber them;
- execute with `--overwrite` may replace them after safe path checks;
- externally supplied files should be marked as external evidence unless
  KiCadAI generated them during the current export.

### Missing KiCad CLI

When no `--kicad-cli` is provided:

- no plotting is attempted;
- Gerber/drill evidence remains missing or skipped;
- readiness cannot become `ready`;
- the result must explain that fabrication output generation requires KiCad CLI.

When `--require-drc` is set:

- missing required external KiCad evidence remains blocking;
- plotting failures are blocking.

## KiCad CLI Command Contract

The implementation should isolate KiCad CLI invocation behind an interface so
tests can use a fake runner.

Required runner capabilities:

- receive a project root and PCB path;
- receive output directories for Gerber and drill files;
- run commands with context cancellation;
- return stdout, stderr, exit code, generated file paths, and command metadata;
- never expose shell interpolation to user-controlled paths.

Expected KiCad CLI operations:

- plot Gerber output from the project PCB;
- export drill output from the project PCB.

The exact command-line syntax may vary by KiCad version and should be centralized
in one adapter. The adapter should prefer explicit argv execution over shell
strings.

The output evidence must record:

- command kind: `gerber` or `drill`;
- KiCad CLI path;
- KiCad version if available;
- output directory;
- generated relative paths;
- exit status;
- stderr/stdout snippets when failures occur.

## Artifact Validation Rules

### Required Gerber Evidence

Gerber validation should inspect the generated output directory and the PCB
model, then require:

- `F.Cu` Gerber for any board with top copper;
- `B.Cu` Gerber for any board with bottom copper;
- all modeled internal copper layer Gerbers when internal copper layers are
  supported by the writer/reader;
- `Edge.Cuts` Gerber for any board outline;
- files are non-empty;
- paths are relative to the package directory;
- paths do not escape the package directory.

Mask, paste, and silkscreen files should be checked as warning-level evidence
until layer expectation support is complete. Missing copper or edge cuts is
blocking.

### Required Drill Evidence

Drill validation should inspect the PCB model for drilled features:

- through-hole pads;
- vias;
- mounting holes if modeled as drilled pads.

If no drilled features exist, drill output may pass with a skipped/no-drills
status. If drilled features exist, the package must include non-empty drill
output. Drill map/report files are useful but not required for initial
readiness.

### Failure Classification

Validation should produce structured issues with stable paths and categories:

- `fabrication.gerber.missing`;
- `fabrication.gerber.empty`;
- `fabrication.gerber.missing_layer`;
- `fabrication.gerber.missing_edge_cuts`;
- `fabrication.drill.missing`;
- `fabrication.drill.empty`;
- `fabrication.kicad_cli.unavailable`;
- `fabrication.kicad_cli.failed`;
- `fabrication.output.overwrite_required`;
- `fabrication.output.path_escape`.

Existing issue code enums may be reused, but messages and paths must be stable
for tests and AI callers.

## Data Model Changes

Add or extend fabrication models with:

- plot request/options:
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
- plot result:
  - attempted;
  - skipped reason;
  - generated artifact paths;
  - command evidence;
  - issues.
- artifact validation result:
  - artifact kind;
  - evidence status;
  - artifact status;
  - expected layer/file evidence;
  - generated/external files;
  - issues.

The manifest should remain backward-compatible with
`kicadai.fabrication.package.v1` unless a breaking schema change is required.
If new fields fit existing artifact/evidence structures, keep the schema version
unchanged.

## Readiness Semantics

Readiness must stay conservative:

- `ready` requires pass evidence for project, schematic, PCB, writer
  correctness, board validation, manifest, BOM, CPL, Gerber, drill,
  component readiness, block readiness, and any required KiCad external checks.
- `candidate` means no blocking issue exists, but some evidence is warning,
  missing, or skipped.
- `blocked` means any blocking issue or failed required evidence exists.

Gerber/drill generation should raise confidence only when validation passes.
Generated file existence alone is not sufficient.

## Testing Strategy

Required default tests:

- fake KiCad CLI produces deterministic Gerber and drill files;
- dry-run does not write files;
- execute writes Gerber/drill directories and metadata;
- overwrite is required for existing generated directories/files;
- missing KiCad CLI reports missing/skipped evidence;
- fake KiCad CLI failure blocks readiness;
- missing copper Gerber blocks readiness;
- missing `Edge.Cuts` blocks readiness;
- missing drill output blocks readiness when drilled features exist;
- drill output is skipped/pass-like when no drilled features exist;
- package manifest includes Gerber/drill artifacts and issue evidence;
- CLI JSON includes generated artifact paths and stable evidence statuses;
- `design create` fabrication-candidate flow consumes generated fabrication
  evidence when export execution is enabled in that path, or remains dry-run
  when not explicitly executed.

Optional local tests:

- run a smoke project through real `kicad-cli`;
- validate generated files are non-empty and named as expected;
- parse KiCad CLI failures into stable issues.

## Documentation Requirements

Update:

- README fabrication export section;
- CLI examples;
- roadmap near-term sequence;
- any fabrication readiness caveat that says KiCadAI does not generate Gerber
  or drill files.

Docs must clearly say:

- KiCad CLI is required for generated fabrication artifacts;
- export remains dry-run by default;
- generated artifacts are validated but not a manufacturer acceptance guarantee;
- missing or failed Gerber/drill evidence blocks `ready`.

## Risks

### KiCad CLI Version Differences

Risk: command syntax or output names differ by KiCad version.

Mitigation:

- isolate CLI command construction;
- capture version evidence when possible;
- validate by file content/presence patterns, not exact filenames alone;
- keep fake-runner tests deterministic and add optional local smoke coverage.

### Overclaiming Readiness

Risk: generated Gerbers and drill files could make the project report `ready`
while other manufacturing requirements remain unchecked.

Mitigation:

- keep readiness gates explicit;
- document unsupported DFM checks;
- preserve candidate/blocked status when evidence is missing or warning;
- do not add manufacturer acceptance claims.

### Destructive Output Handling

Risk: export overwrites user-supplied fabrication files.

Mitigation:

- preserve dry-run default;
- require `--execute`;
- require `--overwrite` for existing files/directories;
- enforce package-root path containment.

### Flaky Tests

Risk: tests depend on local KiCad installs.

Mitigation:

- use fake runners for default tests;
- gate real KiCad smoke tests behind explicit configuration.

## Acceptance Criteria

- `export fabrication --execute --overwrite --kicad-cli ...` can generate
  Gerber and drill artifact directories for a supported generated PCB.
- Generated Gerber/drill artifacts are validated and reflected in
  `readiness.json`.
- `package-manifest.json` includes generated Gerber/drill artifacts with stable
  relative paths.
- Missing or failed Gerber/drill output prevents `ready`.
- Default test suite passes without KiCad installed.
- README and roadmap no longer describe Gerber/drill generation as completely
  absent; they describe it as KiCad-CLI-backed and evidence-gated.
