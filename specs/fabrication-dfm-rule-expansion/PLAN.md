# Fabrication DFM Rule Expansion Implementation Plan

Date: 2026-06-29

This plan implements `specs/fabrication-dfm-rule-expansion/SPEC.md`.

Each phase should be implemented, reviewed with Prism, tested, and committed
before moving to the next phase.

## Phase 1: Taxonomy, Profile Thresholds, And Report Shape

### Goal

Extend the existing physical-rule model with new DFM categories and policy
thresholds without changing readiness behavior yet.

### Work

- Add physical-rule categories:
  - `annular_ring`;
  - `copper_sliver`;
  - `edge_plating`;
  - `impedance`;
  - `differential_pair`;
  - `fabrication_metadata`.
- Add stable check IDs for annular ring, copper width, mask web, edge plating,
  impedance evidence, differential-pair evidence, board finish, panelization,
  and fabrication notes.
- Extend physical/manufacturer profile options with optional thresholds:
  - minimum plated pad annular ring;
  - minimum via annular ring;
  - minimum copper feature width;
  - minimum solder-mask web;
  - castellation/edge-plating policy;
  - board-finish and fabrication-note policy;
  - controlled-impedance and panelization policy.
- Add default threshold helpers and profile normalization.
- Update deterministic JSON/report tests for category summaries.

### Tests

- New categories serialize deterministically.
- New check IDs map to correct default issue paths.
- Default profile thresholds are stable.
- Profile overrides preserve existing behavior when unset.

### Acceptance

- The model can represent all planned checks.
- Existing physical-rule reports remain backward-compatible.

### Commit

```text
Extend fabrication DFM rule taxonomy
```

## Phase 2: Annular Ring Checks

### Goal

Block impossible or under-sized plated pad/via annular rings using parsed PCB
geometry.

### Work

- Inspect the current PCB pad and via model for:
  - through-hole pad type;
  - plated versus NPTH evidence;
  - drill diameter;
  - pad size;
  - via diameter and drill.
- Add annular-ring evaluation helpers:
  - plated pad ring;
  - via ring;
  - missing geometry warnings.
- Emit measurements for outer diameter, drill diameter, and annular ring.
- Add profile threshold handling for pad and via rings.
- Keep NPTH/mechanical holes out of plated annular-ring blockers.

### Tests

- Passing plated through-hole pad.
- Pad drill greater than or equal to pad diameter blocks.
- Pad annular ring below threshold blocks.
- Passing via.
- Via annular ring below threshold blocks.
- Missing geometry on likely plated objects warns.
- NPTH holes are skipped or handled by mounting-hole rules, not annular-ring
  blockers.

### Acceptance

- Physical-rule reports expose deterministic annular-ring evidence.
- Fabrication readiness blocks impossible drilled copper geometry.

### Commit

```text
Add annular ring fabrication checks
```

## Phase 3: Copper Sliver And Narrow Copper Checks

### Goal

Flag explicitly modeled copper features below profile minimums and warn about
unsupported copper geometry.

### Work

- Reuse parsed track, arc, via, zone, and copper graphic data where available.
- Add copper feature-width checks for:
  - track segments;
  - arcs if parsed with width;
  - generated copper graphics if represented;
  - zones with minimum thickness/minimum width evidence where parsed.
- Emit affected net, layer, object, and width measurements.
- Warn for unsupported copper polygon/zone geometry when it prevents a sliver
  conclusion.
- Avoid claiming full polygon sliver safety.

### Tests

- Track width above threshold passes.
- Track width below threshold blocks.
- Zone minimum width below threshold blocks or warns according to available
  data.
- Unsupported copper geometry warns.
- No copper evidence skips cleanly.

### Acceptance

- Below-minimum generated copper is visible before export readiness claims.
- Unsupported geometry is explicit rather than silently passing.

### Commit

```text
Add copper sliver fabrication checks
```

## Phase 4: Solder-Mask Web Checks

### Goal

Detect simple same-side solder-mask web risks using conservative pad bounding
box evidence.

### Work

- Add pad-side and mask-exposure helpers.
- Compute conservative pad bounding boxes where pad shape/size is available.
- Estimate mask web between same-side exposed pads:

```text
estimated_mask_web = pad_to_pad_spacing - expansion_a - expansion_b
```

- Apply profile minimum solder-mask web threshold.
- Warn when pad geometry or mask expansion is unknown.
- Scope pairwise checks to nearby pads to avoid excessive runtime on large
  boards.

### Tests

- Same-side SMD pads with sufficient web pass.
- Same-side SMD pads below threshold block.
- Opposite-side pads do not create a false mask-web blocker.
- THT-only boards skip or pass without paste/mask-web false positives.
- Unknown pad geometry warns with stable object references.

### Acceptance

- Common dense-pad mask-web risks become deterministic physical-rule findings.

### Commit

```text
Add solder mask web fabrication checks
```

## Phase 5: Edge Plating And Castellation Policy

### Goal

Detect likely castellations/edge-plating features and report whether the active
profile permits them.

### Work

- Add edge proximity helpers using the current board outline/bounds model.
- Detect likely edge-plated features from:
  - plated pads intersecting or near Edge.Cuts;
  - footprint/library names that mention castellated pads;
  - KiCadAI metadata requesting edge plating, when present.
- Add profile policy:
  - allow;
  - warn;
  - block.
- Emit evidence references for footprint refs, pad UUIDs, and edge distance.
- Keep unknown edge-plating geometry as warning rather than pass.

### Tests

- No edge-plating features skip cleanly.
- Allowed castellation profile passes with evidence.
- Disallowed castellation profile blocks.
- Near-edge plated pad warns when intent/profile is unknown.
- Feature-name heuristic is deterministic.

### Acceptance

- Edge-plating/castellation risk is visible and profile-controlled.

### Commit

```text
Add edge plating fabrication policy checks
```

## Phase 6: Impedance And Differential-Pair Fabrication Evidence

### Goal

Connect existing high-speed/coupled-net intent to fabrication evidence without
pretending to solve impedance.

### Work

- Inspect existing net-class, coupled-net, differential-pair, and
  controlled-impedance intent models.
- Add checks that detect:
  - controlled-impedance intent without stackup/material evidence;
  - differential-pair intent without width/gap/length evidence;
  - routed coupled nets whose evidence is incomplete.
- Add profile policy for impedance:
  - `ignore`;
  - `warn`;
  - `block`.
- Emit explicit "solver not implemented" evidence where relevant.

### Tests

- No impedance intent skips.
- Controlled-impedance intent with no stackup warns or blocks by profile.
- Differential pair with declared width/gap evidence passes the modeled check.
- Missing pair evidence creates stable warnings/blockers.

### Acceptance

- High-speed fabrication claims become evidence-backed and conservative.

### Commit

```text
Add impedance fabrication evidence checks
```

## Phase 7: Fabrication Metadata Checks

### Goal

Represent board finish, panelization, and fabrication-note evidence in the
physical-rule report.

### Work

- Define the first metadata source:
  - KiCadAI-managed fabrication options if already available;
  - otherwise project text variables or `.kicadai` metadata as a follow-up
    boundary.
- Add checks for:
  - board finish present when profile requires it;
  - panelization declared when requested;
  - fabrication notes present when required by edge plating, impedance, or
    special processing.
- Add consistency checks:
  - edge plating detected but notes/profile forbid it;
  - impedance intent but no fabrication note;
  - panelized delivery requested but no panelization metadata.

### Tests

- Missing board finish warns or blocks by profile.
- Provided board finish passes.
- Edge-plating feature without fabrication note warns/blocks as configured.
- Panelization policy handles single-board default without false blockers.

### Acceptance

- Fabrication metadata gaps are visible in readiness evidence.

### Commit

```text
Add fabrication metadata rule checks
```

## Phase 8: Readiness, Export, Workflow, And Documentation Integration

### Goal

Expose the new DFM findings through existing readiness, export, design workflow,
and documentation surfaces.

### Work

- Ensure `EvaluateBoard` includes all new checks in deterministic order.
- Ensure `physical-rules.json` includes category summaries for new checks.
- Update fabrication readiness tests so blockers downgrade readiness.
- Update `design create` fabrication-candidate tests where physical-rule
  summaries are asserted.
- Update docs:
  - `docs/fabrication.md`;
  - README status if needed;
  - `specs/ROADMAP.md`.
- Add focused CLI/golden updates only if visible output shape changes.

### Tests

- `go test ./internal/fabrication/physicalrules`
- `go test ./internal/fabrication`
- `go test ./internal/designworkflow`
- `go test ./...`

### Acceptance

- New DFM checks affect readiness consistently.
- Exported package evidence remains deterministic.
- Users can see which checks are heuristic versus proof-backed.

### Commit

```text
Integrate expanded fabrication DFM evidence
```
