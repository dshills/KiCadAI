# Fabrication Profile Expansion Specification

## Objective

Make fabrication readiness more useful by replacing the single generic profile
view with validated, deterministic manufacturer-style profile support.

KiCadAI already emits conservative fabrication readiness evidence, generated
Gerber/drill artifacts, BOM/CPL reports, physical-rule checks, and a built-in
`generic_assembly` profile. The next gap is breadth and specificity: agents
need to understand which physical constraints came from the active fabrication
profile, which checks are unsupported by that profile, and how to load local
board-house rule snapshots without hardcoding every policy in code.

This project adds a profile registry and local profile import path that can
drive physical-rule thresholds, assembly metadata requirements, edge-plating
policy, and readiness summaries without claiming live manufacturer acceptance.

## Current Foundation

Existing implementation already includes:

- `export fabrication`, `export preview`, `export bom`, and CPL/BOM output;
- generated package manifests and readiness reports;
- Gerber/drill generation and artifact validation;
- physical fabrication checks for:
  - copper feature width;
  - solder mask web estimates;
  - annular rings;
  - courtyard presence/overlap;
  - silkscreen board clearance;
  - mounting-hole geometry/edge clearance;
  - board stackup basics;
  - net-class fabrication dimensions;
  - edge-plating/castellation policy;
  - differential-pair and impedance evidence gaps;
  - fabrication metadata;
- manufacturer-profile summary evidence;
- built-in generic assembly profile support;
- local procurement snapshot support through `--source-dir`.

## Problem

The current profile support is enough to keep fabrication claims conservative,
but it is not yet good enough for AI agents to reason about real board-house
constraints.

Gaps:

- only a narrow generic profile is modeled;
- profile fields are not clearly versioned as a public local-data contract;
- profile validation does not yet provide detailed field-level diagnostics for
  board-house-like presets;
- users cannot maintain a directory of trusted local profile snapshots;
- physical-rule reports do not always distinguish profile thresholds from
  parser evidence and unsupported geometry;
- readiness output does not clearly explain which profile requirements were
  enforced, skipped, unsupported, or not modeled;
- profile provenance is not rich enough for future AI rationale reports.

## Goals

- Define a stable fabrication profile schema for local, version-controlled
  profile snapshots.
- Support multiple built-in presets:
  - `generic_assembly`;
  - `generic_2layer_economy`;
  - `generic_2layer_standard`;
  - `generic_4layer_standard`;
  - `generic_castellated_review`.
- Add a profile registry that can load:
  - built-in profiles;
  - local JSON profiles from a directory supplied by CLI or environment;
  - profile aliases without changing downstream rule code.
- Validate profiles with structured issues for:
  - unsupported schema;
  - missing ID/name/version;
  - invalid units;
  - negative or zero thresholds;
  - inconsistent stackup/copper-layer policy;
  - impossible annular-ring or drill settings;
  - unknown feature flags.
- Feed profile constraints into physical rules consistently.
- Report profile provenance in fabrication readiness, package manifests, and
  physical-rule evidence.
- Keep all tests hermetic. No network, no live manufacturer lookup.

## Non-Goals

- Live supplier/manufacturer API integration.
- Real-time price, lead-time, stock, panel, or quoting data.
- Upload-ready package submission.
- Legal/manufacturer acceptance claims.
- Solver-grade impedance calculation.
- Exact polygonal DFM for all copper, mask, paste, and milling geometries.
- Replacing KiCad DRC.

## Profile Schema

Profiles are JSON documents loaded from built-ins or local files.

`source.kind` values are:

- `builtin` for profiles shipped with KiCadAI;
- `local` for trusted JSON snapshots loaded from a user-configured directory
  or validated from a path.

Version-one profiles use `units: "mm"` only. Non-mm profiles must fail
validation until an explicit unit-conversion policy exists.
Numeric field names intentionally retain `_mm` suffixes in v1 even though a
global `units` marker exists. The suffix makes generated reports and code
bindings self-describing; future non-mm support should add an explicit schema
version rather than reinterpret existing `_mm` fields.

Required top-level fields:

```json
{
  "schema": "kicadai.fabrication.profile.v1",
  "id": "generic_2layer_standard",
  "name": "Generic 2-layer Standard",
  "version": "2026-06",
  "source": {
    "kind": "builtin",
    "url": "",
    "retrieved_at": ""
  },
  "units": "mm",
  "stackup": {},
  "copper": {},
  "drill": {},
  "solder_mask": {},
  "silkscreen": {},
  "assembly": {},
  "edge_plating": {},
  "impedance": {},
  "metadata": {}
}
```

### Stackup

Fields:

- `min_layers`
- `max_layers`
- `allowed_layer_counts`
- `min_board_thickness_mm`
- `max_board_thickness_mm`
- `default_board_thickness_mm`
- `requires_two_outer_copper_layers`
- `requires_internal_planes_for_impedance`

Validation must reject contradictory stackup constraints. Every
`allowed_layer_counts` value must be positive and fall inside the inclusive
`min_layers`/`max_layers` range when those bounds are set.

### Copper

Fields:

- `min_trace_width_mm`
- `min_spacing_mm`
- `min_zone_min_thickness_mm`
- `min_copper_to_edge_mm`
- `min_copper_sliver_mm`
- `allow_neckdown`
- `min_neckdown_width_mm`
- `max_neckdown_length_mm`

### Drill And Annular Ring

Fields:

- `min_drill_mm`
- `min_via_drill_mm`
- `min_finished_hole_mm`
- `min_plated_slot_width_mm`
- `min_pad_annular_ring_mm`
- `min_via_annular_ring_mm`
- `min_hole_to_edge_mm`
- `allow_microvias`
- `allow_blind_buried_vias`

### Solder Mask And Paste

Fields:

- `min_solder_mask_web_mm`
- `default_mask_expansion_mm`
- `min_mask_clearance_mm`
- `paste_required_for_smd`
- `allow_missing_paste_on_smd`

`paste_required_for_smd` is the default rule for SMD assembly pads.
`allow_missing_paste_on_smd` is an escape hatch for profile families that allow
hand-soldered or paste-exempt SMD pads. If both are true, the permissive
`allow_missing_paste_on_smd` behavior wins and missing paste should warn or
skip according to the active check policy rather than block.

### Silkscreen And Assembly

Fields:

- `min_silkscreen_line_width_mm`
- `min_silkscreen_text_height_mm`
- `min_silkscreen_to_mask_mm`
- `require_courtyards`
- `require_reference_designators`
- `require_bom_identity`
- `require_cpl_for_assembled_parts`

### Edge Plating And Castellations

Fields:

- `allow_castellations`
- `allow_edge_plating`
- `min_castellation_drill_mm`
- `min_castellation_pitch_mm`
- `requires_edge_plating_notes`
- `requires_manual_review`

### Impedance And Differential Pairs

Fields:

- `allow_impedance_claims_without_solver`
- `require_stackup_for_impedance`
- `require_diff_pair_width_gap_evidence`
- `require_diff_pair_skew_evidence`

### Metadata Requirements

Fields:

- `require_board_finish`
- `allowed_board_finishes`
- `require_panelization`
- `require_fabrication_notes`
- `require_assembly_notes`
- `warning_only_fields`

`warning_only_fields` is an array of strings. Each string is a property path
within this profile schema, such as `edge_plating` or
`copper.min_trace_width_mm`; empty values, duplicates, and unknown paths must
be rejected.

## CLI Surface

Existing flag:

```sh
kicadai --manufacturer-profile generic_assembly export fabrication ./project
```

Add:

```sh
kicadai --manufacturer-profile generic_2layer_standard export fabrication ./project
kicadai --manufacturer-profile-dir ./profiles --manufacturer-profile my_board_house export fabrication ./project
kicadai fabrication profile list
kicadai fabrication profile show generic_2layer_standard
kicadai fabrication profile validate ./profiles/my_board_house.json
```

Environment:

```text
KICADAI_FABRICATION_PROFILE_DIR=/path/to/profiles
```

The CLI must default to existing behavior when no profile directory is
configured.

## Readiness And Reporting

Fabrication readiness should include:

- active profile ID, name, version, and source;
- profile validation status;
- enforced profile thresholds;
- unsupported profile fields;
- skipped profile checks and why they were skipped;
- physical-rule checks grouped by profile category;
- warning/blocking counts by category;
- profile-derived suggestions.

Physical-rule report checks should include:

- `source: "profile"` when the threshold came from the active profile;
- `profile` or `profile_id`;
- measurements with units;
- threshold values;
- parser limitations where exact geometry is not available.

Package manifests should include profile provenance without embedding huge
profile payloads:

- profile ID;
- version;
- source kind/path;
- profile hash;
- validation status.

## Safety Rules

- Local profile paths must stay inside the configured profile directory.
- Profile IDs must be stable ASCII identifiers.
- Unknown schema versions block profile loading.
- Invalid profile fields block the profile, not necessarily the whole export
  unless the requested profile is required for readiness.
- Built-in profile IDs cannot be shadowed by local profiles unless an explicit
  `--allow-profile-shadow` flag is added in a future project.
- No profile may downgrade hard writer, board-validation, ERC/DRC, or artifact
  failures into readiness.

## Testing Requirements

Default tests must be hermetic and deterministic.

Required coverage:

- profile schema validation;
- built-in profile registry;
- local profile directory loading;
- duplicate/alias behavior;
- CLI list/show/validate JSON;
- physical-rule threshold application for at least:
  - copper width;
  - annular ring;
  - solder mask web;
  - edge plating;
  - fabrication metadata;
- readiness report profile provenance;
- package manifest profile hash/provenance;
- invalid profile blocks readiness with actionable issue paths.

## Acceptance Criteria

- Users can select from multiple built-in fabrication profiles.
- Users can load local profile snapshots without network access.
- Invalid profiles produce structured diagnostics.
- Physical-rule checks show profile-derived thresholds.
- Fabrication readiness and package manifests record active profile provenance.
- Generated boards cannot be marked fabrication-ready by selecting a looser
  profile unless all modeled required evidence still passes.
