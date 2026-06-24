# KiCad CLI Report Fixtures

These fixtures are deterministic ERC/DRC JSON reports used by normal unit
tests. They are intentionally redacted: paths, UUIDs, and timestamps are either
absent or stable.

Default tests parse these files without invoking KiCad. To generate or compare
against local KiCad output, run the opt-in checks integration hook:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
KICADAI_KICAD_FIXTURE_TARGET=/path/to/project-or-board \
go test ./internal/kicadfiles/checks -run TestOptionalKiCadCLIReportFixtureGeneration -count=1
```

Use a generated or disposable project target. KiCad CLI DRC with zone refill can
write board files when requested by higher-level repair flows.
