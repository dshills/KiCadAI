# Bounded board sizes and connector placement

Both BMP280 and SHT31 families accept an optional structured layout in the
confirmed-specification workflow. The circuit, parts, pin assignments and local
controller/sensor/support placement groups stay fixed. The generator places
connectors from mechanical constraints and routes their connections around
retained pads and copper. Every build still requires all 14 native gates before
it receives manufacturing exports and a successful bundle manifest.

There are two board sizes, 120 x 70 mm and 140 x 80 mm. Each connector has two
choices, giving 16 mechanical combinations per family. All existing electrical
profiles are available: three BMP280 and two SHT31 profiles, for 80 configurations.
The existing 120 x 80 mm reference remains the default when layout is omitted.

| Connector | Inboard position, mm | Edge position, mm |
|---|---|---|
| J1 power | 20, 22.5 | west: 5, 22.5 |
| J2 UART | 65, 50 | south: 65, board height minus 10 |
| J3 GPIO | 78.02, 32 | east: board width minus 5, 32 |

Positions are footprint origins in native board coordinates; pin numbering and
rotation are unchanged. Sizes are Edge.Cuts centerline dimensions. Headers remain
through-hole parts. Stack, parts, operating envelopes and assembly restrictions
from the family catalog still apply.

## Generate a bundle

List the closed choices with `kicadai-board-family spec layouts`. For example:

```sh
go build -o ./kicadai-board-family ./cmd/kicadai-board-family
./kicadai-board-family spec new \
  --config examples/board-family-v2/sht31-standard/configuration.json \
  --layout examples/board-layouts/edge-120x70.json --output draft.json
./kicadai-board-family spec review --input draft.json
./kicadai-board-family spec confirm --input draft.json \
  --accept-sha256 COPY_THE_REVIEW_SHA256_HERE --output confirmed.json
./kicadai-board-family spec build --input confirmed.json \
  --output generated-board --kicad-cli /path/to/kicad-cli
```

Use `edge-140x80.json` for the larger board, or edit individual connector fields
to `inboard` before review. Review displays the selected dimensions, actual
positions and limits; its fingerprint binds the layout. Size or position changes
invalidate confirmation. Unknown sizes, locations, versions, extra fields and
duplicate JSON keys are rejected. No AI request or API key is required.

The bundle includes native KiCad files, local libraries, BOM, previews,
`layout.json`, `layout-report.json`, electrical and validation reports, Gerbers,
drills, placement CSV, confirmation receipt and an exact file-hash manifest.
The layout report records newly routed pads, segments, vias, obsolete anchor
segments retired during generation and search work. Former PTH pad locations
become vias only when needed to connect copper on both layers. Redundant escape
segments and obsolete open stubs are removed by the source generator.

Routing uses a 0.25 mm grid, 0.2 mm clearance, 0.25 mm signal traces and 1 mm J1
power/return extensions, with a 150,000-node limit per branch and a 45-second
overall routing deadline. Strict KiCad DRC, connectivity, parity and round trips
then check the emitted board. A blocked route or failed check returns an error;
no successful bundle manifest is written. Correct source or the specification and
regenerate into a new directory. Output files are never manually repaired.

## Verification and limits

Offline tests reproduce all 32 family/mechanical combinations, bind changed
layouts to confirmation, and inject routing/physical-check failures. Run the
complete 80-case installed-KiCad matrix and the full-bundle tests with:

```sh
KICADAI_OFFLINE_NATIVE_CLI=/path/to/kicad-cli \
  go test ./internal/boardfamily ./cmd/kicadai-board-family \
  -run '^TestLayout' -count=1 -parallel 4 -timeout 12m
```

Generation is deterministic; native exporter timestamps and measured validation
durations can differ between builds. Compare the native board, schematic, project,
BOM and layout report by hash. Each delivered manufacturing inventory retains
its own raw-file hashes.

These are bounded connector-layout variants. They do not add circuits, rotate
arbitrary parts or move sensitive local groups. Total I2C capacitance remains an
explicit declared bound including the new traces, headers and probes; it is not
extracted or measured. Power integrity, assembly yield and sensor performance
still require hardware measurements. The earlier prototype package applies only
to its original board hash and does not qualify these new geometries physically.
