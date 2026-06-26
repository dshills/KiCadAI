# Verified Regulator Family Expansion Specification

Date: 2026-06-26

## Summary

KiCadAI now has a working verified regulator path for a common 5 V to 3.3 V
linear-regulator design: a fixed 3.3 V AMS1117-style SOT-223 regulator with
generic 0805 capacitor roles, planner-generated rating overrides, block
integration, workflow evidence, and documentation. That slice is useful, but it
is too narrow to be the long-term power foundation for autonomous board
generation.

The next item is to broaden the regulator family in a controlled way. The goal
is not to add many power parts. The goal is to make KiCadAI model regulator
selection safely enough that an AI can choose among known alternatives, reject
unsafe current/voltage/thermal cases, and explain why a generated design is only
structural, connectivity-oriented, or a fabrication candidate.

This project expands verified regulator coverage, capacitor policy, and planner
evidence while keeping analog stability, thermal, and derating limits explicit.

## Problem Statement

The current regulator slice has several intentional limits:

- only one concrete fixed-output 3.3 V regulator record exists;
- regulator selection can match electrical ratings but does not yet rank
  alternatives by dropout, current, package, or thermal evidence;
- generic ceramic capacitor records do not prove LDO stability, ESR window, DC
  bias derating, or exact manufacturer behavior;
- planner capacitor voltage policy currently emits minimum voltage-class
  evidence, not professional ceramic derating proof;
- `voltage_regulator` block behavior is effectively tuned to the one known 3.3
  V path;
- generated workflow evidence shows selected components, but not enough
  regulator-specific warnings for thermal or capacitor-stability review.

These gaps are acceptable for a seed path, but they are not acceptable as the
power-selection foundation for broader autonomous AI generation.

## Goals

- Add a small set of verified regulator alternatives for common generated-board
  needs.
- Represent regulator-specific evidence needed for safe selection:
  - output voltage;
  - input voltage;
  - output current;
  - dropout voltage;
  - package;
  - thermal review requirement;
  - capacitor stability requirements where known.
- Improve capacitor policy so generic capacitor selection can distinguish
  minimum voltage-class evidence from fabrication-candidate derating evidence.
- Make component selection reject unsafe or under-evidenced regulator requests
  with actionable diagnostics.
- Let the `voltage_regulator` block and structured intent select a supported
  regulator variant deterministically.
- Persist regulator decision evidence in generated request, workflow result,
  and rationale artifacts.
- Keep fabrication-candidate behavior conservative: thermal and stability gaps
  must block or remain explicit, not disappear into generic catalog selection.

## Non-Goals

- Do not implement a switching-regulator controller or buck converter topology
  in this slice.
- Do not scrape distributor, stock, pricing, or lifecycle data.
- Do not claim that a generic ceramic capacitor proves LDO stability.
- Do not model full thermal simulation, copper pour heatsinking, or enclosure
  temperature rise.
- Do not add arbitrary output-voltage synthesis. Only verified fixed-output
  variants or explicitly modeled adjustable variants are allowed.
- Do not add a broad power-part catalog without tests and evidence.
- Do not require network access or a live KiCad instance for default tests.

## Candidate Expansion Slice

The first implementation should stay small. A good target is:

- keep the existing AMS1117 3.3 V SOT-223 path;
- add one lower-current, lower-dropout 3.3 V SOT-23 or SOT-23-5 LDO with a
  KiCad-resolvable symbol/footprint/pinmap;
- defer 5 V fixed-output LDO support until planner and workflow tests can prove
  source/output voltage handling for that rail family;
- add concrete capacitor policy evidence rather than only expanding generic
  capacitor records:
  - minimum selected voltage class;
  - recommended ceramic voltage class for fabrication-candidate output;
  - explicit note that effective capacitance under DC bias is not proven unless
    the component record carries part-specific derating metadata.

Candidate parts should be chosen from symbols and footprints already available
through KiCad libraries in the configured external library roots or through
existing built-in pinmap fixtures. If that evidence is missing, the
implementation should add the missing pinmap fixture before promoting a record
to `verified`.

## Catalog Model Requirements

Regulator records must include:

- `family: "regulator"`;
- concrete `manufacturer` and `mpn` for verified records;
- output-voltage value metadata;
- input-voltage and output-current ratings;
- dropout-voltage value metadata using the existing `values[]` collection with
  `kind: "dropout_voltage"`;
- temperature range where known;
- package variant with footprint ID and pad functions;
- symbol function pins for required regulator roles such as `VIN`, `VOUT`, and
  `GND`, plus optional roles such as `EN`, `NC`, `BYP`, and `ADJ` when the
  selected part exposes them;
- verification metadata with pinmap evidence;
- companion requirements for input and output capacitors using the existing
  `companions[]` structure: `id`, `family`, `role`, `required`, and
  `description`;
- derating or review rules for thermal and stability risks;
- placement hints for input/output capacitors near regulator pins;
- routing hints for power nets where applicable.

Capacitor records or policy metadata must support:

- nominal capacitance matching;
- voltage rating checks;
- tolerance metadata where available;
- package variant evidence;
- nonpolar or polarized behavior;
- explicit limitations for MLCC DC-bias and ESR when not part-specific.

## Selection Behavior

Component selection must support regulator requests that constrain:

- `output_voltage`;
- `input_voltage`;
- `output_current`;
- `enable_voltage`;
- `enable_voltage_abs_max`;
- `output_capacitor_esr`;
- package preference;
- minimum confidence;
- fabrication-candidate acceptance where thermal/stability evidence is
  required.

Selection should:

- reject regulators with insufficient input-voltage or output-current ratings;
- reject fixed-output regulators whose output voltage does not match the
  requested rail within a deterministic nominal tolerance. The default
  tolerance is +/-3 percent unless a component record or request explicitly
  declares a narrower tolerance;
- reject requests where the input voltage does not satisfy output voltage plus
  dropout/headroom requirements, including a modeled safety margin when
  available. When no component-specific headroom margin is modeled, use a
  default 200 mV margin above dropout and reject when
  `input_voltage < output_voltage + dropout_voltage + margin`;
- prefer explicit package preferences when multiple candidates satisfy
  requirements;
- report thermal/stability review rules in the selected component evidence;
- surface rejected candidates with concrete reason codes;
- preserve deterministic ordering.

Selection must not:

- choose a higher-current regulator solely because it has a larger absolute
  rating when the request's explicit constraints are already satisfied by a
  lower-current part;
- treat a generic capacitor as proof of LDO stability;
- silently use a 3.3 V regulator for a 5 V rail or vice versa.

When more than one regulator satisfies the explicit request, selection policy
should rank deterministic evidence before convenience:

1. required electrical constraints, including output voltage, input voltage,
   dropout/headroom, and current;
2. explicit package preference;
3. quantitative thermal metadata if the catalog has comparable values for every
   remaining candidate;
4. stability/capacitor evidence when it is encoded in comparable catalog
   fields;
5. deterministic tie-break by component ID.

If thermal or stability evidence is present only as review text, it must be
reported but must not be converted into an implicit ranking score.

## Block Integration

The `voltage_regulator` block should become variant-aware without becoming
open-ended. The block should accept or derive:

- `output_voltage`;
- `input_voltage`;
- `output_current`;
- optional `regulator_package`;
- optional `enable_control`; when omitted for a regulator with `EN`, generated
  connectivity must tie `EN` to the regulator input rail unless the catalog
  record states a different safe default and the input voltage is within the
  modeled `EN` pin rating after derating;
- input and output capacitor values;
- optional `capacitor_voltage_policy`;
- optional `include_power_led`.

For supported variants, the block should emit component role selection requests
for:

- regulator;
- input capacitor;
- output capacitor;
- optional power LED and resistor when enabled.

Generated connectivity must handle non-power regulator pins explicitly:

- `EN` defaults to VIN unless a verified external enable control is requested;
- tying `EN` to VIN is allowed only when the selected record proves that the
  normal operation stays within the modeled recommended `EN` voltage range and
  the expected transient maximum stays below the modeled `EN` absolute maximum.
  If the request does not model transients, use `input_voltage + 0.5 V` as the
  initial expected maximum for generated low-voltage LDO designs;
- `NC` pins must receive explicit KiCad schematic no-connect flags so ERC does
  not treat them as floating design mistakes;
- manufacturer do-not-connect pins must be represented separately from generic
  `NC` pins and must remain floating unless the record explicitly permits a
  connection;
- `ADJ` and `BYP` pins are unsupported until their required companion networks
  are modeled.

For unsupported variants, the block should fail closed at connectivity-oriented
acceptance and report a clear unsupported regulator variant issue.

## Intent Planner Integration

Structured intent power rails should drive regulator selection with explicit
evidence:

- `power.inputs[]` establishes source voltage and source net;
- `power.rails[].voltage` establishes requested output voltage;
- `power.rails[].current_ma` establishes output current requirement;
- `power.rails[].supplied_targets` or aliases explain rail consumers;
- manufacturing or acceptance constraints can request stricter evidence.

The planner should:

- derive input voltage, output voltage, and output current requirements;
- derive capacitor voltage-class requirements;
- record whether capacitor voltage class is minimum-only or
  fabrication-candidate recommended;
- preserve regulator headroom checks;
- add thermal-review and stability-review evidence to synthesis calculations or
  constraints;
- produce a generated `component_policy` that can be audited.

## Workflow And Artifact Requirements

Generated projects should persist regulator evidence in:

- `.kicadai/generated-request.json`;
- `.kicadai/workflow-result.json`;
- `.kicadai/intent-plan.json`;
- `.kicadai/design-rationale.json` where rationale is generated.

Evidence should include:

- selected regulator component ID and package;
- selected capacitor component IDs and packages;
- selected or required ratings;
- rejected candidates where meaningful;
- thermal review requirement;
- capacitor stability and derating limitations;
- whether the request met structural, connectivity, or fabrication-candidate
  evidence expectations.

## Validation Requirements

Default tests must remain hermetic and KiCad-independent unless explicitly
opted in. The expansion must add:

- catalog validation tests;
- selection positive and negative tests;
- block component-role tests;
- planner mapping tests;
- workflow persistence tests for generated evidence;
- documentation updates;
- full `go test ./...` compatibility.

Optional KiCad-backed tests can be added only as opt-in fixtures if the selected
symbols and footprints are present in local KiCad libraries.

## Safety And Fabrication Readiness

The implementation must make these limits visible:

- linear-regulator thermal dissipation is not automatically proven;
- selection or planning should calculate first-order regulator dissipation with
  `P = ((Vin - Vout) * Iout) + (Vin * Ignd)` when ground-current metadata is
  available, or `P = (Vin - Vout) * Iout` plus a missing-ground-current warning
  when it is not, and compare it against a conservative package
  threshold when available. For AP2112K SOT-23-5, encode that threshold as
  `power_dissipation_max` and use 250 mW as an initial
  conservative free-air threshold until package thermal metadata is modeled;
- LDO stability with ceramic output capacitors is not automatically proven;
- ceramic capacitor effective capacitance under DC bias is not automatically
  proven;
- generic capacitor records are acceptable for structural/connectivity evidence
  only unless part-specific derating/stability metadata is present;
- fabrication-candidate acceptance must either require stronger evidence or
  emit blocking issues for unresolved thermal/stability requirements.

## Success Criteria

- At least one additional verified regulator candidate is present and tested.
- Regulator selection can choose a supported variant based on output voltage,
  input voltage, current, and package constraints.
- Unsupported or unsafe regulator requests fail with actionable diagnostics.
- `voltage_regulator` block component role selection is variant-aware.
- Structured intent generates regulator component policy with current,
  voltage, thermal, and capacitor evidence.
- Generated workflow artifacts persist the regulator decision evidence.
- Documentation and agent guidance explain the safe scope and remaining
  fabrication gaps.
- `go test ./...` passes.
