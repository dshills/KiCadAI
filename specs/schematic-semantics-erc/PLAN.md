# Schematic Semantics And ERC Closeout Plan

Date: 2026-06-29

## Phase 1: Reproduce And Classify Current ERC Failures

Deliverables:

- Add or update a deterministic test fixture that captures the current LED
  KiCad-backed ERC failure shape without requiring KiCad by default.
- Record the generated schematic symbol, wire, and ERC finding signatures that
  distinguish writer-semantic failures from real electrical failures.
- Add helper code to classify KiCad ERC findings by location, object type, rule,
  and likely source stage.

Verification:

- `go test ./internal/designworkflow ./internal/kicadfiles/schematic ./internal/kicadfiles/checks`
- Optional `KICADAI_KICAD_CLI=... go test ./internal/designworkflow -run KiCadBacked`

## Phase 2: Add Verified Embedded Symbol Templates For Seed Symbols

Deliverables:

- Introduce a small verified embedded-symbol template registry for generated
  seed symbols, starting with `Device:R`, `Device:C`, `Device:LED`, common
  generated diode symbols, and generated power symbols.
- Connect `designapi.Builder.AddSymbol` or transaction apply to hydrate
  `SchematicFile.LibSymbols` for supported generated symbols when resolver
  data is absent.
- Ensure templates include KiCad 10 compatible pin electrical types, pin
  numbers, names, lengths, orientation, default properties, and embedded-font
  metadata where required.

Verification:

- Golden schematic writer tests assert non-empty deterministic `lib_symbols`.
- Read/write round-trip tests preserve embedded symbol bodies.
- Existing simple examples remain parseable.

## Phase 3: Make Pin Anchors Resolver/Template Backed

Deliverables:

- Resolve schematic pin anchors from symbol metadata instead of relying only on
  block-provided synthetic pin offsets when verified symbol metadata exists.
- Apply symbol rotation and unit/body orientation consistently.
- Add validation that each generated symbol pin exists in the resolved symbol
  body and that generated wire endpoints land on the resolved anchors.
- Preserve block-provided synthetic pin offsets only as explicit fallback
  evidence with warning or blocker severity depending on acceptance level.

Verification:

- Unit tests for two-pin horizontal, rotated, and power symbol anchors.
- Schematic connectivity tests prove wire endpoints match symbol pins.
- The LED smoke schematic no longer produces near-origin synthetic pin
  evidence in local validation.

## Phase 4: Add Schematic Semantic Validation Stage Evidence

Deliverables:

- Add a schematic semantics validator that checks generated symbols, pins,
  wire endpoints, no-connect markers, labels, duplicate refs, and power
  requirements.
- Integrate the validator into `design create` after schematic generation and
  before project write or before KiCad ERC, depending on current workflow
  structure.
- Emit compact summary evidence into workflow stages and promotion reports.
- Produce operation-correlated issues where possible.

Verification:

- Tests cover missing symbol body, missing pin, dangling wire endpoint,
  duplicate reference, and missing no-connect cases.
- Promotion report includes schematic semantic evidence and next-action text.

## Phase 5: Power And No-Connect Policy For Generated Blocks

Deliverables:

- Add explicit block-level intent for intentionally open pins, external ports,
  and power pins.
- Emit KiCad no-connect markers only for explicit unused-pin intent.
- Emit or require power symbols/PWR_FLAG semantics for generated power nets
  where ERC requires a driven source.
- Add block verification expectations for power/no-connect schematic evidence.

Verification:

- Tests cover LED, regulator, op-amp gain stage, and connector/open-port cases.
- False no-connect insertion is rejected by tests when a required block pin is
  accidentally omitted.

## Phase 6: KiCad-Backed Fixture Promotion

Deliverables:

- Rerun the LED KiCad-backed smoke fixture with local KiCad CLI.
- Update fixture metadata from `expected_fail` to `candidate` only when writer,
  schematic semantic validation, board validation, and required ERC evidence
  pass or have explicitly documented non-writer blockers.
- Keep DRC tool crashes or environment issues classified separately from ERC
  semantic defects.

Verification:

- `go test ./...`
- Optional KiCad-backed fixture run with `KICADAI_KICAD_CLI`.
- `kicadai design create` output includes clear schematic semantics and ERC
  evidence.

## Phase 7: Documentation And AI Guidance

Deliverables:

- Update README/docs to explain schematic semantic evidence and ERC-required
  behavior.
- Update the AI agent skill/docs to require checking schematic semantic
  blockers before claiming ERC readiness.
- Add troubleshooting notes for common ERC repair categories.

Verification:

- Documentation examples use `kicadai`, not `go run`.
- CLI JSON examples show the new schematic semantic summary shape.

## Commit Strategy

- Commit after each phase.
- Run Prism on staged changes before each commit.
- Keep generated promotion artifacts ignored and out of commits.

## Completion Criteria

This project is complete when supported generated seed schematics produce
KiCad/ERC-recognized symbols and pin-connected wires, schematic semantic
validation catches incomplete generated designs before KiCad does, and the LED
KiCad-backed smoke fixture can be promoted beyond writer-semantic failure.
