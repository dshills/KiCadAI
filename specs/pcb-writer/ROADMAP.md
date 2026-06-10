# KiCad Writer Roadmap

This roadmap captures the next projects needed to move the KiCad file writer from
"KiCad can open it" to "KiCad treats it like a native project."

## 1. Round-Trip Validation

Generate KiCad files, run them through KiCad's own CLI save/upgrade path, and
compare the resulting files against our originals.

The goal is to discover every structural difference KiCad rewrites so the writer
can emit native-looking files before KiCad touches them.

Implementation workflow: [README.md](../../internal/kicadfiles/roundtrip/README.md).

## 2. Golden Corpus Tests

Use real KiCad demo projects as fixtures for project, schematic, and PCB file
structure.

The goal is to assert that our writer includes the same required sections,
ordering, metadata, layer tables, setup blocks, project sidecar files, and object
shapes used by KiCad-generated designs.

Initial PCB corpus coverage lives in `internal/kicadfiles/pcb`. The normal unit
tests use synthetic fixtures, and the external KiCad demo corpus can be scanned
locally with:

```sh
KICADAI_RUN_KICAD_DEMO_CORPUS=1 \
KICADAI_KICAD_DEMO_CORPUS="/Users/dshills/Documents/KiCad Demos" \
go test ./internal/kicadfiles/pcb -run TestScanCorpusExternalKiCadDemos
```

## 3. PCB Object Correctness

Tighten the writer for all PCB object types:

- Footprints
- Pads
- Tracks
- Vias
- Zones
- Arcs
- Board graphics
- Text
- Groups
- Dimensions
- Properties

The goal is not only valid syntax, but objects that KiCad can edit, DRC, plot,
and preserve without unexpected rewrites.

## 4. Connectivity and DRC

Generate small known-good boards with intentional nets, run KiCad validation
where available, and fail tests on disconnected pads, invalid net assignments,
missing outlines, clearance issues, or zone problems.

The goal is to prove generated designs are electrically meaningful, not merely
parseable.

## 5. Symbol and Footprint Library Mapping

Build reliable symbol-to-footprint assignment and project-local library table
support.

The goal is for schematic-generated designs to naturally produce valid PCB
footprints, stable references, usable netlists, and KiCad-resolvable library
links.

## 6. Round-Trip Preservation

Preserve unknown nodes, unsupported settings, ordering-sensitive sections, and
user-authored KiCad content when reading and rewriting files.

The goal is to avoid damaging projects that contain KiCad features our writer
does not fully model yet.

## 7. Higher-Level Design API

Expose a Go API above the raw file writer for AI-assisted design generation.

Expected operations include:

- `AddSymbol`
- `Connect`
- `AssignFootprint`
- `PlaceFootprint`
- `Route`
- `AddZone`
- `WriteProject`

The goal is to let agents build schematics and PCBs from design intent while the
lower-level writer guarantees KiCad-native output.
