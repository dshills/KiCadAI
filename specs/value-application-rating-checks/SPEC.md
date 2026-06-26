# Value Application And Rating Checks Specification

Date: 2026-06-26

## Summary

KiCadAI now records semantic value calculations during intent planning, including
LED series resistor sizing, I2C pull-up policy, regulator headroom, crystal load
capacitor sizing, and op-amp gain ratio evidence. Those calculations are useful
for explanations, but they are not yet consistently applied to the generated
block parameters or checked against selected component ratings.

This project closes that gap. The planner must convert supported calculations
into safe block parameter mutations, preserve deterministic trace evidence for
what was applied or deferred, and feed calculated requirements into component
rating checks where the current component model can enforce them.

## Goals

- Apply calculated values to generated design blocks when the target block
  explicitly supports the destination parameter.
- Preserve every calculation as traceable planner evidence, including whether it
  was applied, deferred, or blocked.
- Translate planner calculations into component rating requirements when the
  requirement can be expressed by the existing component selection model.
- Detect impossible or unsafe calculated values before `design create` writes a
  project.
- Keep the design rationale report honest by distinguishing applied values,
  deferred values, and rating blockers.
- Maintain deterministic behavior without live datasheet lookup, network
  access, or hidden LLM decisions.

## Non-Goals

- Do not perform arbitrary analog circuit synthesis.
- Do not mutate unsupported blocks or imported KiCad projects.
- Do not add live distributor, sourcing, or datasheet scraping.
- Do not guarantee production fitness beyond the local verified catalog,
  block rules, writer checks, and configured KiCad validation.
- Do not optimize exact E-series part values unless the local component catalog
  and block implementation already provide enough evidence.
- Do not replace the existing component catalog or selection API.

## Existing Foundation

- `internal/intentplanner/synthesis.go` records calculation evidence through
  `recordValueCalculationTrace`.
- `internal/intentplanner/plan.go` defines `SynthesisCalculation`.
- `internal/intentplanner/map.go` already records intent-derived power rating
  strings for component policy explanations.
- `internal/components/selection.go` supports `RequiredRating` and rejects
  candidates with missing or insufficient ratings.
- `internal/blocks/builtin.go` defines supported block parameters:
  - `led_indicator`: `supply_voltage`, `led_forward_voltage`, `led_current`,
    `resistor_value`.
  - `i2c_sensor`: `supply_voltage`, `pullup_value`, `include_pullups`,
    `decoupling_value`.
  - `voltage_regulator`: `input_voltage`, `output_voltage`, related package
    and LED options.
  - `opamp_gain_stage`: `gain` and feedback footprint controls.
- The current calculation trace has naming mismatches that must be resolved
  before application, such as `led_current_ma` vs `led_current` and
  `pullup_ohms` vs `pullup_value`.

## Core Design

### Calculation Lifecycle

Each synthesis calculation moves through three deterministic stages:

1. `calculate`: derive a numeric or policy result from normalized intent and
   block parameters.
2. `apply`: write supported values into selected block parameters through an
   allowlisted rule.
3. `verify`: emit component rating requirements or planner blockers where the
   generated value requires downstream proof.

The trace should make these stages visible.

Recommended additions to `SynthesisCalculation`:

```go
type SynthesisCalculation struct {
    ID           string                  `json:"id"`
    Kind         string                  `json:"kind"`
    Path         string                  `json:"path,omitempty"`
    Inputs       map[string]string       `json:"inputs,omitempty"`
    Result       map[string]string       `json:"result,omitempty"`
    Formula      string                  `json:"formula,omitempty"`
    Assumptions  []string                `json:"assumptions,omitempty"`
    Confidence   string                  `json:"confidence,omitempty"`
    Status       string                  `json:"status,omitempty"` // applied, deferred, blocked
    Applied      []AppliedValue          `json:"applied,omitempty"`
    Requirements []CalculatedRequirement `json:"requirements,omitempty"`
    Issues       []reports.Issue         `json:"issues,omitempty"`
}

type AppliedValue struct {
    Target string `json:"target"` // block, component, policy
    Path   string `json:"path"`   // blocks.status.params.resistor_value
    Value  string `json:"value"`
    Unit   string `json:"unit,omitempty"`
    Method string `json:"method,omitempty"` // calculated, default_policy, explicit_request
}

type CalculatedRequirement struct {
    Subject  string `json:"subject"` // component role, net, block, or catalog query
    Kind     string `json:"kind"`    // voltage, current, power, capacitance, resistance
    Operator string `json:"operator,omitempty"`
    Value    string `json:"value"`
    Unit     string `json:"unit,omitempty"`
    Source   string `json:"source,omitempty"`
}
```

If importing `reports` into this part of the planner creates a cycle or awkward
ownership, `Issues` can be represented by a small local issue DTO and converted
at workflow boundaries.

### Application Rules

Application must be allowlisted by block ID, calculation kind, and destination
parameter. Rules should be local, deterministic, and easy to audit.

Initial supported mappings:

| Block | Calculation | Result Key | Destination Param | Status |
| --- | --- | --- | --- | --- |
| `led_indicator` | `led_resistor` | `resistance_ohms` | `resistor_value` | applied |
| `i2c_sensor` | `i2c_pullup` | explicit or selected range value | `pullup_value` | applied when pull-ups are included |
| `voltage_regulator` | `regulator_headroom` | `headroom_v` | none | requirement/warning only |
| `crystal_oscillator` | `crystal_load_cap` | `capacitor_pf_each` | existing load-cap parameter if supported | applied only when the block exposes a compatible parameter |
| `opamp_gain_stage` | `opamp_gain` | `rf_over_rg` | none initially | requirement/evidence only because the block already derives feedback values from `gain` |

Unsupported mappings must remain visible as `deferred`, not silently ignored.

### Value Formatting

Applied values must use block-compatible engineering literals:

- LED resistance: format as an ohm literal accepted by block validation, for
  example `260`, `1k`, or normalized `260Ω` depending current block parsing.
- I2C pull-up: preserve explicit request when present; otherwise choose the
  conservative default from the planner policy and write `4.7k` or `10k` rather
  than a range string.
- Capacitance: write `pF`/`nF` literals accepted by block validation.
- Voltage/current inputs: normalize only when needed for calculations; do not
  rewrite user-provided literals unless the destination parameter is explicitly
  an applied calculated value.

### Rating Requirements

Calculated requirements should feed the existing component selection and
validation paths instead of creating a second rating system.

Initial requirements:

- LED resistor:
  - resistance value requirement for resistor catalog query when the block
    participates in component selection;
  - resistor power requirement when supply voltage, LED forward voltage, and
    current are known.
- LED:
  - forward-current requirement when current is known;
  - rail/polarity evidence remains block-level validation, not a rating check.
- I2C pull-ups:
  - resistance value requirement for pull-up resistors;
  - voltage rating requirement at least equal to the I2C rail where catalog
    ratings exist.
- Regulator:
  - input voltage, output voltage, output current, and dropout/headroom
    constraints where the intent provides enough power data.
  - negative or too-small headroom is a planner blocker for
    fabrication-candidate acceptance and at least a warning for lower
    acceptance levels.
- Crystal load capacitors:
  - capacitance requirement;
  - voltage rating requirement tied to the oscillator rail when known.
- Op-amp gain:
  - preserve gain evidence;
  - require op-amp supply compatibility when supply voltage is known;
  - defer exact feedback resistor value constraints until the block exposes
    selected resistor values or catalog queries for them.

### Failure Policy

- Invalid calculations, such as non-positive LED current, supply voltage below
  LED forward voltage, negative regulator headroom, or non-positive load
  capacitance, must produce blocked trace status and planner issues.
- Missing block support must produce deferred trace status and an actionable
  gap, not a generated file that pretends the value was used.
- Missing catalog rating evidence must reuse existing
  `COMPONENT_RATING_MISSING` behavior where selection is attempted.
- Insufficient catalog rating evidence must reuse existing
  `COMPONENT_RATING_TOO_LOW` behavior.
- Fabrication-candidate requests should fail closed on blocked calculations or
  missing required rating evidence. Lower acceptance levels may continue with
  explicit warnings if the generated design remains structurally valid.

## CLI And Artifacts

No new top-level CLI command is required.

Existing commands should expose the new evidence:

- `kicadai intent plan`: calculation statuses, applied values, and requirements
  appear in the plan JSON.
- `kicadai intent rationale`: the rationale report includes applied values,
  deferred calculations, and rating blockers.
- `kicadai design create`: generated workflow artifacts include the same trace
  and fail according to acceptance policy.

## Compatibility

- Existing plan JSON remains readable. New fields are optional.
- Existing fixtures should not change unless they intentionally assert the new
  calculation status, applied values, or rating requirements.
- Generated KiCad output should only change where a block previously omitted a
  supported calculated parameter.

## Testing Strategy

- Unit tests for each calculation result, status, and failure condition.
- Unit tests for application rules, including supported, deferred, and blocked
  cases.
- Planner tests proving LED resistor and I2C pull-up values are written into
  block params when supported.
- Planner tests proving regulator, crystal, and op-amp requirements are
  preserved without unsafe mutation.
- Component tests proving calculated required ratings are passed to selection
  and produce existing rating errors.
- Rationale tests proving applied/deferred/blocked calculation evidence is
  rendered.
- CLI golden tests for `intent plan` and `intent rationale`.
- Full `go test ./...` must pass without requiring KiCad, network access, or
  external source trees.

## Acceptance Criteria

- Supported calculated values are applied through explicit allowlist rules.
- Unsupported calculated values are reported as deferred with an actionable
  message.
- Impossible values are blocked before project generation.
- Calculated rating requirements are traceable from intent to selection result
  or rationale report.
- Rating failures use the existing component issue codes where possible.
- The generated plan and rationale explain what changed and what remains
  unresolved.
- Existing workflows remain deterministic and backwards compatible.
