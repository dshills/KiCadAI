# Placement Model Support Plan

## Goal

Resolve currently unsupported or partially modeled placement fields by either
implementing them or explicitly reporting that they are unsupported. Silent
no-op fields should not remain in the public placement model.

## Fields To Resolve

- `Group.KeepTogether`
- `Group.MaxSpreadMM`
- `Request.Seed`
- `ExistingPlacementPolicy.PreserveFixed`
- `Metrics.EstimatedBoundsCount`

## Phase 1 - Unsupported Field Audit

1. Add tests that set each field and assert current behavior.
2. Decide for each field whether the next step is implementation or explicit
   unsupported issue reporting.
3. Document public API compatibility concerns for each decision.

Acceptance:

- No field can be silently ignored without a test naming that behavior.

## Phase 2 - `Request.Seed`

1. Confirm all tie-break paths that should consume seed.
2. Add deterministic tests showing different seeds can choose different
   otherwise-equivalent placements, while the same seed is stable.
3. Keep seed from overriding hard constraints.
4. Audit placement code for map iteration in candidate scoring, net traversal,
   group traversal, and operation generation; replace any seed-sensitive map
   iteration with sorted slices or deterministic iterators.
5. If candidate evaluation becomes concurrent, sort worker results by stable
   component/candidate IDs before selection. Do not serialize the solver merely
   because a seed is provided.
6. Perform sorting once during request normalization or candidate preparation,
   not repeatedly inside hot scoring loops.
7. Establish a placement-solver coding rule: no direct map iteration in solver
   selection logic without first sorting keys into a stable slice.
8. Add a custom AST scanner or equivalent lint check that flags `range` loops
   over maps inside solver selection code, and prefer normalized sorted slices
   or an ordered container in those paths.
9. If concurrent tasks use pseudo-randomness, derive each task sub-seed with a
   stable hash of the global `Request.Seed` and task ID.

Acceptance:

- Seed behavior is deterministic and documented.
- Seeded placement remains stable across repeated `go test` runs.

## Phase 3 - `ExistingPlacementPolicy.PreserveFixed`

1. Define how imported existing footprints map into fixed placement requests.
2. If `PreserveFixed` is true, convert existing placements into fixed occupancy.
3. If false, allow re-placement while preserving explicit user-fixed requests.
4. Explicit user-supplied component positions take precedence over the global
   `PreserveFixed` policy for that component.

Acceptance:

- Existing board placement can be preserved during AI-generated updates.

## Phase 4 - Group Constraints

1. Model `Group.KeepTogether` with a continuous barrier or penalty function that
   strongly discourages separation without switching abruptly between hard and
   soft solver modes. Emit a warning naming the group when the final placement
   violates the requested grouping.
2. Implement `Group.MaxSpreadMM` as a validation constraint first, then as a
   placement constraint if needed.
3. Define a hard failure threshold for `KeepTogether`/spread violations where
   the placement is rejected or emits a high-severity issue.
4. Add tests for pass/fail spread conditions.

Acceptance:

- Group placement failures are structured issues, not silent score changes.

## Phase 5 - Estimated Bounds Metrics

1. Increment `Metrics.EstimatedBoundsCount` when placement uses estimated,
   pad-derived, or generated bounds rather than library courtyard bounds.
2. Preserve source details in placement summaries.
3. Add tests for mixed bound sources.

Acceptance:

- Users can see when placement quality depends on estimated geometry.

## Phase 6 - Unsupported Field Reporting

1. For any field not implemented, emit a warning or validation issue naming the
   unsupported field.
2. Document the limitation in CLI output and README placement notes.

Acceptance:

- No placement request field is silently ignored.

## Non-Goals

- No autorouter changes.
- No board outline polygon packing beyond existing rectangular placement area.
- No mechanical constraint solver.
