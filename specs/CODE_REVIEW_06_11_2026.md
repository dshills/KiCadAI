# Code Review - 2026-06-11

Scope: full repository review with emphasis on non-generated Go code, KiCad file writers, the round-trip validation harness, and the high-level design API. Generated protobuf code under `internal/kiapi/gen` and vendored KiCad proto/source snapshots were not reviewed line-by-line.

## Verification Run

- `GOCACHE=/tmp/kicadai-gocache go list ./...` passed.
- `GOCACHE=/tmp/kicadai-gocache go test ./...` passed.
- `GOCACHE=/tmp/kicadai-gocache go vet ./...` passed.
- `GOCACHE=/tmp/kicadai-gocache go test -cover ./...` passed.

Coverage highlights from the last run:

- `cmd/kicadai`: 71.8%
- `internal/kicadfiles/design`: 81.1%
- `internal/kicadfiles/designapi`: 76.3%
- `internal/kicadfiles/pcb`: 84.8%
- `internal/kicadfiles/roundtrip`: 64.1%
- `internal/kicadfiles/schematic`: 86.9%

## Findings

### P1 - Child schematic symbols bypass library-reference validation

Evidence:

- `internal/kicadfiles/design/design.go:129-138` calls `validateSymbolLibraryReferences(design)` once, then separately validates sheet files.
- `internal/kicadfiles/design/design.go:322-349` builds embedded/tabled/known symbol-library state from `design.Schematic` and iterates only `design.Schematic.Symbols`.
- `internal/kicadfiles/design/design.go:519-560` validates child sheet files structurally, but does not apply symbol-library reference checks to their symbols.

Impact:

Hierarchical projects can pass `design.Validate` even when a child sheet references an unresolved symbol library. That defeats one of the main guarantees of the project writer: generated projects may open with missing symbols even though validation passed.

Recommendation:

Make symbol-library validation walk the root schematic and every `SheetFiles` schematic. For each file, consider that file's embedded `LibSymbols`, the project tables, and known libraries. Add tests for unresolved symbols in child sheets and embedded child-sheet symbols.

### P1 - UUID uniqueness validation misses nested KiCad objects

Evidence:

- `internal/kicadfiles/design/design.go:441-515` checks UUID uniqueness for project/root/sheet files and selected top-level schematic/PCB objects only.
- PCB nested UUIDs are validated for shape but not uniqueness: footprint properties/texts/pads call validators such as `validateFootprintProperty`, `validateFootprintText`, and `validatePad` at `internal/kicadfiles/pcb/pcb.go:1891-1985` and `internal/kicadfiles/pcb/pcb.go:2040-2075`.
- Those nested validators verify `Valid()` UUIDs, but do not check for duplicate IDs across sibling or cross-file KiCad objects.

Impact:

A design can pass validation with duplicate pad, footprint property, footprint text, symbol pin, no-connect, bus, polyline, or raw-item UUIDs. KiCad treats UUIDs as object identity; duplicate IDs can corrupt editing behavior, round-trip preservation, or schematic/PCB association.

Recommendation:

Extend `validateUniqueUUIDs` to include every modeled UUID-bearing object:

- schematic: no-connects, buses, bus entries, polylines, texts, raw items, symbol pins, sheet pins, and any UUID-bearing embedded/raw modeled items.
- PCB: footprint properties, texts, pads, graphics, dimensions internals where applicable, zone filled polygons if UUIDs are later added, and preserved/raw modeled nodes when parseable.

Add regression tests with duplicate nested UUIDs that currently pass.

### P1 - Hierarchical schematic reference validation is incomplete without a PCB

Evidence:

- `internal/kicadfiles/design/design.go:136` calls `validateSchematicReferences(design.Schematic)`.
- `internal/kicadfiles/design/design.go:307-317` checks only the provided schematic file, with exact reference comparison.
- Sheet files are structurally validated by `validateSheetFiles`, but duplicate references across root/child sheets are only partly caught later by PCB-related validation paths (`validateFootprintReferences`) and only for symbols that require footprints.

Impact:

Schematic-only hierarchical designs can pass with duplicate references in child sheets. Even PCB-backed designs can miss duplicates for symbols excluded from board placement. This can produce ambiguous annotations and broken KiCad project state before PCB generation begins.

Recommendation:

Replace root-only reference validation with a design-level pass over root plus all child sheets. Use the same normalization as footprint matching where appropriate, and decide explicitly whether KiCad sheet-local reference reuse is supported. Add tests for duplicate references across two child sheets and root/child collisions.

### P2 - High-level `PlaceFootprint` accepts incomplete or non-symbol pad specs

Evidence:

- `internal/kicadfiles/designapi/builder.go:362-370` blindly converts caller-provided `PadSpec` entries into PCB pads.
- `internal/kicadfiles/designapi/builder.go:378-380` builds the pad index from whatever pad names were supplied.
- `internal/kicadfiles/designapi/builder.go:553-567` generates complete default pads from symbol pins, but that completeness guarantee is lost as soon as `options.Pads` is non-empty.
- `internal/kicadfiles/designapi/builder.go:570-614` validates only that the pad name is non-empty, not that it maps to a symbol pin or that connected symbol pins are represented.

Impact:

An agent can connect schematic pin `R1.2`, then call `PlaceFootprint("R1", Pads: []PadSpec{{Name: "1"}})`. The design can still carry the net in `ExpectedNets`, but the PCB footprint has no pad for pin 2. Local validation is likely to pass while the generated PCB is electrically meaningless or missing expected connections.

Recommendation:

For the high-level API, enforce symbol-to-pad consistency by default:

- reject pad names not present in `symbolState.pins`;
- reject duplicate pad specs before constructing the footprint;
- require every connected pin in `state.pinNets` to have a pad unless an explicit `AllowMissingPads`/`NoPCBPad` option is added;
- add tests for omitted connected pads and unknown pad names.

### P2 - PCB round-trip results can return paths to deleted artifact files

Evidence:

- `internal/kicadfiles/roundtrip/pcb.go:58-61` forces `compareOpts.KeepArtifacts = true`, causing `CompareFiles` to populate raw and normalized diff paths.
- `internal/kicadfiles/roundtrip/artifacts.go:41-44` deletes the workspace when the original `opts.KeepArtifacts` is false.
- By contrast, schematic round-trip sets only `ArtifactDir` at `internal/kicadfiles/roundtrip/schematic.go:60-62`, so its behavior differs.

Impact:

When callers do not request artifact retention, `RoundTripPCB` can return `RawDiffPath`, `NormalizedDiffPath`, or `SummaryPath` values that point inside a workspace already removed by deferred cleanup. Reports or tools consuming those paths can fail later with confusing "file not found" errors.

Recommendation:

Choose one contract and enforce it consistently:

- do not force `compareOpts.KeepArtifacts = true`; or
- keep the workspace whenever returning artifact paths; or
- clear artifact paths before returning when cleanup will delete them.

Add tests for `RoundTripPCB` with `KeepArtifacts=false` that assert returned paths are either empty or exist.

## Additional Notes

- The codebase is in good test health for deterministic unit tests, and `go vet` is clean.
- Round-trip and KiCad CLI-backed integration tests remain environment gated. That is appropriate, but the validation report should clearly distinguish "unit green" from "KiCad verified".
- Several clone implementations are necessarily manual today. They are well tested in `designapi`, but the broader codebase would benefit from a reusable clone strategy or explicit `Clone` methods on writer model types before many more fields are added.
