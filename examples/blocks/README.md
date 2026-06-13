# Circuit Block Examples

This directory contains KiCad projects generated through the public block CLI.

## Contents

- `requests/`: source JSON requests used to generate each example.
- `led_indicator/`: LED plus current-limit resistor.
- `connector_breakout/`: four-pin VCC/SDA/SCL/GND breakout.
- `voltage_regulator/`: 5 V to 3.3 V fixed regulator with capacitors and power LED.
- `i2c_sensor/`: generic I2C sensor with pull-ups, interrupt, and decoupling.
- `opamp_gain_stage/`: non-inverting gain stage with output resistor.
- `usb_c_power/`: USB-C sink power input with fuse, TVS, bulk capacitor, and LED.
- `mcu_minimal/`: ATmega328P-A minimal system with power, reset, AREF, decoupling, GPIO, and ISP header.
- `composed_sensor_breakout/`: USB-C power, regulator, I2C sensor, and connector breakout composition.

## Regeneration

Run from the repository root:

```sh
GOCACHE=/tmp/kicadai-gocache go run ./cmd/kicadai --json --request examples/blocks/requests/led_indicator.json --output examples/blocks/led_indicator --name led_indicator --overwrite block instantiate led_indicator
GOCACHE=/tmp/kicadai-gocache go run ./cmd/kicadai --json --request examples/blocks/requests/composed_sensor_breakout.json --output examples/blocks/composed_sensor_breakout --name composed_sensor_breakout --overwrite block compose
```

Use the same pattern for the other request files.

## Validation

These examples are structural schematic/project outputs, not fabrication-ready boards. `inspect project` should parse them. Generated PCB files are intentionally omitted in this phase because footprint pad geometry and routing are still known writer gaps.
