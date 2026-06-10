# KiCad PCB Writer Specification

## Purpose

Build a Go PCB writer that emits KiCad `.kicad_pcb` files compatible with the
installed KiCad 10 toolchain and with the structure observed in KiCad-saved demo
projects. The writer must support AI-assisted board generation from higher-level
design intent, starting with simple two-layer boards and growing toward
full-fidelity project, schematic, and PCB generation.

This specification is based on representative boards in
`/Users/dshills/Documents/KiCad Demos`, including `complex_hierarchy`,
`royalblue54L_feather`, `tiny_tapeout`, `vme-wren`, `cm5_minima`, and
`jetson-agx-thor-baseboard`.

## Observed KiCad PCB File Shape

KiCad 10 writes PCB files as S-expressions with a top-level `kicad_pcb` node.
After running `kicad-cli pcb upgrade --force` on a KiCad 9 demo board, the
current KiCad 10 header is:

```scheme
(kicad_pcb
  (version 20260206)
  (generator "pcbnew")
  (generator_version "10.0")
  ...
)
```

The common top-level ordering in saved boards is:

1. `version`, `generator`, `generator_version`
2. `general`
3. `paper`
4. optional `title_block`
5. `layers`
6. `setup`
7. `net` declarations
8. `footprint` blocks
9. board graphics such as `gr_line`, `gr_rect`, `gr_circle`, `gr_arc`,
   `gr_poly`, and `gr_text`
10. route objects such as `segment`, route `arc`, and `via`
11. `zone` blocks
12. optional `group`, `dimension`, and `embedded_fonts`

The writer should emit this order deterministically so diffs remain readable and
generated files are easy to compare with KiCad-saved files.

## Compatibility Target

The writer must target KiCad 10 by default:

- `version`: `20260206`
- `generator`: `pcbnew` or a project-specific generator accepted by KiCad
- `generator_version`: `10.0`
- `general.thickness`: default `1.6`
- `general.legacy_teardrops`: `no`

The writer may retain options for older versions only behind explicit versioned
profiles. New generated examples must not trigger KiCad's "older version" banner.

## Layer Table Requirements

The default two-layer board profile must use KiCad-saved layer numbers, not the
current sparse writer numbering. Observed KiCad 10 default layer numbers are:

| Number | Name | Kind | Optional Display Name |
| --- | --- | --- | --- |
| 0 | `F.Cu` | `signal` | |
| 2 | `B.Cu` | `signal` | |
| 9 | `F.Adhes` | `user` | `F.Adhesive` |
| 11 | `B.Adhes` | `user` | `B.Adhesive` |
| 13 | `F.Paste` | `user` | |
| 15 | `B.Paste` | `user` | |
| 5 | `F.SilkS` | `user` | `F.Silkscreen` |
| 7 | `B.SilkS` | `user` | `B.Silkscreen` |
| 1 | `F.Mask` | `user` | |
| 3 | `B.Mask` | `user` | |
| 17 | `Dwgs.User` | `user` | `User.Drawings` |
| 19 | `Cmts.User` | `user` | `User.Comments` |
| 21 | `Eco1.User` | `user` | `User.Eco1` |
| 23 | `Eco2.User` | `user` | `User.Eco2` |
| 25 | `Edge.Cuts` | `user` | |
| 27 | `Margin` | `user` | |
| 31 | `F.CrtYd` | `user` | `F.Courtyard` |
| 29 | `B.CrtYd` | `user` | `B.Courtyard` |
| 35 | `F.Fab` | `user` | |
| 33 | `B.Fab` | `user` | |
| 39-127 | `User.1` through `User.45` | `user` | |

Requirements:

- Default output must include the full KiCad 10 layer table unless the user
  explicitly requests a smaller custom table.
- Copper layers must be validated by layer name and number.
- Footprint, pad, board graphic, route, zone, and dimension layers must exist in
  the board layer table.
- `*.Cu` and `*.Mask` wildcard pad layers must be supported for through-hole
  pads.

## Setup Requirements

The writer must support a KiCad 10 setup block with stable defaults:

```scheme
(setup
  (pad_to_mask_clearance 0)
  (allow_soldermask_bridges_in_footprints no)
  (tenting
    (front yes)
    (back yes)
  )
  (covering
    (front no)
    (back no)
  )
  (plugging
    (front no)
    (back no)
  )
  (capping no)
  (filling no)
  (pcbplotparams ...)
)
```

Stackup requirements:

- Support board thickness in `general.thickness`.
- Support optional `setup.stackup` when dielectric/copper detail is known.
- Emit default plot parameters compatible with KiCad 10 so generated files match
  KiCad-saved structure and can be plotted without KiCad adding large diffs on
  first save.

## Nets

PCB files declare nets as top-level nodes:

```scheme
(net 0 "")
(net 1 "GND")
(net 2 "+3V3")
```

Requirements:

- Net code `0` is reserved for the empty/no-net net.
- Net names must be unique.
- Net codes must be unique.
- Pads, tracks, vias, route arcs, copper graphics, and zones must reference
  declared net codes.
- Writer APIs should allow callers to refer to nets by name while the writer
  assigns deterministic numeric codes.
- A PCB generated from a schematic should preserve schematic net names and map
  them consistently into pad and route objects.

## Footprints

KiCad-saved boards embed footprint geometry. A board cannot be considered
complete if it only records a library identifier and assumes KiCad will look up
the full footprint later.

A generated footprint must support:

- Library identifier, for example `Resistor_SMD:R_0603_1608Metric`
- Board layer, UUID, absolute position, and rotation
- Properties, including at least:
  - `Reference`
  - `Value`
  - `Datasheet`
  - `Description`
  - optional user/BOM properties
  - optional `ki_fp_filters`
- Optional `path`, `sheetname`, and `sheetfile` for schematic association
- Attributes such as `smd`, `through_hole`, `exclude_from_pos_files`,
  `exclude_from_bom`, or board-only metadata when applicable
- Footprint graphics:
  - `fp_line`
  - `fp_rect`
  - `fp_circle`
  - `fp_arc`
  - `fp_poly`
  - `fp_text`
  - `fp_text_box` when needed
- Pads
- Optional `model` blocks with offset, scale, and rotation
- Optional per-footprint `embedded_fonts`

Properties are not plain text labels in KiCad 10 output. They are structured
`property` blocks with `at`, `layer`, `uuid`, optional `hide` and `unlocked`,
and `effects`.

## Pads

The writer must support SMD and through-hole pads in the first complete PCB
writer milestone.

Observed SMD pad shape:

```scheme
(pad "1" smd roundrect
  (at -0.5 0.75)
  (size 0.3 1.5)
  (layers "F.Cu" "F.Mask" "F.Paste")
  (roundrect_rratio 0.15)
  (net 1 "/ANT")
  (pinfunction "Pin_1")
  (pintype "passive")
  (thermal_bridge_angle 45)
  (uuid "...")
)
```

Observed through-hole pad shape:

```scheme
(pad "1" thru_hole rect
  (at 0 0)
  (size 2 2)
  (drill 1)
  (layers "*.Cu" "*.Mask")
  (remove_unused_layers no)
  (net 40 "+12V")
  (pintype "passive")
  (uuid "...")
)
```

Requirements:

- Pad type: `smd`, `thru_hole`, and later `np_thru_hole` and connector-specific
  pad types.
- Pad shape: `rect`, `circle`, `oval`, `roundrect`, and later custom pads.
- Position and rotation relative to the footprint.
- Size and optional drill.
- Layer list, including wildcard layers.
- Net code and net name rendering.
- Optional pin function and pin type.
- Optional `roundrect_rratio`.
- Optional `thermal_bridge_angle`.
- Optional `remove_unused_layers`.
- Optional teardrop settings.
- Unique UUID per pad.

## Board Graphics And Outline

The writer must support board-level graphics:

- `gr_line`
- `gr_rect`
- `gr_circle`
- `gr_arc`
- `gr_poly`
- `gr_text`

Every board graphic must include:

- Geometry
- `stroke` block with width and type
- Optional `fill`
- Layer
- UUID

The board outline is represented on `Edge.Cuts`. The writer must validate that
required board outlines are closed. For simple generated boards, the writer
should offer helpers for rectangular, rounded rectangle, and polygonal outlines.

Copper graphics are allowed and may carry net references, as observed in NFC
antenna demo files:

```scheme
(gr_poly
  (pts ...)
  (stroke (width 0) (type default))
  (fill yes)
  (layer "F.Cu")
  (net 1)
  (uuid "...")
)
```

## Routing

The writer must support route objects:

- `segment`
- route `arc`
- `via`

Observed route segment:

```scheme
(segment
  (start 150.95051 67.310695)
  (end 150.95051 70.905695)
  (width 0.381)
  (layer "F.Cu")
  (net 1)
  (uuid "...")
)
```

Observed via:

```scheme
(via
  (at 152.0 74.0)
  (size 1.27)
  (drill 0.7112)
  (layers "F.Cu" "B.Cu")
  (tenting front back)
  (net 1)
  (uuid "...")
)
```

Requirements:

- Track segments with start, end, width, layer, net, and UUID.
- Track arcs with start, mid, end, width, layer, net, and UUID.
- Vias with at, size, drill, layer pair/list, tenting, net, and UUID.
- Routing objects must reference existing nets and valid copper layers.
- Route endpoints should be validated against pad centers or explicit junctions
  where practical.

## Zones

KiCad zones are richer than the current writer model. They may represent copper
fills, keepouts, or generated teardrops.

Observed zone structure:

```scheme
(zone
  (net 1)
  (net_name "-VAA")
  (layer "F.Cu")
  (uuid "...")
  (name "$teardrop_padvia$")
  (hatch full 0.1)
  (priority 30001)
  (attr
    (teardrop
      (type padvia)
    )
  )
  (connect_pads yes
    (clearance 0)
  )
  (min_thickness 0.0254)
  (filled_areas_thickness no)
  (fill yes
    (thermal_gap 0.5)
    (thermal_bridge_width 0.5)
    (island_removal_mode 1)
    (island_area_min 10)
  )
  (polygon
    (pts ...)
  )
  (filled_polygon
    (layer "F.Cu")
    (pts ...)
  )
)
```

Requirements:

- Net code and net name.
- One layer or multiple layers.
- UUID, optional name, hatch style, and priority.
- Optional attributes, including teardrop metadata.
- `connect_pads` and clearance settings.
- Minimum thickness.
- Fill settings.
- One or more source polygons.
- Optional filled polygons. Generated boards may omit filled polygons initially
  if KiCad can refill zones, but imported boards need preservation support.
- Later support for keepout zones and rule-area zones.

## Dimensions, Groups, Fonts, And Preservation Nodes

The writer should support dimensions after the routing/zone milestone. KiCad
uses structured `dimension` nodes with type, layer, UUID, points, format, style,
and embedded `gr_text`.

Groups, embedded fonts, teardrops, and other KiCad-generated metadata should be
handled with a preservation mechanism:

- A parsed board should retain unknown or unsupported S-expression subtrees.
- A generated board may omit advanced nodes unless explicitly requested.
- A round-tripped board must not silently drop unsupported data.

## Validation Requirements

The writer must validate before writing:

- File version and generator metadata are present.
- Paper is present.
- General thickness is positive.
- Layer numbers and names are unique.
- All referenced layers exist.
- Net `0` exists and is empty.
- All referenced net codes exist.
- UUIDs are present and unique for objects that require them.
- Footprint references are unique unless intentionally duplicated for board-only
  objects.
- Pads have valid type, shape, size, layers, and UUID.
- SMD pads have copper/mask/paste layers appropriate to their side.
- Through-hole pads have drill size and copper/mask layers.
- Tracks, arcs, and vias use copper layers only.
- Vias have at least two copper layers.
- Zones have polygons with at least three points.
- Required board outlines on `Edge.Cuts` are geometrically closed.
- Coordinates and sizes are finite numeric values.

The writer should also provide an external validation helper that runs:

```sh
/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli pcb upgrade --force <copy>
/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli pcb drc --exit-code-violations <board>
```

The upgrade check catches old-format output. The DRC check catches invalid
geometry, connectivity, and manufacturing-rule issues.

## Current Writer Gaps

The current `internal/kicadfiles/pcb` writer is a useful start but does not yet
match KiCad 10 saved boards.

Known gaps:

- Uses old PCB version defaults in examples.
- Missing `generator_version`.
- `PCBGeneral` is empty and cannot emit thickness or `legacy_teardrops`.
- Default two-layer stack uses incorrect KiCad 10 layer numbers for `B.Cu`,
  silkscreen, and `Edge.Cuts`.
- Default layer table is too sparse for KiCad 10 saved-file compatibility.
- Setup block lacks many KiCad 10 defaults, including tenting shape and plot
  parameters.
- Footprint model lacks structured properties, attributes, sheet metadata,
  3D models, property UUIDs, and full KiCad 10 footprint graphics.
- Pads lack type, pin function, pin type, layer wildcard support,
  `remove_unused_layers`, teardrop settings, and net-name rendering.
- Board graphics are missing `gr_rect` and `gr_text`.
- Tracks lack route arcs.
- Vias lack tenting and extended via options.
- Zones are too minimal for KiCad-saved copper fills and teardrops.
- No preservation model exists for unsupported KiCad subtrees.

## MVP Acceptance Criteria

The first production-ready PCB writer milestone is complete when it can generate
a simple two-layer board that:

- Opens in KiCad 10 without the older-version warning.
- Contains a valid KiCad 10 header and setup block.
- Uses the canonical KiCad 10 layer table.
- Contains a closed `Edge.Cuts` outline.
- Declares deterministic nets including `0`.
- Places at least two footprints with embedded geometry and structured
  properties.
- Contains SMD pads connected to declared nets.
- Contains at least one route segment and one via.
- Optionally contains a simple copper zone.
- Passes internal writer validation.
- Passes `kicad-cli pcb upgrade --force` on a copied file without format changes
  that indicate old output.
- Runs through `kicad-cli pcb drc --exit-code-violations`; any expected DRC
  exclusions must be documented in the test fixture.

## Full-Fidelity Acceptance Criteria

The full PCB writer is complete when it can:

- Generate board files that KiCad 10 opens and saves with minimal structural
  churn.
- Represent all object classes observed in the demo boards.
- Preserve unsupported imported nodes during parse/write round trips.
- Generate boards from schematic connectivity with correct net and footprint pad
  mapping.
- Support both SMD and through-hole board examples.
- Support zone fills and board outlines robustly enough for realistic PCBs.
- Validate with `kicad-cli` in automated tests.
- Keep deterministic output for stable Git diffs.

## Non-Goals

- Autorouting is not part of this writer. Route objects may be generated by
  higher-level AI or separate routing tools and then serialized by this package.
- Full DRC rule authoring is not required in the initial writer. The writer only
  needs to serialize board geometry and invoke KiCad DRC as an external check.
- KiCad GUI automation is not required for writing `.kicad_pcb` files.
- KiCad 5/6/7/8 compatibility is not required unless a version profile is added
  later.
