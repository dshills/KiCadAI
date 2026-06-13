# ERC/DRC Check Fixtures

These fixtures support the `kicadai check` command family.

## Fixtures

- `erc_fail/erc_fail.kicad_sch`
  - Representative schematic expected to produce ERC findings under KiCad 10.
  - Based on the generated LED indicator example.
- `drc_pass/drc_pass.kicad_pcb`
  - Representative generated PCB for DRC smoke testing.
  - Current local KiCad 10.0.3 DRC probing exited before writing a report for
    existing PCB examples, so this fixture is kept as a future integration target
    rather than a normal test dependency.
- `erc_pass/`
  - Reserved for a known-clean schematic once the generator can produce one with
    complete power/no-connect annotations.
- `drc_fail/`
  - Reserved for an intentional DRC-violation board once DRC execution is stable
    in local KiCad CLI.

Normal `go test ./...` does not invoke KiCad. Real CLI checks are opt-in:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
GOCACHE="$(mktemp -d)" \
go test ./internal/kicadfiles/checks
```
