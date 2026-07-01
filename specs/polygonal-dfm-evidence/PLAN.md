# Polygonal DFM Evidence Implementation Plan

## Objective

Implement deterministic polygon-backed fabrication evidence for copper and
solder-mask DFM checks while preserving conservative fail-closed behavior for
unsupported geometry.

Status: implemented through Phase 7. Remaining limitations, especially explicit
nested-ring/hole modeling for complex KiCad geometry, are documented as future
closeout work rather than silently accepted evidence.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests KiCad-independent.
- Do not claim manufacturer acceptance.
- Prefer structured geometry evidence over message parsing.
- Reuse existing PCB parser/model data and physical-rule report structures.
- Report unsupported geometry explicitly instead of silently skipping it.

## Phase 1: Geometry Primitive Foundation

### Goal

Add deterministic geometry primitives needed by physical DFM checks.

### Work

- Add internal geometry helpers under `internal/fabrication/physicalrules` or a
  small sibling package if reuse is clear.
- Define:
  - point in KiCad internal units, converting to millimeters only at
    measurement/reporting boundaries;
  - polygon with source metadata;
  - bounding box;
  - geometry support status and unsupported reason.
- Implement:
  - polygon closure normalization for edge iteration;
  - signed area and absolute area;
  - winding normalization;
  - bounding box;
  - point-on-segment;
  - segment intersection;
  - point-in-polygon;
  - point/segment and segment/segment distances.
- Keep numeric tolerances explicit and centralized.

### Tests

- Area and bounding-box fixtures.
- Point-in-polygon interior, edge, and exterior fixtures.
- Segment intersection fixtures for crossing, touching, parallel, and
  overlapping segments.
- Distance fixtures for point/segment and segment/segment cases.

### Verification

```sh
go test ./internal/fabrication/physicalrules
```

### Commit

```text
Add polygon geometry primitives for DFM checks
```

## Phase 2: Geometry Observation Extraction

### Goal

Normalize PCB copper and mask-related source geometry into reusable
observations.

### Work

- Add extraction helpers for:
  - zone declared polygons;
  - zone filled polygons;
  - copper-layer board drawings where polygon geometry is parsed;
  - footprint copper graphics where polygon geometry is parsed;
  - board outline polygons already derived from Edge.Cuts;
  - supported pad aperture approximations for mask web checks.
- Include source metadata:
  - source path;
  - UUID/object ID;
  - reference when applicable;
  - layer;
  - net code/name;
  - geometry kind.
- Mark unsupported geometry with explicit reasons:
  - too few distinct points;
  - self-intersection;
  - holes/cutouts not modeled in the first pass, with strict unsupported
    evidence so affected checks cannot silently pass;
  - unknown layer;
  - raw/unparsed geometry that may affect the check.
- Ensure extraction is deterministic by sorting observations by integer geometry
  keys first, such as layer, kind, bounds, and lexicographically ordered point
  coordinates, then source path and object ID. UUIDs must not be the only stable
  key when KiCad can regenerate them.

### Tests

- Filled zone polygon extraction.
- Copper graphic polygon extraction where existing models expose points.
- Board outline polygon observation extraction.
- Supported rectangular pad mask aperture extraction.
- Unsupported raw/ambiguous geometry evidence.
- Deterministic ordering.

### Verification

```sh
go test ./internal/fabrication/physicalrules ./internal/kicadfiles/pcb
```

### Commit

```text
Extract PCB geometry observations for DFM checks
```

## Phase 3: Conservative Polygon Width Estimation

### Goal

Measure simple polygon feature width well enough to catch obvious slivers and
fail closed when confidence is insufficient.

### Work

- Implement a deterministic conservative width estimator:
  - use parallel-edge distance for verified rectangles, with a bounding-box
    shortcut only for proven axis-aligned rectangles;
  - use all non-adjacent edge-distance checks and triangle-altitude measurements
    for rotated or simple non-rectangular polygons;
  - reject self-intersecting or complex polygons as unsupported.
- Return a structured result:
  - status: measured or unsupported;
  - minimum width in millimeters;
  - method name;
  - sample count when relevant;
  - unsupported reason.
- Avoid overstating confidence. If the estimator cannot produce a conservative
  lower bound, return unsupported evidence.
- If scanline sampling is added later, cap the interval at no more than half of
  the active `MinCopperFeatureMM` threshold and keep edge-distance evidence as
  the preferred deterministic method.

### Tests

- Wide rectangle passes expected width.
- Narrow rectangle detects sliver width.
- L-shaped or corridor polygon reports the narrow corridor conservatively.
- Self-intersecting polygon is unsupported.
- Degenerate polygon is unsupported.
- Estimator is stable across point winding and repeated closing point.

### Verification

```sh
go test ./internal/fabrication/physicalrules
```

### Commit

```text
Measure conservative polygon copper widths
```

## Phase 4: Copper Polygon DFM Checks

### Goal

Integrate polygon width and copper-to-edge measurements into physical-rule
reports.

### Work

- Add checks:
  - `physical.copper_sliver.filled_polygon_width`;
  - `physical.copper_sliver.copper_polygon_width`;
  - `physical.copper_sliver.polygon_edge_clearance`.
- For filled zone polygons:
  - measure minimum width;
  - compare to `Options.MinCopperFeatureMM`;
  - report checked/unsupported/violating counts and minimum observed width.
- For copper graphic polygons:
  - measure supported polygon widths;
  - compare to `Options.MinCopperFeatureMM`;
  - report unsupported graphics separately.
- For copper-to-edge:
  - measure supported copper polygon distance to board-outline polygons;
  - compare to `Options.MinCopperEdgeMM`;
  - skip with evidence when no usable outline exists.
- Keep existing track/arc/zone-min-thickness checks; the new checks add
  geometry evidence rather than replacing existing evidence immediately.

### Tests

- Filled zone polygon passes with width above profile threshold.
- Filled zone polygon blocks below threshold.
- Copper polygon blocks below threshold.
- Copper polygon edge clearance blocks below threshold.
- Unsupported polygon produces warning evidence and does not pass silently.
- Profile threshold changes affect pass/fail status.
- JSON report includes expected measurement names.

### Verification

```sh
go test ./internal/fabrication/physicalrules ./internal/fabrication
```

### Commit

```text
Add polygonal copper DFM checks
```

## Phase 5: Solder-Mask Polygon Web Checks

### Goal

Improve solder-mask web evidence for supported pad aperture geometries.

### Work

- Build supported mask-opening approximations for:
  - rectangular SMD pads;
  - circular, oval, oblong, and rounded-rectangle pads using polygonal
    aperture approximations rather than bounding-box-only web evidence;
  - supported same-side pads after pad-to-mask expansion.
- Add check:
  - `physical.solder_mask.polygon_web_width`.
- Measure polygon-to-polygon distances between same-side mask openings after
  AABB pruning by active web threshold, and add a reusable bucketed or indexed
  candidate lookup before applying the check to dense boards.
- Compare against `Options.MinSolderMaskWebMM`.
- Preserve the existing axis-aligned fast path if useful, but report whether
  evidence came from polygon geometry, exact primitive distance checks, or
  fallback approximation. Curved pads must use polygonal aperture approximations
  or exact primitive distances rather than bounding-box-only web evidence.
- Add unsupported geometry accounting for custom or malformed pad shapes while
  supporting arbitrary pad rotation for modeled aperture shapes.

### Tests

- Supported SMD pads with sufficient web pass.
- Supported SMD pads below web threshold block.
- Through-hole/circular approximation produces deterministic evidence.
- Rotated/custom unsupported pads emit unsupported evidence.
- Pad-to-mask clearance changes measured web.
- Profile threshold changes affect pass/fail status.

### Verification

```sh
go test ./internal/fabrication/physicalrules ./internal/fabrication
```

### Commit

```text
Add polygonal solder-mask web checks
```

## Phase 6: Strict Evidence Policy And Fabrication Integration

### Goal

Make unsupported polygonal DFM evidence visible in readiness outputs and
strict policies without making ordinary previews noisy.

### Work

- Add or reuse option/policy controls for unsupported geometry:
  - default: warning for unsupported relevant geometry;
  - strict/fabrication-candidate: blocking when unsupported geometry prevents
    proving a required DFM gate.
- Thread unsupported polygonal DFM findings into fabrication readiness results.
- Ensure `physical-rules.json`, readiness JSON, and package manifests expose:
  - unsupported count;
  - source object IDs;
  - profile threshold;
  - suggestion for KiCad DRC or parser expansion.
- Update design workflow promotion summaries to count these checks under
  physical/fabrication gates where already collected.

### Tests

- Unsupported polygon geometry warns by default.
- Unsupported polygon geometry blocks under strict evidence policy.
- Fabrication preview includes new physical-rule checks.
- Package manifest carries resulting physical-rule evidence status.
- Design promotion report includes blockers/warnings when physical-rule
  reports contain polygonal DFM issues.

### Verification

```sh
go test ./internal/fabrication ./internal/designworkflow ./cmd/kicadai
```

### Commit

```text
Integrate polygonal DFM evidence into fabrication readiness
```

## Phase 7: Documentation And Roadmap

### Goal

Document what polygonal DFM evidence proves and what remains delegated to KiCad
DRC or manufacturer review.

### Work

- Update `docs/fabrication.md`.
- Update `docs/validation-and-analysis.md` if report interpretation changes.
- Update `docs/cli-reference.md` if new flags or report fields are exposed.
- Update `README.md` status summary.
- Update `specs/ROADMAP.md` Priority 8 status and remaining work.

### Tests

- Run full test suite.

### Verification

```sh
go test ./...
```

### Commit

```text
Document polygonal DFM evidence
```
