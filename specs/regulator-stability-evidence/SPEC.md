# Regulator Stability Evidence Specification

Date: 2026-07-02

## Summary

KiCadAI has a useful fixed 3.3 V linear-regulator path: AMS1117 SOT-223 and
AP2112K SOT-23-5 records, capacitor role selection, AP2112K EN/NC handling,
headroom checks, workflow evidence, schematic component identity properties,
and documentation that warns about stability and MLCC derating limits.

The next task is to close the most important remaining safety gap in that path:
regulator output-capacitor stability and high-value ceramic capacitor derating
must be represented as first-class evidence instead of only documentation
warnings. Until that exists, regulator-generated designs can be structurally
useful but should not be treated as fabrication-candidate power circuits.

This project adds a conservative evidence model for LDO stability requirements,
capacitor effective-capacitance review, and fabrication-candidate gating. It
should also prepare the path for future regulator family expansion without
forcing broad part coverage in this slice.

## Problem Statement

The current catalog can select concrete regulators and concrete capacitors, but
selection does not prove that a selected output capacitor satisfies a specific
regulator's stability requirements. The risky cases are different by part:

- AMS1117-family regulators commonly require output-capacitor ESR constraints
  and may be unstable with low-ESR ceramic output capacitors unless the specific
  manufacturer/datasheet and capacitor network prove otherwise.
- AP2112K-family LDOs are ceramic-capacitor-friendly, but high-value X5R/X7R
  MLCC effective capacitance depends on DC bias, package, dielectric, and rail
  voltage.
- Generic capacitor records can satisfy shape and nominal value requirements,
  but they cannot prove ESR windows, DC-bias derating, capacitance retention,
  or regulator transient stability.
- Fabrication-candidate acceptance currently has review blockers, but the
  underlying evidence is mostly encoded as free-form derating rules instead of
  queryable, role-specific regulator/capacitor compatibility data.

The result is safe enough for structural and connectivity-oriented generated
projects, but not yet strong enough for autonomous power-design claims.

## Goals

- Add structured regulator stability requirements to catalog records.
- Add structured capacitor capability/evidence fields for the parts already in
  the verified first-slice catalog.
- Make regulator output-capacitor compatibility checkable by role:
  `input_capacitor` and `output_capacitor` must be evaluated separately.
- Preserve conservative behavior:
  - structural/connectivity generation may continue when evidence is missing,
    but must surface explicit review requirements;
  - fabrication-candidate generation must block when required stability,
    effective-capacitance, ESR, or thermal evidence is missing.
- Distinguish ceramic-friendly regulators from ESR-sensitive regulators without
  hardcoding every behavior into block code.
- Add deterministic CLI/workflow evidence so an AI agent can explain why a
  generated regulator design is accepted, warned, or blocked.
- Keep default tests hermetic. No network, distributor API, or live KiCad
  dependency is required.

## Non-Goals

- Do not implement full analog transient simulation.
- Do not claim AMS1117 plus a ceramic output capacitor is fabrication-safe.
- Do not add switching regulators, buck converters, or adjustable regulator
  feedback synthesis in this slice.
- Do not scrape datasheets or distributor data.
- Do not require real-time availability or pricing.
- Do not solve thermal layout by simulation. Thermal evidence remains a
  conservative gate.
- Do not broaden beyond the currently modeled regulator block unless the new
  variant can carry the same evidence.

## Evidence Model

### Regulator Records

Regulator component records should be able to describe:

- `output_voltage`
- `input_voltage`
- `output_current`
- `dropout_voltage`
- `ground_current` when modeled
- `enable_voltage` and `enable_voltage_abs_max` when relevant
- package and thermal review requirements
- required input/output capacitor roles
- output-capacitor stability requirements

The stability evidence should be structured enough to support deterministic
checks. The implementation may choose the exact JSON shape, but it must be able
to encode at least:

- stability requirement kind:
  - `ceramic_stable`
  - `esr_window_required`
  - `datasheet_specific`
  - `unknown`
- required output capacitance range or nominal minimum;
- accepted dielectric families where known;
- ESR min/max window where known;
- whether a concrete capacitor part has been checked against the regulator;
- whether fabrication-candidate use is blocked without explicit proof.

### Capacitor Records

Concrete capacitor records should be able to describe:

- nominal capacitance;
- voltage rating;
- dielectric, such as `X7R`, `X5R`, `C0G`, or unknown;
- package;
- tolerance;
- polarity;
- DC-bias review status;
- effective-capacitance evidence when known;
- ESR evidence where known;
- whether the part is suitable only for structural/connectivity use until
  derating has been reviewed.

The current Murata 100 nF X7R and 10 uF X5R records should be updated only as
far as local curated evidence supports. Missing DC-bias curves must remain a
review requirement, not an inferred pass.

## Selection Behavior

Regulator selection should consider three layers:

1. Basic electrical suitability:
   - output voltage matches request;
   - input voltage is within rating;
   - output current rating satisfies request;
   - dropout/headroom satisfies request with modeled safety margin.
2. Role/component compatibility:
   - input capacitor meets minimum voltage and nominal capacitance constraints;
   - output capacitor meets nominal capacitance constraints;
   - output capacitor compatibility is checked against regulator stability
     requirement.
3. Acceptance level:
   - structural/connectivity may pass with review warnings when compatibility
     is not fully proven;
   - fabrication-candidate must block on missing stability, thermal,
     DC-bias/effective-capacitance, or ESR proof.

Selection diagnostics should distinguish:

- rating too low;
- output voltage mismatch;
- insufficient headroom;
- missing capacitor stability evidence;
- incompatible capacitor dielectric or ESR;
- missing DC-bias/effective-capacitance evidence;
- missing thermal evidence;
- unsupported adjustable/BYP/EN behavior.

## Block And Workflow Behavior

The `voltage_regulator` block should continue to generate supported 3.3 V paths
but must surface regulator/capacitor compatibility evidence in:

- component selection summary;
- generated request/rationale artifacts where applicable;
- workflow result;
- schematic component identity properties where selected evidence exists.

For AP2112K:

- `EN` tied to VIN remains supported only within modeled EN voltage limits;
- `NC` remains explicitly no-connected;
- ceramic output-capacitor use may be allowed for connectivity when the
  regulator record says ceramic-stable, but fabrication-candidate still needs
  concrete capacitor voltage/DC-bias/effective-capacitance evidence.

For AMS1117:

- structural/connectivity use may continue with review evidence;
- fabrication-candidate must block unless the selected output-capacitor network
  carries compatible ESR/stability evidence.

## AI-Agent Expectations

After this work, an AI agent should be able to inspect a generated regulator
design and answer:

- which regulator was selected and why;
- which input and output capacitors were selected;
- whether the regulator is ceramic-stable or ESR-sensitive;
- whether the output capacitor compatibility is proven, warned, or blocked;
- whether MLCC DC-bias/effective-capacitance review remains open;
- whether the design can be treated as structural, connectivity-oriented, or
  fabrication-candidate.

The agent must not claim fabrication readiness for regulator paths that only
have nominal capacitance and voltage checks.

## Acceptance Criteria

- Catalog validation rejects malformed regulator stability metadata.
- Component selection surfaces stability and derating evidence in deterministic
  result fields or issues.
- AMS1117 fabrication-candidate selection blocks without ESR-compatible output
  capacitor evidence.
- AP2112K connectivity selection remains supported but fabrication-candidate
  selection blocks when capacitor effective-capacitance/DC-bias evidence is
  missing.
- Existing structural/connectivity regulator workflows continue to pass with
  explicit warnings or review requirements.
- CLI `component select`, `component coverage`, and `design create` evidence
  make regulator stability gaps visible.
- Full `go test ./...` passes.

## Risks

- Over-modeling the schema could slow progress. Prefer a minimal structured
  model with explicit blockers over a large analog rule engine.
- Datasheet constraints differ across regulator manufacturers using similar
  symbols and packages. Do not generalize one manufacturer's requirements to a
  family unless the record explicitly says so.
- A concrete capacitor MPN does not imply suitability for every regulator. It
  must be checked in the regulator/capacitor context.
- Fabrication-candidate gating may initially feel conservative. That is
  intentional.
