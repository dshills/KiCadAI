# Symbol Resolver Hardening Implementation Plan

## 1. Objective

Implement the symbol resolver hardening work described in `SPEC.md`.

The end state is a resolver-backed symbol layer that can parse representative
KiCad `.kicad_sym` libraries, model pins/units/common pins/electrical types,
resolve symbols through library tables and roots, validate component symbol
bindings, hydrate schematic operations with real pin geometry, and expose stable
CLI output for AI agents.

## 2. Implementation Rules

- Keep phases independently reviewable and commit-sized.
- Prefer extending existing `internal/libraryresolver` and related APIs unless
  the code shape clearly requires a small internal subpackage.
- Do not require external KiCad repositories in default tests.
- Use compact checked-in fixtures for parser, resolver, and CLI tests.
- Preserve existing public CLI behavior unless explicitly extended.
- Emit structured `reports.Issue` diagnostics for parse, resolution, unit, and
  symbol semantics failures.
- Do not silently claim connectivity readiness from unsupported symbol features.
- Run `gofmt` on edited Go files.
- Run focused tests after each phase.
- Run `go test ./...` before the final phase commit.
- Run `prism review staged` before every phase commit and address material
  findings.

## 3. Phase 1: Symbol Model And Fixture Parser Baseline

### Goal

Create the hardened symbol data model and parse simple `.kicad_sym` fixtures
without changing generation behavior.

### Work

- Extend or add symbol resolver model types:
  - `SymbolLibraryIndex`;
  - `SymbolLibraryRecord`;
  - `SymbolRecord`;
  - `SymbolUnit`;
  - `SymbolPin`;
  - `SymbolGraphic`;
  - symbol parser diagnostics.
- Parse root `kicad_symbol_lib` files from fixture directories.
- Parse simple `symbol`, `property`, and `pin` nodes.
- Extract:
  - symbol name;
  - library nickname;
  - description/keywords/datasheet properties;
  - footprint filters when present;
  - pin number;
  - pin name;
  - pin electrical type;
  - pin position;
  - orientation;
  - length;
  - hidden flag.
- Preserve deterministic sort order for symbols and pins.
- Add test fixtures for:
  - `Device:R`;
  - `Device:C`;
  - a simple connector.

### Tests

- Parse passive fixture with two pins.
- Parse connector fixture with ordered pins.
- Invalid symbol ID format produces a structured issue.
- Malformed fixture produces a blocking parse diagnostic.
- JSON marshal shape is deterministic.

### Acceptance Criteria

- Symbol fixtures parse without external repositories.
- Existing resolver and generator tests still pass.
- No design workflow behavior changes yet.

### Commit Message

```text
Add hardened symbol resolver model
```

## 4. Phase 2: Units, Common Pins, Power Symbols, And Electrical Types

### Goal

Model symbol semantics that are required for safe autonomous schematic
generation.

### Work

- Parse unit/body-style suffixes used in KiCad symbol definitions.
- Normalize unit labels:
  - numeric units are 1-based;
  - parse numeric unit indexes directly from `.kicad_sym` symbol sub-node names;
  - reserve alphabetic labels such as `A` and `B` for optional UI/CLI display;
  - `0` represents common/shared pins.
- Group pins by unit and body style.
- Mark common pins and hidden pins.
- Preserve KiCad electrical type strings in normalized form.
- Detect unknown electrical types.
- Detect duplicate pin numbers in a unit unless explicitly allowed.
- Detect multi-unit symbols that lack explicit unit/common-pin policy.
- Add power symbol detection:
  - symbols from power libraries;
  - symbols with power input/output semantics;
  - hidden power pins.

### Tests

- Parse multi-unit fixture with units A/B and common power pins.
- Parse power symbol fixture.
- Hidden power pin without policy produces blocking diagnostic when used for
  connectivity acceptance.
- Unknown electrical type produces a diagnostic.
- Duplicate pin numbers in the same unit block.
- Duplicate pin numbers across separate units are allowed when modeled
  explicitly.

### Acceptance Criteria

- Unit/common pin behavior is deterministic.
- Unsupported multi-unit and hidden-power cases block safely.

### Commit Message

```text
Model symbol units and power pins
```

## 5. Phase 3: Inheritance, Alternate Bodies, And Graphics Bounds

### Goal

Support common KiCad symbol library composition patterns and preserve enough
graphics metadata for schematic placement/wiring improvements.

### Work

- Parse `extends` relationships.
- Resolve inherited base symbol properties, pins, units, and graphics.
- Mark inherited records with `Inherited`.
- Detect unresolved base symbols as blocking diagnostics.
- Parse alternate body/style variants enough to preserve unit/body style
  metadata.
- Parse graphics primitives needed for rough bounds:
  - rectangle;
  - circle;
  - arc;
  - polyline;
  - text.
- Compute rough per-unit and per-symbol bounds.
- Bounds are local, zero-rotation estimates from parsed primitive coordinates;
  transformed or inherited graphics that cannot be safely flattened should emit
  a warning and be excluded from acceptance-critical bounds.
- Report unsupported non-critical graphics as warnings.

### Tests

- Inherited symbol fixture resolves base pins and overrides properties.
- Missing inherited base blocks.
- Alternate body fixture preserves body style.
- Graphics-only unsupported node produces a warning, not a parser panic.
- Bounds are deterministic for simple fixtures.

### Acceptance Criteria

- Common inherited symbols can be indexed.
- Unsupported inheritance semantics do not silently pass as verified evidence.

### Commit Message

```text
Resolve inherited symbol metadata
```

## 6. Phase 4: Library Table And Root Resolution

### Goal

Resolve symbols through project-local tables, configured roots, and optional
global roots with deterministic precedence.

### Work

- Parse `sym-lib-table` fixture files.
- Resolve library nicknames to paths for:
  - project-local tables;
  - explicit roots;
  - optional global table/root configuration.
- Expand KiCad-style path variables in table URIs, including `${KIPRJMOD}`,
  `${KICAD_SYMBOL_DIR}`, and versioned symbols roots such as
  `${KICAD9_SYMBOL_DIR}` when configured.
- Unresolved variables must produce structured diagnostics for affected table
  entries and must not silently expand to an empty path.
- Implement resolution precedence:
  1. project-local table;
  2. configured global table/root;
  3. explicit roots as virtual-table fallback;
  4. test fixtures only.
- Detect duplicate nicknames in the same scope.
- Detect duplicate symbol IDs in the same scope.
- Allow project-local symbols to shadow root/global symbols.
- Add cache/index metadata for source scope and path.

### Tests

- Project-local symbol wins over root symbol.
- Duplicate nickname in one table blocks.
- Duplicate symbol ID in same scope blocks.
- Missing library path produces a structured issue.
- Missing symbol ID blocks with `Library:Symbol` path context.

### Acceptance Criteria

- Symbol resolution behavior is deterministic and explainable.
- All default tests use fixtures, not external KiCad roots.

### Commit Message

```text
Resolve symbols through library tables
```

## 7. Phase 5: Component Evidence And Pinmap Integration

### Goal

Use real symbol records when validating component catalog records and pinmaps.

### Work

- Extend component evidence validation to accept/use hardened symbol records.
- Keep package dependencies one-way: component intelligence consumes resolver
  APIs and records; `internal/libraryresolver` must not import
  `internal/components`.
- Validate `SymbolBinding.SymbolID` resolves.
- Validate `FunctionPin.SymbolPin` exists as a real symbol pin number.
- Validate optional electrical type compatibility.
- Validate unit/common-pin semantics for multi-unit bindings.
- Reject verified active components without compatible symbol evidence.
- Ensure pinmap validation compares symbol pin numbers to footprint pad names.
- Keep pin names as hints only.

### Tests

- Component record with valid fixture symbol passes.
- Function pin referencing a missing symbol pin blocks.
- Pin name-only mapping does not pass when pin number is wrong.
- Multi-unit component record without unit policy blocks.
- Verified active component without compatible pinmap blocks.
- Passive rule-inferred fixture remains allowed where explicitly permitted.

### Acceptance Criteria

- Component readiness checks can prove symbol pin existence from fixtures.
- Unsafe symbol/pin assumptions produce structured diagnostics.

### Commit Message

```text
Validate components with symbol evidence
```

## 8. Phase 6: Schematic Transaction Hydration

### Goal

Use resolver-backed symbol pin geometry in schematic generation where available.

### Work

- Add an opt-in symbol hydration path for `add_symbol` transactions.
- When a symbol resolves and the requested pins exist:
  - replace block-local pin positions with real symbol pin positions;
  - apply symbol insertion transform, including `at`, rotation, and mirror
    policy, before using the pin positions as schematic wire anchors;
  - preserve block-local positions when resolver data is absent and acceptance
    allows fallback;
  - emit diagnostics when resolver data is required but missing.
- Update connection/wire planning to use hydrated pin anchors.
- Keep generated output deterministic.
- Ensure unsupported multi-unit symbols block before file writes for
  connectivity or stronger acceptance.

### Tests

- Hydrate `Device:R` transaction pin anchors from fixture geometry.
- Missing symbol falls back only at draft/structural acceptance.
- Connectivity acceptance blocks missing resolver geometry.
- Generated wires connect to hydrated pin positions.
- Existing block generation tests remain compatible.

### Acceptance Criteria

- Supported symbols produce real pin-based schematic connectivity.
- Unsafe fallback is gated by acceptance level.

### Commit Message

```text
Hydrate schematic symbols from resolver
```

## 9. Phase 7: CLI Symbol Commands

### Goal

Expose symbol resolver evidence to humans and AI agents.

### Work

- Extend `library` CLI with:

```sh
kicadai --json library symbols list
kicadai --json library symbols show Device:R
kicadai --json library symbols pins Device:R
kicadai --json library symbols validate Device:R
```

- `library symbols list` should default to compact summaries and support limit
  and text/library filters. Full symbol records should require `show` or an
  explicit verbose option.
- Support existing resolver root flags.
- Return stable JSON with:
  - symbol ID;
  - library nickname;
  - source path;
  - properties;
  - units;
  - pins;
  - electrical types;
  - hidden/common flags;
  - diagnostics.
- Add concise structured errors for unsupported subcommands and missing args.

### Tests

- `library symbols list` returns fixture symbols.
- `library symbols show` returns units and properties.
- `library symbols pins` returns deterministic pin list.
- `library symbols validate` reports diagnostics.
- Missing symbol exits with structured error.

### Acceptance Criteria

- AI agents can inspect symbol evidence without reading raw `.kicad_sym` files.

### Commit Message

```text
Expose symbol resolver CLI
```

## 10. Phase 8: Design Workflow And Writer Correctness Integration

### Goal

Make symbol resolver issues visible in AI design workflow and writer correctness
results.

### Work

- Add resolver-backed symbol checks to the design workflow before project write
  for connectivity or stronger acceptance.
- Surface issues with paths that identify:
  - block instance;
  - component role;
  - symbol ID;
  - missing pin number;
  - unsupported unit/common-pin state.
- Extend writer correctness to flag unresolved symbols when resolver evidence is
  available.
- Ensure schematic-to-PCB transfer consumes symbol pin numbers consistently.
- Add allow/skip behavior for projects without configured symbol roots at draft
  or structural acceptance.

### Tests

- `design create` with valid fixture symbols passes symbol stage.
- `design create` with missing symbol blocks before project write at
  connectivity acceptance.
- Writer correctness reports unresolved symbol with resolver enabled.
- Schematic-to-PCB transfer keeps pin-number mapping stable.

### Acceptance Criteria

- Generated workflows can fail early on unsafe symbol semantics.
- Feedback identifies the exact symbol/pin issue an agent must repair.

### Commit Message

```text
Gate workflows on symbol evidence
```

## 11. Phase 9: Golden Corpus And External Integration Hooks

### Goal

Lock down symbol resolver behavior with deterministic fixtures and optional
external KiCad library probes.

### Work

- Add fixture corpus for:
  - passive symbol;
  - connector symbol;
  - power symbol;
  - multi-unit symbol;
  - inherited symbol;
  - malformed file;
  - duplicate symbol;
  - hidden power pin.
- Add golden JSON outputs for:
  - list;
  - show;
  - pins;
  - validation blocked.
- Add `-update` golden behavior if the package does not already provide it.
- Add optional external test entry point gated by environment variables:

```text
KICAD_SYMBOLS_DIR
KICADAI_RUN_EXTERNAL_SYMBOL_TESTS=1
```

- Document external test setup.

### Tests

- Golden output tests.
- Full `go test ./...`.
- Optional external symbol checkout smoke test when explicitly enabled.

### Acceptance Criteria

- Symbol resolver regressions are caught by default fixture tests.
- External KiCad symbol checks are available but never required by default.

### Commit Message

```text
Add symbol resolver golden tests
```

## 12. Final Completion Checklist

- All phase tests pass.
- `go test ./...` passes.
- Prism review has no unresolved high-severity findings.
- README or docs mention the new symbol resolver CLI and limitations.
- `specs/ROADMAP_GAP.md` is updated to mark this project complete or partially
  complete with remaining follow-ups.
