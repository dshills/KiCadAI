# Transistor Tester Component Catalog Specification

## Objective

Add the component identities needed to describe Transistor Tester V1 and its
planned analog switching stage without weakening KiCadAI's evidence gates.
Exact manufacturer parts may become connectivity-capable only when their
symbol, package, and pin mapping are supported by primary-source and installed
KiCad-library evidence. Unknown plug-in boards must remain draft-only until the
actual hardware is identified and measured.

## Scope

### Exact catalog records

The checked-in catalog shall contain:

- Omron `G5V-1 DC5`, through-hole SPDT relay;
- STMicroelectronics `ULN2803A`, DIP-18 Darlington array;
- Texas Instruments `ADS1115IDGSR`, DGS/VSSOP-10 ADC;
- Microchip `MCP4725A0T-E/CH`, SOT-23-6 DAC;
- STMicroelectronics `BD139-16`, SOT-32/TO-126 NPN transistor; and
- Texas Instruments `CD74HC4053E`, DIP-16 triple analog switch.

The relay record shall expose `COIL_A`, `COIL_B`, `COMMON`,
`NORMALLY_OPEN`, and `NORMALLY_CLOSED`. Both internally common relay
terminals shall remain visible in the mapping. Choosing the 5 V record for a
physical build still requires the case marking on the purchased relay to match.

The ULN2803A record shall expose eight visually distinct input/output channel
pairs, `COM`, and `GND`. `IN_n` shall correspond to `OUT_n`; `COM` denotes the
internal suppression-diode common and is not interchangeable with ground.

The BD139 record shall use the exact ST pin order. Headline ratings and pinout
do not constitute proven linear-mode Safe Operating Area or heatsink evidence.

### Provisional module records

The catalog shall contain draft-only records for:

- a 38-pin ESP32-DevKitC-compatible board;
- an ADS1115 I2C breakout;
- an MCP4725 I2C breakout; and
- an SSD1306 128x64 I2C display breakout.

These records document expected logical functions and the evidence still
needed. They shall use `placeholder` confidence and shall fail structural,
connectivity, ERC/DRC, and fabrication-candidate acceptance. Any connector
footprint attached to a provisional record is a schematic surrogate, not a
module footprint.

The ESP32 provisional record may preserve the Espressif ESP32-DevKitC V4
reference header order for review, but it must not claim that order as the
actual HiLetgo board identity. Flash-connected GPIO6 through GPIO11 and the
mutually exclusive power-source boundary shall remain explicit.

### Transistor fixture connector

A neutral three-terminal record shall expose only `TERMINAL_1`,
`TERMINAL_2`, and `TERMINAL_3`. It shall not assign base, collector, emitter,
gate, drain, or source roles. A generic 1x03 header may provide structural
evidence, but the actual ZIF socket remains a separate mechanical identity.

## Evidence sources

Primary-source evidence shall come from:

- Omron G5V-1 product data and terminal drawing;
- ST ULN2803A product data and datasheet;
- TI ADS1115 product data and datasheet;
- Microchip MCP4725 product data and datasheet;
- ST BD139 product data and datasheet;
- TI CD74HC4053 product data and datasheet; and
- Espressif ESP32-DevKitC V4 documentation for the provisional reference
  header order.

Installed KiCad symbols and footprints shall be resolved locally. Human-reviewed
symbol-to-pad bindings shall be recorded in the built-in pinmap registry.

## Out of scope

- Inferring a purchased module from a marketplace family name.
- Treating a generic pin header as a mechanically correct module footprint.
- Building the complete transistor tester circuit block in this milestone.
- Claiming analog accuracy, protection, calibration, thermal safety, or
  production readiness from catalog identity alone.
- Querying live distributor inventory or current pricing.

## Acceptance criteria

1. The checked-in catalog validates deterministically.
2. The six exact records resolve their installed KiCad symbol and footprint
   identities and carry exact function-to-pin and function-to-pad mappings.
3. ULN2803A channel-pair regression tests prove `IN_n` to `OUT_n` correspondence.
4. Relay regression tests prove the coil, duplicated common, NO, and NC
   mappings.
5. The four provisional module records are accepted for draft output only.
6. The generic transistor socket is structural-only and contains no fixed
   transistor terminal roles.
7. Catalog coverage and golden output are updated deterministically.
8. Local unit, lint, coverage, catalog-library, and relevant protected KiCad
   regression suites pass.
9. Documentation states both the new capability and the remaining physical
   evidence boundary.
10. The staged diff receives Prism review before commit.
