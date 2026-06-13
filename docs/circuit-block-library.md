# Circuit Block Library

The circuit block library provides reusable, parameterized schematic fragments
for common circuits. Blocks are exposed through the `kicadai block` CLI and are
intended to be a safer AI-facing design primitive than raw file writes.

The current implementation generates structural KiCad project and schematic
outputs. It does not yet claim fabrication-ready PCB output.

## Available Blocks

Use `block list` to see the registry:

```sh
go run ./cmd/kicadai --json block list
```

Current built-in blocks:

| Block ID | Category | Current level | Purpose |
|---|---|---|---|
| `led_indicator` | indicator | `structural` | Series resistor plus LED indicator. |
| `connector_breakout` | interconnect | `experimental` | Generic connector with exported named pins. |
| `voltage_regulator` | power | `structural` | Fixed-output linear regulator with input/output capacitors. |
| `i2c_sensor` | sensor | `structural` | I2C peripheral with pull-ups, interrupt, and decoupling. |
| `opamp_gain_stage` | analog | `structural` | Non-inverting op-amp gain stage with feedback network. |
| `usb_c_power` | power | `structural` | USB-C sink power input with CC pull-downs and optional protection. |
| `mcu_minimal` | digital | `structural` | ATmega328P-A minimal system with reset, decoupling, GPIO, and ISP header. |

Inspect one block:

```sh
go run ./cmd/kicadai --json block show led_indicator
```

`block show` returns parameters, ports, required libraries, and verification
notes. Use this output as the machine-readable contract for agent workflows.
The shape is stable JSON:

```json
{
  "id": "led_indicator",
  "parameters": [
    {"name": "supply_voltage", "type": "voltage", "default": "3.3V"},
    {"name": "led_current", "type": "current", "default": "5mA"},
    {"name": "color", "type": "enum", "default": "green"}
  ],
  "ports": [
    {"name": "IN", "direction": "input"},
    {"name": "VCC", "direction": "power", "voltage": "supply_voltage"},
    {"name": "GND", "direction": "power"}
  ],
  "verification": {"level": "structural"}
}
```

## Verification Levels

Each block declares a verification level:

| Level | Meaning |
|---|---|
| `experimental` | The block can parse/write, but its electrical behavior is not verified. |
| `structural` | Symbol/footprint references, nets, and basic structure are modeled and validated. |
| `roundtrip_verified` | Generated files pass KiCad round-trip validation. |
| `erc_drc_verified` | Available KiCad ERC/DRC or equivalent validation passes. |
| `reference_verified` | The block has been checked against a known-good reference design. |

Only `roundtrip_verified` or stronger blocks should be treated as candidates
for fully autonomous generation without an explicit warning. The current block
examples are useful for schematic generation experiments, not manufacturing.

## Instantiate One Block

Request files use this shape:

```json
{
  "block_id": "led_indicator",
  "instance_id": "status",
  "params": {
    "color": "green",
    "led_current": "5mA",
    "supply_voltage": "3.3V"
  }
}
```

Generate a KiCad project:

```sh
go run ./cmd/kicadai \
  --json \
  --request examples/blocks/requests/led_indicator.json \
  --output ./out/led_indicator \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
```

For `block instantiate`, the current CLI requires both the positional block ID
and the request `block_id`; they must match. If they do not match, the command
fails instead of choosing one. Pass `--name` for a stable generated project
name. If `--name` is omitted, the request `instance_id` is used as the fallback.

`--overwrite` is required when the output directory already exists. Without an
`--output`, instantiate returns the block operation plan and issues without
writing a project.

Connector breakout request:

```json
{
  "block_id": "connector_breakout",
  "instance_id": "io",
  "params": {
    "pin_count": 4,
    "pin_names": ["VCC", "SDA", "SCL", "GND"]
  }
}
```

Generate it:

```sh
go run ./cmd/kicadai \
  --json \
  --request examples/blocks/requests/connector_breakout.json \
  --output ./out/connector_breakout \
  --name connector_breakout \
  --overwrite \
  block instantiate connector_breakout
```

## Compose Multiple Blocks

Composition connects named ports from multiple block instances.

```json
{
  "project_name": "composed_sensor_breakout",
  "instances": [
    {
      "id": "usb",
      "block_id": "usb_c_power",
      "params": {
        "current_limit": "500mA"
      }
    },
    {
      "id": "reg3v3",
      "block_id": "voltage_regulator",
      "params": {
        "input_voltage": "5V",
        "input_voltage_min": "4.5V",
        "input_voltage_max": "5.5V",
        "output_voltage": "3.3V",
        "output_current": "250mA"
      }
    },
    {
      "id": "sensor",
      "block_id": "i2c_sensor",
      "params": {
        "i2c_address": "0x48",
        "include_interrupt": true
      }
    },
    {
      "id": "io",
      "block_id": "connector_breakout",
      "params": {
        "pin_count": 4,
        "pin_names": ["VCC", "SDA", "SCL", "GND"]
      }
    }
  ],
  "connections": [
    {"from": {"instance_id": "usb", "port": "VBUS_OUT"}, "to": {"instance_id": "reg3v3", "port": "VIN"}},
    {"from": {"instance_id": "usb", "port": "GND"}, "to": {"instance_id": "reg3v3", "port": "GND"}},
    {"from": {"instance_id": "reg3v3", "port": "VOUT"}, "to": {"instance_id": "sensor", "port": "VCC"}},
    {"from": {"instance_id": "reg3v3", "port": "GND"}, "to": {"instance_id": "sensor", "port": "GND"}},
    {"from": {"instance_id": "sensor", "port": "VCC"}, "to": {"instance_id": "io", "port": "VCC"}},
    {"from": {"instance_id": "sensor", "port": "GND"}, "to": {"instance_id": "io", "port": "GND"}},
    {"from": {"instance_id": "sensor", "port": "SDA"}, "to": {"instance_id": "io", "port": "SDA"}},
    {"from": {"instance_id": "sensor", "port": "SCL"}, "to": {"instance_id": "io", "port": "SCL"}}
  ]
}
```

Generate the composed project:

```sh
go run ./cmd/kicadai \
  --json \
  --request examples/blocks/requests/composed_sensor_breakout.json \
  --output ./out/composed_sensor_breakout \
  --name composed_sensor_breakout \
  --overwrite \
  block compose
```

For `block compose`, pass `--name` for a stable generated project name. If
`--name` is omitted, the request `project_name` is used. If both are omitted,
the default generated project name is used.

Composition currently offsets each block instance on the schematic to keep the
generated symbols readable. Connections are still emitted as logical
ref/pin-based nets rather than routed schematic artwork.

## Examples

Checked-in examples live under `examples/blocks/`.

```sh
go run ./cmd/kicadai --json inspect project examples/blocks/led_indicator
go run ./cmd/kicadai --json inspect project examples/blocks/composed_sensor_breakout
```

The example directories include:

- the request JSON under `examples/blocks/requests/`;
- the generated `.kicad_pro`;
- the generated `.kicad_sch`;
- `.kicadai/manifest.json` with generation metadata.

Generated `.kicad_pcb` files are intentionally omitted from these examples
until block PCB pad geometry and routing mature.

## Resolver Requirements

Blocks refer to KiCad symbol and footprint library IDs such as `Device:R` and
`Resistor_SMD:R_0805_2012Metric`. For deeper validation, configure the local
library resolver:

```sh
export KICADAI_KLC_ROOT=/path/to/klc
export KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols
export KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints
export KICADAI_TEMPLATES_ROOT=/path/to/kicad-templates

go run ./cmd/kicadai --json library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric
```

Resolver-backed validation is needed before agents should treat generated
designs as fabrication candidates. See
[library-resolver.md](library-resolver.md) for setup and cache behavior.

Pinmap validation checks whether schematic symbol-to-footprint assignments have
verified pin mappings:

```sh
go run ./cmd/kicadai --json pinmap validate examples/blocks/led_indicator
```

## AI Usage Pattern

For AI-assisted design, prefer this loop:

1. Use `block list` and `block show` to discover available primitives and
   parameter contracts.
2. Choose blocks and construct instantiate or compose request JSON.
3. Generate the project into a fresh output directory.
4. Run `inspect project`.
5. Run `evaluate project`.
6. Run `pinmap validate` when footprints are assigned.
7. Run KiCad CLI round-trip checks when available.
8. Surface warnings and blocked issues to the user before further edits.

Agents should preserve request JSONs alongside generated projects so designs can
be regenerated deterministically.

## Current Limitations

- Current blocks are structural schematic generators, not full PCB generators.
- Block-generated PCB files are not committed as examples because footprint pad
  geometry and routing are still incomplete.
- Connector breakout remains `experimental`.
- `usb_c_power` power-only mode does not emit D+/D- no-connect markers yet.
- MCU support uses a fixed ATmega328P-A role map; arbitrary MCU semantic
  extraction is not implemented.
- Composition checks voltage-domain conflicts but does not solve placement or
  routing.
- ERC/DRC integration is not yet part of the normal block workflow.
- Blocks do not execute external block-pack code. Future block packs must remain
  data-only and path-safe.
