# Semantic Design Synthesis Implementation Plan

Date: 2026-06-26

## Phase 1: Synthesis Trace Model

Add the data model without changing planner behavior.

Tasks:

1. Add synthesis trace types to `internal/intentplanner`:
   - `SynthesisTrace`;
   - `SynthesisDecision`;
   - `SynthesisEvidence`;
   - `SynthesisConstraint`;
   - `SynthesisCalculation`;
   - `SynthesisGap`.
2. Add schema constant `kicadai.intent.synthesis.v1`.
3. Add a `Synthesis SynthesisTrace` field to `PlanResult`.
4. Normalize and deterministically sort trace records in `NormalizePlan`.
5. Derive synthesis status from plan status and trace gaps.
6. Add unit tests for empty trace, populated sorting, JSON stability, and
   backwards-compatible fixture decoding.

Acceptance:

- Existing plan tests pass.
- Existing JSON fixtures remain readable.
- Minimal plans include an empty or ready normalized synthesis trace.

Commit message:

`Add semantic synthesis trace model`

## Phase 2: Planner Trace Recording Helpers

Add planner-internal helpers for recording synthesis evidence while preserving
existing behavior.

Tasks:

1. Add `planBuilder` helper methods:
   - `recordSynthesisDecision`;
   - `recordSynthesisEvidence`;
   - `recordSynthesisConstraint`;
   - `recordSynthesisCalculation`;
   - `recordSynthesisGap`.
2. Link records to requirement IDs where available.
3. Record current component policy as synthesis constraints.
4. Record current validation/routing policy as synthesis decisions.
5. Add tests that current known fixtures now contain trace evidence without
   changing generated requests.

Acceptance:

- Trace exists for current supported plans.
- Generated `designworkflow.Request` output remains stable except for added
  plan metadata.

Commit message:

`Record planner synthesis evidence`

## Phase 3: Voltage-Domain Table

Centralize voltage-domain reasoning.

Tasks:

1. Add an internal voltage-domain table builder.
2. Populate domains from inputs, rails, regulator sources, block supplies, and
   explicit supply aliases.
3. Record voltage/current constraints into the synthesis trace.
4. Block unknown supply aliases and incompatible explicit voltages.
5. Emit assumptions for inferred compatible rails.
6. Add golden tests for explicit aliases, inferred single rail, ambiguous
   multi-rail, and current/rating constraints.

Acceptance:

- Voltage-domain choices are visible in `intent-plan.json`.
- Bad voltage-domain requests fail before `design create`.

Commit message:

`Synthesize voltage domain evidence`

## Phase 4: Interface And Bus Synthesis

Strengthen interface resolution beyond first-pass I2C.

Tasks:

1. Emit bus-resolution decisions for I2C, UART, SPI/ISP, and GPIO connector
   intent.
2. Add UART connector/programming synthesis where supported by current block
   ports.
3. Add SPI/ISP trace evidence for reset/programming support.
4. Add I2C pull-up requirement detection and trace gaps when no modeled pull-up
   source exists.
5. Preserve fail-closed behavior for unsupported GPIO pin assignment.
6. Add golden tests for I2C, UART, ISP, and unsupported GPIO requests.

Acceptance:

- Supported bus topology decisions are explicit and deterministic.
- Unsupported pin assignment is a precise known gap/blocker.

Commit message:

`Synthesize interface topology evidence`

## Phase 5: Value Calculation Foundation

Add planner-visible calculation records for simple supported policies.

Tasks:

1. Implement calculation helpers for:
   - LED resistor;
   - I2C pull-up recommendation;
   - regulator headroom;
   - decoupling policy;
   - crystal load capacitor estimate;
   - op-amp gain ratio.
2. Record calculation inputs, formulas, assumptions, and confidence.
3. Add warnings/gaps for missing inputs where exact values are impossible.
4. Do not mutate block internals unless the target block already supports the
   parameter safely.
5. Add tests for successful calculations and deferred calculations.

Acceptance:

- Plans explain value decisions or deferrals.
- No generated schematic changes are made without explicit block support.

Commit message:

`Add planner value calculation evidence`

## Phase 6: External Clock Topology Gate

Make external-clock intent more actionable while still safe.

Tasks:

1. Represent internal clock, crystal, and canned oscillator topology decisions.
2. Keep unsupported external-clock generation blocked until the MCU block can
   emit the necessary schematic/PCB topology.
3. When block support exists, connect XTAL/clock roles through generated
   request connections.
4. Record required placement proximity and local-route evidence.
5. Add tests for blocked external clock and supported topology path.

Acceptance:

- Current unsupported cases produce a precise synthesis gap.
- Future block support has a defined implementation path and acceptance tests.

Commit message:

`Gate external clock synthesis safely`

## Phase 7: Component Constraint Synthesis

Make component selection constraints planner-visible.

Tasks:

1. Convert package preferences, confidence, acceptance, voltage, current,
   lifecycle, and fabrication intent into synthesis constraints.
2. Link selected component records to constraints.
3. Include rejected/blocked constraint reasons where component selection already
   exposes them.
4. Add tests for fabrication-candidate, connectivity, package preference, and
   placeholder-policy cases.

Acceptance:

- Component policy decisions are explainable before workflow execution.
- Fabrication-candidate plans clearly require verified evidence.

Commit message:

`Synthesize component selection constraints`

## Phase 8: Rationale Mapping

Expose synthesis trace data in `design-rationale.json`.

Tasks:

1. Map synthesis decisions to rationale decisions.
2. Map synthesis evidence, constraints, and calculations to rationale evidence.
3. Map synthesis gaps to known limits.
4. Add next actions for common synthesis gaps:
   - unknown supply alias;
   - unsupported peripheral;
   - missing value inputs;
   - unsupported external clock topology;
   - insufficient component evidence.
5. Add unit and CLI tests for `intent rationale`.

Acceptance:

- AI callers can read the rationale report without separately parsing the full
  plan to understand synthesis choices.

Commit message:

`Map synthesis trace into rationale reports`

## Phase 9: CLI Goldens And Examples

Add durable examples for the richer synthesis behavior.

Tasks:

1. Add or update `examples/intent/` fixtures for:
   - MCU I2C sensor with explicit supply;
   - UART programming connector;
   - ISP programming;
   - regulator headroom warning;
   - blocked unknown supply;
   - blocked or supported external clock.
2. Add CLI golden tests for plan/explain/rationale snapshots where practical.
3. Ensure generated request shape remains stable.
4. Document fixture purpose in `examples/README.md` or relevant README section.

Acceptance:

- Future changes can detect synthesis regressions from golden output.

Commit message:

`Add semantic synthesis fixtures`

## Phase 10: Documentation And Roadmap Update

Document the new synthesis layer.

Tasks:

1. Update `README.md` intent planner section.
2. Update `specs/ROADMAP.md` Priority 9 foundation and remaining work.
3. Document trace fields and examples.
4. Keep command examples using compiled `kicadai`.

Acceptance:

- Project status clearly describes what the semantic synthesis layer does and
  what remains blocked.

Commit message:

`Document semantic design synthesis`

## Phase 11: Review And Compatibility Sweep

Finalize with full validation.

Tasks:

1. Run `go test ./...`.
2. Run `prism review staged`.
3. Fix all high/medium findings.
4. Confirm normal tests do not require KiCad CLI, network access, or external
   library roots.
5. Commit any final cleanup.

Acceptance:

- Full tests pass.
- Prism has no unresolved high/medium findings.
- Work is committed phase by phase.

Commit message:

`Harden semantic design synthesis`
