# KiCad Project Structure Compatibility Specification

## Purpose

Align KiCadAI's generated project directories with the structure observed in real KiCad demo projects. The direct file writers already generate the core project, schematic, and board files; this specification defines the next compatibility layer required for KiCad-style directory layouts, project-local library references, hierarchical schematics, KiCad-shaped project metadata, and optional project artifacts.

This work supports the larger goal of generating schematics and printed circuit boards through artificial-intelligence-assisted workflows without relying on live KiCad write application programming interface commands.

## Background

Review of an external local KiCad demo project collection showed that simple KiCad projects commonly contain a root project file trio:

```text
<project>.kicad_pro
<project>.kicad_sch
<project>.kicad_pcb
```

Richer projects also commonly contain:

- `sym-lib-table`
- `fp-lib-table`
- project-local `.kicad_sym` libraries
- project-local `.pretty/` footprint libraries
- project-local 3D model directories such as `.3dshapes/`, `3d_shapes/`, `3dmodels/`, or `lib/3dmodels/`
- additional hierarchical `.kicad_sch` files in the project root or subdirectories
- optional `.kicad_dru` design rule files
- optional `.kicad_wks` worksheet files

The demo corpus also shows that the project directory name is not required to match the project file basename. Examples include directories such as `cm5_minima/` containing `CM5_MINIMA_3.kicad_pro`, `openair-max/` containing `One-Air-Max.kicad_pro`, and `royalblue54L_feather/` containing `RoyalBlue54L-Feather.kicad_pro`.

## Goals

1. Preserve the minimal project trio for simple generated projects.
2. Allow output directories whose basename differs from the KiCad project basename.
3. Generate project-local symbol and footprint library tables when libraries are referenced.
4. Support hierarchical schematic references and emit every referenced child schematic file.
5. Generate `.kicad_pro` JSON with KiCad-shaped top-level sections and sensible defaults.
6. Add explicit extension points for optional project artifacts such as custom design rules, worksheets, local symbol libraries, local footprint libraries, and 3D model directories.
7. Keep output deterministic and suitable for golden-file tests.
8. Validate the project directory graph before writing any file.
9. Preserve atomic project-directory write behavior.
10. Maintain backward compatibility for the current generated light-emitting diode indicator project.
11. Use LF line endings for all generated text KiCad files on every platform.

## Non-Goals

- Lossless parsing or editing of arbitrary existing KiCad projects.
- Copying demo project source files into this repository.
- Generating a complete replacement for KiCad's global symbol and footprint libraries.
- Implementing autorouting.
- Running KiCad graphical user interface workflows as part of normal unit tests.
- Guaranteeing compatibility with every historical KiCad project format.
- Mutating live KiCad editor state through inter-process communication write calls.

## Compatibility Target

The compatibility target is KiCad project structure as observed in KiCad demo projects using modern `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb` formats. The implementation should default to the active local KiCad-compatible format already used by `internal/kicadfiles`.

Where KiCad accepts optional files, KiCadAI must emit those files only when the generated design model needs them. The writer must not create empty local library directories or optional artifacts unless they are referenced by generated content or explicitly requested by the design model.

## Required Directory Semantics

### Project Directory

The output root passed to the writer is the project directory. It may have any filesystem-safe basename independent of the KiCad project basename.

Example:

```text
generated/board-demo/
  sensor_frontend.kicad_pro
  sensor_frontend.kicad_sch
  sensor_frontend.kicad_pcb
```

The writer must continue to reject unsafe target paths, reserved Windows filenames for generated file basenames, and traversal through generated relative paths.

### Project Basename

The KiCad project basename comes from the design model. It controls the names of the root `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb` files.

The design basename must be a single safe filename component. Spaces, underscores, hyphens, periods inside the name, and mixed case are valid when the name does not end in a space or period and does not use reserved Windows device names.

### Root Files

For a schematic-only project:

```text
<project>.kicad_pro
<project>.kicad_sch
```

For a schematic and board project:

```text
<project>.kicad_pro
<project>.kicad_sch
<project>.kicad_pcb
```

The writer must continue to omit `.kicad_pcb` when the design has no board.

## Project Model Extensions

The design model must be extended to represent all project files that the directory writer can emit.

```go
type Design struct {
    Name          string
    Project       project.ProjectFile
    Schematic     *schematic.SchematicFile
    SheetFiles    []*schematic.SchematicFile
    PCB           *pcb.PCBFile
    SymbolTables  []library.TableEntry
    FootprintTables []library.TableEntry
    LocalSymbolLibraries []library.SymbolLibraryFile
    LocalFootprintLibraries []library.FootprintLibrary
    RuleFiles     []project.RuleFile
    WorksheetFiles []project.WorksheetFile
    AssetFiles    []project.AssetFile
    ExpectedNets  []string
}
```

The exact package names may differ, but the model must support these concepts:

- root project file
- root schematic file
- zero or more child schematic files
- optional board file
- symbol library table entries
- footprint library table entries
- optional local symbol library files
- optional local footprint library directories and modules
- optional design rule files
- optional worksheet files
- optional asset files, especially 3D models

Asset files may be large or binary. The asset model must support streaming content through `io.Reader` or an equivalent lazy source instead of requiring all asset bytes to be held in memory at once. `project.AssetFile` must expose an `Open() (io.ReadCloser, error)` style source or equivalent interface.

`SheetFiles` is a flat registry of unique child schematic files on disk. Hierarchical relationships are still expressed by `Sheetfile` references inside the root and child schematic files.

## Library Tables

### Symbol Library Table

When any schematic symbol uses a library identifier that depends on a project-local symbol library, the writer must emit `sym-lib-table`.

The table must use S-expression syntax:

```text
(sym_lib_table
  (lib (name "local_symbols")(type "KiCad")(uri "${KIPRJMOD}/lib/local_symbols.kicad_sym")(options "")(descr ""))
)
```

Requirements:

- Table entries must be deterministic.
- Entry names must be unique within `sym-lib-table`.
- Entry names must be valid KiCad library nicknames.
- Project-local paths must use `${KIPRJMOD}`. The design model may provide a project-relative path, but the library table writer must render it as a `${KIPRJMOD}` URI. Standard KiCad environment variables such as `${KICAD_SYMBOL_DIR}` or versioned KiCad library variables may be used for external libraries.
- Environment variable paths such as `${KICAD_SYMBOL_DIR}` may be represented when explicitly requested.
- KiCad environment-variable URIs are treated as opaque KiCad references by default. The generator must validate URI syntax, but it must not require the referenced global KiCad library path to exist on the host running tests unless an explicit opt-in validation mode requests filesystem resolution.
- A symbol reference such as `local_symbols:PartName` must resolve to a matching table entry or embedded symbol.

### Footprint Library Table

When any board footprint references a library requiring a local or explicit footprint library, the writer must emit `fp-lib-table`.

The table must use S-expression syntax:

```text
(fp_lib_table
  (lib (name "local_footprints")(type "KiCad")(uri "${KIPRJMOD}/footprints.pretty")(options "")(descr "project footprints"))
)
```

Requirements:

- Table entries must be deterministic.
- Entry names must be unique within `fp-lib-table`.
- Entry names must be valid KiCad footprint library nicknames.
- Project-local paths must follow the same `${KIPRJMOD}` rendering and validation rules as symbol library table entries.
- Local `.pretty` directories must be emitted when the design includes local footprint modules.
- Local `.pretty` directories must contain one `.kicad_mod` file for each local footprint module represented by the design model.
- A footprint reference such as `local_footprints:R_0603` must resolve to a matching table entry and local module when local footprint generation is used.

## Hierarchical Schematics

### Root Sheet References

The schematic writer must render sheet nodes for every sheet declared by the root schematic.

A sheet node must include:

- sheet UUID
- position and size when represented by the model
- `Sheetname` property
- `Sheetfile` property
- optional hierarchical pins when represented by the model
- instances section entries required by the target KiCad format

### Child Sheet Files

Every `Sheetfile` path emitted by a parent schematic must resolve to a child schematic file in the generated directory. Multiple sheet instances may reference the same child schematic file because KiCad supports reusable hierarchical sheets.

The generated file inventory stores child schematic paths relative to the project root for disk writes. The `Sheetfile` property written inside a schematic must be relative to the directory containing that parent schematic file, using KiCad forward-slash separators.

Valid examples:

```text
power.kicad_sch
logic.kicad_sch
sch/connectors.kicad_sch
```

Invalid examples:

```text
../outside.kicad_sch
/absolute/path.kicad_sch
sch/../../outside.kicad_sch
```

Requirements:

- Child sheet paths must be relative to the project directory.
- Child sheet paths must be normalized.
- Paths stored inside KiCad project and schematic files must use forward slashes (`/`) as separators, including on Windows.
- Internal KiCad reference paths must be built with Go's `path` package, not `path/filepath`, so generated file contents do not inherit host-specific separators.
- Parent references and child filenames must resolve to the same project-inventory file after normalizing from the parent schematic's directory to KiCad's forward-slash path form.
- Duplicate child sheet paths are invalid.
- Child schematic UUIDs must be unique across the full design.
- A root schematic may reference sheets in subdirectories.
- The sheet graph must reject cycles so validation and generation cannot recurse indefinitely.
- The writer must create required child sheet subdirectories atomically as part of the project write.

## Project JSON

The project writer must emit KiCad-shaped `.kicad_pro` JSON. The current minimal JSON is not sufficient as the long-term compatibility target.

The generated project file must include the top-level sections normally observed in modern KiCad projects:

```json
{
  "board": {
    "design_settings": {}
  },
  "boards": [],
  "component_class_settings": {},
  "cvpcb": {},
  "erc": {},
  "libraries": {},
  "meta": {
    "version": 1
  },
  "net_settings": {
    "classes": []
  },
  "pcbnew": {},
  "schematic": {},
  "sheets": [],
  "text_variables": {},
  "time_domain_parameters": {}
}
```

The exact defaults must be based on a KiCad-generated fixture or demo-derived baseline. Generated JSON must:

- remain deterministic
- preserve `meta.version`
- include default net class settings
- include ERC and DRC severity defaults where supported by the model
- include schematic settings such as drawing defaults and page behavior where supported
- include board settings such as design defaults, rule severities, and view settings where supported
- include the project sheet list with root and child sheet UUID/name pairs
- avoid unsupported or unknown user-specific paths

The example above shows the required section names and representative non-empty modeled sections. Some KiCad sections may be initialized as empty default objects when fixture validation shows KiCad accepts and preserves them. Sections with modeled data must not be emitted as arbitrary empty objects. For example, `net_settings` must include the modeled net class data rather than an empty object.

If the writer cannot fully model a section yet, it may emit a stable default object for that section only when fixture validation shows KiCad accepts and preserves that object. Tests must document any defaulted section.

`.kicad_pro` `meta.version` uses KiCad's project JSON schema version, typically `1`, and must not reuse the date-style S-expression format version used by `.kicad_sch` and `.kicad_pcb`.

## Optional Project Artifacts

### Design Rule Files

The model may include one or more `.kicad_dru` files. A rule file must:

- have a safe relative path
- use `.kicad_dru` extension
- use a KiCad-supported rule-file form, either the modern `(kicad_dru ...)` root form or the legacy/demo bare `(version N)` form when the target KiCad version accepts it
- contain deterministic rule text

The writer does not need to understand every rule expression initially, but it must validate safe paths and preserve exact rule content supplied by the model.

### Worksheet Files

The model may include `.kicad_wks` worksheet files. A worksheet file must:

- have a safe relative path
- use `.kicad_wks` extension
- contain deterministic text

The project JSON may reference the worksheet when the project model supports it.

### Asset Files

The model may include copied or generated asset files such as 3D models. Asset paths must:

- be relative to the project directory
- reject path traversal
- avoid overwriting generated KiCad core files
- be deterministic in tests

Line-ending normalization applies only to generated text KiCad files. Asset files must be streamed byte-for-byte so binary 3D models such as STEP or WRL files are not corrupted.

## Validation

Validation must happen before the writer creates or replaces the target directory.

Validation requirements:

1. Project basename is safe.
2. Output directory path is safe and names a directory target.
3. Root schematic filename, when set, matches `<project>.kicad_sch`.
4. Root board filename, when represented, matches `<project>.kicad_pcb`.
5. Every child sheet reference resolves to an emitted child schematic file; multiple references may target the same reusable child file.
6. No generated relative path escapes the project directory.
7. No two generated files claim the same relative path.
8. No two generated files claim paths that collide on case-insensitive filesystems.
9. Root project, schematic, and board file paths do not overlap with child sheets, assets, library tables, local library files, or optional artifacts.
10. No generated file path collides with a generated directory path; for example, `lib` and `lib/symbol.kicad_sym` cannot both be generated as files.
11. The hierarchical sheet graph has no cycles.
12. Library table nicknames are unique within each table.
13. Project-local library table entries point to emitted local library files or directories.
14. Schematic symbol library identifiers resolve through embedded symbols, symbol tables, or known external libraries.
15. PCB footprint library identifiers resolve through footprint tables, embedded footprint data, or known external libraries.
16. UUIDs use KiCad-compatible canonical 8-4-4-4-12 lowercase hexadecimal text and are unique across project, schematics, board objects, and generated library objects when those objects carry UUIDs.
17. Optional artifacts use allowed extensions and safe relative paths.

## Atomic Write Behavior

The directory writer must preserve the existing atomic behavior:

- write into a temporary project directory
- create the temporary project directory under the destination's parent directory so final renames stay on the same filesystem
- fsync files where supported
- fsync containing directories where supported
- refuse overwrite unless explicitly requested
- journal overwrite operations
- clean up temporary and backup directories on success
- leave enough information for manual recovery on failure

When subdirectories and additional files are introduced, they must be written inside the temporary project directory before the final rename.

The writer must not use a cross-device copy/delete fallback for final replacement because that would weaken atomicity. If an unexpected cross-device rename error occurs despite creating the temporary directory under the destination parent, the writer must fail and leave recovery information rather than perform a non-atomic replacement.

## Testing Requirements

The implementation must include:

- unit tests for filename and relative path validation
- unit tests for directory basename differing from project basename
- golden tests for `sym-lib-table`
- golden tests for `fp-lib-table`
- unit tests for hierarchical sheet rendering
- unit tests for child sheet file emission
- unit tests for duplicate generated path rejection
- unit tests for project JSON top-level section coverage
- integration-style tests that write a complete generated project directory in a temporary directory
- compatibility tests comparing generated structure against demo-derived expectations without copying demo files into the repository

If KiCad command-line validation is available locally, add an opt-in test target or documented command for opening or validating generated fixtures. Normal `go test ./...` must not require KiCad to be installed.

## Acceptance Criteria

The work is complete when:

1. The LED indicator project still writes the original minimal core files.
2. A project can be written into a directory whose basename differs from `Design.Name`.
3. A generated design with local symbol and footprint library references emits `sym-lib-table` and `fp-lib-table`.
4. A generated hierarchical design emits root and child `.kicad_sch` files with matching sheet references.
5. Generated `.kicad_pro` files include KiCad-shaped top-level sections.
6. Optional `.kicad_dru` and `.kicad_wks` files can be represented and emitted when requested.
7. All generated paths are validated as project-relative and non-overlapping.
8. `go test ./...` passes.
9. Documentation explains the supported project structure and known limitations.

## Open Questions

1. Should KiCadAI emit empty `sym-lib-table` and `fp-lib-table` for every project, or only when explicit table entries exist?
2. Should local symbol and footprint library generation be implemented in this compatibility pass or only table emission plus path validation?
3. Which KiCad version should define the canonical project JSON defaults when local KiCad and demo files differ?
4. Should binary 3D model assets be supported during the first optional-artifact phase?
5. Should the command-line interface expose separate `--output-dir` and `--project-name` flags to make directory and project basename differences explicit?
