# Phase 0 Baseline

Captured: 2026-07-29

- Review baseline: `7c7fd9c0`
- Phase 0 implementation base: `8eeaf5ca`
- Go: `go1.26.5 darwin/arm64`
- KiCad CLI: `10.0.3`
- Prism: `0.5.0`
- Platform: macOS arm64

The review baseline identifies the code assessed by Fable. Phase 0 was completed
after the Phase 1 and Phase 2 commits, so the finding ledger preserves both the
original disposition and the current closing evidence.

## Local validation commands

```sh
go test ./internal/blocks ./internal/kicadfiles/pcb -count=1
go test ./internal/transactions ./internal/repair -count=1
go test ./internal/designworkflow ./internal/routing -count=1
go test ./internal/components ./internal/compositionlowering -count=1
go test ./cmd/kicadai -count=1
go test -short -count=1 -timeout 10m ./...
make lint
```

Installed-KiCad evidence uses:

```text
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints
```

GitHub Actions is intentionally outside this remediation plan’s required
validation ladder.
