# SHT31 controlled-prototype package

**HOLD: not authorized for ordering, assembly or physical testing.**
Software verification is complete only if `package-manifest.json` exists and
passes the included verifier. Physical validation is `not_performed`.
Paths below are relative to the delivered package root.

## Contents

- `board/`: complete generated KiCad project, schematic/PCB, local libraries,
  BOM CSV/JSON, electrical report, all 14 validation gates and board manifest.
- `board/manufacturing/`: Gerber layers/job, plated/nonplated drill files/maps,
  placement CSV and source-bound export manifest. Headers are through-hole even
  though included in placement data. No automatic machine-convention conversion.
- `firmware/`: cross-compiled application, bootloader, partition table, ELF/map
  and explicit flash-offset record. Nothing has been uploaded to hardware.
- `inspection/`: derived top/bottom, sensor copper/mask/paste, schematic, PCB
  and numbered top-assembly views, plus renderer logs. These are review aids,
  never replacements for native files or manufacturing inputs.
- `source/`: exact firmware, packaging/verifier scripts, engineering review,
  bench procedure and blank measurement record used for this package.
- `verification/`: host-test, log-verifier, firmware-build and CAD validation logs.
- `confirmation.json`: the agent-reviewed standard design confirmation used
  for this preparation, not a fabrication authorization or human signature.

Review `source/docs/prototypes/sht31/REVIEW.md` first. It lists required human,
fabricator, assembler and component-approval holds. Follow
`source/docs/prototypes/sht31/BENCH.md` only after authorization. Flash instructions
are in `source/firmware/sht31-bringup/README.md`.

The unchanged CAD catalog says no firmware is delivered by the generic board
generator. This package adds a **separate, standard-profile diagnostic firmware**;
it does not change the generic catalog, earlier evaluations or other profiles.

## Check integrity before use

With Node.js installed, from this directory:

```sh
node source/scripts/sht31-prototype/verify-package.mjs .
```

Compare the externally recorded SHA-256 of the ZIP or package manifest as well.
A self-contained manifest detects changed bytes against that manifest; it is
not a cryptographic signer identity or proof that an untrusted manifest is genuine.
Do not open/save CAD files in the sole retained evidence copy: work on a duplicate,
because editor preferences or timestamps may change. Never repair output artifacts.

## Rebuild from the repository

Use the confirmed-specification CLI built from this branch; review/confirm the
standard configuration using `docs/confirmed-specification.md`. The receipt must
contain the exact SHT31 standard example configuration. Install the pinned firmware
dependencies, KiCad 10.0.3, Gerbonara 1.6.3 and librsvg (`rsvg-convert`). From the
repository root, supply executable paths and a NEW output directory:

```sh
node scripts/sht31-prototype/package.mjs \
  /path/to/kicadai-board-family /path/to/confirmed.json \
  /path/to/kicad-cli /path/to/pio /path/to/new-package \
  /path/to/gerbonara /path/to/rsvg-convert
```

The builder runs sanitized child environments, host tests, a clean target firmware
build, fresh CAD generation/validation, native-reference comparison, inspection
renders and complete file hashing. A failure retains diagnostics and does not
produce a successful manifest. No ordering/flashing/API commands are included.
A newly rendered package still requires visual inspection and release review.
Tool timestamps/build metadata may differ across rebuilds; do not claim byte-for-byte
firmware reproducibility across machines. Exact delivered bytes are hashed.
