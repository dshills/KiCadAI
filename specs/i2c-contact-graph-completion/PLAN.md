# I2C Contact Graph Completion Implementation Plan

Date: 2026-07-04

## Objective

Complete or narrowly diagnose the remaining I2C route-tree contact graph proof
gaps so `i2c_sensor_breakout_candidate` can move as close as current evidence
allows toward `candidate`.

Implement in phases. After each implementation phase:

1. run focused tests for that phase;
2. run `go test ./...` when production behavior changes;
3. stage only that phase;
4. run `prism review staged`;
5. fix high/medium findings;
6. commit before moving to the next phase.

## Phase 1: Current Partial-Net Forensics

### Goals

- Identify exactly which three route-tree nets remain partial.
- Identify the missing endpoint IDs, refs, pads, graph components, operation
  IDs, and branch IDs involved.
- Lock the current evidence before changing graph semantics.

### Tasks

- Add a focused I2C diagnostic test or helper that extracts:
  - route-tree managed nets;
  - per-net required/proven endpoint counts;
  - per-net component counts;
  - missing endpoint IDs;
  - issue codes and paths;
  - branch indexes and selected access roles.
- Add structured debug helpers in tests only if production evidence is already
  sufficient.
- If production evidence is insufficient, add compact production summaries under
  routing/contact evidence rather than relying on ad hoc test parsing.

### Acceptance

- Tests identify the exact remaining partial nets and missing endpoints.
- No production behavior changes unless required for evidence.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'I2C|RouteTree|ContactGraph' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Capture I2C contact graph gaps`.

## Phase 2: Graph Detail Evidence

### Goals

- Expose enough contact graph detail for AI repair loops and fixture promotion.
- Keep summary fields compact and stable.

### Tasks

- Add a per-net contact graph detail model if missing:
  - net name;
  - required endpoint IDs;
  - proven endpoint IDs;
  - missing endpoint IDs;
  - component count;
  - stable component summaries;
  - contributing operation IDs;
  - contributing branch indexes when available.
- Preserve existing `RouteTreeContactGraphSummary` JSON shape unless extending
  it with backward-compatible optional fields.
- Add tests for stable JSON/evidence shape.

### Acceptance

- Partial groups are explainable without manually reading route geometry.
- Existing promotion summaries remain compatible.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'ContactGraph|I2C' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Report route-tree contact graph detail`.

## Phase 3: Same-Net Segment Intersection Merge

### Goals

- Connect same-net route segments that physically touch or cross on the same
  layer, not only route vertices and target projections.

### Tasks

- Extend the contact graph builder to test same-net segment-to-segment contact
  on the same layer.
- Use spatial indexing or bounded candidate lookup to avoid O(N^2) behavior on
  larger route sets.
- Add tests for:
  - crossing same-net segments on the same layer;
  - overlapping collinear same-net segments;
  - near-miss segments outside tolerance;
  - wrong-net overlap rejection;
  - wrong-layer overlap rejection.

### Acceptance

- Same-net intersecting route-tree/local-route copper belongs to one component.
- Wrong-net/wrong-layer cases remain blocked or separate.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'ContactGraph|SameNet|Segment' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Merge same-net route segments in contact graph`.

## Phase 4: Local-Route Anchor Integration

### Goals

- Ensure block-local route anchors participate as real graph connectors for
  route-tree completion.

### Tasks

- Verify generated local-route operations are included in the graph operations
  used by I2C contact proof.
- If local-route anchors are summary-only today, add graph nodes/edges for
  those anchors and their source route operations.
- Add tests for:
  - local-route anchor on a route-tree segment;
  - route-tree branch ending at local-route anchor;
  - local-route anchor wrong-net rejection;
  - local-route anchor wrong-layer rejection.

### Acceptance

- Valid local-route anchors can complete route-tree endpoint proof.
- Invalid anchors do not mask blockers.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'LocalRoute|ContactGraph|RouteTree|I2C' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Connect local-route anchors in route-tree graph`.

## Phase 5: Via And Layer Transition Evidence

### Goals

- Avoid false graph splits when valid emitted via/layer transition evidence is
  present.
- Keep wrong-layer contact conservative when via evidence is absent.

### Tasks

- Inspect transaction route/via operation shape.
- Add graph layer-transition edges only for explicit via evidence.
- Add tests for:
  - F.Cu to B.Cu contact through a via;
  - wrong-layer segment overlap without via remains split;
  - via wrong-net rejection;
  - via outside tolerance remains split.

### Acceptance

- Contact graph understands modeled vias.
- No implicit layer merging occurs.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'Via|Layer|ContactGraph|RouteTree' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Model via transitions in route-tree contact graph`.

## Phase 6: I2C Completion Run And Metadata Decision

### Goals

- Run the I2C fixture after graph improvements.
- Promote only if evidence supports it.
- Otherwise narrow expected-fail metadata.

### Tasks

- Run focused I2C tests and generate/inspect promotion evidence.
- Compare current evidence with Phase 1:
  - complete groups;
  - partial groups;
  - proven endpoints;
  - blocking issue codes;
  - reached stages.
- If all route-tree nets complete and downstream gates pass, update fixture
  metadata readiness.
- If not, keep `expected_fail` and update known gaps with exact remaining
  blockers.

### Acceptance

- Fixture metadata matches fresh evidence.
- Promotion report distinguishes contact graph blockers from KiCad evidence
  blockers.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Update I2C contact graph promotion evidence`.

## Phase 7: Documentation And Roadmap Update

### Goals

- Keep user-facing docs aligned with the new I2C evidence.

### Tasks

- Update `README.md` I2C status paragraph.
- Update `docs/layout-routing.md`.
- Update `specs/ROADMAP.md`.
- Remove stale references to blockers that are no longer present.

### Acceptance

- Docs accurately state whether I2C is `expected_fail`, `candidate`, or `pass`.
- Docs name the current next blocker.
- Full tests pass:

  ```sh
  go test ./...
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Document I2C contact graph completion status`.

## Phase 8: Full Regression

### Goals

- Confirm no candidate fixtures regressed.

### Tasks

- Run:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|RouteTree|ContactGraph|InterBlockContact|Promotion|DesignExamples|ConnectorLED|LEDPrompt' -count=1
  go test ./...
  ```

- Check worktree cleanliness.

### Acceptance

- All tests pass.
- No uncommitted changes remain except intentional follow-up specs.

