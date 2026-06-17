# Symbol Resolver Hardening Specification

## 1. Purpose

Harden KiCadAI's symbol resolver so generated schematics and component
selection can rely on real KiCad symbol semantics instead of approximate symbol
IDs and block-local pin hints.

This is the next foundation project after component intelligence. Component
selection can now choose catalog-backed records, but autonomous design still
needs stronger symbol parsing, unit handling, electrical pin metadata, power
symbol behavior, inheritance, and project/global library resolution before AI
can safely generate broad schematic designs.

## 2. Goals

- Parse KiCad `.kicad_sym` files with enough fidelity for generation,
  validation, and pinmap evidence.
- Resolve symbols through global, project-local, and direct library roots.
- Extract real pin numbers, names, positions, orientations, lengths, electrical
  types, units, body styles, hidden state, and alternate representations.
- Model multi-unit symbols deterministically, including common/shared pins.
- Support symbol inheritance/extension semantics used by KiCad libraries.
- Treat power symbols and hidden power pins conservatively.
- Provide stable resolver diagnostics for unsupported or ambiguous symbols.
- Feed real symbol pin geometry into schematic generation where safe.
- Feed real symbol pin metadata into component intelligence, pinmap validation,
  schematic-to-PCB transfer, writer correctness, and design workflow feedback.

## 3. Non-Goals

- Reimplement KiCad's full schematic editor.
- Support arbitrary non-KiCad symbol file formats.
- Infer unsafe electrical intent from symbol names alone.
- Treat every parsed symbol as fabrication-ready without compatible footprint
  and pinmap evidence.
- Auto-repair malformed third-party symbol libraries in place.
- Replace KiCad ERC. This project supplies stronger local evidence and
  diagnostics before KiCad-backed checks run.

## 4. Current Gaps

Current resolver and schematic paths can resolve some symbols and inspect basic
fields, but the following gaps remain:

- incomplete `.kicad_sym` grammar coverage;
- incomplete symbol inheritance handling;
- weak multi-unit and common-pin semantics;
- limited alternate body/style handling;
- limited hidden power pin handling;
- symbol pin geometry not consistently used by generated schematic placement;
- no robust distinction between normal symbols, power symbols, aliases, and
  inherited variants;
- incomplete project/global `sym-lib-table` resolution;
- limited unsupported-feature diagnostics for AI feedback;
- incomplete resolver-backed validation for component records and pinmaps.

## 5. Inputs

The resolver must support these symbol sources:

- configured symbol library root, typically `$KICAD_SYMBOLS_DIR`;
- project-local `sym-lib-table`;
- global KiCad `sym-lib-table` when explicitly configured;
- checked-in or test fixture libraries under `internal/.../testdata`;
- component catalog symbol IDs such as `Device:R`;
- block definitions and transaction `add_symbol` operations.

The implementation must avoid hard-coding user-specific absolute paths. Use
flags, environment variables, config, or explicit test fixtures.

## 6. Data Model

### 6.1 Symbol Library Index

```go
type SymbolLibraryIndex struct {
    GeneratedAt time.Time
    Roots       []SymbolRoot
    Libraries   map[string]SymbolLibraryRecord
    Symbols     map[string]SymbolRecord
    Diagnostics []reports.Issue
}
```

### 6.2 Symbol Library Record

```go
type SymbolLibraryRecord struct {
    Nickname string
    Path     string
    Source   string // root, project_table, global_table, fixture
    Symbols  []string
}
```

### 6.3 Symbol Record

```go
type SymbolRecord struct {
    LibraryID       string
    LibraryNickname string
    Name            string
    Extends         string
    Description     string
    Keywords        []string
    Datasheet       string
    FootprintFilter []string
    Properties      map[string]string
    Units           []SymbolUnit
    Pins            []SymbolPin
    Graphics        []SymbolGraphic
    PowerSymbol     bool
    Inherited       bool
    Diagnostics     []reports.Issue
}
```

`SymbolRecord.Pins` and `SymbolRecord.Graphics` are the canonical flat source
of truth. `SymbolUnit` entries are deterministic derived views for callers that
need unit-level grouping. Code that mutates or validates pins must read from the
flat `SymbolRecord.Pins` list; `SymbolUnit.PinIndexes` and
`SymbolUnit.CommonPinIndexes` must be rebuilt from the flat list after parsing
or transformation.

### 6.4 Symbol Unit

```go
type SymbolUnit struct {
    Unit             int
    BodyStyle        int
    PinIndexes       []int
    Graphics         []SymbolGraphic
    CommonPinIndexes []int
}
```

Rules:

- Unit numbers are 1-based.
- Unit `0` represents pins common to all units or package-level bindings.
- KiCad UI labels such as `A`, `B`, and `C` map to units `1`, `2`, and `3`.
- A physical component record should normally bind the whole package, not one
  isolated unit, unless it intentionally models a single gate.

### 6.5 Symbol Pin

```go
type SymbolPin struct {
    Number         string
    Name           string
    ElectricalType string
    Unit           int
    BodyStyle      int
    Position       kicadfiles.Point
    Orientation    string
    Length         kicadfiles.IU
    Hidden         bool
}
```

Rules:

- `Number` is the unique KiCad pin identifier used for netlists and footprint
  pad mapping.
- `Name` is display text and must not be used as the unique pad mapping key.
- Hidden pins are not automatically safe; power hidden pins require explicit
  policy or verified component metadata.
- Electrical types must preserve KiCad semantics such as input, output,
  bidirectional, tri-state, passive, free, unspecified, power input, power
  output, open collector, open emitter, and no-connect.
- `ElectricalType` must be normalized to a string-backed enum with constants in
  code.
- Global connectivity behavior is derived from KiCad semantics, such as hidden
  power-input pins, not from a symbol-pin global-label field.

### 6.6 Symbol Graphics

The resolver should preserve enough symbol graphics for future schematic
placement and review, but graphics are not the primary acceptance gate for this
project.

```go
type SymbolGraphic struct {
    Kind      string
    Unit      int
    BodyStyle int
    Bounds    kicadfiles.Bounds
}
```

## 7. Parser Requirements

The `.kicad_sym` parser must support:

- root `kicad_symbol_lib` metadata;
- `symbol` entries;
- `extends`;
- `property`;
- `pin`;
- pin `name`, `number`, `type`, `at`, `length`, `hide`;
- unit/body-style suffixes used in KiCad symbol definitions;
- graphical primitives needed to compute rough bounds;
- aliases or inherited symbol variants where present;
- unknown nodes preserved or reported without panics.

Unsupported symbol features must produce structured diagnostics rather than
silent partial success.
The S-expression parser path used for symbol libraries must enforce a maximum
nesting depth of 100 levels so malicious or corrupt input cannot exhaust the
stack.

## 8. Resolution Rules

Resolution order:

1. project-local `sym-lib-table`;
2. configured global KiCad table/root;
3. configured explicit symbol roots as a virtual-table fallback for development
   and tests;
4. checked-in fixtures for tests only.

Project and global `sym-lib-table` entries are authoritative when present.
Explicit roots must not override project table mappings; they only populate a
fallback virtual table for libraries that are otherwise unavailable.

If the same `LibraryNickname:SymbolName` appears in multiple sources, project
scope wins. Ties inside the same scope are blocking ambiguity diagnostics.

Symbol IDs must use KiCad `Library:Symbol` format. Missing nicknames, missing
symbols, invalid IDs, and ambiguous IDs must block acceptance levels that depend
on verified symbol evidence.

## 9. Validation Rules

The resolver must detect:

- missing symbol library files;
- malformed S-expression syntax;
- duplicate library nicknames in the same scope;
- duplicate symbol IDs in the same scope;
- invalid `Library:Symbol` identifiers;
- symbols without pins where pins are required;
- duplicate pin numbers across the whole `SymbolRecord` when not explicitly
  allowed. Stacked duplicate pin numbers are allowed only when they have the
  same position and compatible electrical type, and must be treated as one
  logical connection. Multiple physical pins with different pin numbers but the
  same name/net are not duplicate pin numbers; they remain distinct pins and
  require explicit pinmap/component policy;
- multi-unit symbols used without explicit unit policy;
- hidden power pins without explicit handling policy;
- unknown electrical types;
- unresolved inherited base symbols;
- unsupported symbol constructs in acceptance-critical paths.

Severity policy:

- Parser failures are blocking for the affected library.
- Unsupported non-critical graphics are warnings.
- Unsupported pin/unit semantics are blocking when the symbol is used for
  generation, pinmap validation, or connectivity acceptance.

## 10. Integration Points

### 10.1 Component Intelligence

Component evidence checks must use real symbol records:

- verify `SymbolBinding.SymbolID` resolves;
- verify `FunctionPin.SymbolPin` exists as a real pin number;
- validate electrical type compatibility when requested;
- validate unit/common-pin semantics for multi-unit records;
- reject verified active components without compatible symbol evidence.

### 10.2 Pinmap Validation

Pinmap validation must compare symbol pin numbers against footprint pad names.
Pin names may be used as hints only, never as the primary mapping key.

### 10.3 Schematic Writer And Transactions

`add_symbol` planning should optionally hydrate pin geometry from the resolver.
When hydrated geometry is available, generated wires should connect to real pin
locations instead of block-local approximations.

### 10.4 Design Workflow

`design create` should report resolver-backed symbol issues in a distinct stage
or in existing component/library stages with clear issue paths. Unsupported
symbols must block before project writes for connectivity or stronger
acceptance.

### 10.5 Writer Correctness

Writer correctness should detect unresolved symbols and mismatched symbol pin
references when resolver data is available.

## 11. CLI Requirements

Add or extend machine-readable commands under the existing `library` command
family:

```sh
kicadai --json library symbols list
kicadai --json library symbols show Device:R
kicadai --json library symbols pins Device:R
kicadai --json library symbols validate Device:R
```

Output must include:

- symbol ID;
- source path;
- units;
- pins;
- electrical types;
- hidden/common flags;
- unsupported features;
- diagnostics.

## 12. Test Strategy

Required tests:

- parse simple passive symbol fixture;
- parse power symbol fixture;
- parse multi-unit fixture with common pins;
- parse inherited symbol fixture;
- reject malformed symbol file;
- reject duplicate symbol ID in same scope;
- resolve project-local symbol before global/root symbol;
- validate `FunctionPin.SymbolPin` against real pin numbers;
- validate hidden power pin policy;
- hydrate transaction pin geometry from resolver evidence;
- preserve deterministic JSON output for CLI symbol inspection.

External tests against `$KICAD_SYMBOLS_DIR` may exist but must be opt-in and
skipped by default.

## 13. Acceptance Criteria

- Common symbols used by existing blocks resolve with real pin metadata.
- Multi-unit symbols either resolve safely or block with actionable diagnostics.
- Power symbols and hidden pins are not silently treated as normal passive pins.
- Component evidence validation uses real symbol pins for supported fixtures.
- Generated schematic transactions can use resolver-backed pin geometry when
  available.
- Full unit suite passes without external KiCad repositories.
- Optional external integration tests can validate against a local KiCad symbols
  checkout when configured.

## 14. Open Questions

- Should symbol hydration be part of `libraryresolver` or split into a dedicated
  `symbolresolver` package?
- Should schematic writer output embed missing project-local symbols, or block
  until library tables are present?
- How much symbol graphics data is needed before placement/wiring quality
  materially improves?
- Should hidden power pins be connected automatically only for verified power
  symbols, or always require explicit component metadata?
