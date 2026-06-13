# ERC/DRC Checks Package

`internal/kicadfiles/checks` runs KiCad CLI ERC/DRC checks and converts KiCad
JSON reports into stable Go structures for CLI output and AI repair loops.

## Normal Tests

Normal tests do not require KiCad:

```sh
GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/checks
```

They cover:

- result models and stable finding IDs;
- artifact workspace cleanup and path safety;
- JSON report parsing;
- allowlist filtering;
- fake-runner ERC/DRC execution;
- deterministic summary output.

## Real KiCad Smoke Checks

Real KiCad checks can be run from the CLI:

```sh
go run ./cmd/kicadai \
  --json \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  --keep-artifacts \
  --artifact-dir ./examples/check_artifacts \
  check erc ./examples/checks/erc_fail/erc_fail.kicad_sch
```

DRC execution is wired through the same package, but the local KiCad 10.0.3 CLI
probe exited before writing a DRC JSON report for the current generated PCB
examples. The DRC parser and runner paths are covered with deterministic JSON
samples and fake-runner tests until a stable real DRC fixture is available.

## Result Semantics

- `pass`: KiCad ran, the report parsed, and no remaining findings exist.
- `fail`: KiCad ran, the report parsed, and findings remain after allowlisting.
- `skipped`: KiCad is unavailable and the caller chose skip semantics.
- `error`: KiCad failed before producing a parseable report, or report parsing
  failed.

When `--exit-code-violations` makes KiCad return exit code `1`, the report file
is still parsed. A parseable report with findings is a validation failure, not a
tool failure.

