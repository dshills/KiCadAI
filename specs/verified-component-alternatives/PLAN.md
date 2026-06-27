# Verified Component Alternatives Implementation Plan

Date: 2026-06-27

## Implementation Rules

- Commit each phase independently after `prism review staged`.
- Keep all tests hermetic; no network, live distributor APIs, or credentials.
- Prefer small catalog slices with high confidence over broad unverified data.
- Preserve deterministic selection and stable issue ordering.
- Do not remove existing generic records; add concrete alternatives and policy
  gates around them.
- Run focused tests after each phase and `go test ./...` before closeout.

## Phase 1: Equivalence Metadata Model

Objective: define the minimal catalog metadata needed to represent verified
alternatives safely.

Tasks:

- Add an optional equivalence model to component records if no existing field
  can express it cleanly.
- Define allowed roles:
  - `preferred`;
  - `alternate`;
  - `fallback`.
- Clarify that equivalence `fallback` is a lower-priority member inside a
  declared equivalent set and is distinct from generic fallback records used at
  draft/structural acceptance.
- Normalize and validate equivalence groups.
- Add validation issues for:
  - invalid equivalence role;
  - missing group with role;
  - multiple preferred records in one group;
  - incompatible family/package/value group members.
- Add unit tests for valid groups, invalid roles, duplicate preferred records
  within the same normalized family and equivalence group, and deterministic
  issue paths.

Review:

- `go test ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Add component equivalence metadata`.

## Phase 2: Deterministic Alternative Selection

Objective: make the selector choose concrete alternatives without ambiguous
equal-score failures.

Tasks:

- Extend candidate scoring or post-filter tie-breaking to prefer concrete
  verified records when acceptance is connectivity or stronger.
- Apply equivalence role order after required electrical/package/confidence and
  procurement gates:
  - preferred;
  - alternate;
  - fallback.
- Preserve `COMPONENT_AMBIGUOUS` for non-equivalent equal-score records.
- Preserve draft/structural generic fallback behavior.
- Add tests for:
  - preferred concrete alternative wins;
  - declared equivalent alternatives do not block as ambiguous;
  - non-equivalent equal-score records still block;
  - generic fallback remains selectable when no concrete part satisfies the
    request.

Review:

- `go test ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Select deterministic component alternatives`.

## Phase 3: Seed Concrete Alternative Records

Objective: add the first small, reviewable concrete alternative catalog slice.

Tasks:

- Add concrete resistor alternatives for selected 0603/0805 values used by
  current planner/block flows.
- Add concrete ceramic capacitor alternatives for selected decoupling and
  regulator values with voltage ratings.
- Add one LED indicator alternative for a supported LED package/path if
  evidence is sufficient.
- Add one connector/header alternative for existing connector breakout or
  programming-header flows if evidence is sufficient.
- Keep generic records in place as fallback.
- Add or update source snapshot fixtures for at least one new concrete
  alternative.
- Add catalog validation and selection golden tests for the new records.

Review:

- `go test ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Add verified component alternative records`.

## Phase 4: Coverage And CLI Reporting

Objective: expose alternative health through existing component reporting.

Tasks:

- Extend component coverage with:
  - per-family `concrete_records`;
  - per-family `generic_fallback_records`;
  - per-family and aggregate `equivalence_groups`;
  - aggregate `groups_missing_preferred`;
  - aggregate `groups_with_duplicate_preferred`;
  - aggregate `concrete_records_missing_mpn`.
- Keep existing coverage output backward-compatible where practical.
- Extend CLI tests for `component coverage`.
- Add focused JSON assertions rather than brittle large goldens when possible.

Review:

- `go test ./internal/components ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Report component alternative coverage`.

## Phase 5: Workflow And Fabrication Evidence

Objective: prove concrete alternatives flow through generated design evidence
and BOM output.

Tasks:

- Add a small design workflow test that selects a concrete alternative and
  verifies:
  - selected component ID;
  - manufacturer;
  - MPN;
  - confidence;
  - procurement evidence when a source snapshot is provided.
- Add a fabrication/BOM test that verifies concrete alternative identity is
  present in BOM rows.
- Ensure source snapshot enrichment still works for the new alternatives.
- Ensure generic fallback in fabrication-candidate mode remains warning/blocking
  according to existing policy.

Review:

- `go test ./internal/designworkflow ./internal/fabrication ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Propagate component alternative evidence`.

## Phase 6: Documentation And Examples

Objective: document the alternative/equivalence contract for users and agents.

Tasks:

- Update:
  - `docs/component-intelligence.md`;
  - `docs/libraries-and-components.md`;
  - `docs/kicadai-agent-skill.md`;
  - `specs/ROADMAP.md`.
- Add example component selection requests under `examples/components/` for at
  least one concrete resistor or capacitor alternative.
- Document that equivalence is scoped to modeled requirements and does not mean
  universal datasheet interchangeability.
- Check for stale source-tree invocation docs.

Review:

- `rg -n "go run ./cmd/kicadai|go run" README.md docs`
- `git diff --check`
- `prism review staged`

Commit:

- Commit with a message like `Document verified component alternatives`.

## Phase 7: Final Compatibility Sweep

Objective: prove alternative support does not regress current generation.

Tasks:

- Run `go test ./...`.
- Run focused component, workflow, fabrication, and CLI tests.
- Verify common existing examples still resolve their previous or intentionally
  updated selections.
- Verify no test requires network, KiCad, or procurement credentials.
- Update this spec directory with follow-up notes only if implementation
  reveals new gaps.

Review:

- `go test ./...`
- `prism review staged` only if files changed in this phase.

Commit:

- Commit only if the final sweep requires file changes.
