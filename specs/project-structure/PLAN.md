# KiCad Project Structure Compatibility Implementation Plan

## Overview

This plan implements the KiCad project structure compatibility specification in incremental phases. Each phase should be implemented, tested, reviewed, and committed before moving to the next phase.

The work starts by removing the mismatch between KiCad's real directory conventions and KiCadAI's current stricter output rule, then adds project-local library tables, hierarchical schematic file emission, KiCad-shaped project JSON, and optional artifacts.

## Phase 0: Demo Corpus Baseline

### Objective

Turn the manual review of an external local KiCad demo project collection into documented, testable structure expectations without copying demo source files into the repository.

### Tasks

1. Add a short fixture note under `testdata` or `docs` describing observed demo structures:
   - matching directory/project basename examples
   - mismatched directory/project basename examples
   - projects with `sym-lib-table`
   - projects with `fp-lib-table`
   - projects with hierarchical sheets
   - projects with `.kicad_dru`
   - projects with `.kicad_wks`
2. Add a small internal test helper that builds demo-like structural expectations from in-repo synthetic data.
3. Record the expected top-level `.kicad_pro` section names from a representative modern demo:
   - `board`
   - `boards`
   - `component_class_settings`
   - `cvpcb`
   - `erc`
   - `libraries`
   - `meta`
   - `net_settings`
   - `pcbnew`
   - `schematic`
   - `sheets`
   - `text_variables`
   - `time_domain_parameters`
4. Document that external demo files are review inputs, not repository fixtures.

### Deliverables

- Demo-structure notes.
- Synthetic structure expectations usable by tests.

### Tests

- No behavioral tests required beyond compiling any added helpers.

### Done When

- The repo documents the observed KiCad project structure expectations and does not depend on files outside the repo for normal tests.

## Phase 1: Decouple Output Directory Name from Project Basename

### Objective

Allow generated projects to be written into directories whose basename differs from the KiCad project basename.

### Tasks

1. Remove the `WriteProjectDirectory` requirement that `filepath.Base(root) == design.Name`.
2. Keep validation that `design.Name` is a safe filename component.
3. Keep validation that the target path is a valid name for a new or existing directory, is not `.`, and has a writable parent when the writer reaches the filesystem phase.
4. Update overwrite journal naming so it remains based on the target directory basename.
5. Update tests:
   - replace the rejection test with an acceptance test for mismatched target directory and project basename
   - preserve tests for unsafe project names
   - preserve tests for existing target refusal and overwrite behavior
6. Update documentation to explain that the directory path and KiCad project basename are separate concepts.

### Deliverables

- Updated directory writer validation.
- Updated tests and docs.

### Tests

```text
go test ./...
```

### Done When

- `WriteProjectDirectory("/tmp/my-folder", design.Name == "sensor_board")` writes `sensor_board.kicad_pro`, `sensor_board.kicad_sch`, and optional `sensor_board.kicad_pcb` inside `/tmp/my-folder`.

## Phase 2: Generated Path Registry and Project File Inventory

### Objective

Create a central inventory of all files the project writer intends to emit, so future phases can safely add subdirectories, library tables, sheet files, and optional artifacts.

### Tasks

1. Introduce an internal generated-file descriptor:

```go
type generatedFile struct {
    Path string
    Mode os.FileMode
    Write func(context.Context, io.Writer) error
}
```

`Write` functions must be repeatable. If a generated file is backed by an `io.Reader`, the descriptor must use a provider/factory internally so retries or multi-pass validation do not reuse an already-exhausted reader.

Large-file writers must honor cancellation by checking `ctx.Err()` before and during streaming writes where practical.

Any writer that opens an `io.ReadCloser` must close it after the write attempt, including error paths.

2. Build a generated file list before writing any files.
3. Validate all generated paths:
   - relative path only
   - convert all incoming separators to forward slashes before validation
   - reject backslash traversal strings before any filesystem join
   - reject `path.IsAbs` paths
   - reject any cleaned path equal to `..` or starting with `../`
   - normalized with `path.Clean` for KiCad/internal forward-slash paths and checked with `filepath.Clean` after joining to the target root for filesystem writes
   - cleaned path does not escape the project directory
   - no absolute paths
   - no `..` traversal
   - no duplicate paths after normalization
   - no case-insensitive duplicate paths after normalization
   - no file path collides with a generated directory path
   - no path component contains null bytes or filesystem-reserved characters such as `:`, `*`, `?`, `"`, `<`, `>`, or `|`
   - no path component is a reserved Windows device filename
   - no empty path
4. Refactor `writeDesignFiles` to consume the generated file list.
5. Make the inventory writer create required parent directories for any generated path.
6. Preserve deterministic ordering of written files.
7. Ensure generated text KiCad files keep LF line endings on every platform while binary or opaque asset files are written byte-for-byte.
8. Add tests for duplicate paths, case-insensitive duplicate paths, traversal paths, and LF line endings.

### Deliverables

- File inventory builder.
- Shared relative path validation.
- Refactored writer using the inventory.

### Tests

```text
go test ./...
```

### Done When

- The writer can describe all core files before creating the temporary project directory, and invalid future file paths fail before any output is written.

## Phase 3: Library Table Model and Writers

### Objective

Support `sym-lib-table` and `fp-lib-table` generation using structured S-expression writers.

### Tasks

1. Add a library table package, for example:

```text
internal/kicadfiles/library
```

2. Define:

```go
type TableEntry struct {
    Name string
    Type string
    URI string
    Options string
    Description string
}
```

3. Implement `WriteSymbolLibraryTable(io.Writer, []TableEntry)`.
4. Implement `WriteFootprintLibraryTable(io.Writer, []TableEntry)`.
5. Validate:
   - non-empty unique names
   - non-empty type, defaulting to `KiCad` at construction sites where appropriate
   - non-empty URI
   - safe project-local URI patterns when generated by KiCadAI
6. Extend `design.Design` with symbol and footprint table entries.
7. Add `sym-lib-table` to the generated file inventory when symbol table entries exist.
8. Add `fp-lib-table` to the generated file inventory when footprint table entries exist.
9. Add golden tests based on small demo-style tables.
10. Update docs.

### Deliverables

- Library table data model.
- Symbol and footprint library table writers.
- Directory writer support for table files.

### Tests

```text
go test ./...
```

### Done When

- A design with one local symbol table entry emits a valid `sym-lib-table`.
- A design with one local footprint table entry emits a valid `fp-lib-table`.
- Designs without entries preserve the current minimal output.

## Phase 4: Library Reference Validation

### Objective

Validate that generated schematic and board references are consistent with embedded definitions, local tables, or declared external library dependencies.

### Tasks

1. Add helper parsing for KiCad library identifiers, using KiCad's `lib_id` terminology in new code:
   - `library_nickname:item_name`
   - reject empty nickname when table resolution is required
   - reject empty item name
2. Validate schematic symbol `LibraryID` values:
   - embedded symbols satisfy matching IDs
   - table entries satisfy matching nicknames
   - declared known external libraries satisfy matching nicknames
3. Validate PCB footprint `LibraryID` values:
   - full footprint data may satisfy references for generated inline footprints
   - table entries satisfy matching nicknames
   - declared known external libraries satisfy matching nicknames
4. Decide and document how standard KiCad libraries are represented in the design model.
5. Add tests:
   - unresolved local symbol library fails
   - unresolved local footprint library fails
   - embedded symbol passes without `sym-lib-table`
   - declared external library passes

### Deliverables

- Library identifier parser.
- Design-level reference validation.
- Tests for valid and invalid references.

### Tests

```text
go test ./...
```

### Done When

- The writer refuses designs whose generated symbols or footprints reference missing project-local libraries.

## Phase 5: Hierarchical Sheet Rendering

### Objective

Render sheet nodes in `.kicad_sch` files and validate parent-to-child sheet relationships.

### Tasks

1. Extend `schematic.Sheet` to include required rendering data:
   - UUID
   - name
   - filename
   - position
   - size
   - optional hierarchical pins
2. Implement sheet node rendering in the schematic writer.
3. Add validation for:
   - non-empty sheet name
   - non-empty sheet filename
   - safe relative sheet filename
   - valid UUID
   - positive size when size is represented
4. Add golden tests for a root schematic containing one sheet.
5. Add tests for invalid sheet filenames.
6. Ensure generated output ordering matches KiCad-style ordering.

### Deliverables

- Sheet model updates.
- Sheet S-expression rendering.
- Validation and golden tests.

### Tests

```text
go test ./...
```

### Done When

- A root schematic can render a KiCad-style sheet reference with `Sheetname` and `Sheetfile` properties.

## Phase 6: Child Schematic File Emission

### Objective

Emit every child schematic file referenced by root or parent sheets.

### Tasks

1. Extend the design model with child schematic files, preserving root schematic as a separate field.
2. Give every schematic file an explicit relative filename.
3. Validate:
   - every parent `Sheetfile` resolves to a child schematic file
   - multiple parent sheet instances may reuse the same child schematic file
   - the sheet graph has no cycles
   - every child schematic file is referenced by at least one parent unless explicitly marked standalone
   - distinct child schematic definitions cannot claim the same filename
   - multiple sheet references to the same child schematic file object are allowed and produce one inventory entry
   - child paths cannot escape the project directory
4. Add child schematic files to the generated file inventory.
5. Ensure required child sheet subdirectories are created while writing the temporary project.
6. Add tests for:
   - root plus one child in project root
   - root plus one child under `sch/`
   - missing child file rejection
   - duplicate child path rejection
   - traversal path rejection

### Deliverables

- Child schematic file support.
- Directory writer support for subdirectories.
- Validation tests.

### Tests

```text
go test ./...
```

### Done When

- A hierarchical project writes root and child `.kicad_sch` files whose sheet references resolve inside the project directory.

## Phase 7: KiCad-Shaped Project JSON

### Objective

Expand `.kicad_pro` generation from a minimal JSON document to a KiCad-shaped project document with the top-level sections observed in modern demo projects.

### Tasks

1. Add a KiCad project JSON fixture or synthetic golden baseline.
2. Extend the project document model with:
   - `board`
   - `boards`
   - `component_class_settings`
   - `cvpcb`
   - `erc`
   - `libraries`
   - `meta`
   - `net_settings`
   - `pcbnew`
   - `schematic`
   - `sheets`
   - `text_variables`
   - `time_domain_parameters`
3. Preserve existing project fields by mapping them into the expanded document.
4. Populate the `sheets` array from the root and child schematic UUID/name pairs.
5. Ensure all sheet UUIDs are deterministic, derived from stable generation inputs such as project name and relative schematic path.
6. Add stable default objects for sections not yet fully modeled.
7. Add tests that assert required top-level keys are present.
8. Add golden tests for a minimal project and a hierarchical project.
9. Update docs to describe modeled versus defaulted project settings.

### Deliverables

- Expanded project JSON model.
- Project sheet list support.
- Golden tests.

### Tests

```text
go test ./...
```

### Done When

- Generated `.kicad_pro` files include the modern KiCad top-level section set and remain deterministic.

## Phase 8: Optional Rule, Worksheet, and Asset Files

### Objective

Support optional project artifacts observed in demo projects without making them mandatory for simple generated projects.

### Tasks

1. Add model types for text artifacts:

```go
type TextArtifact struct {
    Path string
    Open func() (io.ReadCloser, error)
}
```

2. Add typed wrappers or fields for:
   - `.kicad_dru`
   - `.kicad_wks`
   - text asset files
3. Validate allowed extensions for rule and worksheet files.
4. Validate all artifact paths through the generated path registry.
5. Add artifact files to the generated file inventory.
6. Add tests for:
   - emitted `.kicad_dru`
   - emitted `.kicad_wks`
   - invalid extension rejection
   - duplicate path rejection against core files
7. Document binary asset support as deferred unless implemented.

### Deliverables

- Optional text artifact support.
- Validation and writer integration.
- Tests and docs.

### Tests

```text
go test ./...
```

### Done When

- Generated projects can include `.kicad_dru` and `.kicad_wks` files when requested.

## Phase 9: Demo-Like Acceptance Fixtures

### Objective

Add generated in-repo acceptance fixtures that exercise the full compatible directory structure without copying external demo files.

### Tasks

1. Add a generated demo-like design fixture containing:
   - mismatched output directory and project basename
   - root schematic
   - child schematic under `sch/`
   - board file
   - `sym-lib-table`
   - `fp-lib-table`
   - optional `.kicad_dru`
   - optional `.kicad_wks`
2. Add an integration-style test that writes the fixture to a temporary directory.
3. Assert the final directory tree exactly.
4. Assert project JSON contains required top-level keys.
5. Assert every sheet reference resolves to an emitted child file.
6. Assert library table files contain expected entries.
7. Keep the current LED indicator acceptance fixture passing.

### Deliverables

- Demo-like generated acceptance fixture.
- Directory tree assertions.
- End-to-end temporary-directory write test.

### Tests

```text
go test ./...
```

### Done When

- The test suite proves KiCadAI can generate both the current minimal project and a demo-like project structure.

## Phase 10: Documentation and Command-Line Interface Polish

### Objective

Expose the improved project structure behavior clearly to users and future contributors.

### Tasks

1. Update `docs/kicad-file-writers.md` with:
   - project directory versus project basename behavior
   - emitted file inventory
   - library table behavior
   - hierarchical sheet behavior
   - optional artifact behavior
2. Update command-line help if needed.
3. Consider adding explicit flags:
   - `--output-dir`
   - `--project-name`
4. Add examples:
   - minimal LED project
   - hierarchical project
   - project-local library table project
5. Document remaining limitations:
   - no lossless edit support
   - limited local symbol library generation if still deferred
   - limited footprint module generation if still deferred
   - optional KiCad graphical validation is manual or opt-in

### Deliverables

- Updated docs.
- Updated command-line help or examples where applicable.

### Tests

```text
go test ./...
```

### Done When

- Users can understand how to generate a KiCad-compatible project directory and what file types are currently supported.

## Review and Commit Process

For each implementation phase:

1. Make the scoped changes for that phase only.
2. Run `gofmt` on changed Go files.
3. Run `go test ./...`.
4. Stage only relevant files.
5. Run Prism review on staged changes.
6. Address review findings or document why no change is needed.
7. Commit with a phase-specific message.
8. Move to the next phase only after the commit succeeds.

## Risk Register

| Risk | Impact | Mitigation |
|---|---|---|
| KiCad accepts minimal project JSON today but changes behavior later | Generated projects may open with warnings or lose settings | Generate KiCad-shaped defaults and maintain golden fixtures |
| Library resolution rules differ between standard, embedded, and project-local libraries | Schematics or boards may open with missing symbols or footprints | Add explicit table/reference validation and acceptance fixtures |
| Hierarchical sheet paths can escape the project directory | Unsafe writes or broken projects | Central generated path registry and traversal tests |
| Optional artifacts collide with generated core files | Corrupt output | Duplicate path detection before writing |
| Binary assets need different write handling than text artifacts | 3D models may not be supported initially | Start with text artifacts, then add binary-safe asset support explicitly |
| Demo files are outside the repo and may change | Tests become environment-dependent | Use demos only as discovery input; keep tests synthetic and in-repo |

## Final Acceptance Checklist

- [ ] Output directory basename may differ from project basename.
- [ ] Core project trio behavior is preserved.
- [ ] `sym-lib-table` writer is implemented.
- [ ] `fp-lib-table` writer is implemented.
- [ ] Library references are validated.
- [ ] Sheet nodes render in schematics.
- [ ] Child sheet files are emitted.
- [ ] `.kicad_pro` includes KiCad-shaped top-level sections.
- [ ] Optional `.kicad_dru` and `.kicad_wks` files can be emitted.
- [ ] Demo-like acceptance fixture passes.
- [ ] Documentation is updated.
- [ ] `go test ./...` passes.
