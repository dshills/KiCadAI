# KiCad Schematic Writer Gap-Bridging Implementation Plan

## Purpose

Implement the schematic writer gaps described in `SPEC.md` in small, verifiable
phases. Each phase should be suitable for one focused implementation commit
after tests and review. The end state is a KiCad 10-compatible schematic writer
that produces current-format `.kicad_sch` files, renders source-derived ordering,
supports core schematic objects, validates generated connectivity, and
round-trips cleanly through KiCad for generated examples.

## Working Rules

- Follow KiCad source behavior before inferred behavior from generated examples.
- Keep generated-from-scratch support separate from future lossless editing.
- Preserve existing simple schematic APIs until replacements are proven.
- Use the shared S-expression renderer rather than ad hoc string formatting.
- Add tests in the same phase as behavior changes.
- Run `go test ./...` after each phase.
- Run schematic round-trip tests once the harness exists and KiCad CLI behavior
  is confirmed.
- Review each phase before commit.

## Phase 1: Source Baseline and Format Constants

### Goal

Lock down the KiCad 10 schematic writer baseline so future changes target a
known file shape.

### Tasks

1. Inspect KiCad source and/or KiCad-saved schematics to confirm:
   - current schematic file version
   - default schematic generator value KiCad uses
   - generator version formatting
   - whether KiCad writes empty `lib_symbols` for every schematic
   - root `sheet_instances` behavior for empty and non-empty root sheets
2. Add explicit schematic format constants in the Go model.
3. Ensure default generated schematics use the KiCad 10 schematic version.
4. Ensure `generator_version` is emitted for KiCad 10 output.
5. Add tests for version parsing, default values, and header rendering.
6. Update documentation comments for the schematic writer compatibility target.

### Acceptance Criteria

- A minimal generated schematic has a KiCad 10-compatible header.
- The writer has named constants for supported schematic file versions.
- Tests fail if `generator_version` is accidentally omitted for KiCad 10 output.
- No example or test depends on an unnamed raw version string.

### Review Focus

- Version constant correctness.
- No silent compatibility break for existing callers.
- Header output matches KiCad source or a KiCad-saved fixture.

## Phase 2: Canonical Top-Level Rendering Order

### Goal

Render top-level schematic structure in KiCad source-derived order.

### Tasks

1. Always emit `lib_symbols`, including `(lib_symbols)` when empty.
2. Introduce a canonical render pipeline for top-level schematic items.
3. Add an internal schematic item kind enum matching KiCad type order:
   - junction
   - no-connect
   - wire-to-bus entry
   - bus-to-bus entry
   - line-like item
   - bitmap
   - table
   - table cell
   - local label
   - global label
   - hierarchical label
   - rule area
   - directive label
   - symbol
   - group
   - sheet pin
   - sheet
4. Convert existing compatibility slices into canonical render items.
5. Sort render items by item kind and UUID.
6. Add snapshot tests proving deterministic order independent of input slice
   order.
7. Verify existing examples still render.

### Acceptance Criteria

- `lib_symbols` is always present.
- Existing labels, wires, junctions, symbols, and sheets render through the
  canonical item stream.
- Snapshot tests prove KiCad type ordering and UUID secondary ordering.
- No top-level item ordering depends on input slice order.

### Review Focus

- Item kind ordering against KiCad `typeinfo.h`.
- Backward compatibility of existing public structs.
- Snapshot stability.

## Phase 3: Symbol Model Alignment

### Goal

Make schematic symbols structurally match KiCad-written symbols.

### Tasks

1. Extend `SchematicSymbol` with:
   - optional library name/nickname if needed by KiCad output
   - mirror mode
   - passthrough mode
   - locked flag
   - `fields_autoplaced`
   - ordered `Properties`
   - `Pins`
   - per-symbol `Instances`
2. Add `SymbolPin` with number, UUID, and optional alternate.
3. Add a richer field/property model:
   - canonical name
   - value
   - private flag
   - hidden state
   - show-name flag
   - do-not-autoplace flag
   - position
   - rotation
   - text effects
4. Preserve the existing `Fields` API as a compatibility adapter.
5. Move symbol instance rendering into each symbol.
6. Stop rendering or exposing top-level `symbol_instances` output.
7. Add validation for:
   - required symbol UUID
   - required library ID
   - duplicate symbol pin UUIDs
   - invalid mirror modes
   - malformed instance paths
   - duplicate or missing required properties where applicable
8. Add unit and snapshot tests for:
   - basic resistor/capacitor symbols
   - power symbols
   - symbol pins
   - hidden fields
   - symbol instances
   - migrated compatibility fields

### Acceptance Criteria

- Symbols render KiCad-compatible behavior flags.
- Symbol instances are nested under `symbol`.
- Top-level `symbol_instances` no longer appears in generated output.
- Existing generated examples still compile and render.
- Tests cover symbol pins, fields, instances, and validation failures.

### Review Focus

- Compatibility adapter clarity.
- Correct KiCad property syntax.
- Required flag defaults.
- No accidental first-save churn from omitted symbol fields.

## Phase 4: Sheet and Hierarchy Alignment

### Goal

Support KiCad-compatible hierarchical sheets, sheet pins, sheet fields, and root
sheet instances.

### Tasks

1. Extend `Sheet` with:
   - behavior flags
   - locked flag
   - `fields_autoplaced`
   - stroke
   - fill
   - fields/properties
   - pins
   - sheet instances
2. Add `SheetPin` with:
   - text
   - electrical shape/type token
   - position
   - rotation
   - UUID
   - text effects
3. Add top-level `SheetInstances`.
4. Emit root `sheet_instances` after top-level items.
5. Convert current `Sheet.Name` and `Sheet.Filename` into `Sheetname` and
   `Sheetfile` properties during rendering.
6. Validate:
   - relative sheet filenames
   - required sheet name and file fields
   - sheet pin positions on sheet borders
   - valid sheet pin shape/type tokens
   - valid sheet and root instance paths
7. Add hierarchical fixture tests.

### Acceptance Criteria

- Root schematics emit KiCad-compatible `sheet_instances`.
- Hierarchical sheets render fields and pins in KiCad-compatible form.
- Invalid sheet filenames and off-border pins fail validation.
- Simple and hierarchical generated schematics remain deterministic.

### Review Focus

- Instance path shape.
- Sheet field rendering order.
- Border-position validation precision.
- Backward compatibility for simple sheets.

## Phase 5: Labels, Text Effects, and Core Electrical Markers

### Goal

Complete the common schematic electrical object set needed for reliable circuit
generation.

### Tasks

1. Add shared schematic text-effects model.
2. Extend labels with:
   - KiCad shape
   - locked flag
   - fields
   - `fields_autoplaced`
   - text effects
3. Add directive labels.
4. Add no-connect markers.
5. Extend junctions with diameter and color.
6. Add validation for:
   - label kind/shape combinations
   - no-connect UUID and coordinate
   - text-effect dimensions
   - duplicate UUIDs across all schematic object types
7. Add render tests for:
   - local labels
   - global labels
   - hierarchical labels
   - directive labels
   - no-connect markers
   - junction defaults

### Acceptance Criteria

- Global and hierarchical labels include required shape data.
- No-connect markers are first-class renderable schematic items.
- Duplicate UUIDs are rejected globally.
- Tests cover all label kinds and invalid shape combinations.

### Review Focus

- KiCad label token names.
- Default text effects.
- Global UUID accounting.

## Phase 6: Wires, Buses, Entries, and Graphics

### Goal

Support KiCad-compatible line-like objects and enough graphics for generated
schematics to annotate circuits without unsupported structures.

### Tasks

1. Generalize current `Wire` rendering into a line-like model that supports:
   - wire
   - bus
   - graphical polyline
2. Add shared stroke model if one is not already reusable from PCB code.
3. Add wire-to-bus and bus-to-bus entries.
4. Add simple text and graphical shapes:
   - text
   - text box
   - rectangle
   - circle
   - arc
   - polygon
5. Add rule areas if KiCad source behavior is clear enough for generated use.
6. Add validation for:
   - minimum point counts
   - valid stroke and fill values
   - bus entry direction/size
   - graphics UUIDs
7. Add render tests for each supported object type.

### Acceptance Criteria

- Wires, buses, and polylines render as distinct KiCad tokens.
- Bus entries are supported and sorted in KiCad order.
- Basic graphical annotation objects are supported.
- Invalid line-like objects fail validation.

### Review Focus

- Reuse of stroke/fill/text effects.
- KiCad token mapping.
- Avoiding overreach into objects not yet validated against source.

## Phase 7: Connectivity Validation

### Goal

Detect the class of visual-but-not-electrical connection problems seen in early
examples.

### Tasks

1. Add a generated-connectivity validation mode.
2. Define schematic anchors:
   - symbol pin anchors
   - wire endpoints
   - bus endpoints
   - label anchors
   - junction positions
   - sheet pin anchors
   - no-connect positions
3. Add helper APIs for deriving symbol pin anchors from known embedded symbol
   definitions or explicit symbol templates.
4. Validate that generated wire endpoints land exactly on known anchors.
5. Validate that labels land exactly on the intended net.
6. Validate that no-connect markers land on intended unconnected pins.
7. Detect near-miss endpoints and return actionable errors.
8. Add tests for:
   - valid pin-to-wire connections
   - valid label-to-wire connections
   - multi-wire junctions
   - off-grid endpoints
   - near-miss pin connections
   - missing junctions where required by the generated profile

### Acceptance Criteria

- The validator catches a wire endpoint that is visually close to but not on a
  symbol pin.
- Existing examples pass generated-connectivity validation after updates.
- Validation errors include object UUIDs and coordinates.

### Review Focus

- Coordinate precision and tolerance policy.
- Clear error messages.
- Separation of structural validation from generated-connectivity validation.

## Phase 8: Schematic Round-Trip Harness

### Goal

Add automated KiCad round-trip validation for generated schematics.

### Tasks

1. Discover and document the installed KiCad CLI schematic command that can
   load, upgrade, and save schematic files.
2. Add schematic support to the existing round-trip package or a sibling helper.
3. Use project-owned artifact directories when enabled to avoid working-directory
   failures from deleted temporary directories.
4. Generate or copy schematic fixtures into the artifact directory.
5. Run KiCad CLI only when an environment variable enables integration tests.
6. Parse original and KiCad-saved schematic S-expressions.
7. Normalize only confirmed KiCad-owned metadata.
8. Produce readable diff summaries.
9. Add documentation for running schematic round-trip tests locally.

### Acceptance Criteria

- Round-trip integration tests are opt-in.
- The harness works from `examples/roundtrip_artifacts` or another stable
  project-owned artifact root.
- At least minimal, LED, RC filter, and hierarchical fixtures round-trip with no
  unexpected diffs.
- Failures include artifact paths and concise diff summaries.

### Review Focus

- KiCad CLI command correctness.
- Artifact cleanup behavior.
- Diff normalization restraint.

## Phase 9: Example Regeneration and Fixture Expansion

### Goal

Update generated examples so they demonstrate the aligned writer and become
useful regression fixtures.

### Tasks

1. Regenerate existing examples with the aligned schematic writer:
   - LED indicator
   - RC low-pass filter
   - 555 timer
   - sensor node
   - class AB headphone amplifier
2. Add a minimal empty schematic fixture.
3. Add a simple hierarchical fixture with sheet pins.
4. Confirm examples open without old-format warnings.
5. Confirm examples pass structural validation.
6. Confirm examples pass generated-connectivity validation.
7. Run opt-in round-trip tests for examples where KiCad CLI support is ready.
8. Update example documentation with supported validation commands.

### Acceptance Criteria

- Generated examples open in KiCad without old-format warnings.
- Wires visibly and electrically connect to symbols.
- Examples serve as deterministic snapshot and round-trip fixtures.
- Any intentionally unsupported object type is documented.

### Review Focus

- Visual quality of examples.
- Example determinism.
- Whether fixtures cover enough schematic object variety.

## Phase 10: Raw Node Preservation Path

### Goal

Prepare the writer for future read-modify-write support without claiming full
lossless editing too early.

### Tasks

1. Add a `RawSExpr` preservation model for unsupported top-level objects.
2. Preserve raw nodes with item kind metadata where known.
3. Add strict mode and preserve mode:
   - strict mode rejects unsupported objects
   - preserve mode keeps unsupported raw nodes
4. Add parser tests with representative unknown nodes.
5. Ensure raw nodes are sorted or retained according to KiCad-compatible item
   positioning rules.
6. Document that full arbitrary editing remains incomplete until parser coverage
   expands.

### Acceptance Criteria

- Unsupported nodes are never silently dropped.
- Strict mode fails clearly on unsupported nodes.
- Preserve mode round-trips unsupported raw nodes in controlled fixtures.
- Documentation states the limits of read-modify-write support.

### Review Focus

- No false claim of full lossless editing.
- Raw node ordering safety.
- Parser/render symmetry.

## Phase 11: Cleanup, API Polish, and Documentation

### Goal

Consolidate the schematic writer into a clean API surface after behavior is
proven.

### Tasks

1. Mark deprecated compatibility fields with comments.
2. Add constructors for common schematic objects:
   - symbol
   - property
   - wire
   - label
   - no-connect
   - sheet
   - sheet pin
3. Add package documentation for supported schematic object types.
4. Add examples in Go tests showing idiomatic writer use.
5. Review validation error naming and messages.
6. Remove dead helper code.
7. Ensure all generated files are gofmt-clean and tests pass.

### Acceptance Criteria

- Public internal APIs are coherent for the next PCB/schematic integration work.
- Deprecated fields have migration guidance.
- Package docs describe supported and unsupported KiCad schematic features.
- `go test ./...` passes.

### Review Focus

- API ergonomics for future AI generation.
- Clear migration path.
- Avoiding unnecessary abstractions.

## Suggested Commit Boundaries

1. `schematic: add KiCad 10 format baseline`
2. `schematic: render items in KiCad order`
3. `schematic: align symbol output with KiCad`
4. `schematic: align sheet hierarchy output`
5. `schematic: add labels and electrical markers`
6. `schematic: add buses and graphics`
7. `schematic: validate generated connectivity`
8. `roundtrip: add schematic validation harness`
9. `examples: regenerate schematic fixtures`
10. `schematic: preserve unsupported raw nodes`
11. `schematic: document final writer API`

## Final Completion Checklist

- All generated schematic examples open without old-format warnings.
- All generated schematic examples pass structural validation.
- Generated examples pass connectivity validation.
- Opt-in schematic round-trip tests pass for supported fixtures.
- No top-level `symbol_instances` appears in generated schematics.
- `lib_symbols` is always emitted.
- Item ordering matches KiCad source-derived order.
- Symbols, sheets, labels, wires, buses, no-connects, and junctions have focused
  tests.
- `go test ./...` passes.
- The remaining unsupported KiCad schematic features are documented.
