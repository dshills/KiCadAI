# Code Review Fix Plan - 2026-06-11

Source review: `specs/CODE_REVIEW_06_11_2026.md`

Goal: close every code-review finding with focused implementation phases, regression tests, and verification. The order below prioritizes validation correctness because those gaps can allow invalid KiCad projects to be generated while local tests remain green.

## Phase 1 - Design-Wide Symbol Library Validation

Finding addressed: `P1 - Child schematic symbols bypass library-reference validation`

### Scope

Extend symbol-library validation from root-only schematic validation to all schematic files in a `design.Design`: root plus every child sheet file.

### Implementation Steps

1. Refactor `validateSymbolLibraryReferences` in `internal/kicadfiles/design/design.go` to iterate over a list of schematic files with stable field prefixes.
2. For each schematic file, build an embedded-symbol set from that file's `LibSymbols`.
3. Keep project-level `SymbolTables` and `KnownSymbolLibraries` as shared resolution sources across all files.
4. Report errors with precise prefixes:
   - `schematic.symbols[N].library_id` for root symbols.
   - `sheet_files[I].symbols[N].library_id` for child sheet symbols.
5. Preserve existing behavior for embedded symbols in the root schematic.

### Tests

Add tests in `internal/kicadfiles/design/design_test.go` covering:

- unresolved symbol library in a child schematic fails validation;
- child schematic embedded `LibSymbols` satisfy child symbol references;
- project-level `KnownSymbolLibraries` and `SymbolTables` still satisfy child references;
- root schematic behavior is unchanged.

### Acceptance Criteria

- `design.Validate` catches unresolved symbol libraries anywhere in the schematic hierarchy.
- Error paths identify the specific child sheet index and symbol index.
- Existing LED/demo design tests still pass.

## Phase 2 - Complete UUID Uniqueness Validation

Finding addressed: `P1 - UUID uniqueness validation misses nested KiCad objects`

### Scope

Extend `validateUniqueUUIDs` so the design-level UUID uniqueness contract covers every modeled UUID-bearing KiCad object, not only selected top-level objects.

### Implementation Steps

1. Introduce helper functions in `internal/kicadfiles/design/design.go`:
   - `addSchematicUUIDs(seen, file, prefix)`
   - `addPCBUUIDs(seen, board)`
   - small helpers for nested footprint and drawing UUIDs.
2. Add schematic nested UUIDs:
   - symbol pins;
   - no-connects;
   - buses;
   - bus entries;
   - polylines;
   - texts;
   - raw schematic items;
   - sheet pins.
3. Add PCB nested UUIDs:
   - footprint properties;
   - footprint texts;
   - pads;
   - footprint graphics;
   - drawing objects are already included at top level, but keep this path explicit;
   - dimensions are already included at top level.
4. Preserve the existing error format where possible, while adding enough path detail to locate the duplicate.
5. Do not attempt to parse raw S-expression bodies for additional nested UUIDs in this phase unless already modeled as fields.

### Tests

Add regression tests in `internal/kicadfiles/design/design_test.go` covering duplicate UUIDs for:

- two pads in one footprint;
- a footprint pad and a footprint property;
- two symbol pins;
- root schematic item and child schematic item;
- raw schematic item duplicated with a modeled schematic item.

### Acceptance Criteria

- Any duplicate modeled UUID anywhere in the generated design fails `design.Validate`.
- Tests prove nested duplicates that previously passed now fail.
- Error messages point to both current and prior UUID locations.

## Phase 3 - Hierarchical Reference Validation

Finding addressed: `P1 - Hierarchical schematic reference validation is incomplete without a PCB`

### Scope

Replace root-only schematic reference uniqueness with design-wide reference validation across root and child schematics.

### Design Decision

Default behavior should reject duplicate non-power schematic references across the entire design, including child sheets. This matches the current writer's flat PCB association model and prevents ambiguous footprint mapping. If sheet-local reference reuse is needed later, it should be introduced explicitly with hierarchical path-aware references.

### Implementation Steps

1. Replace `validateSchematicReferences(*schematic.SchematicFile)` with `validateDesignSchematicReferences(design Design)`.
2. Iterate root schematic and all `SheetFiles`.
3. Skip symbols whose trimmed reference starts with `#`, preserving existing power-symbol behavior.
4. Normalize references consistently with PCB association helpers:
   - use exact trimmed reference for user-facing duplicate messages;
   - use `referenceKey` for collision detection so path/space/case normalization conflicts are caught consistently with footprint matching.
5. Include file prefixes in errors:
   - `schematic.symbols[N].reference`;
   - `sheet_files[I].symbols[N].reference`.

### Tests

Add tests covering:

- duplicate reference in root schematic still fails;
- duplicate reference across root and child fails;
- duplicate reference across two child sheets fails;
- power references starting with `#` remain ignored;
- normalized collisions such as `U/1` and `U\1` fail.

### Acceptance Criteria

- Schematic-only hierarchical designs cannot pass with ambiguous references.
- PCB-backed designs keep existing behavior, with better coverage for excluded/non-footprint symbols.

## Phase 4 - High-Level API Pad Consistency

Finding addressed: `P2 - High-level PlaceFootprint accepts incomplete or non-symbol pad specs`

### Scope

Make `designapi.Builder.PlaceFootprint` enforce a safe mapping between schematic pins and PCB pads when callers provide explicit pad geometry.

### Implementation Steps

1. Add pad-spec validation before constructing the footprint:
   - trim pad names;
   - reject empty names;
   - reject duplicate pad names;
   - reject pad names that are not present in `symbolState.pins`.
2. Require every connected schematic pin in `state.pinNets` to have a pad.
3. Preserve current default behavior where omitted `options.Pads` generates one pad per symbol pin.
4. Add an explicit escape hatch only if needed after implementation review:
   - candidate option: `AllowMissingPads bool`;
   - default must remain strict.
5. Ensure net inheritance from `state.pinNets` continues to work for explicit pads.

### Tests

Add tests in `internal/kicadfiles/designapi/builder_test.go` covering:

- explicit pad with unknown pin name fails;
- duplicate explicit pad names fail;
- missing connected pin pad fails;
- missing unconnected pin pad behavior is decided and tested;
- default pads still generate a complete pad set;
- custom explicit pads still inherit connected nets.

### Acceptance Criteria

- The high-level builder cannot generate a PCB footprint missing a connected schematic pin by accident.
- Existing design API tests still pass or are updated to the stricter behavior intentionally.
- `kicaddesign.Validate(builder.Design())` remains green for valid builder flows.

## Phase 5 - PCB Round-Trip Artifact Path Contract

Finding addressed: `P2 - PCB round-trip results can return paths to deleted artifact files`

### Scope

Make `RoundTripPCB` artifact-path behavior consistent with cleanup behavior and with `RoundTripSchematic`.

### Contract

When `Options.KeepArtifacts` is false, returned artifact paths must be empty. When artifact paths are returned, those files must exist after the function returns.

### Implementation Steps

1. Remove the unconditional `compareOpts.KeepArtifacts = true` in `internal/kicadfiles/roundtrip/pcb.go`.
2. Set `compareOpts.ArtifactDir = workspace.Root` as today.
3. Write `summary.txt` only when `opts.KeepArtifacts` is true.
4. Ensure `RawDiffPath`, `NormalizedDiffPath`, and `SummaryPath` remain empty when artifacts are not kept.
5. Keep PCB and schematic round-trip behavior aligned.

### Tests

Add tests in `internal/kicadfiles/roundtrip/pcb_test.go` or `artifacts_test.go` covering:

- `CompareFiles` path behavior with `KeepArtifacts=false`;
- `RoundTripPCB`-level behavior if a fake KiCad CLI helper already exists or can be added safely;
- returned artifact paths are empty when cleanup will remove the workspace;
- returned artifact paths exist when `KeepArtifacts=true`.

### Acceptance Criteria

- No returned path points to a deleted workspace.
- Existing round-trip tests pass.
- The behavior is documented in `internal/kicadfiles/roundtrip/README.md` if the README currently describes artifacts.

## Phase 6 - Final Verification and Review

### Required Commands

Run after all fixes:

```sh
GOCACHE=/tmp/kicadai-gocache go list ./...
GOCACHE=/tmp/kicadai-gocache go test ./...
GOCACHE=/tmp/kicadai-gocache go vet ./...
GOCACHE=/tmp/kicadai-gocache go test -cover ./...
```

If KiCad CLI is available, also run the gated round-trip suite:

```sh
KICADAI_RUN_KICAD_CLI=1 GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/roundtrip ./internal/kicadfiles/pcb
```

### Review Gate

Run Prism on staged changes before committing:

```sh
prism review staged
```

Address high and actionable medium findings. Document intentionally deferred low-risk findings in the commit summary or follow-up issue/spec.

### Commit Plan

Use one commit per phase unless two adjacent phases are mechanically coupled:

1. `Validate child schematic libraries`
2. `Validate nested KiCad UUID uniqueness`
3. `Validate hierarchical schematic references`
4. `Harden design API pad placement`
5. `Fix round-trip artifact path reporting`

## Done Definition

The review findings are complete when:

- every finding in `specs/CODE_REVIEW_06_11_2026.md` has a regression test;
- all new tests fail against the pre-fix behavior and pass after the fix;
- the full Go test and vet suite passes;
- Prism has no unresolved high or actionable medium findings in the staged diff;
- the original review file can be annotated or superseded with the fixing commits.
