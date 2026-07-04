# I2C Route-Tree Promotion Closeout Plan

Date: 2026-07-04

## Objective

Implement the next promotion project after the LED strict lane: reduce or close
the route-tree branch/contact blockers in
`examples/design/kicad-backed/i2c_sensor_breakout_candidate` and promote the
fixture as far as actual evidence allows.

After each phase:

1. run focused tests;
2. run broader tests appropriate to the touched packages;
3. stage changes;
4. run `prism review staged`;
5. fix high and medium findings;
6. commit before moving to the next phase.

## Phase 1: Baseline And Promotion Inventory

### Goals

- Freeze the current I2C selected-attempt route-tree failure shape.
- Make future improvements measurable.
- Protect the LED strict prompt and connector/LED candidate lanes.

### Tasks

- Add focused tests that run or load the I2C candidate workflow and extract:
  - promotion achieved readiness/status;
  - `inter_block_routing` summary;
  - route-tree complete/partial/blocked groups;
  - required/proven endpoints;
  - branch attempts/completions/blockers;
  - graph component counts;
  - selected retry attempt summary;
  - promotion issue codes.
- Add assertions that VCC/GND/SDA/SCL are route-tree-managed and do not fall
  back to generic net routing.
- Add assertions that blockers are branch/contact scoped.
- Add regression assertions for:
  - simple LED prompt strict candidate/pass behavior;
  - connector/LED candidate route evidence;
  - no new writer-correctness pad/copper net-code blockers.
- Document current blocker inventory in test names or stable assertion
  messages.

### Acceptance

- Current I2C failure is deterministic without KiCad.
- Tests fail if route-tree ownership or contact proof silently disappears.
- LED strict prompt tests remain green.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|RouteTree|LEDPrompt|ConnectorLED' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Capture I2C route-tree promotion baseline`.

## Phase 2: Same-Net Geometry Merge Semantics

### Goals

- Allow safe same-net geometry to participate in branch completion.
- Keep other-net and unsupported geometry conservative blockers.

### Tasks

- Audit route-tree occupancy and branch routing code with Atlas:
  - route-tree branch builder;
  - route search occupancy;
  - pad obstacle handling;
  - generated copper obstacle handling;
  - inter-block contact graph.
- Add or refine same-net classification:
  - pad same net;
  - generated branch copper same net;
  - generated local-route same net;
  - other-net pad/copper;
  - keepout/edge/unsupported geometry.
- Permit legal terminal/merge/pass-through behavior for same-net geometry.
- Add evidence fields or summary counters for:
  - terminal contacts;
  - same-net merges;
  - same-net pass-throughs;
  - rejected other-net contacts.
- Add tests for:
  - same-net pad terminal contact;
  - same-net branch copper merge;
  - local-route same-net merge;
  - other-net pad/copper remains blocked;
  - keepouts remain blocked.

### Acceptance

- Same-net geometry no longer blocks legal branch completion.
- Other-net geometry still blocks.
- Existing routing tests remain green.
- Focused tests pass:

  ```sh
  go test ./internal/routing ./internal/designworkflow -run 'SameNet|Occupancy|RouteTree|Merge' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Allow I2C route trees to merge same-net geometry`.

## Phase 3: Endpoint Access Candidate Resolution

### Goals

- Resolve every I2C branch endpoint into deterministic physical access
  candidates before search.
- Prefer proven local-route anchors where they reduce branch/contact failure.

### Tasks

- Introduce or extend an internal endpoint access model with:
  - endpoint ID;
  - role;
  - ref/pad;
  - net/net-code;
  - layer;
  - coordinate;
  - source;
  - ranking reason.
- Extract access candidates from:
  - hydrated footprint pads;
  - local-route anchors;
  - generated same-net copper;
  - current external anchors.
- Implement deterministic ranking:
  - proven local-route anchor;
  - shortest legal pad access;
  - compatible layer/low collision;
  - stable ref/pad/source tie-breaker.
- Emit structured diagnostics for:
  - no access candidates;
  - ambiguous candidates;
  - selected access blocked before search.
- Add I2C fixture tests that show VCC/GND/SDA/SCL endpoints have access
  candidates with stable selected roles.

### Acceptance

- I2C required endpoints expose physical access candidates.
- Local-route anchors are selected where they are the best safe access.
- Missing access is a precise route-tree blocker.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'EndpointAccess|RouteTree|PadHydration|LocalRoute|I2C' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Resolve I2C route-tree endpoint access`.

## Phase 4: Contact Graph Completion

### Goals

- Compute route-tree completion from physical same-net contact graph evidence.
- Stop relying on route operation counts as a proxy for connectivity.

### Tasks

- Build same-net graph nodes for:
  - required endpoints;
  - selected access points;
  - route segments;
  - vias;
  - pads;
  - local-route anchors;
  - same-net merge points.
- Build graph edges for:
  - geometric contact on compatible layers;
  - through-hole/via layer transitions;
  - legal same-net pad/segment contact;
  - legal local-route anchor contact.
- Compute completion:
  - complete when all required endpoints are one component;
  - partial when at least two endpoints are connected but not all;
  - blocked when no useful branch/contact proof exists.
- Report:
  - missing endpoints;
  - split components;
  - branch IDs contributing to each component;
  - same-net merge evidence.
- Add rounding tolerance tests for writer/readback coordinates.

### Acceptance

- I2C route-tree group completion is graph-derived.
- Split VCC/SDA/GND evidence identifies exact components and missing
  endpoints.
- Same-net local-route/branch merge decreases contact misses where valid.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'ContactGraph|InterBlockContact|RouteTree|I2C' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Compute I2C route-tree completion from contact graphs`.

## Phase 5: Branch Ordering And Clean Retry State

### Goals

- Improve route-tree completion by routing constrained branches first.
- Prevent failed partial branches from poisoning later branch search or retry
  attempts.

### Tasks

- Add deterministic branch difficulty scoring using:
  - endpoint access count;
  - Manhattan route distance;
  - local-route anchor availability;
  - immediate obstacle pressure when available;
  - layer/via constraints.
- Route harder/access-constrained branches before easier branches.
- Preserve successful same-net copper as merge targets.
- Discard failed partial branch operations.
- Ensure retry attempts rebuild route-tree-managed copper from clean generated
  transaction state.
- Add tests for:
  - constrained branch ordering;
  - failed branch cleanup;
  - successful branch merge target reuse;
  - repeated retry attempts without stale route-tree copper accumulation.

### Acceptance

- Branch ordering is deterministic and evidence-backed.
- Failed branch operations do not remain in emitted transactions.
- Retry attempts are clean.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow -run 'RouteTreeBranch|Retry|I2C' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Clean I2C route-tree branch retry state`.

## Phase 6: Fixture Promotion Decision

### Goals

- Run the I2C fixture after the route-tree changes.
- Promote it only if evidence supports promotion.
- Otherwise update metadata with narrowed blockers.

### Tasks

- Generate the fixture into `examples/.generated/i2c_sensor_breakout_candidate`.
- Inspect:
  - `.kicadai/workflow-result.json`;
  - `.kicadai/design-promotion.json`;
  - route-tree summaries;
  - validation summary;
  - generated schematic and PCB files.
- If route-tree/internal validation is clean enough:
  - update fixture metadata from `expected_fail` to `candidate`;
  - require KiCad only when configured evidence supports it.
- If blockers remain:
  - keep `expected_fail`;
  - update known gaps with exact branch/contact blockers;
  - add tests asserting the narrower blocker shape.
- Ensure KiCad gate status is separated from route-tree blockers.
- Update docs or roadmap status if fixture readiness changes.

### Acceptance

- I2C fixture metadata matches actual evidence.
- Promotion report distinguishes route-tree blockers from KiCad ERC/DRC
  blockers.
- The fixture either promotes or has narrower known blockers than the Phase 1
  baseline.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Update I2C route-tree promotion evidence`.

## Phase 7: Documentation And Roadmap Update

### Goals

- Keep user-facing docs aligned with the new I2C fixture evidence.
- Remove stale LED-promotion remaining-work language from the roadmap.

### Tasks

- Update `specs/ROADMAP.md`:
  - mark LED strict promotion closeout as complete;
  - describe the I2C route-tree outcome;
  - set the next blocker based on Phase 6.
- Update README if the I2C fixture readiness changed.
- Update docs that describe promotion reports or AI generation status if new
  semantics were added.
- Keep examples in docs using compiled `kicadai`, not `go run`.

### Acceptance

- Roadmap no longer lists completed LED strict promotion as remaining work.
- Roadmap accurately describes I2C candidate/pass/expected-fail status.
- Docs do not overclaim autonomous generation.

### Review And Commit

- Run `prism review staged`.
- Commit: `Document I2C route-tree promotion status`.

## Phase 8: Full Regression

### Goals

- Prove the closeout did not regress the broader project.

### Tasks

- Run:

  ```sh
  go test ./...
  ```

- Run the I2C fixture manually into `examples/.generated/i2c_sensor_breakout_candidate`.
- Inspect the generated evidence artifacts.
- Confirm `git status --short` is clean after committed code/docs.

### Acceptance

- Full test suite passes without requiring KiCad.
- Manual fixture output matches documented readiness.
- Generated scratch output remains ignored.
- No uncommitted tracked files remain.

### Review And Commit

- If Phase 8 requires code or doc changes, run `prism review staged` and commit.
- Otherwise record verification in the final implementation summary.
