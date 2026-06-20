# Routing Engine Hardening Implementation Plan

## Objective

Implement the Routing Engine Hardening spec in deterministic, reviewable
phases while preserving compatibility with existing routing requests, placement
handoff, transaction operations, PCB writer output, validation, repair, and
design workflow stages.

## Implementation Rules

- Keep routing logic in `internal/routing` unless integration requires
  `internal/designworkflow`, `internal/routingadapters`, or documentation
  updates.
- Put shared net class, layer, trace, via, clearance, zone, and length policy
  definitions in `internal/pcbrules`, so routing and PCB writing resolve the
  same rules.
- Do not introduce a second routing engine or direct KiCad file writer.
- Preserve existing request/result behavior unless the phase explicitly extends
  it.
- Keep route ordering, path search, diagnostics, and reports deterministic.
- Prefer extending existing `routing.Result`, `Route`, `Metrics`, diagnostics,
  and rule models over adding unrelated parallel structures.
- Add tests with every behavior change.
- Run `gofmt` and focused tests for each phase.
- Run `prism review staged` before each phase commit and address actionable
  findings.
- Run full `go test ./...` before the final documentation commit. Cache
  isolation belongs in CI configuration when needed, not in the default dev
  command.

## Phase 1: Rule Model And Resolution

### Goal

Make routing rules explicit enough to support net classes, role policies, via
policy, layer policy, and future length/differential constraints.

### Tasks

1. Extend or harden routing rule types for:
   - net class allowed layers;
   - preferred layer;
   - class-to-class clearance matrix;
   - neckdown width and neckdown length near constrained pads;
   - max vias;
   - warning/max length;
   - role policy;
   - high-current/power/clock/analog/differential intent.
2. Add optional per-net rule override support if not already represented.
3. Implement a rule-resolution helper:
   - explicit net override;
   - net class;
   - net role defaults;
   - global rules;
   - system defaults.
4. Add physical footprint/pad gate checks after logical rule resolution:
   - trace width exceeds pad entry capacity or local clearance unless a
     valid neckdown is available;
   - neckdown exceeds allowed length;
   - via diameter/drill cannot fit local geometry;
   - pad-specific clearance overrides.
5. Validate malformed rules:
   - negative widths/clearances;
   - invalid clearance matrix pairs;
   - clearance matrix entries must use normalized sorted class-pair keys so
     asymmetric values cannot be represented;
   - neckdown wider than trace or negative neckdown length;
   - trace width or neckdown width below board/system manufacturing minimums;
   - via drill larger than via diameter;
   - unknown layers;
   - forbidden via/back-layer combinations;
   - missing net classes;
   - unsupported differential policy.
6. Add tests for rule precedence, physical pad gates, clearance matrix
   resolution, neckdown constraints, and invalid rules.

### Acceptance Criteria

- Existing routing requests continue to normalize and route.
- Per-net effective rules are deterministic and testable.
- Invalid rule combinations produce structured blocking or warning issues.

### Suggested Commit

`Add routing rule resolution`

## Phase 2: Route Quality Report Model

### Goal

Expose per-net routing quality without changing route search behavior yet.

### Tasks

1. Add route quality/report structs to `routing.Result` or a nested
   `QualityReport`.
2. Report per-net:
   - role;
   - class;
   - endpoint count;
   - routed endpoint count;
   - status;
   - segment count;
   - via count;
   - length;
   - layer usage;
   - search nodes;
   - search limit hit;
   - failure category.
3. Add score dimensions for:
   - completion;
   - connectivity;
   - clearance;
   - via policy;
   - layer policy;
   - length;
   - search pressure.
4. Preserve existing `Metrics`.
5. Add tests for routed, partial, failed, and skipped nets.

### Acceptance Criteria

- Callers can explain route quality from JSON output.
- Existing route metrics remain stable.
- Failed route reports identify the affected net and failure category.

### Suggested Commit

`Add routing quality reports`

## Phase 3: Net-Class And Role-Aware Routing

### Goal

Use resolved rules during actual route construction and validation.

### Tasks

1. Apply effective trace width, clearance, via diameter, and via drill per net.
2. Apply allowed/preferred layer policy during search.
3. Add role defaults:
   - power/high-current wider defaults or warnings when class is missing;
   - clock long-route sensitivity;
   - analog route separation metadata where available;
   - signal defaults.
4. Validate output against effective per-net rules.
5. Add tests for:
   - net class trace width in generated segments;
   - net class via geometry;
   - allowed layer enforcement;
   - power/high-current missing class warning;
   - role ordering stability.

### Acceptance Criteria

- Generated route geometry reflects net-specific rules.
- Routing validation uses the same effective rule model as search.
- Missing high-current/power class evidence is surfaced.

### Suggested Commit

`Apply net class routing rules`

## Phase 4: Obstacle And Clearance Diagnostics

### Goal

Improve failure evidence when paths are blocked or clearance validation fails.

### Tasks

1. Keep occupancy data compact for the hot search path, and store obstacle
   source metadata in a secondary sparse map used only for collisions and
   diagnostic generation where practical.
2. Report blocking obstacle kinds:
   - board edge;
   - keepout;
   - pad;
   - other-net existing copper;
   - same-net existing copper as target/traversal evidence;
   - via keepout;
   - zone.
3. Report nearest or last blocking obstacle when no path is found.
4. Include refs/nets/source paths when available.
5. Add tests for:
   - no path due to keepout;
   - no path due to other-net pad;
   - fixed copper blockage;
   - same-net copper connection target;
   - center-line obstacle inflation using half trace width plus clearance;
   - edge clearance blockage;
   - clearance violation naming affected nets.

### Acceptance Criteria

- No-path failures tell the caller what blocked the route when evidence is
  available.
- Clearance failures name both nets when possible.
- Diagnostics remain deterministic.

### Suggested Commit

`Add routing obstacle diagnostics`

## Phase 5: Two-Layer And Via Policy Hardening

### Goal

Make two-layer routing and via behavior explicit, testable, and repairable.

### Tasks

1. Enforce `AllowVias`, `AllowBackLayer`, `MaxViasPerNet`, allowed layers, and
   preferred layer policy consistently.
2. Prefer fewer vias when path cost ties.
3. Report via policy violations in route reports.
4. Treat through-hole pads as valid zero-cost cross-layer access only for
   segments of the net assigned to that pad during search-grid initialization.
5. Add diagnostics for:
   - vias forbidden but required;
   - too many vias;
   - back layer disabled;
   - preferred layer violated.
6. Add tests for:
   - two-layer success with one via;
   - via-forbidden failure;
   - max-vias failure;
   - back-layer disabled failure;
   - through-hole access across layers.

### Acceptance Criteria

- Via usage is bounded and explainable.
- Two-layer routes remain deterministic.
- Via policy failures have repair actions.

### Suggested Commit

`Harden routing via policy`

## Phase 6: Length And Search-Pressure Scoring

### Goal

Expose route length and search effort as quality signals before KiCad DRC.

### Tasks

1. Add route length thresholds from:
   - net override;
   - net class;
   - role default;
   - global fallback.
2. Warn or fail when length exceeds configured thresholds.
3. Report search nodes per route and whether the search budget was hit.
4. Add search-pressure score dimension.
5. Add tests for:
   - long route warning;
   - max length failure;
   - search node budget hit;
   - integer score aggregation by priority or net role;
   - stable tie-breaking by net ID, layer, Y coordinate, X coordinate, and
     endpoint order.

### Acceptance Criteria

- Route reports distinguish long-but-routed from failed.
- Search budget exhaustion is visible and repairable.
- Score dimensions separate route quality from route completion.
- Scoring is deterministic across platforms because it avoids floating-point
  path ranking and defines stable tie-breakers.

### Suggested Commit

`Score routing length and search pressure`

## Phase 7: Zone Policy Hardening

### Goal

Make zone handling explicit rather than implicit.

### Tasks

1. Validate `TreatZonesAs` policy.
2. Implement/report:
   - `ignore`: route through zones and add warning evidence;
   - `obstacle`: treat zones as blocked geometry;
   - `unsupported`: block routing when zones are present;
   - `sufficient`: skip only zone-proven completed net segments and report the
     validation evidence that made the skip legal.
3. Build or reuse cached zone-connectivity evidence, such as a conservative
   simplified polygon decomposition or same-net connectivity graph, once during
   search-grid initialization rather than running raw grid flood fill for every
   segment check.
4. Key cached zone evidence by a hash of zone geometry, connection points, and
   the specific rule IDs/values used for the proof; invalidate only when that
   key changes.
5. Ensure ground/power zones are not assumed to complete a net unless explicit
   validation evidence exists.
6. Add tests for:
   - ignore warning;
   - obstacle detour/failure;
   - unsupported blocked issue;
   - sufficient-zone skip for an evidenced same-net segment;
   - insufficient or uncertain zone evidence remains unrouted;
   - cached zone evidence reuse;
   - no false connectivity completion from zones.

### Acceptance Criteria

- Every request with zones has an explicit route policy outcome.
- Zone behavior is visible in route reports and diagnostics.
- Router does not claim zone connectivity without evidence.
- Router does not create redundant trace loops for zone-proven satisfied
  segments.

### Suggested Commit

`Harden routing zone policy`

## Phase 8: Differential And Length-Matching Intent Foundation

### Goal

Represent advanced route intent honestly without falsely claiming support.

### Tasks

1. Add coupled-net constraint model or reserved rule extension with
   `DifferentialPair` as the first supported mode.
2. Validate pair member nets and endpoint compatibility:
   - exactly two member nets for `DifferentialPair`;
   - reserved N-member mode rejected with an explicit unsupported diagnostic
     until implemented;
   - explicit polarity metadata or configured complementary suffixes;
   - both nets exist;
   - endpoint counts are comparable.
3. Add route report fields for:
   - pair group;
   - measured length;
   - target/max length;
   - length mismatch.
4. Emit blocked or warning diagnostics for unsupported pair tuning depending
   on policy.
5. Add length-matching warnings/failures for ordinary nets with configured
   tolerances.
6. Add tests for:
   - valid pair metadata;
   - differential pair with more or fewer than two members;
   - unsupported N-member coupled group;
   - polarity naming or metadata mismatch;
   - missing pair member;
   - unsupported differential routing diagnostic;
   - length tolerance warning/failure.

### Acceptance Criteria

- Differential intent is never silently downgraded to generic routing.
- Unsupported advanced behavior is explicit and actionable.
- Length fields are available for future tuning.

### Suggested Commit

`Add routing differential intent model`

## Phase 9: Repairable Routing Diagnostics

### Goal

Convert route reports and validation findings into AI-actionable diagnostics.

### Tasks

1. Extend routing diagnostics categories/actions for:
   - move components closer;
   - rerun placement;
   - widen board or relax keepouts;
   - increase search budget;
   - allow vias/back layer;
   - assign net class;
   - change trace width/clearance;
   - resolve zone policy;
   - handle unsupported differential intent.
2. Include nets, refs, operation IDs, and rule IDs where available.
3. Map internal validation issues to these diagnostics.
4. Map KiCad DRC findings to route diagnostics where parser evidence exists.
5. Add tests for major diagnostic categories and suggestions.

### Acceptance Criteria

- Failed routes produce repairable structured diagnostics.
- Diagnostics identify whether repair belongs to placement, routing rules, or
  writer/validation.
- Design workflow feedback can surface route repair actions.

### Suggested Commit

`Expose repairable routing diagnostics`

## Phase 10: Design Workflow And Repair Integration

### Goal

Feed hardened routing reports into `design create`, repair planning, and
operation-correlated feedback.

### Tasks

1. Include route quality reports in routing stage summaries or artifacts.
2. Ensure route diagnostics are available in `design create` feedback.
3. Preserve operation IDs for generated route operations.
4. Add repair planner mappings for route diagnostics.
5. Add route-stage retry hooks where safe:
   - increase search budget;
   - allow two-layer routing;
   - request placement retry when routing failure is caused by spacing.
6. Bound every retry hook with an explicit attempt limit and convergence check
   so routing/placement feedback cannot loop indefinitely.
7. Add workflow tests for failed route feedback, repair suggestions, retry
   limits, and non-improving retry termination.

### Acceptance Criteria

- Workflow users can see why routing failed or is weak.
- Repair suggestions point to routing, placement, or rules as appropriate.
- Generated route operations remain transaction-compatible.
- Non-converging placement/routing retries stop with a structured blocked
  result.

### Suggested Commit

`Integrate routing diagnostics with workflow`

## Phase 11: Golden Routing Corpus

### Goal

Lock representative routing behavior and prevent regressions.

### Tasks

1. Add golden routing cases for:
   - LED indicator;
   - voltage regulator;
   - MCU local nets;
   - USB-C power entry;
   - I2C sensor;
   - op-amp feedback;
   - connector breakout;
   - forced single-layer failure;
   - two-layer via success;
   - keepout detour;
   - zone policy.
2. Compare compact summaries:
   - status;
   - routed/failed/skipped counts;
   - segment/via counts;
   - route report statuses;
   - diagnostic categories/actions;
   - topology or route-shape hash for generated segments;
   - operation counts.
3. Add update guidance in test comments or docs.
4. Keep fixtures deterministic and small.

### Acceptance Criteria

- Golden tests cover supported seed block-shaped routes.
- Failures are readable and actionable.
- Route output remains deterministic across test runs.

### Suggested Commit

`Add routing hardening golden corpus`

## Phase 12: Documentation And Roadmap

### Goal

Document hardened routing behavior and advance the roadmap.

### Tasks

1. Update README routing section with:
   - net classes;
   - role policies;
   - route quality reports;
   - diagnostics;
   - remaining limitations.
2. Update `specs/ROADMAP.md` Priority 5 status and near-term sequence.
3. Document how agents should interpret route diagnostics.
4. Run full tests.

### Acceptance Criteria

- README explains current routing support and limits.
- Roadmap reflects completed routing hardening foundation and remaining gaps.
- Full `go test ./...` passes.

### Suggested Commit

`Document routing engine hardening`
