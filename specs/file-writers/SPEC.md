# KiCad Project, Schematic, and Printed Circuit Board Writer Specification

## Purpose

Implement direct KiCad file writers in Go so KiCadAI generates new KiCad projects, schematics, and printed circuit board layouts even when the live KiCad inter-process communication application programming interface does not expose write/mutation commands. The writers must produce valid, deterministic KiCad files that open in KiCad and serve as a foundation for artificial-intelligence-assisted electronic design.

The first target is generated designs from scratch. Editing arbitrary existing KiCad files is a later phase and must not be assumed by this specification.

## Background

Live inter-process communication probing showed that the running KiCad application programming interface accepted basic common commands but returned `AS_UNHANDLED` for editor, schematic, and board mutation commands. Direct file generation is therefore the required write path for this project until live mutation commands are handled.

KiCad uses text-based formats:

- `.kicad_pro`: project settings, JSON.
- `.kicad_sch`: schematic, S-expression.
- `.kicad_pcb`: PCB layout, S-expression.
- `.kicad_sym`: symbol libraries, S-expression.
- `.kicad_mod`: footprint libraries, S-expression.
- `sym-lib-table` and `fp-lib-table`: library table S-expression style files.

This specification covers only project, schematic, and printed circuit board writers. Symbol and footprint library writers are out of scope for the first implementation, but the initial design must leave extension points for them.

## Glossary

| Term | Meaning |
|---|---|
| AI | Artificial intelligence. |
| API | Application programming interface. |
| ASCII | American Standard Code for Information Interchange. |
| BUS | KiCad bus label or bus net grouping. |
| CLI | Command-line interface. |
| DRC | Design rule check. |
| ERC | Electrical rule check. |
| GND | Ground power net name. |
| GUI | Graphical user interface. |
| IPC | Inter-process communication. |
| IU | KiCad internal unit. |
| JSON | JavaScript Object Notation. |
| LED | Light-emitting diode. |
| MM | Millimeter helper used for unit conversion. |
| PCB | Printed circuit board. |
| PWR | KiCad power-symbol reference prefix. |
| REFERENCE | KiCad text variable for a component reference designator. |
| RFC | Request for Comments. |
| VCC | Positive supply power net name. |
| Reserved Windows device filenames | Case-insensitive names matching `(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9]|CLOCK\$|CONIN\$|CONOUT\$)(\\..*)?$`; generated filenames must also reject trailing spaces and periods. |

## Goals

1. Generate a complete minimal KiCad project directory.
2. Generate a valid `.kicad_pro` project file.
3. Generate a valid `.kicad_sch` schematic file.
4. Generate a valid `.kicad_pcb` board file.
5. Support a simple light-emitting diode indicator design as an acceptance fixture.
6. Produce deterministic output suitable for snapshot tests.
7. Preserve stable IDs and UUIDs when generation inputs are stable.
8. Avoid direct string-concatenation formatting bugs by using structured writers.
9. Validate generated data before writing files.
10. Keep direct file generation independent of live KiCad.

## Non-Goals

- Lossless parsing/editing of arbitrary existing KiCad files.
- Autorouting.
- Full symbol library generation.
- Full footprint library generation.
- Replacing KiCad's electrical rule check, design rule check, or import tools.
- Guaranteeing compatibility with every historical KiCad file version.
- Using IPC write commands.
- Modifying user files without explicit output paths.

## Compatibility Target

Initial writers must target the KiCad file-format family used by KiCad 8, 9, and 10, with KiCad 10 as the active local validation target.

The writer must expose the target file-format version as configuration:

```go
type KiCadFormatVersion string

const (
    KiCadFormatV20230121 KiCadFormatVersion = "20230121"
)
```

If future KiCad versions require newer format identifiers, add explicit constants and tests rather than silently changing output.

## Writer Architecture

### Package Layout

Add writer packages under:

```text
internal/kicadfiles
internal/kicadfiles/sexpr
internal/kicadfiles/project
internal/kicadfiles/schematic
internal/kicadfiles/pcb
internal/kicadfiles/validate
```

Responsibilities:

- `sexpr`: generic S-expression rendering primitives.
- `project`: `.kicad_pro` model and JSON writer.
- `schematic`: `.kicad_sch` model and S-expression writer.
- `pcb`: `.kicad_pcb` model and S-expression writer.
- `validate`: cross-file validation helpers.
- `kicadfiles`: public internal orchestration for generating a project directory.

### Writer Contract

Every writer must implement:

```go
type Writer[T any] interface {
    Validate(T) error
    Write(io.Writer, T) error
}
```

Concrete writers must expose these specific APIs:

```go
// internal/kicadfiles/project
func Write(w io.Writer, project ProjectFile) error

// internal/kicadfiles/schematic
func Write(w io.Writer, schematic SchematicFile) error

// internal/kicadfiles/pcb
func Write(w io.Writer, pcb PCBFile) error
```

File-system orchestration must be separate:

```go
func WriteProjectDirectory(root string, design Design, opts WriteOptions) (WriteResult, error)
```

`WriteProjectDirectory` is the only writer function permitted to create directories and files. Low-level writers must only write to the provided `io.Writer`.

## S-Expression Writer Requirements

The S-expression writer is foundational and must be implemented before schematic or PCB writers.

### Data Model

```go
type Node any
type List []Node
type Atom string
type String string
type Int int64
type Float float64
type Fixed string
```

The implementation must choose a concrete structured model, and that model must support:

- Symbols/atoms.
- Quoted strings.
- Integers.
- Floats.
- Preformatted fixed-point numbers that render unquoted after numeric validation.
- Nested lists.
- Optional omitted nodes.
- Stable two-space indentation.

### Rendering Rules

1. Output must be UTF-8.
2. Line endings must be `\n`.
3. Strings must escape:
   - `\`
   - `"`
   - newline as `\n`
   - carriage return as `\r`
   - tab as `\t`
   - other ASCII control characters as `\xNN` with uppercase hexadecimal digits.
4. Atoms must match `^[A-Za-z0-9_+*/<>=!?$%&~^:.@#\[\]{}-]+$` and must not match the numeric-literal pattern `^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`; any value outside this set must be rendered as a quoted string or rejected by validation. The atom grammar permits common KiCad atoms such as `+5V`, `#PWR`, `BUS[0..7]`, `${REFERENCE}`, and `1N4148` while preventing numeric-looking atoms such as `-123`, `.45`, `+1`, and `1e10`.
5. Generic floats must render deterministic fixed-point decimal text with `strconv.FormatFloat(value, 'f', -1, 64)`, followed by trimming of negative zero to `0`.
6. KiCad coordinate values derived from `IU` must not round-trip through `float64`; they must be formatted from the integer value by inserting the decimal point at six decimal places for millimeters and passed to the S-expression renderer as a `Fixed` numeric node.
7. `Fixed` values must match the numeric-literal pattern `^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)$` and must render without quotes.
8. Empty optional fields must be omitted, not rendered as malformed empty lists.
9. Top-level files must end with a trailing newline.
10. Formatting must be stable across operating systems.

### Tests

Required tests:

- Atom rendering.
- String escaping.
- Nested list rendering.
- Stable float formatting.
- Empty optional omission.
- Golden snapshot for a small schematic fragment.

## Common Domain Model

The project, schematic, and printed circuit board writers must share common primitives.

### IDs

Use explicit UUID strings for KiCad UUID fields:

```go
type UUID string
```

Rules:

- IDs must be non-empty where KiCad requires them.
- Generated IDs must be deterministic when a deterministic seed is supplied.
- Random IDs must be used only when the caller has not requested deterministic output.
- IDs must be stable across related files when they represent the same logical entity.
- Deterministic IDs must include a namespace derived from a persistent `DesignID` in addition to the caller seed and logical path.
- Deterministic IDs must use a standard RFC 4122 UUID version 5 implementation. Normalize `seed` and `logicalPath` individually to Unicode Normalization Form C first, then pass the raw UTF-8 byte sequence of `fmt.Sprintf("%d:%s:%d:%s", len(seed), seed, len(logicalPath), logicalPath)` as the UUID version 5 name, where the lengths are byte counts of those normalized strings.
- `DesignID` must be a valid UUID string and must be parsed into its 16-byte binary UUID representation before use as the UUID version 5 namespace.
- Logical path means the stable structural path inside the design model, such as `root.component.resistor1`, `root.component.resistor1.pad.1`, `root.schematic.net.led_out`, or `root.pcb.track.net.led_out.segment.1`; it must never mean the filesystem path of the generated project directory.
- A schematic symbol and its paired PCB footprint must share the same component logical path so their UUID/path linkage remains stable across schematic and board files.
- Anonymous elements such as wires, junctions, tracks, vias, drawings, and zones must use caller-supplied stable keys when available; otherwise, their logical path must use a canonical content signature from type, layer/net, and sorted coordinate values rather than slice index.
- The human-readable project name must not be part of UUID derivation so renaming a project does not rewrite every generated object UUID.

### Coordinates

Use KiCad internal units for layout math and convert only at writer boundaries when the file format requires decimal millimeters:

```go
type IU int64 // KiCad internal unit
type Point struct {
    X IU
    Y IU
}

type Angle float64 // degrees, matching KiCad S-expression angle fields
```

KiCad 6 and later define one internal unit as one nanometer. Millimeter output from `IU` must therefore use exactly six decimal places before trimming insignificant trailing zeroes.

Signed `IU` formatting must handle signs and zero-padding explicitly: format the absolute integer value as at least seven decimal digits by left-padding with zeroes, insert the decimal point six digits from the right without floating-point rounding, keep a leading zero for values between -1 and 1, trim insignificant trailing zeroes while preserving at least one fractional digit, then prepend `-` only when the original value was negative.

Provide helpers:

```go
func MM(value float64) IU // millimeters to KiCad internal units
func Mil(value float64) IU
func ToMMString(value IU) string
```

Rules:

- No implicit unit conversion in domain structs.
- Writer functions convert to KiCad file units explicitly.
- Serialization code must use `ToMMString` plus S-expression `Fixed` for coordinates, sizes, and other `IU`-derived millimeter values.
- `MM` and `Mil` must use `math.Round` before casting to `IU` to avoid truncation errors from floating-point representation.
- `MM` and `Mil` must reject non-finite values and values that would overflow `int64` after conversion.
- Tests must cover exact expected output for common values.

### Layers

Board layers must be represented with typed constants:

```go
type BoardLayer string
```

At minimum:

```text
F.Cu
B.Cu
F.SilkS
B.SilkS
F.Mask
B.Mask
Edge.Cuts
*.Cu
*.Mask
All
```

### LED Indicator Fixture Input

```go
type LEDIndicatorInput struct {
    Name       string
    DesignID   UUID
    Seed       string
    IncludePCB bool
}
```

Validation must reject an empty `Name`, invalid `DesignID` when deterministic output is requested, and project-name values that fail the project writer's filename rules.

## Project Writer Specification

### File

The project writer emits:

```text
<name>.kicad_pro
```

### Format

Project output is JSON.

### Required Inputs

```go
type ProjectFile struct {
    Name           string
    DesignID       UUID
    FormatVersion  KiCadFormatVersion
    Generator      string
    PageSettings   PageSettings
    NetClasses     []NetClass
    TextVariables  map[string]string
}
```

The exact struct is permitted to gain fields after comparing against KiCad-generated project files, but every implementation must preserve these concerns:

- Project identity.
- File-format version.
- Generator string.
- Net classes.
- Page settings.
- Text variables.

### Required Output Behavior

1. Write deterministic JSON key ordering.
2. Use two-space indentation.
3. Include a generator string such as `kicadai`.
4. Include minimal sections required for KiCad to open the project without repair prompts.
5. Omit unsupported optional sections unless explicitly requested.

### Validation

Project validation must reject:

- Empty project name.
- Empty `DesignID` when deterministic ID generation is enabled.
- Project names longer than 128 UTF-8 bytes after Unicode Normalization Form C. This limit leaves path budget for generated suffixes, temporary directories, and backup journals on filesystems with 255-byte filename component limits.
- Project names outside the safe filename pattern `^[\p{L}\p{N}_]([\p{L}\p{N}._ -]*[\p{L}\p{N}_])?$`.
- Reserved Windows device filenames matching the glossary-defined reserved-name regular expression.
- Project names with trailing spaces or periods.
- Empty generator.
- Duplicate net class names.
- Missing default net class when net classes are supplied.

### Tests

Required tests:

- Minimal project snapshot.
- Project with custom net classes.
- Project with text variables.
- Invalid empty name.
- Duplicate net classes.

## Schematic Writer Specification

### File

The schematic writer emits:

```text
<name>.kicad_sch
```

### Format

Schematic output is S-expression.

### Required Inputs

```go
type LibraryReference struct {
    LibraryID string
}

type EmbeddedSymbol struct {
    LibraryID string
    Body      []sexpr.Node
}

type SchematicFile struct {
    Filename   string
    Version    KiCadFormatVersion
    Generator  string
    UUID       UUID
    Paper      Paper
    TitleBlock TitleBlock
    Libraries  []LibraryReference
    LibSymbols []EmbeddedSymbol
    Symbols    []SchematicSymbol
    Wires      []Wire
    Labels     []Label
    Junctions  []Junction
    Sheets     []Sheet
    Instances  []SymbolInstance
}
```

### Minimal Required Output

A minimal schematic must include:

- Top-level `kicad_sch` list.
- Version.
- Generator.
- UUID.
- Paper.
- Embedded `lib_symbols` definitions for every placed symbol library identifier.
- Placed symbols.
- Wires.
- Labels.
- Sheet/path instance data when required by KiCad.

### Symbol Strategy

The initial schematic writer must reference placed symbols by KiCad library identifier and must also embed matching `lib_symbols` definitions for every used symbol so generated schematic files remain portable across machines without relying on global KiCad libraries.

The first production milestone must use these existing library identifiers:

```text
power:VCC
power:GND
Device:R
Device:LED
```

The implementation must provide a symbol-library fixture or a symbol extraction step from known KiCad libraries to populate `LibSymbols` for these identifiers.

### Schematic Elements

#### Symbol

```go
type SchematicSymbol struct {
    UUID       UUID
    Path       string
    LibraryID string
    Reference string
    Value     string
    Position  Point
    Rotation  Angle
    Fields    []Field
}

type Field struct {
    Name     string
    Value    string
    Visible  bool
    Position Point
    Rotation Angle
}
```

Validation:

- UUID required.
- Path required and must reference the paired schematic symbol path when a schematic is generated with the PCB.
- Library ID required.
- Reference required.
- Value required unless symbol type explicitly allows empty value.
- Position required.
- If the caller configures custom schematic placement bounds, position must be within those bounds. KiCad itself permits placement outside the visible paper border, so paper size must not be treated as a hard placement bound by default.
- Field names unique per symbol.

#### Wire

```go
type Wire struct {
    UUID   UUID
    Points []Point
}
```

Validation:

- UUID required.
- At least two points.
- No zero-length adjacent segments.

#### Label

```go
type Label struct {
    UUID     UUID
    Text     string
    Kind     LabelKind
    Position Point
    Rotation Angle
}
```

Supported label kinds:

```text
local
global
hierarchical
directive
```

Validation:

- UUID required.
- Text required.
- Kind recognized.

#### Junction

```go
type Junction struct {
    UUID     UUID
    Position Point
}
```

Validation:

- UUID required.
- Position required.

### Connectivity Rules

The writer does not need to solve ERC, but it must preserve intended connectivity:

- Wire endpoints must align with symbol pin positions when generated from a known symbol model.
- Labels placed on wires must use matching coordinates.
- Generated test fixtures must include explicit expected net names.

### Determinism

Schematic output order must be deterministic:

1. Header fields.
2. Library symbols.
3. Junctions.
4. Wires.
5. Labels.
6. Symbols.
7. Sheets.
8. Instances.

If KiCad requires a different canonical ordering, update the writer and snapshots to match KiCad's saved output.

### Tests

Required tests:

- Minimal empty schematic snapshot.
- LED indicator schematic snapshot.
- Symbol validation.
- Wire validation.
- Label validation.
- UUID determinism.
- String escaping in labels and fields.
- KiCad-open smoke test as optional integration/manual test.

## PCB Writer Specification

### File

The PCB writer emits:

```text
<name>.kicad_pcb
```

### Format

PCB output is S-expression.

### Required Inputs

```go
type PCBFile struct {
    Version      KiCadFormatVersion
    Generator    string
    General      PCBGeneral
    Paper        Paper
    Layers       []LayerDefinition
    Setup        PCBSetup
    Nets         []Net
    Footprints   []Footprint
    Tracks       []Track
    Vias         []Via
    Zones        []Zone
    Drawings     []Drawing
    Dimensions   []Dimension
    TitleBlock   TitleBlock
}
```

Core supporting PCB types must include at least these fields:

```go
type PCBGeneral struct {}

type PCBSetup struct {
    Stackup             PCBStackup
    SolderMaskMinWidth IU
    PadToMaskClearance IU
}

type PCBStackup struct {
    Thickness IU
}

type LayerDefinition struct {
    Number int
    Name   BoardLayer
    Kind   string
}

type Paper struct {
    Name   string
    Width  IU
    Height IU
}

type TitleBlock struct {
    Title    string
    Date     string
    Revision string
    Company  string
    Comments []string
}

type Pad struct {
    Name      string
    NetCode   int
    Shape     string
    Position  Point
    Rotation  Angle
    Size      Point
    Drill     IU
    Layers    []BoardLayer
}

type Drawing struct {
    UUID   UUID
    Layer  BoardLayer
    Kind   string
    Line   *LineDrawing
    Circle *CircleDrawing
    Arc    *ArcDrawing
    Poly   *PolylineDrawing
}

type LineDrawing struct {
    Start Point
    End   Point
    Width IU
}

type CircleDrawing struct {
    Center Point
    End    Point
    Width  IU
}

type ArcDrawing struct {
    Start Point
    Mid   Point
    End   Point
    Width IU
}

type PolylineDrawing struct {
    Points []Point
    Width  IU
}

type FootprintText struct {
    Kind     string
    Text     string
    Position Point
    Rotation Angle
    Layer    BoardLayer
}

Allowed `FootprintText.Kind` values are `reference`, `value`, and `user`.

type FootprintGraphic struct {
    UUID   UUID
    Layer  BoardLayer
    Kind   string
    Line   *LineDrawing
    Circle *CircleDrawing
    Arc    *ArcDrawing
    Poly   *PolylineDrawing
}

type Dimension struct {
    UUID     UUID
    Type     string
    Layer    BoardLayer
    Points   []Point
    Height   IU
    Text     string
    Position Point
    Rotation Angle
}
```

Project, schematic, and PCB writers must validate `TitleBlock` before rendering and must reject any title block with more than nine comments because KiCad title blocks expose comment fields 1 through 9.

### Minimal Required Output

A minimal PCB must include:

- Top-level `kicad_pcb` list.
- Version.
- Generator.
- General section.
- Paper.
- Layers.
- Setup.
- Nets.
- Footprints.

Tracks, vias, zones, drawings, and dimensions are optional in the first milestone.

### Net Model

```go
type Net struct {
    Code int
    Name string
}
```

Validation:

- Net code `0` is reserved for the empty net.
- The writer must always emit `(net 0 "")`, even when the caller supplies no nets.
- Net names must be unique.
- Net codes must be unique.
- Footprint pads, tracks, vias, and zones must reference existing nets by net code.

### Footprint Model

```go
type Footprint struct {
    UUID       UUID
    Path       string
    LibraryID string
    Reference string
    Value     string
    Position  Point
    Rotation  Angle
    Layer     BoardLayer
    Texts     []FootprintText
    Pads      []Pad
    Graphics  []FootprintGraphic
}
```

KiCad PCB files must contain full footprint definitions. `LibraryID` records the source library identity for traceability, but it is not sufficient by itself. The writer must emit enough footprint geometry, footprint text (`fp_text` reference and value), pads, pad shapes, pad sizes, pad layers, and footprint graphics (`fp_line`, `fp_circle`, `fp_arc`, `fp_poly`) for KiCad to display and connect the component without resolving an external footprint library.

Validation:

- UUID required.
- Library ID required.
- Reference required.
- Layer required.
- A reference `FootprintText` and value `FootprintText` are required.
- Reference and value `FootprintText.Text` values must match `Footprint.Reference` and `Footprint.Value`.
- Pad name required for every pad.
- Pad names unique within footprint.
- Pad shape required.
- Pad size positive.
- Pad rotation must be rendered as the optional third `(at x y angle)` value when non-zero.
- Pad layer list not empty.
- Pad nets exist in PCB net table.

### Track Model

```go
type Track struct {
    UUID   UUID
    Start  Point
    End    Point
    Width  IU
    Layer  BoardLayer
    NetCode int
}
```

Validation:

- UUID required.
- Start and end must differ.
- Width positive.
- Layer must be copper.
- Net code must exist.

### Via Model

```go
type Via struct {
    UUID     UUID
    Position Point
    Size     IU
    Drill    IU
    NetCode  int
    Layers   []BoardLayer
}
```

Validation:

- UUID required.
- Size positive.
- Drill positive.
- Drill must be less than size so vias have a non-zero annular ring.
- Net code must exist.
- Layers must include at least two distinct copper layers.

### Zone Model

```go
type Zone struct {
    UUID     UUID
    NetCode  int
    Layers   []BoardLayer
    Polygons [][]Point
    Priority int
}
```

Validation:

- UUID required.
- Net code must exist.
- At least one layer.
- At least one polygon.
- Each polygon must have at least three points. The first polygon is the outer boundary; subsequent polygons represent holes or islands as supported by the writer.

### Board Outline

The writer must support `Edge.Cuts` drawings for board outline.

Initial outline types:

- Line segment.
- Rectangle, represented as a closed `PolylineDrawing` with four corners plus the repeated first point.

Validation:

- Each drawing must have exactly one shape payload.
- Drawing width must be positive.
- Polyline drawings must contain at least two distinct points.
- Outline must be closed when required by the caller.
- Edge cuts drawings must use `Edge.Cuts`.

### Determinism

PCB output order must be deterministic:

1. Header fields.
2. General.
3. Paper.
4. Layers.
5. Setup.
6. Nets ordered by code.
7. Footprints ordered by reference, then UUID.
8. Drawings.
9. Tracks.
10. Vias.
11. Zones.
12. Dimensions.

If KiCad canonical save order differs, prefer KiCad's order after validation.

### Tests

Required tests:

- Minimal board snapshot.
- LED indicator board snapshot.
- Net validation.
- Footprint validation.
- Track validation.
- Via validation.
- Board outline validation.
- Deterministic ordering.
- KiCad-open smoke test as optional integration/manual test.

## Project Directory Writer

### Purpose

Generate a complete project directory from a single design model.

### Input

```go
type Design struct {
    Name      string
    Project   ProjectFile
    Schematic SchematicFile
    Sheets    []SchematicFile
    PCB       *PCBFile
}
```

`Schematic` is the root schematic. `Sheets` contains optional additional hierarchical schematic files. Every `SchematicFile` must include a unique `Filename`, and each parent `Sheet` reference must resolve to exactly one child schematic filename.

`SchematicFile.Filename` must be a portable path relative to the generated project directory. It must not be absolute, must not contain `..` path traversal segments, and must be normalized to Unicode Normalization Form C before validation or persistence.

### Output

For a design named `blink`:

```text
blink/
  blink.kicad_pro
  blink.kicad_sch
  blink.kicad_pcb
```

If `PCB` is nil, omit the board file.

The result must report generated files, non-fatal warnings, and any backup directory that requires user recovery:

```go
type WriteResult struct {
    ProjectDir     string
    WrittenFiles   []string
    Warnings       []WriteWarning
    BackupDir      string
    JournalPath    string
    OrphanTempDirs []string
}

type WriteWarning struct {
    Code    string
    Path    string
    Message string
}
```

### Safety Rules

1. Refuse to overwrite an existing non-empty directory unless `Overwrite` is explicitly true.
2. Use `os.MkdirTemp` under the target parent directory with a sanitized project-name prefix so temporary and final paths are on the same filesystem and the temporary name includes an unpredictable suffix. The temporary prefix must be derived from the validated project name by replacing every non-alphanumeric byte with `_`.
3. For new project directories, construct the entire project inside the temporary directory and then perform one directory rename into place only when the target does not exist and the temporary directory is on the same filesystem as the target parent. Existing empty target directories are treated as existing targets; callers must remove them or use explicit overwrite mode.
4. If a whole-directory rename fails because of platform or filesystem behavior, leave the target absent, remove the temporary directory on ordinary failure, and return a structured error. The writer must not silently fall back to a partial copy for new project directories.
5. For explicit overwrites of existing project directories, construct the new project in a temporary directory on the same filesystem, create a unique backup container directory with `os.MkdirTemp` under the target parent using the project-name prefix and a random suffix, rename the existing project directory to a child path inside that backup container, rename the temporary directory into place, then remove the backup container after success. This is a recoverable directory swap, not a fully atomic transaction.
6. The writer must fail before moving the existing project when it cannot create a unique backup container.
7. Before moving the existing project directory, the writer must create a recovery journal file in the target parent directory that records the target path, temporary project path, backup child path, and current swap phase, then call `Sync()` on the journal file before proceeding to the first directory rename.
8. The writer must update the recovery journal after each swap phase, call `Sync()` after each update, and remove the journal only after the target directory is in its final state and backup cleanup has succeeded or has been reported.
9. On startup or before writing a project, the writer must detect an existing recovery journal for that project name and return a structured error with recovery instructions rather than starting a second swap.
10. If moving the temporary directory into place fails after the existing project was moved to backup, the writer must attempt a best-effort rollback by renaming the backup child directory back to the original target path.
11. The writer must return a structured error with `WriteResult.BackupDir` and `WriteResult.JournalPath` set for any overwrite swap failure that happens after the existing project directory was moved to backup and rollback does not fully restore the original target.
12. If overwrite swap cleanup fails after the new directory is in place, return a warning in `WriteResult` that names the backup directory, sets `WriteResult.BackupDir`, and instructs the caller to inspect that sibling directory before deleting it.
13. Preserve user files that are not part of the generated output only when they are outside the overwritten project directory. Whole-directory overwrite mode replaces the generated project directory.
14. Return a structured list of written files.
15. Remove the operation temporary directory on ordinary failures.
16. Post-crash orphan temporary, backup, or journal paths must be identifiable by their generated project-name prefix and must be safe for a user to inspect and delete manually after following recovery instructions.

### Tests

Required tests:

- Creates expected files.
- Refuses unsafe overwrite.
- Allows explicit overwrite.
- Cleans temporary files after failure.
- Simulates each overwrite recovery journal phase and verifies the writer reports recovery state before starting a second swap.
- Verifies rollback restores the original target when the final rename fails after backup.
- Omits PCB when nil.

## Validation Requirements

Validation must run before writing.

### Cross-File Validation

When generating both schematic and PCB:

- Project name matches file basenames.
- Schematic symbols that imply footprints must map to printed circuit board footprints.
- Paired schematic symbols and PCB footprints must share the same component logical path and the PCB footprint `Path` must reference that schematic path.
- PCB nets include expected schematic net names when a net mapping is supplied.
- Board footprints have unique references matching schematic references where applicable.
- UUID values must be unique across the full generated project scope, including all schematic sheets and the PCB.
- Hierarchical schematic sheet references must form a directed acyclic graph; validation must reject cycles before rendering.

### Error Model

Validation errors must be structured:

```go
type ValidationError struct {
    File    string
    Section string
    Field   string
    Message string
}
```

Errors must be collectable:

```go
type ValidationErrors []ValidationError
```

Validation must report all independent errors where practical rather than failing at the first issue.

## Acceptance Fixture: LED Indicator

The first acceptance fixture must generate a complete project containing:

- VCC power symbol.
- Series resistor.
- LED.
- GND power symbol.
- Wires connecting VCC -> resistor -> LED -> GND.
- Local label `LED_OUT`.
- Optional PCB with resistor footprint, LED footprint, and simple two-layer board outline.

### Expected Files

```text
led_indicator.kicad_pro
led_indicator.kicad_sch
led_indicator.kicad_pcb
```

### Acceptance Criteria

1. Files are deterministic for a fixed seed.
2. Files must open in KiCad without manual repair.
3. Schematic visually shows the LED indicator circuit.
4. PCB file opens and displays footprints/nets if PCB generation is enabled.
5. Re-saving in KiCad must not radically restructure the file beyond KiCad's canonical formatting changes.

## Testing Strategy

### Unit Tests

Unit tests must cover:

- S-expression rendering.
- Project JSON rendering.
- Schematic rendering.
- PCB rendering.
- Validation.
- Deterministic UUID generation.
- Project directory safety.

### Golden Tests

Golden snapshots must be used for:

- Minimal project.
- Minimal schematic.
- LED schematic.
- Minimal PCB.
- LED PCB.

Golden tests must be easy to update intentionally. The update command must be explicit:

```text
UPDATE_GOLDEN=1 go test ./internal/kicadfiles/...
```

### Integration Tests

Integration/manual tests are permitted to use KiCad to open generated files.

They must be opt-in:

```text
go test -tags=integration ./internal/kicadfiles/...
```

Integration tests must not require the inter-process communication application programming interface. They are permitted to use KiCad command-line interface commands for validation when available, but opening graphical KiCad must remain a manual verification step unless a headless validator is available.

## CLI Requirements

Add generation commands:

```text
kicadai generate-project
kicadai generate-led-demo
```

Options:

```text
--output string
--name string
--with-pcb
--seed string
--overwrite
--json
```

CLI output in JSON mode:

```json
{
  "project": "led_indicator",
  "written_files": [
    "led_indicator.kicad_pro",
    "led_indicator.kicad_sch",
    "led_indicator.kicad_pcb"
  ],
  "warnings": [],
  "backup_dir": "",
  "journal_path": "",
  "recovery_instructions": ""
}
```

When an overwrite recovery journal exists or a swap failure leaves `backup_dir` or `journal_path` set, the CLI must print concrete recovery instructions in human output and include `recovery_instructions` in JSON output.

## Implementation Phases

The authoritative phased implementation sequence is the plan document in `specs/file-writers`. Keep phase numbering in that plan only so implementation tracking has a single source of truth.

## Implementation Decisions Required Before Phase 1

1. Decide whether the first schematic writer embeds library symbol definitions or relies on KiCad standard libraries.
2. Decide which KiCad file-format version is the default output for KiCad 10.
3. Decide whether project JavaScript Object Notation is generated from a minimal template copied from KiCad or modeled fully in Go.
4. How much PCB footprint data is required for KiCad to open a useful generated board without external library resolution?
5. Decide whether direct file writers live under `internal` until the output format stabilizes or move to `pkg` earlier.

## Risks

### Risk: KiCad Requires Embedded Symbol Definitions

Mitigation: Start with a KiCad-generated fixture, compare against writer output, and add a symbol fixture library if standard-library references are insufficient.

### Risk: Generated PCB Footprints Are Incomplete

Mitigation: Start with board net/outline generation and add footprints from known fixture templates before generating arbitrary footprints.

### Risk: File Format Drift

Mitigation: Pin format versions, add snapshots, and validate output by opening in the local KiCad version.

### Risk: String Formatting Bugs

Mitigation: Use structured S-expression nodes and snapshot tests instead of ad hoc string assembly.

### Risk: Accidentally Overwriting User Projects

Mitigation: Safe project directory writer with explicit overwrite flag and temporary-file writes.

## Done Criteria

The writer implementation is complete when:

- `make test` passes.
- `make coverage-check` passes.
- Minimal project, schematic, and PCB snapshots are stable.
- LED indicator project generation works from CLI.
- Generated files open in KiCad without repair prompts.
- Writer errors are structured and actionable.
- No live inter-process communication application programming interface write support is required.
