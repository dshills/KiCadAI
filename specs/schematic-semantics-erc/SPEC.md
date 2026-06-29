# Schematic Semantics And ERC Closeout Specification

Date: 2026-06-29

## Purpose

KiCadAI can write modern KiCad schematics, route schematic wires orthogonally,
and produce readable generated diagrams. The next blocker is ERC-level semantic
correctness: generated schematics must be interpreted by KiCad as real
connected symbols, not only as visually plausible drawings.

This project closes the gap between KiCadAI's internal schematic model and
KiCad ERC expectations for generated projects. It is not a broad schematic
feature expansion. The first goal is to make small generated designs pass
structural schematic validation and produce meaningful KiCad ERC evidence when
the design intent is electrically complete.

## Current Problem

The KiCad-backed LED smoke fixture now writes and validates PCB connectivity
correctly, but KiCad ERC still reports:

- pin-not-connected findings near origin;
- dangling wire findings;
- unconnected wire endpoint warnings.

The generated schematic contains symbol instances and wire geometry, but its
KiCad-facing symbol semantics are incomplete for ERC. The root causes to
investigate and fix are:

- generated schematics may have empty or insufficient `(lib_symbols)` sections;
- generated symbol instances may not align with actual library pin geometry;
- KiCad may be resolving fallback or unknown symbol bodies, producing ERC
  locations near origin;
- wire endpoints may be based on internal synthetic pin offsets rather than
  resolver-backed library pin anchors;
- power, no-connect, and intentionally external ports may be represented as
  generic block endpoints instead of KiCad-recognized ERC constructs.

## Goals

- Generated schematic symbols have KiCad-resolvable semantics for supported
  libraries.
- Wires terminate on KiCad/ERC-recognized pin anchors.
- Supported two-pin passive/LED/power cases produce no false ERC pin/wire
  findings.
- Missing symbol, pin, power, and no-connect evidence is surfaced before KiCad
  ERC as structured workflow issues.
- KiCad-backed design promotion reports distinguish true schematic electrical
  problems from writer semantic defects.

## Non-Goals

- Full support for every KiCad symbol library construct.
- Editing arbitrary imported schematics.
- Full hierarchical-sheet synthesis.
- Analog correctness proof beyond basic ERC semantics.
- Treating every KiCad ERC warning as auto-repairable.

## Scope

### Symbol Semantics

For generated schematics, KiCadAI must support ERC-meaningful symbol metadata
for the seed design set:

- `Device:R`
- `Device:C`
- `Device:LED`
- `Device:D`
- common power symbols used by generated blocks, such as `power:VCC`,
  `power:+3V3`, `power:GND`, and `power:PWR_FLAG` where applicable
- op-amp and connector symbols already used by checked-in examples where
  feasible through resolver-backed metadata

The supported path should prefer resolver-backed KiCad library data when a
symbol index is configured. For built-in generated workflows without a resolver,
KiCadAI may use a small verified embedded-symbol template set for seed symbols,
but those templates must be explicit, tested, and marked as verified fixtures.

### Pin Anchor Semantics

Schematic wire generation must use the same pin anchors KiCad ERC uses. The
writer must not assume internal synthetic pin offsets are valid when a resolver
or embedded symbol template provides authoritative pin geometry.

For each generated symbol:

- pin number must exist in the symbol definition;
- unit/body selection must be known for the emitted instance;
- hidden pins must be explicit in the model;
- pin electrical type must be preserved where known;
- pin anchor position must include symbol rotation and unit/body orientation;
- wire endpoints must land exactly on the chosen anchor.

### Embedded Library Bodies

Generated schematics must embed library symbol bodies when required for
KiCad/ERC stability. Embedded bodies must be deterministic and KiCad 10
compatible.

The writer should avoid embedding incomplete placeholder bodies. If no
resolver-backed or verified embedded body is available, it should emit a
structured warning or blocker depending on requested validation level.

### Power And No-Connect Policy

Generated designs need explicit semantics for pins that are not connected by
signal wires:

- known power input pins must be tied to a generated power net or marked as an
  unresolved power requirement;
- power output/source symbols must be represented with KiCad-recognized power
  symbol semantics;
- intentionally unused pins must get no-connect markers;
- external ports that are intentionally open must be represented as labels,
  connectors, power symbols, or no-connects according to block intent;
- floating inputs or incomplete block ports must block ERC-clean promotion.

### Schematic Validation

Add schematic semantic validation before KiCad ERC. It should report:

- missing symbol body;
- missing pin definition;
- mismatched synthetic vs resolver pin anchors;
- wire endpoint not on a symbol pin;
- duplicate reference in the same sheet scope;
- missing no-connect marker for intentionally unused known pin;
- power net without a power source when known;
- generated external endpoint that has no KiCad schematic representation.

### Workflow Evidence

The design workflow should expose a compact schematic semantics summary:

- symbol count checked;
- embedded/resolver-backed symbol count;
- unknown symbol count;
- pin anchor count;
- wire endpoint contact count;
- no-connect count;
- power net count;
- ERC readiness: `unknown`, `blocked`, `candidate`, or `clean`.

Promotion reports should include this evidence and use it to explain whether
KiCad ERC failures are likely generated-writer defects or real electrical
design issues.

## User-Facing Behavior

For `kicadai design create --require-erc`, the workflow should:

- fail before KiCad ERC when schematic semantics are structurally incomplete;
- run KiCad ERC only when required schematic semantic evidence is present;
- include ERC findings as structured issues with repair categories;
- keep generated artifacts for diagnosis when `--keep-artifacts` is set.

For non-required ERC workflows, schematic semantic issues may be warnings unless
they imply invalid KiCad files.

## Acceptance Criteria

- The LED KiCad-backed smoke fixture no longer reports false origin-based
  `pin_not_connected` or `wire_dangling` ERC findings caused by writer
  semantics.
- Generated schematic files for supported seed symbols include ERC-usable
  symbol definitions or resolver-backed evidence.
- Wire endpoint validation proves every generated wire endpoint touches a
  known symbol pin, junction, label, no-connect, or intentional port endpoint.
- A generated incomplete schematic still fails with clear local schematic
  semantic issues rather than confusing KiCad ERC locations.
- `go test ./...` passes.
- Optional KiCad-backed tests can run with `KICADAI_KICAD_CLI` and preserve
  diagnostic artifacts without polluting the repository.

## Risks

- KiCad symbol library semantics are broad; the initial implementation must
  stay constrained to verified seed symbols.
- Embedded symbol templates can drift from KiCad upstream. Tests should compare
  against KiCad-saved fixtures where practical.
- Treating power symbols incorrectly can hide real ERC problems. Unknown power
  behavior should fail closed at ERC-required acceptance levels.
- Over-eager no-connect insertion can mask design mistakes. No-connects must
  come from explicit block intent, not automatic silence.
