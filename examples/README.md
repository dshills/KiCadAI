# KiCadAI Demo Schematics

These examples are hand-authored KiCad project fixtures that range from a single LED indicator to a hierarchical sensor node. They are intended as small reference projects for the direct file writers and for AI-assisted schematic generation experiments.

Only `01_led_indicator` currently has a matching Go generator. The remaining
schematic examples are checked-in fixtures and should not be mechanically
rewritten until dedicated generators exist for them.

| Example | Focus |
|---|---|
| `01_led_indicator` | Single resistor and LED from VCC to GND. |
| `02_button_pullup` | Pull-up resistor, push button, and output label. |
| `03_rc_filter` | Passive RC low-pass filter with input/output labels. |
| `04_555_timer` | Medium-complexity timer oscillator schematic. |
| `05_sensor_node` | Hierarchical project with power, MCU, and sensor sheets. |
| `06_class_ab_headphone_amp` | Op-amp gain stage with diode-biased class AB headphone output. |
| `09_class_a_headphone_amp` | Class A headphone amplifier fixture with bias, feedback, and output-load landmarks. |
| `10_opamp_buffer_headphone_amp` | Op-amp headphone buffer fixture with buffer feedback and output-stage landmarks. |

Open each directory in KiCad by opening its `.kicad_pro` file.

Fixture UUIDs are deterministic so parser, round-trip, and semantic tests stay
stable. Treat each example directory as a standalone project; do not merge the
fixtures into one KiCad project without regenerating UUIDs.

Structured intent examples live under `examples/intent/`. Files prefixed with
`synthesis_` exercise the semantic design synthesis trace:

- `synthesis_mcu_i2c_explicit_supply.json`: MCU, I2C sensor, connector, and an
  explicit 3.3V voltage domain.
- `synthesis_uart_programming.json`: UART programming topology evidence.
- `synthesis_unknown_supply_blocked.json`: intentional blocked unknown supply
  alias fixture.
- `synthesis_external_clock_blocked.json`: intentional external-clock topology
  limitation fixture.
- `regulator_ap2112k_sensor.json`: USB-powered 3.3 V sensor breakout that
  exercises the AP2112K SOT-23-5 regulator path.
- `regulator_high_current_fallback.json`: high-current 3.3 V rail fixture that
  must not select AP2112K.
- `regulator_insufficient_headroom_blocked.json`: intentional blocked regulator
  headroom fixture.
- `amplifier_class_a_headphone.json`: Class A headphone amplifier intent that
  remains partial until output-stage, thermal, and PCB evidence are verified.
- `amplifier_class_ab_headphone.json`: Class AB headphone amplifier intent that
  remains partial until bias, stability, and load-drive evidence are verified.
- `amplifier_low_voltage_power_blocked.json`: intentional blocked low-voltage
  power amplifier topology fixture.

Generated design requests live under `examples/design/`. The amplifier seed is
`examples/design/amplifier/opamp_headphone_buffer.json`; it is draft-only and
uses supported op-amp/connector blocks plus an explicit
`output_dc_block_pending` placeholder. Optional KiCad-backed amplifier metadata
lives under `examples/design/kicad-backed/` as
`opamp_headphone_buffer_kicad_candidate`, marked `expected_fail` until verified
component evidence, output protection/DC blocking, analog layout proof, and
ERC/DRC-clean evidence exist.

Round-trip validation for the Go-generated LED schematic, Go-generated LED PCB,
checked-in LED schematic fixture, and checked-in generated PCB fixture is
available as an opt-in integration test. Run this command from the repository
root:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1 \
KICADAI_ROUNDTRIP_ARTIFACT_DIR="$(pwd)/examples/roundtrip_artifacts" \
go test ./internal/kicadfiles/roundtrip
```

KiCad CLI 7.0 or later is required. Set `KICADAI_KICAD_CLI` when `kicad-cli`
is not available on `PATH`; its value should be the absolute path to the
`kicad-cli` executable. The `examples/roundtrip_artifacts/` output directory
is created by the test harness when needed and ignored by git.
