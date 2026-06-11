# PCB Object Correctness Gap Matrix

Date: 2026-06-11

Source corpus: `$KICAD_DEMO_CORPUS`

Workspace default: set `$KICAD_DEMO_CORPUS` to the local KiCad demo directory.

Scan command: external KiCad demo corpus traversal using the in-repo PCB corpus scanner from `internal/kicadfiles/pcb`.

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

## Top-Level Board Objects

| Object | Count | Implementation status | Plan phase |
| --- | ---: | --- | --- |
| `segment` | 64,951 | Modeled, needs route correctness validation | Phase 6 |
| `via` | 10,159 | Modeled, needs via shape/layer/net validation | Phase 6 |
| `net` | 5,157 | Modeled | Existing |
| `footprint` | 4,154 | Modeled, needs metadata/property hardening | Phase 2 |
| `arc` | 4,552 | Modeled, needs route arc validation | Phase 6 |
| `gr_line` | 1,169 | Modeled, needs board graphic fixture coverage | Phase 5 |
| `gr_text` | 1,093 | Modeled, needs board text fixture coverage | Phase 5 |
| `zone` | 498 | Modeled, needs fill/thermal/island settings | Phase 7 |
| `gr_circle` | 346 | Modeled, needs board graphic fixture coverage | Phase 5 |
| `gr_arc` | 183 | Modeled, needs board graphic fixture coverage | Phase 5 |
| `gr_poly` | 134 | Modeled, needs board graphic fixture coverage | Phase 5 |
| `dimension` | 37 | Modeled for common KiCad dimension types; richer format/style nodes deferred | Phase 8 |
| `property` | 37 | Modeled for board metadata where applicable | Phase 2 |
| `group` | 20 | Preservation-only until modeled | Phase 8 |
| `embedded_fonts` | 18 | Preservation-only | Phase 8 |
| `gr_rect` | 15 | Modeled, needs board graphic fixture coverage | Phase 5 |
| `title_block` | 10 | Modeled | Existing |

## Footprint Child Objects

| Object | Count | Implementation status | Plan phase |
| --- | ---: | --- | --- |
| `property` | 94,889 | Modeled, needs exact KiCad save shape validation | Phase 2 |
| `fp_line` | 382,657 | Modeled, needs footprint graphic fixture coverage | Phase 4 |
| `pad` | 24,487 | Modeled, needs exhaustive pad correctness | Phase 3 |
| `fp_text` | 5,252 | Modeled, needs hidden/justify/effects validation | Phase 4 |
| `layer` | 4,547 | Modeled | Phase 2 |
| `attr` | 4,529 | Modeled, needs full flag coverage | Phase 2 |
| `embedded_fonts` | 4,459 | Preservation-only | Phase 8 |
| `uuid` | 4,273 | Modeled | Existing |
| `sheetfile` | 4,044 | Needs footprint provenance support | Phase 2 |
| `sheetname` | 4,021 | Needs footprint provenance support | Phase 2 |
| `path` | 4,021 | Needs footprint path support | Phase 2 |
| `model` | 3,818 | Modeled, needs transform validation | Phase 2 |
| `descr` | 3,762 | Needs footprint description support | Phase 2 |
| `fp_arc` | 2,951 | Modeled, needs fixture coverage | Phase 4 |
| `fp_circle` | 2,470 | Modeled, needs fixture coverage | Phase 4 |
| `fp_rect` | 2,337 | Modeled, needs fixture coverage | Phase 4 |
| `tags` | 2,159 | Needs footprint tag support | Phase 2 |
| `fp_poly` | 1,503 | Modeled, needs fixture coverage | Phase 4 |
| `duplicate_pad_numbers_are_jumpers` | 341 | Needs support | Phase 2 |
| `embedded_files` | 216 | Preservation-only | Phase 8 |
| `component_classes` | 96 | Preservation-only until modeled | Phase 8 |
| `locked` | 72 | Needs support | Phase 2 |
| `units` | 63 | Needs footprint unit metadata support | Phase 2 |
| `net_tie_pad_groups` | 19 | Needs support | Phase 2 |
| `fp_curve` | 18 | Needs modeling and fixture coverage | Phase 4 |
| `dimension` | 14 | Modeled for common KiCad dimension types; richer format/style nodes deferred | Phase 8 |
| `group` | 9 | Preservation-only until modeled | Phase 8 |
| `zone` | 9 | Needs footprint-local zone coverage | Phase 7 |

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

Pad gaps from corpus tokens:

| Token | Count | Gap |
| --- | ---: | --- |
| `pintype` | 18,490 | Add pin type preservation/emission where KiCad writes it |
| `pinfunction` | 15,392 | Add pin function preservation/emission where KiCad writes it |
| `remove_unused_layers` | 9,580 | Add pad layer cleanup flag support |
| `roundrect_rratio` | 7,360 | Validate roundrect ratio emission against KiCad |
| `keep_end_layers` | 4,836 | Add pad layer option support |
| `zone_layer_connections` | 4,836 | Add pad/zone connection option support |
| `thermal_bridge_angle` | 254 | Add thermal angle support |

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

Zone gaps from corpus tokens:

These counts are raw token hits across zone-adjacent settings and nested
objects, not one-to-one zone object counts. They are gap signals for fields the
writer must own or preserve.

| Token | Count | Gap |
| --- | ---: | --- |
| `connect_pads` | 774 | Validate clearance and zone pad connection emission |
| `min_thickness` | 774 | Add/validate minimum zone thickness |
| `filled_areas_thickness` | 754 | Add fill thickness setting |
| `priority` | 700 | Add priority ordering support |
| `thermal_gap` | 777 | Add thermal gap support |
| `thermal_bridge_width` | 777 | Add thermal bridge width support |
| `island_removal_mode` | 484 | Add island removal mode support |
| `island_area_min` | 470 | Add island area threshold support |

## Preservation-Only Objects

| Object | Count | Required behavior |
| --- | ---: | --- |
| `embedded_fonts` | 4,528 | Preserve on parse/write once preservation infrastructure exists; do not synthesize yet |
| `teardrops` | 549 | Preserve on parse/write; generated authoring can defer |
| `dimension` | 51 | Modeled for generated boards; preserve richer imported format/style details when parser work lands |
| `group` | 29 | Preserve on parse/write; do not synthesize until membership references are modeled |
| `embedded_files` | 223 | Preserve on parse/write; do not synthesize |
| `component_classes` | 96 | Preserve on parse/write; model later if needed |

## Parser Classification Notes

The corpus scanner intentionally over-reports nested tokens as unsupported when they are not top-level KiCad objects. High-frequency examples include `type`, `thickness`, `hide`, `unlocked`, `justify`, `offset`, `rotate`, `scale`, `front`, `back`, `center`, and setup-specific tokens such as `capping`, `covering`, `filling`, and `plugging`.

These are still useful as gap signals, but implementation should map them to the owning object model rather than treating each as an independent board object.

## Phase 1 Acceptance Criteria

This report is the Phase 1 output. Phase 1 is complete when this report and the
implementation plan are reviewed and committed.

- The measured object inventory is committed.
- Every high-frequency object class is mapped to a planned implementation or preservation phase.
- Pad, footprint, route, board-graphic, zone, group, and dimension gaps are explicitly identified.
- Later phases can use this matrix as the test fixture checklist.
