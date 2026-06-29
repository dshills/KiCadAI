# KiCad-Backed Design Promotion Implementation Plan

## Phase 1: Fixture Audit And Promotion Baseline

Objective: document the current optional KiCad-backed fixture state before
changing behavior.

Tasks:

- Inventory files under `examples/design/kicad-backed/`.
- Record each fixture's:
  - request file;
  - metadata file;
  - tier;
  - acceptance level;
  - declared readiness;
  - expected stages;
  - expected artifacts;
  - known gaps.
- Run the existing metadata/test path without real KiCad.
- If locally configured, run the optional KiCad-backed tier with
  `KICADAI_KICAD_CLI` and capture observed blockers.
- Write the findings to
  `specs/kicad-backed-design-promotion/AUDIT.md`.

Validation:

- Audit includes all current fixtures:
  - `led_indicator_kicad_smoke`;
  - `connector_led_kicad_smoke`;
  - `i2c_sensor_breakout_candidate`;
  - `opamp_headphone_buffer_kicad_candidate`.
- Audit distinguishes documented metadata from observed behavior.
- `go test ./...` still passes without KiCad.

Suggested commit:

```text
Audit KiCad-backed design promotion fixtures
```

## Phase 2: Promotion Metadata Validation

Objective: make fixture metadata validation strict enough to support promotion.

Tasks:

- Locate the existing optional design fixture metadata loader.
- Add validation for:
  - known readiness values;
  - known acceptance values;
  - request file existence;
  - metadata `id` and request filename consistency;
  - non-empty `known_gaps` for `expected_fail` and `blocked`;
  - non-empty `expected_stages` for runnable fixtures;
  - explicit `require_erc` and `require_drc` values;
  - valid expected artifact paths.
- Add tests with valid and invalid metadata fixtures.
- Keep validation errors deterministic and machine-readable.

Validation:

- Invalid readiness is rejected.
- Missing request file is rejected.
- `expected_fail` without `known_gaps` is rejected.
- Existing fixtures remain valid.
- `go test ./...` passes.

Suggested commit:

```text
Validate KiCad-backed design promotion metadata
```

## Phase 3: Promotion Report Model

Objective: introduce a normalized report type that can represent promotion
evidence independently of KiCad availability.

Tasks:

- Add Go types for:
  - promotion report;
  - gate result;
  - stage summary;
  - promotion issue;
  - artifact reference;
  - achieved readiness.
- Define stable status values:
  - `pass`;
  - `warn`;
  - `failed`;
  - `expected_fail`;
  - `unexpected_pass`;
  - `blocked`;
  - `skipped`;
  - `error`;
  - `not_run`.
- Implement deterministic sorting for gates, issues, artifacts, and stages.
- Add JSON marshal tests with golden expected output.
- Place the model in a non-test package accessible to the workflow and CLI. The
  expected location is `internal/designworkflow/promotion.go` unless the
  implementation reveals a cleaner existing package boundary.

Validation:

- Report JSON is stable across repeated marshals.
- Empty reports fail validation.
- Gate and issue ordering is deterministic.
- `go test ./...` passes.

Suggested commit:

```text
Add KiCad-backed design promotion report model
```

## Phase 4: Internal Evidence Gates

Objective: convert internal workflow evidence into promotion gate results.

Tasks:

- Map existing `design create` result data into gates:
  - metadata;
  - stage;
  - writer correctness;
  - connectivity;
  - route completion;
  - physical rules;
  - artifacts.
- Reuse existing board validation, route-contact, route-completion, and
  physical-rule evidence where possible.
- Distinguish missing evidence from failing evidence.
- Add synthetic unit tests for:
  - clean internal candidate;
  - expected route-contact miss;
  - wrong net assignment;
  - missing writer artifact;
  - missing route completion evidence.

Validation:

- Route emission without physical endpoint contact does not pass route
  completion.
- Writer correctness blockers prevent `candidate`.
- Connectivity blockers prevent `candidate`.
- Expected blockers can still classify an `expected_fail` fixture as expected.
- `go test ./...` passes.

Suggested commit:

```text
Evaluate internal promotion evidence gates
```

## Phase 5: Optional KiCad Evidence Gate

Objective: normalize optional real KiCad ERC/DRC evidence in promotion reports.

Tasks:

- Map existing KiCad check summaries into a promotion gate.
- Represent missing `KICADAI_KICAD_CLI` as `skipped` external evidence.
- Treat skipped KiCad evidence as acceptable only for default non-promoted test
  runs, never as a `pass` fixture proof.
- Honor metadata allowlists.
- Record raw report artifact paths when available.
- Add fake-runner tests for:
  - missing CLI;
  - clean ERC/DRC;
  - allowed findings;
  - unexpected findings;
  - missing artifacts.

Validation:

- `require_erc` and `require_drc` affect promotion status.
- Missing CLI does not create false pass evidence.
- Unexpected KiCad findings block `candidate` and `pass`.
- Existing optional tests still skip cleanly without KiCad.
- `go test ./...` passes.

Suggested commit:

```text
Add optional KiCad promotion evidence gate
```

## Phase 6: Promotion Runner And Artifact Output

Objective: produce promotion reports from the optional design example runner and
CLI artifact path.

Tasks:

- Extend the optional KiCad-backed design example test harness to build a
  promotion report for every fixture.
- Write `design-promotion.json` under the output `.kicadai/` directory when
  artifacts are enabled.
- Include promotion summary data in CLI JSON output for matching runs.
- Preserve existing transaction and manifest artifacts.
- Add tests for:
  - artifact path selection;
  - report write behavior;
  - report content for expected-fail fixtures;
  - no artifact write when artifacts are disabled.

Validation:

- Optional fixture runs produce a report when artifact output is enabled.
- CLI JSON includes promotion summary data without breaking existing fields.
- Existing artifact expectations still pass.
- `go test ./...` passes.

Suggested commit:

```text
Write KiCad-backed design promotion artifacts
```

## Phase 7: Fixture Classification And First Promotion Decision

Objective: apply the new gates to current fixtures and update readiness only
where evidence justifies it.

Tasks:

- Run all current optional fixtures through the promotion runner.
- For each fixture, compare declared readiness to achieved readiness.
- Keep `expected_fail` when blockers remain and update `known_gaps` with more
  precise issue codes.
- Promote a fixture to `candidate` only if:
  - internal gates pass;
  - required optional KiCad evidence is clean when configured;
  - artifact expectations are met;
  - repeated run output is stable.
- Do not promote the amplifier fixture until fabrication-candidate blockers are
  closed.

Validation:

- Every fixture has an explicit achieved-readiness result.
- Any readiness change is backed by gate evidence.
- `expected_fail` fixtures have current blockers.
- No fixture reports accidental success without metadata review.
- `go test ./...` passes.

Suggested commit:

```text
Classify KiCad-backed design promotion fixtures
```

## Phase 8: AI-Actionable Repair Guidance

Objective: turn promotion blockers into useful next actions for AI agents.

Tasks:

- Add deterministic repair guidance for common issue codes:
  - missing component evidence;
  - missing pinmap;
  - wrong net assignment;
  - route contact miss;
  - route incomplete;
  - missing outline;
  - DRC clearance;
  - missing KiCad CLI;
  - missing report artifact.
- Include repair guidance in promotion report issues.
- Add tests for stable guidance text.
- Ensure guidance is concise enough for CLI JSON consumers.

Validation:

- Every blocking promotion issue has a repair suggestion.
- Repair suggestions are deterministic.
- Suggestions do not claim unsupported automatic fixes.
- `go test ./...` passes.

Suggested commit:

```text
Add repair guidance for design promotion blockers
```

## Phase 9: Documentation And Roadmap Update

Objective: make the promotion workflow visible to users and future agents.

Tasks:

- Update `examples/design/kicad-backed/README.md` with:
  - readiness definitions;
  - promotion report location;
  - optional KiCad run command;
  - fixture-specific current blockers.
- Update `docs/intent-planning.md` and `docs/validation-and-analysis.md` with
  promotion report interpretation.
- Update `docs/kicadai-agent-skill.md` with guidance for AI agents.
- Update `specs/ROADMAP.md` to reflect completed promotion infrastructure and
  remaining fixture-specific blockers.
- Keep command examples using compiled `kicadai`, not `go run`.

Validation:

- Documentation matches implemented readiness states.
- Roadmap lists any promoted fixtures accurately.
- README does not overstate generated-board readiness.
- `go test ./...` passes.

Suggested commit:

```text
Document KiCad-backed design promotion workflow
```

## Final Acceptance

The implementation is complete when:

- all current optional KiCad-backed fixtures are covered by metadata validation;
- every fixture can produce a normalized promotion report;
- internal and optional KiCad evidence are represented as explicit gates;
- promotion status is deterministic and machine-readable;
- blocking issues include repair guidance;
- at least one fixture has an explicit promotion decision under the new model;
- default tests remain KiCad-independent;
- optional KiCad tests remain opt-in through `KICADAI_KICAD_CLI`;
- docs and roadmap accurately describe the current generated-board readiness.
