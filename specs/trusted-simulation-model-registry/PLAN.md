# Implementation Plan

## Phase 1 — Trust Boundary and Registry (Complete)

- Define provider-safe intent, catalog evidence, resolved-plan, diagnostic,
  measurement, assertion, and report types in `internal/simmodel`.
- Define a canonically ordered trusted registry and stable SHA-256 registry
  digest.
- Reject unknown IDs, roles, parameters, metrics, duplicates, non-finite or
  out-of-range values, and provider model content.

## Phase 2 — Catalog and Resolution Integration (Complete)

- Add optional trusted simulation evidence to generic component records.
- Validate evidence against registry roles and required catalog parameters.
- Resolve model bindings only after component/catalog resolution and snapshot
  catalog identity, component identity/family/value, and model parameters.
- Lower only resolved plans into design-workflow requests.

## Phase 3 — Generic Evaluation and Artifacts (Complete)

- Replace the regulator-specific workflow map with registry evaluation.
- Implement catalog-parameterized linear-regulator, resistor-divider DC, and RC
  low-pass AC families.
- Emit one deterministic normalized `.kicadai/simulation.json` artifact and
  feed the existing simulation promotion gate.
- Validate resolved plans before execution and fail closed with suggestions.

## Phase 4 — Held-Out and Regression Evidence (Complete)

- Add `generic_filtered_divider_hierarchy` without adding a block family.
- Require automatic hierarchy and trusted divider assertions.
- Add RC model evidence to the existing generic RC promotion fixture.
- Extend optional KiCad acceptance to execute the held-out sanitized replay and
  compare every generated KiCad file and simulation artifact byte-for-byte.
- Preserve protected LED, protected I2C, and hierarchical BMP280 pass evidence.

## Phase 5 — Completion Audit (Complete)

- Regenerate deterministic catalog-dependent goldens.
- Run focused fail-closed tests, full Go tests, lint, required strict optional
  KiCad promotion fixtures, protected fixture regressions, and diff checks.
- Require held-out replay to compare the complete generated KiCad file set and
  trusted simulation report byte-for-byte.
