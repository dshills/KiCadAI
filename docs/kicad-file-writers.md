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

If `--name` is omitted, the CLI derives it from the `--output` directory basename. When both are supplied, the output directory basename must match `--name`. This keeps the generated directory and KiCad project basename aligned.

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
- Generated footprints cover the initial LED fixture and core primitives; broader library-quality footprint generation is future work.
- Autorouting is out of scope.
- Live IPC mutation remains separate and currently blocked by KiCad API write-command availability.
