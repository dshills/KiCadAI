# Polygonal DFM Evidence Specification

## Objective

Strengthen fabrication readiness by replacing broad heuristic copper and
solder-mask geometry warnings with deterministic polygon-backed evidence where
KiCadAI has enough parsed geometry.

Status: implemented as a first conservative polygon-backed evidence pass.
Explicit nested-ring/hole support remains a documented future closeout gap.

The goal is to prove more than "the file parses": generated and inspected PCBs
should expose measurable evidence for copper islands, copper neckdowns,
polygonal copper containment, solder-mask web constraints, and unsupported
geometry boundaries before AI workflows claim fabrication-candidate quality.

## Motivation

The current fabrication physical-rule checks already cover:

- explicit track, arc, and copper drawing widths;
- zone `min_thickness`;
- conservative solder-mask web estimates between axis-aligned pads;
- board-outline containment for zones and drawings;
- profile-backed DFM thresholds and provenance.

Those checks are useful but incomplete. They do not yet derive polygonal copper
feature evidence from filled zone polygons, copper graphics, pad apertures,
or mask openings. As a result, some designs can look plausible while still
hiding narrow copper islands, narrow copper corridors, missing filled-zone
evidence, or solder-mask slivers that are only visible after geometry is
considered.

## Scope

This project adds a geometry evidence layer for PCB fabrication checks.

In scope:

- create reusable polygon geometry primitives for physical-rule checks;
- normalize PCB polygon inputs into layer/net-tagged geometry observations;
- derive conservative width/clearance measurements from polygon edges,
  bounding boxes, and sampled cross-sections;
- improve copper sliver evidence for parsed zones and copper polygons;
- improve solder-mask web evidence for supported pad geometries;
- distinguish unsupported geometry from passed geometry;
- add report evidence that explains what was measured, skipped, or blocked;
- keep default tests KiCad-independent.

Out of scope:

- replacing KiCad DRC;
- exact constructive solid geometry for every KiCad feature;
- field-solver impedance validation;
- manufacturer quote/upload validation;
- live fabricator APIs;
- automatic board repair in this project.

## Definitions

- **Geometry observation**: A normalized shape extracted from PCB data with
  source object ID, layer, net name/code where available, kind, polygon points,
  and confidence.
- **Supported polygon**: A simple polygon with enough points, finite
  coordinates, and a layer that can be interpreted by the target check. Holes
  or cutouts are supported only when represented as explicit inner rings by a
  check-specific implementation; otherwise they must be reported as unsupported
  geometry and strict evidence must fail closed.
- **Unsupported geometry**: A shape that may affect DFM evidence but cannot be
  safely measured by this implementation. Unsupported geometry must not be
  silently ignored.
- **Copper sliver**: A copper feature or corridor narrower than the active
  fabrication profile's `MinCopperFeatureMM`.
- **Solder-mask web**: The remaining mask material between adjacent exposed pad
  openings after pad-to-mask expansion is considered.

## Current Inputs

The project already parses or models:

- board zones, including `Polygons` and `FilledPolygons`;
- copper drawings and footprint copper graphics;
- tracks, track arcs, vias, pads, and pad layers;
- board outline polygons from Edge.Cuts;
- profile-backed physical-rule thresholds:
  - `MinCopperFeatureMM`;
  - `MinSolderMaskWebMM`;
  - `MinCopperEdgeMM`;
  - `MinHoleEdgeMM`.

The implementation must reuse existing parsed data where possible and avoid
ad hoc string parsing of raw KiCad text except as unsupported evidence.

## Geometry Model

Add an internal geometry layer for physical-rule checks. It may live under
`internal/fabrication/physicalrules` initially unless reuse pressure justifies a
separate package.

Required types:

- `GeometryPoint`
  - X/Y stored as `int64` KiCad internal units for robust deterministic
    geometry calculations, with `float64` used only for derived measurements,
    rotations, normalized vectors, and millimeter reporting.
- `GeometryPolygon`
  - ordered points;
  - closed/open input accepted but normalized to closed edge iteration;
  - source object ID;
  - source path such as `zones[0].filled_polygons[0]`;
  - layer;
  - net code/name where known;
  - kind: `zone_polygon`, `filled_zone_polygon`, `copper_graphic`,
    `pad_aperture`, `mask_opening`, `board_outline`;
  - supported flag and unsupported reason.
- `GeometryMeasurement`
  - name;
  - value;
  - unit;
  - object references;
  - sampling method.

Required helpers:

- polygon area;
- signed area and winding normalization;
- bounding box;
- point-in-polygon;
- segment intersection;
- point/segment distance;
- minimum edge-to-edge distance between polygons;
- conservative minimum polygon width estimate.

The minimum-width estimate does not need to be exact medial-axis analysis in
the first implementation. It must be deterministic, documented, and
conservative. Acceptable first-pass approaches include:

- parallel-edge distance for verified rectangular polygons, including rotated
  rectangles, with an axis-aligned bounding-box shortcut only when alignment is
  proven;
- minimum altitude for triangular polygons;
- edge-pair distance sampling for non-adjacent edges on other simple polygons;
- optional scanline sampling at intervals no larger than half of the active
  `MinCopperFeatureMM` profile threshold, with unsupported evidence if sampling
  cannot produce a conservative lower bound;
- reporting `unsupported` when confidence is not high enough.

## Copper DFM Checks

### Filled Zone Polygon Width

For each filled zone polygon on a copper layer:

- measure a conservative minimum width estimate;
- compare against active `MinCopperFeatureMM`;
- report:
  - checked polygon count;
  - unsupported polygon count;
  - minimum observed width;
  - required minimum width;
  - violating object IDs;
  - affected nets.

If a zone has no filled polygons but has unfilled `polygon` declarations, the
check must not claim polygonal copper evidence. It should emit skipped or
warning evidence depending on whether zone refill evidence is required by the
caller.

### Copper Graphic Polygon Width

For copper-layer `gr_poly` and footprint `fp_poly` geometry that is parsed into
PCB drawings/footprint graphics:

- convert supported polygons into geometry observations;
- measure conservative minimum width;
- compare against `MinCopperFeatureMM`;
- report unsupported graphics separately from passing graphics.

This extends, but does not remove, current stroke-width checks for tracks,
arcs, and line-like drawings.

### Copper-To-Edge Polygon Clearance

For supported copper polygons and filled zone polygons:

- measure minimum distance to board outline polygons;
- compare against active `MinCopperEdgeMM`;
- report the closest observed distance and violating objects.

If board outline geometry is unsupported, this check must not be reported as a
clean pass. Under strict evidence policy it should become warning or blocked
evidence because the copper-to-edge safety proof cannot be completed; only
non-strict summary views may classify the measured portion as skipped with a
clear unsupported-outline reason.

## Solder-Mask DFM Checks

### Supported Pad Apertures

For pads with supported shapes:

- derive a conservative mask opening polygon from pad geometry and
  `pad_to_mask_clearance`;
- resolve effective mask clearance in KiCad order: pad-level override first,
  footprint-level override second, then board/global solder-mask settings;
- include side/layer evidence;
- support common generated pad shapes first:
  - rectangular SMD pads;
  - circular, oval, oblong, and rounded-rectangle pads represented by polygonal
    aperture approximations or exact primitive distance checks;
  - round vias where mask exposure is modeled.

Unsupported rotated or custom pad shapes must be counted as unsupported
geometry unless a deterministic approximation is explicitly implemented and
reported.

### Mask Web Between Openings

For same-side mask openings:

- measure polygon-to-polygon distance;
- compare against active `MinSolderMaskWebMM`;
- report:
  - candidate opening count;
  - compared pair count;
  - unsupported opening count;
  - minimum observed web;
  - required web;
- violating pad/object IDs and references.

Existing axis-aligned pad checks may remain as a fast path, but curved pads must
not rely on bounding-box-only web evidence when that would understate the true
opening separation. The report must make clear whether the result came from
polygonal geometry, exact primitive distance checks, or fallback approximation.

Candidate selection must be spatially bounded. Generated-example scale may use
deterministic expanded-bounds pruning, but dense boards must use a reusable
spatial index or bucketed index so solder-mask web checks do not devolve into
unbounded all-pairs comparisons.

## Unsupported Evidence Policy

Unsupported geometry is a first-class result, not a silent skip.

Each relevant check should distinguish:

- `pass`: all supported measured geometry satisfies the profile;
- `warning`: some relevant geometry is unsupported, but measured supported
  geometry passes;
- `blocked`: measured supported geometry violates the profile, or caller policy
  requires full geometry proof and unsupported geometry exists;
- `skipped`: no relevant geometry exists or required upstream data is missing.

Unsupported evidence should include:

- source object IDs;
- source paths when available;
- unsupported reason;
- suggested next action, such as KiCad DRC evidence, zone refill, footprint
  hydration, or geometry parser expansion.

## Reporting Requirements

Physical-rule reports must add or enrich checks under existing categories:

- `physical.copper_sliver.filled_polygon_width`;
- `physical.copper_sliver.copper_polygon_width`;
- `physical.copper_sliver.polygon_edge_clearance`;
- `physical.solder_mask.polygon_web_width`;
- `physical.solder_mask.unsupported_polygon_geometry`.

Measurements should use stable names:

- `checked_polygon_count`;
- `unsupported_polygon_count`;
- `minimum_observed_polygon_width`;
- `minimum_required_copper_feature`;
- `minimum_observed_copper_edge_clearance`;
- `minimum_required_copper_edge_clearance`;
- `candidate_opening_count`;
- `compared_opening_pair_count`;
- `minimum_observed_solder_mask_web`;
- `minimum_required_solder_mask_web`.

Future reporting should add area coverage metrics such as
`total_checked_area` and `total_unsupported_area` once unsupported polygon
area can be computed without overstating confidence.

Reports must continue to include profile provenance added by the fabrication
profile expansion work.

## Validation Rules

The implementation must fail closed for malformed or ambiguous geometry:

- fewer than three distinct polygon points is unsupported;
- non-finite coordinates are unsupported;
- edge-crossing self-intersection is unsupported unless a deterministic
  decomposition exists; weakly simple vertex-touching cases may be promoted
  only after they are classified separately, decomposed or proven not to contain
  zero-width necks, and covered by tests;
- holes and complex zone cutouts are unsupported until explicitly modeled as
  inner rings or decomposed polygons, and strict policy must treat them as
  incomplete evidence rather than silently passing affected checks. Basic
  nested-ring support should be prioritized as the next geometry closeout item
  for dense real-world copper zones;
- unknown layer semantics are unsupported;
- raw KiCad nodes that are known to contain geometry but are not parsed must
  produce unsupported evidence.

## Test Strategy

Default tests must be hermetic and KiCad-independent.

Required tests:

- geometry primitive unit tests for area, bounding boxes, point-in-polygon,
  segment intersection, and distance;
- minimum-width estimator tests for simple rectangles, narrow corridors, and
  unsupported self-intersections;
- copper filled-zone pass/fail tests;
- copper polygon pass/fail tests;
- copper-to-edge clearance pass/fail tests;
- solder-mask web pass/fail tests for supported pad shapes;
- unsupported geometry reporting tests;
- profile-threshold sensitivity tests;
- report JSON shape tests.

Optional tests:

- KiCad-backed DRC comparison smoke tests gated by the existing KiCad CLI
  environment variables;
- corpus tests against downloaded KiCad demo boards where geometry is parsed
  well enough to compare with KiCad's own DRC findings.

## Acceptance Criteria

- Supported generated boards produce polygonal DFM evidence rather than only
  heuristic sliver warnings.
- Narrow polygonal copper features below the active profile threshold block
  fabrication readiness.
- Solder-mask web violations between supported pad openings block fabrication
  readiness.
- Unsupported geometry is visible in reports and can prevent a false-ready
  result when strict evidence is required.
- `go test ./...` remains KiCad-independent and passes.
