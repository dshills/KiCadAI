# Fabrication DFM Rule Expansion Specification

Date: 2026-06-29

## 1. Purpose

KiCadAI already emits fabrication readiness evidence and a physical-rule report
for stackup, net-class, solder mask/paste layer policy, Edge.Cuts containment,
courtyard presence/overlap, silkscreen clearance, and mounting-hole basics.
That foundation is useful, but it still misses several board-fabrication risks
that commonly turn a parseable PCB into a board-house question, DFM warning, or
fabrication rejection.

This project expands deterministic internal DFM checks so `export preview`,
`export fabrication`, and `design create` fabrication-candidate evidence can
surface additional physical risks before relying on a board house or KiCad DRC.

## 2. Goals

This project must add local, deterministic checks for:

- annular ring evidence on plated through-hole pads and vias;
- copper sliver and narrow copper geometry risk;
- solder-mask sliver and mask-web risk;
- castellated or edge-plated feature detection and policy reporting;
- controlled-impedance and differential-pair fabrication evidence gaps;
- panelization, board finish, and fabrication-note metadata evidence;
- manufacturer-profile thresholds for the checks that can be policy driven;
- report and readiness integration using the existing
  `physicalrules.Report` model;
- focused tests using synthetic PCB/project fixtures and existing generated
  examples where practical.

The result should make boards more honestly classifiable as `blocked`,
`candidate`, or `ready` without requiring network access or vendor-specific DFM
systems.

## 3. Non-Goals

This project does not:

- implement a complete computational geometry or CAM engine;
- certify acceptance by JLCPCB, PCBWay, OSH Park, or any other manufacturer;
- replace KiCad DRC;
- perform live vendor API checks;
- solve impedance from field-solver physics;
- add automatic panelization generation;
- mutate user-authored imported projects;
- guarantee fabrication readiness for boards whose source data is absent,
  unsupported, or ambiguous.

Unsupported geometry must produce structured `warning` or `blocked` evidence
instead of silent `pass` evidence.

## 4. Existing Foundation

Relevant current code:

- `internal/fabrication/physicalrules`: report model, taxonomy, normalization,
  and board/project evaluation;
- `internal/fabrication`: readiness evaluation, package export, manifest
  integration, and physical-rule artifact output;
- `internal/kicadfiles/pcb`: PCB parser/writer data model for footprints, pads,
  tracks, vias, zones, board setup, layers, and graphics;
- `internal/pcbrules`: net-class, trace, via, layer, clearance, length, zone,
  and coupled-net policy resolution;
- `internal/boardvalidation`: connectivity-first board evidence;
- `docs/fabrication.md`: current user-facing fabrication readiness behavior.

Existing physical-rule categories are:

- `stackup`
- `net_class`
- `solder_mask`
- `solder_paste`
- `edge_cuts`
- `courtyard`
- `silkscreen`
- `mounting_hole`

This project should extend that taxonomy conservatively rather than replacing
it.

## 5. New Rule Categories And Checks

### 5.1 Annular Ring

Add a physical-rule category:

```text
annular_ring
```

Add stable check IDs:

- `physical.annular_ring.plated_pad`
- `physical.annular_ring.via`
- `physical.annular_ring.profile_threshold`

The evaluator should inspect plated through-hole pads and generated vias where
outer diameter and drill diameter are available.

Required measurements:

- pad/via outer diameter in mm;
- drill diameter in mm;
- calculated annular ring in mm:

```text
annular_ring = (outer_diameter - drill_diameter) / 2
```

Expected behavior:

- block when drill diameter is greater than or equal to outer diameter;
- block when annular ring is below the active profile minimum;
- warn when required geometry is missing but the object is likely plated;
- pass when all plated pads/vias have sufficient ring evidence;
- skip when the board has no plated through-hole pads and no vias.

Default profile thresholds should be conservative and documented. A reasonable
initial default is:

- minimum plated pad annular ring: `0.15 mm`;
- minimum via annular ring: `0.10 mm`.

### 5.2 Copper Sliver And Narrow Copper Risk

Add a physical-rule category:

```text
copper_sliver
```

Add stable check IDs:

- `physical.copper_sliver.track_width`
- `physical.copper_sliver.zone_min_width`
- `physical.copper_sliver.unsupported_geometry`

The first implementation should avoid pretending to be a full polygon DFM
engine. It should model checks that are directly supported by parsed data:

- track/segment width below profile minimum;
- arc or graphical copper width below profile minimum where parsed;
- zone minimum thickness/minimum width below profile minimum where parsed;
- unsupported copper polygon geometry that cannot be analyzed.

Expected behavior:

- block for explicit generated copper narrower than the profile minimum;
- warn for unsupported copper shapes that could contain slivers;
- pass when all modeled copper widths meet the profile;
- skip when no copper width evidence is available.

Default threshold:

- minimum copper feature width: `0.127 mm` unless overridden by profile.

### 5.3 Solder Mask Sliver And Mask Web Risk

Add or extend category:

```text
solder_mask
```

Add stable check IDs:

- `physical.solder_mask.web_width`
- `physical.solder_mask.pad_expansion`
- `physical.solder_mask.unsupported_geometry`

The evaluator should detect simple mask-web risks where pad shapes and mask
expansion are known. The first pass can use conservative pad bounding boxes
rather than exact rounded-rectangle geometry.

Expected behavior:

- block when two exposed pads on the same side produce an estimated mask web
  below profile minimum;
- warn when pad geometry or mask expansion is unknown;
- pass when same-side pad spacing leaves sufficient estimated mask web;
- skip when the board has no SMD or exposed assembly pads.

Default threshold:

- minimum solder-mask web: `0.10 mm`.

The calculation should be documented as conservative bounding-box evidence:

```text
estimated_mask_web = pad_to_pad_spacing - (mask_expansion_a + mask_expansion_b)
```

### 5.4 Castellations And Edge Plating

Add a physical-rule category:

```text
edge_plating
```

Add stable check IDs:

- `physical.edge_plating.castellation_detected`
- `physical.edge_plating.profile_support`
- `physical.edge_plating.edge_contact`

The evaluator should identify likely castellated or edge-plated features using
conservative evidence:

- plated pads intersecting or very near the board outline;
- footprint names or pad properties that explicitly mention castellated pads;
- generated design metadata requesting castellations or edge plating.

Expected behavior:

- warn when likely castellations are detected without explicit profile support;
- block when the active manufacturer profile forbids castellations;
- warn when edge-plating evidence is incomplete or unsupported;
- pass only when requested/observed edge plating is allowed and the necessary
  evidence is present;
- skip when no edge-plating features are requested or detected.

This check must not require automatic generation of castellations.

### 5.5 Controlled Impedance And Differential-Pair Fabrication Evidence

Add or extend categories:

```text
impedance
differential_pair
```

Add stable check IDs:

- `physical.impedance.stackup_evidence`
- `physical.impedance.width_gap_evidence`
- `physical.differential_pair.fabrication_evidence`

These checks should connect existing rule intent to fabrication evidence:

- if a net class, routing policy, or design request declares controlled
  impedance, the board must provide stackup/material evidence or report a
  blocker/warning depending on acceptance mode;
- if a coupled/differential pair has explicit target impedance, width, gap, or
  length-match intent, those values must appear in physical-rule evidence;
- unsupported impedance solving should be explicit.

Expected behavior:

- block fabrication `ready` when controlled impedance is requested but stackup
  evidence is missing;
- warn for candidate workflows when impedance intent exists but solver-grade
  proof is unavailable;
- pass only for modeled evidence, not for guessed impedance.

This project should not implement an impedance solver.

### 5.6 Fabrication Metadata

Add a physical-rule category:

```text
fabrication_metadata
```

Add stable check IDs:

- `physical.fabrication_metadata.board_finish`
- `physical.fabrication_metadata.panelization`
- `physical.fabrication_metadata.fabrication_notes`

The evaluator should collect and validate project-level fabrication metadata
where KiCadAI models it:

- board finish, such as HASL, lead-free HASL, ENIG, immersion silver, or
  unspecified;
- panelization intent, such as single-board, array, mouse bites, V-score, or
  external;
- fabrication notes, such as impedance requirement, stackup note, controlled
  depth, castellations, edge plating, special solder mask, or assembly notes.

Expected behavior:

- warn when fabrication-candidate or ready export lacks board finish metadata;
- warn when panelization is unspecified for outputs that request panelized
  delivery;
- block when metadata contradicts detected features, such as castellations
  detected but notes/profile forbid edge plating;
- pass when metadata is explicitly provided and internally consistent.

Metadata may initially live in KiCadAI-managed project/fabrication options
rather than KiCad-native fields if no stable writer model exists yet.

## 6. Manufacturer Profile Extension

Extend the current physical/manufacturer profile model with optional thresholds:

- `min_plated_pad_annular_ring_mm`;
- `min_via_annular_ring_mm`;
- `min_copper_feature_mm`;
- `min_solder_mask_web_mm`;
- `allow_castellations`;
- `allow_edge_plating`;
- `require_board_finish`;
- `require_fabrication_notes`;
- `controlled_impedance_policy`: `ignore`, `warn`, or `block`;
- `panelization_policy`: `ignore`, `warn`, or `block`.

Default behavior must remain conservative but not overly disruptive:

- existing projects should not fail because optional metadata is missing unless
  the active profile or acceptance mode requires it;
- explicit impossible geometry should block regardless of profile;
- unsupported geometry should warn unless it hides a required proof.

## 7. Report Integration

The existing `physical-rules.json` report must include the new checks using the
same schema version unless the shape changes. If only new categories/check IDs
are added, keep:

```text
kicadai.fabrication.physical_rules.v1
```

Each check must include:

- stable ID;
- category;
- status;
- severity when issue-producing;
- message;
- suggestion;
- references and object UUIDs where available;
- affected nets where available;
- affected layers where available;
- measurements in mm;
- source (`parser`, `profile`, `heuristic`, `writer`, or `kicad_drc`).

Readiness summaries should continue exposing compact physical-rule evidence:

- aggregate status;
- blocker count;
- warning count;
- profile ID;
- report artifact path.

## 8. CLI And Workflow Behavior

Affected commands:

- `kicadai export preview`
- `kicadai export fabrication`
- `kicadai export bom`
- `kicadai design create` when fabrication-candidate acceptance is requested

Expected behavior:

- default JSON output remains stable and includes existing physical-rule
  summary locations;
- executed fabrication exports write updated `physical-rules.json`;
- dry-run previews evaluate checks but do not write package artifacts except
  where the current command already does;
- `design create` fabrication-candidate acceptance downgrades when blocking
  physical-rule findings appear;
- optional KiCad DRC evidence may be cross-referenced but does not replace
  internal check output.

## 9. Testing Strategy

Add tests at the lowest practical level first:

- `internal/fabrication/physicalrules` unit tests for each new category;
- profile parsing/default tests;
- report JSON determinism tests;
- readiness/export integration tests for physical-rule summaries;
- CLI golden updates only where output shape changes;
- generated example or synthetic board fixtures that fail exactly one new rule
  at a time.

Tests must not require KiCad CLI. Optional KiCad-backed smoke tests may be added
only behind the existing opt-in environment variables.

## 10. Acceptance Criteria

This project is complete when:

- annular ring checks block impossible or below-threshold plated pads/vias;
- copper sliver checks flag below-minimum generated copper widths;
- solder-mask web checks flag simple below-threshold pad spacing cases;
- castellated/edge-plating detection reports explicit allowed/forbidden/unknown
  evidence;
- controlled-impedance and differential-pair intent produces explicit
  fabrication evidence or warnings/blockers;
- fabrication metadata is represented in the report and profile policy;
- `physical-rules.json` includes deterministic new category summaries;
- fabrication readiness and `design create` acceptance consume the new report
  results;
- `go test ./...` passes without KiCad CLI.

## 11. Open Questions

- Which manufacturer profile should become the first stricter named profile
  beyond `generic_assembly`?
- Should board finish and fabrication notes live in `.kicadai` metadata first,
  or should KiCad project text variables be the source of truth?
- How strict should controlled-impedance blockers be for `candidate` versus
  `ready` acceptance?
- How much exact geometry should be implemented before relying on KiCad DRC for
  copper and mask sliver proof?
