# KiCad 10 File Writer Fixtures

This directory records the fixture baseline for direct KiCad file writers.

## Current Baseline

No KiCad-saved fixture files are checked in yet. Implementation begins from
the documented KiCad file formats and synthetic golden outputs. When a tiny
project is saved from KiCad 10, add it here for comparison:

```text
minimal/minimal.kicad_pro
minimal/minimal.kicad_sch
minimal/minimal.kicad_pcb
led_indicator/led_indicator.kicad_pro
led_indicator/led_indicator.kicad_sch
led_indicator/led_indicator.kicad_pcb
```

## Required Fixture Notes

When adding fixtures, record:

- KiCad version.
- Operating system.
- Whether the file was created by KiCad GUI or KiCad CLI.
- Any manual edits made after saving.

Generated writer golden files must remain separate from KiCad-saved fixtures
so tests can distinguish expected writer output from observed KiCad canonical
output.
