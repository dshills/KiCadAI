# Phase 9 Audit

Completed: 2026-07-30

Phase 9 closes the July 29 Fable remediation plan with stronger local static
analysis, focused concurrency testing, direct durability-tool coverage,
repository-local cache hygiene, and compatible platform dependency updates.
The tooling implementation is commit `48cfdcbd` (`Add durability tooling and
static checks`). Dependency maintenance is isolated in commit `f8f45f40`
(`Update Go platform dependencies`).

## Disposition

| Work stream | Disposition | Evidence |
| --- | --- | --- |
| Atomic writes | closed | `internal/atomicfile` directly tests replacement, requested mode, temporary-file cleanup, and preservation of the destination after failed replacement. Existing group-commit tests retain transition, rollback, recovery, locking, and platform replacement coverage. Group recovery now returns lock-release failures instead of silently discarding them. |
| Static analysis | closed | `make lint` runs `go vet` plus golangci-lint with `errcheck` and correctness-focused `staticcheck` families `SA1` through `SA6` and `SA9`. Generated protobuf code is path-excluded. The only general error exclusion is ignored teardown-only `Close` errors in tests; intentional nil-context rejection tests use line-local suppressions. |
| Concurrency | closed | `make race-short` runs `go test -race -short -count=1` over IPC, atomic locking, AI providers, and transactions. The target is local-only and does not add GitHub execution to acceptance. |
| Promotion CLI | closed | `cmd/kicadai-promotion` has direct tests for missing and unknown commands, missing required paths, invalid timeout, failure diagnostics, and nonzero failure exit status through the extracted `execute` boundary. |
| Cache policy | closed | The Makefile derives build, module, and golangci-lint caches from one optional `GO_CACHE_ROOT`, defaulting to ignored `.cache/go`. `docs/development.md` documents the override and local race target. |
| Generated-artifact cleanup | closed | The ignored Mach-O Go test binaries `compositionlowering.test` and `designworkflow.test` were confirmed as generated artifacts before removal. Empty `.tmpdebug` and `internal/tmp_corpus_report` directories and redundant legacy cache roots were removed. The post-validation audit found no test binaries outside the consolidated ignored cache and no legacy cache or temporary paths. Cleanup intentionally has no tracked commit because it changes no source or recorded evidence. |
| Platform dependencies | closed | `go-winio` is updated from `v0.5.2` to `v0.6.2`, `x/sys` from the 2021 pseudo-version to `v0.35.0`, and `x/text` from `v0.26.0` to `v0.28.0`. These are the latest selected versions compatible with the module's Go 1.23 floor: the selected module directives require Go 1.21 or 1.23, while the next `x/sys` and `x/text` releases require Go 1.24. `go mod tidy` also corrected stale `// indirect` labels for `x/sys` and `x/text`, which production packages import directly. |

## Tool versions

- Go `go1.26.5 darwin/arm64`, validating a module that retains `go 1.23.0`;
- golangci-lint `2.12.2`, built with Go `1.26.4`;
- Prism `0.5.0`;
- installed KiCad CLI from `/Applications/KiCad/KiCad.app`.

## Local validation

The following bounded repository matrix passed after both implementation
commits:

```sh
make lint
make race-short
make GO_TEST_FLAGS='-short -count=1' test
make COVER_TEST_FLAGS='-short -count=1' coverage-check
make review-matrix
go test -count=1 \
  ./internal/writercorrectness \
  ./internal/kicadfiles/roundtrip \
  ./internal/atomicfile \
  ./cmd/kicadai-promotion
GOOS=linux GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go test -c \
  -o /tmp/kicadai-ipc-windows.test.exe ./internal/ipc
```

The generated-protobuf-excluded coverage result is `81.6%`, above the enforced
`75.0%` threshold. The Windows IPC command is a compile-only compatibility
check; it does not claim Windows runtime execution. It directly compiles the
Mangos path that selects `go-winio`, while the native IPC suite supplies the
passing runtime evidence. The temporary Windows executable was inspected as a
PE32+ x86-64 binary and removed immediately after validation.

The repository-wide `-short` boundaries are deliberate. Phase 8 records the
separate long frozen electrothermal corpus's bounded routing limitation; Phase
9 does not weaken that fail-closed result or misstate it as a pass.

## Installed-KiCad preservation

The optional installed-KiCad design tier passed after the atomic group error
propagation and dependency updates for:

- `class_a_bjt_line_preamplifier`;
- `class_ab_headphone_driver`;
- `class_ab_headphone_protected`;
- `class_ab_speaker_10w_protected`;
- `esp32_wroom_32e_minimal_pass`;
- `usb_c_i2c_sensor_3v3_protected`; and
- `usb_c_led_indicator_protected`.

The tier requires applicable fixtures to provide clean ERC, strict DRC,
internal and KiCad connectivity, route completion, writer correctness, and zero
normalized round-trip differences.

## Prism review

Prism reviewed the tooling commit repeatedly while its findings were resolved.
The review led to panic-safe lock release, explicit `errors.Join` propagation,
broader correctness-focused staticcheck coverage, a narrowly scoped test-close
exclusion, explicit plan-map initialization, and a fail-closed bisection
invariant. No material tooling finding remains open.

Prism separately reviewed the dependency commit. Its `go-winio` compatibility
concern is dispositioned by the absence of direct HCN use, successful Linux and
Windows repository builds, and successful Windows compilation of the exact IPC
test path. Its concern about removed `// indirect` labels is disproved by
production imports of `golang.org/x/sys/{unix,windows}` and
`golang.org/x/text/unicode/norm`, plus a stable `go mod tidy`. No material
dependency finding remains open.

All required validation was local. Phase 9 did not initiate GitHub Actions or
push repository state.
