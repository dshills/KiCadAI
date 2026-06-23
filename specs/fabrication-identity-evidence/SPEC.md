# Fabrication Identity Evidence Specification

## Objective

Strengthen fabrication package confidence by making BOM/CPL output,
component identity, and manufacturer profile evidence explicit, validated, and
AI-readable.

The current fabrication workflow can preview/export package artifacts, generate
BOM and CPL CSVs, validate Gerber/drill outputs, and report readiness. The next
gap is not file generation. The gap is proving that every assembled part row and
placement row is tied to a trustworthy component identity and, where requested,
to manufacturer-specific release expectations.

Target flow:

```text
generated KiCad project
  -> schematic component properties
  -> PCB footprint placements
  -> component identity extraction
  -> BOM/CPL cross-check
  -> manufacturer profile validation
  -> fabrication readiness evidence
  -> package manifest and AI-facing issues
```

This project does not guarantee manufacturer acceptance. It adds deterministic
evidence gates so KiCadAI can explain whether a package is missing exact part,
assembly, or manufacturer-profile data before claiming fabrication readiness.

## Current Foundation

The repository already has:

- `internal/fabrication` readiness models, package manifests, artifact models,
  and `export preview|bom|fabrication` commands.
- Deterministic BOM rows with reference grouping, value, symbol ID, footprint
  ID, component ID, manufacturer, MPN, confidence, and readiness notes.
- Deterministic CPL rows with reference, footprint, coordinates, rotation,
  layer, placement source, and fixed state.
- Fabrication readiness summaries for BOM, CPL, Gerber, drill, ERC, DRC,
  writer correctness, board validation, component readiness, and block
  readiness.
- Component catalog records with manufacturer, MPN, lifecycle, package,
  ratings, confidence, acceptance levels, and pinmap evidence.
- Design workflow component-selection output with component ID, manufacturer,
  MPN, confidence, pinmap evidence, companion metadata, and rejected
  alternatives.
- Fabrication output validation for generated Gerber/drill evidence.

Current gaps:

- BOM and CPL are generated independently and are not cross-checked as an
  assembly pair.
- Component identity is row-level text, not a reusable evidence object with
  source, confidence, and blocking status.
- Missing manufacturer or MPN data is currently warning-level in BOM generation,
  even when fabrication-candidate workflows should require exact parts for
  active or assembly-critical components.
- CPL rows do not carry component identity, manufacturer profile data,
  side/rotation normalization evidence, or BOM linkage evidence.
- Manufacturer profile constraints are not modeled. There is no way to ask for
  a JLCPCB-like, PCBWay-like, or generic assembly profile gate.
- The package manifest does not summarize identity completeness, BOM/CPL
  consistency, assembly-side coverage, rotation policy, or manufacturer-profile
  compatibility.

## Scope

### In Scope

- Add a component identity evidence model for fabrication packages.
- Extract identity evidence from:
  - schematic symbol properties;
  - component catalog IDs and selected component metadata where available;
  - generated workflow provenance where available;
  - footprint and PCB references for CPL linkage.
- Strengthen BOM rows with:
  - identity status;
  - identity source;
  - component kind/class when known;
  - exact manufacturer and MPN requirement status;
  - lifecycle/rating evidence summary when available;
  - package/footprint compatibility status;
  - readiness severity.
- Strengthen CPL rows with:
  - component ID or BOM identity key;
  - manufacturer and MPN when known;
  - package/footprint;
  - normalized side;
  - normalized rotation;
  - placement provenance;
  - BOM linkage status.
- Add BOM/CPL consistency validation:
  - every CPL reference should resolve to a BOM reference unless excluded;
  - every on-board BOM reference should resolve to a CPL placement unless it is
    explicitly non-placeable or excluded;
  - DNP, `in_bom`, and `on_board` semantics must be consistent;
  - duplicate references must be blocking;
  - mismatched footprints between BOM and CPL must be blocking for assembly;
  - missing side/rotation/coordinate evidence must block assembly readiness.
- Add manufacturer profile model:
  - profile ID and display name;
  - required BOM columns;
  - required CPL columns;
  - accepted side names;
  - rotation convention notes;
  - package/footprint support policy;
  - exact manufacturer/MPN requirement policy;
  - allowed missing fields by component class;
  - severity mapping for warnings vs blockers.
- Include at least one built-in `generic_assembly` profile.
- Support profile selection through fabrication options and CLI flags.
- Integrate identity/profile evidence into:
  - `fabrication.Result.Summary`;
  - `fabrication.Manifest`;
  - BOM/CPL CSV/JSON data structures;
  - readiness scoring and blocking issues;
  - `design create` fabrication-candidate preview.
- Keep default tests hermetic:
  - no network sourcing lookup;
  - no live manufacturer API;
  - no real KiCad required.
- Document the new evidence and caveats.

### Out Of Scope

- Live Octopart/DigiKey/Mouser/LCSC/JLCPCB availability lookup.
- Automatic part substitution.
- Manufacturer upload automation.
- Panelization.
- Full DFM checks such as annular ring, solder sliver, impedance, creepage,
  paste aperture tuning, or assembly stencil rules.
- Guaranteeing rotation conventions for every manufacturer footprint library.
- Changing schematic or PCB writers beyond adding/read-propagating existing
  properties needed for identity evidence.
- Natural-language intent planning.

## Evidence Model

### Component Identity

Add a reusable identity object, for example:

```go
type ComponentIdentity struct {
    Reference         string
    ComponentID       string
    Value             string
    SymbolID          string
    FootprintID       string
    Manufacturer      string
    MPN               string
    Package           string
    ComponentClass    string
    Lifecycle         string
    Confidence        string
    Source            string
    ExactPartRequired bool
    ExactPartPresent  bool
    Issues            []reports.Issue
}
```

Expected sources:

- `schematic_property`: symbol properties such as `Component ID`,
  `Manufacturer`, `MPN`, `Footprint`, `Value`;
- `catalog_selection`: selected component catalog evidence;
- `pcb_footprint`: PCB footprint reference, library ID, position, side;
- `inferred`: only for non-fabrication convenience, never sufficient for
  fabrication readiness;
- `missing`: no usable evidence.

### Identity Status

Identity status should be one of:

- `pass`: exact identity evidence satisfies requested profile;
- `warning`: usable but incomplete identity evidence for non-critical contexts;
- `missing`: required identity data is absent;
- `conflict`: schematic, catalog, and PCB evidence disagree;
- `skipped`: component is DNP, not in BOM, or not on board;
- `fail`: invalid or unsafe identity evidence.

### Exact Part Requirements

Fabrication-candidate workflows and manufacturer profiles should require exact
manufacturer/MPN for:

- active ICs;
- polarized components;
- connectors;
- electromechanical parts;
- crystals/oscillators;
- protection devices;
- any component selected from a concrete catalog record;
- any part with package-specific pinout or orientation risk.

Generic passives may be profile-configurable:

- `allow_generic_passives`: resistor/capacitor/inductor rows may pass with
  value, tolerance/rating/package evidence but no exact MPN;
- `require_all_mpn`: every BOM row requires manufacturer and MPN;
- `warn_generic_passives`: generic passives are allowed but produce warnings.

## BOM Requirements

BOM row output should remain deterministic and grouped, but each row should
also carry identity evidence.

Required structured fields:

- references;
- quantity;
- value;
- symbol ID;
- footprint ID;
- component ID;
- manufacturer;
- MPN;
- package;
- component class;
- lifecycle;
- confidence;
- identity status;
- identity source;
- readiness note;
- issue count/blocking count.

CSV output may remain compact, but JSON/report objects should retain the full
identity evidence. If CSV columns are expanded, ordering must be stable and
tests must lock the header.

## CPL Requirements

CPL rows should prove that assembly placement data can be reconciled with the
BOM.

Required structured fields:

- reference;
- footprint;
- BOM identity key or component ID;
- manufacturer and MPN when known;
- side;
- x/y coordinates in millimeters;
- rotation in normalized degrees;
- raw rotation if different;
- placement source;
- fixed/locked state;
- BOM linkage status;
- profile compatibility status;
- readiness note.

Side normalization:

- KiCad `F.Cu` should map to `top` by default;
- KiCad `B.Cu` should map to `bottom` by default;
- unsupported or unknown layers should block assembly readiness.

Rotation normalization:

- use a deterministic default convention;
- keep raw KiCad rotation and normalized rotation when they differ;
- manufacturer profiles may define convention notes, but this milestone should
  not claim universal rotation correctness.

## BOM/CPL Consistency Rules

Validation should produce structured issues with reference attribution.

Blocking issues:

- duplicate BOM reference;
- duplicate CPL reference;
- on-board BOM reference missing from CPL;
- CPL reference missing from BOM;
- footprint mismatch between BOM and CPL evidence;
- missing placement coordinate;
- unknown placement side;
- exact part required but manufacturer or MPN missing;
- conflicting manufacturer or MPN between evidence sources;
- component selected as fabrication candidate with insufficient confidence.

Warnings:

- generic passive allowed by profile;
- lifecycle unknown for non-critical passive;
- fixed placement present but not problematic;
- rotation convention cannot be independently verified.

Skipped:

- DNP component;
- `in_bom=false`;
- `on_board=false`;
- virtual/non-placeable symbols.

## Manufacturer Profiles

Add a first built-in profile:

```json
{
  "id": "generic_assembly",
  "display_name": "Generic Assembly",
  "require_cpl": true,
  "require_bom": true,
  "require_exact_mpn": "active_and_polarized",
  "allow_generic_passives": true,
  "accepted_sides": ["top", "bottom"],
  "rotation_convention": "kicad_default_recorded",
  "required_bom_columns": ["References", "Quantity", "Value", "FootprintID"],
  "required_cpl_columns": ["Reference", "X", "Y", "Rotation", "Side"]
}
```

Profile support should be local and deterministic. Profiles should be defined
in Go or checked-in JSON test fixtures, not fetched from the network.

Future manufacturer-specific profiles can tighten:

- side naming;
- column naming;
- rotation convention;
- allowed packages;
- exact MPN requirements;
- lifecycle and availability requirements;
- preferred ordering-code fields.

## Readiness Integration

Fabrication readiness should gain identity-specific evidence, either as new
summary fields or a nested identity summary in the result/manifest.

Suggested summary fields:

- `component_identity`;
- `bom_cpl_consistency`;
- `manufacturer_profile`;
- `assembly_readiness`.

Status rules:

- missing BOM or CPL remains blocking for fabrication package readiness;
- missing exact identity blocks `ready` when required by profile;
- BOM/CPL inconsistency blocks `ready`;
- profile warnings keep status at `candidate`;
- profile blockers make status `blocked`;
- `ready` requires all modeled identity and assembly gates to pass in addition
  to existing writer, board, DRC, Gerber, and drill gates.

## CLI Behavior

### Existing Commands

`export preview`, `export bom`, and `export fabrication` should include identity
evidence in JSON results.

### New Flags

Add profile selection where fabrication options are parsed:

```text
--manufacturer-profile generic_assembly
```

Default:

- `generic_assembly` for fabrication package/readiness commands;
- no external manufacturer claim.

If an unknown profile is requested, return a blocking argument issue.

### AI-Facing Output

JSON output should let an agent answer:

- Which references lack exact manufacturer/MPN?
- Which references are in BOM but not CPL?
- Which references are in CPL but not BOM?
- Which rows are allowed generic passives?
- Which profile gate blocked fabrication readiness?
- Which fields need to be added to the schematic/component catalog?

## Testing Strategy

Default tests:

- identity extraction from schematic symbol properties;
- identity extraction from component catalog-style properties;
- BOM grouping by identity key;
- CPL side and rotation normalization;
- BOM/CPL cross-reference validation;
- exact MPN policy by component class;
- generic passive policy under `generic_assembly`;
- profile validation for unknown profile;
- readiness summary and manifest fields;
- CLI selected-field goldens for `export bom` and `export fabrication`.

Fixtures:

- all identity present and BOM/CPL consistent;
- missing MPN on active part blocks;
- generic passive allowed with warning or pass depending on profile;
- CPL missing BOM reference blocks;
- BOM missing CPL placement blocks;
- footprint mismatch blocks;
- unknown side blocks;
- DNP and not-on-board parts skip correctly.

Optional tests:

- none required for real KiCad or network access in this milestone.

## Documentation Requirements

Update:

- README fabrication export section;
- README limitations;
- `specs/ROADMAP.md`;
- any CLI examples that show fabrication package JSON.

Docs must say:

- this does not contact manufacturer APIs;
- `generic_assembly` is a conservative local profile, not a board-house
  guarantee;
- exact part identity is required for assembly-critical parts;
- generic passives are only allowed when the selected profile permits them;
- BOM/CPL consistency is required for fabrication readiness.

## Risks

### Overclaiming Manufacturer Compatibility

Risk: A local profile is interpreted as board-house approval.

Mitigation:

- call the first profile `generic_assembly`;
- avoid real manufacturer names unless rules are validated;
- include caveats in README and result summaries.

### Incomplete Component Classification

Risk: Components without class metadata are treated too leniently.

Mitigation:

- unknown class defaults to requiring exact identity for fabrication-candidate
  readiness unless profile explicitly allows it;
- issue messages should explain how to add class metadata.

### BOM/CPL False Positives

Risk: Virtual symbols, DNP parts, and not-on-board parts create noisy blockers.

Mitigation:

- honor KiCad and writer metadata for DNP, `in_bom`, and `on_board`;
- include skip reasons in evidence.

### Rotation Overconfidence

Risk: Rotation normalization is treated as guaranteed manufacturer convention.

Mitigation:

- record raw and normalized rotations;
- mark rotation convention as recorded, not externally verified;
- keep manufacturer-specific rotation proof out of this milestone.

## Acceptance Criteria

- Fabrication reports include component identity evidence for BOM rows.
- CPL rows include BOM linkage and normalized side/rotation evidence.
- BOM/CPL consistency validation produces structured issues.
- `generic_assembly` profile gates exact part identity for assembly-critical
  parts.
- Fabrication readiness and package manifests include identity/profile summary
  evidence.
- CLI JSON exposes blocked references and profile gate failures.
- Default tests remain hermetic and pass without KiCad or network access.
- README and roadmap identify the next remaining fabrication and autonomy gaps.
