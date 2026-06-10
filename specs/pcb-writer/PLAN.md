# KiCad PCB Writer Phased Implementation Plan

## Goal

Implement the PCB writer described in `specs/pcb-writer/SPEC.md` in small,
reviewable phases. Each phase should leave the repository in a working state,
include focused tests, and be suitable for an independent commit.

The first target is a KiCad 10-compatible generated two-layer PCB that opens
without an older-version warning. Later phases expand the model toward
full-fidelity KiCad board generation and round-trip preservation.

## Working Rules

- Keep changes scoped to one phase at a time.
- Update tests before moving to the next phase.
- Run `gofmt -w` on changed Go files.
- Run `go test ./...` after each phase.
- When a phase emits PCB examples, validate with:

```sh
/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli pcb upgrade --force <copy>
/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli pcb drc --exit-code-violations <board>
```

- If DRC reports expected fixture limitations, document them next to the test or
  example.
- Keep generated output deterministic so golden tests and Git diffs are stable.

## Phase 1: KiCad 10 PCB Header, General, Layers, And Setup

### Objective

Make the existing PCB writer emit modern KiCad 10 board files that do not
trigger the older-version warning.

### Implementation Tasks

1. Add KiCad 10 PCB constants in the shared KiCad file model:
   - PCB format version `20260206`
   - generator version `10.0`
   - default generator behavior for PCB files

2. Extend `PCBFile` metadata:
   - Add `GeneratorVersion string`
   - Replace empty `PCBGeneral` with fields:
     - `Thickness kicadfiles.IU`
     - `LegacyTeardrops bool`
   - Ensure zero-value generation can be normalized to KiCad 10 defaults.

3. Fix default layer table:
   - Replace the current sparse `DefaultTwoLayerStack` numbers with KiCad 10
     saved-file numbers.
   - Add optional display names to `LayerDefinition`.
   - Include the full KiCad 10 default layer table.

4. Expand `PCBSetup`:
   - Add soldermask bridge setting.
   - Add front/back tenting, covering, plugging.
   - Add capping and filling flags.
   - Add a minimal `PCBPlotParams` model with KiCad 10 default values.

5. Update PCB rendering:
   - Emit `generator_version`.
   - Emit `general.thickness`.
   - Emit `general.legacy_teardrops`.
   - Emit full layer definitions with optional display names.
   - Emit KiCad 10 setup fields in KiCad-saved order.

6. Update LED/demo PCB construction to use KiCad 10 defaults.

### Tests

- Unit test header rendering includes:
  - `(version 20260206)`
  - `(generator "pcbnew")` or selected accepted generator
  - `(generator_version "10.0")`
  - `(thickness 1.6)`
  - `(legacy_teardrops no)`

- Unit test default layer table includes:
  - `F.Cu` number `0`
  - `B.Cu` number `2`
  - `F.SilkS` number `5`
  - `B.SilkS` number `7`
  - `Edge.Cuts` number `25`
  - `User.45` number `127`

- Unit test setup block includes KiCad 10 soldermask, tenting, covering,
  plugging, capping, filling, and plot params.

### Acceptance Criteria

- Existing PCB writer tests pass.
- New KiCad 10 header/layer/setup tests pass.
- A generated simple PCB no longer looks like an old-format KiCad file when
  upgraded with `kicad-cli pcb upgrade --force` on a copy.

## Phase 2: Net Model Normalization And Validation

### Objective

Make net handling deterministic and strict enough for pads, routes, vias, zones,
and schematic-derived boards.

### Implementation Tasks

1. Add a net registry/helper:
   - Always reserve net `0` for `""`.
   - Assign deterministic net codes by insertion order or sorted name,
     documented in code.
   - Resolve net names to codes for higher-level APIs.

2. Extend net validation:
   - Require net `0`.
   - Require net `0` name to be empty.
   - Reject duplicate net names and duplicate net codes.
   - Reject negative net codes.

3. Update object validation to verify all net references:
   - Pads
   - Tracks
   - Vias
   - Zones
   - Copper graphics when implemented

4. Add helper APIs for generated boards:
   - `EnsureNet(name string) Net`
   - `NetCode(name string) (int, bool)`
   - `MustNetCode(name string) int` only for internal fixture use

### Tests

- Net `0` is inserted when omitted by builder helpers.
- Duplicate net names fail validation.
- Duplicate net codes fail validation.
- Pad/track/via/zone references to undeclared nets fail validation.
- Deterministic net assignment is stable across runs.

### Acceptance Criteria

- Net-related validation errors point to the field and object index.
- Fixtures can build boards by net name without manually assigning codes.

## Phase 3: KiCad 10 Footprint Properties And Embedded Geometry

### Objective

Represent and render footprints as KiCad-saved board objects instead of minimal
library references.

### Implementation Tasks

1. Replace or extend `FootprintText` with structured `FootprintProperty`:
   - Name
   - Value
   - Position
   - Rotation
   - Layer
   - UUID
   - Hide
   - Unlocked
   - Effects

2. Add required property helpers:
   - `Reference`
   - `Value`
   - `Datasheet`
   - `Description`

3. Extend `Footprint`:
   - `Description`
   - `Tags`
   - `Attributes []string`
   - `SheetName`
   - `SheetFile`
   - `Properties []FootprintProperty`
   - `Models []Model3D`
   - `EmbeddedFonts *bool`

4. Expand footprint graphic support:
   - `fp_line`
   - `fp_rect`
   - `fp_circle`
   - `fp_arc`
   - `fp_poly`
   - `fp_text`

5. Render footprint objects in KiCad-saved order:
   - Header, layer, UUID, at
   - `descr`, `tags`
   - properties
   - path/sheet metadata
   - attributes
   - graphics
   - pads
   - embedded fonts
   - models

6. Add helpers for common simple footprints needed by examples:
   - Two-pad SMD resistor/capacitor
   - Through-hole 2-pin connector
   - LED SMD or through-hole footprint

### Tests

- Golden render test for a KiCad 10 SMD resistor footprint.
- Golden render test for a through-hole connector footprint.
- Validation rejects footprints without UUID.
- Validation rejects duplicate footprint references unless explicitly marked as
  board-only or reference-hidden.
- Validation rejects properties without UUIDs.
- Model block renders offset, scale, and rotate.

### Acceptance Criteria

- Generated footprints contain actual pads and graphics.
- KiCad does not show missing-footprint geometry for generated example boards.
- Existing examples can be migrated away from placeholder/minimal footprints.

## Phase 4: Pad Model Completeness

### Objective

Support the pad structures required for SMD and through-hole PCBs.

### Implementation Tasks

1. Extend `Pad`:
   - `Type string`
   - `PinFunction string`
   - `PinType string`
   - `NetName string`
   - `RemoveUnusedLayers *bool`
   - `ThermalBridgeAngle *float64`
   - `Teardrops *TeardropSettings`

2. Allow wildcard pad layers:
   - `*.Cu`
   - `*.Mask`

3. Add pad render support for:
   - SMD rectangular and roundrect pads
   - Through-hole rect, circle, and oval pads
   - Optional drill
   - Optional roundrect ratio
   - Optional thermal bridge angle
   - Optional remove-unused-layers

4. Add pad validation:
   - Type is known.
   - Shape is known.
   - Size is positive.
   - Drill is required and positive for through-hole pads.
   - Drill is absent or ignored for SMD pads.
   - SMD pads use compatible side layers.
   - Through-hole pads use copper/mask wildcard or valid copper/mask layers.
   - Net code and net name agree when both are present.

### Tests

- Golden render test for observed SMD roundrect pad.
- Golden render test for observed through-hole rect pad.
- Validation test for missing through-hole drill.
- Validation test for invalid wildcard layer.
- Validation test for net name/code mismatch.

### Acceptance Criteria

- Simple SMD and through-hole generated boards load with visible connected pads.
- Pad output matches KiCad-saved style closely enough that first KiCad save does
  not rewrite the whole pad block.

## Phase 5: Board Graphics, Text, And Closed Outlines

### Objective

Support board outline generation and common board-level drawings.

### Implementation Tasks

1. Extend drawing model:
   - Add `RectDrawing`
   - Add `TextDrawing`
   - Add fill and stroke type support.
   - Add optional net code/name for copper graphics.

2. Update renderers:
   - `gr_line`
   - `gr_rect`
   - `gr_circle`
   - `gr_arc`
   - `gr_poly`
   - `gr_text`

3. Add outline helper APIs:
   - Rectangular outline
   - Rounded rectangle outline
   - Polygon outline

4. Improve outline validation:
   - Validate all required `Edge.Cuts` shapes form a closed chain.
   - Allow closed `gr_rect` on `Edge.Cuts`.
   - Report the first open endpoint when validation fails.

5. Add text effects support shared with footprints:
   - Font size
   - Thickness
   - Justification
   - Hide when applicable

### Tests

- Golden render test for each board graphic type.
- Closed rectangular outline validates.
- Open outline fails with a useful error.
- Copper `gr_poly` requires valid net.
- Text rendering includes layer, UUID, and effects.

### Acceptance Criteria

- Generated boards have clean visible outlines in KiCad.
- The writer can generate simple labels and copper polygons.

## Phase 6: Routing Segments, Arcs, And Vias

### Objective

Generate connected route objects with KiCad 10 syntax.

### Implementation Tasks

1. Add route arc model:
   - Start
   - Mid
   - End
   - Width
   - Layer
   - Net code/name
   - UUID

2. Extend via model:
   - Tenting front/back
   - Via type when needed later
   - Layer pair/list validation

3. Update renderers:
   - `segment`
   - route `arc`
   - `via`

4. Add route validation:
   - Width is positive.
   - Layers are valid copper layers.
   - Nets exist.
   - Vias span at least two copper layers.
   - Arcs have distinct start, mid, and end points.

5. Add optional connectivity checks:
   - Route endpoints on same net should touch pads, vias, or other route
     objects within a small coordinate tolerance.
   - Start with warnings/test helpers if strict validation would block useful
     manual placement.

### Tests

- Golden render test for segment.
- Golden render test for route arc.
- Golden render test for via with front/back tenting.
- Validation rejects route objects on non-copper layers.
- Validation rejects vias with fewer than two copper layers.

### Acceptance Criteria

- A generated two-layer board can route at least one net with pads, traces, and
  a via that KiCad displays as connected.

## Phase 7: Zones And Copper Fills

### Objective

Support copper zones well enough for generated power fills and preserve richer
KiCad zone data when importing later.

### Implementation Tasks

1. Extend `Zone`:
   - `NetName`
   - `Name`
   - `HatchStyle`
   - `HatchPitch`
   - `Priority`
   - `ConnectPads`
   - `Clearance`
   - `MinThickness`
   - `FilledAreasThickness`
   - `FillSettings`
   - `Attributes`
   - `Polygons`
   - `FilledPolygons`

2. Render source polygons in KiCad style.

3. Support optional filled polygons:
   - Generated boards may omit filled polygons initially.
   - Imported boards must eventually preserve them.

4. Add validation:
   - Zone has at least one layer.
   - Zone layers are copper or valid zone layers.
   - Polygons have at least three points.
   - Net code/name references are valid.
   - Priority is non-negative when present.

5. Add simple helper:
   - Rectangular copper pour inside the board outline.

### Tests

- Golden render test for a simple GND zone.
- Golden render test for an observed teardrop-like zone shape.
- Validation rejects polygons with fewer than three points.
- Validation rejects undeclared net references.

### Acceptance Criteria

- Generated boards can include a basic GND pour that KiCad accepts and can
  refill.

## Phase 8: KiCad CLI Validation Harness

### Objective

Add automated external validation for generated PCB fixtures using KiCad CLI
when it is installed.

### Implementation Tasks

1. Add test helper to locate `kicad-cli`:
   - Prefer `/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli`.
   - Fall back to `PATH`.
   - Skip external tests when unavailable.

2. Add temp-copy upgrade validation:
   - Write generated board to temp directory.
   - Run `kicad-cli pcb upgrade --force`.
   - Fail if command exits non-zero.
   - Optionally compare before/after for major structural rewrites.

3. Add DRC validation:
   - Run `kicad-cli pcb drc --exit-code-violations`.
   - Capture report output.
   - Allow fixture-specific expected violations only with explicit allowlists.

4. Add CI-safe controls:
   - `KICADAI_RUN_KICAD_CLI=1` opt-in for slow/external tests.
   - Unit tests remain independent of KiCad installation.

### Tests

- Unit test CLI path discovery using injected fake paths.
- Integration test is skipped by default without env opt-in.
- Integration test validates the generated MVP board when opt-in is enabled.

### Acceptance Criteria

- Developers can run a single command to verify generated PCBs with KiCad CLI.
- External validation failures include command output and board path.

## Phase 9: MVP Generated PCB Example

### Objective

Create a complete generated PCB example that exercises the implemented writer
without requiring full autorouting.

### Implementation Tasks

1. Add an example board generator:
   - Small two-layer board.
   - Closed rectangular or rounded rectangular outline.
   - At least two SMD footprints.
   - At least one through-hole connector.
   - Deterministic nets.
   - Route segment on front copper.
   - Via to back copper.
   - Optional GND pour.

2. Write generated project files:
   - `.kicad_pro`
   - `.kicad_sch` if schematic pairing is available.
   - `.kicad_pcb`
   - local library table files if needed.

3. Add documentation:
   - How to regenerate the example.
   - What KiCad CLI validation was run.
   - Any known DRC limitations.

4. Add golden snapshot tests for the example board output.

### Tests

- Generated example matches golden output.
- Internal validation passes.
- KiCad CLI upgrade passes when external validation is enabled.
- KiCad CLI DRC passes or reports only documented allowlisted fixture issues.

### Acceptance Criteria

- The example opens in KiCad 10 without an older-version warning.
- Footprints and pads are visible and connected.
- Route objects visually connect to pads.
- The board has a valid outline.

## Phase 10: Parser And Preservation Model

### Objective

Prepare for full-fidelity read/modify/write workflows by preserving unsupported
KiCad S-expression nodes.

### Implementation Tasks

1. Add PCB parser entry point:
   - Parse top-level `kicad_pcb`.
   - Extract known fields into typed model.
   - Store unknown nodes in order-preserving preservation containers.

2. Add preservation fields:
   - Top-level unknown nodes.
   - Unknown footprint children.
   - Unknown pad children.
   - Unknown zone children.
   - Unknown setup children.

3. Update writer:
   - Re-emit preserved unknown nodes when writing parsed boards.
   - Keep known-node ordering deterministic.

4. Add round-trip tests using small slices from demo files:
   - Footprint with unsupported child nodes.
   - Zone with filled polygon.
   - Group/dimension/embedded fonts.

### Tests

- Parse/write preserves unknown top-level nodes.
- Parse/write preserves unknown footprint child nodes.
- Round-trip fixture does not drop `embedded_fonts`.
- Round-trip fixture does not drop `teardrops`.

### Acceptance Criteria

- Imported demo board fragments can be parsed and written without losing
  unsupported data.
- Unsupported data preservation is explicit and tested.

## Phase 11: Full Demo Coverage And Regression Corpus

### Objective

Use the KiCad demo files as a compatibility corpus without requiring enormous
golden files in normal unit tests.

### Implementation Tasks

1. Add a corpus scanner:
   - Count top-level object types.
   - Count footprint child types.
   - Count pad types/shapes.
   - Count layer usage.
   - Count zone variants.

2. Add compatibility reports:
   - Supported object count.
   - Unsupported object count.
   - Preservation-only object count.

3. Add targeted fixtures copied from demos:
   - Small SMD footprint.
   - Through-hole footprint with teardrops.
   - Route arc/via sequence.
   - Copper polygon.
   - Basic zone.
   - Dimension.

4. Keep large demo files outside the repository unless explicitly approved.

### Tests

- Corpus scanner works against a configurable external demo directory.
- Scanner skips cleanly when demo directory is absent.
- Targeted fixtures cover every object type in the spec.

### Acceptance Criteria

- We can measure progress against real KiCad files.
- Adding support for a new KiCad object type has an obvious fixture and test
  location.

## Phase 12: Documentation And Public API Cleanup

### Objective

Make the PCB writer usable from Go code and from future AI generation layers.

### Implementation Tasks

1. Document package-level PCB writer usage.

2. Add builder examples:
   - Create board.
   - Add net.
   - Add footprint.
   - Add pads.
   - Add outline.
   - Add route.
   - Write file.

3. Review public structs:
   - Keep low-level model available for exact KiCad output.
   - Add higher-level helpers for common generated-board tasks.
   - Avoid exposing unstable parser internals.

4. Add examples as Go tests where practical.

5. Update specs if implementation intentionally diverges.

### Tests

- Go doc examples compile.
- Public helper examples generate internally valid boards.
- Existing examples still pass golden and validation tests.

### Acceptance Criteria

- A Go caller can generate a minimal KiCad 10 PCB with a small amount of code.
- The package boundary is clear enough for later AI schematic-to-PCB workflows.

## Suggested Commit Boundaries

1. `pcb: emit KiCad 10 header layers and setup`
2. `pcb: normalize nets and validation`
3. `pcb: add KiCad footprint properties and geometry`
4. `pcb: complete SMD and through-hole pad rendering`
5. `pcb: add board graphics and outline validation`
6. `pcb: add route arcs and via options`
7. `pcb: add zone rendering and validation`
8. `pcb: add KiCad CLI PCB validation harness`
9. `examples: add generated KiCad 10 PCB fixture`
10. `pcb: preserve unsupported parsed PCB nodes`
11. `pcb: add KiCad demo corpus compatibility scanner`
12. `docs: document PCB writer API`

## Risk Register

- KiCad may accept minimal files but rewrite them heavily on save. Mitigation:
  compare generated output to KiCad-upgraded/saved files and add missing defaults
  deliberately.
- Full footprint generation is larger than route serialization. Mitigation:
  implement simple generated footprints first, then add import/preservation.
- DRC can fail for reasons unrelated to syntax. Mitigation: separate syntax
  upgrade validation from DRC validation and document expected fixture issues.
- Demo files are large and varied. Mitigation: use targeted extracted fixtures
  and an optional corpus scanner instead of committing huge boards.
- KiCad format details may shift. Mitigation: keep version profiles explicit and
  centralize format constants.

## Definition Of Done

The PCB writer effort is done when:

- Generated `.kicad_pcb` files open in KiCad 10 without old-format warnings.
- The MVP generated board passes internal validation and KiCad CLI upgrade.
- DRC is automated for generated fixtures.
- SMD and through-hole footprints render with visible pads and correct nets.
- Routes, vias, outlines, graphics, and zones are supported.
- Unsupported imported KiCad nodes are preserved instead of silently dropped.
- Public Go helpers are documented and tested.
