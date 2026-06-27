# Component Symbol Properties Implementation Plan

Date: 2026-06-27

## Phase 1: Add Property Constants And Merge Helpers

Goal: define the KiCadAI-owned identity property contract in one place.

Tasks:

- Add a small package or internal helper for component identity property names.
- Define constants for:
  - `KiCadAI Component ID`;
  - `KiCadAI Variant ID`;
  - `KiCadAI Component Role`;
  - `KiCadAI Block ID`;
  - `Manufacturer`;
  - `MPN`;
  - `Component Class`;
  - `Component Confidence`;
  - `Component Source`;
  - `Lifecycle Status`;
  - `Availability Status`;
  - `Pinmap ID`.
- Add a merge helper that:
  - appends hidden identity properties after KiCad-required properties;
  - keeps unrelated user-authored custom properties in their existing relative
    order and appends newly generated KiCadAI-owned properties at the end;
  - replaces existing KiCadAI-owned identity properties;
  - removes existing KiCadAI-owned properties when the new selected evidence
    omits or clears that value, preventing stale identity metadata;
  - preserves unrelated custom properties;
  - returns structured diagnostics for conflicts, duplicate property
    canonicalization, and stale-property removal so callers can distinguish
    non-blocking updates from ownership conflicts;
  - is idempotent.
- Add unit tests for merge ordering, replacement, conflict reporting, and
  empty-value omission.

Review:

- `go test ./internal/...` for the affected package.
- `prism review staged`.

Commit:

- `Add component symbol property helpers`

## Phase 2: Extend Transaction Add-Symbol Payload Support

Goal: ensure generated transaction operations can carry schematic symbol
properties cleanly.

Tasks:

- Inspect the typed `AddSymbolOperation` payload.
- Add or confirm a `properties` field that maps cleanly to schematic writer
  properties.
- Validate property payloads:
  - non-empty names;
  - string values are preserved through the schematic writer's formal
    S-expression encoder, not string concatenation, including quotes,
    parentheses, and newlines;
  - deterministic ordering;
  - duplicate handling consistent with the schematic writer.
- Ensure transaction apply writes properties to schematic symbols with stable
  name-based ordering and merging.
- Reuse the shared component identity property merge helper in both workflow
  selection propagation and transaction apply paths.
- Add tests for:
  - add-symbol operation with hidden properties;
  - duplicate or invalid property diagnostics;
  - deterministic output across repeated apply.

Review:

- `go test ./internal/transactions ./internal/kicadfiles/schematic`.
- `prism review staged`.

Commit:

- `Support schematic properties in symbol transactions`

## Phase 3: Thread Component Selections Into Symbol Operations

Goal: populate identity properties from selected workflow components before
project writing.

Tasks:

- Extend `ApplyComponentSelectionsToPlan` or the operation update helper to
  merge identity properties into matching `add_symbol` operations.
- Include selected evidence:
  - component ID;
  - variant ID;
  - role;
  - block/instance ID where available;
  - manufacturer;
  - MPN;
  - confidence;
  - lifecycle and availability status where source evidence is present;
  - pinmap ID where known.
- Emit warning issues when generated identity properties are replaced with
  different selected values.
- Preserve existing footprint assignment behavior.
- Add workflow tests proving generated transaction operations carry identity
  properties after selection.

Review:

- `go test ./internal/designworkflow`.
- `prism review staged`.

Commit:

- `Propagate selected component properties to schematics`

## Phase 4: Verify Generated Schematics And Round-Trip Preservation

Goal: prove written KiCad schematic files contain and preserve the properties.

Tasks:

- Add generated-project fixtures or tests that run `design create` for a
  catalog-backed design.
- Parse the resulting `.kicad_sch` and assert expected properties on selected
  symbols.
- Add schematic round-trip tests for hidden identity properties.
- Ensure writer correctness does not flag the properties as unstable or
  unsupported.
- Add golden coverage where the project already uses CLI goldens for generated
  workflow output.

Review:

- `go test ./cmd/kicadai ./internal/kicadfiles/schematic ./internal/kicadfiles/roundtrip ./internal/writercorrectness`.
- `prism review staged`.

Commit:

- `Verify component properties in generated schematics`

## Phase 5: Integrate Fabrication And BOM Identity Reads

Goal: make downstream fabrication evidence able to consume schematic identity
properties without weakening gates.

Tasks:

- Update fabrication identity extraction to read:
  - `KiCadAI Component ID`;
  - `KiCadAI Variant ID`;
  - `Manufacturer`;
  - `MPN`;
  - `Component Confidence`;
  - `Component Class`.
- Prefer schematic property evidence when internally consistent and validated
  against `KiCadAI Component ID` plus workflow/catalog evidence where available.
- Fall back to existing workflow/catalog evidence where properties are absent.
- Emit structured issues for conflicts:
  - unknown component ID;
  - manufacturer mismatch;
  - MPN mismatch;
  - footprint mismatch where checkable.
- Add tests for property-first identity extraction and conflict cases.

Review:

- `go test ./internal/fabrication ./cmd/kicadai`.
- `prism review staged`.

Commit:

- `Use schematic component properties for fabrication identity`

## Phase 6: Update Documentation And Examples

Goal: document the property contract for users and AI agents.

Tasks:

- Update README or focused docs to explain that generated schematics now carry
  component identity properties.
- Update `docs/kicadai-agent-skill.md` with guidance:
  - inspect schematic properties before claiming part identity;
  - compare schematic properties against `.kicadai/workflow-result.json`;
  - treat conflicts as blockers for imported projects.
- Add a short example showing:
  - `design create`;
  - `inspect schematic`;
  - `export bom`.
- Update `specs/ROADMAP.md` Priority 1 current/remaining work.

Review:

- `go test ./...`.
- `prism review staged`.

Commit:

- `Document component symbol properties`

## Phase 7: Compatibility Sweep

Goal: ensure the new properties do not destabilize existing generated projects.

Tasks:

- Run full test suite:
  - `go test ./...`
- Check for docs examples that still mention stale `--json` requirements.
- Inspect generated schematic snippets for hidden property ordering.
- Confirm working tree is clean after commits.

Review:

- `prism review staged` only if additional changes are needed.

Commit:

- No commit if the sweep produces no changes.

## Acceptance Checklist

- Generated schematics include selected component identity properties.
- Metadata properties are hidden and deterministic.
- Generic selections do not fabricate manufacturer/MPN values.
- Workflow evidence and schematic properties agree for generated projects.
- Fabrication/BOM identity readers can consume schematic properties.
- Conflicts are reported as structured issues.
- Full `go test ./...` passes.
