# LED Prompt Strict Promotion Plan

## Overview

This plan promotes the simple LED text prompt from AI-lane candidate to strict
promotion readiness. The work is intentionally narrow: one deterministic prompt,
one generated project shape, and evidence-driven promotion gates.

The plan should be implemented in phases. After each phase:

1. run focused tests;
2. stage the phase changes;
3. run `prism review staged`;
4. fix high and medium findings;
5. commit before moving to the next phase.

## Phase 1: Baseline And Promotion Gate Inventory

### Goals

- Freeze the current strict-promotion blockers in tests.
- Make the LED prompt promotion gap visible without relying on manual JSON
  inspection.

### Tasks

- Add a focused test that runs:

  ```sh
  kicadai --text "make a simple LED indicator board" --output <temp> --overwrite intent create
  ```

- Decode `.kicadai/design-promotion.json`.
- Assert the current failing gates and issue categories:
  - component selection placeholder warnings;
  - `indicator.VCC` endpoint-to-pad warning;
  - missing GND net class;
  - pinmap/writer correctness warning;
  - skipped KiCad checks when KiCad CLI is not configured.
- Keep the test structured so later phases can invert individual assertions.
- Document the baseline in the spec directory if useful.

### Acceptance

- Focused test proves the exact current blocker inventory.
- Default `go test ./cmd/kicadai ./internal/designworkflow` passes.
- No behavior is changed yet except test coverage.

### Review And Commit

- Run focused tests.
- Run `prism review staged`.
- Commit: `Baseline LED strict promotion gates`.

## Phase 2: Component Selection Evidence Closeout

### Goals

- Remove connector placeholder component-selection warnings for the LED prompt.

### Tasks

- Trace the component-selection warning path for block components with no
  `component_id` or `component_query`.
- Decide whether generated connectors should select checked-in catalog records
  or be classified as verified synthetic block components.
- Implement the narrow policy:
  - retain warnings for truly missing component evidence;
  - suppress or downgrade only verified synthetic block-local components;
  - emit explicit workflow evidence identifying the evidence source.
- Update tests for connector and LED prompt component-selection summaries.

### Acceptance

- LED prompt no longer emits component-selection placeholder warnings.
- Missing evidence in unrelated blocks still warns or blocks as before.
- Workflow summaries expose selected or synthetic evidence source.

### Review And Commit

- Run focused component-selection and LED prompt tests.
- Run `prism review staged`.
- Commit: `Close LED prompt component evidence gap`.

## Phase 3: Endpoint-To-Pad Mapping Closeout

### Goals

- Resolve `indicator.VCC` and all LED prompt endpoints to generated PCB pads or
  validated local-route anchors.

### Tasks

- Use Atlas to locate the endpoint resolution path for placement and
  inter-block routing.
- Inspect LED block fragment metadata, port names, pin maps, and generated pad
  summaries.
- Fix the source of the mismatch:
  - block port aliasing;
  - schematic-to-PCB transfer metadata;
  - footprint pad naming;
  - or local-route anchor export.
- Add regression tests that assert `indicator.VCC` resolves.
- Update the LED prompt promotion test to remove the endpoint warning from the
  expected blocker set.

### Acceptance

- No `connection endpoint does not resolve to a generated PCB pad` warning for
  the LED prompt.
- No disconnected-pad blocker is introduced.
- Existing I2C/amplifier expected-fail fixtures retain honest blockers.

### Review And Commit

- Run focused placement/routing workflow tests.
- Run `prism review staged`.
- Commit: `Resolve LED prompt endpoint pad mapping`.

## Phase 4: Net Class And Route Warning Closeout

### Goals

- Give generated LED prompt nets explicit net class evidence.
- Prevent non-blocking fixed-net preservation evidence from downgrading strict
  promotion readiness.

### Tasks

- Add deterministic default net classes for generated simple designs when the
  request does not provide net class policy.
- Ensure GND/VCC/signal nets receive trace width, clearance, and role metadata
  consistent with existing routing rules.
- Serialize net class evidence into workflow summaries and PCB output where
  supported.
- Review promotion route-completion gate classification for
  `FIXED_NET_SKIPPED` info findings:
  - keep info evidence visible;
  - stop treating connected fixed-net preservation as a readiness warning.
- Add tests for:
  - GND net class present;
  - route gate does not warn only because fixed connected copper was preserved;
  - actual route graph/contact failures still warn or block.

### Acceptance

- LED prompt no longer emits `MISSING_NET_CLASS`.
- Promotion route-completion gate is pass or warn only for genuine route
  incompleteness.
- Fixed-net info findings remain available in detailed evidence.

### Review And Commit

- Run routing and promotion-builder tests.
- Run `prism review staged`.
- Commit: `Close LED prompt route evidence gaps`.

## Phase 5: Writer Correctness And Pinmap Evidence Closeout

### Goals

- Remove LED prompt writer-correctness warnings caused by missing pad net hints
  or unverified pinmaps.

### Tasks

- Trace schematic-to-PCB transfer evidence for LED, resistor, and connector
  symbols.
- Ensure verified fallback templates or resolver-backed pinmaps are carried into
  transaction pad-net hints.
- Make writer correctness distinguish verified fallback pinmaps from unknown
  pinmaps.
- Add tests that assert:
  - LED prompt transaction includes pad-net hints;
  - writer correctness does not warn for missing library index when verified
    built-in templates cover every footprint in the prompt;
  - unrelated unsupported footprints still warn.

### Acceptance

- LED prompt no longer emits `PINMAP_UNVERIFIED`.
- LED prompt no longer emits `library index not provided; generated placement
  transaction will omit pad net hints`.
- Unsupported footprints still produce writer-correctness warnings.

### Review And Commit

- Run writer correctness, schematic-to-PCB, and LED prompt tests.
- Run `prism review staged`.
- Commit: `Verify LED prompt pinmap evidence`.

## Phase 6: Strict Promotion Candidate Gate

### Goals

- Make no-KiCad default output reach strict promotion `candidate` when all
  non-KiCad gates pass and skipped KiCad gates are policy-optional.

### Tasks

- Review promotion-builder readiness rules for candidate readiness.
- Decide and encode policy for skipped KiCad gates in default structural mode:
  - either allow `candidate` with explicit skipped-KiCad caveat; or
  - keep strict candidate KiCad-required and update docs/tests accordingly.
- Prefer narrow fixture/request metadata over global relaxation.
- Update the LED prompt golden test to assert the chosen promotion readiness.
- Ensure `data.ai_status` and promotion report remain distinct.

### Acceptance

- LED prompt promotion report reaches the intended strict default readiness.
- Reports still state when KiCad evidence is missing.
- Other promotion fixtures do not regress.

### Review And Commit

- Run promotion-builder, designworkflow, and CLI prompt tests.
- Run `prism review staged`.
- Commit: `Promote LED prompt to strict candidate`.

## Phase 7: KiCad-Backed Pass Path

### Goals

- Make real KiCad ERC/DRC evidence promote the LED prompt to pass when clean.

### Tasks

- Extend the optional KiCad smoke test to assert promotion pass when checks are
  clean, or precise blockers when KiCad finds real issues.
- Persist KiCad version/path/check artifact evidence in the promotion report.
- Ensure `--require-erc --require-drc` cannot silently downgrade to optional
  skipped checks.
- If KiCad exposes legitimate ERC/DRC findings, either:
  - fix the writer/model issue; or
  - mark the optional test as expected-fail with exact findings until the writer
    issue is addressed.

### Acceptance

- Default tests still skip without KiCad.
- With `KICADAI_KICAD_CLI`, `kicad_checks` is not skipped.
- Clean KiCad evidence reaches promotion `pass`.
- Dirty KiCad evidence reports exact blockers.

### Review And Commit

- Run optional KiCad smoke if KiCad is configured.
- Run default focused tests without requiring KiCad.
- Run `prism review staged`.
- Commit: `Add KiCad-backed LED prompt pass gate`.

## Phase 8: Documentation And Roadmap Update

### Goals

- Keep user and agent docs aligned with the stricter readiness model.

### Tasks

- Update README with the new LED prompt strict-promotion status.
- Update `docs/intent-planning.md`.
- Update `docs/kicadai-agent-skill.md`.
- Update `docs/validation-and-analysis.md` if promotion semantics changed.
- Update `specs/ROADMAP.md`:
  - mark this roadmap item implemented if done;
  - add the next gap clearly.

### Acceptance

- Docs distinguish AI-lane candidate, promotion candidate, and promotion pass.
- Docs do not imply arbitrary AI generation.
- Commands use `kicadai`.

### Review And Commit

- Run doc-relevant tests if any examples changed.
- Run `prism review staged`.
- Commit: `Document LED prompt strict promotion`.

## Phase 9: Full Regression

### Goals

- Prove the closeout did not regress the rest of KiCadAI.

### Tasks

- Run:

  ```sh
  go test ./...
  ```

- Run the LED prompt manually into `examples/.generated/ai_led_prompt_strict`.
- Inspect:
  - `.kicadai/validation-summary.json`;
  - `.kicadai/workflow-result.json`;
  - `.kicadai/design-promotion.json`;
  - generated project, schematic, and PCB files.
- Confirm `git status --short` is clean after commits.

### Acceptance

- Full tests pass.
- Manual LED prompt output matches documented readiness.
- All phase commits are present.

### Review And Commit

- If Phase 9 creates code/doc changes, review and commit them.
- Otherwise record verification in the final summary.

