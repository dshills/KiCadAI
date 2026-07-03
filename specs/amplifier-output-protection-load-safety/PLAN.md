# Amplifier Output Protection And Load Safety Implementation Plan

## Phase 1: Protection Block Model And Validation

Goal: introduce a conservative block contract for headphone output protection
without changing generation behavior yet.

Tasks:

- Add a `headphone_output_protection` block definition or equivalent internal
  model following the spec.
- Define exported ports:
  - `AMP_OUT`
  - `HP_OUT`
  - `LOAD_RET`
  - `LOAD_REF`
- Add structured params for:
  - `load_kind`
  - `nominal_load_ohms`
  - `dc_blocking_capacitance`
  - `bleed_resistor_ohms`
  - `series_resistor_ohms`
  - `connector_return_policy`
  - `fault_protection_status`
- Validate supported load classes: 16, 32, and 64 ohm headphones.
- Fail closed for unsupported values:
  - speaker load;
  - bridge-tied output;
  - unknown load kind;
  - impedance below supported headphone range;
  - missing return/reference policy where required.
- Add unit tests for model defaults, accepted headphone loads, and blocked
  unsupported loads.

Validation:

- `go test ./internal/blocks`
- `go test ./...`
- `prism review staged`

Commit:

- `Add headphone output protection block model`

## Phase 2: Schematic Realization

Goal: emit human-readable schematic operations for the supported headphone
output-protection fragment.

Tasks:

- Implement block realization for `headphone_output_protection`.
- Emit a DC-blocking capacitor between `AMP_OUT` and `HP_OUT`.
- Emit the output connector signal/return landmarks or connector-facing labels
  required by the current block realization model.
- Emit optional bleed/reference resistor when the policy requires it.
- Emit optional series output resistor when requested by params.
- Ensure generated labels are explicit:
  - `AMP_OUT_DC_BIASED`
  - `HP_OUT`
  - `LOAD_REF`
- Keep the fragment readable left-to-right and compatible with existing
  schematic readability checks.
- Add tests for expected operations, labels, component values, and deterministic
  capacitor polarity/orientation.

Validation:

- `go test ./internal/blocks`
- `go test ./internal/designworkflow`
- `go test ./...`
- `prism review staged`

Commit:

- `Realize headphone output protection schematic block`

## Phase 3: Class AB Driver Integration

Goal: update the Class AB headphone design path to use the explicit
output-protection/load-safety block instead of a bare DC-blocking capacitor
where appropriate.

Tasks:

- Update `examples/design/amplifier/class_ab_headphone_driver.json` to use
  `headphone_output_protection` for the output path, or add a new paired
  example if preserving the bare-capacitor fixture is useful.
- Connect:
  - Class AB output `AMP_OUT` to protection `AMP_OUT`;
  - protection `HP_OUT` to headphone connector signal;
  - protection `LOAD_RET`/`LOAD_REF` to the load reference/return path.
- Ensure the existing Class AB diagnostics still detect AC output coupling.
- Update or add workflow tests for the integrated Class AB headphone driver.
- Keep the KiCad-backed amplifier candidate expected-fail unless separate
  validation evidence justifies promotion.

Validation:

- `go test ./internal/designworkflow`
- `go test ./internal/blocks`
- `go test ./...`
- `prism review staged`

Commit:

- `Integrate headphone protection into Class AB driver`

## Phase 4: Workflow Diagnostics And Rationale

Goal: make output-protection/load-safety state explicit for AI callers.

Tasks:

- Add a structured output-protection summary to block planning or design
  workflow output.
- Include fields for:
  - instance ID;
  - load kind;
  - nominal load impedance;
  - AC output coupling present;
  - DC-blocking capacitance;
  - bleed policy status;
  - series output resistor status;
  - connector return/reference status;
  - fault protection status;
  - readiness;
  - blockers and notes.
- Ensure non-blocking warnings remain notes and blocking issues drive blocked
  readiness.
- Update rationale output if needed so generated reports explain headphone
  output-protection assumptions.
- Add tests for:
  - supported protected headphone output;
  - missing return/reference;
  - missing required bleed policy;
  - unmodeled fault protection remaining a candidate blocker;
  - unsupported speaker/high-power request remaining blocked.

Validation:

- `go test ./internal/designworkflow`
- `go test ./internal/rationale`
- `go test ./...`
- `prism review staged`

Commit:

- `Explain headphone output protection readiness`

## Phase 5: Intent Planner Mapping

Goal: allow supported headphone amplifier intent to request the protection block
instead of relying on examples only.

Tasks:

- Extend intent mapping for supported headphone-output phrases or structured
  output connector fields.
- Map supported Class AB headphone intent to:
  - `class_ab_output_stage`;
  - `headphone_output_protection`;
  - headphone connector breakout;
  - required load-reference connections.
- Preserve fail-closed behavior for:
  - speaker;
  - power amplifier;
  - bridge output;
  - unknown load impedance;
  - unsafe or unbounded output-power requests.
- Update natural-language/structured intent fixtures where existing tests cover
  amplifier requests.
- Add planner tests for supported and blocked headphone output-protection
  mapping.

Validation:

- `go test ./internal/intentplanner`
- `go test ./internal/intentdraft`
- `go test ./internal/designworkflow`
- `go test ./...`
- `prism review staged`

Commit:

- `Map headphone intent to output protection`

## Phase 6: Readiness, Docs, And Examples

Goal: document the completed slice and keep readiness conservative.

Tasks:

- Update `data/ai-readiness/matrix/amplifier.json` for
  `amplifier.block.output_protection`.
- Promote only to the readiness level supported by tests:
  - likely `connectivity` if schematic generation and diagnostics are covered;
  - not `candidate` unless KiCad-backed evidence and stronger protection proof
    exist.
- Update README and docs with:
  - supported headphone-only scope;
  - AC output coupling/DC-blocking terminology;
  - bleed/reference resistor policy;
  - optional series resistor policy;
  - fault-protection and speaker/power-amplifier non-goals.
- Update example READMEs and KiCad-backed amplifier metadata so stale blockers
  distinguish "output protection exists at connectivity level" from remaining
  KiCad/thermal/stability evidence.
- Add or update a design example for protected Class AB headphone output.

Validation:

- `go test ./internal/aireadiness`
- `go test ./...`
- `git diff --check`
- `prism review staged`

Commit:

- `Document headphone output protection readiness`

## Completion Criteria

- AI-facing requests can represent a Class AB headphone output with explicit
  output protection/load-safety assumptions.
- Single-supply headphone outputs require AC coupling through a DC-blocking
  capacitor.
- Supported headphone load classes are accepted; speakers and unsafe/unknown
  loads are blocked.
- Workflow summaries and rationale artifacts expose machine-readable protection
  readiness.
- The project still does not claim power-amplifier, speaker, fault-protected,
  or fabrication-ready amplifier output capability.
