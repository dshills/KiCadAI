# Fabrication Export And Readiness Gates Implementation Plan

## Objective

Replace the export command stubs with deterministic fabrication preview, BOM,
and package-readiness behavior that prevents KiCadAI from overclaiming
manufacturability.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests independent of KiCad CLI, network access, and manufacturer
  services.
- Dry-run by default; require `--execute` for writes.
- Keep output paths inside the project root unless a future explicit package
  root policy is implemented.
- Do not mark current generated boards fabrication-ready unless all configured
  gates pass.
- Prefer structured issue output over prose-only failures.

## Phase 1: Readiness Model And Manifest

### Goal

Add internal fabrication readiness types and deterministic package manifest
serialization without CLI wiring.

### Work

- Add an internal package, likely `internal/fabrication`.
- Define:
  - readiness status: `blocked`, `candidate`, `ready`;
  - artifact kind/status;
  - evidence status;
  - package manifest;
  - readiness result;
  - export options.
- Add deterministic sorting for artifacts, issues, and evidence keys.
- Add helper functions for:
  - status calculation;
  - artifact path normalization;
  - manifest validation;
  - JSON serialization.
- Define standard artifact kinds:
  - `bom`;
  - `cpl`;
  - `manifest`;
  - `gerber`;
  - `drill`;
  - `erc`;
  - `drc`;
  - `readiness_report`.

### Tests

- Manifest serializes deterministically.
- Status calculation handles blocked, candidate, and ready-like mocked
  evidence.
- Artifact sorting is stable.
- Invalid manifest fields produce structured issues.

### Acceptance

- Fabrication readiness can be tested without reading or writing real projects.

### Commit

```text
Add fabrication readiness model
```

## Phase 2: Project Inspection And Gate Evaluation

### Goal

Evaluate a project directory or `.kicad_pro` into a readiness result using
existing project inspection, writer correctness, board validation, and
generated provenance evidence.

### Work

- Add a project evaluator in `internal/fabrication`.
- Accept target path and resolve project root/name.
- Detect:
  - generated manifest/provenance;
  - project file;
  - schematic file;
  - PCB file;
  - expected fabrication output directory.
- Integrate existing deterministic gates where available:
  - inspect project;
  - writer correctness;
  - board validation;
  - component/block readiness evidence if accessible.
- Model missing ERC/DRC/Gerber/drill evidence as blocking for `ready`, but not
  necessarily for preview/candidate.
- Return structured issues with stable paths and codes.

### Tests

- Missing project blocks.
- Generated minimal project preview produces a readiness result.
- Missing board/schematic/project files are reported.
- Missing ERC/DRC/Gerber/drill evidence prevents `ready`.
- Imported or unproven projects are preview-only.

### Acceptance

- A generated project can be evaluated for fabrication readiness without
  exporting files.

### Commit

```text
Add fabrication readiness evaluator
```

## Phase 3: BOM And CPL Report Generation

### Goal

Generate deterministic internal BOM and CPL/position reports from generated
project evidence where possible.

### Work

- Add BOM row model:
  - reference;
  - value;
  - quantity/group;
  - symbol ID;
  - footprint ID;
  - component ID;
  - manufacturer;
  - MPN;
  - confidence;
  - readiness notes.
- Add CPL/position row model:
  - reference;
  - footprint;
  - x/y;
  - rotation;
  - layer;
  - placement source;
  - fixed/movable.
- Source initial data from generated transactions/manifests/project files where
  available. If exact data is not yet available, emit structured missing-data
  issues rather than fabricating rows.
- Add CSV and JSON serialization helpers.
- Keep ordering deterministic by reference/group.

### Tests

- BOM rows sort deterministically.
- CPL rows sort deterministically.
- Missing component identity creates readiness issues.
- CSV escaping is correct.
- Empty or incomplete generated evidence blocks fabrication-ready status.

### Acceptance

- The fabrication package can include deterministic BOM/CPL reports or
  explicit missing-evidence issues.

### Commit

```text
Add fabrication BOM and CPL reports
```

## Phase 4: Export Service And Filesystem Safety

### Goal

Add a service that previews or writes fabrication package artifacts with safe
path and overwrite policy.

### Work

- Add `ExportPreview`, `ExportBOM`, and `ExportPackage` service functions.
- Implement default output paths:
  - `<project>/fabrication/readiness.json`;
  - `<project>/fabrication/bom.csv`;
  - `<project>/fabrication/cpl.csv`;
  - `<project>/fabrication/package-manifest.json`.
- Enforce:
  - dry-run by default;
  - `Execute` required for writes;
  - `Overwrite` required for existing package files;
  - output path inside project root.
- Return `reports.Artifact` records for expected and generated files.
- Write package manifest and readiness report deterministically.

### Tests

- Dry-run writes no files.
- Execute writes manifest/readiness report.
- BOM execute writes BOM when evidence is available or writes a report with
  structured missing-data issues when not.
- Existing files block without overwrite.
- Outside-root output paths block.

### Acceptance

- Export behavior is safe and testable without CLI wiring.

### Commit

```text
Add fabrication export service
```

## Phase 5: CLI Wiring

### Goal

Replace the `export` stubs with real structured commands.

### Work

- Wire:
  - `export preview <project>`;
  - `export bom <project>`;
  - `export fabrication <project>`.
- Require `--json` following existing structured-command behavior.
- Support existing global flags where applicable:
  - `--output`;
  - `--execute`;
  - `--overwrite`;
  - `--kicad-cli`;
  - `--require-drc`;
  - `--allow-missing-drc`;
  - `--keep-artifacts`;
  - `--artifact-dir`.
- Return standard `reports.Result` envelopes.
- Preserve current error shape for unsupported subcommands.
- Update help text.

### Tests

- `export preview` returns readiness result.
- `export bom` dry-run returns artifact expectations.
- `export fabrication` dry-run returns package manifest expectation.
- Execute writes files.
- Missing target and unsupported subcommand behavior remains structured.
- Old unsupported-stub test is replaced with real command tests.

### Acceptance

- The export command family is no longer a placeholder.

### Commit

```text
Wire fabrication export CLI
```

## Phase 6: KiCad CLI Policy Hook

### Goal

Model optional/required KiCad CLI artifact evidence for Gerber/drill/ERC/DRC
without requiring it in default tests.

### Work

- Add internal policy model:
  - `disabled`;
  - `optional`;
  - `required`.
- Reuse existing KiCad CLI path configuration where possible.
- When disabled:
  - Gerber/drill/ERC/DRC are expected or skipped based on command/readiness
    level;
  - `ready` remains blocked if those artifacts are required.
- When optional:
  - missing CLI creates warning evidence.
- When required:
  - missing or failed CLI creates blocking issues.
- Add seams for future actual Gerber/drill invocation.

### Tests

- Missing KiCad CLI optional mode warns.
- Missing KiCad CLI required mode blocks.
- Disabled mode is deterministic and does not invoke external tools.
- Existing ERC/DRC policy tests keep passing.

### Acceptance

- Fabrication readiness can represent external artifact proof without making
  default tests environment-dependent.

### Commit

```text
Add fabrication KiCad CLI policy
```

## Phase 7: Workflow Integration

### Goal

Connect fabrication readiness to `design create` acceptance and reports.

### Work

- When `AcceptanceFabricationCandidate` is requested, run fabrication preview
  after project write and validation stages when enough files exist.
- Set `Acceptance.FabricationReady` only from the fabrication readiness result.
- Attach fabrication readiness issues to workflow feedback.
- Do not run package writes from `design create` unless a future explicit export
  option is added.
- Preserve existing structural/connectivity/ERC-DRC acceptance behavior.

### Tests

- Fabrication-candidate request cannot claim ready without readiness evidence.
- Structural request does not run fabrication gate by default.
- Fabrication readiness issues appear in feedback for candidate requests.
- Existing design workflow tests keep passing.

### Acceptance

- `design create` cannot overclaim fabrication readiness.

### Commit

```text
Integrate fabrication readiness with design workflow
```

## Phase 8: Documentation And Roadmap

### Goal

Document the export/readiness commands and move the roadmap to the next
priority.

### Work

- Update README export section with:
  - preview;
  - BOM;
  - fabrication package;
  - dry-run/execute/overwrite behavior;
  - `candidate` vs `ready`;
  - KiCad CLI policy.
- Update `specs/ROADMAP.md`:
  - mark fabrication export/readiness gates as implemented foundation;
  - move next recommended priority to generated target transaction provenance
    or generated movable placement semantics, depending on implementation
    findings.
- Add implementation notes to this plan if behavior is narrowed.

### Tests

- `go test ./...`

### Acceptance

- Documentation matches implemented commands and current limitations.

### Commit

```text
Document fabrication readiness gates
```
