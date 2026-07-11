# Verified I2C Sensor Family Expansion Plan

Date: 2026-07-11

## Implementation Rules

- Implement phases in order.
- Run focused tests in every phase and the full suite at closeout.
- Stage each completed phase, run `prism review staged`, resolve findings, and
  commit before moving to the next phase.
- Keep default tests hermetic and preserve generic I2C requests.
- Fail closed on unknown component IDs or mismatched library assignments.

## Phase 1: Scope And Evidence Contract

Deliverables:

- `SPEC.md` and this phased plan;
- explicit BMP280/SHT31-DIS source, pinmap, topology, and acceptance boundaries;
- an audit note in the existing verified-coverage plan naming this as the next
  completed expansion wave.

Tests:

- documentation consistency checks;
- confirm the current worktree and baseline test suite are clean.

Acceptance:

- the expansion is finite, names exact parts, and does not imply arbitrary
  sensor synthesis.

Risk: low; documentation-only and independently revertible.

## Phase 2: Catalog And Pinmap Evidence

Likely files:

- `data/components/verified_active.json`;
- `internal/pinmap/pinmap.go` and focused tests;
- `internal/components/catalog_test.go`, `coverage_test.go`, or golden fixtures.

Work:

- add verified BMP280 and SHT31-DIS records;
- add exact built-in symbol/footprint pinmaps;
- add positive catalog/coverage checks and negative rating/function selection
  tests;
- update deterministic coverage goldens.

Tests:

- `go test ./internal/components ./internal/pinmap`.

Acceptance:

- both parts load, validate, select deterministically, and expose resolver and
  pinmap evidence; unsafe requests are rejected.

Risk: medium; incorrect pin identity would invalidate generated connectivity.

## Phase 3: Concrete Sensor Block Profiles

Likely files:

- `internal/blocks/builtin.go`;
- `internal/blocks/builtin_realization.go`;
- `internal/blocks/i2c_sensor.go`;
- `internal/blocks/component_selection.go` and tests.

Work:

- add `sensor_component_id` support;
- define allowlisted BME280, BMP280, and SHT31-DIS profiles;
- emit profile-specific symbols, footprints, pin anchors, supplies, grounds,
  SDA/SCL, optional interrupt, and auxiliary-pin treatment;
- propagate the dynamic component ID through block component selection;
- preserve the generic block path exactly.

Tests:

- `go test ./internal/blocks ./internal/components`.

Acceptance:

- each concrete profile produces a valid transaction with exact real-pin
  connectivity; unsupported or mismatched input fails closed.

Risk: high; block topology and operation rewriting touch schematic electrical
connectivity, so this phase remains a separate commit.

## Phase 4: Intent And Workflow Integration

Likely files:

- `internal/intentplanner` mapping/model tests;
- `internal/designworkflow` component-selection/workflow tests;
- `examples/intent` and `examples/design` fixtures.

Work:

- pass explicit concrete sensor IDs through structured intent;
- preserve ambiguity as a structured issue when a concrete selection is
  required but absent;
- add BMP280 and SHT31-DIS generated schematic fixtures;
- assert selected identity properties, parseability, electrical checks,
  readability, and writer correctness.

Tests:

- `go test ./internal/intentplanner ./internal/designworkflow ./cmd/kicadai`.

Acceptance:

- structured intent deterministically reaches a written real-part schematic and
  reports the selected component/package evidence.

Risk: medium; integration must not alter existing seed phrase behavior.

## Phase 5: Documentation And Compatibility Closeout

Likely files:

- `docs/component-intelligence.md`;
- `docs/circuit-blocks.md`;
- `docs/intent-planning.md`;
- `docs/kicadai-agent-skill.md`;
- `specs/ROADMAP.md`;
- `specs/verified-coverage-expansion/PLAN.md`.

Work:

- document concrete sensor selection and limits;
- record catalog/topology breadth gained and remaining families;
- run focused CLI examples and full regression suite;
- audit every specification acceptance item against current evidence.

Tests:

- `rg -n "go run ./cmd/kicadai|go run" README.md docs`;
- `go test -p 1 ./...`.

Acceptance:

- docs and roadmap match actual behavior, all tests pass, and no default network
  or KiCad dependency was introduced.

Risk: low; final fixes are committed separately if code changes are required.
