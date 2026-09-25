# SHT31 controlled-prototype preparation result

Date: 2026-09-25. Implementing-agent review, not independent engineering
certification. Base: confirmed-specification commit
`c6e5148ef57f63cd235a7054e459be3d5f210c65`.

**Preparation delivered; hardware release remains on hold.** No order, assembly,
flashing, physical testing, API spending or firewall change was performed.

## Deliverables

- [Engineering review and release holds](../../docs/prototypes/sht31/REVIEW.md).
- [Bench procedure](../../docs/prototypes/sht31/BENCH.md) and [blank measurement record](../../docs/prototypes/sht31/measurement-record.json).
- [Pinned, cross-built firmware and instructions](../../firmware/sht31-bringup/README.md).
- [Package rebuild/integrity instructions](../../docs/prototypes/sht31/PACKAGE.md).
- Complete local archive: `.cache/sht31-prototype-01/sht31-prototype-reviewed-final.zip`
  within the isolated `sht31-prototype-01` worktree. The ZIP contains `package-03/`.
  This archive is retained locally, not uploaded to a vendor or GitHub.

## Evidence and exact identities

| Artifact | SHA-256 |
|---|---|
| ZIP | `94626d308481f76b7f52ba01bfcd772ab5716971a6f9b7412d107b4f7a0de20e` |
| Package manifest | `b61d93ee80d322784f61a613c00b7b9cee083d81986b7d600d139503812b8c6f` |
| Application firmware.bin | `3d9a1cee1bfad2f6c01de27c44c8327ae3e4911609d5055d3f756a1f770dd181` |
| Native board.kicad_pcb | `88087370d8338853473256fc0965249eeabdb61f246dd4bfbb83a9767c7038ea` |

The package verifier authenticated all 134 listed files and the exact inventory;
the manifest is excluded from its own hash list. ZIP CRC testing passed. Native
schematic/PCB/project, BOM and configuration are byte-identical to the reviewed
SHT31 standard example. Native validation passed 14/14 gates with KiCad 10.0.3
in 4.913 seconds on this host; this timing is not a performance guarantee.

The firmware was clean-built with PlatformIO 6.1.19, espressif32 7.0.1,
Arduino-ESP32 package 3.20017.241212+sha.dcc1105b and Xtensa GCC
8.4.0+2021r2-patch5. Link report: 22,748 bytes RAM, 278,901 bytes program.
Application binary file is 279,264 bytes (image overhead included). Esptool
4.11.0 validated application and bootloader image checksums/hashes. Decoding
the partition binary confirmed NVS at 0x9000/24 KiB and factory app at
0x10000/1280 KiB, with no OTA-data partition.

Host C++ tests passed with address/undefined-behavior sanitizers: manufacturer
CRC vector, all 48 single-bit data/CRC faults, endpoint conversions, command
sequence and waits, all ten initialization/measurement I/O failure points,
heater/reset/command/checksum flags, and latched failures. Nineteen Node test
cases passed for UART-log consistency and package tamper/extra/missing/symlink
negative controls. All UART test samples are synthetic, not physical evidence.
Both existing Go package suites passed (`internal/boardfamily`: 56.631 s;
`cmd/kicadai-board-family`: 47.070 s). Attempting to rebuild into package-02
was rejected before work began; its 134-file verification still passed afterward.

Final visual review included regenerated schematic/PCB, top/bottom Gerber
overviews, U2 copper/mask/paste and the revised black-and-white assembly view.
Sensor pin/land drawings were checked visually against Sensirion v7 pages 8,
17 and 18. No new circuit or land-pattern defect was identified. Crowded labels
and sparse silkscreen remain disclosed; use native pad numbers and placement/BOM
data together. No native or manufacturing file was manually repaired.

## Changes and remaining boundaries

New firmware, scripts, documentation and host CI checks only; historical evaluation
files and generator reference geometry are unchanged. A preliminary package-01
was retained. RTOS wait rounding and assembly-view contrast were corrected in
source before package-02 was regenerated. A whitespace-only cleanup then produced
final package-03; firmware binary and all eight inspection PNGs match package-02
exactly. Both preliminary packages remain retained, not relabeled as final.
The package's generic CAD report still
states that the board generator does not deliver firmware; the separately bound
diagnostic firmware extends this prototype package only, not the generic catalog.

Remote CI for this new branch has not run; prior PR #14's green checks are not
claimed for these new changes. The added workflow runs offline host tests only,
not flashing or physical tests. Target firmware was cross-built locally.

Before any order: named human/package approval, fabricator stack/feature review,
assembler stencil/pad/via/solder-process agreement, current component approval
sheets and exact part/orientation checks remain mandatory. Before any performance
claim: authorized hardware work and retained measurements per the bench procedure.
