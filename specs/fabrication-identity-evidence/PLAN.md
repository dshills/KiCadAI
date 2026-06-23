# Fabrication Identity Evidence Implementation Plan

## Objective

Implement deterministic component identity, BOM/CPL consistency, and local
manufacturer profile evidence for fabrication packages.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests hermetic: no network, no real manufacturer services, no
  real KiCad dependency.
- Do not claim manufacturer acceptance.
- Preserve existing CSV behavior unless a phase explicitly updates headers and
  tests.
- Treat fabrication readiness conservatively: unknown assembly-critical
  identity should block `ready`.
- Prefer extending `internal/fabrication` models and report builders before
  changing CLI behavior.
- Run focused tests after each phase and full `go test ./...` before the final
  documentation commit.

## Phase 1: Component Identity Evidence Model

### Goal

Add a reusable fabrication identity model without changing export behavior.

### Work

- Add `ComponentIdentity`, `IdentityStatus`, and identity source constants in
  `internal/fabrication`.
- Add fields for:
  - reference;
  - component ID;
  - value;
  - symbol ID;
  - footprint ID;
  - manufacturer;
  - MPN;
  - package;
  - component class;
  - lifecycle;
  - confidence;
  - source;
  - exact-part requirement flags;
  - issues.
- Add normalization and deterministic comparison helpers.
- Add issue helpers for missing, conflicting, skipped, and under-evidenced
  identity.

### Tests

- Identity normalization trims fields and stabilizes status.
- Conflict detection preserves source and reference evidence.
- Missing exact part produces a reference-scoped issue.

### Acceptance

- The fabrication package can represent component identity evidence independent
  of BOM/CPL rows.

### Commit

```text
Add fabrication component identity evidence model
```

## Phase 2: BOM Identity Hydration

### Goal

Attach identity evidence to BOM rows while preserving deterministic grouping.

### Work

- Extend `BOMRow` with identity-related fields:
  - package;
  - component class;
  - lifecycle;
  - identity status;
  - identity source;
  - issue count;
  - blocking count.
- Build `ComponentIdentity` from schematic symbol properties and fields.
- Recognize common property aliases:
  - `Component ID`;
  - `Manufacturer`;
  - `MPN`;
  - `Manufacturer Part Number`;
  - `Package`;
  - `Component Class`;
  - `Lifecycle`;
  - `Confidence`.
- Keep CSV headers stable unless adding columns is required by the model; if
  columns are added, update golden tests.
- Keep grouping deterministic by identity key.

### Tests

- BOM rows include identity status/source.
- Existing manufacturer/MPN rows still export as high confidence.
- Missing manufacturer/MPN produces identity evidence and readiness notes.
- Property aliases are recognized.
- Grouping is stable when identity fields match.

### Acceptance

- BOM output tells an AI caller whether each grouped row has sufficient identity
  evidence.

### Commit

```text
Hydrate BOM rows with component identity evidence
```

## Phase 3: CPL Identity And Placement Evidence

### Goal

Make CPL rows linkable to BOM identity and normalize side/rotation evidence.

### Work

- Extend `CPLRow` with:
  - component ID;
  - manufacturer;
  - MPN;
  - identity key or BOM linkage key;
  - normalized side;
  - raw layer;
  - raw rotation;
  - normalized rotation;
  - BOM linkage status;
  - readiness note.
- Add side normalization:
  - `F.Cu` -> `top`;
  - `B.Cu` -> `bottom`;
  - unknown layer -> blocking evidence.
- Add rotation normalization helper:
  - preserve raw KiCad rotation;
  - output deterministic normalized degrees in `[0, 360)`.
- Use schematic/BOM identity maps where available to hydrate CPL identity
  fields by reference.

### Tests

- Top and bottom layer side normalization.
- Negative and over-360 rotations normalize deterministically.
- CPL row links to BOM identity by reference.
- Unknown side produces a structured issue.

### Acceptance

- CPL evidence can be reconciled with BOM evidence and exposes side/rotation
  information needed for assembly review.

### Commit

```text
Hydrate CPL rows with identity and placement evidence
```

## Phase 4: BOM/CPL Consistency Validation

### Goal

Fail fabrication readiness when BOM and CPL disagree.

### Work

- Add `ValidateBOMCPLConsistency`.
- Validate:
  - duplicate BOM references;
  - duplicate CPL references;
  - on-board BOM reference missing from CPL;
  - CPL reference missing from BOM;
  - footprint mismatch;
  - missing coordinates;
  - unknown side;
  - DNP/not-in-BOM/not-on-board skip handling.
- Add a consistency summary:
  - checked references;
  - matched references;
  - skipped references;
  - warning count;
  - blocking count.
- Integrate consistency issues into `BuildReports`.

### Tests

- Consistent BOM/CPL passes.
- Missing CPL placement blocks.
- Extra CPL placement blocks.
- Duplicate refs block.
- Footprint mismatch blocks.
- DNP and not-on-board entries skip.

### Acceptance

- Fabrication reports can prove BOM and CPL represent the same assembly set.

### Commit

```text
Validate BOM CPL consistency evidence
```

## Phase 5: Manufacturer Profile Model

### Goal

Add local manufacturer profile rules without external services.

### Work

- Add `ManufacturerProfile` model:
  - ID;
  - display name;
  - required BOM columns;
  - required CPL columns;
  - accepted sides;
  - exact MPN policy;
  - generic passive policy;
  - rotation convention note;
  - severity mapping.
- Add built-in `generic_assembly` profile.
- Add profile registry and lookup.
- Add validation for unknown profile IDs.
- Add profile application to identity and BOM/CPL consistency evidence.

### Tests

- `generic_assembly` loads deterministically.
- Unknown profile returns a blocking issue.
- Active/connector/unknown class requires exact manufacturer/MPN.
- Generic passive policy is honored.
- Accepted side validation works.

### Acceptance

- Fabrication readiness can be evaluated against a local assembly profile.

### Commit

```text
Add generic assembly manufacturer profile
```

## Phase 6: Readiness And Manifest Integration

### Goal

Make identity/profile evidence first-class in fabrication readiness results.

### Work

- Extend fabrication summary with identity/profile gates, for example:
  - `component_identity`;
  - `bom_cpl_consistency`;
  - `manufacturer_profile`;
  - `assembly_readiness`.
- Extend package manifest evidence map with the same gates.
- Include identity/profile issues in readiness status calculation.
- Ensure fabrication-candidate workflows downgrade achieved acceptance when
  identity/profile blockers exist.
- Add report artifacts or manifest fields for identity summaries if needed.

### Tests

- Missing exact active MPN blocks readiness.
- Generic passive allowed by profile does not block.
- BOM/CPL mismatch blocks readiness.
- Manifest includes identity/profile evidence statuses.
- `design create` fabrication-candidate preview consumes identity blockers.

### Acceptance

- A fabrication package cannot be `ready` when identity/profile evidence is
  missing or inconsistent.

### Commit

```text
Gate fabrication readiness on identity evidence
```

## Phase 7: CLI Profile Selection And Goldens

### Goal

Expose manufacturer profile selection through CLI and lock down JSON evidence.

### Work

- Add global or export-specific flag:
  - `--manufacturer-profile generic_assembly`.
- Wire profile selection into fabrication options.
- Add selected-field CLI tests for:
  - `export bom`;
  - `export fabrication`;
  - unknown profile;
  - missing exact MPN blocker;
  - BOM/CPL mismatch blocker.
- Keep dry-run behavior unchanged.

### Tests

- CLI accepts `--manufacturer-profile generic_assembly`.
- Unknown profile blocks with an argument/evidence issue.
- JSON includes profile ID, profile status, identity blockers, and linked refs.
- Existing export tests continue passing.

### Acceptance

- AI callers can request and inspect local manufacturer profile evidence from
  CLI JSON.

### Commit

```text
Expose fabrication manufacturer profile evidence in CLI
```

## Phase 8: Documentation And Roadmap

### Goal

Document the new fabrication identity/profile gates and update project status.

### Work

- Update README fabrication export section:
  - identity evidence;
  - BOM/CPL consistency;
  - generic assembly profile;
  - exact MPN policy caveats;
  - no manufacturer API or acceptance guarantee.
- Update `specs/ROADMAP.md`:
  - mark BOM/CPL identity/profile evidence foundation implemented;
  - move next recommended priority to verified component/block expansion.
- Run full test suite.

### Tests

- `go test ./...`
- Prism review staged docs.

### Acceptance

- Documentation matches implemented behavior and clearly states remaining
  fabrication limitations.

### Commit

```text
Document fabrication identity evidence
```

## Final Acceptance Checklist

- BOM rows include component identity evidence.
- CPL rows include BOM linkage and normalized placement evidence.
- BOM/CPL consistency validation blocks unsafe package readiness.
- `generic_assembly` profile gates exact identity for assembly-critical parts.
- Fabrication readiness and manifests include identity/profile gates.
- CLI exposes profile selection and identity/profile issues.
- Default tests pass without KiCad or network access.
- README and roadmap identify the next remaining autonomy gap.
