# ERC/DRC Check Fixtures

These fixtures support the `kicadai check` command family.

## Fixtures

- `erc_fail/erc_fail.kicad_sch`
  - Representative schematic expected to produce ERC findings under KiCad 10.
  - Based on the generated LED indicator example.
- `drc_pass/drc_pass.kicad_pcb`
  - Historical-path generated PCB for DRC runner and parser smoke testing.
  - A current KiCad 10.0.3 run writes a report with four warning findings: two
    library-footprint mismatches and two dangling vias. The fixture proves
    report ingestion, not a clean DRC pass; its directory name is retained for
    compatibility.
- `erc_pass/`
  - Reserved for a known-clean schematic once the generator can produce one with
    complete power/no-connect annotations.
- `drc_fail/`
  - Reserved for a purpose-built blocking DRC-violation board. The existing
    `drc_pass` historical fixture already exercises warning finding parsing.

Normal `go test ./...` does not invoke KiCad. Real CLI checks are opt-in:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
GOCACHE="$(mktemp -d)" \
go test ./internal/kicadfiles/checks
```
