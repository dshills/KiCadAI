# Design Workflow Examples

These requests are executable `kicadai design create` inputs. They use explicit
circuit blocks rather than natural-language planning, and the design workflow
test suite verifies that every `*.json` file in this directory can generate
readable KiCad project, schematic, and PCB files.

## LED Indicator

```sh
kicadai --request examples/design/led_indicator.json --output ./out/led_indicator --overwrite design create
```

This structural example generates an active-high green LED indicator with a
270 ohm current-limiting resistor. Routing is skipped intentionally; the example
demonstrates block planning, component selection, schematic identity
properties, PCB realization, placement, and project artifact writing.

## Active-Low LED

```sh
kicadai --request examples/design/active_low_led.json --output ./out/active_low_led --overwrite design create
```

This structural example exercises the active-low LED path and explicit voltage,
LED forward-voltage, and resistor parameters. Routing is skipped intentionally.

## Op-Amp Headphone Buffer

```sh
kicadai --request examples/design/amplifier/opamp_headphone_buffer.json --output ./out/opamp_headphone_buffer --overwrite design create
```

This draft amplifier seed uses the supported `opamp_gain_stage`
block plus audio input, headphone output, and power connector breakouts. Routing
is skipped intentionally and placeholder component confidence is allowed. The
signal path includes an `output_dc_block_pending` placeholder connector and
`HP_OUT_DC_BLOCK_REQ` net alias to mark where a real series output
DC-blocking/protection block belongs. Known gaps remain: headphone load-drive
behavior, analog stability/layout, output DC blocking, output protection, and
connector pinout mapping are not verified, so this example must not be treated
as fabrication-ready. The input path requests the op-amp block's AC-coupled
single-supply bias network; practical output offset management is still future
work.

Generated artifacts include:

- `<output>/<name>.kicad_pro`
- `<output>/<name>.kicad_sch`
- `<output>/<name>.kicad_pcb`
- `<output>/.kicadai/transaction.json`
- `<output>/.kicadai/manifest.json`

## Current Limitations

These examples are artifact-generation fixtures, not fabrication-ready boards.
They currently surface writer-correctness feedback around PCB net-code evidence
after project write when resolver-backed library context is not supplied.

The previous multi-block I2C sensor breakout request is not a default design
example right now because the current generic sensor/connector realization can
lead to unsatisfiable placement constraints or project-write failures. It
should return once the sensor symbol/footprint model and connector PCB
realization are hardened enough for a default runnable fixture.

## Optional KiCad-Backed Examples

Optional fixtures live under `examples/design/kicad-backed/`. They require
`KICADAI_KICAD_CLI` for local test runs and currently document `expected_fail`
evidence for richer generated boards that block before stable KiCad ERC/DRC
proof.

See `examples/design/kicad-backed/README.md` for the fixture list and command
shape.
