# KiCad Schematic Writer Gap-Bridging Specification

## Purpose

Bring the Go schematic writer into alignment with KiCad's native schematic
writer so generated `.kicad_sch` files open as current KiCad files, round-trip
cleanly through KiCad, and provide a reliable foundation for AI-generated
schematics.

The immediate problem is not that KiCad refuses every generated schematic. The
problem is that generated files still differ from KiCad-authored files in ways
that cause old-format warnings, first-save churn, missing connectivity, and
fragile examples. This specification defines the gaps to close before expanding
schematic generation into complex hierarchical designs.

## Source Basis

This specification is based on the local KiCad source checkout supplied during
development. In this workspace that checkout was available at:

```text
<local-kicad-source>
```

The primary source files reviewed are:

```text
eeschema/sch_io/kicad_sexpr/sch_io_kicad_sexpr.cpp
include/core/typeinfo.h
```

The implementation target is KiCad 10-style schematic output. KiCad source is
the compatibility authority for ordering, required nodes, optional nodes, and
writer behavior. Demo files and KiCad-saved generated files remain fixtures, but
the writer should follow the source implementation rather than reverse-engineer
only from examples.

## Current State

The current Go schematic writer can emit a usable subset:

- top-level `kicad_sch` metadata
- UUID, paper, and title block
- optional embedded `lib_symbols`
- symbols with basic properties
- wires
- labels
- junctions
- basic hierarchical sheets

The current model is intentionally small and lives primarily in:

```text
internal/kicadfiles/schematic/schematic.go
```

Known gaps:

- `lib_symbols` is omitted when empty, while KiCad writes an empty
  `lib_symbols` block.
- Top-level schematic items are not sorted in KiCad's type and UUID order.
- Symbol instances are modeled as top-level `symbol_instances`, but KiCad 10
  writes instances inside each symbol and writes root sheet instances as
  `sheet_instances`.
- Label shapes, fields, and field autoplacement flags are incomplete.
- Symbol pins, pin UUIDs, alternate pin functions, mirroring, locking, and
  field flags are incomplete.
- Sheet pins, sheet fields, sheet instances, and sheet behavior flags are
  incomplete.
- No-connects, buses, bus entries, graphical polylines, text, shapes, rule
  areas, bitmaps, tables, groups, embedded fonts, embedded files, and net-chain
  data are not represented.
- There is no schematic round-trip test harness equivalent to the PCB
  round-trip validation.
- The writer does not preserve unknown or unsupported nodes, so it is not yet
  suitable for editing arbitrary existing schematics.

## Goals

1. Emit schematic files that KiCad opens without the old-format warning.
2. Match KiCad source-derived top-level structure and ordering.
3. Support clean KiCad round-trip validation for generated schematic fixtures.
4. Produce deterministic output for stable diffs and snapshot tests.
5. Preserve existing simple-generation APIs while introducing a more complete
   internal model.
6. Make wire-to-symbol connectivity reliable by anchoring symbols, pins, and
   wires to KiCad-compatible coordinates and pin metadata.
7. Support hierarchical schematics with sheet pins and sheet instances.
8. Support enough schematic object types to cover the existing examples and
   common AI-generated circuits.
9. Leave a deliberate path toward lossless read-modify-write support.

## Non-Goals

- Implementing a full schematic editor.
- Replacing KiCad ERC.
- Autorouting or PCB synchronization.
- Full lossless parsing of every historical KiCad schematic in the first
  implementation pass.
- Generating full symbol libraries from scratch.
- Depending on live KiCad IPC write calls.
- Mutating user project files in place without explicit output paths.

## Compatibility Target

Default generated schematic files must target the installed KiCad 10 toolchain.
The writer must expose version configuration, but new examples should use the
current KiCad 10 schematic file version discovered from KiCad source or
KiCad-saved output.

Required top-level header shape:

```scheme
(kicad_sch
  (version <current-kicad-10-schematic-version>)
  (generator "<generator-name>")
  (generator_version "<major.minor>")
  ...
)
```

Generator defaults:

- `generator`: project-specific generator name is allowed, but should be stable.
- `generator_version`: must be emitted for KiCad 10 output.
- Version constants must be explicit. Do not silently change output versions
  without a named constant and tests.

## KiCad Source-Derived File Shape

KiCad's schematic writer emits a top-level `kicad_sch` node with this broad
ordering:

1. `version`
2. `generator`
3. `generator_version`
4. `uuid`
5. `paper`
6. optional `title_block`
7. `lib_symbols`
8. schematic items sorted by KiCad item type and UUID
9. optional top-level `net_chain` data for the first root sheet
10. root `sheet_instances`
11. optional `embedded_fonts`
12. optional embedded files

The Go writer must make this ordering deterministic and source-derived.

### Required `lib_symbols`

KiCad writes `lib_symbols` even when it is empty:

```scheme
(lib_symbols)
```

Requirement:

- Always emit `lib_symbols`.
- Preserve existing embedded-symbol support.
- Add validation that each referenced symbol either has an embedded symbol body
  or is intentionally external-library-only according to the selected writer
  profile.

### Schematic Item Ordering

KiCad sorts schematic items by internal type and UUID. The relevant order from
KiCad source is:

1. junction
2. no-connect
3. wire-to-bus entry
4. bus-to-bus entry
5. line-like items: wire, bus, graphical polyline
6. bitmap
7. table
8. table cell
9. local label
10. global label
11. hierarchical label
12. rule area
13. directive label
14. symbol
15. group
16. sheet pin
17. sheet

Requirement:

- Introduce an internal `SchematicItem` abstraction or equivalent sorting layer.
- Assign every renderable schematic object a KiCad-compatible item kind.
- Sort first by item kind and then by UUID.
- Keep current convenience slices as compatibility inputs if useful, but render
  through the canonical sorted item stream.

## Data Model Requirements

### Top-Level Schematic Model

Extend `SchematicFile` to include:

- format version
- generator and generator version
- UUID
- paper
- title block
- embedded library symbols
- canonical item collection
- root sheet instances
- optional net-chain data
- optional embedded-font flag
- optional embedded files
- writer profile metadata

The writer profile must capture:

- KiCad target version
- whether external library symbols are allowed
- whether unsupported objects are rejected or preserved as raw nodes
- whether round-trip-compatible formatting is required

### Preserved Raw Nodes

For generated-from-scratch files, unsupported objects may be rejected. For future
editing of existing schematics, unsupported objects must be preserved.

Requirement:

- Add a planned `RawSExpr` preservation mechanism before claiming arbitrary
  read-modify-write support.
- Raw nodes must retain their top-level item kind when possible so sorting does
  not move them into invalid positions.
- Raw nodes must never be silently dropped.

### Symbols

A schematic symbol must support:

- UUID
- library identifier
- optional library nickname/name
- position and rotation
- mirror mode on X or Y
- unit
- body style
- `exclude_from_sim`
- `in_bom`
- `on_board`
- `in_pos_files`
- `dnp`
- optional passthrough mode
- optional locked flag
- optional `fields_autoplaced`
- ordered fields/properties
- raw pin records
- per-symbol instances

Required symbol output shape:

```scheme
(symbol
  (lib_id "Device:R")
  (at 100 80 0)
  (unit 1)
  (body_style 1)
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (dnp no)
  (uuid "...")
  ...
)
```

If a symbol uses a mirrored orientation, emit the KiCad-compatible `mirror`
node. If the symbol has raw pin UUIDs, emit them in KiCad's `pin` form.

### Symbol Pins

KiCad writes per-symbol pin UUIDs and optional alternate pin functions:

```scheme
(pin "1" (uuid "..."))
(pin "2" (uuid "...") (alternate "ALT"))
```

Requirement:

- Add `SymbolPin` with number, UUID, and optional alternate.
- Validate unique pin numbers per symbol when alternates are not in use.
- Validate unique pin UUIDs globally within the schematic.
- Provide helper APIs that can derive pin anchor points from embedded symbol
  definitions for generated symbols.

### Symbol Fields and Properties

Fields are KiCad `property` nodes, not free text. A field must support:

- canonical name
- value
- private flag
- visibility or hide state
- position and rotation
- show-name flag
- do-not-autoplace flag
- text effects

Required field output shape:

```scheme
(property "Reference" "R1"
  (at 100 75 0)
  (effects (font (size 1.27 1.27)))
)
```

Requirement:

- Preserve KiCad property order when known.
- Emit at least `Reference`, `Value`, `Footprint`, `Datasheet`, and
  `Description` consistently for component symbols.
- Support private fields.
- Support hidden fields with KiCad-compatible hide syntax.
- Support text effects through a shared schematic text-effects model.

### Symbol Instances

KiCad writes symbol instances inside each symbol:

```scheme
(instances
  (project "project-name"
    (path "/..." (reference "R1") (unit 1))
  )
)
```

Requirement:

- Remove or deprecate top-level `symbol_instances` output.
- Add `Instances []SymbolInstance` to the symbol model.
- Group symbol instances by project name.
- Sort instance paths deterministically.
- Emit reference, unit, and optional per-instance variant data.
- Ensure every placed physical symbol has at least one valid instance for the
  root project unless an explicit no-instance writer profile is selected.

### Sheets

A hierarchical sheet must support:

- UUID
- position
- size
- flags: `exclude_from_sim`, `in_bom`, `on_board`, `dnp`
- optional locked flag
- optional `fields_autoplaced`
- stroke and fill
- fields/properties
- sheet pins
- sheet instances

Required shape:

```scheme
(sheet
  (at 40 40)
  (size 60 35)
  (stroke ...)
  (fill ...)
  (uuid "...")
  (property "Sheetname" "Power" ...)
  (property "Sheetfile" "power.kicad_sch" ...)
  (pin "VIN" input (at 40 50 180) (uuid "...") ...)
  (instances ...)
)
```

Requirement:

- Keep `Sheetname` and `Sheetfile` as sheet fields, not ad hoc attributes.
- Validate that sheet file paths are relative project paths.
- Validate sheet pin direction/shape tokens.
- Validate sheet pins align to sheet borders.
- Emit sheet instances inside each sheet.

### Root Sheet Instances

KiCad writes root sheet instance data as:

```scheme
(sheet_instances
  (path "/" (page "1"))
)
```

Requirement:

- Add a top-level `SheetInstances` model.
- Always emit at least the root path `/` with page `1` for generated root
  schematics unless a fixture proves KiCad omits it for a specific valid case.
- Sort sheet instance paths deterministically.

### Labels

Labels must support:

- local labels
- global labels
- hierarchical labels
- directive labels
- shape for label kinds that require it
- UUID
- position and rotation
- optional locked flag
- optional fields
- optional `fields_autoplaced`
- text effects

Requirement:

- Local labels render as `label`.
- Global labels render as `global_label` and include `shape`.
- Hierarchical labels render as `hierarchical_label` and include `shape`.
- Directive labels render as `directive_label` and include `shape`.
- Validate allowed shapes per label kind.

### Wires, Buses, and Graphical Lines

Line-like objects must distinguish:

- schematic wires
- buses
- graphical polylines

Common fields:

- UUID
- points
- stroke
- optional locked flag

Requirement:

- Wires render as `wire`.
- Buses render as `bus`.
- Graphical polylines render as `polyline`.
- Validate at least two points.
- Validate coordinates are grid-compatible for generated connectivity profiles.

### Junctions

Junctions must support:

- UUID
- position
- diameter
- color

Requirement:

- Default generated junctions should match KiCad defaults.
- Junctions must be generated where multiple wires meet and KiCad would require
  visible electrical connection clarity.

### No-Connects

Add no-connect support:

```scheme
(no_connect
  (at 120 80)
  (uuid "...")
)
```

Requirement:

- No-connect markers must be placeable at symbol pins.
- Validate no-connect coordinates match a known unconnected pin when generated
  from the higher-level circuit model.

### Bus Entries

Add support for:

- wire-to-bus entries
- bus-to-bus entries

Requirement:

- Model entry position, size/direction, stroke, and UUID.
- Validate entry points connect to wire or bus endpoints when generated from a
  connectivity graph.

### Text and Graphics

Add support for:

- plain text
- text boxes
- rectangles
- circles
- arcs
- polygons
- rule areas
- bitmaps as a future optional object

Requirement:

- Use shared stroke, fill, and text-effects models.
- Generated examples may initially use only text and simple shapes.
- Unsupported bitmap payloads may be rejected until raw-node preservation exists.

### Groups

KiCad supports schematic groups.

Requirement:

- Add group support after primary electrical items are stable.
- Group membership must refer to existing item UUIDs.
- Validate that group references do not point to missing items.

### Embedded Fonts and Files

KiCad may write `embedded_fonts` and embedded file data.

Requirement:

- Support an `EmbeddedFonts` boolean.
- Preserve embedded file data when parsing existing files.
- Generated-from-scratch schematics may omit embedded files unless text
  rendering requires them.

### Net Chain Data

KiCad can write top-level `net_chain` data on the first root sheet.

Requirement:

- Treat net-chain data as KiCad-maintained metadata until fully understood.
- Preserve existing `net_chain` raw nodes when editing parsed schematics.
- Do not synthesize net-chain data unless confirmed against KiCad source and
  round-trip fixtures.

## Connectivity Requirements

The writer must make schematic connectivity deliberate rather than visual-only.

Generated schematic construction should use a circuit graph that knows:

- components
- symbol units
- pins
- pin numbers
- pin anchor coordinates
- nets
- labels
- wires
- junctions
- no-connects

Requirements:

- Every generated wire endpoint should land on another wire endpoint, label
  anchor, junction, sheet pin, or symbol pin anchor.
- A pin-to-wire connection must use the pin's exact schematic coordinate.
- Symbol pin anchor coordinates must be derived from embedded symbol geometry or
  an explicitly declared symbol template, not guessed from component bounding
  boxes.
- Generated labels must land exactly on the intended net.
- Validation must flag near-misses where a wire endpoint is visually close to a
  pin but not electrically connected.

## Formatting Requirements

The schematic writer must share KiCad-compatible formatting rules with the PCB
writer where possible:

- UTF-8 output.
- `\n` line endings.
- deterministic two-space indentation.
- stable numeric formatting.
- explicit `(at x y angle)` where KiCad writes an angle.
- quoted strings escaped through the shared S-expression renderer.
- no trailing whitespace.
- final newline.

Numeric formatting requirements:

- Preserve integers as integers when coordinates are integer millimeters.
- Preserve fixed decimal precision only when needed.
- Avoid `-0`.
- Round only at the formatting boundary, never in the model.

## Validation Requirements

Add schematic validation for:

- required top-level version, generator, generator version, UUID, and paper.
- valid UUID format for every object that has a UUID.
- global UUID uniqueness.
- valid label kinds and shapes.
- valid sheet pin kinds and border placement.
- valid symbol library IDs.
- valid symbol fields and required field names.
- valid symbol instances and sheet instances.
- valid relative sheet filenames.
- no unsupported item kinds in strict writer mode.
- every wire, bus, and polyline has at least two points.
- connectivity endpoints exactly match known anchors in generated-connectivity
  validation mode.
- all group member references resolve.
- no top-level `symbol_instances` output.

Validation should have at least two levels:

- `Structural`: file can be rendered and KiCad should parse it.
- `GeneratedConnectivity`: generated schematic endpoints and symbol pins align
  exactly.

## Round-Trip Validation Requirements

Add a schematic round-trip test harness parallel to the PCB round-trip harness.

Requirements:

- Run only when explicitly enabled by environment variable.
- Use the installed KiCad CLI.
- Use project-owned artifact directories, such as
  `examples/roundtrip_artifacts`, to avoid working-directory failures observed
  with temporary directories.
- Copy or generate fixture projects into the artifact directory.
- Ask KiCad to load/upgrade/save schematics using the confirmed KiCad CLI
  command for schematics.
- Parse both original and KiCad-saved schematic S-expressions.
- Normalize known KiCad-owned metadata if unavoidable.
- Fail on all unexpected structural diffs.
- Write a readable summary of differences when failures occur.

The exact KiCad CLI command for schematic save/upgrade must be discovered and
documented during implementation. Do not hard-code an assumed command without a
fixture proving it works on the installed KiCad.

Acceptance fixtures:

- empty schematic
- LED indicator
- RC low-pass filter
- 555 timer
- sensor node
- class AB headphone amplifier
- simple hierarchical sheet fixture

## Backward Compatibility

Existing helper APIs should keep working for simple examples where practical.

Required migration approach:

- Keep current `SchematicFile.Symbols`, `Wires`, `Labels`, `Junctions`, and
  `Sheets` as compatibility inputs initially.
- Convert compatibility slices into canonical `SchematicItem` objects before
  rendering.
- Deprecate top-level `Instances` with documentation and tests proving it is no
  longer rendered as `symbol_instances`.
- Provide new constructors for symbols, fields, sheets, labels, and wires that
  populate KiCad-required defaults.
- Update examples incrementally to use the new constructors.

## Implementation Boundaries

The schematic writer should not depend on the PCB writer, but both should share:

- UUID helpers
- S-expression renderer
- numeric formatting
- stroke model where compatible
- fill model where compatible
- text-effects model where compatible
- round-trip normalization utilities

The schematic package may expose high-level generation helpers, but low-level
rendering must stay data-model driven.

## Acceptance Criteria

The gap-bridging work is complete when:

1. All generated example schematics open in KiCad without the old-format warning.
2. Existing examples no longer show visually disconnected pin/wire near-misses.
3. The writer always emits `lib_symbols`.
4. The writer emits KiCad source-derived schematic item ordering.
5. The writer no longer emits top-level `symbol_instances`.
6. Symbols support fields, pins, instances, mirroring, lock state, and KiCad
   behavior flags.
7. Sheets support fields, pins, instances, and KiCad behavior flags.
8. Labels support KiCad shape requirements.
9. Wires, buses, junctions, no-connects, and bus entries are structurally
   supported.
10. Schematic round-trip tests pass for the selected generated fixture set.
11. Unit tests cover validation failures for invalid IDs, bad sheet paths,
    unsupported label shapes, duplicate UUIDs, dangling group members, and
    connectivity near-misses.
12. Snapshot tests prove deterministic output for representative schematics.

## Risks

- KiCad may compute or rewrite metadata that is not obvious from generated
  files. Mitigation: use source review plus round-trip fixtures.
- Symbol pin coordinates are easy to get subtly wrong. Mitigation: derive pin
  anchors from embedded symbol definitions or explicit templates.
- Lossless editing is broader than generation. Mitigation: keep generated-only
  strict mode separate from future raw-node preservation.
- KiCad CLI schematic commands may differ from PCB commands. Mitigation:
  discover and test commands before building the round-trip harness around them.
- Large symbol bodies can make snapshots noisy. Mitigation: use focused fixtures
  and normalized comparisons.

## Open Questions

1. What exact KiCad 10 schematic version constant should be emitted by default?
2. Which installed KiCad CLI command reliably loads and saves schematics in
   batch mode?
3. Should generated schematics embed full library symbols by default, or rely on
   KiCad global libraries for common symbols?
4. How much raw-node preservation is required before supporting edits to
   existing user schematics?
5. Should AI generation operate directly on schematic geometry, or should it
   always produce a netlist-style circuit graph that is then laid out by helper
   code?

## Deliverables

The implementation that follows this specification should produce:

- updated schematic data model
- KiCad-compatible schematic renderer
- expanded schematic validation
- schematic round-trip harness
- updated example schematics
- tests for deterministic output and validation
- documentation for supported schematic object types
- migration notes for any deprecated API fields
