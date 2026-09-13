# Two-family electrical and assembly-data review

Result: the SHT31 reference is accepted for **bounded software generation and reviewed data export**, following a manufacturer-directed mask/paste correction. This is not independent engineering certification, a qualified assembly process, fabrication approval or a guarantee of working hardware. Reviewer: implementing Codex agent, September 13, 2026. Final language acceptance, example publication and updated PR CI are separate remaining tasks.

## Whole-board electrical review

The shared controller, input, reset/boot, UART, headers, resistors and capacitors retain the [v1 component review and restrictions](../board-family-v1/FAMILY.md). The SHT31 board has 16 populated parts: U2 changes and C6 is removed; C5 remains the local 100 nF bypass. Tests bind all nine sensor copper pads to the [declared pin map](SHT31.md). No input regulator/protection, extra-load capability, firmware or radio qualification is implied.

The declared envelope remains 3.2-3.4 V, 10-35 degrees C and 35-60% RH, noncondensing. Source capability remains at least 1 A, with an appropriate external current limit. The 50 mV peak-to-peak sensor-rail limit is retained as a **family engineering constraint**, not claimed as an SHT31 datasheet limit. Supply/return resistance, effective biased capacitance, transient droop, ground offset and actual bus loading require measurement. The unchanged controller power-path geometry is not a solved power-integrity model.

Results from each fresh `integration-10` electrical report, using maximum declared loading and worst-case resistor bounds:

| SHT31 profile | 30-70% rise | Sink per line | Pull-up dissipation | Allocation / 1 A source margin |
|---|---:|---:|---:|---:|
| standard, 4.7 kohm / 70 pF | 281.97 ns | 0.732 mA | 2.488 mW | 594.964 / 405.036 mA |
| fast, 2.2 kohm / 100 pF | 188.55 ns | 1.563 mA | 5.316 mW | 596.627 / 403.373 mA |

The current allocation is `500 + 1.5 + 2*bus_sink + 2 + 90 mA`. The controller term is a supply-recommendation allowance, not its measured/guaranteed maximum. The 90 mA reserve is an engineering allowance. Rise time is an RC calculation using declared capacitance, not measured timing. Firmware timing, clock-low/high periods, fall time and receiver levels must also be checked; passing this one calculation is insufficient to establish bus compliance.

At nominal 3.3 V, calculated low-level margins are `0.25*3.3 - 0.4 = 0.425 V` for sensor-to-controller and `0.3*3.3 - 0.1*3.3 = 0.660 V` for controller-to-sensor. These use sensor Table 21 and controller Table 15. Importantly, Espressif labels its DC table **3.3 V, 25 degrees C**; these are nominal-condition calculations, not guaranteed margins across the whole family supply/temperature envelope. The typical 28 mA sink entry is not a guaranteed minimum. Open-drain released-high voltage depends on pull-ups, leakage and offsets; push-pull VOH is not the correct released-high model. [Espressif module datasheet v2.1, sections 6.2-6.3](https://documentation.espressif.com/esp32-wroom-32e_esp32-wroom-32ue_datasheet_en.html).

Firmware must honor the selected clock/address and sensor command/conversion timing, validate CRC, leave the heater off and disable controller internal bus pulls. Use the specified startup/reset waits; resetting the controller does not reset U2. The sensor's measuring-current allocation and logic/timing bounds are taken from [Sensirion SHT3x-DIS v7, Tables 3-4 and 21](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf). No firmware behavior or electrical waveform was measured by these software checks.

## Resolved footprint finding

The PDF's section 5.3/Figure 15 was visually inspected, not inferred from a package name. Existing copper matches the recommended land geometry, and the existing center paste was already reduced. The old perimeter paste had no outward offset and its mask openings followed copper without expansion. The new project-local footprint is `Sensor_Humidity:SHT31-DIS_SensirionV7_MaskPaste`; it is explicitly a KiCad-derived custom reference, not an unmodified upstream library part.

The revised data implements 0.1 mm outward I/O paste offsets, one mask opening per four-pad row with 0.075 mm outer clearance, and 0.075 mm exposed-pad mask clearance. Copper, routing, pin assignments and component locations are unchanged. The reduced chamfered center paste is retained. Its polygon area is 1.40355 square mm versus 1.655 square mm copper: approximately 84.8% coverage. [Sensirion v7 section 5.3/Figure 15](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf).

Tests check every new paste center and size, both mask-row bounds, unchanged electrical pads, unchanged source inputs and the captured footprint against the recipe. The intentional shared mask is enabled **only on U2** with KiCad's footprint-local `allow_soldermask_bridges` attribute. Board-wide settings remain unchanged; copper-clearance checks remain enabled. [KiCad footprint serialization](https://docs.kicad.org/doxygen/pcb__io__kicad__sexpr_8cpp_source.html).

This is reference engineering before final evaluation, not editing evaluated outputs. `sht31-assembly-01` exposed a symbol-library mismatch; the instance-only update corrected it. A failed import caught an extra terminal newline and remains recorded. `integration-08` failed native round-trip canonicalization and withheld manufacturing exports. The recipe was corrected to canonical coordinate/fill/property forms, without weakening the checker. [Accepted reference identities](development/reference-assembly-canonical-import.json) bind the fresh `sht31-assembly-04` source. Earlier canonical files, failed runs and receipts remain intact.

## Assembly and measurement boundary

Use an ESD-controlled, contamination-conscious process. Sensirion recommends no-clean paste and no board washing; keep the sensing opening free of coatings, solvents and mechanical contact. The sensor should be installed in the last solder cycle, with at most three cycles; manual soldering is not recommended. Immediate post-reflow humidity offset can be temporary, so do not treat initial readings alone as a calibrated performance result. The assembler must apply the detailed profile and handling limits in [Sensirion Handling Instructions v9, sections 1-2](https://sensirion.com/media/documents/6D95AA80/6840311F/HT_Handling_Instructions_SHTxx.pdf).

That sensor profile is not a whole-board process approval. Confirm compatibility with the ESP32 module, Omron switches, Murata/Yageo parts and through-hole Samtec headers, including stencil thickness, orientation, plating/finish, rework and the module's exposed-pad via solder-wicking treatment. In particular, the inherited TSW header evidence does not qualify lead-free wave soldering. No purchase, component substitution, fabrication or assembly is authorized by this review.

U2's center is about 51.25 mm from U1, but separation is not thermal isolation. The board has no characterized enclosure, airflow model or measured controller-to-sensor heating. Sensor readings describe local conditions and do not establish system-level ambient accuracy; the [Sensirion design guide v2, thermal-design sections](https://sensirion.com/media/documents/FC5BED84/662B494D/Sensirion_Humidity_Temperature_Design_Guide.pdf) explains these dependencies. Physical bring-up must check input/rail waveforms, reset/boot/UART, both bus endpoints and sensor response over the declared conditions. No pressure, RF, EMC, battery-life or environmental-sealing capability is added.

## Fresh manufacturing-data inspection and evidence

The revised SHT31 export was independently rendered and visually inspected in 15 views: native schematic/PCB; top/bottom overview and copper/drill overlays; sensor copper, paste and mask; whole-board paste/mask; outline and both drill maps. The shifted paste and grouped mask are visible. Copper/drill registration, rectangular outline, 70 PTH hits, empty NPTH and 16 placement rows remain consistent. The CSV names the custom U2 package at (100,-15) mm, 0 degrees. Crowded controller ground labels and sparse assembly silkscreen remain disclosed limitations; native project/BOM/placement data must be used together.

All five configurations pass all 14 gates in both `integration-09` and `integration-10`. [Checkpoint 04](evidence/integration-checkpoint-04.json), [checkpoint 05](evidence/integration-checkpoint-05.json) and [replay 02](evidence/deterministic-replay-02.json) authenticate 201 repeated deliverable files and all 677 unchanged prior publication files. BMP280 visual review is reused only after exact native/BOM and narrowly timestamp-normalized export equivalence checks. No API calls were made. Full offline regression passed in 40.402 seconds; lint reported zero issues. Remote CI for these local changes remains pending.

### Manufacturer PDF identities

Fetched source PDFs remain local review inputs, not republished vendor documents. SHA-256 identities:

| Document | SHA-256 |
|---|---|
| SHT3x-DIS December 2022 v7 | `095b1853e7f4328f5897c9ca6c392a7dd8b0202eda66b0a2629f9cb840dd496d` |
| SHT handling June 2025 v9 | `80be25deb1b1539668fb7dd6b15285f6e8c8fccd0be95b71c9deb13256231060` |
| Humidity/temperature design guide March 2024 v2 | `b7db78c1e8a80000411c258b9a86b2c588fbf6ee5b315cc05c728f3ab2a20662` |
