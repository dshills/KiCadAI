# Component Symbol Properties Specification

Date: 2026-06-27

## Summary

KiCadAI already selects catalog-backed components and records selected
manufacturer, MPN, confidence, lifecycle, footprint, pinmap, and companion
evidence in workflow artifacts. Generated schematics, however, do not yet carry
that selected identity on the schematic symbols themselves. That means a KiCad
user, a BOM extractor, or a later imported-project reader must rely on
sidecar `.kicadai` metadata instead of the design file.

This project threads selected component evidence into generated schematic
symbol properties. The goal is to make generated KiCad schematics self-
describing enough for AI review, BOM/fabrication checks, and future imported
project analysis while preserving deterministic writer output and existing
round-trip behavior.

## Goals

- Add stable KiCad symbol properties for selected component identity.
- Preserve existing KiCad-required symbol properties and property ordering.
- Populate component properties from workflow component-selection results.
- Make fabrication/BOM identity readers prefer schematic properties when
  available and fall back to existing workflow/catalog evidence when not.
- Surface conflicts between schematic identity properties and selected
  workflow evidence as structured issues.
- Keep default behavior deterministic and hermetic: no network, no live KiCad
  requirement, and no distributor API.

## Non-Goals

- Do not add live sourcing, pricing, lifecycle, or stock lookup.
- Do not replace `.kicadai` workflow artifacts; schematic properties complement
  those artifacts.
- Do not mutate imported user projects in this milestone.
- Do not infer manufacturer/MPN values for generic or placeholder records.
- Do not claim that a schematic property alone proves fabrication readiness.
- Do not add arbitrary KiCad property UI positioning logic beyond stable,
  hidden metadata fields.

## Current Foundation

Relevant existing behavior:

- Schematic symbols already support arbitrary `Properties`.
- The schematic writer derives required KiCad properties in stable order:
  `Reference`, `Value`, `Footprint`, `Datasheet`, and `Description`.
- Extra symbol properties are preserved after the required properties.
- `ApplyComponentSelectionsToPlan` already maps workflow component-selection
  entries back to generated schematic references.
- Component-selection entries already carry:
  - component ID;
  - variant ID;
  - manufacturer;
  - MPN;
  - confidence;
  - footprint ID;
  - procurement evidence;
  - companions;
  - rejected-candidate evidence.
- Fabrication identity evidence and BOM/CPL reports already model component
  identity but do not yet rely on generated schematic properties as a primary
  source.

## Property Contract

Generated schematic symbols should use explicit, stable property names:

- `KiCadAI Component ID`
- `KiCadAI Variant ID`
- `KiCadAI Component Role`
- `KiCadAI Block ID`
- `Manufacturer`
- `MPN`
- `Component Class`
- `Component Confidence`
- `Component Source`
- `Lifecycle Status`
- `Availability Status`
- `Pinmap ID`

`KiCadAI Component ID` is the primary identity anchor. When it is present,
`Manufacturer`, `MPN`, footprint, class, lifecycle, availability, and pinmap
properties must be consistent with the catalog or workflow evidence for that
component ID. Conflicts must be reported rather than silently resolved.

`Manufacturer` and `MPN` intentionally use common KiCad/BOM property names for
tool compatibility. KiCadAI may replace those fields only in generated-project
flows where transaction provenance proves KiCadAI owns the symbol operation and
`KiCadAI Component ID` is present. A manually added `KiCadAI Component ID` is
not ownership proof by itself. Imported-project readers must treat existing
non-prefixed `Manufacturer` and `MPN` values as user-authored unless generated
transaction provenance proves ownership; if those values conflict with the
component ID, imported-project readers must block instead of overwriting.
Generated-project rewrites may replace KiCadAI-owned fields only while applying
the current generated transaction. If a later workflow detects a manual edit
that differs from both the previous workflow evidence and the new selected
evidence, it must treat that value as a user override and use `preserve_block`
until the user explicitly accepts regeneration.

Ownership proof is sidecar-based, not inferred from the schematic file alone.
The required proof is:

- `.kicadai/transaction.json` or equivalent generated-project provenance maps
  the schematic symbol to a KiCadAI-generated `add_symbol` operation by symbol
  UUID. Initial generation must assign or capture symbol UUIDs before the first
  successful write. Reference/operation path fallback is allowed only for
  legacy generated projects that predate UUID capture and have not been
  re-annotated; if re-annotation happens without UUID preservation, the
  ownership chain is broken and the symbol must become `read_only_unproven`;
- `.kicadai/workflow-result.json` or equivalent workflow evidence maps that
  reference, role, or block instance to the same `KiCadAI Component ID`;
- the current operation is running in a generated-project rewrite path, not an
  imported-project preservation path.

If any of those checks is absent, non-prefixed `Manufacturer` and `MPN` are
read-only evidence and must not be clobbered. This is intentionally
fail-closed: losing `.kicadai` sidecar files disables automated mutation of
generic BOM fields, but it does not prevent read-only inspection, BOM export, or
conflict reporting. An in-band origin marker may be added in a future
preservation milestone, but it is not required for this generated-project
property propagation milestone.

Non-prefixed fields in this contract are compatibility mirrors for KiCad/BOM
tools, not independent ownership markers. KiCadAI-specific ownership and
selection identity must be anchored by the prefixed `KiCadAI Component ID`
property plus sidecar provenance.

Ownership state machine:

- `generated_owned`: sidecar provenance exists, the schematic reference maps to
  a generated `add_symbol` operation, and workflow evidence maps the same
  reference/role/block instance to the same component ID. Generated workflows
  may replace KiCadAI-owned fields and emit warnings for changed values.
- `generated_user_override`: sidecar provenance exists, but the current
  schematic value differs from both the previous workflow evidence stored by
  the last successful write in `.kicadai/workflow-result.json` or equivalent
  provenance and the new selected value. The workflow must use
  `preserve_block` until the user accepts regeneration.
- `read_only_unproven`: sidecar provenance is missing, incomplete, stale, or
  does not match the schematic reference/component ID. Schematic properties may
  be read for BOM/export evidence, but non-prefixed fields must not be mutated.
- `imported_preserve`: imported projects without generated ownership proof must
  preserve user-authored properties and report conflicts as blocking issues.

Required minimum for any catalog-selected symbol:

- `KiCadAI Component ID`
- `KiCadAI Component Role`
- `Component Confidence`

Additional properties should be emitted only when values are known and
non-empty:

- `KiCadAI Variant ID`
- `Manufacturer`
- `MPN`
- `Lifecycle Status`
- `Availability Status`
- `Pinmap ID`

`Lifecycle Status` and `Availability Status` are optional snapshot evidence.
Emit them only when local catalog/source evidence supplied those values; do not
invent them and do not imply live distributor status.

`Component Confidence` uses the catalog confidence enum string, such as
`verified`, `library_derived`, `rule_inferred`, `placeholder`, or `blocked`.
`Lifecycle Status` uses local source enum values such as `active`, `mature`,
`nrnd`, `eol`, `obsolete`, or `unknown`. `Availability Status` uses local
source enum values such as `in_stock`, `limited`, `backorder`, `unavailable`,
`unknown`, or `not_checked`.

`Component Source` should describe the evidence source at a high level, not a
live supplier claim. Initial allowed values:

- `catalog`
- `catalog+source_snapshot`
- `generic`
- `policy_allowed`

`Component Class` should use existing catalog/fabrication classification when
available. If no classification is modeled, omit the property instead of
guessing.

## Visibility And KiCad Behavior

KiCadAI-specific identity properties are metadata, so they should be hidden by
default in the schematic canvas. Standard BOM fields such as `Manufacturer` and
`MPN` may follow generated-project visibility policy or user configuration; they
must still be written as normal KiCad symbol properties so KiCad's property
editor, BOM tools, and readers can inspect them.

Defaults:

- `Hidden: true` for KiCadAI-specific metadata fields;
- `ShowName: false`
- `DoNotAutoplace: true` as a KiCad property attribute when supported by the
  schematic property format, not as a separate custom property key;
- position derived deterministically from the symbol reference/value property
  layout, using stable offsets so properties remain predictable if a user
  unhides them. The initial fallback offset is one 2.54 mm grid step per
  property slot after KiCad's derived `Reference`, `Value`, `Footprint`, and
  `Datasheet` rows, relative to the symbol position.

Required KiCad properties must remain first and must not be displaced by
identity properties. Existing explicit `Reference`, `Value`, `Footprint`,
`Datasheet`, and `Description` behavior must not regress.

## Workflow Integration

Component identity properties should be applied after component selection and
before transaction application/project writing.

For each selected component:

1. Map the selected role to a generated schematic reference using the existing
   `componentSelectionsByRef` logic.
2. Decode the matching `add_symbol` operation.
3. Merge identity properties into the operation payload.
4. Preserve existing symbol properties not owned by KiCadAI.
5. Replace existing KiCadAI-owned identity properties for the same names.
6. Keep the standard KiCad `Footprint` property synchronized with the selected
   footprint assignment through the existing footprint assignment operation.
7. Re-encode the operation with stable property ordering.

The merge must be idempotent. Running the workflow twice with the same
selection should not duplicate properties or produce unstable ordering.

## Transaction Model Requirements

The `add_symbol` operation must support optional symbol properties if it does
not already expose them in its typed payload.

Requirements:

- property names and values are strings;
- empty property names are rejected;
- duplicate property names in one transaction operation produce a non-blocking
  diagnostic and are de-duplicated with KiCad-compatible last-value-wins
  semantics before writing;
- identity properties are matched by canonical property name and emitted in
  deterministic order; KiCad symbol properties do not have their own UUIDs;
- property merge helpers are unit-testable without writing full projects.

## Conflict Detection

If a symbol already contains identity properties that conflict with the
selected workflow component, generated-project workflows may replace them
because KiCadAI owns the generated transaction. The workflow should still
record a warning issue when replacement happens.

For future imported-project support, the same conflict must become blocking
unless ownership is proven. This milestone should design the helper so callers
can choose one of two policies:

- `generated_replace`: replace KiCadAI identity properties and warn on
  mismatch;
- `preserve_block`: report a blocking conflict and do not mutate.

Initial implementation should use `generated_replace` only in `design create`
and `intent create` generated-project paths.

## Fabrication And BOM Interaction

Fabrication identity extraction should prefer schematic symbol properties when
they are present and internally consistent:

1. schematic property evidence;
2. workflow/component-selection evidence;
3. catalog lookup by component ID;
4. legacy BOM or footprint-derived fallback where already supported.

Preference does not mean blind trust. When `KiCadAI Component ID` is present,
fabrication checks must validate `Manufacturer`, `MPN`, footprint, class,
lifecycle, availability, and pinmap values against workflow/catalog evidence
where that evidence is available. Mismatches are conflicts, not last-writer-wins
overrides.

Conflicts should be explicit:

- schematic `Manufacturer` differs from selected/catalog manufacturer;
- schematic `MPN` differs from selected/catalog MPN;
- schematic `KiCadAI Component ID` cannot be found in the catalog;
- schematic component ID and footprint assignment are incompatible.

Fabrication readiness should not become more lenient. If a property is missing
or conflicting, existing conservative readiness behavior should remain.

## CLI And Artifact Behavior

No new top-level command is required.

Generated artifacts should expose property propagation through existing
outputs:

- `design create` project schematic contains identity properties;
- `.kicadai/workflow-result.json` still contains component-selection evidence;
- `writer check` preserves the properties;
- `inspect schematic` or `inspect project` should expose symbol properties if
  those commands already serialize symbol detail;
- `export bom` and `export fabrication` should include identity evidence from
  properties when available.

Example usage:

```sh
kicadai --request examples/design/led_indicator.json --output ./out/led --overwrite design create
kicadai inspect schematic ./out/led/led.kicad_sch
kicadai export bom ./out/led
```

JSON remains the default output format.

## Validation

Required tests:

- schematic writer preserves hidden custom properties in deterministic order;
- component-selection property merge is idempotent;
- selected concrete component emits component ID, manufacturer, MPN, role,
  confidence, and variant metadata where known;
- generic or policy-allowed component omits manufacturer/MPN when unknown;
- conflicting generated identity properties are replaced with a warning;
- `design create` generated schematic contains expected identity properties;
- BOM/fabrication identity extraction can read properties from a generated
  schematic;
- writer correctness and round-trip checks preserve identity properties.

## Success Criteria

- Generated schematics are self-describing for selected component identity.
- Existing generated projects continue to pass writer correctness, board
  validation, and full `go test ./...`.
- Fabrication/BOM output can use schematic properties as an identity source
  without weakening readiness gates.
- Property propagation is deterministic, idempotent, and covered by tests.

## Risks

### Property Name Drift

Risk: downstream tools expect different property names.

Mitigation: use explicit names, document them, and keep old `.kicadai`
artifacts as canonical workflow evidence.

### Visual Clutter

Risk: generated schematics become noisy if metadata properties are visible.

Mitigation: write identity properties hidden by default.

### Conflicting Evidence

Risk: schematic properties diverge from workflow/catalog evidence.

Mitigation: generated workflows replace KiCadAI-owned properties with
warnings; imported-project workflows block until ownership rules exist.

### Fabrication Overclaim

Risk: manufacturer/MPN properties are treated as proof of readiness.

Mitigation: readiness still depends on existing identity, consistency,
validation, and configured profile gates.
