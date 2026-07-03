# Amplifier KiCad Output Protection Evidence Implementation Plan

## Phase 1: Baseline Current Amplifier Evidence

Goal: establish the current behavior before adding the protected Class AB
fixture.

Tasks:

- Run or inspect the existing optional amplifier fixture,
  `opamp_headphone_buffer_kicad_candidate`.
- Run or inspect the connectivity example,
  `examples/design/amplifier/class_ab_headphone_driver.json`.
- Capture the current promotion stages, generated artifacts, protected-output
  summaries, validation issues, and known blockers.
- Confirm whether strict acceptance policy, component evidence, schematic
  generation, PCB realization, placement, routing, writer correctness, or KiCad
  checks are the first true blocker.
- Record the before-state in tests, metadata notes, or implementation comments
  only where useful.

Acceptance:

- The implementation has a precise current blocker list.
- Work does not rely on stale op-amp-buffer fixture assumptions.
- The protected Class AB connectivity example can be used as the source shape
  for the new KiCad-backed fixture.

Validation:

```text
go test -v ./internal/designworkflow -count=1
```

Prism:

```text
prism review staged
```

Commit:

```text
Baseline protected amplifier KiCad evidence
```

## Phase 2: Add Protected KiCad-Backed Fixture Metadata

Goal: introduce the optional generated-design fixture for the protected Class AB
headphone output path.

Tasks:

- Add
  `examples/design/kicad-backed/class_ab_headphone_protected.json`.
- Add matching metadata at
  `examples/design/kicad-backed/class_ab_headphone_protected.metadata.json`.
- Base the request on `class_ab_headphone_driver`, preserving:
  - `opamp_gain_stage`;
  - `class_ab_output_stage`;
  - `headphone_output_protection`;
  - headphone connector return/reference wiring;
  - explicit single-supply rail, signal, bias-reference, and output net aliases.
- Verify whether the current schematic writer, PCB writer, and net assignment
  pipeline can preserve a net-tie symbol plus footprint between `LOAD_REF` and
  `GND`; if not, keep the fixture expected-fail with that infrastructure
  blocker named.
- Choose an initial metadata readiness based on actual evidence:
  - keep `expected_fail` if component, layout, routing, or KiCad evidence is
    missing;
  - use `candidate` only if the fixture satisfies the promotion policy.
- Resolve the baseline `component_selection.gain.opamp` blocker far enough for
  later phases to measure schematic and PCB evidence. Prefer an existing
  verified catalog op-amp that matches the gain-stage block query; if no
  suitable catalog seed exists, add the narrowest clearly named educational
  seed needed for this fixture, mark it non-fabrication in metadata, and keep
  fabrication-readiness warnings explicit.
- Avoid acceptance strings that fail before amplifier evidence can be measured,
  unless that parse-time blocker is intentionally being documented.
- Update `examples/design/kicad-backed/README.md` with the fixture and current
  status.

Acceptance:

- The fixture appears in the optional promotion queue.
- Metadata describes current blockers accurately.
- The fixture is not silently skipped.

Validation:

```text
go test -v ./internal/designworkflow -run TestDesignExamplesOptionalKiCadBackedTier -count=1
```

Prism:

```text
prism review staged
```

Commit:

```text
Add protected amplifier KiCad fixture
```

## Phase 3: Gate Protected Output Evidence

Goal: make tests fail if the fixture no longer proves the protected output path.

Tasks:

- Extend promotion fixture tests to assert that the amplifier fixture reports
  `headphone_output_protection` evidence when the block-planning stage runs.
- Assert stable fields for:
  - load kind;
  - nominal load impedance;
  - AC coupling state;
  - DC-blocking capacitance;
  - bleed resistor policy;
  - series resistor policy;
  - connector return/reference policy;
  - fault-protection status;
  - readiness and blockers.
- Ensure expected-fail outcomes still require meaningful blockers rather than
  generic failure.
- Ensure candidate outcomes require generated project artifacts and configured
  evidence gates.
- Verify promotion report evidence is deterministic for the same input request
  by checking stable protected-output fields across repeated runs or fixture
  summary construction.
- Add narrow regression tests for report summaries if the current test helpers
  do not expose these assertions.

Acceptance:

- A missing `headphone_output_protection` block or summary fails the optional
  fixture test.
- Expected-fail metadata cannot pass with stale or empty blockers.
- Candidate metadata cannot pass without protected-output evidence.

Validation:

```text
go test -v ./internal/designworkflow -count=1
go test ./...
```

Prism:

```text
prism review staged
```

Commit:

```text
Gate protected amplifier promotion evidence
```

## Phase 4: Capture Schematic, PCB, And Route Blockers

Goal: push the fixture as far through the generated-design workflow as current
writers and engines allow, then classify the actual blockers.

Tasks:

- Run the fixture through `design create` with artifacts kept.
- Keep generated outputs under ignored locations such as `examples/.generated`
  or temporary directories, and confirm no large generated KiCad artifacts are
  staged accidentally.
- Inspect schematic electrical checks for rail, label, power, and load-reference
  issues.
- Inspect PCB realization for missing footprints, missing net declarations,
  missing outlines, or invalid pad/net assignments.
- Enable routing only if the fixture has sufficient placement and pad-anchor
  evidence to make route checks meaningful.
- If routing runs, require route/contact evidence for the key nets that the
  current router attempts.
- If placement or routing fails, report exact operation categories, net names,
  and missing-anchor details.
- Update fixture metadata known gaps and expected stages to match the observed
  workflow result.

Acceptance:

- The promotion report identifies the first true generated-design blocker.
- Metadata expected stages match the actual run.
- Stale blockers are removed.
- No candidate promotion is granted for merely parseable files.

Validation:

```text
go test -v ./internal/designworkflow -count=1
go test -v ./internal/evaluate ./internal/routing ./internal/schematicpcb -count=1
go test ./...
```

Prism:

```text
prism review staged
```

Commit:

```text
Classify protected amplifier validation blockers
```

## Phase 5: Optional Real KiCad Evidence

Goal: capture and classify real KiCad ERC/DRC artifacts without making KiCad a
default test dependency.

Tasks:

- Run the optional fixture tier with `KICADAI_KICAD_CLI` configured when
  available.
- Confirm that missing local KiCad is represented as skipped external evidence
  and still blocks candidate/pass when required.
- When KiCad runs, preserve ERC/DRC artifacts in the fixture output directory
  and reference them from `.kicadai/design-promotion.json`.
- Ensure the workflow creates `.kicadai/checks` when checks run with artifact
  preservation enabled.
- Record the local KiCad CLI version in the promotion evidence when the
  existing check runner exposes it, or add a blocker/note when the version
  cannot be determined.
- Classify KiCad findings as:
  - blocking;
  - warning-only;
  - known local tool instability;
  - unexpected success.
- Do not suppress real amplifier-specific ERC/DRC issues.

Acceptance:

- Real KiCad evidence is visible in the promotion report when available.
- Missing KiCad evidence is not mistaken for candidate readiness.
- Candidate promotion, if achieved, includes required ERC/DRC gates.

Validation:

```text
KICADAI_KICAD_CLI=/path/to/kicad-cli go test -v ./internal/designworkflow -run TestDesignExamplesOptionalKiCadBackedTier -count=1
go test ./...
```

Prism:

```text
prism review staged
```

Commit:

```text
Capture protected amplifier KiCad evidence
```

## Phase 6: Sync Readiness, Roadmap, And Docs

Goal: make project-facing status match the captured evidence.

Tasks:

- Update `data/ai-readiness/matrix/amplifier.json` only if evidence justifies a
  readiness or blocker change.
- Update `specs/ROADMAP.md` to show the protected Class AB headphone path status
  and next blocker.
- Update README and focused docs where they describe amplifier generation,
  optional KiCad-backed fixtures, or readiness limits.
- If the fixture remains expected-fail, state the next smallest blocker to
  remove.
- If the fixture reaches candidate, state exactly which risks still prevent
  pass/fabrication readiness.

Acceptance:

- Documentation does not describe stale op-amp-buffer blockers as the primary
  protected Class AB path blocker.
- Users can tell whether the protected amplifier path is connectivity-only,
  expected-fail with evidence, or candidate.
- The next roadmap task is concrete and traceable to the fixture evidence.

Validation:

```text
go test ./...
git diff --check
```

Prism:

```text
prism review staged
```

Commit:

```text
Document protected amplifier KiCad readiness
```
