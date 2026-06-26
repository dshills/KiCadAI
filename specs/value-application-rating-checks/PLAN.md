# Value Application And Rating Checks Implementation Plan

Date: 2026-06-26

## Phase 1: Extend Calculation Trace Model

Objective: make application and verification status representable without
changing planner behavior yet.

Tasks:

- Add optional fields to `SynthesisCalculation` for `status`, `applied`,
  `requirements`, and `issues` or an equivalent local issue DTO.
- Add `AppliedValue` and `CalculatedRequirement` model types.
- Update clone/sort helpers so new slices and maps are deep-copied and emitted
  deterministically.
- Add focused tests for JSON compatibility, cloning, and deterministic ordering.

Review:

- `go test ./internal/intentplanner`
- `prism review staged`

Commit:

- Commit with a message like `Add calculation application trace model`.

## Phase 2: Add Safe Application Rule Registry

Objective: centralize which calculations may mutate which block params.

Tasks:

- Add a small internal planner rule registry for value application.
- Represent each rule with block ID, calculation kind, result key, destination
  param, unit/format policy, and unsupported behavior.
- Add helpers for block parameter capability checks using the existing block
  registry metadata.
- Add value formatting helpers for resistance and capacitance literals accepted
  by current block validation.
- Add tests proving supported rules apply and unsupported combinations are
  deferred.

Review:

- `go test ./internal/intentplanner ./internal/blocks`
- `prism review staged`

Commit:

- Commit with a message like `Add planner value application rules`.

## Phase 3: Apply LED Resistor Values

Objective: make LED intent calculations affect generated LED blocks.

Tasks:

- Normalize LED calculation inputs so `led_current` and `led_current_ma`
  mismatches are resolved.
- Calculate LED series resistance from `supply_voltage`,
  `led_forward_voltage`, and `led_current`.
- Apply the result to `led_indicator.params.resistor_value`.
- Add calculated requirements for resistor value and resistor power when enough
  inputs are available.
- Mark invalid LED calculations as blocked with actionable issues.
- Update planner and design workflow tests for the changed block params.

Review:

- `go test ./internal/intentplanner ./internal/designworkflow ./internal/blocks`
- `prism review staged`

Commit:

- Commit with a message like `Apply LED resistor calculations`.

## Phase 4: Apply I2C Pull-Up Policy

Objective: make I2C pull-up calculations produce usable generated block params.

Tasks:

- Normalize I2C pull-up calculation inputs so explicit request values and block
  params map to `pullup_value`.
- Choose a deterministic concrete pull-up value from policy ranges:
  - high-speed or faster policy: `4.7k` unless a stricter value is already
    requested;
  - default/low-speed policy: `4.7k` or existing explicit value.
- Apply only when `include_pullups` is true or absent.
- Defer with a clear status when pull-ups are intentionally external.
- Add requirements for pull-up resistance and rail voltage rating where known.
- Add tests for explicit value preservation, default application, and external
  pull-up deferral.

Review:

- `go test ./internal/intentplanner ./internal/designworkflow ./internal/blocks`
- `prism review staged`

Commit:

- Commit with a message like `Apply I2C pull-up calculations`.

## Phase 5: Convert Regulator, Crystal, And Op-Amp Calculations To Requirements

Objective: handle calculations that are not always safe to mutate.

Tasks:

- For regulator headroom, emit calculated requirements for input voltage,
  output voltage, output current, and dropout/headroom when enough intent data
  exists.
- Treat negative headroom as blocked and marginal headroom as warning or
  blocked according to acceptance level.
- For crystal load capacitance, apply only if the current crystal block exposes
  a compatible destination parameter; otherwise mark deferred and emit
  capacitor requirements.
- For op-amp gain, preserve gain evidence and emit supply compatibility
  requirements; do not mutate feedback values unless the block exposes concrete
  feedback resistor params.
- Add tests for blocked regulator headroom, deferred unsupported application,
  and preserved op-amp evidence.

Review:

- `go test ./internal/intentplanner ./internal/blocks`
- `prism review staged`

Commit:

- Commit with a message like `Add calculated analog requirements`.

## Phase 6: Integrate Calculated Ratings With Component Selection

Objective: route calculated requirements through the existing component rating
machinery.

Tasks:

- Add a conversion layer from `CalculatedRequirement` to
  `components.RequiredRating` for supported rating kinds.
- Thread calculated rating requirements into block/component selection requests
  where role and family mapping are known.
- Reuse existing `COMPONENT_RATING_MISSING` and `COMPONENT_RATING_TOO_LOW`
  issue behavior.
- Preserve unmapped calculated requirements as trace evidence instead of
  dropping them.
- Add tests with catalog records that pass, miss, and fail calculated ratings.

Review:

- `go test ./internal/components ./internal/blocks ./internal/designworkflow`
- `prism review staged`

Commit:

- Commit with a message like `Check calculated component ratings`.

## Phase 7: Update Rationale And Workflow Artifacts

Objective: make the new behavior visible to users and AI callers.

Tasks:

- Map calculation status, applied values, requirements, and blockers into the
  design rationale report.
- Update `intent rationale` golden tests or fixtures.
- Ensure `design create` artifacts include the same trace output.
- Add concise human-readable messages for applied, deferred, and blocked
  calculations.

Review:

- `go test ./internal/intentplanner ./internal/designworkflow ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Report calculated value application`.

## Phase 8: CLI Goldens And Documentation

Objective: lock the behavior down for users and future AI integration.

Tasks:

- Add or update CLI golden coverage for `kicadai intent plan` and
  `kicadai intent rationale`.
- Update README documentation for value application and rating checks.
- Update `specs/ROADMAP.md` to reflect this project when implemented.
- Confirm docs use `kicadai` rather than `go run ./cmd/kicadai`.

Review:

- `go test ./...`
- `prism review staged`

Commit:

- Commit with a message like `Document value application checks`.

## Phase 9: Final Compatibility Sweep

Objective: ensure this did not make generated projects less stable.

Tasks:

- Run the full test suite.
- Run focused generated-design fixtures that include LEDs, I2C sensors,
  regulators, crystals, and op-amp blocks.
- Inspect representative plan/rationale JSON for deterministic ordering and
  actionable wording.
- Confirm no command requires KiCad, network access, or external source trees
  unless an optional KiCad validation flag is explicitly enabled.

Review:

- `go test ./...`
- `prism review staged`

Commit:

- Commit any remaining compatibility fixes with a message like
  `Harden value application compatibility`.

## Completion Criteria

- The planner applies LED and I2C calculated values to supported block params.
- Regulator, crystal, and op-amp calculations are either safely applied or
  explicitly deferred with requirements.
- Component rating failures from calculated requirements are visible through the
  existing issue model.
- Rationale and workflow artifacts explain applied/deferred/blocked status.
- README and roadmap are current.
- Full tests and Prism review pass.
