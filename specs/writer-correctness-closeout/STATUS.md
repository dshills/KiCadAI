# Writer Correctness Closeout Status

Date: 2026-06-17

## Summary

The project already has strong writer and validator foundations, but they are
spread across several packages. There is no single generated-project writer
correctness gate that proves schematic connectivity, schematic-to-PCB net
transfer, PCB pad net assignment, copper net assignment, zone assignment, and
optional KiCad round-trip stability in one result.

The implementation should reuse existing packages and add the missing
orchestration, snapshots, CLI surface, design workflow integration, and golden
fixtures.

## Reusable Packages

### Project And File Discovery

- `internal/inspect` discovers project files and summarizes schematic/PCB
  contents.
- `internal/kicadfiles/project` reads and writes `.kicad_pro`.
- `internal/kicadfiles/library` parses and writes local library tables.
- `internal/manifest` writes generated-project manifests.

### Schematic Writer And Reader

- `internal/kicadfiles/schematic` writes and reads KiCad schematic files.
- `internal/schematic` validates internal schematic-domain operations.
- `internal/transactions` validates and applies schematic/PCB/project
  transactions.
- `internal/schematicpcb` extracts schematic components and generates PCB
  placement transactions.

Existing gap:

- There is no dedicated schematic connectivity snapshot for generated files.
- Existing schematic checks are mostly structural and transfer-oriented.

### PCB Writer And Reader

- `internal/kicadfiles/pcb` models, renders, reads, and validates PCB files.
- `internal/kicadfiles/pcb.Validate` performs structural checks.
- `internal/kicadfiles/pcb.ValidateGeneratedConnectivity` checks generated
  same-net connectivity and dangling route endpoints.
- `internal/kicadfiles/pcb` includes object correctness fixtures and tests.

Existing gap:

- PCB validation exists, but the checks are not wrapped in a generated-project
  writer gate with schematic transfer expectations.

### Board Validation

- `internal/boardvalidation` combines project target resolution, structural PCB
  validation, generated connectivity validation, route completion, zone checks,
  and optional KiCad DRC evidence.

Existing gap:

- Board validation answers whether a board is electrically meaningful. Writer
  correctness also needs to answer whether the writer faithfully emitted the
  intended generated design.

### KiCad CLI And Round Trip

- `internal/kicadfiles/checks` runs and parses KiCad ERC/DRC checks.
- `internal/kicadfiles/roundtrip` runs KiCad-backed round-trip checks.
- `cmd/kicadai` exposes `check` and `roundtrip` commands.

Existing gap:

- KiCad evidence is optional and command-specific. Writer correctness needs a
  stable optional check that skips cleanly unless required.
- Round-trip evidence must avoid deleted or missing working directories.

### Footprints, Placement, And Routing

- `internal/libraryresolver` parses symbol and footprint libraries.
- `internal/placement` places footprints and emits operations.
- `internal/routing` routes small deterministic boards and validates route
  results.
- `internal/routingadapters` converts placement output into routing requests.

Existing gap:

- Placement/routing validators are not yet summarized as writer-stage issues
  after project write.

### AI Design Workflow

- `internal/designworkflow` orchestrates explicit block composition, schematic
  generation, PCB realization, placement, routing, validation, KiCad checks, and
  project write.
- `cmd/kicadai design create` exposes the workflow.

Existing gap:

- The workflow does not yet run a dedicated writer correctness gate after write.

## Implemented Checks To Reuse

- Project inspection and missing-file reporting.
- Project read/write validation.
- Schematic read/write round-trip tests.
- Schematic-to-PCB component and footprint extraction.
- Pinmap validation.
- PCB structural validation.
- PCB generated connectivity validation.
- Board validation result model and CLI.
- Route result validation.
- KiCad CLI check parsing.
- Round-trip artifact handling.

## Missing Checks

Writer correctness still needs:

- a result model with writer-specific check names;
- project target discovery suitable for generated project directories;
- local `fp-lib-table` and `sym-lib-table` path checks;
- schematic parse/connectivity snapshot;
- PCB net table snapshot;
- footprint/pad net snapshot;
- copper net snapshot;
- zone net snapshot;
- semantic snapshot comparison for optional KiCad round trip;
- CLI command for writer checks;
- design workflow stage integration;
- golden good and known-bad writer fixtures.

## Candidate Golden Good Fixtures

- `examples/01_led_indicator`
- `examples/03_rc_filter`
- `examples/07_generated_pcb`
- `examples/08_pcb_object_correctness`
- `examples/blocks/led_indicator`
- `examples/blocks/connector_breakout`
- generated `examples/design/led_indicator.json`

## Candidate Known-Bad Fixtures

Create focused fixtures during implementation for:

- missing project file;
- broken library table path;
- schematic parse failure;
- missing schematic footprint assignment;
- duplicate schematic reference;
- missing PCB net table entry;
- duplicate PCB net code;
- pad assigned to a missing net;
- track assigned to a missing net;
- via assigned to a missing net;
- zone assigned to a missing net;
- dangling copper endpoint.

## KiCad CLI Working Directory Constraint

Previous failures included:

```text
Failed to get the working directory (error 2: No such file or directory)
```

Writer correctness should not run KiCad commands from a directory that may be
deleted while the command is active. Optional KiCad evidence should use a stable
project directory or retained artifact workspace, and tests should use fake
KiCad runners unless explicitly gated for real KiCad.

## Implementation Direction

The next phase should add `internal/writercorrectness` with a small result
model and deterministic issue handling. Later phases should add checks
incrementally and call existing validators instead of duplicating validation
logic.
