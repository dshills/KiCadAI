# KiCad Library Resolver Implementation Plan

## 1. Objective

Implement the KiCad library resolver described in `SPEC.md` in small, reviewable
phases. The end state is a resolver that indexes local KiCad symbol and
footprint repositories, exposes JSON CLI commands, validates symbol-footprint
compatibility conservatively, and provides data needed by transactions, pinmap
validation, and future AI design generation.

This plan assumes the local reference repositories are available at:

```text
/Users/dshills/Development/external/klc
/Users/dshills/Development/external/kicad-footprints
/Users/dshills/Development/external/kicad-symbols
/Users/dshills/Development/external/kicad-templates
```

Normal unit tests must not require those external repositories. Integration
tests that use them must be opt-in.

## 2. Implementation Rules

- Keep each phase independently useful and commit-sized.
- Prefer the existing `internal/kicadfiles/sexpr` parser over string parsing.
- Return structured `reports.Issue` diagnostics instead of panics or raw errors
  for malformed library files.
- All CLI output must use `reports.Result`.
- Resolver output must be deterministic: sorted IDs, stable diagnostics, stable
  JSON.
- Do not write into external KiCad repositories.
- Do not claim compatibility or fabrication readiness from inferred mappings
  unless a rule is explicitly trusted.
- Run `gofmt` and `GOCACHE=/tmp/kicadai-gocache go test ./...` before each
  commit.
- Run `prism review staged` before each phase commit and address concrete
  correctness findings.

## 3. Phase 1: Package Skeleton and Configuration

### Goal

Create the resolver package, core data model, root discovery, and configuration
surface without parsing KiCad library files yet.

### Work

- Add `internal/libraryresolver`.
- Define:
  - `LibraryRoots`;
  - `LibraryIndex`;
  - `SymbolRecord`;
  - `FootprintRecord`;
  - `SymbolPin`;
  - `FootprintPad`;
  - `LoadOptions`;
  - `Query`;
  - `MatchOptions`;
  - `CompatibilityResult`.
- Add default root discovery:
  - environment variables first;
  - project/user config file support in a later phase;
  - empty roots allowed with diagnostics.
- Add path validation:
  - root exists;
  - root is a directory;
  - no path traversal in cache path;
  - symlink roots are allowed when they resolve to directories.
- Add deterministic issue helpers for root diagnostics.

### Tests

- Root config from environment variables.
- Empty roots produce clear warnings when no environment/config values are set.
- Missing roots produce warnings, not panics.
- Invalid cache path rejected.
- Data model JSON marshals with expected field names.

### Acceptance Criteria

- `internal/libraryresolver` compiles.
- No CLI changes yet.
- Unit tests pass without local external repositories.

### Commit Message

```text
Add library resolver configuration foundation
```

## 4. Phase 2: Library Discovery

### Goal

Discover symbol and footprint library files from configured roots and report a
deterministic inventory.

### Work

- Implement root walking for:
  - `.kicad_sym`;
  - `.kicad_symdir/*.kicad_sym`;
  - `.pretty/*.kicad_mod`.
- Derive library nicknames:
  - `Device.kicad_sym` -> `Device`;
  - `Foo.kicad_symdir/bar.kicad_sym` -> `Foo`;
  - `Resistor_SMD.pretty/R_0805_2012Metric.kicad_mod` -> `Resistor_SMD`.
- Add discovery records:
  - source path;
  - library nickname;
  - kind: symbol or footprint;
  - expected full ID prefix.
- Sort records by kind, nickname, and path.
- Add detection for duplicate candidate IDs at discovery level where possible.
- Do not parse record contents yet.

### Tests

- Fixture tree with one `.kicad_sym`.
- Fixture tree with one `.kicad_symdir`.
- Fixture tree with one `.pretty`.
- Non-KiCad files ignored.
- Deterministic sorted output.
- Duplicate nickname/path diagnostics.

### Acceptance Criteria

- Resolver can report inventory counts.
- Malformed or irrelevant files do not abort discovery.

### Commit Message

```text
Discover KiCad symbol and footprint libraries
```

## 5. Phase 3: Symbol Parser

### Goal

Parse KiCad symbol library files into `SymbolRecord` entries.

### Work

- Use `internal/kicadfiles/sexpr` to parse symbol files.
- Support root `kicad_symbol_lib`.
- Parse each `symbol` node:
  - symbol name;
  - nested/extended symbol metadata where visible;
  - properties;
  - description;
  - keywords;
  - datasheet;
  - footprint filters;
  - pins;
  - units/body styles;
  - hidden pins.
- Generate full `LibraryID`.
- Preserve raw symbol node for future embedding.
- Add parse diagnostics:
  - invalid root;
  - malformed S-expression;
  - symbol without name;
  - duplicate symbol ID.
- Keep indexing tolerant: one bad file reports an issue and the rest continue.

### Tests

- Simple symbol fixture.
- Symbol with `ki_description`, `ki_keywords`, `ki_fp_filters`.
- Multi-unit symbol fixture.
- Hidden power pin fixture.
- Malformed symbol file diagnostic.
- Duplicate symbol ID diagnostic.
- Golden symbol record JSON for a small fixture.

### Acceptance Criteria

- `ResolveSymbol(index, "Device:R")` works for fixtures.
- Symbol records include pin identifiers and metadata needed by pinmap logic.

### Commit Message

```text
Parse KiCad symbol library records
```

## 6. Phase 4: Footprint Parser

### Goal

Parse KiCad footprint files into `FootprintRecord` entries.

### Work

- Use `internal/kicadfiles/sexpr` to parse `.kicad_mod`.
- Support root `footprint`.
- Parse:
  - footprint name;
  - description;
  - tags;
  - attributes;
  - properties;
  - pads;
  - pad type, shape, position, size, drill, layers, pinfunction, pintype;
  - text records;
  - graphics summary;
  - courtyard/fab/silk presence;
  - model paths.
- Compute a conservative bounding box from pads and supported graphic points.
- Generate full `FootprintID`.
- Preserve raw footprint node for future instantiation.
- Add parse diagnostics:
  - invalid root;
  - malformed S-expression;
  - footprint without name;
  - pad without name;
  - duplicate footprint ID.

### Tests

- SMD footprint fixture.
- Through-hole footprint fixture.
- Footprint with duplicate pad names.
- Footprint with courtyard/fab/silk graphics.
- Footprint with 3D model.
- Malformed footprint diagnostic.
- Golden footprint record JSON for a small fixture.

### Acceptance Criteria

- `ResolveFootprint(index, "Resistor_SMD:R_0805_2012Metric")` works for
  fixtures.
- Footprint records include real pad geometry and layer sets.

### Commit Message

```text
Parse KiCad footprint library records
```

## 7. Phase 5: Index Load and Query API

### Goal

Provide the primary resolver API for loading, resolving, and searching library
records.

### Work

- Implement:
  - `Load(ctx, roots, opts)`;
  - `ResolveSymbol`;
  - `ResolveFootprint`;
  - `FindSymbols`;
  - `FindFootprints`.
- Add context cancellation checks during tree walking and parsing.
- Add query matching by:
  - exact ID;
  - nickname;
  - name substring;
  - description/keyword/tag substring.
- Add deterministic result ordering.
- Add aggregate load summary:
  - symbol file count;
  - footprint file count;
  - symbol count;
  - footprint count;
  - diagnostic count.

### Tests

- Load mixed fixture tree.
- Resolve exact symbol.
- Resolve exact footprint.
- Search symbols by keyword.
- Search footprints by tag.
- Context cancellation returns a controlled error.
- Deterministic repeated load output.

### Acceptance Criteria

- Public Go API is usable by tests and future CLI phase.
- No command-line surface yet.

### Commit Message

```text
Add library resolver load and query API
```

## 8. Phase 6: CLI Library Commands

### Goal

Expose resolver data through JSON CLI commands.

### Work

- Add `library` command family to `cmd/kicadai`.
- Add flags or environment support for:
  - `--klc-root`;
  - `--symbols-root`;
  - `--footprints-root`;
  - `--templates-root`;
  - `--library-cache`;
  - `--refresh-library-cache`.
- Implement:
  - `kicadai library index --json`;
  - `kicadai library symbol <symbol-id> --json`;
  - `kicadai library footprint <footprint-id> --json`;
  - `kicadai library search-symbols <query> --json`;
  - `kicadai library search-footprints <query> --json`.
- Return `reports.Result`.
- Return `MISSING_FILE` or `VALIDATION_FAILED` issues for missing roots and bad
  files.
- Add help text and structured argument validation.

### Tests

- `library` requires `--json`.
- `library index` fixture output includes counts.
- `library symbol Device:R` returns a symbol record.
- `library footprint Resistor_SMD:R_0805_2012Metric` returns a footprint record.
- Unsupported subcommand returns structured error.
- Missing ID returns structured error.

### Acceptance Criteria

- CLI can inspect fixture libraries without external roots.
- Normal full test suite passes.

### Commit Message

```text
Expose library resolver CLI queries
```

## 9. Phase 7: Compatibility Engine

### Goal

Validate symbol-footprint assignments conservatively and rank candidate
footprints.

### Work

- Implement:
  - `ValidateAssignment(index, symbolID, footprintID)`;
  - `CompatibleFootprints(index, symbolID, opts)`.
- Add status values:
  - `compatible`;
  - `candidate`;
  - `needs_verification`;
  - `incompatible`;
  - `unknown`.
- Implement first compatibility rules:
  - missing symbol blocks;
  - missing footprint blocks;
  - zero-pad footprint with electrical symbol pins blocks;
  - pad count mismatch blocks unless duplicate/mechanical pads explain it;
  - symbol footprint filters improve score;
  - package family/name match improves score;
  - passives with numeric pins and matching pad count may be `compatible` only
    for explicitly trusted generic rules;
  - everything else with matching count is `needs_verification`.
- Add evidence records explaining each score/status decision.
- Add `PINMAP_UNVERIFIED` issues for uncertain mappings.

### CLI

Implement:

```text
kicadai library compatible-footprints <symbol-id> --json
kicadai library validate-assignment <symbol-id> <footprint-id> --json
```

### Tests

- Missing symbol blocks.
- Missing footprint blocks.
- Resistor to 0805 resistor compatible.
- Resistor to SOIC footprint incompatible.
- Transistor to TO-92 returns `needs_verification` unless verified mapping
  exists.
- Compatible-footprints returns deterministic ranking.

### Acceptance Criteria

- AI callers can ask for candidate footprints and receive explicit uncertainty.

### Commit Message

```text
Add symbol footprint compatibility checks
```

## 10. Phase 8: Pinmap Candidate Generation

### Goal

Generate candidate pinmaps from resolver records without marking them as
verified.

### Work

- Implement `PinmapCandidate(index, symbolID, footprintID)`.
- Candidate logic:
  - exact numeric pin-to-pad matches;
  - symbol pin name/function hints;
  - footprint `pinfunction` hints;
  - duplicate pad names handled as groups;
  - ambiguous mappings reported with lower confidence.
- Add issue behavior:
  - candidate mappings return `PINMAP_UNVERIFIED`;
  - impossible mappings return blocking mismatch issue;
  - verified mappings can be supplied by existing pinmap package later.
- Include confidence and reason per candidate row.

### CLI

Implement:

```text
kicadai library pinmap-candidate <symbol-id> <footprint-id> --json
```

### Tests

- Numeric two-pin passive candidate.
- Connector candidate with matching pins.
- Transistor candidate with explicit warning/blocked verification status.
- Duplicate pad footprint candidate.
- Mismatched count returns blocking issue.

### Acceptance Criteria

- Resolver can propose reviewable pinmap candidates for human verification.
- No candidate is labeled `human_verified`.

### Commit Message

```text
Generate library pinmap candidates
```

## 11. Phase 9: Pinmap Package Integration

### Goal

Make existing `internal/pinmap` consume resolver records for stronger validation.

### Work

- Add resolver-backed validation path.
- Validate actual footprint pad identifiers, not just built-in map counts.
- Preserve current built-in verified mappings.
- Add optional root/config wiring to `pinmap validate`.
- Report resolver evidence in pinmap output.
- Keep hierarchy blocker until schematic hierarchy flattening exists.
- Distinguish statuses:
  - `verified`;
  - `candidate`;
  - `missing`;
  - `mismatch`;
  - `unsupported_hierarchy`.

### Tests

- Existing pinmap tests still pass.
- Verified resistor mapping validates against real fixture footprint pads.
- Missing footprint library record blocks.
- Pinmap says candidate when resolver can infer but no verified mapping exists.
- Notes and evidence appear in output.

### Acceptance Criteria

- `pinmap validate` can use real symbol and footprint data when available.

### Commit Message

```text
Use library resolver in pinmap validation
```

## 12. Phase 10: Transaction Planning Integration

### Goal

Use resolver data to catch bad library IDs and improve transaction plans.

### Work

- Add optional resolver configuration to transaction planning.
- Validate:
  - `add_symbol.library_id` exists;
  - `assign_footprint.footprint_id` exists;
  - `place_footprint.footprint_id` exists when provided.
- Add suggestions:
  - compatible footprints for unresolved or missing assignment;
  - pinmap verification needed for `connect`;
  - footprint geometry can be enriched from resolver.
- Keep planning usable without configured library roots; missing resolver should
  warn, not block, unless command explicitly requests library validation.

### Tests

- Bad symbol ID emits structured issue.
- Bad footprint ID emits structured issue.
- Valid fixture symbol/footprint passes.
- No roots configured produces warning but not a panic.
- Existing transaction tests remain compatible.

### Acceptance Criteria

- AI-generated transactions get library feedback before write/apply.

### Commit Message

```text
Validate transaction libraries with resolver
```

## 13. Phase 11: Footprint Instantiation Integration

### Goal

Allow PCB writer/design builder to instantiate real footprints from resolver
records.

### Work

- Add conversion from `FootprintRecord` to `pcb.Footprint`.
- Preserve:
  - pads;
  - pad geometry;
  - pad layer sets;
  - properties;
  - graphics;
  - attributes;
  - models where currently modeled by writer.
- Add fallback behavior:
  - if resolver unavailable, use current minimal footprint logic;
  - if footprint record incomplete, block fabrication readiness or produce
    warning depending on severity.
- Update `PlaceFootprint` flow to use real footprint data when `footprint_id`
  resolves.

### Tests

- Resistor SMD fixture instantiates real pads.
- Through-hole connector fixture instantiates drills and all-copper/all-mask
  layers.
- Bottom-side placement mirrors or assigns correct layer sets according to KiCad
  conventions.
- Generated PCB validates with existing writer validation.
- Existing minimal placement tests still pass.

### Acceptance Criteria

- Generated PCBs no longer rely on hardcoded pad geometry when a resolver record
  is available.

### Commit Message

```text
Instantiate PCB footprints from library records
```

## 14. Phase 12: Symbol Instantiation Integration

### Goal

Allow schematic writer/design builder to use real symbol pin data from resolver
records.

### Work

- Add conversion from `SymbolRecord` to schematic symbol placement defaults.
- Use real pins for:
  - pin numbers;
  - pin offsets;
  - units;
  - hidden pin awareness.
- Preserve existing schematic writer validation.
- Support generic passives first, then multi-unit components.
- Block unsupported complex symbol features rather than silently dropping them.

### Tests

- Resistor symbol placement uses real pins.
- Connector symbol placement uses real pins.
- Multi-unit symbol produces clear blocked issue if not fully supported.
- Generated schematic validates.

### Acceptance Criteria

- AI can place symbols with real library pin data for common parts.

### Commit Message

```text
Instantiate schematic symbols from library records
```

## 15. Phase 13: KLC Rule Layer

### Goal

Add resolver-critical KLC checks as structured diagnostics.

### Work

- Add curated KLC checks for:
  - symbol metadata;
  - footprint metadata;
  - pad names;
  - text presence;
  - courtyard/fab/silk presence;
  - attribute consistency.
- Include KLC reference paths when local docs can be identified.
- Keep style issues as warnings.
- Use blocking severity only when correctness is affected.

### CLI

Add optional:

```text
kicadai library klc-symbol <symbol-id> --json
kicadai library klc-footprint <footprint-id> --json
```

Or include KLC diagnostics in existing symbol/footprint output behind a flag.

### Tests

- Missing metadata warning.
- Missing pad name error.
- Missing courtyard warning.
- Valid fixture has no blocking KLC issues.

### Acceptance Criteria

- Resolver can surface KLC-relevant issues without depending on KiCad GUI.

### Commit Message

```text
Add resolver KLC validation checks
```

## 16. Phase 14: Cache

### Goal

Add a cache file for fast repeated resolver loads.

### Work

- Define cache schema version.
- Store:
  - roots;
  - file metadata;
  - symbol records;
  - footprint records;
  - diagnostics;
  - generated timestamp.
- Add invalidation by:
  - schema version;
  - root changes;
  - file count changes;
  - file size or modtime changes;
  - `--refresh-library-cache`.
- Add atomic cache write.
- Ensure cache is never written inside external library roots unless explicitly
  configured.

### Tests

- Cache write/read round trip.
- Cache invalidated by changed file modtime.
- Cache invalidated by schema version.
- `--refresh` rebuilds.
- Corrupt cache falls back to rebuild with diagnostic.

### Acceptance Criteria

- Repeated CLI calls can load cached index.

### Commit Message

```text
Cache KiCad library resolver index
```

## 17. Phase 15: Template Inventory

### Goal

Index KiCad templates enough to expose project-structure examples.

### Work

- Discover templates root.
- Index template directories.
- Extract:
  - template name;
  - project files;
  - metadata files;
  - library tables;
  - board/schematic files when present.
- Add CLI:

```text
kicadai library templates --json
kicadai library template <name> --json
```

### Tests

- Fixture template directory indexed.
- Missing templates root warning.
- Deterministic sorted output.

### Acceptance Criteria

- Future generators can query available template scaffolds.

### Commit Message

```text
Index KiCad project templates
```

## 18. Phase 16: Opt-In Integration Tests Against Local Repositories

### Goal

Verify resolver behavior against the actual local KiCad library checkouts.

### Work

- Add integration tests guarded by:

```text
KICADAI_RUN_LIBRARY_INTEGRATION=1
KICADAI_SYMBOLS_ROOT=/Users/dshills/Development/external/kicad-symbols
KICADAI_FOOTPRINTS_ROOT=/Users/dshills/Development/external/kicad-footprints
KICADAI_KLC_ROOT=/Users/dshills/Development/external/klc
KICADAI_TEMPLATES_ROOT=/Users/dshills/Development/external/kicad-templates
```

- Test known lookups:
  - a resistor symbol;
  - a capacitor symbol;
  - an LED symbol;
  - a resistor SMD footprint;
  - a pin header footprint.
- Test compatibility:
  - resistor to 0805 resistor;
  - connector symbol to pin header;
  - transistor to TO-92 candidate/needs verification.
- Keep tests skipped by default.

### Acceptance Criteria

- Normal `go test ./...` does not require external roots.
- Opt-in integration tests pass on the user's machine.

### Commit Message

```text
Add KiCad library resolver integration tests
```

## 19. Phase 17: Documentation

### Goal

Document resolver setup, CLI usage, integration with generation, and safety
boundaries.

### Work

- Update `README.md`.
- Add `docs/library-resolver.md`.
- Document:
  - root configuration;
  - cache behavior;
  - CLI commands;
  - compatibility statuses;
  - pinmap candidate versus verified mapping;
  - known limitations;
  - integration-test environment variables.
- Add examples:
  - resolving `Device:R`;
  - resolving `Resistor_SMD:R_0805_2012Metric`;
  - validating assignment;
  - generating pinmap candidate.

### Tests

- None required beyond markdown link sanity if available.

### Acceptance Criteria

- A new developer can configure and run resolver commands from documentation.

### Commit Message

```text
Document KiCad library resolver
```

## 20. Cross-Phase Risks

### 20.1 Symbol Format Complexity

KiCad symbols may use inheritance, split units, alternate body styles, and
hidden pins. Mitigation: parse visible fields first and block unsupported
features with diagnostics instead of pretending complete support.

### 20.2 Footprint Geometry Completeness

Footprints contain many graphics and advanced pad options. Mitigation: extract
all fields needed for pad correctness first; preserve raw data for future
writer expansion.

### 20.3 False Compatibility

Matching pin count is not proof of correctness. Mitigation: status must remain
`needs_verification` unless a rule or verified pinmap supports compatibility.

### 20.4 Performance

Full KiCad libraries are large. Mitigation: deterministic walking, efficient
S-expression parsing, and cache phase.

### 20.5 AI Misuse

AI may over-trust candidates. Mitigation: report statuses explicitly and block
fabrication readiness on unverified candidates.

## 21. Final Definition of Done

The full resolver project is complete when:

- local KiCad symbol and footprint repositories can be indexed;
- common symbol and footprint records can be resolved by ID;
- compatibility commands return conservative, explainable results;
- pinmap candidates can be generated but not mislabeled as verified;
- existing pinmap validation uses resolver data when available;
- transactions get resolver-backed symbol/footprint validation;
- PCB placement can instantiate real footprint pad geometry;
- schematic placement can use real symbol pin data for common parts;
- KLC-critical issues are reported;
- cache and template inventory are available;
- default tests pass without external roots;
- opt-in integration tests pass against the provided local repositories;
- prism review is clean or has only documented non-blocking findings for each
  phase.
