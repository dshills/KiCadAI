# SHT31 prototype second engineering review

2026-10-08. Agent-led review accepted by the user; this is a second review pass,
not an independent engineer's certification.

## Build recommendation

The standard SHT31 design is a reasonable candidate for a small, supervised
prototype build after the user approves the exact package, risks and cost and
the selected supplier accepts the assembly process. Assemble and bring up one
unit first before committing remaining units. An additional human engineering
reviewer is not a prerequisite under the user's chosen process.

Do not treat this as fabrication authorization. Supply integrity, assembly yield,
bus waveforms and ambient-measurement performance remain unmeasured. The board
is unsuitable for unattended or safety-critical use and has no input protection.

## Rechecked design

| Check | Evidence and result |
|---|---|
| Pin mapping | Separate source-level audit checks 102 numbered pad instances across all 16 references, including all ESP32 ground-pad instances and intentionally unused pins. UART RX/TX, GPIO21/22, reset, boot, sensor address and exposed pad agree with manufacturer pin functions. Deliberately altered SDA/power/UART/reset nets fail the audit. |
| BOM | Exact MPN checks cover all 16 parts, including 4.7 kohm pull-ups, controller N4 flash variant, sensor and decoupling/reset capacitors. Missing parts and substituted pull-ups fail. These checks do not establish availability or component authenticity. |
| Supply | The declared 3.2-3.4 V range lies inside both ICs' operating supply ranges. The external 1 A capability exceeds the module's 0.5 A supply recommendation. Current allocation is not a measured load; rail droop/ripple/slew and connector/return losses remain bench gates. |
| Reset and boot | R1/C3 and SW1 feed EN; R2/SW2 feed GPIO0. UART uses module pads 34/35 and J2.3/J2.4. Unloaded strapping pins retain module defaults. Manual download, no DTR/RTS wiring and no adapter-power connection remain required. |
| Bus | Independently recalculated worst-case rise: 0.8473 x 4700 x 1.0115 x 70 pF = 281.967 ns. Only about 18 ns separates this estimate from the 300 ns limit. Probe loading cannot be ignored. Do not extend wiring or attach extra loads without redesign. |
| Geometry and exports | The retained native project/schematic/PCB match the reviewed reference. U2 copper pad pitch/row spacing, offset paste and shared row mask remain unchanged. Strict CAD/export gates check connectivity/parity, placement and drill registration; export reports contain 70 plated holes, zero unplated holes and 16 top-side placements. |
| Firmware | GPIO21/22, address 0x44, 100 kHz, external pulls, bounded transfers, heater off, CRC, reset/status checks and conversion formulas were rechecked. No radio initialization, retry loop, auto-calibration or silent address fallback. Sensor waits include a rounding margin. |
| Programming package | Factory app offset 0x10000 and partition table offset 0x8000 match the decoded partition binary; bootloader is at 0x1000. App/bootloader image checksums passed previously and delivered binary hashes remain bound. No OTA partition or boot_app0 dependency. |

Primary references checked again: [Espressif module datasheet, pin definitions,
boot and electrical tables](https://www.espressif.com/sites/default/files/documentation/esp32-wroom-32e_esp32-wroom-32ue_datasheet_en.pdf),
[Sensirion SHT3x-DIS v7, pinout, commands and timing](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf),
and [the pinned IDF driver API](https://docs.espressif.com/projects/esp-idf/en/v4.4.7/esp32/api-reference/peripherals/i2c.html).
Nominal logic margins remain subject to the manufacturer's stated test conditions.

## Findings and disposition

No new blocking circuit or firmware defect was identified. Two process improvements
were made in source: the separate pad/net/BOM audit now runs during packaging and
in host CI, and the release hold no longer demands an unavailable independent
engineer. The audit has negative controls and does not replace copper-continuity
or clearance checks. All affected output is regenerated into a new package;
previous packages and historical evaluations remain intact.

The original sparse silkscreen and crowded small-part labels remain usability
risks, not concealed passes. Use numbered native/assembly views together with
placement and BOM data. Module exposed-pad vias may wick solder, and U2 needs
a contamination-safe, no-wash process. These require an assembler's process
acceptance even without an independent design reviewer. Current approval sheets,
exact parts and supplier stack/finish/feature capability remain purchase gates.

## First hardware decision

Choose a supplier and obtain its acceptance of the requirements in REVIEW.md,
then approve a specific package hash, quantity and cost. Keep the external supply,
UART isolation and test equipment requirements in BENCH.md. Physical acceptance
requires recorded measurements; green CI or a valid sensor ACK cannot supply them.
