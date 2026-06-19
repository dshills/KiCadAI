# Placement Engine Hardening Implementation Plan

## Objective

Implement the Placement Engine Hardening spec in small, reviewable phases while
preserving deterministic output and compatibility with existing design workflow,
transaction, routing, and repair code.

## Implementation Rules

- Keep placement logic in `internal/placement` unless integration requires
  design workflow or block metadata changes.
- Do not introduce a second placement engine.
- Preserve existing public behavior unless the phase explicitly changes it.
- Keep all placement decisions deterministic.
- Prefer extending `QualityReport`, diagnostics, and existing request models
  over adding unrelated report structures.
- Add tests with each behavior change.
- Run `gofmt`, focused tests, and `go test ./...` before final docs commit.
- Run `prism review staged` before each phase commit and address actionable
  findings.

## Phase 1: Placement Intent Model

### Goal

Represent role/proximity/region intent explicitly enough for scoring and
diagnostics without changing placement behavior yet.

### Tasks

1. Add placement intent model types:
   - intent role constants;
   - `ProximityRule`;
   - `RegionRule`;
   - score dimension/report types if not folded into `QualityReport`.
2. Extend `placement.Request` or `placement.Rules` with optional proximity and
   region rules.
3. Add normalization helpers for:
   - stable rule IDs;
   - empty weights;
   - duplicate refs;
   - unknown refs.
4. Add validation for malformed intent rules.
5. Add tests for valid rules, invalid refs, duplicate IDs, and deterministic
   normalization.

### Acceptance Criteria

- Existing placement behavior is unchanged.
- Intent rules are accepted, normalized, and validated.
- Invalid hard rules produce blocking issues; invalid soft rules produce
  warnings only when explicitly marked optional.

### Suggested Commit

`Add placement intent rule models`

## Phase 2: Block Metadata To Placement Intent

### Goal

Convert existing circuit-block PCB realization metadata into placement intent
rules.

### Tasks

1. Extend design workflow placement request assembly to translate:
   - block placement groups;
   - block keepouts;
   - block PCB constraints;
   - required local routes;
   - component roles.
2. Generate proximity rules for known block patterns:
   - LED resistor to LED;
   - regulator capacitors to regulator;
   - MCU decoupling/reset/clock support to MCU;
   - USB-C protection/power path to connector;
   - I2C pull-ups/decoupling to sensor;
   - op-amp feedback network to op-amp.
3. Preserve block IDs and component roles in rule source metadata.
4. Add workflow tests that inspect the placement request generated for one or
   two representative blocks.

### Acceptance Criteria

- `designworkflow.PlaceFragments` receives normalized block-derived placement
  intent.
- Existing examples still place successfully.
- Rule source metadata makes diagnostics traceable to circuit blocks.

### Suggested Commit

`Derive placement intent from circuit blocks`

## Phase 3: Proximity Scoring

### Goal

Score and report whether electrically related parts are close enough.

### Tasks

1. Add proximity scoring after placement.
2. Prefer pad-to-pad distance when both anchor and target pads are available.
3. Fall back to center-to-center distance with weaker evidence.
4. Record per-rule status:
   - pass;
   - warning;
   - fail.
5. Emit diagnostics for failed required proximity rules and weak optional rules.
6. Add tests for:
   - decoupling near anchor pass;
   - decoupling far from anchor warning/fail;
   - pad-distance evidence;
   - center-distance fallback.

### Acceptance Criteria

- Quality reports include proximity evidence.
- Required proximity failures produce actionable placement issues.
- Optional proximity misses do not block placement but appear in diagnostics.

### Suggested Commit

`Score placement proximity rules`

## Phase 4: Group-Cohesion Hardening

### Goal

Make circuit-block groups physically coherent and measurable.

### Tasks

1. Improve group placement candidate scoring using:
   - group anchor;
   - max spread;
   - group role;
   - component role priority.
2. Add group spread score dimensions.
3. Make required max-spread failures blocking.
4. Add soft warnings when groups are legal but poor quality.
5. Add tests for:
   - regulator group cohesion;
   - MCU support group cohesion;
   - impossible max spread;
   - deterministic group tie-breaking.

### Acceptance Criteria

- Block groups remain closer than generic placement would put them.
- Group failures name affected refs and group IDs.
- Existing collision/board-fit constraints still take precedence.

### Suggested Commit

`Harden placement group cohesion`

## Phase 5: Connector Edge And Fanout Policy

### Goal

Make connector placement satisfy edge and orientation intent reliably.

### Tasks

1. Prioritize edge-constrained components before ordinary components.
2. Add edge-facing rotation defaults where explicit rotations are absent.
3. Reserve connector fanout keepout/clearance regions inside the board.
4. Score edge distance and orientation satisfaction.
5. Add diagnostics for:
   - wrong edge;
   - wrong side/layer;
   - wrong rotation;
   - insufficient fanout clearance.
6. Add tests for connector breakout and USB-C power connector placement.

### Acceptance Criteria

- Edge-constrained connectors land on the requested edge.
- Connector edge failures are blocking and repairable.
- Connector diagnostics include suggested action and affected refs.

### Suggested Commit

`Harden connector edge placement`

## Phase 6: Mechanical Constraints

### Goal

Treat board and mechanical geometry as placement constraints, not afterthoughts.

### Tasks

1. Add or harden rectangular mechanical constraint models for:
   - mounting holes;
   - component keepouts;
   - connector clearance;
   - user/fixed placement exclusion.
2. Normalize mechanical constraints into placement keepouts.
3. Include keepout/mechanical score dimensions.
4. Emit blocking issues for hard keepout intersection.
5. Add tests for:
   - mounting hole keepout;
   - connector clearance;
   - fixed placement exclusion;
   - soft versus hard keepout handling.

### Acceptance Criteria

- Components never occupy hard mechanical keepouts.
- Mechanical failures are visible in quality and issues.
- Routing handoff sees keepouts consistently.

### Suggested Commit

`Add placement mechanical constraints`

## Phase 7: Region Separation

### Goal

Support coarse analog/digital/power/clock/noisy/user-facing placement regions.

### Tasks

1. Add region rule normalization and scoring.
2. Map known net roles and component roles to region hints.
3. Score whether components are inside preferred regions.
4. Penalize noisy/clock/power crossing into sensitive analog regions.
5. Add tests for:
   - analog region preference;
   - power path region preference;
   - optional region miss warning;
   - required region miss blocking.

### Acceptance Criteria

- Region goals are reported clearly.
- Optional region preferences do not block.
- Required region constraints block with actionable issues.

### Suggested Commit

`Add placement region scoring`

## Phase 8: Routing-Readiness Metrics

### Goal

Expose whether placement is likely to route cleanly before the routing stage.

### Tasks

1. Add per-net rough HPWL metrics.
2. Add coarse congestion grid scoring.
3. Detect long local-route violations from block required route metadata.
4. Include routing-readiness diagnostics in `QualityReport`.
5. Add tests for:
   - HPWL determinism;
   - high-congestion detection;
   - long required local route warning/fail.

### Acceptance Criteria

- Placement output includes routing-readiness metrics.
- Routing handoff remains compatible.
- Diagnostics identify nets and refs responsible for poor route readiness.

### Suggested Commit

`Add placement routing readiness metrics`

## Phase 9: Repair Feedback Integration

### Goal

Make placement diagnostics directly useful to repair workflows.

### Tasks

1. Map placement diagnostics to `reports.Issue` values consistently.
2. Include refs, nets, group IDs, and operation IDs where available.
3. Add suggested actions:
   - increase board size;
   - move group together;
   - move connector to edge;
   - move decoupling near anchor;
   - move out of keepout;
   - assign richer footprint geometry.
4. Ensure design workflow feedback groups placement repair suggestions under
   placement retry scope.
5. Add tests for workflow feedback output.

### Acceptance Criteria

- Placement hardening failures produce repairable structured issues.
- AI-facing workflow feedback points to placement retry, not generic failure.
- Existing repair classification recognizes the major placement categories.

### Suggested Commit

`Expose repairable placement diagnostics`

## Phase 10: Golden Placement Corpus

### Goal

Lock down representative placement behavior and prevent regression.

### Tasks

1. Add golden examples for:
   - LED indicator;
   - voltage regulator;
   - MCU minimal;
   - USB-C power;
   - I2C sensor;
   - op-amp gain stage;
   - connector breakout.
2. Store deterministic placement summaries rather than brittle full JSON when
   possible.
3. Include expected quality metrics and key diagnostics.
4. Add update guidance for golden files.
5. Add tests that compare golden summaries.

### Acceptance Criteria

- Golden tests cover the supported seed block layouts.
- Golden failures are readable and actionable.
- Output remains deterministic across test runs.

### Suggested Commit

`Add placement hardening golden corpus`

## Phase 11: Documentation And Roadmap

### Goal

Document hardening behavior and advance the roadmap.

### Tasks

1. Update README placement/design workflow sections.
2. Document placement quality fields and common diagnostics.
3. Update `specs/ROADMAP.md` Priority 4 status.
4. List remaining placement limitations.
5. Run full tests.

### Acceptance Criteria

- README explains how agents should interpret placement quality and repair
  diagnostics.
- Roadmap reflects completed hardening phases and remaining gaps.
- `go test ./...` passes.

### Suggested Commit

`Document placement engine hardening`

## Cross-Phase Test Matrix

Run focused tests after each implementation phase:

```sh
go test ./internal/placement ./internal/designworkflow
```

Run full tests before final documentation commit:

```sh
go test ./...
```

Run Prism before each phase commit:

```sh
prism review staged
```

## Completion Criteria

The hardening project is complete when:

- all phases are implemented;
- supported seed block layouts have deterministic golden coverage;
- quality reports include group, proximity, edge, mechanical, region, and
  routing-readiness evidence;
- design workflow placement stages expose actionable repair diagnostics;
- full test suite passes;
- Prism has no unresolved high or medium findings.
