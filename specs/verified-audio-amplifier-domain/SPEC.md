# Verified Audio Amplifier Domain Specification

## Purpose

Establish a fabrication-oriented audio-amplifier domain without adding
topology-specific exceptions to the generic generation pipeline. The domain
must support evidence-backed component selection, deterministic Class A and
Class AB synthesis, amplifier-aware PCB constraints, bounded electrical and
thermal validation, and complete KiCad-backed promotion evidence.

The first promoted designs are:

1. a low-power single-ended Class A preamplifier; and
2. a protected complementary-BJT Class AB headphone amplifier.

Implementation evidence is recorded in `CAPABILITY_REPORT.json`. It freezes
the catalog records, deterministic DC/AC/transient/tolerance/stability/thermal/
SOA measurements, amplifier layout-policy results, unsafe diagnostic corpus,
and hashes of both KiCad-backed fixture requests and metadata.

Unsafe variants are first-class corpus members. They must fail closed with
stable diagnostic codes and actionable paths.

## Existing Foundation

The implementation must build on the current shared infrastructure:

- the component catalog and verified symbol/footprint/pinmap resolver;
- `generic-circuit-v1` function and explicit-graph lowering;
- the built-in amplifier input, op-amp, bias, Class AB output, output
  protection, DC-blocking, and supply-decoupling blocks;
- graph-derived linear AC, nonlinear DC, transient, and bounded
  tolerance/sensitivity analysis;
- amplifier PCB constraint evidence and generated placement/routing;
- KiCad ERC, strict DRC, connectivity, route-completion, writer-correctness,
  normalized round-trip, and byte-identical replay gates.

No amplifier rule may depend on a fixture ID, reference designator, component
coordinate, or a hard-coded emitted transaction.

## Supported Milestone Envelope

### Class A Preamplifier

- single-ended, low-voltage, low-power voltage-amplifier stage;
- resistive or catalog-backed constant-current bias/load;
- BJT common-emitter and MOSFET common-source device contracts;
- emitter/source degeneration;
- AC-coupled input and output where required by the operating point;
- explicit quiescent current, output bias, gain, low-frequency cutoff,
  dissipation, and headroom assertions;
- line-level resistive load only for the first promotion fixture.

### Class AB Headphone Amplifier

- op-amp or small-signal voltage-gain/input-buffer stage;
- complementary BJT emitter-follower output pair;
- diode-string or VBE-multiplier bias with explicit quiescent-current model;
- emitter resistors, local rail decoupling, DC blocking when single-supply,
  output connector, and declared headphone load;
- bounded output swing/current, DC offset, dissipation, SOA, crossover,
  stability, and reactive-load assertions;
- the first promoted load envelope is 32 ohm or higher and below the verified
  peak-current limit of the selected devices.

Speaker loads, bridge outputs, mains-connected supplies, and output powers
outside verified device and thermal evidence remain blocked.

## Component Evidence Contract

All fabrication-oriented selections require concrete manufacturer and MPN,
verified symbol and footprint bindings, checked function-to-pad mappings,
source provenance, lifecycle status, and the following typed evidence.

### Op-Amps

- supply-voltage range and supported supply mode;
- input common-mode range;
- output swing and output current versus declared load;
- gain-bandwidth product and slew rate;
- input voltage-noise density where noise is claimed;
- unity/noise-gain stability and capacitive-load restrictions;
- junction-temperature and package thermal resistance;
- explicit status for distortion and headphone-drive claims.

### Capacitors

- dielectric/technology and polarity;
- nominal capacitance, tolerance, and voltage rating;
- effective-capacitance or voltage-derating evidence where applicable;
- ESR at a declared frequency and temperature;
- ripple-current rating at a declared frequency and temperature;
- operating temperature and endurance/lifetime evidence;
- package dimensions and verified footprint/polarity mapping.

### Power BJTs

- polarity, pin order, package, complementary group, and intended role;
- VCEO, continuous and peak collector current, and power dissipation;
- gain range at declared current, transition frequency, and junction limit;
- junction-to-case and junction-to-ambient thermal resistance;
- bounded DC SOA points containing voltage, current, pulse duration, and
  temperature basis;
- secondary-breakdown and mounting assumptions.

### Power MOSFETs

- channel polarity, G/D/S pin order, package, complementary group, and role;
- VDS, continuous and pulsed drain current, and power dissipation;
- RDS(on) with its gate voltage, threshold range, transconductance, gate
  charge, and input/reverse-transfer capacitance;
- linear-mode suitability status and bounded DC/pulsed SOA points;
- junction and thermal-resistance limits plus mounting assumptions;
- body-diode and gate-protection review status.

Missing, malformed, mutually inconsistent, or unproven evidence must block the
requested acceptance level. Free-form review notes never substitute for typed
limits.

## Generic Topology Contracts

### `class_a_voltage_stage`

The block accepts semantic parameters for device technology, supply, target
quiescent current, target output bias, target gain, source/load impedances, and
coupling policy. Deterministic synthesis selects compatible catalog parts and
calculates the smallest supported bias, degeneration, load, and coupling
network. It exports signal input/output, supply, and reference ports.

### `class_ab_output_stage`

The existing block must select a compatible complementary pair from catalog
evidence rather than from fixed IDs. Bias, emitter resistance, coupling,
protection, and load policy are semantic inputs. The block must expose the
calculated peak current, idle current, device dissipation, and validation
envelope.

Unsupported device technologies, load classes, bias schemes, or requested
operating points produce stable blocked diagnostics.

## Amplifier PCB Constraints

The PCB contract must emit and verify:

- controlled signal, analog-reference, power-return, and load-return roles;
- star-point or explicitly declared return topology;
- input/output separation and bounded parallelism;
- feedback sense from the declared output node, kept away from switched or
  high-current paths;
- local high-frequency and bulk decoupling proximity to active-device rails;
- short output-current loops, width/current rules, and via-current rules;
- Kelvin routing for emitter/source resistors where current sense matters;
- complementary-device symmetry and bias-device thermal coupling;
- device/heatsink thermal keepouts and temperature-sensitive-part separation;
- polarity/orientation evidence for electrolytic capacitors;
- connector-current and fault-path continuity.

Each required constraint must appear in machine-readable placement/routing
evidence and must be checked after every repair attempt.

## Deterministic Validation

The provider may request analyses and assertions but may not provide executable
models, equations, device parameters, corners, solver settings, tolerances, or
acceptance overrides. Those come from the trusted catalog and registered
policy.

Promotion requires:

1. **DC operating point:** node voltages, device currents, idle current,
   output offset, headroom, and per-device dissipation.
2. **AC response:** gain, cutoff frequencies, bandwidth, and declared-load
   response.
3. **Transient:** clipping, slew behavior, startup/settling, crossover region,
   and bounded resistive/reactive load response.
4. **Tolerance/sensitivity:** deterministic registered corners for passives,
   bias-device spread, gain spread, capacitor effective value, and relevant
   temperature coefficients.
5. **Stability:** loop-gain or trusted equivalent phase/gain-margin evidence;
   unsupported loop breaking or device models block the claim.
6. **Thermal:** ambient, dissipation, thermal path, heatsink/interface,
   junction-temperature margin, and coupled-bias temperature policy.
7. **SOA:** every output-device operating and fault point must lie inside a
   catalog-backed interpolated boundary with declared derating policy.

Every report includes registry/catalog hashes, topology hash, analysis policy
version, exact corners, solver evidence, measurements, assertions, and stable
diagnostics.

## Frozen Corpus

### Positive Milestone Cases

- `class_a_bjt_line_preamplifier`
- `protected_bjt_class_ab_headphone_amplifier`

Both must produce byte-identical repeated artifacts and pass:

- catalog and library resolution;
- schematic electrical validation and clean KiCad ERC;
- PCB placement, all amplifier PCB constraints, route completion,
  connectivity, and clean strict KiCad DRC;
- DC, AC, transient, tolerance, stability, thermal, and SOA validation;
- writer correctness and zero normalized schematic/PCB round-trip diffs;
- fabrication package identity/consistency gates;
- recorded-provider replay where provider intent is used.

### Required Unsafe Cases

- op-amp common-mode or output-drive violation;
- capacitor voltage, ESR, ripple-current, or polarity violation;
- BJT secondary-breakdown/SOA violation;
- MOSFET linear-mode/SOA or gate-drive violation;
- Class A excessive device dissipation;
- Class AB thermal-runaway bias condition;
- inadequate phase margin or unsupported stability model;
- high-current route, return topology, thermal-coupling, or feedback-layout
  violation.

Each case must fail at the earliest authoritative stage with a stable category,
code, path, message, and suggested correction. No unsafe case may pass through
an allowlist or fixture-specific policy.

## Promotion And Regression Requirements

- Existing USB-C LED, I2C sensor, function-level, adversarial, and tolerance
  promotion evidence must remain unchanged or improve.
- Optional KiCad-backed amplifier fixtures run after each routing, writer, or
  layout correction.
- Full repository tests and coverage gates pass.
- Prism reports no unresolved high- or medium-severity findings.
- Generated reports and hash sidecars are committed and reproducible.
- The final branch is committed, pushed, and GitHub Actions is green.

## Definition Of Complete

The domain is complete only when every positive and unsafe corpus case has
checked-in, reproducible evidence; both milestone boards pass all declared
KiCad, electrical, thermal, SOA, fabrication, writer, and replay gates; the
generic infrastructure contains no milestone identity logic; regressions are
green; and the AI readiness matrix marks each required amplifier record
`verified` with evidence.
