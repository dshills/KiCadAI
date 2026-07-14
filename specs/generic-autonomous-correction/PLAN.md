# Generic Autonomous Placement And Routing Correction Plan

## Objective

Extend the existing bounded placement-routing retry foundation into a
generic-circuit-only autonomous correction contract with stable diagnostics,
pure plans, guarded application, persisted evidence, and one recoverable
identity-neutral stress fixture.

## Implementation Rules

- Extend existing `designworkflow` retry and route-tree evidence.
- Keep provider output strict, untrusted, and outside engineering correction.
- Preserve existing non-generic retry behavior.
- Keep every mutation deterministic and bounded.
- Fail closed when an action is not explicitly authorized.
- Do not add topology-specific production logic.
- Run focused tests after every phase.
- Review staged changes with Prism before every commit.
- Commit phases separately when practical.

## Phase 1: Specification And Baseline

### Work

- Record the existing retry, route-tree repair, generic lowering, provider, and
  artifact-writing foundations.
- Define scope eligibility, taxonomy, authorization, invariant, budget, retry
  key, artifact, and stop contracts.
- Record the green repository baseline.

### Likely files

- `specs/generic-autonomous-correction/SPEC.md`
- `specs/generic-autonomous-correction/PLAN.md`

### Tests

- `go test ./...`
- Markdown/diff checks.

### Acceptance

- The specification identifies reuse boundaries and does not promise
  unsupported routing-only transformations.

### Rollback risk

- None; documentation only.

## Phase 2: Taxonomy, Diagnostics, And Stable Keys

### Work

- Add typed autonomous correction categories and diagnostic records.
- Map placement, routing, route-tree, endpoint-access, and contact-graph issues
  into the stable taxonomy.
- Preserve source diagnostic evidence.
- Add canonical SHA-256 retry keys.
- Add an explicit-circuit invariant fingerprint.
- Prove path/message independence and deterministic ordering.

### Likely files

- `internal/designworkflow/autonomous_correction.go`
- `internal/designworkflow/autonomous_correction_test.go`
- existing route-tree diagnostic helpers only if shared evidence is missing.

### Tests

- Focused taxonomy tests for all categories.
- Retry-key normalization tests.
- Invariant fingerprint mutation tests.
- `go test ./internal/designworkflow -count=1`.

### Acceptance

- Equivalent failures produce identical diagnostics and retry keys.
- Protected circuit changes alter the invariant fingerprint.

### Rollback risk

- Low; additive models and pure functions.

## Phase 3: Pure Planning

### Work

- Add a deterministic correction plan model.
- Convert eligible diagnostics into ordered authorized actions.
- Represent routing-only reserved actions as unsupported in v1.
- Return structured stop reasons for unsupported, ambiguous, or repeated
  failures.
- Keep planning independent of file IO and provider calls.

### Likely files

- `internal/designworkflow/autonomous_correction.go`
- `internal/designworkflow/autonomous_correction_plan_test.go`

### Tests

- One focused test per supported action.
- Unsupported and ambiguous fail-closed tests.
- Stable plan JSON golden.

### Acceptance

- Planning does not mutate its input.
- Every action has explicit evidence and authorization.

### Rollback risk

- Low; plan remains unused by production flow until Phase 4.

## Phase 4: Guarded Application

### Work

- Apply authorized plans through existing placement retry adjustments.
- Verify scope, budget, retry key, expected invariant, component mobility, and
  hard constraints before application.
- Revalidate the adjusted placement request and invariant fingerprint.
- Capture before/after placement hashes and adjustment summaries.
- Reject all reserved routing-only actions.

### Likely files

- `internal/designworkflow/autonomous_correction.go`
- `internal/designworkflow/placement_routing_retry.go`
- focused tests.

### Tests

- Spacing, fanout, distance, and region-preserving application.
- Fixed-component, fingerprint, duplicate-key, and unsupported-action rejection.
- Original request immutability.

### Acceptance

- No protected invariant changes during application.
- Rejected plans cannot reach placement or routing.

### Rollback risk

- Medium; shared retry orchestration changes. Preserve legacy behavior with
  explicit regression tests.

## Phase 5: Generic Workflow And Artifact Integration

### Work

- Enable a three-attempt correction budget when lowering a valid generic graph.
- Feed correction plans into the existing placement-routing loop.
- Enforce repeated diagnostic retry keys.
- Attach the typed correction report to routing-stage evidence.
- Write `.kicadai/autonomous-correction.json` only for the generic provider
  lane.
- Include the artifact in AI status and workflow artifact inventories.

### Likely files

- `internal/circuitgraph/design_request.go`
- `internal/designworkflow/placement_routing_retry.go`
- `cmd/kicadai/ai_graph_design.go`
- CLI and workflow tests.

### Tests

- Generic lowering enables bounded correction.
- Non-generic workflows retain previous behavior.
- Artifact is written for routed, corrected, and fail-closed generic results.
- Artifact paths are project-relative and stable.

### Acceptance

- Generic correction is active without project or fixture identity checks.
- Attempt evidence is persisted even when no correction is needed.

### Rollback risk

- Medium; provider-backed generic output gains an artifact and retry behavior.

## Phase 6: Synthetic Category And Stop Tests

### Work

- Cover each supported category and action.
- Cover no diagnostics, no authorized action, ambiguity, repeated key,
  repeated placement, invariant mismatch, budget exhaustion, context
  cancellation, regression, and non-improvement.
- Prove best-attempt selection remains stable.

### Likely files

- `internal/designworkflow/autonomous_correction_*_test.go`
- existing retry tests where integration assertions belong.

### Tests

- Focused designworkflow suite.
- Race-safe deterministic repeated runs where practical.

### Acceptance

- Every automatic stop has a structured reason.
- No test depends on absolute output paths or provider credentials.

### Rollback risk

- Low; test-only except for evidence fields uncovered by tests.

## Phase 7: Identity-Neutral Stress Fixture

### Work

- Derive a small generic graph stress case from existing catalog-resolved
  semantics.
- Perturb relative placement or spacing to produce one supported initial
  failure.
- Prove automatic correction improves and selects the passing attempt.
- Assert graph/request invariant identity and deterministic repeated output.
- Avoid fixture checks in production code.

### Likely files

- `internal/designworkflow/testdata/generic_autonomous_correction/`
- focused fixture harness/tests.
- optional provider promotion metadata only if the existing harness requires
  it; do not add a public topology reference.

### Tests

- Offline stress recovery.
- Repeated-run equivalence.
- Optional KiCad-backed stress validation.

### Acceptance

- The stress case recovers within one correction and passes required internal
  gates.
- Production behavior is identity-neutral.

### Rollback risk

- Medium; fixture geometry can be brittle. Assert evidence and invariants, not
  exact coordinates beyond the intended perturbation.

## Phase 8: Existing Fixture Regression And KiCad Evidence

### Work

- Run all recorded generic provider fixtures.
- Run the optional KiCad-backed promotion suite with configured local KiCad and
  library roots.
- Confirm correction artifacts are deterministic and existing pass status is
  preserved.
- Fix only generic behavior exposed by evidence.

### Likely files

- Existing fixture metadata/tests only when evidence fields must be declared.

### Tests

- Recorded provider tests.
- Optional `TestAIProviderOptionalKiCadPromotion` lane.
- Writer, route completion, ERC/DRC, and round-trip evidence.

### Acceptance

- Existing RC, protected LED, BMP280, LMV321, dual LMV321, and LM358 fixtures
  remain pass.
- The stress case has clean configured KiCad evidence.

### Rollback risk

- High if a shared behavior regresses. Compare canonical circuit projections,
  lowered requests, placement summaries, transaction operations, and route
  completion evidence across repeated baseline/correction-capable runs. Revert
  experiments rather than weakening fixture gates. Do not use raster image
  comparisons as electrical or geometry evidence.

## Phase 9: CLI Documentation And Roadmap

### Work

- Document automatic correction behavior, scope, evidence artifact, and stop
  conditions.
- Add artifact inspection examples using the compiled `kicadai` binary.
- Update the agent skill, README status, and roadmap.
- Record routing-only actions that remain unsupported.

### Likely files

- `docs/ai-generation.md`
- `docs/kicadai-agent-skill.md`
- `README.md`
- `specs/ROADMAP.md`
- this plan's status section.

### Tests

- Documentation link/path checks.
- `go test ./...`
- `make lint`.

### Acceptance

- Agents can distinguish provider retry, engineering correction, deterministic
  repair, and manual review.

### Rollback risk

- Low; documentation only.

## Final Verification

Run:

```sh
go test ./internal/designworkflow -count=1
go test ./internal/circuitgraph -count=1
go test ./cmd/kicadai -count=1
go test ./...
make lint
```

When local KiCad and library roots are available:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols \
KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints \
  go test ./cmd/kicadai -run '^TestAIProviderOptionalKiCadPromotion$' -count=1 -v
```

## Status

- [x] Phase 1 complete
- [ ] Phase 2 complete
- [ ] Phase 3 complete
- [ ] Phase 4 complete
- [ ] Phase 5 complete
- [ ] Phase 6 complete
- [ ] Phase 7 complete
- [ ] Phase 8 complete
- [ ] Phase 9 complete
