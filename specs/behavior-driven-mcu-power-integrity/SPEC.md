# Behavior-Driven MCU Power-Integrity Synthesis

## Status

Implementation target.

## Objective

Replace fixed MCU decoupling companion recipes with deterministic,
calculation-backed local and bulk decoupling synthesis. Requests describe
electrical behavior and operating conditions; they do not name an MCU,
capacitor, topology, net, pin, footprint, coordinate, layer, route, provider,
or expected output.

The promoted path must:

- select a verified MCU through the existing generic controller search;
- map every reviewed MCU supply domain into a reviewed rail group;
- derive local and bulk capacitance from startup and transient-current
  behavior, source impedance, ripple/noise limits, and brownout margin;
- select concrete capacitors using capacitance tolerance, ESR, ripple current,
  voltage derating, temperature range, symbol, package, and pin-map evidence;
- connect one local capacitor to every supply domain and one bulk capacitor to
  every rail group;
- attach deterministic proximity constraints to the selected MCU;
- emit hash-bound worst-case calculations and stable fail-closed diagnostics.

## Non-Goals

This milestone does not:

- infer unreviewed RF power-distribution networks or package-plane impedance;
- model capacitor ESL or PCB spreading inductance without reviewed evidence;
- qualify a new MCU or capacitor from free-form text;
- weaken regulator-stability, dynamic-electrothermal, or fabrication gates;
- claim arbitrary dense-board routing or unrestricted power-integrity design.

## Behavior Contract

The generic `programmable_controller` request remains capability-neutral.
Power-integrity overrides use shared numeric constraints in SI units:

| Constraint | Relation | Unit | Meaning |
| --- | --- | --- | --- |
| `mcu_startup_current` | `maximum` | `A` | Worst startup current for each selected rail group. |
| `mcu_transient_current_step` | `maximum` | `A` | Worst positive load step. |
| `mcu_transient_duration` | `maximum` | `s` | Bulk energy hold-up interval. |
| `mcu_local_transient_duration` | `maximum` | `s` | Local high-frequency hold-up interval. |
| `maximum_supply_ripple` | `maximum` | `V` | Maximum bulk transient droop. |
| `maximum_supply_noise` | `maximum` | `V` | Maximum local transient droop. |
| `mcu_brownout_voltage` | `minimum` | `V` | Minimum safe MCU rail voltage. |
| `power_source_impedance` | `maximum` | `Ohm` | Worst reviewed source-path resistance. |
| `ambient_temperature_minimum` | `minimum` | `degC` | Minimum capacitor operating temperature. |
| `ambient_temperature_maximum` | `maximum` | `degC` | Maximum capacitor operating temperature. |

Absent overrides use the selected MCU's reviewed per-rail power-integrity
envelope. Present overrides must be finite, positive, use the declared
relation and unit, and remain inside the reviewed envelope where the override
would otherwise claim less stress than the catalog evidence.

## MCU Evidence

Each verified MCU must provide one `power_integrity` record per normalized rail
group. Every record requires:

- rail-group identity;
- startup current;
- transient-current step;
- bulk transient duration;
- local transient duration;
- maximum ripple and maximum local noise;
- brownout threshold;
- maximum source impedance;
- local and bulk maximum placement distance;
- explicit test/review conditions for every quantitative measurement.

Missing, duplicate, unknown, non-finite, non-positive, unit-incompatible, or
incomplete evidence blocks catalog validation and synthesis.

## Capacitor Qualification

A power-integrity capacitor candidate must be a concrete, verified catalog
part with:

- a verified package and `A`/`B` terminal mapping;
- fixed nominal capacitance;
- capacitance tolerance;
- numeric maximum ESR;
- numeric ripple-current capability;
- proven or not-applicable DC-bias review;
- proven effective-capacitance review;
- proven voltage-derating review with a typed maximum voltage-use ratio;
- a voltage rating that remains above the applied rail after applying that
  reviewed per-capacitor ratio;
- an operating-temperature range covering the requested range;
- fabrication-candidate evidence that is not blocked.

The deterministic worst-case effective capacitance is:

`C_effective = C_nominal * (1 - tolerance_percent / 100)`

No nominal-to-effective DC-bias or temperature factor may be invented.
Technologies whose catalog evidence requires an application-specific bias or
effective-capacitance review remain ineligible.

## Calculations

For each rail group:

`V_source_drop = I_startup * R_source`

`V_brownout_budget = V_rail_min - V_brownout - V_source_drop`

`V_bulk_budget = min(V_ripple_max, V_brownout_budget)`

For a capacitor candidate:

`V_esr = I_step * ESR_max`

`V_capacitive = I_step * duration / C_effective`

`V_total = V_esr + V_capacitive`

The local capacitor uses the local duration and noise budget. The bulk
capacitor uses the bulk duration and bulk budget. Both calculations must pass
ESR, capacitance, ripple-current, voltage-derating, temperature, and placement
evidence gates. A non-positive budget fails before part selection.

## Topology And Placement

- Emit one local capacitor for every `MCUSupplyDomain`, connected directly
  between that domain's reviewed power and ground functions.
- Emit one bulk capacitor for every normalized rail group, connected to a
  deterministic power/ground function in that electrically equivalent group.
- Use stable instance identities derived from rail-group and domain IDs.
- Mark local parts `decoupling_capacitor` and bulk parts `bulk_capacitor`.
- Set `near` to the selected MCU instance and use the reviewed local/bulk
  distance bound.
- Include rail-group, domain, power-function, ground-function, effective
  capacitance, ESR, and derating evidence in realization parameters and
  calculation evidence.

Existing fixed MCU decoupling companions must not be emitted when the typed
provider realization contains these connected calculated companions.

## Diagnostics

Stable rejection codes must distinguish:

- missing or invalid MCU power-integrity evidence;
- unmapped or ambiguous supply-domain/rail-group evidence;
- non-positive ripple, noise, or brownout budget;
- unavailable capacitor meeting capacitance, ESR, ripple, derating,
  temperature, package, and evidence gates.

Unsupported input must remain unsupported. Synthesis must not fall back to a
generic capacitor, fixed value, guessed rail, default coordinate, or allowlist.

## Corpus

The held-out behavior-only corpus must cover:

- ESP32 single-domain radio-current transient;
- STM32 single-domain mixed-peripheral controller;
- ATmega multi-domain controller with VCC and AVCC in one reviewed rail group;
- missing transient evidence;
- missing capacitor ESR evidence;
- exceeded source-impedance/brownout budget;
- disjoint or unmapped domain evidence;
- temperature or voltage-derating rejection.

Positive cases must avoid MCU names, capacitor identities, topology, and
coordinates.

## Acceptance Gates

All positive cases must pass:

- deterministic architecture selection and replay;
- component, value, rating, ESR, ripple, voltage-derating, and temperature
  checks;
- complete local/bulk route and connectivity evidence;
- clean installed-KiCad ERC and strict DRC;
- writer correctness;
- zero-difference round trip.

The existing clock/programming corpus, protected USB-C LED and I2C fixtures,
ESP32 fixture, and Class-A/Class-AB amplifier evidence must remain green.
