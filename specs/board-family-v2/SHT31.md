# SHT31 reference — development qualification notes

Status: shared generation and all 14 native/export checks pass for both profiles; full electrical/assembly review, manufacturing-data visual review and final live acceptance remain pending. Reviewer: implementing Codex agent, not an independent engineer. No fabricated or measured hardware.

The second board reuses the ESP32-WROOM-32E-N4, external regulated 3.3 V input, reset/boot, UART and fixed headers/outline/stack of [the first family](../board-family-v1/FAMILY.md). It replaces pressure measurement with an SHT31-DIS-B temperature/humidity sensor, changes the sensor footprint and wiring, removes C6 and relocates the local C5 bypass. U2 is at (100,15) mm and C5 at (103.875,16.5) mm. Reference routes are engineered once; production generation must not route or repair them.

## Manufacturer constraints under review

Sensirion's [December 2022 v7 datasheet](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf), Tables 3, 7, 8, 21 and 23, identifies the selected bare sensor as **SHT31-DIS-B2.5kS**. Supply range: 2.15–5.5 V; measuring current: up to 1.5 mA. Keep its heater disabled for this family. Retain the narrower shared board supply/environment limits, not the sensor's full ratings.

| Pin | Connection |
|---|---|
| 1 SDA | ESP32 GPIO21 / SDA |
| 2 ADDR | GND, address 0x44 |
| 3 ALERT | Unconnected |
| 4 SCL | ESP32 GPIO22 / SCL |
| 5 VDD | 3.3 V rail, local 100 nF C5 |
| 6 nRESET | Unconnected, as recommended when unused |
| 7 R, 8 VSS, 9 exposed pad | GND |

The sensor requires external bus pull-ups; its table specifies 300 ns maximum rise time and 0.4 V maximum low output at 3 mA. Do not import the BMP280 internal-pull-up model. Firmware must validate the sensor's CRC and convert its digital readings. Sensor specifications are not assembled-board accuracy measurements.

## Implemented bounded profiles

These are implemented configurations with offline validation, not final milestone acceptance. Using the first family's 1.0115 worst-case resistance multiplier and `0.8473 × R × C`:

- Standard: 4.7 kΩ, 100 kHz, declared total loading 50–70 pF; worst-case rise approximately 282 ns.
- Fast: 2.2 kΩ, 400 kHz, 50–100 pF; worst-case rise approximately 189 ns.
- The previous low-current profile is not supported for this sensor envelope.

Current allocation now includes 500 mA controller supply-recommendation allocation (not a measured peak), 1.5 mA sensor measuring current, both worst-case bus sinks, 2 mA control circuits and 90 mA reserve. The minimum declared source capability remains 1000 mA. Supply slew below 20 V/ms within the sensor operating range, heater-off firmware, CRC checking, power/reset waits and handling restrictions appear in the catalog, AI contract and generated electrical notes. Non-numeric fixed conditions are not bench measurements or automatically sensed properties; the AI must refuse contradictory requests.

Complete-board logic-threshold margins, decoupling and final assembly qualification still require consolidated source-backed review. Temperature/humidity capability must not imply pressure, wireless, extra loads or guaranteed ambient accuracy.
