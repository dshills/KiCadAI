# ESP32/BMP280 wired pressure-controller family

Review owner: the implementing Codex agent, September 13, 2026. Reference engineering is disclosed assistance. This is a bounded **software-validated design**, not an independently certified or bench-validated product. Ten fresh supported language requests passed; a separately approved live follow-up passed four refusals and two targeted clarifications. Earlier failed trials remain failed. Design/native evidence is reused only with verified unchanged source and artifact hashes. See [results](RESULTS.md) for staged acceptance and the inherited CI limitation.

## Fixed integrated design

The board contains an ESP32-WROOM-32E-N4 module, BMP280 sensor, manual RESET/BOOT, enable RC network, local decoupling, UART programming header and fixed peripheral header. It has 17 populated BOM positions. Only R3/R4 values change between profiles; pin assignments, placement, copper and board dimensions stay fixed.

| Profile | Installed R3/R4 | Firmware clock | Declared total bus capacitance |
|---|---|---|---|
| `standard` | 4.7 kΩ, RC0805FR-074K7L | 100 kHz | 50–200 pF |
| `fast` | 2.2 kΩ, RC0805FR-072K2L | 400 kHz | 50–100 pF |
| `low_current` | 10 kΩ, RC0805FR-0710KL | 100 kHz | 50–100 pF |

These are distinct electrical configurations, not different sensor types. `low_current` reduces pull-up sink current only. There is no claim of whole-board low power or battery life.

The 120 × 80 mm board has two copper layers and an explicit nominal 1.6 mm stack: 35 µm outer copper, 1.51 mm FR4 core, 10 µm mask per side. Dielectric constants in the native stack are nominal design inputs, not an impedance qualification or a selected fabricator's guaranteed stack. There are no mounting holes. Do not resize, substitute footprints, change copper or add hardware while retaining this family's evidence.

## Operating requirements

- External regulated 3.3 V only: input stays within **3.2–3.4 V**, including ripple, with **1–10 A source capability**. Capability is not a permitted board load; the source must be current-limited appropriately for the assembly.
- Indoor, noncondensing air, **10–35 °C**, **35–60% RH**, **300–1100 hPa**. Keep the sensor vent unobstructed, dry and free of solder/flux contamination; this is not a sealed/waterproof assembly.
- Sensor-rail ripple must remain ≤50 mV peak-to-peak. Supply and ground wiring, capacitor bias/aging and transient response must be checked on the eventual assembly; those quantities were not measured here.
- Radios disabled in firmware. No RF/antenna, wireless, EMC, USB, RS-232, regulator, battery, motor, relay, humidity sensor or accurate ambient-temperature claim.
- No hot-plug, reverse polarity, surge or ESD protection. Connect only with power off. Use common ground and 3.3 V logic. No extra pull-ups, external GPIO loads or independently powered peripheral injection.

The declared 50 pF minimum is a conservative engineering allowance, not an extracted board capacitance. Total loading includes pads, IC pins, connectors and any measurement probe. Exceeding a selected profile's maximum requires another design review, not an automatic resistor substitution.

## Pin and firmware contract

| Connection | Native net / pins |
|---|---|
| Supply J1 | 1 = VCC; 2 = GND |
| Programming J2 | 1 = VCC reference; 2 = GND; 3 = controller RX; 4 = controller TX |
| I²C | ESP32 module 33/GPIO21 → BMP280 3/SDI (SDA); module 36/GPIO22 → BMP280 4/SCK (SCL) |
| BMP280 power/mode | 8/VDD, 6/VDDIO and 2/CSB → VCC; 1/GND, 7/GND and 5/SDO → GND; address 0x76 |
| Reset/boot | module 3/EN: R1 10 kΩ to VCC, C3 1 µF to GND, SW1 to GND; module 25/GPIO0: R2 10 kΩ to VCC, SW2 to GND |
| Peripheral J3 | 1=MOSI, 2=SCL, 3=SDA, 4=MISO, 5=SCK, 6=GPIO; physical breakout only, external loads not qualified |

Program through an externally provided 3.3 V UART adapter; connect adapter TX to J2.3 and RX to J2.4. **Do not connect adapter power to J2.1 while J1 is powered.** Keep RESET asserted until the supply is stable. For download mode hold BOOT while releasing RESET, following the module's boot protocol; automatic DTR/RTS reset is not implemented. Firmware must disable radio and internal bus pull-ups, select GPIO21/GPIO22 and the selected bus clock, address 0x76, and apply the sensor's stored calibration. Firmware, power-up waveforms and actual sensor readings were not tested by the native-file checks.

## Electrical calculations and integrated review

`electrical.json` is calculated for every output, using the supplied numerical envelope rather than model-invented ratings. Resistance bounds include 1% tolerance plus 0.15% temperature drift. Rise time uses `0.8473 × Rmax × Ctotal`; the sensor's internal pull-up is ignored for the slow-rise bound and its minimum 70 kΩ is included in the high-current bound. Both lines are budgeted simultaneously low. Resistor dissipation is limited to half the 125 mW nominal rating.

At 3.4 V and maximum loading, rounded rise/sink results are standard **806 ns / 0.780 mA**, fast **189 ns / 1.612 mA**, and low_current **857 ns / 0.393 mA** per line. The source allocation is `500 + 1.12 + two bus sinks + 2 + 90 mA`, leaving over 400 mA headroom from a 1 A source. The 500 mA term follows Espressif's supply recommendation; it is **not a measured or guaranteed maximum module current**.

Supply and return connectivity were inspected beyond ERC: J1 reaches all module ground contacts and the sensor's two ground pads; both sensor rails have separate 100 nF bypasses C5/C6, with C4 retained. C1/C2 were moved close to the module in development after reviewing the initial placement. The controller's available centerline supply/return paths contain approximately 69.4/61.4 copper squares. This is a geometry inventory, not a solved PDN: via plating, connector contact resistance, capacitor effective capacitance/ESR, transient droop, thermal rise and ground-offset noise remain assembly-verification obligations. Native DRC does not prove these properties.

I²C speed, resistor/current margins, pin roles, same-rail supply compatibility and reset/programming topology have been reviewed against the sources below. Silicon logic-threshold guarantees remain subject to the manufacturers' stated test conditions; the rise calculation is not a substitute for checking assembled high/low levels at both ICs. No source rating is represented as a measurement of this board.

## Source-linked component evidence

- **Controller:** exact N4 module, 4 MB flash, 3.0–3.6 V recommended supply, onboard crystal and module pinout. The narrow family ambient lies within the module's range. See Espressif's [ESP32-WROOM-32E/32UE datasheet](https://documentation.espressif.com/esp32-wroom-32e_esp32-wroom-32ue_datasheet_en.html), and the [hardware supply/layout guidance](https://docs.espressif.com/projects/esp-hardware-design-guidelines/en/latest/esp32/pcb-layout-design.html). Chip-level examples are guidance, not a claim that bare-chip pins equal module pins.
- **Sensor:** Bosch [BMP280 datasheet, revision 1.26](https://www.bosch-sensortec.com/media/boschsensortec/downloads/datasheets/bst-bmp280-ds001.pdf), supply/current tables, I²C section and Figure 17. Supports the selected supply, pressure range, address straps, 1.12 mA peak pressure-current allocation, 50 mVpp ripple limit and two 100 nF bypasses. Its temperature output is used for compensation, not promised ambient accuracy.
- **Bus:** [NXP UM10204 revision 7](https://www.nxp.com/docs/en/user-guide/UM10204.pdf), rise-time and pull-up sizing sections, and [Espressif I²C guidance](https://docs.espressif.com/projects/esp-idf/en/stable/esp32/api-reference/peripherals/i2c.html). The reviewed clocks are 100/400 kHz; no high-speed modes are admitted.
- **Resistors:** Yageo [RC_L specification v14, November 14, 2025](https://www.yageogroup.com/content/datasheet/asset/file/PYU-RC_GROUP_51_ROHS_L), ordering code and RC0805 row: 1%, ±100 ppm/°C at these values, 125 mW standard power. This family-level primary evidence covers 2K2 even though the individual 2K2 web specsheet could not be fetched.
- **Capacitors:** C1/C4/C5/C6 [Murata GRM21BR71H104KA01](https://search.murata.co.jp/Ceramy/image/img/A01X/G101/ENG/GRM21BR71H104KA01-01.pdf), 100 nF/50 V/X7R; C2 [GRM21BR61A106KE19](https://search.murata.co.jp/Ceramy/image/img/A01X/G101/ENG/GRM21BR61A106KE19-01.pdf), nominal 10 µF/10 V/X5R; C3 [GCM21BR71H105KA03-01A](https://search.murata.co.jp/Ceramy/image/img/A01X/G101/ENG/GCM21BR71H105KA03-01A.pdf), 1 µF/50 V/X7R. All are 0805, ±10%; `L` identifies packaging. Nominal voltage ratings do not establish biased effective capacitance. C2's primary reference sheet was successfully retrieved and read locally, but dates to March 7, 2016. Obtain current approval sheets before ordering; no current availability/lifecycle claim is inferred.
- **Buttons:** [Omron B3U datasheet A162-E1](https://components.omron.com/us-en/system/files/2023-01/datasheet_pdf/A162-E1.pdf), B3U-1000P normally-open top actuator. Its operating/relative-humidity conditions informed the narrower family envelope. No washing/waterproof claim is made.
- **Headers:** [Samtec TSW catalog](https://suddendocs.samtec.com/catalog_english/tsw_th.pdf) and [TSW footprint](https://suddendocs.samtec.com/prints/tsw-xxx-xx-x-x-xx-xxx-footprint.pdf). TSW-102/104/106-07-L-S are single-row 2.54 mm pitch, 0.635 mm square posts; -07 tail is 2.54 mm. Current ratings depend on the mating part, not just the header. The catalog does **not** qualify TSW for lead-free wave soldering; assembly process needs its own review. Do not substitute an arbitrary cable/socket or claim assembly-ready output from ERC/DRC alone.

## Validation and readability boundary

Every delivered example must pass its configuration calculation, exact approved-reference/BOM match, native parser/writer/connectivity checks, actual KiCad 10.0.3 ERC, strict DRC including schematic parity, native schematic/PCB round trips, preview exports and unchanged-input hash gate. No per-finding exclusion or project-specific disabling was added. The CLI uses `--severity-all`, `--all-track-errors` and `--schematic-parity`; this is not a claim that every optional KiCad rule is enabled.

Actual reports list default-ignored ERC categories `single_global_label`, `four_way_junction`, `simulation_model_issue`, `footprint_filter`; and DRC categories `missing_courtyard`, `track_not_centered_on_via`, `tuning_profile_track_geometries`, `footprint_filters_mismatch`, `footprint_type_mismatch`. These defaults remain visible in the evidence. Connectivity, exact reference/footprint identity and placement/clearance checks provide separate coverage, but do not turn omitted native categories into executed checks. SPICE simulation and impedance/tuning qualification are outside this family.

The implementing agent inspected the actual rendered A3 schematic for all three profiles: groups are on-sheet, labels and resistor values are readable, and power, reset, programming and sensor sections are separated. The native ESP32 grouped-ground pin numbers are crowded beneath the symbol; net identity is visible and checked but this is a disclosed drawing limitation. The PCB is routed, spacious and inspectable in KiCad; visible assembly-reference silkscreen labeling is incomplete. No independent reviewer, assembly drawing approval, fabricated board, firmware boot or pressure measurement is claimed.
