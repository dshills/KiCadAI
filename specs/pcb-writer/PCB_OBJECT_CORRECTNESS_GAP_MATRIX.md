# PCB Object Correctness Gap Matrix

Date: 2026-06-11

Source corpus: `$KICADAI_KICAD_DEMO_CORPUS`

Workspace default: set `$KICADAI_KICAD_DEMO_CORPUS` to the local KiCad demo directory and set `$KICADAI_RUN_KICAD_DEMO_CORPUS=1`.

Scan command:

```sh
KICADAI_RUN_KICAD_DEMO_CORPUS=1 \
KICADAI_KICAD_DEMO_CORPUS="/path/to/kicad-demos" \
go test ./internal/kicadfiles/pcb -run TestScanCorpusExternalKiCadDemos -count=1 -v
```

Latest scan result: passed during PCB Object Correctness Phase 10.

The source demo files are not copied into this repository. This document records the object coverage findings needed to drive the PCB object correctness implementation plan.

## Corpus Summary

| Metric | Count |
| --- | ---: |
| PCB files scanned | 345 |
| Footprints | 4,154 |
| Footprint child `pad` objects | 24,487 |
| Raw pad tokens, including nested pad-related scanner hits | 24,713 |
| Top-level route `segment` objects | 64,951 |
| Top-level route `arc` objects | 4,552 |
| Raw segment tokens, including nested scanner hits | 65,894 |
| Top-level `via` objects | 10,159 |
| Raw via tokens, including nested scanner hits | 10,342 |
| Zones | 498 |
| Board graphics | 2,940 |
| Footprint geometric graphics, raw child sum | 391,936 |
| Footprint text objects | 5,252 |
| Core preservation-only scanner hits | 5,157 |

## Phase 10 Coverage Summary

| Area | Before Object Correctness | After Phase 10 |
| --- | --- | --- |
| Footprint metadata | Properties, descriptions, sheet provenance, units, net-tie groups, lock state, and KiCad-saved flags were gaps | Modeled and validated for generated footprints |
| Pads | SMD/through-hole shapes existed but layer, drill, net, pin metadata, roundrect, and duplicate-layer checks were incomplete | Validated for generated SMD and through-hole pads with net-name/code consistency |
| Footprint graphics | Common graphics existed; curves were missing | `fp_line`, `fp_rect`, `fp_circle`, `fp_arc`, `fp_poly`, `fp_curve`, and text are covered by tests |
| Board graphics/outlines | Basic graphics existed; outline closure was strict and single-loop oriented | Closed line outlines support multiple loops and 0.0001 mm endpoint tolerance |
| Routes/vias | Route objects rendered but route-local net names and via layer spans were weakly checked | Segments, route arcs, and vias validate copper layers, geometry, net consistency, and via spans |
| Zones | Basic zones rendered; keepouts and richer fill settings were not explicit | Copper zones, filled polygons, keepouts, thermal settings, island removal, and layer declarations are validated |
| Dimensions | Treated as preservation-only in the first matrix | Common KiCad dimension types are modeled and rendered with nested `gr_text` |
| Groups/images/tables/targets | Unmodeled | Explicit preservation-only families with raw-node validation |
| Generated fixture | No single fixture exercised all core corrected PCB objects | `examples/08_pcb_object_correctness/pcb_object_correctness.kicad_pcb` is generated and sync-tested |

## Top-Level Board Objects

| Object | Count | Implementation status | Plan phase |
| --- | ---: | --- | --- |
| `segment` | 64,951 | Modeled with route correctness validation | Phase 6 |
| `via` | 10,159 | Modeled with via shape/layer/net validation | Phase 6 |
| `net` | 5,157 | Modeled | Existing |
| `footprint` | 4,154 | Modeled with metadata/property hardening | Phase 2 |
| `arc` | 4,552 | Modeled with route arc validation | Phase 6 |
| `gr_line` | 1,169 | Modeled with board graphic fixture coverage | Phase 5 |
| `gr_text` | 1,093 | Modeled with board text fixture coverage | Phase 5 |
| `zone` | 498 | Modeled with fill/thermal/island/keepout settings | Phase 7 |
| `gr_circle` | 346 | Modeled with board graphic fixture coverage | Phase 5 |
| `gr_arc` | 183 | Modeled with board graphic fixture coverage | Phase 5 |
| `gr_poly` | 134 | Modeled with board graphic fixture coverage | Phase 5 |
| `dimension` | 37 | Modeled for common KiCad dimension types; richer format/style nodes deferred | Phase 8 |
| `property` | 37 | Modeled for board metadata where applicable | Phase 2 |
| `group` | 20 | Preservation-only until membership references are modeled | Phase 8 |
| `embedded_fonts` | 18 | Preservation-only | Phase 8 |
| `gr_rect` | 15 | Modeled with board graphic fixture coverage | Phase 5 |
| `title_block` | 10 | Modeled | Existing |

## Footprint Child Objects

| Object | Count | Implementation status | Plan phase |
| --- | ---: | --- | --- |
| `property` | 94,889 | Modeled with KiCad-saved property shape validation | Phase 2 |
| `fp_line` | 382,657 | Modeled with footprint graphic fixture coverage | Phase 4 |
| `pad` | 24,487 | Modeled with generated SMD/through-hole correctness validation | Phase 3 |
| `fp_text` | 5,252 | Modeled with effects validation | Phase 4 |
| `layer` | 4,547 | Modeled | Phase 2 |
| `attr` | 4,529 | Modeled for generated footprint attributes | Phase 2 |
| `embedded_fonts` | 4,459 | Preservation-only | Phase 8 |
| `uuid` | 4,273 | Modeled | Existing |
| `sheetfile` | 4,044 | Modeled for footprint provenance | Phase 2 |
| `sheetname` | 4,021 | Modeled for footprint provenance | Phase 2 |
| `path` | 4,021 | Modeled for footprint path identity | Phase 2 |
| `model` | 3,818 | Modeled with transform validation | Phase 2 |
| `descr` | 3,762 | Modeled for generated footprints | Phase 2 |
| `fp_arc` | 2,951 | Modeled with fixture coverage | Phase 4 |
| `fp_circle` | 2,470 | Modeled with fixture coverage | Phase 4 |
| `fp_rect` | 2,337 | Modeled with fixture coverage | Phase 4 |
| `tags` | 2,159 | Modeled for generated footprints | Phase 2 |
| `fp_poly` | 1,503 | Modeled with fixture coverage | Phase 4 |
| `duplicate_pad_numbers_are_jumpers` | 341 | Modeled | Phase 2 |
| `embedded_files` | 216 | Preservation-only | Phase 8 |
| `component_classes` | 96 | Preservation-only until modeled | Phase 8 |
| `locked` | 72 | Modeled | Phase 2 |
| `units` | 63 | Modeled for footprint unit metadata | Phase 2 |
| `net_tie_pad_groups` | 19 | Modeled | Phase 2 |
| `fp_curve` | 18 | Modeled with validation coverage | Phase 4 |
| `dimension` | 14 | Modeled for common KiCad dimension types; richer format/style nodes deferred | Phase 8 |
| `group` | 9 | Preservation-only until membership references are modeled | Phase 8 |
| `zone` | 9 | Board zones modeled; footprint-local zones belong to the future import preservation project | Future import preservation |

## Pad Classifier Coverage

The pad type and shape counts below are classifier totals across board and
footprint-module corpus inputs. They are used to prioritize shape/type support;
the distinct footprint child `pad` object count above remains the object-count
baseline.

| Category | Value | Count | Plan phase |
| --- | --- | ---: | --- |
| Pad type | `smd` | 19,239 | Phase 3 |
| Pad type | `thru_hole` | 4,874 | Phase 3 |
| Pad type | `connect` | 481 | Phase 3 |
| Pad type | `np_thru_hole` | 119 | Phase 3 |
| Pad shape | `rect` | 9,339 | Phase 3 |
| Pad shape | `roundrect` | 7,360 | Phase 3 |
| Pad shape | `circle` | 6,431 | Phase 3 |
| Pad shape | `oval` | 1,431 | Phase 3 |
| Pad shape | `custom` | 152 | Phase 3 |

Pad status from corpus tokens:

| Token | Count | Status |
| --- | ---: | --- |
| `pintype` | 18,490 | Modeled for generated pads |
| `pinfunction` | 15,392 | Modeled for generated pads |
| `remove_unused_layers` | 9,580 | Modeled for generated pads |
| `roundrect_rratio` | 7,360 | Modeled and validated for roundrect pads |
| `keep_end_layers` | 4,836 | Future import preservation project: pad layer option support |
| `zone_layer_connections` | 4,836 | Future import preservation project: pad/zone connection option support |
| `thermal_bridge_angle` | 254 | Modeled for generated pads |

## Zone Coverage

| Zone layer | Count |
| --- | ---: |
| `B.Cu` | 767 |
| `F.Cu` | 725 |
| `In4.Cu` | 88 |
| `In2.Cu` | 54 |
| `In5.Cu` | 34 |
| `In1.Cu` | 31 |
| `In9.Cu` | 14 |
| `In3.Cu` | 10 |
| `In7.Cu` | 10 |
| `In6.Cu` | 6 |
| `In8.Cu` | 2 |
| `In10.Cu` | 1 |

Zone status from corpus tokens:

These counts are raw token hits across zone-adjacent settings and nested
objects, not one-to-one zone object counts. They are gap signals for fields the
writer must own or preserve.

| Token | Count | Status |
| --- | ---: | --- |
| `connect_pads` | 774 | Modeled and validated |
| `min_thickness` | 774 | Modeled and validated |
| `filled_areas_thickness` | 754 | Modeled |
| `priority` | 700 | Modeled and validated |
| `thermal_gap` | 777 | Modeled and validated |
| `thermal_bridge_width` | 777 | Modeled and validated |
| `island_removal_mode` | 484 | Modeled and validated |
| `island_area_min` | 470 | Modeled and validated |

## Preservation-Only Objects

| Object | Count | Required behavior |
| --- | ---: | --- |
| `embedded_fonts` | 4,528 | Preserve on parse/write once preservation infrastructure exists; do not synthesize yet |
| `teardrops` | 549 | Preserve on parse/write; generated authoring can defer |
| `dimension` | 51 | Modeled for generated boards; preserve richer imported format/style details when parser work lands |
| `group` | 29 | Preserve on parse/write; do not synthesize until membership references are modeled |
| `embedded_files` | 223 | Preserve on parse/write; do not synthesize |
| `component_classes` | 96 | Preserve on parse/write; model later if needed |

## Remaining Priorities

1. Connectivity/DRC project:
   - Add geometry-aware endpoint connectivity checks across pads, tracks,
     arcs, and vias.
   - Add opt-in KiCad DRC execution for generated fixtures.
   - Record allowed DRC baselines explicitly when a fixture intentionally
     exercises a violation.
2. Symbol/Footprint Library Mapping project:
   - Map schematic symbols to generated or library footprints deterministically.
   - Generate project-local footprint libraries when embedded geometry should
     be reusable outside a single board file.
   - Validate pin-to-pad mapping before PCB generation.
3. Import preservation project:
   - Parse and re-emit group membership, embedded files, component classes,
     richer dimension format/style nodes, and low-frequency pad/zone options.
   - Keep unsupported imported nodes attached to their original owning object.

## Parser Classification Notes

The corpus scanner intentionally over-reports nested tokens as unsupported when they are not top-level KiCad objects. High-frequency examples include `type`, `thickness`, `hide`, `unlocked`, `justify`, `offset`, `rotate`, `scale`, `front`, `back`, `center`, and setup-specific tokens such as `capping`, `covering`, `filling`, and `plugging`.

These are still useful as gap signals, but implementation should map them to the owning object model rather than treating each as an independent board object.

## Phase 10 Acceptance Criteria

This report now includes the Phase 10 coverage update. Phase 10 is complete
when this report is reviewed and committed.

- The measured object inventory is still committed.
- Each Object Correctness object family has an after-state.
- Remaining object gaps are explicitly scoped to future projects.
- The external KiCad demo corpus scan passes locally.
