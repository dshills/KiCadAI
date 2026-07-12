package aiprovider

const BMP280ReferenceCapabilityContext = `Supported design scope:
- one protected USB-C powered BMP280 I2C breakout
- verified component ID sensor.bosch.bmp280.lga8
- USB-C input voltage 5V
- USB-C input current requirement 500 mA
- regulated sensor and connector rail 3.3V at 100 mA
- existing USB-C fuse, TVS, and bulk-capacitance options
- require fuse, TVS, and bulk capacitance; keep signal ESD and reverse-polarity protection optional
- existing BMP280 pull-up and local-decoupling options
- one external I2C connector
- 100 mm by 75 mm two-layer reference board with 0.25 mm edge clearance
Do not select identifiers or features outside this list.`

const ProtectedLEDReferenceCapabilityContext = `Supported design scope:
- one protected USB-C powered LED indicator
- USB-C power-only sink at 5V
- existing USB-C CC pull-down, fuse, TVS, and bulk-capacitance capabilities
- require fuse, TVS, and bulk capacitance; keep reverse-polarity protection optional
- one active-high indicator LED characterized at 2.0V and 5 mA
- one 600 ohm current-limiting resistor
- 50 mm by 30 mm two-layer reference board
- automatic readable schematic layout and ERC/DRC acceptance
Do not select identifiers, components, or features outside this list.`
