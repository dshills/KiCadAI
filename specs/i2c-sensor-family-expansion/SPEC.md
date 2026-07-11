# Verified I2C Sensor Family Expansion Specification

Date: 2026-07-11

Implementation status: completed. The acceptance evidence is recorded in the
phase plan and enforced by focused catalog, pinmap, block, intent, workflow,
post-write electrical, readability, and writer-correctness tests.

## Summary

KiCadAI currently has one verified sensor record, Bosch BME280, while the
`i2c_sensor` block emits only the project-local `Sensor:Generic_I2C` template.
This milestone broadens both the component catalog and executable circuit
topology by adding two concrete, KiCad-resolvable I2C sensor variants and by
allowing the existing block to emit their real pin topology.

The finite expansion wave is:

- Bosch BMP280 pressure/temperature sensor;
- Sensirion SHT31-DIS humidity/temperature sensor;
- existing Bosch BME280 record promoted into the same concrete block path;
- the existing generic I2C template retained for backward compatibility.

## Goals

- Add verified catalog records for BMP280 and SHT31-DIS with exact KiCad
  symbol, footprint, pinmap, supply, address, and companion evidence.
- Add built-in symbol-to-footprint pinmaps for both parts.
- Let `i2c_sensor` accept a concrete `sensor_component_id` and emit the
  selected part's real symbol, footprint, power pins, bus pins, address/config
  pins, optional interrupt pin, and required no-connect markers.
- Reject mismatched component/symbol/footprint combinations and unsupported
  sensor IDs.
- Prove deterministic component selection for all three concrete sensors and
  negative selection for incompatible supply/rating/function requests.
- Map structured sensor intent to the concrete component choice without
  guessing.
- Add generated schematic evidence for at least BMP280 and SHT31-DIS.

## Non-Goals

- Do not add SPI topology, even when a selected sensor also supports SPI.
- Do not add arbitrary sensor pin-role inference.
- Do not add a new PCB router or change general placement policy.
- Do not claim environmental measurement accuracy, calibration quality, or
  production suitability beyond recorded component evidence.
- Do not require external KiCad libraries or network access in default tests.

## AI-Facing Contract

The `i2c_sensor` block gains an optional `sensor_component_id` parameter. Known
values in this milestone are:

- `sensor.bosch.bme280.lga8`
- `sensor.bosch.bmp280.lga8`
- `sensor.sensirion.sht31_dis.dfn8`

When omitted, the block retains its generic structural template. When present,
the block must derive symbol, footprint, displayed value, pin roles, and
auxiliary-pin handling from a checked-in profile keyed by the component ID.
Caller-provided symbol or footprint overrides must either match that profile or
fail closed.

## Topology Semantics

Every concrete profile defines:

- one or more supply pins and ground pins;
- SDA and SCL pins used in I2C mode;
- an optional interrupt/alert pin;
- mode/address/reset/reserved pins and their safe generated treatment;
- schematic pin anchors corresponding to the checked-in KiCad symbol;
- the exact catalog component and package variant.

The block connects every required supply and ground pin, connects SDA/SCL,
connects or marks the optional interrupt pin, and emits explicit no-connect
markers only where the profile declares that treatment valid. Address or mode
pins that require a defined level must be represented by deterministic local
connections, not silently left floating.

## Catalog Evidence

Each added record must include:

- concrete manufacturer and MPN identity;
- active lifecycle state as a curated snapshot, without claiming live stock;
- verified KiCad symbol and footprint IDs;
- required function pins and pad functions;
- built-in pinmap identifier;
- supported supply-voltage range;
- supported I2C address values or address-selection policy;
- required decoupling and pull-up companions;
- source labels identifying the KiCad libraries and manufacturer datasheet.

## Validation

Acceptance requires:

- catalog load and validation with no new errors;
- pinmap validation for both added assignments;
- deterministic positive selection for BME280, BMP280, and SHT31-DIS;
- negative tests for missing functions, unsupported supply voltage, unknown
  component IDs, and symbol/footprint mismatch;
- block instantiation tests proving exact real-pin connectivity;
- structured intent and generated workflow tests proving the selected component
  identity reaches written schematic symbols;
- schematic parse, electrical, readability, and writer-correctness evidence;
- full hermetic `go test -p 1 ./...` success.

## Compatibility And Safety

Existing requests without `sensor_component_id` must retain their current
output and tests. Concrete profiles are allowlisted; unknown IDs fail closed.
The milestone must not weaken component confidence, pinmap, writer, ERC/DRC,
or fabrication gates.

## Completion Criteria

The component catalog contains two additional verified I2C sensors, all three
concrete sensor records are selectable, and the `i2c_sensor` block can generate
electrically checked KiCad schematics using each real part topology. Intent and
workflow evidence must expose the selected component ID and package without
hidden fallback to the generic template.
