# PCB Object Correctness Implementation Plan

## Purpose

Move the PCB writer from "KiCad can parse the file" to "KiCad can edit, DRC,
plot, save, and preserve generated objects without surprising rewrites."

This plan follows the roadmap item `PCB Object Correctness` and assumes the
round-trip harness and golden corpus scanner already exist. The work should be
implemented in small phases, with Prism review and a commit after each phase.

Target save format: the current local KiCad save format and the writer headers
already produced by this project. Compatibility with older KiCad release
branches requires an explicit compatibility phase before changing output
semantics.

## Scope

In scope:

- Footprints
- Footprint properties
- Footprint graphics
- Pads
- Tracks
- Route arcs
- Vias
- Zones
- Board graphics
- Text
- Groups
- Dimensions
- Properties and metadata needed for KiCad-native behavior
- Validation and golden/corpus fixtures for each object family

Out of scope for this project:

- Full autorouting
- Schematic-to-PCB netlist import
- Full parser/read-modify-write preservation beyond object correctness fixtures
- Perfect support for every historical KiCad version

## Ground Rules

- Use KiCad source and KiCad-saved demo files as the source of truth.
- Keep generated output deterministic.
- Treat the board net registry as the source of truth for net code/name
  mappings; object-local net names must be synchronized from the registry
  before emission.
- Prefer focused object-family commits over broad rewrites.
- Add validation in the same phase as rendering changes.
- Add golden tests for syntax and focused tests for invalid input.
- Use targeted demo-derived fixtures instead of checking in full large demo
  boards.
- Run `gofmt -w` on changed Go files.
- Run `GOCACHE=/tmp/kicadai-gocache go test ./...` before each commit.
- Run Prism review before each commit.
- When object output changes are user-visible, run opt-in KiCad checks where
  possible:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1 \
KICADAI_ROUNDTRIP_ARTIFACT_DIR="$(pwd)/examples/roundtrip_artifacts" \
go test ./internal/kicadfiles/roundtrip
```

## Current Baseline

Already present in `internal/kicadfiles/pcb`:

- Current target header/setup/layer rendering for the target writer version.
- Net registry and net validation.
- Footprint model with properties, text, graphics, pads, 3D models, and
  `embedded_fonts`.
- Pad model with SMD, through-hole fields, wildcard layers, net name/code,
  pin function/type, roundrect ratio, remove-unused-layers, thermal bridge
  angle, and teardrop fields; demo-derived teardrops still need
  preservation/correctness treatment in Phase 8.
- Board drawing model for lines, rectangles, circles, arcs, polygons, and text.
- Track, route arc, via, zone, filled polygon, zone attributes, and dimension
  models.
- Corpus scanner for `.kicad_pcb` and `.kicad_mod` files.
- Round-trip validation harness for generated PCB files.

This plan is therefore not a greenfield model build. It is a correctness pass
over supported objects, their KiCad-shaped output, and their validation.

## Phase 1: Object Inventory And Gap Matrix

### Goal

Build an explicit gap matrix from real KiCad files so object work is driven by
evidence instead of memory.

### Tasks

1. Run the golden corpus scanner against the local KiCad demo directory.
2. Capture object counts for:
   - top-level PCB objects
   - footprint child objects
   - pad types
   - pad shapes
   - layer usage
   - zone layers
   - preservation-only nodes
   - unsupported nodes
3. Add a checked-in markdown report under `specs/pcb-writer/` summarizing:
   - supported modeled objects
   - preservation-only objects
   - unsupported object families
   - high-frequency corpus gaps
4. Add targeted TODO fixture list for each unsupported/high-risk object family.
5. Keep raw demo boards out of the repository.

### Tests

- Normal corpus unit tests must pass.
- Optional external corpus test must pass locally:

```sh
KICADAI_RUN_KICAD_DEMO_CORPUS=1 \
KICADAI_KICAD_DEMO_CORPUS="$HOME/Documents/KiCad Demos" \
go test ./internal/kicadfiles/pcb -run TestScanCorpusExternalKiCadDemos -count=1
```

### Acceptance Criteria

- The repo contains a clear object gap matrix.
- Every later object phase can point to a corpus count or targeted fixture.
- Unsupported but common object types are explicitly tracked.

### Commit

`pcb: document object correctness gaps`

## Phase 2: Footprint Properties And Metadata

### Goal

Make generated footprints look like KiCad-saved footprints, including
properties and metadata KiCad expects.

### Tasks

1. Review KiCad-saved footprint blocks from demos and generated round-trip
   artifacts.
2. Verify or fix footprint rendering order:
   - `footprint`
   - `layer`
   - `uuid`
   - `at`
   - `descr`
   - `tags`
   - structured `property` blocks
   - `path`, `sheetname`, `sheetfile`
   - `attr`
   - graphics
   - pads
   - `embedded_fonts`
   - `model`
3. Ensure standard properties render as KiCad properties, not text labels:
   - `Reference`
   - `Value`
   - `Footprint`
   - `Datasheet`
   - `Description` where observed
4. Validate:
   - footprint UUID required
   - library ID required
   - layer exists
   - property UUID required
   - duplicate property names rejected unless KiCad allows repeats
   - sheet metadata paths are valid when present
5. Add or refine helper constructors for common generated footprints.
6. Add targeted golden tests for:
   - SMD resistor/capacitor footprint
   - through-hole connector footprint
   - footprint with model block
   - footprint with `embedded_fonts`

### Tests

- Unit render tests for footprint property ordering and syntax.
- Validation tests for missing UUIDs and duplicate properties.
- Existing example board tests pass.

### Acceptance Criteria

- KiCad does not rewrite a generated footprint wholesale on first save.
- Footprint properties are editable in KiCad as properties.
- Generated footprints have visible geometry, not only library references.

### Commit

`pcb: correct footprint properties and metadata`

## Phase 3: Pad Correctness

### Goal

Make pads complete enough for KiCad editing, DRC, plotting, net connectivity,
and round-trip stability.

### Tasks

1. Compare generated pads with KiCad-saved SMD and through-hole pads.
2. Confirm pad rendering order:
   - `pad`
   - type and shape
   - `at`
   - `size`
   - optional `drill`
   - `layers`
   - optional `remove_unused_layers`
   - optional `roundrect_rratio`
   - `net`
   - `pinfunction`
   - `pintype`
   - optional `thermal_bridge_angle`
   - optional teardrops
   - `uuid`
3. Validate SMD pads:
   - allowed types/shapes
   - positive size
   - no required drill
   - valid F/B side layer combinations
   - paste/mask side consistency
4. Validate through-hole pads:
   - drill required and positive
   - copper/mask wildcard layers allowed
   - invalid wildcard layers rejected
5. Validate nets:
   - net code exists
   - net name/code agree when both are present, using the board net registry as
     the authority
   - unconnected pads emit the target writer version's explicit `(net 0 "")`
     representation, with Phase 3 fixture/source verification before changing
     writer behavior
6. Add targeted fixtures for:
   - SMD roundrect pad
   - SMD rect pad
   - through-hole rect pad
   - through-hole circle/oval pad
   - teardrop-bearing pad as preservation-only or modeled if supported

### Tests

- Golden tests for each pad type/shape.
- Validation tests for missing drill, invalid layer, invalid net, and
  net-name mismatch.
- Optional round-trip test for a pad-heavy generated board.

### Acceptance Criteria

- SMD and through-hole generated pads are visible and editable in KiCad.
- Pad nets appear connected in KiCad.
- First KiCad save does not reorder or rewrite complete pad blocks heavily.

### Commit

`pcb: tighten pad rendering and validation`

## Phase 4: Footprint Graphics And Text

### Goal

Ensure generated footprint drawings and text render like KiCad footprint
geometry.

### Tasks

1. Validate and render:
   - `fp_line`
   - `fp_rect`
   - `fp_circle`
   - `fp_arc`
   - `fp_poly`
   - `fp_curve`
   - `fp_text`
   - `fp_text_box` if corpus shows it often enough for this phase
2. Confirm footprint graphic ordering inside footprints.
3. Validate:
   - UUID required
   - layer exists
   - layer is appropriate for footprint-local graphics and text
   - stroke width non-negative or positive where KiCad requires it
   - polygon point count
   - text string required for text objects
   - text effects valid
4. Add fixture snippets copied or minimized from demo footprints.

### Tests

- Golden render tests for each modeled footprint graphic.
- Validation tests for missing UUID, invalid layer, bad point counts.
- Corpus report should show fewer unsupported footprint child types or explicitly
  classify remaining ones as preservation-only.

### Acceptance Criteria

- Generated footprints have real outline, courtyard, fab, and silkscreen
  geometry.
- KiCad does not replace footprint text/graphics with defaults on save.

### Commit

`pcb: correct footprint graphics and text`

## Phase 5: Board Graphics, Text, And Outline Closure

### Goal

Make board-level drawings correct enough for fabrication outlines, labels,
polygons, and copper graphics.

### Tasks

1. Verify board graphic syntax against KiCad-saved boards:
   - `gr_line`
   - `gr_rect`
   - `gr_circle`
   - `gr_arc`
   - `gr_poly`
   - `gr_text`
2. Ensure each graphic emits:
   - geometry
   - layer
   - width/stroke
   - fill where supported
   - UUID
   - effects for text
   - net when copper graphics support it
3. Improve outline validation:
   - closed `Edge.Cuts` line chains pass
   - multiple independent closed `Edge.Cuts` loops pass for internal cutouts
     and slots
   - coincident endpoints are matched with a 0.0001 mm tolerance to avoid false
     failures from floating-point formatting
   - `gr_rect` on `Edge.Cuts` passes
   - open edge chains fail with endpoint details
   - mixed valid outline primitives are accepted when they form a closed shape
4. Add helper APIs for:
   - rectangular board outline
   - rounded rectangle outline if supported
   - polygon outline
5. Add targeted corpus fixtures for graphics/drawings.

### Tests

- Golden tests for each board graphic type.
- Closed outline validation passes.
- Open outline validation fails with actionable coordinates.
- Text rendering includes layer, UUID, effects, and justification.

### Acceptance Criteria

- Generated boards have a valid visible outline.
- Board labels and copper graphics open/edit cleanly in KiCad.
- DRC no longer fails due only to missing/open board outline in generated
  fixtures.

### Commit

`pcb: correct board graphics and outline validation`

## Phase 6: Routes, Arcs, And Vias

### Goal

Make generated route objects electrically meaningful and KiCad-native.

### Tasks

1. Compare generated `segment`, route `arc`, and `via` output with KiCad-saved
   demo routes.
2. Confirm render order and required fields:
   - segment start/end/width/layer/net/uuid
   - arc start/mid/end/width/layer/net/uuid
   - via at/size/drill/layers/net/uuid/tenting when present
3. Validate:
   - width positive
   - route layers are copper
   - layers exist in board layer table
   - nets exist
   - via spans at least two copper layers
   - via drill/size positive and drill smaller than size
   - route arc start/mid/end are distinct
4. Add generated fixture with:
   - front copper segment
   - via to back copper
   - back copper segment
   - route arc
5. Add optional connectivity helper that checks route endpoints touch pads,
   vias, or same-net route endpoints.

### Tests

- Golden render tests for segment, arc, via.
- Validation tests for invalid layer, net, via geometry, and degenerate arc.
- Optional generated fixture passes KiCad CLI upgrade.

### Acceptance Criteria

- A generated two-layer routed net appears connected in KiCad.
- Route objects are editable and preserved on KiCad save.

### Commit

`pcb: correct route segments arcs and vias`

## Phase 7: Zones And Copper Fills

### Goal

Support basic copper pours and preserve/validate richer KiCad zone structures.

### Tasks

1. Compare zone output with KiCad-saved zones from demo boards.
2. Confirm render support for:
   - net code/name
   - zone name
   - layers
   - hatch style/pitch
   - priority
   - connect pad mode
   - clearance
   - minimum thickness
   - fill settings
   - source polygons
   - filled polygons when supplied
   - attributes
3. Validate:
   - UUID required
   - at least one valid copper layer
   - net exists
   - polygons have at least three distinct points
   - filled polygons use declared zone layers
   - priority non-negative
   - clearance/min thickness non-negative
4. Add helper for rectangular copper pour inside a board outline.
5. Keep generated zone output deterministic:
   - omit filled polygons by default and document the generated file as
     intentionally unfilled until KiCad refills it
   - emit filled polygons only when deterministic fill geometry is explicitly
     supplied by the caller
6. Add or document a KiCad CLI refill helper in the generation pipeline so
   consumers that need export-ready copper can produce a deterministic
   post-KiCad artifact.

### Tests

- Golden test for simple GND zone.
- Golden test for multi-layer zone.
- Validation tests for missing net, invalid layer, bad polygon, and filled
  polygon layer mismatch.
- Optional KiCad CLI refill test if supported; refill output is validated as an
  external KiCad artifact, not as the deterministic writer golden file.

### Acceptance Criteria

- Generated GND pour is accepted by KiCad and can be refilled.
- KiCad does not drop zone metadata on save.

### Commit

`pcb: correct zone rendering and validation`

## Phase 8: Groups, Dimensions, And Preservation-Only Objects

### Goal

Handle lower-frequency KiCad board objects deliberately instead of accidentally
dropping or misclassifying them.

### Tasks

1. Use corpus report to identify group/dimension/table/image/target frequency.
2. For dimensions:
   - model and validate common dimension types
   - render layer, UUID, points, height, text, and effects
3. For groups:
   - decide whether to model membership fully or preserve raw nodes for now
   - avoid generating invalid group references
4. For lower-frequency objects:
   - classify as modeled, preservation-only, or unsupported
   - add targeted fixtures for each classification
5. Ensure preservation-only objects are never silently dropped by future parser
   work.

### Tests

- Golden render test for modeled dimension.
- Validation tests for malformed dimension geometry.
- Corpus classification test for preservation-only objects.
- Documentation lists unsupported/preservation-only object families.

### Acceptance Criteria

- The writer has an explicit story for every object family seen in the corpus.
- Unsupported objects are tracked and do not block generated-board correctness.

### Commit

`pcb: classify groups dimensions and preserved objects`

## Phase 9: Generated Correctness Fixture

### Goal

Create a small generated board that exercises all corrected core object types.

### Fixture Requirements

- Two-layer board.
- Closed board outline.
- At least:
  - one SMD resistor/capacitor footprint
  - one SMD LED or IC-like footprint
  - one through-hole connector
  - front copper route
  - back copper route
  - via
  - route arc
  - GND zone
  - silkscreen text
  - fab/courtyard footprint graphics
- Deterministic UUIDs.
- Deterministic net names.
- No dependency on KiCad library lookup for visible footprint geometry.

### Tasks

1. Add or update an example under `examples/`.
2. Add generated golden output test or stable snapshot comparison.
3. Add internal validation test.
4. Add optional KiCad CLI upgrade test.
5. Add optional KiCad DRC test with explicit fixture gates:
   - no disconnected pad violations
   - no clearance violations
   - no malformed board outline violations
   - any remaining expected violation must be committed as an explicit fixture
     baseline
6. Document how to regenerate and validate the fixture.

### Tests

- `go test ./...`
- Optional round-trip/upgrade validation.
- Optional DRC validation.

### Acceptance Criteria

- The fixture opens in KiCad without old-format warnings.
- Core object types are visible and editable.
- Internal validation passes.
- KiCad CLI upgrade passes when enabled.

### Commit

`examples: add pcb object correctness fixture`

## Phase 10: Corpus Coverage Report Update

### Goal

Close the loop against the real KiCad corpus after object correctness work.

### Tasks

1. Rerun the external corpus scanner.
2. Update the object gap matrix.
3. Show which corpus object counts are now:
   - modeled
   - preservation-only
   - unsupported
4. Add next-project notes for Connectivity/DRC and Symbol/Footprint Library
   Mapping.

### Tests

- Corpus scanner unit tests pass.
- Optional external corpus scan passes.

### Acceptance Criteria

- The roadmap has a clear before/after view.
- Remaining object gaps are explicit and prioritized.

### Commit

`docs: update pcb object corpus coverage`

## Suggested Execution Order

1. Phase 1: inventory and matrix.
2. Phase 2: footprint properties and metadata.
3. Phase 3: pads.
4. Phase 4: footprint graphics and text.
5. Phase 5: board graphics and outline closure.
6. Phase 6: routes, arcs, vias.
7. Phase 7: zones.
8. Phase 8: groups, dimensions, preservation-only objects.
9. Phase 9: generated correctness fixture.
10. Phase 10: corpus report update.

## Definition Of Done

PCB Object Correctness is complete when:

- The object gap matrix has been updated from the real KiCad demo corpus.
- Footprints, pads, footprint graphics, board graphics, routes, vias, zones,
  dimensions, and preservation-only object families have explicit tests.
- Generated footprints contain enough embedded geometry to be useful without
  KiCad library lookup.
- Generated SMD and through-hole pads are valid and connected to declared nets.
- Generated route objects and zones reference valid nets and layers.
- Generated board outline validation prevents open outlines.
- A generated correctness fixture passes internal validation.
- Optional KiCad CLI upgrade passes for the generated correctness fixture.
- `GOCACHE=/tmp/kicadai-gocache go test ./...` passes.
- Each phase has been Prism-reviewed and committed.
