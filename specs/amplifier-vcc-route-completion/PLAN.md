# Amplifier VCC Route Completion Plan

Date: 2026-07-06

## Phase 1: Lock Current VCC Failure Snapshot

Goal: preserve the current protected amplifier VCC blocker before changing route
selection or contact proof.

Tasks:

1. Add or extend focused design workflow tests for
   `class_ab_headphone_protected`.
2. Assert:
   - routing runs and blocks;
   - `skip_routing` is false;
   - local route connectivity resolves all 40 endpoints;
   - required-net classification has 7 required nets, 6 complete, 1 partial;
   - VCC is the only partial required net;
   - VCC missing endpoint IDs contain `output.3`;
   - route-tree repair hints include only VCC;
   - downstream stages are skipped because routing did not complete.
3. Capture stable branch evidence for the failing VCC branch:
   - branch index;
   - selected access roles;
   - selected endpoint IDs, refs/pads, layer, and coordinates;
   - primary blocker code/message.
4. Keep the fixture `expected_fail`.

Acceptance:

- The test fails if the first blocker regresses to placement, schematic
  electrical, project write, validation, or generic KiCad evidence.
- The test fails if route-tree repair evidence broadens beyond VCC.

Review and commit:

- Run focused design workflow routing tests.
- Run `prism review staged`.
- Commit as `Lock amplifier VCC route blocker evidence`.

## Phase 2: Add Missing Endpoint Traceability

Goal: make `output.3` resolvable from the routing summary without manually
inspecting transaction payloads.

Tasks:

1. Use Atlas to locate route-tree contact graph, branch evidence, endpoint
   access, and repair summary code paths.
2. Add compact missing-endpoint trace evidence to the routing stage summary:
   - net;
   - missing endpoint ID;
   - instance/ref/pad when known;
   - expected point/layer;
   - contact graph component status;
   - nearest emitted same-net copper or local-route anchor.
3. Ensure evidence is deterministic and sorted.
4. Add unit tests for summary JSON shape and missing endpoint ordering.
5. Extend the protected amplifier test to assert `output.3` trace evidence.

Acceptance:

- The CLI/report path can identify what `output.3` means.
- Missing endpoint trace is compact and stable across runs.
- Existing route-tree contact graph tests still pass.

Review and commit:

- Run route-tree contact graph and design workflow tests.
- Run `prism review staged`.
- Commit as `Trace amplifier VCC missing endpoint`.

## Phase 3: Audit VCC Access Pair Selection

Goal: determine whether a legal VCC access pair exists but is not selected or
not attempted.

Tasks:

1. Add test helpers that extract VCC branch attempt evidence from the protected
   fixture run.
2. Compare candidate pairs for the failed branch:
   - pad-to-pad;
   - pad-to-local-route-anchor;
   - pad-to-same-net-copper;
   - local-route-anchor-to-pad.
3. Verify ranking prefers:
   - exact required endpoints;
   - same-net proven graph components;
   - candidates with known layer and no net mismatch;
   - shorter legal path only after graph correctness.
4. Patch ranking if it prefers a same-net candidate that cannot prove
   `output.3`.
5. Preserve bounded attempt limits and deterministic ordering.

Acceptance:

- VCC branch evidence explains whether the selected pair is best available.
- Any ranking change improves or preserves I2C route-tree completion tests.
- Fixed-net skip notices remain informational, not repair blockers.

Review and commit:

- Run routing, route-tree branch, I2C route-tree, and amplifier fixture tests.
- Run `prism review staged`.
- Commit as `Audit amplifier VCC access ranking`.

## Phase 4: Complete or Repair the VCC Branch

Goal: either connect `output.3` legally or produce a precise repair action.

Tasks:

1. If access ranking exposes a legal path, update branch route generation to use
   it.
2. If the route needs an intermediate legal same-net anchor, split the branch
   into deterministic sub-branches and prove contact graph completion.
3. If the route is blocked by placement, emit a repair hint with:
   - category;
   - affected refs;
   - affected net;
   - candidate pair;
   - blocker code/message.
4. If via/layer policy blocks the path, report an explicit
   `allow_layer_or_via` hint without silently changing rules.
5. Add regression tests for both solved and fail-closed behavior where feasible.

Acceptance:

- Best case: VCC proves 5 of 5 endpoints, all seven required nets are complete,
  and routing no longer blocks the amplifier fixture.
- Fallback: VCC remains blocked, but the repair hint is precise enough for an AI
  workflow to attempt placement/routing repair.
- No same-net contact is inferred from approximate overlap or wrong-net copper.

Review and commit:

- Run routing, placement retry, design workflow, and board validation tests.
- Run `prism review staged`.
- Commit as `Complete amplifier VCC route branch`.

## Phase 5: Advance Promotion Handoff

Goal: when VCC completes, let downstream evidence run; when it does not,
preserve the correct route-completion blocker.

Tasks:

1. Rerun `class_ab_headphone_protected` after Phase 4.
2. If routing completes:
   - assert `project_write` runs;
   - assert writer correctness runs;
   - assert structural validation runs;
   - assert KiCad checks are skipped only due to missing/disabled KiCad CLI or
     run when configured.
3. If routing remains blocked:
   - assert `route_completion` remains the first promotion blocker;
   - assert downstream gates remain skipped because routing did not complete;
   - assert the repair hint is VCC-specific.
4. Update fixture metadata readiness only if promotion gates justify it.

Acceptance:

- Promotion report truthfully identifies the first remaining blocker.
- KiCad evidence is not requested before writer-level prerequisites complete.
- Missing KiCad CLI remains non-flaky.

Review and commit:

- Run focused promotion and design workflow tests.
- Run `prism review staged`.
- Commit as `Advance amplifier VCC promotion handoff`.

## Phase 6: Docs, Roadmap, and Full Regression

Goal: make documentation and status files reflect the new amplifier VCC state.

Tasks:

1. Update `README.md`, `specs/ROADMAP.md`,
   `examples/design/kicad-backed/class_ab_headphone_protected.metadata.json`,
   and `specs/amplifier-validation-routing-closeout/BASELINE.md`.
2. Document whether VCC was completed or remains a precise repair blocker.
3. If a new repair hint exists, document how AI callers should interpret it.
4. Run full regression:

   ```sh
   GOCACHE=$(pwd)/.cache/go-build go test ./...
   ```

Acceptance:

- Docs agree with tests and fixture metadata.
- Full tests pass.
- Worktree contains only intentional changes.

Review and commit:

- Run `prism review staged`.
- Commit as `Update amplifier VCC route status`.

## Implementation Notes

- Use Atlas for structural code navigation before reading source files.
- Keep fixture-specific assertions in tests, not production special cases.
- Prefer shared route-tree/contact graph fixes over amplifier-only routing
  hacks.
- Preserve fail-closed behavior for unsupported amplifier topologies and loads.
- Do not weaken I2C route-tree promotion tests while fixing amplifier VCC.
