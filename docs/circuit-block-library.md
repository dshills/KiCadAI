# Circuit Block Library

The circuit block library provides reusable, parameterized schematic fragments
for common circuits. Blocks are exposed through the `kicadai block` CLI and are
intended to be a safer AI-facing design primitive than raw file writes.

The library generates structural schematic operations and PCB realization
metadata that the design workflow can place, route, write, and validate. Most
individual blocks deliberately retain `structural` verification even when a
specific composed design has stronger promotion evidence. A whole-design
KiCad-backed `pass` is not an automatic fabrication claim for every parameter
combination of its constituent blocks.
The current readiness review and gap matrix are tracked in
[circuit-block-readiness.md](circuit-block-readiness.md).

## Available Blocks

Use `block list` to see the registry:

```sh
kicadai block list
```

Current built-in blocks:

| Block ID | Category | Current level | Purpose |
|---|---|---|---|
| `amplifier_bias_network` | analog | `structural` | Diode-string Class-AB headphone bias network. |
| `amplifier_input_buffer` | analog | `structural` | AC-coupled input and bias-reference conditioning. |
| `amplifier_supply_decoupling` | power | `structural` | Local single/dual-rail amplifier decoupling. |
| `canned_oscillator` | timing | `structural` | Catalog-backed canned-clock source. |
| `class_a_voltage_stage` | analog | `structural` | Bounded Class-A voltage amplification stage. |
| `class_ab_output_pair` | analog | `structural` | Headphone-scale complementary emitter follower. |
| `class_ab_output_stage` | analog | `structural` | Bounded Class-AB headphone output stage. |
| `class_ab_speaker_power_stage` | analog_power | `structural` | Protected 10 W/8 ohm speaker-power slice. |
| `connector_breakout` | interconnect | `structural` | Generic connector with exported named pins. |
| `crystal_oscillator` | timing | `structural` | Crystal and load-capacitor clock network. |
| `dc_blocking_capacitor` | analog | `structural` | AC-coupled load path. |
| `esd_protection` | protection | `structural` | Entry-anchored 5 V ESD shunt. |
| `esp32_wroom_32e_minimal` | digital | `erc_drc_verified` | Exact ESP32-WROOM-32E-N4 minimal system and antenna keepout. |
| `headphone_output_connector` | interconnect | `structural` | Mono headphone load interface. |
| `headphone_output_protection` | analog | `structural` | Headphone output coupling and protection. |
| `i2c_sensor` | sensor | `structural` | Generic or concrete I2C sensor with pull-ups and decoupling. |
| `led_indicator` | indicator | `structural` | Series resistor plus LED indicator. |
| `mcu_minimal` | digital | `structural` | ATmega328P-A minimal system with reset, GPIO, and ISP. |
| `opamp_gain_stage` | analog | `structural` | Non-inverting op-amp gain stage with feedback. |
| `reset_programming_header` | mcu_support | `structural` | Reset and programming interface support. |
| `reverse_polarity_protection` | protection | `structural` | Series-diode reverse-polarity protection. |
| `speaker_opamp_driver` | analog | `structural` | Driver stage for the bounded speaker lane. |
| `speaker_output_protection` | protection | `structural` | DC-fault detection and relay-isolated speaker output. |
| `usb_c_power` | power | `structural` | USB-C sink power with optional fuse/TVS/bulk protection. |
| `voltage_regulator` | power | `structural` | Verified fixed 3.3 V LDO profiles and capacitors. |

Inspect one block:

```sh
kicadai block show led_indicator
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

Only `roundtrip_verified` or stronger block-level records should be treated as
autonomous block evidence without an explicit warning. Whole-design promotion
reports may prove a narrower composition at a stronger level; inspect the
report instead of projecting that result onto every use of the block.

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
kicadai \
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
kicadai \
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
kicadai \
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
kicadai inspect project examples/blocks/led_indicator
kicadai inspect project examples/blocks/composed_sensor_breakout
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

kicadai library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric
```

Resolver-backed validation is needed before agents should treat generated
designs as fabrication candidates. See
[library-resolver.md](library-resolver.md) for setup and cache behavior.

Pinmap validation checks whether schematic symbol-to-footprint assignments have
verified pin mappings:

```sh
kicadai pinmap validate examples/blocks/led_indicator
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

- Most blocks remain `structural` at the block contract level even though the
  design workflow can realize their PCB fragments and several exact
  compositions have KiCad-backed pass evidence.
- Standalone checked-in block examples emphasize schematic/project output;
  whole-board placement, routing, and promotion are exercised by design
  fixtures and the block verification harness.
- `usb_c_power` power-only mode does not emit D+/D- no-connect markers yet.
- Legacy MCU blocks retain fixed target profiles for compatibility. The
  behavior-driven catalog provider instead selects among verified ATmega328P-A,
  ESP32-WROOM-32E, and STM32G031K8T6 records and resolves their modeled
  alternate functions. It does not infer MCU evidence from arbitrary KiCad
  symbols or support unverified device and module variants.
- The composed design workflow supports deterministic placement and routing for
  proven shapes, not general dense-board autorouting.
- Block verification can require KiCad ERC/DRC, but default tests remain
  hermetic and report external checks as skipped unless configured.
- The protected 10 W/8 ohm speaker composition is bounded evidence, not general
  bridge, mains, high-power, or arbitrary-load amplifier support.
- Blocks do not execute external block-pack code. Future block packs must remain
  data-only and path-safe.
