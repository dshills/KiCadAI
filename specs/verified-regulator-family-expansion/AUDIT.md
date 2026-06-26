# Verified Regulator Family Expansion Audit

Date: 2026-06-26

## Current Coverage

The checked-in catalog currently has one concrete regulator record:

- `regulator.linear.ams1117_3v3.sot223`
  - symbol: `Regulator_Linear:AMS1117-3.3`
  - footprint: `Package_TO_SOT_SMD:SOT-223-3_TabPin2`
  - pinmap: built in
  - ratings: 12 V input maximum, 800 mA output-current maximum
  - limits: thermal dissipation, exact manufacturer stability requirements,
    and capacitor ESR/DC-bias behavior are not proven by the record.

Generic capacitor support exists for:

- `capacitor.ceramic.0603`
- `capacitor.ceramic.0805`

Those records can prove value/rating compatibility and symmetric pin mapping,
but they do not prove MLCC DC-bias derating or LDO output-capacitor stability.

## Local KiCad Library Evidence

The local KiCad symbol checkout inspected for this audit contains directory
split symbol files for `Regulator_Linear`. This checkout is evidence only; tests
and implementation must not depend on that machine-specific path. Any symbol,
footprint, or pinmap facts required by default tests must be committed as
catalog records, built-in pinmaps, or hermetic test fixtures.

The most practical next candidate is:

- `Regulator_Linear:AP2112K-3.3`
  - extends KiCad's `AP2204K-1.5` symbol body;
  - footprint property: `Package_TO_SOT_SMD:SOT-23-5`;
  - KiCad description: 600 mA low-dropout linear regulator, 3.8 V to 6 V input,
    fixed 3.3 V output, SOT-23-5;
  - pin functions inherited from the base symbol:
    - pin 1: `VIN`
    - pin 2: `GND`
    - pin 3: `EN`
    - pin 4: `NC`
    - pin 5: `VOUT`
  - footprint pads `1` through `5` exist in
    `Package_TO_SOT_SMD:SOT-23-5`.

This candidate is useful because it broadens the 3.3 V regulator family from a
high-current SOT-223 part to a smaller LDO with an enable pin and lower dropout.
It also forces the block and selector to handle a five-pin regulator role map
instead of only AMS1117-compatible three-pin parts.

Secondary candidates found locally include:

- `Regulator_Linear:MCP1700x-330xxTT`
  - footprint: `Package_TO_SOT_SMD:SOT-23`
  - pins: GND=1, VO=2, VI=3
  - useful as a low-current three-pin LDO later, but the first slice should
    prioritize AP2112K because it exercises the enable/no-connect path.
- `Regulator_Linear:AMS1117-5.0` and related LM1117/AMS1117 variants
  - useful for future 5 V fixed-output support;
  - should not be added until the planner can distinguish source/output
    voltage relationships cleanly for 5 V rails.

## Selected Phase 2 Slice

Add one verified component record:

- `regulator.linear.ap2112k_3v3.sot23_5`

Required catalog evidence:

- manufacturer: Diodes Incorporated
- MPN: `AP2112K-3.3`
- output voltage: 3.3 V
- input voltage: 3.8 V minimum, 6 V maximum
- output current: 600 mA maximum
- dropout voltage: record 400 mV maximum at 600 mA in `values[]` with
  `kind: "dropout_voltage"` for initial headroom modeling, plus a
  component-specific 100 mV safety margin so the selector accepts the
  datasheet-supported 3.8 V minimum input for a 3.3 V output at full load
- symbol functions: `VIN`, `GND`, `EN`, `NC`, `VOUT`
- EN voltage: model EN absolute maximum separately from recommended operating
  input voltage and require `input_voltage + 0.5 V <= enable_voltage.abs_max`
  before tying EN directly to VIN when no request-specific transient maximum is
  known
- output capacitor ESR: record stability as a review requirement until a
  concrete capacitor ESR range and capacitor part are modeled
- package: `Package_TO_SOT_SMD:SOT-23-5`
- pad functions matching symbol pin numbers
- companions: input capacitor and output capacitor
- review rules:
  - thermal dissipation check
  - stability with selected input/output capacitor values and capacitor type
  - MLCC DC-bias derating when ceramic capacitors are selected

Required built-in pinmap:

- symbol: `Regulator_Linear:AP2112K-3.3`
- footprint: `Package_TO_SOT_SMD:SOT-23-5`
- pins:
  - 1/VIN -> pad 1
  - 2/GND -> pad 2
  - 3/EN -> pad 3
  - 4/NC -> pad 4
  - 5/VOUT -> pad 5

## Explicit Non-Promotions

This slice must not promote generated regulator designs to fabrication-ready by
default. Even after AP2112K catalog support exists, fabrication-candidate flows
must still report or block unresolved evidence for:

- regulator thermal dissipation at the requested current and voltage drop;
- output-capacitor stability requirements;
- distinction between `NC` and manufacturer `DNC`/internal connection pins;
- ceramic capacitor effective capacitance under DC bias;
- exact capacitor part-number selection.

## Phase 1 Decision

Proceed with AP2112K-3.3 SOT-23-5 as the first family-expansion record. Defer
MCP1700 and 5 V 1117 variants until the AP2112K path proves that selector,
block, planner, workflow, and rationale evidence can handle regulator variants
without unsafe fallback.
