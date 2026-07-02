# Route-Tree VCC Proof Closeout Implementation Plan

Date: 2026-07-02

## Objective

Close or precisely classify the final VCC route-tree proof gap in the
KiCad-backed I2C sensor breakout fixture. The implementation should improve
branch/access evidence first, then use that evidence to safely adjust routing,
proof, or diagnostics without weakening validation gates.

## Phase 1: Baseline VCC Failure Lock

### Goals

- Capture the current final VCC proof gap in focused regression tests.
- Make the failing branch and contact proof queryable without manual JSON
  inspection.

### Tasks

- Add a focused I2C test that asserts the current selected retry evidence:
  - 12 required route-tree endpoints;
  - 11 proven endpoints;
  - 3 complete route-tree contact-graph groups;
  - 1 partial route-tree contact-graph group;
  - VCC appears in route-tree repair or contact issues.
- Add helpers to extract routing-stage summaries by key.
- Add helpers to filter route-tree branch issues and contact proofs by net.
- Assert the remaining blocker is VCC-specific, not stale GND/SDA evidence.
- Preserve existing connector/LED and general I2C baseline tests.

### Acceptance

- The test fails if the remaining blocker regresses to GND/SDA or loses VCC
  specificity.
- The test fails if proven endpoint count drops below 11.
- `go test ./internal/designworkflow -run 'I2C|VCC|RouteTree' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Capture VCC route-tree proof gap`.

## Phase 2: Branch Access Attempt Evidence

### Goals

- Record compact evidence for each route-tree access pair attempted by a
  branch.
- Make failed VCC pair selection explainable.

### Tasks

- Extend branch evidence with bounded `access_attempts`.
- Capture for each attempted pair:
  - pair rank;
  - source/target roles;
  - source/target coordinates;
  - source/target layers;
  - route status;
  - primary issue code/message;
  - primary issue refs/nets where available.
- Add selected source/target coordinates and layers to branch evidence.
- Mark whether the selected operation was snap-exempt.
- Keep evidence bounded to the existing candidate pair limit.
- Add unit tests for:
  - first-pair success evidence;
  - multiple failed attempts;
  - selected role/coordinate fields;
  - no partial copper from failed attempts.

### Acceptance

- VCC branch evidence includes attempted access pairs and selected coordinates.
- Evidence is deterministic and compact.
- Existing branch issue paths remain stable.
- `go test ./internal/designworkflow -run 'AccessAttempt|RouteTreeBranch|VCC' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Record route-tree access attempt evidence`.

## Phase 3: Contact Graph Gap Classification

### Goals

- Distinguish the remaining VCC failure mode precisely.
- Separate route failures from proof failures.

### Tasks

- Add route-tree contact diagnostic classification for:
  - no emitted route operation;
  - emitted route did not touch target;
  - emitted route touched same-net local anchor but graph remains split;
  - snap-exempt endpoint intentionally preserved;
  - wrong-net contact;
  - wrong-layer contact.
- Add VCC-focused tests that build small contact graphs for each category.
- Include contact graph component IDs or stable component counts in diagnostic
  evidence where practical.
- Update route-tree repair hint generation to use the narrower category.

### Acceptance

- Remaining VCC issue maps to one clear category.
- Contact misses that are really graph splits are reported as graph splits.
- Wrong-net/wrong-layer contacts remain blocking.
- `go test ./internal/designworkflow -run 'ContactGraph|RouteTreeRepair|VCC' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Classify VCC route-tree contact gaps`.

## Phase 4: VCC Candidate Ranking And Limit Audit

### Goals

- Verify whether the first legal VCC candidate pair is being tried.
- Improve ranking only where evidence shows a better legal pair exists.

### Tasks

- Add tests that inspect VCC candidate lists for the I2C fixture:
  - connector-side VCC pad access;
  - sensor/decoupling/pull-up side VCC pad access;
  - local-route anchors on VCC;
  - candidate layers;
  - candidate pair rank ordering.
- Add diagnostics when candidate limits truncate same-net local-route anchor
  pairs.
- If evidence shows a legal pair is excluded:
  - adjust role/distance/layer tie-breakers; or
  - raise the route-tree branch access-pair limit conservatively.
- If evidence shows all tried pairs are genuinely blocked, leave ranking
  unchanged and document the blocker.

### Acceptance

- Candidate ranking for VCC is deterministic and tested.
- The chosen candidate-pair limit is justified by evidence.
- No broader routing behavior regresses.
- `go test ./internal/designworkflow -run 'AccessCandidate|VCC|RouteTreeBranch' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Audit VCC route-tree access ranking`.

## Phase 5: Fixed-Net And Net-Class Diagnostic Cleanup

### Goals

- Reduce non-target fixed-net informational noise in branch blocker evidence.
- Keep real VCC/GND net-class warnings visible.

### Tasks

- Suppress or separate informational fixed non-target skip issues from
  route-tree blocker counts.
- Preserve target-net class warnings and pathfinding blockers.
- Add route-tree summary fields or issue filters for:
  - informational preserved-net notices;
  - blocker issue count;
  - warning issue count.
- Add tests proving fixed non-target notices do not inflate:
  - `route_tree_repair.branch_failures`;
  - `inter_block_route_trees.issue_count`, if that field is intended to mean
    blockers.
- Add explicit generated-power-net repair guidance for missing VCC/GND net
  classes.

### Acceptance

- VCC blocker evidence is not buried under repeated fixed-net skip notices.
- Power net-class warnings remain visible and actionable.
- `go test ./internal/designworkflow -run 'FixedNet|NetClass|RouteTreeRepair|VCC' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Clarify VCC route-tree blocker diagnostics`.

## Phase 6: I2C Fixture Rerun And Promotion Decision

### Goals

- Run the real KiCad-backed fixture and update tracked evidence/docs based on
  actual results.

### Tasks

- Run:

  ```sh
  kicadai \
    --request examples/design/kicad-backed/i2c_sensor_breakout_candidate.json \
    --output examples/.generated/i2c_sensor_breakout_candidate \
    --overwrite \
    design create
  ```

- Inspect:
  - routing stage summary;
  - `.kicadai/design-promotion.json`;
  - route-tree branch evidence;
  - contact graph evidence;
  - route-tree repair hints.
- If the fixture reaches 12/12 internal proof, evaluate whether KiCad and
  promotion gates allow readiness changes.
- If it remains expected-fail, update docs with the exact blocker category and
  candidate evidence.

### Acceptance

- Fixture result is reflected honestly in docs/metadata.
- If promoted, all declared gates support the promotion.
- If still expected-fail, the blocker is narrower than the current broad VCC
  contact/branch gap.
- `go test ./internal/designworkflow -run 'I2CSensorBreakout|RouteTree|Promotion' -count=1`
  passes.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Update VCC route-tree fixture evidence`.

## Phase 7: Full Regression And Documentation

### Goals

- Make the closeout understandable for future AI repair and routing work.

### Tasks

- Update:
  - `README.md`;
  - `docs/layout-routing.md`;
  - `specs/ROADMAP.md`;
  - this plan's implementation status.
- Document:
  - whether VCC is now internally proven;
  - remaining KiCad/DRC or route-tree blockers;
  - how to read access attempt evidence;
  - next follow-up if promotion is still blocked.
- Run full tests.

### Acceptance

- Documentation matches actual fixture behavior.
- `go test ./...` passes.
- Worktree is clean after commit.

### Review And Commit

- Run `prism review staged`.
- Fix high and medium findings.
- Commit message: `Document VCC route-tree closeout status`.

## Implementation Status

Status date: 2026-07-02

- Phase 1 complete: the I2C fixture now locks the VCC-specific proof boundary
  at 12 required route-tree endpoints, 11 proven endpoints, 3 complete contact
  graph groups, and 1 partial group.
- Phase 2 complete: branch evidence records bounded access attempts, selected
  access roles, coordinates, layers, selected refs/pads, and snap-exempt route
  use.
- Phase 3 complete: endpoint contact proof now distinguishes same-net graph
  splits from generic misses and maps those blockers into route-tree repair.
- Phase 4 complete: VCC access ranking now records source/target candidate
  counts, candidate-pair totals, limits, truncation, and deterministic selected
  pair evidence.
- Phase 5 complete: fixed-net skip notices and missing-net-class
  warnings are structured and counted separately from repairable route-tree
  blockers.
- Phase 6 complete: the real KiCad-backed I2C fixture remains `expected_fail`,
  but the blocker is narrowed to VCC: one `ROUTE_GRAPH_INCOMPLETE` contact
  proof plus two branch-scoped `no legal two-layer path` blockers.
- Phase 7 complete when this documentation update, Prism review, and full
  regression pass are committed.

Remaining follow-up: complete the VCC route-tree branch path so the final VCC
endpoint enters the same contact graph component, then rerun promotion gates for
project write, writer correctness, validation, and KiCad ERC/DRC evidence.
