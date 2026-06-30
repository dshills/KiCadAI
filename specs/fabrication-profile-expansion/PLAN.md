# Fabrication Profile Expansion Implementation Plan

## Objective

Implement local and built-in fabrication profile expansion so physical-rule
checks, fabrication readiness, and package manifests can explain which
manufacturer-style constraints were enforced.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests hermetic and KiCad-independent.
- Do not add live manufacturer APIs.
- Do not claim manufacturer acceptance.
- Keep existing `generic_assembly` behavior backward compatible.
- Prefer structured profile evidence over message parsing.

## Phase 1: Profile Schema And Validation Model

### Goal

Create a public local profile schema and deterministic validator.

### Work

- Add `internal/fabrication/profiles` package.
- Define profile model:
  - schema;
  - ID/name/version;
  - source metadata;
  - units;
  - stackup;
  - copper;
  - drill/annular ring;
  - solder mask/paste;
  - silkscreen/assembly;
  - edge plating;
  - impedance/differential pairs;
  - metadata requirements.
- Add validation:
  - required fields;
  - supported schema;
  - ASCII ID;
  - positive numeric thresholds;
  - allowed units;
  - internally consistent layer and drill policies;
  - `allowed_layer_counts` entries must fall within inclusive
    `min_layers`/`max_layers` bounds when those bounds are set;
  - unknown enum values.
- Add canonical hash calculation for profile provenance:
  - use SHA-256;
  - hash the profile after deterministic canonicalization;
  - trim scalar identity fields;
  - ignore volatile source metadata;
  - sort and compact set-like arrays;
  - keep the hash stable across JSON formatting differences.
- Add fixtures for valid and invalid profiles.

### Tests

- Valid profile passes.
- Unsupported schema blocks.
- Missing ID/name/version blocks.
- Negative thresholds block.
- Invalid layer counts block.
- Canonical hash is deterministic across JSON formatting.

### Verification

```sh
go test ./internal/fabrication/profiles
```

### Commit

```text
Add fabrication profile schema validation
```

## Phase 2: Built-In Profile Registry

### Goal

Add multiple built-in profiles without changing existing export behavior.

### Work

- Add built-in profile definitions:
  - `generic_assembly`;
  - `generic_2layer_economy`;
  - `generic_2layer_standard`;
  - `generic_4layer_standard`;
  - `generic_castellated_review`.
- Add registry APIs:
  - list profiles;
  - resolve by ID;
  - show profile details;
  - return validation issues.
- Preserve `generic_assembly` as the default existing profile ID.
- Add aliases only if needed for backward compatibility.

### Tests

- Built-ins validate at package init/test time.
- Profile IDs are unique.
- Registry list ordering is deterministic.
- Unknown profile returns a structured issue.
- `generic_assembly` thresholds match current behavior.

### Verification

```sh
go test ./internal/fabrication ./internal/fabrication/profiles
```

### Commit

```text
Add built-in fabrication profile registry
```

## Phase 3: Local Profile Directory Loading

### Goal

Allow trusted local profile snapshots to be loaded without network access.

### Work

- Add loader for `*.json` files under a configured profile directory.
- Add path containment checks.
- Add duplicate ID handling:
  - local-local duplicates block;
  - built-in shadowing blocks.
- Add environment support:
  - `KICADAI_FABRICATION_PROFILE_DIR`.
- Add CLI/global option:
  - `--manufacturer-profile-dir`.
- Add deterministic merge order:
  - built-ins first;
  - local profiles sorted by ID/path.

### Tests

- Loads valid local profile.
- Rejects malformed JSON.
- Rejects path escapes.
- Rejects duplicate local profile IDs.
- Rejects built-in shadowing.
- Environment directory is honored.
- CLI flag overrides `KICADAI_FABRICATION_PROFILE_DIR`.

### Verification

```sh
go test ./internal/fabrication ./internal/fabrication/profiles ./cmd/kicadai
```

### Commit

```text
Load local fabrication profiles
```

## Phase 4: Profile CLI Commands

### Goal

Expose profile discovery and validation to users and AI agents.

### Work

- Add command family:
  - `fabrication profile list`;
  - `fabrication profile show <id>`;
  - `fabrication profile validate <path>`.
- Keep JSON output as default.
- Include:
  - profile ID/name/version/source;
  - validation status;
  - issue list;
  - profile hash;
  - selected major thresholds.
- Update help text.
- Add selected-field CLI tests.

### Tests

- List includes built-ins.
- Show returns deterministic profile JSON.
- Validate returns OK for valid local profile.
- Validate returns nonzero and structured issues for invalid local profile.
- Unknown subcommands produce structured errors.

### Verification

```sh
go test ./cmd/kicadai ./internal/fabrication/profiles
```

### Commit

```text
Expose fabrication profile CLI commands
```

## Phase 5: Physical Rule Profile Integration

### Goal

Drive physical-rule thresholds from the resolved active profile.

### Work

- Map profile fields into `internal/fabrication/physicalrules.Profile`.
- Normalize every mapped profile threshold to millimeters before comparison;
  the version-one schema only accepts `units: "mm"` so non-mm profiles must
  fail validation rather than be compared directly.
- Replace hardcoded generic thresholds where profile fields exist.
- Keep default profile behavior stable when no explicit profile is selected.
- Add profile provenance to physical-rule report:
  - profile ID;
  - version;
  - hash;
  - source kind/path.
- Mark checks with `source: "profile"` where profile thresholds are used.
- Add unsupported/skipped evidence for fields that are present but not yet
  modeled.

### Tests

- Copper width threshold changes with selected profile.
- Annular ring threshold changes with selected profile.
- Solder mask web threshold changes with selected profile.
- Edge-plating policy changes with selected profile.
- Metadata required fields change with selected profile.
- Unsupported profile fields appear as non-silent evidence.

### Verification

```sh
go test ./internal/fabrication/physicalrules ./internal/fabrication
```

### Commit

```text
Apply fabrication profiles to physical rules
```

## Phase 6: Readiness And Manifest Provenance

### Goal

Make profile selection visible and durable in fabrication outputs.

### Work

- Add active profile summary to fabrication readiness result:
  - ID;
  - name;
  - version;
  - source;
  - hash;
  - validation status.
- Add profile provenance to package manifest.
- Add readiness evidence map entries for:
  - profile validation;
  - unsupported profile fields;
  - profile-derived physical-rule categories.
- Ensure invalid requested profile blocks `ready`.
- Ensure missing optional profile directory does not break default exports.

### Tests

- Readiness JSON includes active profile provenance.
- Manifest includes profile ID/version/hash.
- Invalid requested profile blocks readiness.
- Unknown requested profile blocks readiness.
- Default profile output remains backward compatible except for additive fields.

### Verification

```sh
go test ./internal/fabrication ./cmd/kicadai
```

### Commit

```text
Record fabrication profile provenance
```

## Phase 7: Documentation And Roadmap

### Goal

Document profile usage and remaining limits.

### Work

- Update README fabrication section.
- Update `docs/fabrication.md`.
- Update `docs/cli-reference.md`.
- Update `docs/kicadai-agent-skill.md` with profile stop conditions.
- Update `specs/ROADMAP.md` Priority 8 current foundation and remaining work.
- Add example local profile JSON under docs or testdata if useful.

### Tests

- Documentation examples use `kicadai`, not `go run ./cmd/kicadai`.
- CLI examples match implemented commands and flags.

### Verification

```sh
go test ./...
```

### Commit

```text
Document fabrication profile expansion
```

## Prism And Commit Process

For each phase:

1. Implement the phase.
2. Run the targeted tests.
3. Stage only phase-relevant files.
4. Run `prism review staged`.
5. Fix actionable findings.
6. Commit the phase.
7. Continue only when the worktree is clean or unrelated changes are explicitly
   ignored.

## Success Criteria

The project is complete when:

- multiple built-in profiles are available and validated;
- local profile JSON snapshots can be loaded deterministically;
- users can list, show, and validate profiles from the CLI;
- physical-rule thresholds come from the active profile;
- readiness and manifests record profile provenance;
- invalid profiles block readiness with structured issues;
- all default tests remain KiCad-independent and pass with `go test ./...`.
