# SHT31 standard prototype engineering review

Review scope: the existing confirmed-specification `esp32_sht31_v1 / standard`
design, plus the new diagnostic firmware and preparation process. Reviewer:
implementing Codex agent, 2026-09-25; not an independent engineer. No physical
measurements, fabricated boards, parts purchases or fabrication authorization.

Disposition: suitable for a **controlled prototype preparation package**, subject
to the release holds below. CAD validation and an engineering review do not qualify
the assembly process or establish ambient sensor accuracy.

## Design review and evidence

The starting configuration is external 3.2-3.4 V, 1 A source capability, 10-35 C,
70 pF maximum total I2C loading. Inherited fixed conditions include 35-60% RH,
noncondensing, radio off, no external GPIO loads, no hot-plug and <=50 mVpp rail
ripple. See [family review](../../../specs/board-family-v2/ELECTRICAL_ASSEMBLY_REVIEW.md)
and [fixed controller design](../../../specs/board-family-v1/FAMILY.md).

| Area | Finding / disposition |
|---|---|
| Supply | No regulator, reverse-polarity, surge or hot-plug protection. External current-limited 3.3 V is mandatory; do not power from a UART adapter. Bench-test droop and ground offset, not just nominal voltage. |
| Reset/download | EN pull-up/RC and RESET, GPIO0 pull-up/BOOT are present; no automatic DTR/RTS circuit. Manual sequence and adapter isolation are specified in BENCH.md. |
| Sensor nets | U2.1 SDA -> GPIO21, U2.4 SCL -> GPIO22; U2.2 ADDR ground selects 0x44; U2.5 VDD with local C5 100 nF; U2.7/U2.8/exposed pad ground; ALERT and nRESET intentionally unconnected. These match the vendor drawing. |
| Pull-ups | Standard R3/R4 are 4.7 kohm, 1%. Previous worst-case review calculates 281.97 ns rise at 70 pF and 0.732 mA sink per line. The 300 ns sensor limit is tighter than the general 100 kHz bus rise limit. Probes count toward 70 pF; measure both endpoints. |
| Controller | Exact ESP32-WROOM-32E-N4/4 MB target. Radio is uninitialized. J3 remains unloaded. No antenna/RF performance claim. |
| Land pattern | Existing project-local U2 footprint retains the reviewed 0.5 mm pitch, offset paste, grouped row mask openings and reduced center paste. Source recipe and native/export checks bind its geometry. See vendor v7 sections 5.2-5.3. No copper or footprint correction was identified in this review. |
| Assembly identification | Silkscreen is sparse and ground labels crowded. Supply pin 1, UART pin 1, sensor pin 1, module orientation and all 16 placement rows must be checked against native project/placement data; never build from a silkscreen screenshot alone. |
| Firmware | Added a pinned cross-buildable diagnostic, explicit external pulls, startup/reset waits, heater-off verification, CRC checking, bounded I/O and fault latching. No sensor library, bus scan or auto-retry hides faults. |
| Thermal/environment | Sensor/controller separation is not thermal isolation. Initial reflow humidity drift and board self-heating remain possible; a successful digital read is not ambient calibration. |

Manufacturer sources reviewed:

- [Sensirion SHT3x-DIS December 2022 v7](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf): pin drawing on page 8, conversion/timing/CRC, package and land pattern on pages 17-18. Datasheet SHA-256 `095b1853e7f4328f5897c9ca6c392a7dd8b0202eda66b0a2629f9cb840dd496d`.
- [Sensirion handling v9](https://sensirion.com/media/documents/6D95AA80/6840311F/HT_Handling_Instructions_SHTxx.pdf): no-clean, contamination avoidance, reflow/handling obligations. Detailed profile remains an assembler signoff, not a generic whole-board recipe.
- [Espressif ESP32-WROOM-32E/32UE datasheet](https://documentation.espressif.com/esp32-wroom-32e_esp32-wroom-32ue_datasheet_en.html) and [IDF 4.4 I2C driver](https://docs.espressif.com/projects/esp-idf/en/v4.4.7/esp32/api-reference/peripherals/i2c.html).

## Release holds (all must be closed before an order)

1. Agent-led engineering review is the accepted review route, as directed by the
   user on 2026-10-08; an independent engineer is not a prerequisite. The user
   must still approve the exact package hash, remaining risks, intended prototype
   quantity, supplier, cost and use before ordering. This review is not that approval.
2. Fabricator accepts nominal 120 x 80 mm Edge.Cuts centerline dimensions, two layers,
   nominal 1.6 mm FR4/35 um copper and the actual minimum features/drills. Confirm
   actual stack/tolerances and surface finish; do not infer dimensions from the
   120.1 x 80.1 mm Gerber job stroke bounding box. There are no mounting holes.
3. Assembler accepts U2's shared mask openings, 0.1 mm outward paste offsets,
   reduced center paste and a sensor-compatible stencil (manufacturer recommends
   0.10 or 0.125 mm). Review the module's exposed-pad vias/solder wicking and any
   plugging/capping requirement. No automatic CAM/paste changes are authorized.
4. Assembler agrees to the component-compatible solder sequence/profile, no washing,
   sensor cavity protection without coating/contamination, and sensor last solder
   cycle. TSW through-hole headers are not presumed lead-free wave-compatible;
   plan a separate approved header process. No hand-soldering assumption for U2.
5. Exact MPNs, current approval sheets, availability, storage/MSL and reel/pin-1
   orientation are checked before purchase. The inherited Murata C2 sheet is old;
   current supplier approval is a hold. No unreviewed substitutions.
6. Assembler converts the absolute-origin, negative-Y placement CSV to its own
   machine convention and verifies pin 1/rotation. Headers are in the CSV but are
   through-hole parts, not automatically pick-and-place operations.

Any design/assembly-data correction must change source, pass regression/native
validation and regenerate the package. Do not edit Gerbers, netlists, placement CSV
or successful manifests. Re-review affected gates and issue a new package identity.

## What the package proves

The package receipt records exactly which source files, native files, firmware
outputs and validation reports were delivered. All 14 CAD gates and native hashes
are checked on a fresh generation. Cross-compilation establishes buildability;
host tests establish only the tested protocol logic. Physical state remains
`not_performed`, and release authorization remains false regardless of test passes.

## Fresh inspection findings

The implementing agent inspected the regenerated schematic, top/bottom Gerber
views and separate U2 copper/mask/paste views against the vendor pin and land
drawings. Native schematic/PCB/project, BOM and configuration match the reviewed
standard example byte-for-byte; the regenerated output passes all 14 native/export
gates. The rectangular outline, U2 pin-1 chamfer/marker, unconnected ALERT/reset
pads, grouped row-mask openings and shifted perimeter paste agree with the reviewed
design. No CAD correction was identified. Existing native checks separately bind
drills and all 16 placement rows; pictures alone do not prove connectivity.

The first derived assembly image had poor gray/yellow contrast. The rendering
recipe was changed to black-and-white and regenerated; this did not change CAD
or manufacturing data. Labels remain crowded on small parts, so zoomable numbered
pad drawings, the BOM/placement table and native project must be used together.

Firmware review corrected RTOS wait rounding with an extra tick and tests verify
the required requested waits. Protocol host tests and the target cross-build pass;
no on-target execution is claimed. Final delivery inspection/hashes are recorded
alongside the retained package, without modifying the generated board bundle.
