# Two-family generated examples

These are complete, offline-reviewed output bundles from `integration-10`, copied byte-for-byte with no manual output repair. **Final two-family live-language acceptance is pending.** Open each project in KiCad 10.0.3:

| Project | Capability | Pull-ups / bus clock | Declared total bus capacitance |
|---|---|---|---|
| [SHT31 standard](sht31-standard/board.kicad_pro) | Temperature/humidity | 4.7 kohm / 100 kHz | 50-70 pF |
| [SHT31 fast](sht31-fast/board.kicad_pro) | Temperature/humidity | 2.2 kohm / 400 kHz | 50-100 pF |
| [BMP280 standard](bmp280-standard/board.kicad_pro) | Pressure | 4.7 kohm / 100 kHz | 50-200 pF |
| [BMP280 fast](bmp280-fast/board.kicad_pro) | Pressure | 2.2 kohm / 400 kHz | 50-100 pF |
| [BMP280 low-current pull-ups](bmp280-low_current/board.kicad_pro) | Pressure | 10 kohm / 100 kHz | 50-100 pF |

Each directory contains native project/schematic/PCB files, local libraries, BOM CSV/JSON, configuration and electrical calculations, schematic/PCB SVG previews, validation and raw KiCad reports/logs. `manufacturing/` contains nine Gerber layers plus job metadata, separate plated/nonplated drills and maps, a drill report, placement CSV and a source-bound hash manifest. Placement coordinates are absolute millimeters with native-board Y reversed. Confirm rotations and through-hole handling with the assembler.

All five configurations pass all 14 software gates twice. [Publication identities](../../specs/board-family-v2/evidence/examples-01.json) bind copied files to [checkpoint 05](../../specs/board-family-v2/evidence/integration-checkpoint-05.json). [Replay](../../specs/board-family-v2/evidence/deterministic-replay-02.json) compares 201 deliverables with narrowly defined timestamp normalization. Original reports retain their actual execution paths and raw timestamps; moving a project is not a newly executed validation. Local hashes provide provenance, not an independent signature.

The two SHT31 projects use the reviewed custom mask/paste footprint; their copper, placement and routing are fixed. The BMP280 native/BOM designs remain compatible with the [v1 examples](../board-family-v1/README.md), which are not changed by this publication. Unused original library footprints are retained as provenance; the BOM/placement and native project identify the actual installed footprint.

Use only within the [family restrictions](../../specs/board-family-v2/SHT31.md) and [consolidated electrical/assembly-data review](../../specs/board-family-v2/ELECTRICAL_ASSEMBLY_REVIEW.md), including external regulated 3.3 V, current limiting, radio-off firmware and no extra loads. Hardware performance, ambient accuracy, firmware, stencil/assembly process, fabrication, EMC and independent engineering certification are **not demonstrated**. Sparse silkscreen and crowded controller ground labels remain known limitations; inspect the native project alongside BOM and placement data. Do not send these files directly to production without the separate physical-build review.

To reproduce a bundle offline from the repository root, choose a new output directory:

```sh
go run ./cmd/kicadai-board-family \
  --config examples/board-family-v2/sht31-standard/configuration.json \
  --output /tmp/kicadai-new-sht31-project \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```
