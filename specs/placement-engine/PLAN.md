# Placement Engine Implementation Plan

## Objective

Implement a deterministic placement engine that produces collision-free,
constraint-aware PCB footprint placements from design intent and emits existing
transaction operations for the PCB writer.

The first implementation should favor correctness, stable output, and clear
diagnostics over clever optimization.

## Implementation Rules

- Keep placement in `internal/placement`.
- Do not write KiCad files directly from the placement package.
- Emit `transactions.PlaceFootprintOperation` operations for application.
- Keep all algorithms deterministic for the same request and seed.
- Return structured `reports.Issue` values for all placement problems.
- Treat unknown footprint bounds as a warning only when explicit estimated
  bounds are provided; otherwise block placement.
- Run `gofmt` and `go test ./...` before each phase commit.
- Run `prism review staged` before each phase commit and address actionable
  findings.

## Phase 1: Core Models And Validation

### Goals

Add the placement package, request/result models, default rules, and input
validation.

### Tasks

1. Add `internal/placement`.
2. Define:
   - `Request`;
   - `BoardPlacementArea`;
   - `Component`;
   - `Bounds`;
   - `PadSummary`;
   - `Net`;
   - `Group`;
   - `Keepout`;
   - `Rules`;
   - `Result`;
   - `PlacementResult`;
   - `Metrics`.
3. Add default rule normalization.
4. Validate required fields:
   - positive board dimensions;
   - unique references;
   - known bounds for required components;
   - fixed placements include coordinates;
   - groups reference existing components;
   - nets reference existing component pins when endpoints are provided.
5. Add unit tests for valid requests and validation failures.

### Acceptance Criteria

- `go test ./internal/placement` passes.
- Invalid requests return structured blocking issues.
- Valid minimal requests normalize defaults.

### Suggested Commit

`Add placement engine models`

## Phase 2: Geometry And Occupancy

### Goals

Represent component rectangles, board area, keepouts, collision checks, and
placement occupancy.

### Tasks

1. Add geometry types:
   - `Point`;
   - `Rect`;
   - `Placement`;
   - rotation helpers.
2. Convert component bounds and placement into occupied rectangles.
3. Implement:
   - board containment check;
   - rectangle intersection;
   - keepout intersection;
   - spacing expansion;
   - fixed-placement validation.
4. Add deterministic occupancy map.
5. Add tests for:
   - overlap;
   - edge clearance;
   - rotation swapping width/height;
   - keepout rejection;
   - fixed placement preservation.

### Acceptance Criteria

- Geometry checks are deterministic and covered by table tests.
- Fixed placement conflicts produce blocking issues.

### Suggested Commit

`Add placement geometry checks`

## Phase 3: Deterministic Shelf/Grid Placer

### Goals

Place unconstrained components inside a rectangular board with no overlaps.

### Tasks

1. Implement placement order:
   - fixed first;
   - higher group priority;
   - higher component priority;
   - role order;
   - reference designator.
2. Implement grid candidate generation.
3. Add candidate scoring:
   - inside board;
   - no collision;
   - no keepout;
   - stable top-left or center preference;
   - rough net distance to already placed components.
4. Place all remaining components or return partial/blocking issues.
5. Compute metrics:
   - placed count;
   - unplaced count;
   - collision count;
   - outside outline count;
   - estimated bounds count;
   - rough HPWL.
6. Add tests for deterministic output and impossible placement.

### Acceptance Criteria

- Same request produces identical placements.
- Components do not overlap.
- Components are inside board margins.
- Impossible placement returns `blocked`.

### Suggested Commit

`Implement deterministic footprint placement`

## Phase 4: Edge Constraints And Rotation

### Goals

Support connectors and user-facing components on preferred board edges with
controlled rotation.

### Tasks

1. Add side constraints:
   - top only;
   - bottom only;
   - either side.
2. Add edge constraints:
   - left;
   - right;
   - top;
   - bottom;
   - any.
3. Add rotation constraints:
   - fixed angle;
   - allowed angles;
   - default by edge.
4. Place edge-constrained components before ordinary components.
5. Score candidates by edge distance and edge clearance.
6. Add tests for edge connector placement and rotation satisfaction.

### Acceptance Criteria

- Edge-constrained components are placed on requested edges.
- Rotation constraints are enforced.
- Violations return blocking issues.

### Suggested Commit

`Support placement edge constraints`

## Phase 5: Group-Aware Placement

### Goals

Keep related components close and place common circuit groups coherently.

### Tasks

1. Implement group normalization and ordering.
2. Place primary group component first.
3. Place keep-close components around the group anchor using deterministic
   patterns.
4. Enforce `MaxSpreadMM` when provided.
5. Add simple role heuristics:
   - decoupling capacitor close to MCU/regulator;
   - regulator input/output caps near regulator;
   - op-amp feedback parts near op-amp;
   - connector breakout row alignment.
6. Add group spread metrics and warnings.
7. Add tests for regulator and MCU minimal placement patterns.

### Acceptance Criteria

- Grouped components are closer than ungrouped placement would put them.
- Required max-spread failures are reported.
- Placement remains deterministic.

### Suggested Commit

`Add group-aware placement`

## Phase 6: Footprint Geometry Extraction

### Goals

Derive placement bounds from known footprint data instead of hand-supplied
dimensions.

### Tasks

1. Add conversion from `libraryresolver.FootprintRecord` to placement bounds.
2. Prefer courtyard geometry when present.
3. Fall back to pad extents.
4. Fall back to generated pad specs when using transactions/design API payloads.
5. Report estimated bounds with warnings.
6. Add tests using small synthetic footprint records and existing resolver
   parser fixtures.

### Acceptance Criteria

- Footprint records produce stable bounds.
- Missing geometry produces a clear blocking issue unless explicit estimated
  bounds are allowed.
- Bounds source is reported.

### Suggested Commit

`Extract footprint placement bounds`

## Phase 7: Transaction Output

### Goals

Convert placements into existing transaction operations.

### Tasks

1. Add `Operations(result Result) []transactions.Operation` or include
   operations in `Result`.
2. Emit `place_footprint` operations with:
   - `ref`;
   - `footprint_id`;
   - `at`;
   - `rotation_deg`;
   - `layer`.
3. Preserve fixed placements in output only when explicitly requested.
4. Add tests that apply placement operations through `transactions.Apply`.
5. Validate resulting PCB with existing PCB writer validation.

### Acceptance Criteria

- Placement output applies through the transaction pipeline.
- Generated PCB has placed footprints at expected deterministic coordinates.
- No direct writer bypass exists in the placement package.

### Suggested Commit

`Emit placement transactions`

## Phase 8: Block And Design API Integration

### Goals

Use circuit block PCB hints and design API state as placement inputs.

### Tasks

1. Map block `PCBHints` into placement components, groups, and constraints.
2. Add helper constructors for common block outputs.
3. Add design API adapter where practical:
   - symbols;
   - assigned footprints;
   - existing board outline;
   - existing fixed footprints.
4. Add tests for:
   - LED block placement;
   - regulator block placement;
   - connector breakout placement.

### Acceptance Criteria

- Existing block requests can produce placement requests.
- Placement result is usable by block project generation.
- Missing hints fall back to deterministic placement with warnings.

### Suggested Commit

`Connect placement to circuit blocks`

## Phase 9: CLI Skeleton

### Goals

Expose placement through the CLI once package APIs are stable.

### Tasks

1. Add `kicadai --json place request <request.json>`.
2. Add `kicadai --json place project <project-or-transaction>` only if project
   input mapping is reliable.
3. Return `reports.Result`.
4. Support flags:
   - `--request`;
   - `--output`;
   - `--overwrite`;
   - `--seed`;
   - library resolver roots/cache.
5. Add CLI tests with deterministic JSON output.

### Acceptance Criteria

- CLI can place a small request and return operations.
- CLI validates bad requests with structured errors.
- No KiCad installation is required for normal tests.

### Suggested Commit

`Expose placement command skeleton`

## Phase 10: Validation Loop

### Goals

Use existing validation infrastructure to prove placement output is meaningful.

### Tasks

1. Add placement internal validation reports to `evaluate` where applicable.
2. Add examples under `examples/placement`.
3. Run generated placement through:
   - transaction validation;
   - PCB writer validation;
   - inspect/evaluate;
   - ERC/DRC checks where stable.
4. Add documentation explaining:
   - placement-ready;
   - routing-ready;
   - fabrication-ready.

### Acceptance Criteria

- Example placement output writes a KiCad project.
- Internal validation passes.
- DRC smoke is documented as pass, fail, or currently blocked by known KiCad CLI
  behavior.

### Suggested Commit

`Validate placement examples`

## Phase 11: Optimization Improvements

### Goals

Improve quality without making placement nondeterministic.

### Tasks

1. Add multiple deterministic candidate strategies.
2. Compare candidates by HPWL and group spread.
3. Add optional seed-based stable variation.
4. Keep output reproducible for the same seed.
5. Add benchmark tests for moderate component counts.

### Acceptance Criteria

- Quality metrics improve for representative fixtures.
- Runtime remains bounded for small generated boards.
- Same seed produces same result.

### Suggested Commit

`Improve placement quality scoring`

## Final Validation Checklist

Run before declaring the placement engine initial implementation complete:

```sh
GOCACHE="$(mktemp -d)" go test ./...
kicadai --json inspect project ./examples/placement/basic
kicadai --json evaluate project ./examples/placement/basic
```

If KiCad CLI is available:

```sh
kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  check project ./examples/placement/basic
```

## Known Risks

- Footprint bounds from pads alone underestimate large mechanical parts.
- Edge connector orientation can vary by footprint convention.
- Decoupling placement requires pin-to-pad mapping to be truly meaningful.
- Arbitrary board outlines need stronger PCB outline modeling.
- Placement quality can look plausible while still being difficult to route.

## First Cut Recommendation

Start with Phases 1-3 as the first implementation batch. That creates a useful
engine for simple boards and gives later phases a stable foundation.

