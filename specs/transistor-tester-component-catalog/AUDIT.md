# Transistor Tester Component Catalog Audit

## Result

The catalog milestone passes its local acceptance gates.

The checked-in catalog now contains 171 records across 33 families:

- 156 verified;
- 1 library-derived;
- 6 rule-inferred;
- 7 placeholder; and
- 1 blocked.

This milestone added six exact verified parts, one structural-only generic test
connector, and four draft-only module records. No provisional module can satisfy
structural, connectivity, ERC/DRC, or fabrication-candidate acceptance.

## Exact identities

| Catalog ID | Evidence boundary |
| --- | --- |
| `relay.omron.g5v_1.dc5` | G5V-1 DC5 symbol, through-hole footprint, coil pins, duplicated common, NO, and NC verified. Physical selection still requires a readable 5 V case marking. |
| `driver.st.uln2803a.dip18` | Active ST DIP-18 identity, eight input/output channel pairs, ground, and suppression-diode common verified. |
| `adc.ti.ads1115idgsr.vssop10` | Bare ADS1115 DGS/VSSOP-10 identity and I2C/analog pins verified. Breakout-board header and pull-ups are not implied. |
| `dac.microchip.mcp4725a0t_e_ch.sot23_6` | Bare MCP4725A0 SOT-23-6 identity and pins verified; lifecycle is recorded as NRND. Breakout-board details are not implied. |
| `bjt.st.bd139_16.to126` | Exact ST E-C-B pin order and headline ratings verified. Linear SOA and thermal design remain review-required. |
| `switch.ti.cd74hc4053e.dip16` | Exact TI PDIP-16 channel, select, inhibit, and supply functions verified; lifecycle is recorded as NRND. |

The relay does not claim a trusted simulation model in this milestone. An
initial unregistered model claim was removed so the model-provenance registry
continues to fail closed.

## Provisional identities

The ESP32 development board, ADS1115 breakout, MCP4725 breakout, and SSD1306
display remain `placeholder` records. Their connector symbols expose expected
logical functions for draft schematics only. The attached header footprints
are explicitly non-mechanical surrogates.

Promotion requires actual-hardware evidence:

- readable front/back board photos or purchasing records;
- silkscreened header order;
- installed pull-ups, straps, or level shifting;
- module dimensions and mounting holes;
- readable relay case marking; and
- exact ZIF/test-socket identity or a dimensioned drawing.

## Regression evidence

Focused tests prove:

- G5V-1 coil, common, NO, and NC function mappings;
- all eight ULN2803A `IN_n` to `OUT_n` channel pairs;
- ADS1115, MCP4725, BD139, and CD74HC4053 exact bindings;
- draft-only acceptance for all four provisional modules;
- structural-only acceptance and terminal neutrality for the generic test
  socket; and
- presence of all six human-verified built-in pinmaps.

The catalog change also regenerated deterministic coverage, capability-report,
and schematic-IR provenance hashes.

## Local verification

The following commands passed:

```sh
make GO_TEST_FLAGS='-short -count=1' test
make lint
make COVER_TEST_FLAGS='-short -count=1' coverage-check
GOCACHE=/tmp/kicadai-gocache go test -count=1 \
  ./internal/writercorrectness ./internal/kicadfiles/roundtrip
git diff --check
```

Generated-code-excluded coverage was 81.4%, above the 75% threshold.

The new 11-record catalog slice was resolved against the KiCad 10.0.3
application libraries. All scoped symbol, pin, footprint, and pad bindings had
zero error or blocked issues. Library-wide warnings for unnamed mechanical
pads were not treated as scoped component failures.

The optional installed-KiCad tier passed:

```sh
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
GOCACHE=/tmp/kicadai-gocache \
go test -count=1 ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier$/(esp32_wroom_32e_minimal_pass|usb_c_i2c_sensor_3v3_protected|usb_c_led_indicator_protected)$' \
  -v
```

All three fixtures passed clean ERC, strict DRC, connectivity, route completion,
writer correctness, and zero round-trip differences.

## Existing whole-catalog issue

Running installed-library validation over every selectable catalog record still
reports four symbol-pin errors for the pre-existing
`mcu.espressif.esp32_wroom_32e` record. KiCad 10's installed inherited
`RF_Module:ESP32-WROOM-32E` symbol is not exposing pins 1, 15, 38, and 39 through
the current resolver path. The promoted ESP32 fixture nevertheless passes the
installed-KiCad tier. This issue predates and is independent of the new
transistor-tester records; the scoped 11-record validation has no binding
errors.
