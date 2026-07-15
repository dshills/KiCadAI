# External Agent Workflow Hardening Plan

## Objective

Remove the generic provider, catalog, split-supply, multi-unit schematic,
diagnostic, replay, and overwrite-safety blockers exposed by an independent AI
agent. Prove the result with one recorded split-supply analog graph without
adding topology-specific production behavior.

## Implementation Rules

- Start from the current committed baseline and inspect the existing
  uncommitted prototype patches before editing.
- Do not commit the current patches as one batch.
- Preserve user-owned work and split accepted changes by phase.
- Fix root causes before adding compatibility fallbacks.
- Keep unexplained route-anchor mismatches fail-closed.
- Reproduce overwrite loss before changing overwrite implementation.
- Keep default tests offline, deterministic, and credential-free.
- Run focused tests after every phase.
- Run affected provider-backed and KiCad-backed fixtures after shared changes.
- Review staged changes with Prism before every commit.
- Commit each completed phase separately.

## Phase 1: Baseline, Patch Triage, And Reproduction Assets

### Work

- Record the current repository test and lint baseline.
- Inventory every uncommitted external-agent change and classify it as:
  accepted, needs revision, superseded, or unproven.
- Preserve the external agent's provider response/graph as a sanitized local
  diagnostic input if available.
- Add minimal failing tests for each confirmed issue before accepting fixes.
- Add a reproduce-first test harness for overwrite preservation.
- Confirm exact installed/source KiCad symbol locations for the BJT records.

### Likely files

- `specs/external-agent-workflow-hardening/SPEC.md`
- `specs/external-agent-workflow-hardening/PLAN.md`
- focused test files in `internal/components`, `internal/circuitgraph`,
  `internal/aiprovider`, and `internal/kicadfiles/designapi`
- CLI integration test helpers under `cmd/kicadai`

### Tests

- `go test ./... -count=1`
- `make lint`
- targeted tests that fail on the committed baseline for each confirmed defect.

### Acceptance

- Every proposed code change has a reproducing test or is explicitly marked
  unconfirmed.
- The nonuniform waypoint fallback is not accepted.
- No user-owned patch is lost.

### Rollback risk

- Low. This phase is tests, classification, and evidence capture.

## Phase 2: Catalog Identity And Library Preflight

### Work

- Correct MMBT3904/MMBT3906 symbol IDs to `Transistor_BJT`.
- Correct all associated pinmap and verification evidence strings.
- Add resolver-backed checks to `component validate` using library roots/cache.
- Validate all selectable symbols, units, pins, footprints, and pads.
- Aggregate catalog findings deterministically.
- Ensure blocked placeholders remain deliberately excluded.
- Regenerate only catalog-hash-dependent goldens.

### Likely files

- `data/components/verified_active.json`
- `internal/components/*`
- `internal/libraryresolver/*` only if reusable queries are missing
- `cmd/kicadai/main.go`
- `cmd/kicadai/main_test.go`
- affected circuitgraph golden files
- `docs/libraries-and-components.md`

### Tests

- BJT symbol and pinmap identity regression tests.
- Missing symbol, missing footprint, missing pin, and missing pad tests.
- Aggregate/deterministic catalog diagnostic tests.
- `go test ./internal/components ./internal/libraryresolver ./cmd/kicadai -count=1`.
- Run catalog validation against the configured KiCad symbol/footprint roots.

### Acceptance

- BJT records resolve against KiCad library evidence.
- Evidence strings agree with selected library identities.
- Catalog validation catches the original error before provider execution.

### Rollback risk

- Medium. Catalog hash changes affect many deterministic fixtures.

## Phase 3: Verified Polarized Capacitor Path

### Work

- Select one concrete or narrowly verified polarized capacitor/package pair.
- Verify symbol pins, footprint pads, polarity, package, and voltage evidence.
- Add explicit review-required evidence for unproven operating limits.
- Keep the existing generic placeholder draft-only.
- Improve selection diagnostics to name placeholder confidence and missing
  evidence before generation proceeds.

### Likely files

- `data/components/passives.json` or a focused verified-passive catalog file
- `internal/components/*_test.go`
- component selection diagnostics
- `docs/libraries-and-components.md`

### Tests

- Positive/negative pin-to-pad mapping.
- Connectivity-level selection succeeds for the verified record.
- Fabrication-level selection remains blocked when required rating evidence is
  incomplete.
- Placeholder-only alternatives produce actionable diagnostics.

### Acceptance

- A generic graph needing a polarized coupling/bulk capacitor can pass catalog
  and connectivity gates using trusted evidence.
- No analog lifetime/ripple/ESR claim is inferred.

### Rollback risk

- Medium. Incorrect polarity evidence would be electrically dangerous.

## Phase 4: Explicit Split-Supply Graph Semantics

### Work

- Add `power_negative` and the `lower` lane value to the circuit graph model.
- Update strict provider JSON Schema and capability examples.
- Add normalization, hashing, semantic projection, and lowering support.
- Require the lane for new graphs containing `power_neg` nets.
- Add deterministic compatibility inference only when a legacy captured graph
  contains exactly one distinct `power_neg` net; reject multiple implicit
  negative rails as ambiguous.
- Remove or reduce the prototype-only implicit derivation once the explicit
  contract is available.

### Likely files

- `internal/circuitgraph/model.go`
- `internal/circuitgraph/schema.go`
- `internal/circuitgraph/parse.go`
- `internal/circuitgraph/validate.go`
- `internal/circuitgraph/schematic.go`
- `internal/circuitgraph/*_test.go`
- provider capability/schema fixtures
- `docs/ai-generation.md`

### Tests

- Strict decode accepts `power_negative: lower`.
- Unknown and contradictory lane values fail.
- A `power_neg` graph without the field fails with one actionable diagnostic,
  or produces the documented compatibility warning for legacy input.
- Lowering emits schematic IR `lanes.power_negative`.
- Positive, signal, negative, and ground ordering remains deterministic.
- Existing non-split fixtures remain byte-stable except expected schema hashes.

### Acceptance

- A provider can express every lane required by the downstream IR.
- Split-supply failure is caught during graph validation, not schematic write.

### Rollback risk

- Medium. Provider schemas and recorded fixture hashes may change.

## Phase 5: Canonical Multi-Unit Pin Anchors

### Work

- Reproduce the LM358 later-unit grid/anchor mismatch with the smallest test.
- Trace the router and builder transform pipelines against KiCad symbol data.
- Define one canonical unit-aware transform and snapping boundary.
- Refactor router and builder to consume the same anchor calculation/table.
- Remove silent nonuniform fallback behavior.
- Decide whether uniform compatibility translation remains necessary; if kept,
  emit structured repair evidence and revalidate readability.
- Add self-loop and cross-unit route regressions.

### Likely files

- `internal/schematiclayout/route.go`
- `internal/schematiclayout/geometry.go`
- `internal/schematiclayout/*_test.go`
- `internal/kicadfiles/designapi/builder.go`
- `internal/kicadfiles/designapi/builder_test.go`
- `internal/schematicir/adapter.go` if anchor tables are created there
- symbol resolver transform helpers

### Tests

- Unit A/B/power anchors at all supported rotations and mirrors.
- On-grid and off-grid symbol positions.
- Self-loop and same-package cross-unit nets.
- Correct waypoint termination without repair.
- Diagonal, duplicate, nonuniform, and stale waypoints remain fail-closed.
- Deterministic transaction and schematic output.
- KiCad schematic parse and optional ERC for the multi-unit fixture.

### Acceptance

- Router endpoints equal builder anchors exactly.
- The original LM358 write failure is fixed without discarding route geometry.
- No direct-route fallback bypasses readability validation.

### Rollback risk

- High. This changes a shared schematic geometry contract.

## Phase 6: Provider Budget Configuration And Diagnostics

### Work

- Replace the single hidden token constant with profile-aware defaults.
- Add bounded CLI/environment configuration.
- Include the selected limit in sanitized request/attempt evidence.
- Surface provider incomplete reason, limit, and usage in structured CLI issues.
- Provide retry guidance without automatically creating an unbounded cost loop.
- Add background-mode coverage for incomplete responses.

### Likely files

- `internal/aiprovider/openai.go`
- `internal/aiprovider/model.go`
- `internal/aiprovider/openai_test.go`
- `cmd/kicadai/main.go`
- `cmd/kicadai/ai_graph_design.go`
- CLI docs and agent skill

### Tests

- Profile defaults for bounded and generic schemas.
- CLI/env precedence and bounds.
- Request serialization includes selected limit.
- `max_output_tokens` returns an actionable structured issue.
- Secrets remain redacted.
- Background and foreground behavior agree.

### Acceptance

- Moderately complex generic graphs have a sufficient bounded default.
- Token exhaustion tells an agent exactly what happened and how to retry.

### Rollback risk

- Medium. Larger defaults affect provider cost and latency.

## Phase 7: First-Class Provider Capture And Replay

### Work

- Define a versioned sanitized replay artifact accepted by the recorded
  provider.
- Persist it immediately after provider envelope decode, before downstream
  validation.
- Sanitize in memory before persistence; exclude all raw system, developer,
  user, tool, and correction prompt text as well as credentials and headers.
- Make preflight failures retain the artifact safely.
- Add a stable artifact path and exact replay command to CLI output.
- Add semantic projection comparison for capture versus replay.
- Ensure replay performs no network request.

### Likely files

- `internal/aiprovider/recorded.go`
- `internal/aiprovider/model.go`
- `cmd/kicadai/ai_graph_design.go`
- artifact writers under `cmd/kicadai`
- provider tests and CLI integration tests
- `docs/ai-generation.md`
- `docs/kicadai-agent-skill.md`

### Tests

- Successful live-like response writes a directly replayable artifact.
- Structurally invalid graph is still replayable for deterministic correction.
- Replay graph and critical projection match capture.
- Artifact serialization is deterministic and secret-free.
- Raw prompt sentinels never appear in success or failure artifacts.
- Recorded replay uses a network client that fails if called.

### Acceptance

- No `jq` conversion is required.
- One provider call can be followed by unlimited deterministic offline runs.

### Rollback risk

- Medium. Artifact compatibility becomes a supported user contract.

## Phase 8: Aggregated Preflight Diagnostics

### Work

- Formalize graph preflight stages and root/dependent issue relationships with
  stable issue IDs and explicit root-cause IDs.
- Continue independent validation when partial data is safe.
- Suppress only diagnostics explicitly linked to unresolved root components;
  never infer dependency from message text or path proximity.
- Return all eligible diagnostics to bounded provider correction attempts.
- Add retry-scope and suggested-action fields where missing.
- Preserve stable ordering and deduplication.

### Likely files

- `internal/circuitgraph/validate.go`
- `internal/circuitgraph/resolve.go`
- `internal/circuitgraph/issues.go`
- `cmd/kicadai/ai_graph_design.go`
- `internal/aiprovider` diagnostic adapters

### Tests

- One graph with independent component, net, layout, and policy errors.
- Root/dependent issue suppression.
- Stable ordering under shuffled input.
- Full diagnostic set reaches the next bounded provider attempt.
- Diagnostic byte/entry limits remain enforced.

### Acceptance

- One preflight run reports every safe independent blocker.
- Diagnostics stay concise enough for provider correction and human review.

### Rollback risk

- High. Partial resolution can produce misleading cascades if dependency
  tracking is incorrect.

## Phase 9: Overwrite Preservation Matrix

### Work

- Build a known-good managed project fixture.
- Inject failures at provider, graph, catalog, schematic, routing, writer,
  artifact, timeout, and cancellation boundaries.
- Hash core artifacts before and after every failed `--overwrite` run.
- Reproduce the metadata-only report if possible.
- If reproduced, route all replacement through the atomic writer commit and
  isolate failed-attempt evidence from current-project metadata.
- Prefer native atomic directory exchange where available; otherwise implement
  a journaled move-aside/replace/restore protocol and startup recovery.
- Retain at most the five newest failed-attempt evidence directories per output
  by default, pruning oldest attempts only after new evidence is durable.
- If not reproduced, retain the preservation matrix without speculative writer
  changes.

### Likely files

- `cmd/kicadai/ai_design_test.go`
- `cmd/kicadai/ai_graph_design_test.go`
- `internal/kicadfiles/design/write.go`
- `internal/kicadfiles/design/write_test.go`
- artifact-writing helpers
- manifest/provenance helpers if attempt namespacing is required

### Tests

- Byte-for-byte core artifact preservation for every injected failure.
- Successful overwrite replaces all project files and metadata together.
- Interrupted commit restores the old project.
- Startup recovery completes or rolls back every journal state.
- No journal, staging, or backup directories remain after success.
- Recovery evidence remains available without becoming the current manifest.
- Failure-evidence retention never exceeds the configured bound.

### Acceptance

- A failed overwrite cannot turn a complete project into a metadata-only
  directory.
- Any implementation change is tied to a reproduced failure.

### Rollback risk

- High. Filesystem commit and recovery behavior is safety-critical.

## Phase 10: Split-Supply Analog Regression Fixture

### Work

- Add a recorded identity-neutral graph combining split supply, multi-unit
  op-amp, verified BJT, polarized capacitor, and feedback connectivity.
- Add intentionally broken variants for aggregate diagnostics.
- Run the graph through catalog resolution, schematic IR, placement, routing,
  writer, and checks.
- Keep analog performance evidence `review_required`.
- Fix only generic blockers exposed by this fixture.
- Add a focused regression for the reported GND via-to-through-hole-pad DRC
  geometry only if it recurs in deterministic output.

### Likely files

- `examples/ai/generic_split_supply_analog_stage/*`
- `internal/aiprovider/*_test.go`
- `internal/circuitgraph/*_test.go`
- `internal/designworkflow/*_test.go`
- optional KiCad promotion metadata

### Tests

- Recorded provider strict decode and replay.
- Catalog and library resolution.
- Split-supply schematic layout and multi-unit anchors.
- Internal electrical/connectivity checks.
- Required-net route completion.
- Writer correctness and normalized round-trip.
- Optional KiCad ERC and strict DRC.
- Repeated-run determinism.
- Existing generic RC, LED, BMP280, LMV321, dual LMV321, and LM358 fixtures.

### Acceptance

- The regression reaches its declared KiCad-backed candidate/pass level.
- No production code checks fixture identity, prompt text, or project path.
- Analog limitations remain explicit.

### Rollback risk

- Medium. The fixture may expose additional generic geometry defects; those
  must be handled without scope expansion.

## Phase 11: Documentation, Agent Skill, And Roadmap

### Work

- Document catalog preflight before live provider use.
- Document token-budget selection and cost implications.
- Document live capture followed by recorded replay.
- Document aggregated diagnostics and retry scopes.
- Document overwrite guarantees and recovery locations.
- Update the agent skill with the shortest deterministic workflow.
- Update project status and roadmap with proven support and remaining limits.

### Likely files

- `README.md` only for concise links/status
- `docs/ai-generation.md`
- `docs/cli-reference.md`
- `docs/libraries-and-components.md`
- `docs/kicadai-agent-skill.md`
- `docs/project-status.md`
- `specs/ROADMAP.md`

### Tests

- Documentation command smoke tests where available.
- `rg` check that examples use `kicadai`, not `go run ./cmd/kicadai`.
- Link/path checks.
- Full `go test ./... -count=1` and `make lint`.
- Optional provider-backed and KiCad-backed suites.

### Acceptance

- An independent agent can preflight, call once, capture, replay, diagnose, and
  recover using documented commands.
- Roadmap status distinguishes structural support from analog/fabrication proof.

### Rollback risk

- Low. Documentation reflects already proven behavior.

## Final Completion Gate

Before declaring the project complete:

1. Run `go test ./... -count=1`.
2. Run `make lint`.
3. Run catalog validation against configured KiCad libraries.
4. Run all recorded generic provider fixtures.
5. Run optional KiCad-backed promotion fixtures.
6. Run the optional live provider capture once and replay it offline.
7. Run the overwrite preservation matrix.
8. Review staged changes with Prism before every phase commit.
9. Confirm no unresolved Prism high findings.
10. Confirm the worktree is clean.

After each phase commit, report:

- phase completed;
- blocker fixed;
- tests and external evidence run;
- Prism status;
- next phase/blocker;
- whether independent-agent usability advanced;
- whether existing pass fixtures remained clean;
- whether the change remained generic.
