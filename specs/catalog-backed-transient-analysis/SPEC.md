# Catalog-Backed Transient Analysis

## Objective

Add deterministic time-domain analysis for a reviewed circuit subset containing
resistors, independent sources, capacitors, Shockley diodes, and NPN/PNP
Ebers-Moll BJTs. The circuit topology and device parameters must remain derived
from resolved connectivity and trusted catalog claims.

## Trust boundary

Provider-authored intent may select `transient_circuit_v1`, request one bounded
`transient` observation grid, provide finite independent-source operating
conditions, and assert node voltage or trusted 10–90% edge metrics. It cannot
provide equations, matrices, stamps, device models, initial node state,
integration methods, tolerances, iteration counts, damping, continuation,
topology classifications, executable content, or model/include paths.

`duration_s` and `time_step_s` define the requested deterministic observation
grid. They are analysis conditions, not free-form solver controls. V1 uses the
same fixed step for integration and observation and accepts only an exact
integer number of steps.

## Reviewed primitive subset

- existing trusted resistor and independent voltage/current sources;
- `mna_capacitor_transient_be_v1`, requiring a positive catalog-validated
  capacitance and catalog parameter `max_voltage_v`;
- existing reviewed Shockley diode and NPN/PNP Ebers-Moll primitives;
- no op-amps, inductors, arbitrary behavioral sources, or provider models.

The transient workflow requires at least one transient capacitor and at least
one reviewed nonlinear device. Linear and nonlinear-DC workflows do not select
the transient capacitor claim, preventing ambiguous or accidental expansion of
their contracts.

## Deterministic analysis

- Integration is fixed backward Euler.
- Initial state is the bounded nonlinear DC operating point at `t=0`, with
  capacitors open and sources evaluated at their initial pulse value.
- Every subsequent point uses the previous accepted state as the Newton seed.
- Newton iteration count, voltage damping, residual/update tolerances,
  exponential limiting, and gmin are trusted constants.
- The observation grid is finite, positive, exactly divisible, and limited to
  2048 integration steps. Each analysis is limited to one grid and total
  nonlinear work is bounded by steps times the trusted iteration limit.
- Supported source waveforms are constant DC or a single periodic ideal pulse
  described by initial/pulsed value, delay, width, and period. Pulse boundaries
  must lie exactly on the observation grid. Pulse initial and pulsed values are
  absolute source levels, so `dc_value` must be zero for a pulse; this prevents
  two ambiguous encodings of the same baseline.
- Every accepted point is checked against capacitor voltage, diode current and
  reverse-voltage, and BJT collector-current and collector-emitter-voltage
  catalog limits.
- A failed solve identifies the time/index, bounded iteration count, dominant
  update or residual, and a corrective suggestion.

## Assertions and evidence

Transient assertions support:

- `voltage_v` at an exact observation time;
- `rise_time_s`, the first trusted 10–90% rising transition;
- `fall_time_s`, the first trusted 90–10% falling transition.

Crossing times use deterministic linear interpolation between adjacent stored
points. Edge thresholds are derived from the global minimum and maximum over
the complete bounded analysis duration; providers cannot redefine them or
select a transition window. Reports record time per point, fixed method,
initial-condition method, bounded work, convergence evidence, resolved devices,
catalog/registry hashes, and assertion results. Re-evaluating the same resolved
plan must produce byte-identical JSON.

## Promotion proof

A held-out catalog-resolved transistor switching circuit must contain no block
or topology classification and must pass voltage-at-time, rise-time, and
fall-time assertions. Promotion requires clean KiCad ERC, strict DRC,
connectivity, route completion, writer correctness, zero round-trip diffs, and
byte-identical recorded replay. Existing linear MNA, nonlinear DC, USB-C I2C,
and protected LED promotion evidence must remain passing.

## Non-goals

This is not adaptive integration, SPICE compatibility, arbitrary transient
simulation, parasitic/tolerance/thermal/SOA analysis, or fabrication signoff.
