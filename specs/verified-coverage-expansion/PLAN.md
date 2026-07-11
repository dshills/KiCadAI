# Verified Coverage Expansion Implementation Plan

Date: 2026-06-26

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep tests hermetic by default.
- Do not introduce live distributor or datasheet network calls.
- Do not promote uncertain records to fabrication readiness.
- Add small verified records before broad placeholder catalogs.
- Prefer existing `internal/components`, `internal/blocks`,
  `internal/pinmap`, and workflow evidence patterns.
- Run focused tests after each implementation phase and `go test ./...` before
  final closeout.
- Treat `prism review staged` as the required project review gate for each
  commit in this plan.

## Rollback Strategy

Each phase must be independently revertible because the work is committed in
small slices:

- If catalog changes introduce validation or selection regressions, revert the
  catalog/test commit and keep the Phase 1 audit as the record of the intended
  slice.
- If block or planner integration breaks existing workflows, revert only that
  integration commit and leave the lower-level component coverage in place.
- If generated workflow evidence exposes a writer or routing regression, keep
  the failing fixture only when it is marked as a known gap; otherwise revert
  the fixture and associated workflow assertions.
- If final compatibility fails after several commits, use the latest passing
  phase commit as the recovery point and file the failing behavior as a new
  gap before continuing.

## Phase 1: Coverage Audit And Priority Matrix

Status: completed by `COVERAGE_AUDIT.md`.

Objective: identify the smallest coverage expansion that materially improves
AI-generated designs.

Tasks:

- Generate or inspect current component coverage by family, confidence, rating
  metadata, symbol evidence, footprint evidence, and pinmap evidence.
- Generate or inspect current block coverage by readiness level, component
  roles, PCB realization, and verification fixtures.
- Build a priority matrix for candidate families:
  - design value;
  - required catalog data;
  - required block changes;
  - validation evidence available locally;
  - risk of unsafe inference.
- Finalize the first expansion slice in `COVERAGE_AUDIT.md`; the selected
  slice is concrete LDO/regulator and capacitor coverage.
- Preserve the remaining candidate families as follow-on coverage targets.

Review:

- Verify the slice is small enough for deterministic tests.
- Confirm it improves at least one intent workflow.

Commit:

- Commit audit/spec updates separately if they change repository files.

## Phase 2: Catalog Schema And Evidence Fixtures

Objective: add the component records needed by the selected slice.

Tasks:

- Add or update JSON records under `data/components/`.
- Include families, values, ratings, symbol bindings, package variants,
  function pins, pad functions, verification metadata, and companion
  requirements.
- Add negative cases where a part should fail selection due to missing rating,
  insufficient rating, missing function, unsafe confidence, or missing
  companion metadata.
- Add tests for deterministic catalog loading, sorting, validation, and
  evidence validation.
- Update component coverage expectations if coverage reports are asserted.

Review:

- `go test ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Expand verified component catalog coverage`.

## Phase 3: Component Selection Integration

Objective: prove the new components are selected and rejected through the
existing selection API.

Tasks:

- Add component `find` and `select` fixture requests where appropriate.
- Add selection tests for:
  - exact family/package/value match;
  - required rating pass;
  - required rating failure;
  - required function pass/failure;
  - concrete-part requirement pass/failure;
  - companion requirement pass/failure where modeled.
- Ensure rejected candidate diagnostics remain actionable.
- Ensure acceptance-level gates continue to block unsafe placeholder or
  low-confidence selections.

Review:

- `go test ./internal/components ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Add verified component selection coverage`.

## Phase 4: Block Variant Or Role Integration

Objective: connect the new catalog coverage to one or more circuit blocks.

Tasks:

- Update relevant block definitions or block component queries.
- Add new parameters only when they are safe and validated.
- Add or update component roles, required libraries, validation rules,
  unsupported behavior notes, and verification level.
- For PCB-capable blocks, add or update PCB realization placements, groups,
  local routes, and validation expectations.
- Ensure block metadata exposes enough information for `block show`, planner
  rationale, and workflow evidence.

Review:

- `go test ./internal/blocks`
- `prism review staged`

Commit:

- Commit with a message like `Connect verified components to block variants`.

## Phase 5: Intent Planner Mapping

Objective: make structured intent able to use the new verified coverage without
guessing.

Tasks:

- Add explicit mapping rules for the selected function, interface, protection,
  power, or block family.
- Add target/bus/supply handling where the new slice needs it.
- Add component policy defaults or rating requirements only where catalog
  metadata supports enforcement.
- Add planner tests for:
  - successful selection;
  - ambiguous intent requiring clarification;
  - unsupported variant becoming a known gap or blocker;
  - generated request shape;
  - synthesis trace and rationale evidence.

Review:

- `go test ./internal/intentplanner ./internal/rationale`
- `prism review staged`

Commit:

- Commit with a message like `Map verified coverage into intent planning`.

## Phase 6: Workflow And Generated Project Evidence

Objective: prove the expanded coverage works through generated projects.

Tasks:

- Add or update `examples/intent/` or `examples/design/` request fixtures.
- Run generated workflows in tests or focused golden fixtures.
- Assert component-selection stage output includes the expected component IDs,
  variants, ratings, and evidence.
- For PCB-realized blocks, assert writer correctness and board validation
  evidence where local tests can do so without requiring KiCad.
- Add optional KiCad-backed fixture metadata only if the block claims ERC/DRC
  confidence.

Review:

- `go test ./internal/designworkflow ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Add generated workflow coverage for verified parts`.

## Phase 7: Documentation And Agent Skill Updates

Objective: keep human and agent guidance aligned with the new coverage.

Tasks:

- Update `README.md` only if quick-start or status changes.
- Update focused docs:
  - `docs/component-intelligence.md`;
  - `docs/circuit-blocks.md`;
  - `docs/intent-planning.md`;
  - `docs/kicadai-agent-skill.md`.
- Update `specs/ROADMAP.md` with completed coverage and remaining gaps.
- Document any new request examples and validation expectations.
- Keep examples using the compiled `kicadai` binary.

Review:

- Run local Markdown link check.
- `rg -n "go run ./cmd/kicadai|go run" README.md docs`
- `prism review staged`

Commit:

- Commit with a message like `Document verified coverage expansion`.

## Phase 8: Final Compatibility Sweep

Objective: ensure the expansion does not weaken existing generation behavior.

Tasks:

- Run `go test ./...`.
- Run focused CLI fixtures for component selection, block inspection, intent
  planning, and generated design workflows.
- Confirm no new network or KiCad dependency is required by default tests.
- Inspect generated JSON for deterministic ordering and stable issue paths.
- Confirm fabrication-candidate workflows still fail closed on missing
  evidence.

Review:

- `go test ./...`
- `prism review staged` if any final files changed.

Commit:

- Commit any final compatibility fixes with a message like
  `Harden verified coverage expansion`.

## Completion Criteria

- The selected expansion slice is represented in catalog records, block
  metadata, planner mappings, generated workflow evidence, and docs.
- Positive and negative component selection tests prove rating/function/confidence
  behavior.
- Generated design workflows can use the new coverage without hidden guesses.
- Rationale reports explain the selected parts and remaining gaps.
- Full tests pass.

## Completed Follow-On Wave: Concrete I2C Sensor Families

The finite wave defined in `specs/i2c-sensor-family-expansion/` is complete:
verified BMP280 and SHT31-DIS records join BME280 as executable concrete
variants of the existing `i2c_sensor` block. Catalog, pinmap, topology, intent,
generated operation identity, post-write electrical, readability, and writer
correctness evidence are covered without removing the generic path.
