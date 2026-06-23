# Generated Placement Mobility Implementation Plan

## Objective

Allow bounded placement-routing retry to move eligible generated block-local
placements while preserving hard constraints, local-route intent, and generated
connectivity evidence.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep retry opt-in; disabled retry must preserve existing generated output.
- Keep imported, stale, malformed, or unsupported targets immovable.
- Preserve deterministic output and stable JSON evidence.
- Prefer extending existing placement, design workflow, routing retry, and
  transaction models over adding parallel paths.
- Default tests must not require KiCad CLI, network access, or global KiCad
  libraries.

## Phase 1: Mobility Model And Policy Normalization

### Goal

Introduce typed mobility semantics without changing retry behavior yet.

### Work

- Add placement mobility types near the existing placement model:
  - `fixed`;
  - `group_transform`;
  - `local_rebuild`;
  - `soft_preferred`;
  - `unowned`.
- Add route handling policy values:
  - `transform_with_group`;
  - `invalidate_and_rebuild`;
  - `preserve_fixed`;
  - `unsupported`.
- Add normalized `MobilityPolicy` or equivalent model carrying:
  - ref;
  - group ID;
  - owner scope;
  - class;
  - reason;
  - allowed transforms;
  - route handling policy;
  - hard constraints.
- Make default normalization conservative:
  - imported or unknown ownership becomes `unowned`;
  - explicit fixed refs stay `fixed`;
  - generated block-local refs default to current fixed behavior until later
    phases opt them into mobility.
- Expose mobility summaries from placement results without affecting placement
  coordinates.

### Tests

- Unit tests for default mobility classification.
- Unit tests proving unknown/imported content is `unowned`.
- Unit tests proving explicit fixed refs remain `fixed`.
- Snapshot-free tests for deterministic mobility summary ordering.
- Existing placement and retry tests continue passing.

### Acceptance

- The codebase has an explicit mobility model and conservative evidence, but no
  generated placement movement behavior has changed.

### Commit

```text
Add generated placement mobility policy model
```

## Phase 2: Ownership Hydration From Generated Blocks And Transactions

### Goal

Populate mobility policy from generated workflow, block realization, and
transaction ownership evidence.

### Work

- Trace generated placement participants from block realization into placement
  requests.
- Attach owner scope such as block ID, instance ID, transaction operation ID, or
  workflow stage.
- Mark eligible generated refs as:
  - `group_transform` when the block fragment has relative positions and
    transformable local routes;
  - `local_rebuild` when routes can be safely regenerated from hydrated pads;
  - `soft_preferred` when placement is generated but not route-critical;
  - `fixed` when the request or block declares hard mechanical placement.
- Keep connector edge anchors, mounting holes, and explicit fixed refs hard
  fixed.
- Surface blocked ownership reasons for refs that cannot be moved.

### Tests

- Generated block realization produces mobility metadata for every ref.
- Connector/edge constraints remain fixed.
- Local support parts receive expected mobility class.
- Missing pad or geometry evidence prevents mobility.
- Existing generated examples remain deterministic when retry is disabled.

### Acceptance

- Generated workflow outputs carry enough ownership metadata for retry to know
  which refs are candidates and which are blocked.

### Commit

```text
Hydrate generated placement mobility ownership
```

## Phase 3: Local Route Classification And Invalidation

### Goal

Classify generated local routes so movement either transforms, rebuilds, or
blocks safely.

### Work

- Identify route operations associated with generated block-local nets.
- Classify each local route as:
  - transformable with a group;
  - rebuildable after movement;
  - fixed/preserved;
  - unsupported.
- Add an invalidation path that removes rebuildable route operations before
  re-routing.
- Ensure invalidated routes are regenerated from current hydrated pad
  positions.
- Add validation that stale route geometry is not written after movement.
- Add local-route handling counts to mobility evidence.

### Tests

- Transformable group route moves with group translation.
- Rebuildable route is removed and regenerated after ref movement.
- Unsupported route blocks movement with a structured issue.
- Stale route geometry is detected in tests.
- Net names and pad assignments survive route invalidation/rebuild.

### Acceptance

- Retry has a safe route-handling mechanism for moving generated placements.

### Commit

```text
Classify generated local routes for placement mobility
```

## Phase 4: Retry Candidate Filtering And Movement Application

### Goal

Make the placement-routing retry loop consume mobility policy when building and
applying adjustments.

### Work

- Filter retry hint candidates by mobility class and route policy.
- Allow `group_transform` candidates to move as a group.
- Allow `local_rebuild` and `soft_preferred` candidates to move independently
  when constraints permit.
- Preserve `fixed` and `unowned` refs exactly.
- Add stop reasons for:
  - no movable generated candidates;
  - local route unsupported;
  - hard constraint would be violated;
  - missing pad or geometry evidence.
- Extend repeated-placement detection to include group movement.
- Include moved refs, moved groups, blocked refs, and route handling counts in
  retry summary evidence.

### Tests

- Retry moves eligible generated refs when hints target them.
- Retry moves all refs in a `group_transform` group together.
- Fixed and unowned refs remain unchanged.
- No-safe-movement stop is reported when every candidate is blocked.
- Best-attempt selection keeps the best previous attempt after failed movement.

### Acceptance

- The retry loop can move generated placements safely and explains why it did
  or did not move candidates.

### Commit

```text
Move eligible generated placements during retry
```

## Phase 5: Generated Fixture Corpus

### Goal

Prove generated movement behavior through deterministic full-board fixtures.

### Work

- Add or update fixtures under
  `internal/designworkflow/testdata/full_board_retry`.
- Include fixture metadata for:
  - expected mobility classes;
  - expected moved refs or groups;
  - expected blocked refs;
  - local route handling counts;
  - expected improvement metric or stop reason.
- Add required fixture classes:
  - group transform improvement;
  - local rebuild improvement;
  - hard constraint preservation;
  - no safe movement boundary;
  - multi-block generated board.
- Centralize evidence extraction helpers so tests assert selected stable fields.

### Tests

- At least one generated fixture improves after moving generated content.
- At least one fixture moves a group while preserving relative placement.
- At least one fixture invalidates and rebuilds local routes.
- At least one fixture proves hard constraints are preserved.
- At least one fixture proves no-safe-movement boundary behavior.

### Acceptance

- Generated full-board retry evidence proves actual movement, not merely pad
  hydration or diagnostic boundaries.

### Commit

```text
Add generated placement mobility fixtures
```

## Phase 6: CLI Evidence Contract

### Goal

Expose stable mobility evidence to AI callers through `design create --json`.

### Work

- Add selected mobility fields to existing retry or routing stage summaries.
- Avoid large full-output snapshots.
- Add CLI selected-field tests for:
  - retry enabled;
  - moved ref count;
  - moved group count;
  - blocked ref count;
  - local route transformed/rebuilt/blocked counts;
  - final routing or validation status;
  - no-safe-movement stop reason.
- Normalize output paths and volatile IDs in tests.

### Tests

- CLI generated movement fixture reports expected mobility fields.
- CLI no-safe-movement fixture reports stable blocker evidence.
- Existing CLI retry tests remain compatible.

### Acceptance

- AI callers can inspect why retry moved generated placements or why movement
  was blocked.

### Commit

```text
Expose generated placement mobility retry evidence
```

## Phase 7: Validation, Documentation, And Roadmap

### Goal

Close the milestone with full validation and updated project guidance.

### Work

- Run full default validation:
  - `go test ./...`
- Update README:
  - describe generated placement mobility classes;
  - describe retry movement evidence;
  - state that retry is still opt-in and not fabrication readiness.
- Update `specs/ROADMAP.md`:
  - mark generated placement mobility as implemented;
  - move the next near-term priority to fabrication output validation.
- Add implementation notes to this plan if scope narrows during execution.

### Tests

- `go test ./...`
- Prism review staged documentation/code.

### Acceptance

- Documentation and roadmap match the implemented generated mobility behavior.

### Commit

```text
Document generated placement mobility
```

## Final Verification

After all phases:

```sh
go test ./...
```

Run Prism on the final staged changes and resolve all high/medium actionable
findings before the final commit.

## Expected Final State

- Generated placement participants carry explicit mobility policy.
- Retry moves eligible generated refs or groups while preserving hard
  constraints.
- Local generated routes are transformed, rebuilt, preserved, or blocked
  explicitly.
- Generated full-board fixtures prove actual movement improvement and safe
  no-movement boundaries.
- CLI JSON exposes stable mobility evidence for AI repair and planning loops.
- Existing default generated output remains unchanged when retry is disabled.
