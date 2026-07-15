# KiCadAI Review Follow-up Implementation Plan

This plan implements `specs/kicadai-review-followup/SPEC.md` in dependency
order. Each phase is independently reviewable and revertible. The plan does
not treat every review observation as a code defect: the enum work is already
complete, the HPWL observation is documented as a limitation, and historical
metrics are not implementation phases.

## Phase 0: Baseline and Finding Ledger

**Objective:** Freeze reproducible evidence for the review dispositions before
changing behavior.

**Likely files:**

- `specs/kicadai-review.md`
- `specs/kicadai-review-followup/SPEC.md`
- `specs/kicadai-review-followup/PLAN.md`
- `specs/ROADMAP.md` or the current roadmap location, if an item is tracked

**Work:**

- Record the baseline commit, Go version, golangci-lint version, test command,
  lint command, and coverage command.
- Add a concise disposition link from the review to this plan.
- Confirm the shared vocabulary commit remains in history and keep its tests
  unchanged.
- Do not change production behavior.

**Tests and evidence:**

- `go test ./...`
- `make lint`
- `make coverage-check`
- `git status --short`

**Acceptance:** The finding ledger distinguishes addressed, real, historical,
and unsupported claims, and the working tree is clean before Phase 1.

**Rollback risk:** None beyond documentation drift.

## Phase 1: Offline Continuous Integration

**Objective:** Make the current green baseline enforceable on every change.

**Likely files:**

- `.github/workflows/ci.yml`
- `Makefile`
- `.golangci.yml`
- `go.mod`/`go.sum` only if the supported Go version is clarified
- `README.md` or `docs/development.md`

**Work:**

- Add a CI workflow for formatting, tests, lint, and coverage threshold.
- Use the repository Make targets as the source of truth where practical.
- Pin the Go and golangci-lint versions used by CI.
- Keep KiCad/OpenAI jobs separate and optional.
- Ensure the job does not require a working-directory catalog or credentials.

**Tests and evidence:**

- Run each CI command locally.
- Validate the workflow YAML with the repository's available tooling.
- Confirm optional KiCad checks skip cleanly when unavailable.

**Acceptance:** A clean checkout has one documented offline CI path and a
failure in test, lint, formatting, or coverage blocks the workflow.

**Rollback risk:** Medium. Incorrect tool versions or coverage assumptions can
block contributors; keep the workflow small and use existing Make targets.

## Phase 2: Embedded Default Catalog

**Objective:** Remove the deployment dependency on the caller's working
directory while preserving custom catalog support.

**Likely files:**

- `internal/components/catalog.go`
- `internal/designworkflow/component_selection.go`
- `data/components/**`
- `internal/components/catalog_test.go`
- `internal/designworkflow/*_test.go`
- `cmd/kicadai/**` only if CLI diagnostics expose catalog source

**Work:**

- Embed the checked-in default catalog with `go:embed` in a narrowly owned
  package.
- Refactor parsing so embedded and filesystem records use the same validation
  path.
- Define precedence: explicit filesystem directory first, embedded default
  second.
- Return source/fingerprint diagnostics without making output nondeterministic.
- Test execution from a temporary working directory with no repository catalog.

**Tests and evidence:**

- Catalog unit tests for embedded loading, custom override, missing explicit
  directory, stable ordering, and fingerprint.
- Existing component-resolution tests.
- At least one binary-level or package-level test that changes the working
  directory.
- `go test ./...`, `make lint`, and coverage check.

**Acceptance:** The packaged default path resolves trusted components without
  `data/components` in the working directory, and all existing fixture
  projections remain equivalent.

**Rollback risk:** Medium-high. Embedding can alter release size and catalog
  precedence. Preserve explicit filesystem override and keep the change behind
  the existing loader API.

## Phase 3: Routing Contract Closeout

**Objective:** Stop advertising an unused rip-up/retry capability.

**Likely files:**

- `internal/routing/model.go`
- `internal/routing/route.go`
- `internal/routing/planner.go`
- routing schema/request examples and focused tests found by `rg`
- `specs` routing documentation

**Work:**

- Inventory every serialized use of `RipupRetryLimit`.
- Prefer the smallest compatible resolution: mark the field deprecated and
  reject non-zero values with a structured unsupported diagnostic, or remove it
  with an explicit fixture migration.
- Add a test proving the executor does not silently ignore a non-zero retry
  request.
- Document that deterministic ordered routing is the current contract and that
  real rip-up is future work.

**Tests and evidence:**

- Routing model decode/validation tests.
- Existing route completion and connectivity tests.
- Targeted generic and USB-C promotion fixture tests where routing strategy is
  shared.
- `go test ./...`, lint, and round-trip/writer tests.

**Acceptance:** The public request, validation diagnostics, and executor agree
  about retry behavior; no existing pass fixture regresses.

**Rollback risk:** Medium. Removing a serialized field can break external
  callers. Prefer structured rejection and a deprecation window unless the
  repository proves the field is entirely internal.

## Phase 4: Workflow Stage Seams

**Objective:** Reduce the risk of the large `internal/designworkflow` package
  without changing generation behavior.

**Likely files:**

- `internal/designworkflow/create.go`
- `internal/designworkflow/*` identified by dependency analysis
- new narrow packages under `internal/workflowstages/` only if justified
- stage characterization tests

**Work:**

- Produce a dependency map and identify one extraction boundary.
- Start with a low-risk boundary, preferably evidence/promotion or catalog
  resolution, rather than moving the route engine first.
- Define input/output structs that preserve existing JSON, artifact paths,
  diagnostics, and deterministic ordering.
- Add characterization tests around the old and new path before deleting code.
- Extract one stage, then stop and measure whether the dependency surface
  improved. Do not split files only to reduce line count.

**Tests and evidence:**

- Stage unit tests.
- Existing designworkflow tests.
- Golden transaction/evidence comparisons for representative fixtures.
- Optional KiCad-backed tests for at least one existing pass fixture.

**Acceptance:** The extracted stage can be tested independently, the CLI and
  artifact contract are unchanged, and normalized generated outputs are equal
  before and after extraction.

**Rollback risk:** High. Package movement can create cycles or subtle ordering
  changes. One boundary per commit, with golden comparisons and easy revert.

## Phase 5: Generic Capability Matrix and Planner Boundary

**Objective:** Make the generic path's actual ceiling explicit and prevent
bounded topology logic from being mistaken for arbitrary-circuit generation.

**Likely files:**

- `internal/intentplanner/**`
- generic circuit/provider packages identified by `rg`
- `internal/domain` or a new capability package if shared types are required
- `specs` AI-generation documentation
- capability matrix tests

**Work:**

- Define capabilities in terms of component roles, net multiplicity, units,
  layout requirements, routing requirements, and evidence gates.
- Classify existing fixtures as generic, bounded, or unsupported.
- Add fail-closed tests for ambiguous and unsupported graphs.
- Route only proven generic graphs through the generic path; retain bounded
  mappings for supported legacy topologies.
- Do not add topology-specific provider schemas as a way to hide capability
  gaps.

**Tests and evidence:**

- Capability matrix unit tests.
- Recorded provider-to-planner tests.
- Existing generic RC, protected LED, BMP280, and multi-unit fixture tests.
- Negative tests for project-name/path-based dispatch.

**Acceptance:** The repository can state what “generic” means today, and an
unsupported arbitrary circuit fails with a useful diagnostic instead of making
an unsafe partial design.

**Rollback risk:** Medium-high. Planner dispatch affects many fixtures. Keep
the matrix advisory first, then enforce only cases covered by tests.

## Phase 6: Placement and Error-Handling Quality Closeout

**Objective:** Address the review's partial findings without inventing an
optimizer or performing a mechanical error rewrite.

**Likely files:**

- `internal/placement/**`
- selected cross-package error boundaries identified by `rg 'fmt.Errorf'`
- `internal/*/*_test.go`
- contributor/development documentation

**Work:**

- Document that current placement is deterministic candidate scoring plus HPWL
  measurement, not global optimization.
- Add a focused regression metric for representative layouts so candidate
  scoring cannot silently worsen quality.
- Add error-wrapping guidance and migrate high-value boundaries when touched.
- Add a targeted static check or review rule for new cross-package errors where
  causes are expected to be classified.

**Tests and evidence:**

- Placement candidate and HPWL regression tests.
- Error classification tests using `errors.Is`/`errors.As` at selected
  boundaries.
- Full test and lint runs.

**Acceptance:** Placement limitations are measurable and explicit; new
cross-package failures preserve causes where needed; no broad mechanical
rewrite is required.

**Rollback risk:** Low-medium. Keep metric thresholds fixture-specific and
avoid making aesthetic HPWL changes a correctness gate.

## Phase 7: Historical Regression Corpus and Documentation

**Objective:** Convert historical review concerns into evidence only when they
can be reproduced and keep the project status understandable.

**Likely files:**

- `specs/archive/**` references only
- `examples/checks/**` or the existing golden corpus
- relevant writer/round-trip tests
- `README.md`, `docs/**`, `specs/ROADMAP.md`

**Work:**

- Select historical electronics and round-trip findings that still represent
  meaningful risk.
- Reproduce each selected issue or mark it unreproducible with the command and
  commit tested.
- Add a focused fixture/test for reproduced issues.
- Link the review disposition, this plan, and the capability matrix from the
  roadmap and development docs.

**Tests and evidence:**

- Selected regression tests.
- Full offline suite and lint.
- Optional KiCad-backed evidence for fixtures that require KiCad.
- Clean working-tree check.

**Acceptance:** Historical claims have a reproducible status, current gaps have
owners/next phases, and documentation no longer presents stale metrics as open
defects.

**Rollback risk:** Low. This phase is primarily tests and documentation.

## Phase Completion Protocol

For every implementation phase:

1. Run the phase's baseline command before editing.
2. Implement one coherent change.
3. Run focused tests, then `go test ./...` and `make lint` when shared code is
   touched.
4. Run the relevant optional KiCad-backed fixture when generation or evidence
   behavior changes.
5. Review staged code with Prism before commit.
6. Commit the phase independently and record the evidence paths.
7. Stop and reassess the next phase if the evidence contradicts this plan.

The project is complete for this review follow-up when Phases 1-3 are done,
Phase 4 has at least one safe seam extracted, and the remaining architectural
limitations are represented by the capability matrix and regression evidence.

