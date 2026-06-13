# KiCad CLI ERC/DRC Probe

Probe date: 2026-06-13

Local executable used:

```text
/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```

Local version:

```text
10.0.3
```

The shell `PATH` did not include `kicad-cli`, but the macOS KiCad app bundle
binary existed and was executable. The implementation should therefore keep the
same discovery order used by round-trip validation: explicit CLI path,
environment variable, `PATH`, then platform-specific candidates.

## Shared Behavior

Both schematic ERC and PCB DRC support:

- `--output`
- `--format json`
- `--format report`
- `--units in|mm|mils`
- `--severity-all`
- `--severity-error`
- `--severity-warning`
- `--severity-exclusions`
- `--exit-code-violations`

The default report format is `report`, but the checks implementation should use
`--format json` by default. Text reports can remain a fallback for older KiCad
versions if needed later.

The `--exit-code-violations` flag is important because it allows CI-friendly
behavior where KiCad returns a nonzero exit code when violations exist. The
checks implementation must still parse the report and distinguish "violations
found" from "the tool failed before producing a useful report."

## Schematic ERC

Command:

```text
kicad-cli sch erc [--output OUTPUT_FILE] [--format json|report] [--units in|mm|mils] [--severity-*] [--exit-code-violations] INPUT_FILE
```

Help summary:

```text
Runs the Electrical Rules Check (ERC) on the schematic and creates a report
```

Recommended invocation:

```text
kicad-cli sch erc --format json --severity-all --exit-code-violations --output erc.json INPUT_FILE
```

## PCB DRC

Command:

```text
kicad-cli pcb drc [--output OUTPUT_FILE] [--format json|report] [--all-track-errors] [--schematic-parity] [--units in|mm|mils] [--severity-*] [--exit-code-violations] [--refill-zones] [--save-board] INPUT_FILE
```

Help summary:

```text
Runs the Design Rules Check (DRC) on the PCB and creates a report
```

Recommended invocation:

```text
kicad-cli pcb drc --format json --severity-all --exit-code-violations --output drc.json INPUT_FILE
```

`--refill-zones` is useful for future full-board validation, but the first
implementation should not use `--save-board` because checks must not mutate
source projects.

## Observed Lock Warnings

Some CLI calls printed transient lock-file cleanup warnings while still exiting
successfully. The checks implementation should preserve stderr as command
evidence and should not treat non-empty stderr as failure when the process exit
code and report parsing indicate success.

