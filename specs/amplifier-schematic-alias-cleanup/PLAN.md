# Amplifier Schematic Alias Cleanup Implementation Plan

## Phase 1: Baseline and Failing Evidence Lock

Objective: capture the current protected amplifier alias blocker in narrow tests
before changing behavior.

Tasks:

- Run the relevant design example and amplifier workflow tests to confirm the
  two current conflict pairs.
- Add or tighten assertions that identify the protected fixture's stage status,
  conflict pairs, and downstream skipped stages.
- Record a small baseline note if the current evidence differs from the roadmap.
- Avoid changing promotion behavior in this phase.

Acceptance:

- Tests still show the protected fixture blocked by the known alias conflicts.
- The blocker is expressed in a way that can be cleanly updated in later phases.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 2: Alias Inventory and Canonical Net Contract

Objective: define the amplifier net identity policy in code, not only in tests.

Tasks:

- Inventory emitted net and port names for `class_ab_headphone_protected`.
- Add a small, reusable resolver or contract table for amplifier block aliases
  that can support future amplifier compositions beyond this single fixture.
- Classify the current conflict names as aliases of canonical nets rather than
  independent labels.
- Preserve distinct policy for load return, load reference, signal ground, and
  power ground.
- Add unit tests for canonical selection and forbidden merges, including power
  rails and other supply nets that must not be renamed by signal-flow aliases.
- Add explicit tests proving load return, load reference, analog ground, power
  ground, and shield/chassis nets are not automatically aliased together.
- Add tests proving signal-to-power shorts and conflicting reference-domain
  metadata produce validation errors before alias suppression.
- Implement or verify raw connectivity graph validation for short detection
  before name resolution.
- Add deterministic tie-breaker tests for equal-precedence candidates.
- Add tests proving multiple distinct user-requested names on one conductor are
  hard validation errors.
- Ensure invalid duplicate classifications produce validation issues rather
  than being silently dropped.
- Verify block contracts support direction metadata for forward, feedback, and
  sense paths, or add the minimum schema support needed for deterministic
  topological preference.
- Audit and update the affected Class AB headphone block contracts with the
  directionality and role metadata required by the resolver.

Acceptance:

- The resolver chooses one canonical name for each current conflict pair.
- Tests prove protected output/load-reference nets are not accidentally merged.
- No schematic output behavior changes are required yet.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 3: Schematic Label Emission Cleanup

Objective: stop emitting conflicting electrical labels on connected amplifier
conductors.

Tasks:

- Wire the alias resolver into schematic generation or workflow composition.
- Emit only the canonical label for resolved connected conductors.
- Preserve suppressed aliases in metadata or stage evidence.
- Keep schematic readability conventions intact: left-to-right signal flow,
  rails above/below circuitry, and labels near their conductors.
- Add tests that parse generated schematic data and confirm the old conflicts
  are absent.

Acceptance:

- `headphones_SIG` versus `output_amp_out` no longer appears as a connected-net
  conflict.
- `output_lower_emitter` versus `output_upper_emitter` no longer appears as a
  connected-net conflict.
- Suppressed aliases are observable in machine-readable evidence.
- Existing schematic readability tests remain green.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 4: Protected Fixture Workflow Promotion

Objective: advance `class_ab_headphone_protected` beyond schematic electrical
validation.

Tasks:

- Update design example tests from "expect alias blocked" to "expect alias
  resolved".
- Confirm schematic electrical validation passes or reaches a later real
  blocker.
- Update expected stages so PCB realization and subsequent stages are attempted
  when possible.
- If a new blocker appears, encode it as the fixture's new expected failure.
- Update fixture metadata known gaps.

Acceptance:

- The protected fixture no longer stops because of schematic label aliases.
- Downstream stage statuses reflect actual execution instead of alias-related
  skips.
- Metadata describes the new blocker or candidate state accurately.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 5: PCB Transfer and Route Evidence Guardrails

Objective: make sure the schematic fix survives transfer into PCB generation.

Tasks:

- Verify footprint pad net assignments use canonical amplifier net names.
- Add assertions that the old alias pairs are not split across PCB nets.
- Ensure route planning sees the canonical output, emitter midpoint, and
  headphone/load nets.
- Add evidence summary fields for resolved amplifier aliases if missing.
- Express DRC readiness as discrete gates: board outline present, all required
  footprints placed inside the outline, all electrical nets represented by
  either a route segment, zone connection, plane/zone intent, or explicit
  unrouted-net evidence item, and mechanical-only nets excluded by type.

Acceptance:

- PCB realization is attempted with canonical net names.
- Tests fail if resolved schematic aliases reappear as separate PCB nets.
- Route or validation blockers, if any, are documented as the next blocker.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 6: Optional KiCad ERC Evidence

Objective: allow real KiCad evidence to run once the local schematic blocker is
gone.

Tasks:

- Ensure the protected fixture attempts KiCad ERC when KiCad CLI is configured.
- Preserve deterministic skips when KiCad CLI is unavailable.
- Capture KiCad ERC failures as explicit promotion evidence rather than generic
  workflow failure.
- Treat KiCad ERC errors as a failing promotion gate when `require_erc` is true;
  captured ERC evidence may advance reporting, but not pass/candidate
  classification.
- Do not require DRC until the board stage produces suitable PCB artifacts.

Acceptance:

- Local tests pass without KiCad CLI.
- KiCad-backed tests run when configured and report actionable ERC evidence.
- The fixture does not silently promote past required ERC.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 7: Documentation and Roadmap Update

Objective: keep project guidance aligned with the new amplifier state.

Tasks:

- Update `specs/ROADMAP.md` with the new protected amplifier status.
- Update relevant docs or example README entries if fixture readiness changes.
- Document the alias policy where future block authors will find it.
- Note the next work item after alias cleanup.

Acceptance:

- ROADMAP no longer lists fixed alias conflicts as current blockers.
- The next blocker is concrete and test-backed.
- Documentation uses the compiled `kicadai` command form where command examples
  are needed.
- Prism review passes for staged changes.
- Commit the phase before moving on.

## Phase 8: Full Regression

Objective: close the implementation with the same checks expected for other
promotion work.

Tasks:

- Run the automated project-standard formatting and lint checks for changed
  file types where the repository provides them.
- Run targeted tests during each phase and full `go test -count=1 ./...` at the
  end with the repository-local Go build cache.
- Add or update snapshot/golden assertions for stable schematic and PCB net
  naming so repeated generation remains logically equivalent.
- Run Prism on staged changes before every commit.
- Confirm `git status --short` is clean after the final commit.

Acceptance:

- Full test suite passes.
- Prism reports no blocking findings.
- All phases are committed independently.
- The protected amplifier fixture has moved to the next real blocker or a
  candidate/pass state.
