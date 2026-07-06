# Amplifier PCB Realization Closeout Plan

## Phase 1: Baseline Failure Capture

Capture the current protected amplifier PCB realization failure as structured
test evidence before changing behavior.

Tasks:

1. Run `design create` for
   `examples/design/kicad-backed/class_ab_headphone_protected.json` into an
   ignored generated output directory.
2. Record the current stage order, blocking stage, issue paths, and endpoint
   diagnostics from the generated promotion report.
   Normalize baseline evidence before assertions by sorting stage names and
   issue paths and ignoring timestamps, absolute temporary output paths, and
   other run-local fields.
3. Add or tighten a focused test helper that extracts:
   - `pcb_realization` status and issue paths;
   - `placement` status and issue paths;
   - unresolved endpoint diagnostics;
   - skipped downstream stages.
4. Assert the current known blocker exactly enough to protect the next phases
   from false positives.
5. Verify at least one known-good fixture, such as
   `led_indicator_kicad_smoke`, still reports its existing passing/candidate
   evidence during baseline capture.
6. Audit target amplifier blocks for required placement-intent metadata,
   including signal-flow direction and edge-mounted connector intent, before
   Phase 2 changes placement behavior.

Acceptance:

- The current failure is reproducible without KiCad CLI.
- The test names the old blocker paths directly.
- No production behavior changes yet.

Review and commit:

- Run focused design workflow tests.
- Run `prism review staged`.
- Commit as `Capture amplifier PCB realization baseline`.

## Phase 2: Board Envelope and Placement Bounds

Make the protected amplifier board envelope large enough for realized block
geometry without weakening placement validation.

Tasks:

1. Trace how design requests, block realization metadata, and placement
   requests currently determine board size.
2. Choose the smallest safe implementation:
   - use explicit board dimensions in the protected amplifier fixture if the
     request schema already supports them; when explicit dimensions are
     present, no derived-growth logic may run and undersized boards must block
     instead of growing;
   - if the request schema lacks explicit board dimensions, add the narrow
     schema support needed for this fixture instead of implementing automatic
     derived-envelope growth.
3. Add tests that prove:
   - unrelated small fixtures keep their current board behavior;
   - the protected amplifier generated placements are within board bounds;
   - placement diagnostics still fail for genuinely out-of-board components.
4. Update placement summary evidence to make board envelope decisions visible
   when useful.

Acceptance:

- `class_ab_headphone_protected` no longer reports output-stage placements
  outside the board solely due to board envelope size.
- Board validation still catches deliberate outside-board regressions.
- Existing placement tests remain deterministic.

Review and commit:

- Run focused placement and design workflow tests.
- Run `prism review staged`.
- Commit as `Fit amplifier PCB realization within board envelope`.

## Phase 3: Amplifier Endpoint Binding

Resolve inter-block route endpoints for the protected amplifier fixture to
generated pads or explicit supported endpoints.

Tasks:

1. Use Atlas to locate endpoint binding, anchor discovery, and routing handoff
   code paths before reading large files.
2. Inspect unresolved endpoint diagnostics for the protected amplifier fixture:
   - output/protection `AMP_OUT` port on canonical
     `AMP_OUT_DC_BIASED`;
   - `HP_OUT`;
   - `LOAD_RET` block port on canonical `HP_RET`;
   - `LOAD_REF`;
   - gain/input boundary endpoints, which should bind to generated input
     connector/header pads for this fixture rather than virtual board-edge
     points.
3. Resolve endpoint classification before patching:
   - generated pads are required for in-board amplifier/protection/load
     components;
   - explicit board-edge endpoints are allowed only for connector/interface
     boundaries already modeled as external endpoints.
4. Patch the smallest responsible layer:
   - block PCB realization metadata when pads/ports are missing;
   - endpoint alias mapping when canonical nets are not propagated;
   - routing adapter anchor discovery when generated pads exist but are not
     found.
5. Add tests that assert each formerly unresolved endpoint now binds to:
   - block ID and exported port;
   - generated ref/pad;
   - canonical net.
6. Ensure unresolved endpoint diagnostics remain precise for unsupported future
   endpoints.

Acceptance:

- The protected amplifier fixture has no unresolved endpoint blockers for the
  previously failing inter-block connections.
- Pad net assignments match canonical composition aliases.
- No placeholder center-point routing is introduced.

Review and commit:

- Run focused blocks, routing adapter, placement, and design workflow tests.
- Run `prism review staged`.
- Commit as `Bind amplifier PCB endpoints to generated pads`.

## Phase 4: Placement-to-Routing Handoff

Allow the fixture to advance from placement into routing without losing
block-local route or net evidence.

Tasks:

1. Run the protected amplifier fixture after Phases 2 and 3 and capture the new
   first failing stage.
2. If routing starts, verify route request construction includes all intended
   inter-block nets and excludes unsupported placeholders.
3. Verify local-route coordinate translation:
   - block-local route coordinates are offset into the same global board
     coordinate system as generated pads;
   - translation applies the same rotation and mirroring transform used for
     generated footprint pads, in canonical order:
     `global = translation * rotation * mirror * local`, where mirroring is
     across the block-local Y axis, rotation is around the block's local
     origin, and translation moves that origin to the block's board position;
   - mirrored blocks also flip local-route layer assignments consistently with
     footprint pad layer mapping; for two-layer boards this maps front to back
     and back to front, while multilayer stackups require explicit layer-pair
     metadata that preserves layer functional type; absent that metadata,
     mirrored multilayer local routes are unsupported diagnostics rather than
     guessed reflections;
   - changing board dimensions does not shift local-route evidence away from
     its footprint anchors;
   - endpoint anchors and local-route contact evidence agree on the same global
     coordinates.
4. Add tests that assert:
   - old `pcb_realization.output` and placement blocker paths are absent;
   - routing receives concrete endpoint anchors for amplifier nets;
   - local-route evidence for output/protection blocks is preserved.
5. If routing legitimately blocks, classify the new blocker as routing with
   actionable diagnostics rather than placement or endpoint realization.

Acceptance:

- The fixture advances beyond placement.
- The next blocker, if any, is routing, writer correctness, board validation,
  or KiCad evidence and is reported accurately.
- Existing route-tree/contact graph tests continue to pass.

Review and commit:

- Run focused design workflow and routing tests.
- Run `prism review staged`.
- Commit as `Advance amplifier fixture past PCB placement`.

## Phase 5: Metadata, Roadmap, and Readiness Updates

Update project documentation and fixture metadata to describe the new status.

Tasks:

1. Update `class_ab_headphone_protected.metadata.json` expected stages and
   known gaps to match the new first blocker.
2. Update `examples/design/kicad-backed/README.md`.
3. Update `README.md`, `docs/ai-readiness.md`, and `specs/ROADMAP.md` only
   where status text changes.
4. Remove stale references to placement/endpoint-realization as current
   blockers if they are resolved.
5. Ensure future fabrication requirements remain explicitly documented:
   active output fault protection, load safety, thermal/SOA evidence, analog
   stability/layout proof, and KiCad ERC/DRC-clean evidence.

Acceptance:

- Docs and metadata agree on the current blocker.
- No stale text claims the fixture stops at schematic electrical validation,
  PCB placement, or endpoint realization once those are fixed.

Review and commit:

- Run metadata/promotion classification tests.
- Run `prism review staged`.
- Commit as `Update amplifier PCB realization status`.

## Phase 6: Full Regression

Run the complete regression suite and verify the worktree is clean.

Tasks:

1. Run:
   ```sh
   mkdir -p .cache/go-build
   GOCACHE=$(pwd)/.cache/go-build go test ./...
   ```
2. If optional KiCad CLI is configured, optionally run the protected fixture
   once with KiCad checks enabled and record the result in notes or metadata
   only if deterministic.
3. Check `git status --short`.
4. Fix any regression or commit any missed tracked updates after Prism review.

Acceptance:

- `go test ./...` passes.
- No uncommitted tracked changes remain.
- Generated ignored output may remain ignored.

Review and commit:

- Run `prism review staged` only if changes are needed.
- Commit any final fixes with a precise message.

## Implementation Notes

- Use Atlas for structural questions before reading source files:
  - endpoint binding;
  - placement request construction;
  - design workflow stage classification;
  - routing adapter handoff.
- Prefer existing metadata paths over fixture-specific special cases.
- Keep generated fixture output under ignored directories such as
  `examples/.generated/`.
- Do not relax validation rules to advance the fixture; resolve the underlying
  board envelope or endpoint binding evidence.
- Commit after each phase that changes files.
