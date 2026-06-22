# Fabrication Export And Readiness Gates Specification

## 1. Purpose

Define and implement the first fabrication-readiness gate for KiCadAI projects.

KiCadAI can now generate KiCad projects, write schematics and PCBs, run writer
correctness checks, run structural board validation, optionally run KiCad
ERC/DRC, and report repair evidence. The next milestone is to prevent generated
projects from being described as manufacturable until the required fabrication
artifacts and evidence are present.

This project replaces the current `export preview|bom|fabrication` CLI stubs
with deterministic, structured behavior.

## 2. Goals

This project must:

- add a fabrication readiness model that explains whether a project is:
  - `blocked`;
  - `candidate`;
  - `ready`;
- add deterministic export/readiness reports for generated KiCad projects;
- add a package manifest for fabrication outputs and readiness evidence;
- implement `export preview`, `export bom`, and `export fabrication` as real
  structured commands;
- keep default tests independent of real KiCad CLI and external manufacturer
  services;
- integrate existing writer correctness, board validation, ERC/DRC evidence,
  component/block readiness, and generated provenance;
- support optional KiCad CLI artifact generation when explicitly configured;
- make missing Gerbers, drill files, BOM, CPL/position files, DRC, ERC, or
  project provenance visible as structured blocking issues;
- update `design create` so fabrication-candidate acceptance can reference the
  new readiness gate.

## 3. Non-Goals

This project does not:

- claim every generated board is fabrication-ready;
- implement natural-language planning;
- create manufacturer-specific outputs for every board house;
- require KiCad CLI in default tests;
- bypass existing writer correctness, board validation, ERC, or DRC gates;
- mutate imported projects;
- solve incomplete part selection or lifecycle/rating evidence;
- implement a visual Gerber viewer.

## 4. Current Baseline

Implemented foundations include:

- KiCad project, schematic, and PCB writers;
- generated project manifests and ownership checks;
- writer correctness gates;
- board validation gates;
- optional KiCad ERC/DRC runners and parsers;
- component and block readiness evidence;
- `design create` acceptance levels including `fabrication-candidate`;
- export command placeholders:
  - `export preview`;
  - `export bom`;
  - `export fabrication`.

Current gaps:

- export commands return `UNSUPPORTED_OPERATION`;
- no fabrication package manifest exists;
- no deterministic BOM or CPL report exists;
- no readiness score or issue taxonomy exists for fabrication;
- `fabrication_ready` is not tied to a package/evidence manifest;
- KiCad CLI-generated Gerber/drill artifacts are not integrated into a safe
  package workflow;
- missing manufacturer constraints are not represented as explicit issues.

## 5. Readiness Model

Add a fabrication readiness result with:

- `status`: `blocked`, `candidate`, or `ready`;
- `score`: integer or bounded percentage for user-facing progress, not a
  readiness substitute;
- `summary`:
  - generated provenance present;
  - schematic present;
  - PCB present;
  - writer correctness pass/fail;
  - board validation pass/fail;
  - ERC evidence status;
  - DRC evidence status;
  - BOM status;
  - CPL/position status;
  - Gerber status;
  - drill status;
  - manifest status;
  - component readiness status;
  - block readiness status;
- `issues`: structured blocking and warning issues;
- `artifacts`: generated or expected artifacts;
- `manifest_path`: package manifest path when written.

Status rules:

- `blocked`: any required gate is missing, failed, unsafe, or ambiguous.
- `candidate`: all internal deterministic gates pass, but optional external
  proof such as real KiCad DRC, manufacturer checks, or exact part lifecycle
  evidence is missing.
- `ready`: all required internal gates pass and configured external gates pass.

The first implementation should be conservative. It is acceptable for most
current generated boards to be `blocked` or `candidate`; it is not acceptable to
overclaim `ready`.

## 6. Fabrication Package Manifest

Add a manifest, for example:

```json
{
  "schema": "kicadai.fabrication.package.v1",
  "project": {
    "name": "led_indicator",
    "root": "."
  },
  "status": "blocked",
  "generated": true,
  "created_by": "kicadai",
  "artifacts": [
    {
      "kind": "bom",
      "path": "fabrication/led_indicator.bom.csv",
      "status": "generated"
    }
  ],
  "evidence": {
    "writer_correctness": "pass",
    "board_validation": "pass",
    "erc": "missing",
    "drc": "missing"
  },
  "issues": []
}
```

Manifest requirements:

- deterministic ordering;
- relative paths inside the package root;
- schema version;
- project identity;
- readiness status;
- artifact list;
- evidence summary;
- issues;
- command options used to generate the package;
- no absolute paths unless explicitly marked diagnostic-only.

## 7. Export Commands

### 7.1 `export preview <project>`

Purpose: inspect a project and report fabrication readiness without writing
fabrication artifacts.

Behavior:

- requires `--json`;
- accepts a project directory or `.kicad_pro`;
- detects generated provenance;
- runs lightweight deterministic readiness checks;
- returns readiness result, expected artifacts, and blocking issues;
- does not write files unless `--execute` is provided and a preview report is
  explicitly requested.

### 7.2 `export bom <project>`

Purpose: produce a deterministic BOM report from generated schematic/component
evidence where possible.

Behavior:

- dry-run by default;
- writes only with `--execute`;
- default output under `<project>/fabrication/`;
- refuses output outside project root unless an explicit package output root
  policy is added;
- includes reference, value, quantity, symbol, footprint, component ID,
  manufacturer, MPN, confidence, and readiness notes;
- blocks fabrication readiness when exact part identity is missing for parts
  that require it.

### 7.3 `export fabrication <project>`

Purpose: create or preview a complete fabrication package.

Behavior:

- dry-run by default;
- writes only with `--execute`;
- writes a package manifest;
- includes BOM and CPL/position reports from internal data where possible;
- includes expected Gerber/drill artifact records even when real KiCad CLI
  export is disabled;
- optionally invokes KiCad CLI to generate Gerbers/drill files when configured;
- runs readiness checks before declaring `candidate` or `ready`;
- requires `--overwrite` to replace an existing package manifest or artifact.

## 8. Artifact Types

The first package should model these artifacts:

- BOM CSV or JSON;
- CPL/position CSV or JSON;
- fabrication package manifest JSON;
- Gerber layer files, if KiCad CLI export is enabled;
- drill files, if KiCad CLI export is enabled;
- ERC report, if enabled;
- DRC report, if enabled;
- readiness report JSON.

Each artifact record should include:

- kind;
- path;
- status: `expected`, `generated`, `missing`, `skipped`, or `blocked`;
- required flag;
- generator: `kicadai`, `kicad-cli`, or `external`;
- issues.

## 9. Readiness Gates

Required gates for `candidate`:

- generated provenance or explicitly allowed imported-project preview mode;
- project file exists;
- schematic file exists;
- PCB file exists;
- writer correctness passes for project/schematic/PCB;
- board validation passes required outline, pad-net, route, and zone checks;
- all routed/generated nets have deterministic evidence;
- no missing footprint assignments;
- no missing pad summaries for connected parts;
- component/block readiness does not contain fabrication-blocking issues;
- BOM and CPL reports can be generated deterministically.

Required gates for `ready`:

- all `candidate` gates pass;
- configured ERC passes;
- configured DRC passes;
- Gerbers and drill files are generated and accounted for;
- required manufacturer profile constraints pass if a profile is configured;
- exact part evidence is sufficient for every placed component.

## 10. KiCad CLI Policy

Default tests must not require KiCad CLI.

The implementation should support three modes:

- `disabled`: do not invoke KiCad CLI; report Gerber/drill/ERC/DRC as missing
  or skipped depending on readiness level.
- `optional`: invoke KiCad CLI if configured; missing CLI is warning evidence.
- `required`: missing or failing KiCad CLI is blocking.

Flags may reuse existing patterns:

- `--kicad-cli`;
- `--require-drc`;
- `--allow-missing-drc`;
- future `--require-fabrication-artifacts`.

The first implementation can model Gerber/drill artifacts without invoking
KiCad CLI, as long as readiness does not overclaim generated artifacts.

## 11. Safety Requirements

- Dry-run by default for all export commands.
- `--execute` required for filesystem writes.
- `--overwrite` required for replacing existing package files.
- Default output paths stay inside the project root.
- Imported projects are preview-only unless preservation/ownership is proven.
- Missing evidence produces structured issues, not guessed readiness.
- Package writes must be deterministic and testable.

## 12. Test Requirements

Required tests:

- readiness status calculation for blocked, candidate, and ready-like mocked
  evidence;
- manifest deterministic serialization;
- output path safety;
- dry-run writes no files;
- execute writes manifest/BOM/CPL reports;
- overwrite gate blocks existing files;
- generated project preview returns structured missing-artifact issues;
- export stubs are replaced with real command results;
- CLI selected-field tests for:
  - `export preview`;
  - `export bom`;
  - `export fabrication`;
- optional KiCad CLI unavailable behavior is warning or blocking based on
  policy;
- `go test ./...` passes without KiCad CLI.

Optional integration tests:

- real KiCad CLI Gerber/drill generation when `KICADAI_KICAD_CLI` is set;
- real ERC/DRC evidence attached to the package manifest;
- comparison against checked-in tiny generated board fixtures.

## 13. Documentation Requirements

Update:

- README export section;
- ROADMAP current status and next priority;
- docs explaining that `candidate` is not `ready`;
- examples showing dry-run and execute export commands.

## 14. Acceptance Gates

This project is complete when:

- `export preview`, `export bom`, and `export fabrication` no longer return
  unsupported stubs;
- generated projects produce deterministic readiness results;
- dry-run and execute behavior is tested;
- package manifests are deterministic and path-safe;
- missing Gerber/drill/ERC/DRC/part evidence blocks `ready`;
- README and ROADMAP identify the next priority after fabrication gates;
- `go test ./...` passes without KiCad CLI.
