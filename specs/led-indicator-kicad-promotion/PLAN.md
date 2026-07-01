# LED Indicator KiCad-Backed Promotion Implementation Plan

## Phase 1: Baseline The Current Fixture

Objective: determine whether `led_indicator_kicad_smoke` can promote directly
or still has a real blocker.

Tasks:

- Run the fixture with the current request into `examples/.generated/`.
- Capture:
  - workflow `ok`;
  - achieved acceptance;
  - stage statuses;
  - promotion status;
  - KiCad ERC result;
  - KiCad DRC result;
  - writer correctness summary;
  - board validation summary;
  - route/contact evidence.
- Inspect generated schematic and PCB net declarations if any check fails.
- Compare observed blockers with current metadata `known_gaps`.

Validation:

- Baseline output clearly identifies whether `skip_routing` is still needed.
- Any blocker is concrete and tied to a stage or promotion gate.
- `examples/.generated/` remains ignored and uncommitted.

Commit guidance:

```text
No commit unless code or metadata changes are made.
```

## Phase 2: Enable And Verify LED Local Routing

Objective: make the LED fixture exercise the real block-local route path.

Tasks:

- Remove or override `validation.skip_routing` in
  `led_indicator_kicad_smoke.json`.
- Run `design create`.
- Verify the generated PCB includes:
  - resistor pad to LED pad local route;
  - assigned pad nets for resistor and LED;
  - assigned copper net code for the local route;
  - KiCad-native net declarations.
- Add or update tests if the LED fixture exposes an untested route/contact
  evidence case.
- Keep route warnings only when they are quality warnings, not missing-contact
  blockers.

Validation:

- Routing stage is reached.
- Route/contact evidence proves required LED series net endpoint contact.
- Writer correctness has no blocking pad/copper net assignment issues.
- Focused tests pass.

Suggested commit:

```text
Enable LED KiCad smoke routing evidence
```

## Phase 3: Resolve Schematic And PCB KiCad Check Blockers

Objective: make KiCad checks candidate-compatible.

Tasks:

- Run the fixture with real KiCad checks when `KICADAI_KICAD_CLI` is available.
- If ERC reports symbol mismatch, inspect embedded symbol output and fix the
  symbol template or resolver-backed embedding path.
- If ERC reports dangling wires, inspect schematic wire segmentation, junctions,
  labels, and pin anchors.
- If DRC reports real findings, classify each as:
  - generated-board defect;
  - footprint mismatch warning;
  - silkscreen/mechanical warning;
  - local KiCad tool instability.
- Fix generated-board defects.
- Preserve warning-level findings in promotion output rather than hiding them.

Validation:

- ERC check status is `pass`.
- DRC has no blocking findings.
- The no-finding KiCad DRC crash remains warning-only only for the narrow known
  case.
- `go test ./internal/designworkflow` passes.

Suggested commit:

```text
Clear LED KiCad smoke ERC blockers
```

## Phase 4: Promote Fixture Metadata

Objective: update fixture metadata from expected-fail to candidate only after
evidence supports it.

Tasks:

- Update `led_indicator_kicad_smoke.metadata.json`:
  - set `readiness` to `candidate`;
  - add `.kicadai/design-promotion.json` to `expected_artifacts`;
  - add `schematic_electrical` and `routing` to `expected_stages` if reached;
  - replace stale known gaps with current warning-only gaps;
  - update notes to describe the candidate evidence.
- Run optional design example metadata and promotion tests.
- Ensure candidate metadata policy accepts the updated fixture.

Validation:

- Promotion report declares candidate and achieves candidate or pass.
- Metadata validation passes.
- `go test ./internal/designworkflow -run 'TestDesignExamples|TestBuildPromotionReport|TestRunKiCadChecks'` passes.

Suggested commit:

```text
Promote LED KiCad smoke fixture
```

## Phase 5: Documentation And Roadmap Cleanup

Objective: make project status reflect that there are now two promoted
KiCad-backed candidate fixtures if Phase 4 succeeds.

Tasks:

- Update `specs/ROADMAP.md`:
  - remove stale language saying no KiCad-backed fixture is candidate;
  - list `connector_led_kicad_smoke` and `led_indicator_kicad_smoke` under
    candidate readiness if both are promoted;
  - keep I2C and amplifier as expected-fail unless changed.
- Update README or focused docs only if they currently name the stale fixture
  status.
- Do not overstate DRC proof while local KiCad DRC instability remains.

Validation:

- Roadmap status matches fixture metadata.
- Documentation says candidate, not pass.
- Full `go test ./...` passes.

Suggested commit:

```text
Document promoted LED KiCad smoke status
```

## Phase 6: Review And Final Commit

Objective: finish with the standard review and clean repository state.

Tasks:

- Run:

```text
go test ./...
```

- Stage intended changes.
- Run:

```text
prism review staged
```

- Fix any high or medium findings.
- Commit the final coherent change set.

Validation:

- Full tests pass.
- Prism has no unresolved high or medium findings.
- `git status --short` is clean after commit.

Suggested final commit:

```text
Promote LED KiCad smoke fixture
```
