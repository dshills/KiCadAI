# KiCad Library Resolver Specification

## 1. Purpose

Build a KiCad library resolver that can index the local KiCad reference
repositories, resolve real symbols and footprints, validate symbol-footprint
compatibility, and feed verified library data into the AI design pipeline.

This is the next foundation layer between high-level AI intent and the existing
KiCad file writers. The writer can already emit KiCad-native project,
schematic, and PCB files; the resolver will let it use real KiCad library
objects rather than minimal synthetic symbol and footprint approximations.

## 2. Local Reference Roots

The resolver must support the local repositories provided by the user:

```text
KiCad Library Conventions: /Users/dshills/Development/external/klc
KiCad Footprint Libraries: /Users/dshills/Development/external/kicad-footprints
KiCad Symbols Library:    /Users/dshills/Development/external/kicad-symbols
KiCad Templates Library:  /Users/dshills/Development/external/kicad-templates
```

These paths should be defaults for this development machine, but the
implementation must not hard-code them as the only possible locations. It must
support configuration by flags, environment variables, and a project-local config
file.

Suggested environment variables:

```text
KICADAI_KLC_ROOT
KICADAI_SYMBOLS_ROOT
KICADAI_FOOTPRINTS_ROOT
KICADAI_TEMPLATES_ROOT
KICADAI_LIBRARY_CACHE
```

## 3. Goals

The resolver must:

- discover KiCad symbol libraries and footprint libraries from local checkouts;
- parse `.kicad_sym`, `.kicad_symdir/*.kicad_sym`, and `.kicad_mod` files;
- build a queryable symbol and footprint index;
- expose stable library IDs in KiCad format, such as `Device:R` and
  `Resistor_SMD:R_0805_2012Metric`;
- extract symbol pins, pin names, electrical types, units, and metadata;
- extract footprint pads, pad names, pad geometry, layer sets, courtyard/fab/silk
  metadata, attributes, model references, and bounding boxes;
- propose compatible footprints for a symbol;
- validate explicit symbol-to-footprint assignments;
- produce pinmap candidates from symbol pins and footprint pads;
- integrate with existing transaction planning, pinmap validation, and project
  generation;
- provide machine-readable CLI output suitable for AI agents;
- avoid silently accepting uncertain mappings as fabrication-ready.

## 4. Non-Goals

Initial implementation does not need to:

- download libraries from the network;
- modify upstream KiCad library repositories;
- implement every KLC rule on day one;
- infer manufacturer-specific pinouts without a verified mapping;
- guarantee that every footprint with matching pad count is electrically correct;
- generate placement or routing decisions;
- replace KiCad's own ERC/DRC;
- support arbitrary third-party library formats beyond KiCad S-expression files.

## 5. Repository Layout Assumptions

The resolver must handle these observed layouts:

### 5.1 Symbol Libraries

The current local symbols checkout includes directories with `.kicad_symdir`
suffixes containing one or more `.kicad_sym` files:

```text
kicad-symbols/
  MCU_Microchip_AVR_Dx.kicad_symdir/
    AVR64DA28x-xSS.kicad_sym
    AVR64DB48x-xPT.kicad_sym
```

It should also support traditional single-file library layouts:

```text
kicad-symbols/
  Device.kicad_sym
  Connector.kicad_sym
```

The library nickname must be derived from the root file or directory name:

- `Device.kicad_sym` -> `Device`
- `MCU_Microchip_AVR_Dx.kicad_symdir/AVR64DA28x-xSS.kicad_sym` ->
  `MCU_Microchip_AVR_Dx`

Each parsed symbol gets a full library ID:

```text
<library-nickname>:<symbol-name>
```

### 5.2 Footprint Libraries

Footprint libraries use KiCad `.pretty` directories:

```text
kicad-footprints/
  Resistor_SMD.pretty/
    R_0805_2012Metric.kicad_mod
  Connector_PinHeader_2.54mm.pretty/
    PinHeader_1x02_P2.54mm_Vertical.kicad_mod
```

The footprint library nickname is the directory name without `.pretty`.

Each parsed footprint gets a full footprint ID:

```text
<library-nickname>:<footprint-name>
```

### 5.3 Templates

Template repositories are reference material for:

- project directory structure;
- expected library table shape;
- KiCad project metadata defaults;
- board setup defaults;
- common file naming conventions.

Templates are not primary symbol or footprint sources in the first resolver
phase.

### 5.4 KLC

The KLC repository is the source for convention checks and human-readable rule
references. The implementation should not assume the KLC content has a stable
machine-readable schema unless one is discovered. Initial KLC support may be a
small curated rule layer implemented in Go with references to local KLC docs.

## 6. Data Model

### 6.1 LibraryIndex

```go
type LibraryIndex struct {
    GeneratedAt time.Time
    Roots       LibraryRoots
    Symbols     map[string]SymbolRecord
    Footprints  map[string]FootprintRecord
    Diagnostics []reports.Issue
}
```

### 6.2 LibraryRoots

```go
type LibraryRoots struct {
    KLCRoot        string
    SymbolsRoot    string
    FootprintsRoot string
    TemplatesRoot  string
}
```

### 6.3 SymbolRecord

```go
type SymbolRecord struct {
    LibraryID       string
    LibraryNickname string
    Name            string
    Path            string
    Description     string
    Keywords        []string
    Datasheet       string
    FootprintFilter []string
    Properties      map[string]string
    Units           []SymbolUnit
    Pins            []SymbolPin
    Raw             string
}
```

### 6.4 SymbolPin

```go
type SymbolPin struct {
    Number       string
    Name         string
    Electrical   string
    Unit         int
    BodyStyle    int
    Position     kicadfiles.Point
    Orientation  string
    Length       kicadfiles.IU
    Hidden       bool
    FunctionHint string
}
```

### 6.5 FootprintRecord

```go
type FootprintRecord struct {
    FootprintID     string
    LibraryNickname string
    Name            string
    Path            string
    Description     string
    Tags            []string
    Attributes      []string
    Pads            []FootprintPad
    Texts           []FootprintText
    GraphicsSummary GraphicsSummary
    Models          []string
    BoundingBox     BoundingBox
    Raw             string
}
```

### 6.6 FootprintPad

```go
type FootprintPad struct {
    Name         string
    Type         string
    Shape        string
    Position     kicadfiles.Point
    Size         kicadfiles.Point
    Drill        kicadfiles.IU
    Layers       []kicadfiles.BoardLayer
    PinFunction  string
    PinType      string
    RoundRectR   float64
}
```

### 6.7 CompatibilityResult

```go
type CompatibilityResult struct {
    SymbolID        string
    FootprintID     string
    Status          CompatibilityStatus
    Score           float64
    PinmapCandidate []PinmapCandidate
    Issues          []reports.Issue
    Evidence        []CompatibilityEvidence
}
```

Status values:

```text
compatible
candidate
needs_verification
incompatible
unknown
```

### 6.8 PinmapCandidate

```go
type PinmapCandidate struct {
    SymbolPin    string
    SymbolName   string
    Function     string
    FootprintPad string
    Confidence   float64
    Reason       string
}
```

Pinmap candidates are not equivalent to verified mappings. Anything generated
automatically must be marked as candidate or unverified until a human-verified
database entry exists.

## 7. Parser Requirements

### 7.1 S-Expression Parser

Use the existing structured S-expression parser in `internal/kicadfiles/sexpr`
where possible. Do not parse KiCad files using ad hoc string matching except for
well-contained fallback diagnostics.

### 7.2 Symbol Parsing

The symbol parser must extract at minimum:

- symbol name;
- symbol extends/inheritance information when present;
- properties:
  - `Reference`;
  - `Value`;
  - `Footprint`;
  - `Datasheet`;
  - `ki_keywords`;
  - `ki_description`;
  - `ki_fp_filters`;
- pins:
  - number;
  - name;
  - electrical type;
  - unit;
  - body style;
  - hidden state;
  - position;
  - orientation;
  - length.

It must support symbols nested under root `kicad_symbol_lib` and symbols in
split `.kicad_symdir` files.

### 7.3 Footprint Parsing

The footprint parser must extract at minimum:

- footprint name;
- version/generator when present;
- description;
- tags;
- attributes;
- properties;
- pads:
  - name;
  - type;
  - shape;
  - position;
  - size;
  - drill;
  - layers;
  - pinfunction;
  - pintype;
  - roundrect ratio;
- graphics summary:
  - line count;
  - arc count;
  - circle count;
  - polygon count;
  - text count;
  - courtyard presence;
  - fab outline presence;
  - silk presence;
- 3D model paths;
- bounding box derived from pads and graphics where feasible.

### 7.4 Error Handling

Parsing must be tolerant at the index level:

- one bad file should not abort the whole index;
- parse failures must be reported as `reports.Issue`;
- invalid records must be omitted from successful lookup results unless a
  command explicitly asks for diagnostics;
- every diagnostic must include the source path.

## 8. Indexing

### 8.1 Full Index Build

The resolver must walk symbol and footprint roots and build an in-memory index.

Expected CLI:

```text
kicadai library index --json
```

The command should report:

- roots used;
- number of symbol libraries;
- number of symbols;
- number of footprint libraries;
- number of footprints;
- number of diagnostics;
- cache path when written.

### 8.2 Cache

The resolver should support an optional cache file because KiCad libraries are
large.

Initial cache format:

```text
.kicadai/library-index.json
```

Cache metadata must include:

- cache schema version;
- roots;
- source file path;
- source file size;
- source file mod time;
- content hash if inexpensive enough;
- generated timestamp;
- KiCadAI version.

Cache invalidation must occur when:

- resolver schema version changes;
- root path changes;
- source file count changes;
- any source file size/modtime differs;
- explicit `--refresh` is passed.

The first implementation may build in memory without cache, but the public API
should leave room for cache configuration.

## 9. Resolution API

### 9.1 Go API

New package:

```text
internal/libraryresolver
```

Required functions:

```go
func Load(ctx context.Context, roots LibraryRoots, opts LoadOptions) (LibraryIndex, error)
func ResolveSymbol(index LibraryIndex, libraryID string) (SymbolRecord, bool)
func ResolveFootprint(index LibraryIndex, footprintID string) (FootprintRecord, bool)
func FindSymbols(index LibraryIndex, query Query) []SymbolRecord
func FindFootprints(index LibraryIndex, query Query) []FootprintRecord
func CompatibleFootprints(index LibraryIndex, symbolID string, opts MatchOptions) []CompatibilityResult
func ValidateAssignment(index LibraryIndex, symbolID, footprintID string) CompatibilityResult
func PinmapCandidate(index LibraryIndex, symbolID, footprintID string) CompatibilityResult
```

### 9.2 CLI API

Add `library` command family:

```text
kicadai library index --json
kicadai library symbol <symbol-id> --json
kicadai library footprint <footprint-id> --json
kicadai library search-symbols <query> --json
kicadai library search-footprints <query> --json
kicadai library compatible-footprints <symbol-id> --json
kicadai library validate-assignment <symbol-id> <footprint-id> --json
kicadai library pinmap-candidate <symbol-id> <footprint-id> --json
```

All commands must return the existing `reports.Result` envelope.

### 9.3 Configuration Flags

Suggested flags:

```text
--klc-root
--symbols-root
--footprints-root
--templates-root
--library-cache
--refresh-library-cache
```

These can be global flags or `library`-specific flags. If global flag bloat
becomes an issue, use environment variables plus a config file first.

## 10. Compatibility Rules

Compatibility validation must be explicit and conservative.

### 10.1 Hard Failures

Return `incompatible` with blocking issues when:

- symbol ID does not resolve;
- footprint ID does not resolve;
- footprint has zero pads but symbol has electrical pins;
- symbol has more unique electrical pin numbers than footprint pads and no
  known exception exists;
- footprint pad names conflict with an existing verified pinmap;
- footprint is SMD-only but request requires through-hole, or vice versa;
- pin count mismatch cannot be explained by duplicate pads, mechanical pads, or
  known footprint conventions.

### 10.2 Candidate / Needs Verification

Return `needs_verification` when:

- pin counts match but pin names/functions are ambiguous;
- symbol footprint filters match but no verified pinmap exists;
- footprint name appears compatible by package family but pin order is unknown;
- symbol has generic pins such as `1`, `2`, `3` and footprint pads also match
  numerically;
- KLC metadata is incomplete.

### 10.3 Compatible

Return `compatible` only when:

- both symbol and footprint resolve;
- a verified pinmap exists; or
- the mapping is a trivial passive/connector case covered by trusted built-in
  rules;
- pin count and pin identifiers match;
- footprint filters or package family do not contradict the assignment;
- no blocking KLC/library issues are present.

## 11. Pinmap Integration

The existing `internal/pinmap` package must evolve to consume resolver output.

Required integration:

- use resolver records to validate actual symbol pin identifiers;
- use footprint records to validate actual pad identifiers;
- support verified pinmap entries keyed by:

```text
symbol_id + footprint_id + optional package_variant
```

- include resolver evidence in pinmap validation reports;
- block hierarchical projects unless hierarchy flattening is available;
- distinguish:
  - `verified`;
  - `candidate`;
  - `missing`;
  - `mismatch`;
  - `unsupported_hierarchy`.

Candidate pinmaps should be exportable for human review:

```text
kicadai library pinmap-candidate Device:Q_NPN_BEC Package_TO_SOT_THT:TO-92_Inline --json
```

The output must never mark an automatically inferred candidate as
`human_verified`.

## 12. Writer Integration

### 12.1 Schematic Writer

When adding a symbol from a resolved library record, the writer should:

- use the real library ID;
- use real pin numbers and pin positions;
- preserve units and alternate body styles where supported;
- embed or reference the symbol according to existing project conventions;
- generate symbol instances compatible with KiCad save output.

### 12.2 PCB Writer

When placing a resolved footprint, the writer should:

- instantiate real pad geometry;
- preserve footprint graphics, courtyard, fab, silk, attributes, and 3D models;
- set reference/value properties using KiCad-conventional positions;
- assign nets by pad name using a verified pinmap;
- keep library ID stable;
- avoid hardcoded pad sizes/layers when a real footprint is available.

### 12.3 Transactions

Transaction planning should use resolver data to:

- validate `add_symbol.library_id`;
- validate `assign_footprint.footprint_id`;
- enrich `place_footprint` when `footprint_id` is present;
- block `connect` or fabrication readiness when pinmap is missing;
- suggest compatible footprints for unresolved assignments.

## 13. KLC Validation

Initial KLC support should focus on resolver-relevant checks:

### 13.1 Symbol Checks

- library ID naming appears consistent;
- required metadata exists:
  - description;
  - keywords where expected;
  - datasheet where expected;
- pin numbers are unique within each unit unless KiCad convention permits
  duplicates;
- electrical pin types are present;
- footprint filters are parseable.

### 13.2 Footprint Checks

- footprint has description/tags where expected;
- reference/value text exists;
- courtyard is present for normal components;
- fab outline is present for normal components;
- pad names are non-empty for electrical pads;
- 3D model paths are syntactically valid when present;
- SMD/THT attributes are consistent with pad types.

### 13.3 Reporting

KLC checks must be advisory by default unless they impact correctness. Use:

- warnings for metadata/style issues;
- errors/blocked issues for resolver correctness issues.

Each issue should include:

- code;
- severity;
- source path;
- library ID or footprint ID;
- short message;
- optional KLC reference path.

## 14. Template Integration

The templates repo should be used to improve generated project conventions.

Initial template support:

- index template names and paths;
- inspect template project structure;
- expose `kicadai library templates --json`;
- allow future generators to select a template by name.

Template integration is lower priority than symbols/footprints.

## 15. Report Shapes

All CLI commands must return `reports.Result`.

Example `validate-assignment` response:

```json
{
  "ok": false,
  "command": "library",
  "version": "0.1.0",
  "data": {
    "symbol_id": "Device:Q_NPN_BEC",
    "footprint_id": "Package_TO_SOT_THT:TO-92_Inline",
    "status": "needs_verification",
    "score": 0.72,
    "pinmap_candidate": [
      {
        "symbol_pin": "1",
        "symbol_name": "E",
        "function": "E",
        "footprint_pad": "1",
        "confidence": 0.65,
        "reason": "numeric pin names match but package pinout must be verified"
      }
    ]
  },
  "issues": [
    {
      "code": "PINMAP_UNVERIFIED",
      "severity": "blocked",
      "path": "library.assignment",
      "message": "symbol-footprint assignment requires human pinmap verification"
    }
  ],
  "artifacts": []
}
```

## 16. Security and Safety

The resolver reads local repositories only. It must:

- not execute code from library repositories;
- not follow symlinks outside configured roots unless explicitly enabled;
- avoid path traversal in cache paths;
- avoid writing into external KiCad library repositories;
- treat all parsed files as untrusted input;
- tolerate malformed S-expressions without panics.

## 17. Performance Requirements

Initial target on the provided local KiCad libraries:

- full index build should complete in a practical developer-loop time;
- single symbol or footprint lookup should be near-instant after indexing;
- cache load should be significantly faster than full parse;
- diagnostics should not require reparsing the whole tree when using cache.

The implementation should prefer:

- single tree walk per root;
- bounded memory copies;
- package-level or cached indexes for static built-ins;
- deterministic sorted output for tests and AI reproducibility.

## 18. Testing Requirements

### 18.1 Unit Tests

Add parser fixtures for:

- simple `.kicad_sym`;
- `.kicad_symdir` symbol;
- symbol with multiple units;
- symbol with hidden pins;
- footprint with SMD pads;
- footprint with through-hole pads;
- footprint with duplicate pad names;
- footprint with courtyard/fab/silk/model records;
- malformed files that produce diagnostics rather than panics.

### 18.2 Integration Tests With Local Repos

Add opt-in tests guarded by environment variables:

```text
KICADAI_RUN_LIBRARY_INTEGRATION=1
KICADAI_SYMBOLS_ROOT=/Users/dshills/Development/external/kicad-symbols
KICADAI_FOOTPRINTS_ROOT=/Users/dshills/Development/external/kicad-footprints
KICADAI_KLC_ROOT=/Users/dshills/Development/external/klc
KICADAI_TEMPLATES_ROOT=/Users/dshills/Development/external/kicad-templates
```

Integration tests should verify:

- roots are discoverable;
- a known symbol resolves;
- a known footprint resolves;
- a known resistor symbol and 0805 resistor footprint validate as compatible;
- a known transistor assignment produces `needs_verification` unless a verified
  pinmap exists;
- index diagnostics are bounded and reported.

### 18.3 Golden Tests

Add golden JSON outputs for:

- `library symbol Device:R`;
- `library footprint Resistor_SMD:R_0805_2012Metric`;
- `library compatible-footprints Device:R`;
- `library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric`;
- `library pinmap-candidate Device:Q_NPN_BEC Package_TO_SOT_THT:TO-92_Inline`.

## 19. Acceptance Criteria

The resolver is ready for first use when:

- it indexes the provided symbol and footprint repositories;
- it resolves at least common passives, LEDs, connectors, and transistor records;
- it exposes CLI commands with JSON report envelopes;
- it validates simple verified assignments;
- it blocks or marks uncertain assignments as requiring verification;
- it can generate pinmap candidates without claiming they are verified;
- it can enrich footprint placement with real pad geometry;
- all normal tests pass without requiring local external repositories;
- opt-in integration tests pass against the provided local roots.

## 20. Future Work

After the first resolver implementation:

- add full symbol inheritance support;
- add hierarchy flattening for schematic validation;
- add project-local and user-local verified pinmap databases;
- add library table generation from resolved dependencies;
- add footprint 3D model path validation;
- add package-family classifiers;
- add manufacturer part and distributor data integration;
- add KLC rule coverage beyond resolver-critical checks;
- add template-based project scaffolding;
- feed resolver results into placement/routing constraints.

## 21. Relationship to Autonomous AI Design

This resolver is required before an AI can reliably create complete schematics
and PCBs without human intervention. It closes the gap between plausible KiCad
syntax and real component semantics:

- symbols become real library objects with real pins;
- footprints become real physical packages with real pads;
- assignments can be checked before routing;
- generated netlists can be tied to verified pad mappings;
- uncertain decisions can be surfaced as blocked issues instead of hidden in
  generated files.

Autonomous design should remain blocked from fabrication-ready claims until the
resolver, pinmap validator, KiCad CLI checks, and round-trip checks all agree
that the generated project is structurally and electrically meaningful.
