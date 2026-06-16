# KiCad Direct File Writers

KiCadAI can generate KiCad project files directly from Go. This path is independent of live KiCad IPC write support and is the current write mechanism for generated projects.

## Generate an LED Project

```sh
go run ./cmd/kicadai \
  --output led_indicator \
  --name led_indicator \
  --seed demo \
  --with-pcb \
  --json \
  generate-led-demo
```

Generated files:

```text
led_indicator/led_indicator.kicad_pro
led_indicator/led_indicator.kicad_sch
led_indicator/led_indicator.kicad_pcb
```

If `--name` is omitted, the CLI derives it from the `--output` directory basename. When both are supplied, the output directory basename and KiCad project basename may differ; generated root files still use `--name`.

For example, `--output generated/demo-output --name sensor_frontend` writes:

```text
generated/demo-output/sensor_frontend.kicad_pro
generated/demo-output/sensor_frontend.kicad_sch
generated/demo-output/sensor_frontend.kicad_pcb
```

Use `--overwrite` to replace an existing generated project directory. Overwrite uses a temporary project directory, backup directory, and recovery journal instead of writing directly into the target.

For an overwrite of `parent/led_indicator`, the recovery journal is written as `parent/.led_indicator.kicadai-journal`. If a swap fails after moving the old project, `WriteResult.BackupDir` and `WriteResult.JournalPath` report the backup and journal locations. Do not start another overwrite while the journal exists; inspect the journal phase, restore the backup directory if needed, then remove the journal.

## Go API

```go
generated, err := design.LEDIndicatorDesign(design.LEDIndicatorInput{
    Name:       "led_indicator",
    DesignID:   kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
    Seed:       "demo",
    IncludePCB: true,
})
if err != nil {
    return err
}

result, err := design.WriteProjectDirectory("led_indicator", generated, design.WriteOptions{})
```

The lower-level writers live under:

```text
internal/kicadfiles/project
internal/kicadfiles/schematic
internal/kicadfiles/pcb
```

They write to an `io.Writer` only. Filesystem writes are centralized in `internal/kicadfiles/design.WriteProjectDirectory`.

## Project Structure Support

`WriteProjectDirectory` builds a complete generated-file inventory before writing. The inventory rejects unsafe relative paths, absolute paths, traversal, duplicate generated paths, case-insensitive collisions, reserved filename components, and file/directory collisions.

Supported emitted files include:

```text
<project>.kicad_pro
<project>.kicad_sch
<project>.kicad_pcb
sym-lib-table
fp-lib-table
child/*.kicad_sch
*.kicad_dru
*.kicad_wks
asset files
```

`.kicad_pcb`, library tables, child schematics, and optional artifacts are emitted only when represented by the design model. Project-local `.kicad_sym` libraries and `.pretty/*.kicad_mod` footprint modules are not generated yet; the current support is table emission and reference validation.

Generated `.kicad_pro` files use a KiCad-shaped JSON structure with sections for board settings, ERC, libraries, net settings, PCBNew, schematic settings, sheets, text variables, and time-domain parameters. The project JSON `meta.version` is the KiCad project JSON schema version.

## Library Tables

Project-local symbol and footprint library tables are represented by `design.SymbolTables` and `design.FootprintTables`. When entries are present, the writer emits:

```text
sym-lib-table
fp-lib-table
```

Schematic symbol library identifiers are validated against embedded symbols, symbol table nicknames, or declared external library nicknames. PCB footprint library identifiers are validated against inline footprint content, footprint table nicknames, or declared external library nicknames.

## Hierarchical Sheets

The root schematic can contain `schematic.Sheet` references. Child schematic files are represented by `design.SheetFiles` and are written at their project-relative `Filename` paths.

The writer validates that:

- Every sheet reference resolves to an emitted child schematic.
- Distinct child schematics do not claim the same filename.
- Multiple sheet instances may reuse the same child file.
- Circular sheet references are rejected.
- UUIDs are unique across root and child schematic objects represented by the current model.

## Optional Artifacts

The design model can include:

- `RuleFiles` for `.kicad_dru`
- `WorksheetFiles` for `.kicad_wks`
- `AssetFiles` for additional project-local assets

Rule and worksheet extensions are validated. The writer can emit multiple files if the design model asks for them, but project settings do not yet mark a single emitted `.kicad_dru` or `.kicad_wks` file as active. Asset contents are written byte-for-byte through the generated-file inventory.

## Validation

Default validation is deterministic and does not require KiCad:

```sh
make test
make coverage-check
```

Optional KiCad CLI validation is gated behind the `integration` build tag:

```sh
KICAD_VALIDATE_GENERATED_FILES=1 \
KICAD_CLI=/path/to/kicad-cli \
go test -tags=integration ./internal/kicadfiles/design
```

The integration test generates an LED project, exports a schematic netlist, and runs PCB DRC with `--exit-code-violations`.

## Current Limits

- Existing KiCad files are not parsed or edited losslessly.
- Generated footprints cover the initial LED fixture and core primitives.
  Imported-project transaction apply can also hydrate footprints from resolver
  records, including pads, text, courtyard/fab/silkscreen graphics, attributes,
  metadata properties, and model references. New-project builder hydration,
  advanced KiCad footprint nodes, and full pad-stack fidelity remain future
  work.
- Local `.kicad_sym` and `.kicad_mod` library content generation is not complete; table emission and reference validation are implemented.
- Hierarchical sheet support emits represented files but does not parse arbitrary existing sheet trees or fully manage instance-specific annotation for reused child sheets. For reused child sheets, users should run KiCad's annotation workflow after generation before treating references as final.
- Autorouting is out of scope.
- Live IPC mutation remains separate and currently blocked by KiCad API write-command availability.
