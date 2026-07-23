# External Review Mitigation Completion Audit

Audit date: 2026-07-22

Review source: [`../FEEDBACK.md`](../FEEDBACK.md)

Matrix: [`../../testdata/external-review-mitigation/matrix.json`](../../testdata/external-review-mitigation/matrix.json)

## Outcome

All six findings from the independent review are mitigated by generic behavior
and are bound into the release regression ladder. Phase 7 is commit
`7689f2d578e3e88f7340e668ee65edd35f61246d` (`Promote external review
regression ladder`). Its required GitHub Actions run
[`29971550896`](https://github.com/dshills/KiCadAI/actions/runs/29971550896)
completed successfully for that exact head SHA.

| Finding | Disposition | Generic correction and evidence |
|---|---|---|
| F1 regulator composition placement | Fixed | Translatable fixed groups are placed atomically before individual fixed-coordinate rejection. The AP2112K intent fixture completes placement, routing, write, and validation on its declared 45 mm by 30 mm board. |
| F2 unrelated stock-library blockers | Fixed | Indexed diagnostics are scoped to the resolved design closure; explicit whole-library audit remains available and referenced malformed objects still block. |
| F3 compact LED density | Fixed | Rigid-group candidate generation translates the group envelope while preserving authored offsets and edge constraints. The 40 mm by 25 mm request is unchanged. |
| F4 JSON transport and volume | Fixed | JSON mode emits one bounded document on stdout for success and failure; diagnostic detail remains in durable evidence. |
| F5 function-level discoverability | Fixed | `circuit --help`, capability output, the public function-level example, and exact workflow documentation expose the supported vocabulary and request shape. |
| F6 evidence asymmetry | Fixed | Provider, intent, and circuit creation share six typed, versioned, hash-indexed core artifacts with atomic replacement semantics. |

No production code recognizes a matrix path, scenario ID, coordinate, board
size, block name, net name, or fixture hash. Scenario-specific identities occur
only in test data and test bindings.

## Phase Commits

| Phase | Commit | Subject |
|---|---|---|
| Specification | `77f72cd2` | Document external review mitigation |
| 0 | `cbdb0c48` | Add external review regression coverage |
| 1 | `31666198` | Place translatable groups atomically |
| 2 | `fa518f89` | Promote composed intent placement |
| 2 CI stabilization | `b48a9c91` | Stabilize BMP280 CI timeout |
| 3 | `2b05c215` | Scope library blockers to design closure |
| 4 | `0bf439e0` | Stabilize circuit CLI diagnostics |
| 5 documentation | `40c06286` | Publish function-level circuit workflow |
| 5 compatibility | `59502dea` | Restore frozen function vocabulary compatibility |
| 6 | `8bc81e7b` | Unify creation evidence artifacts |
| 7 | `7689f2d5` | Promote external review regression ladder |

## Reproducible Commands

Default offline gates:

```sh
make review-matrix
make GO_TEST_FLAGS=-short test
make COVER_TEST_FLAGS=-short coverage-check
make lint
```

The matrix target executes every bound test twice (`-count=2`). The scenario
tests additionally compare normalized workflows, projects, or evidence where
applicable. The matrix manifest test requires all six core artifacts and every
internal and optional KiCad gate for each positive scenario. It also requires
these negative cases:

- infeasible rigid-group fit;
- referenced malformed library object;
- invalid function parameter;
- artifact write failure preserving prior evidence.

Installed-KiCad commands and the stock-library quickstart are published in
[`../../docs/external-review-regression.md`](../../docs/external-review-regression.md).
The promotion corpus requires clean ERC, strict DRC, connectivity, required
route completion, writer correctness, and zero normalized round-trip diffs.

## Verification Results

Local default evidence completed successfully:

- `make review-matrix`: all six scenario groups and four negative cases passed
  twice;
- `make GO_TEST_FLAGS=-count=1 test`: the complete unshortened repository suite
  passed, including `internal/compositionlowering` in 666.132 seconds;
- `make lint`: `gofmt`, `go vet`, and configured golangci-lint checks passed;
- no `known F1` through `known F6` skips remain;
- the final worktree was clean before and after the Phase 7 push.

Installed KiCad 10.0.3 evidence completed successfully for:

- the unchanged 40 mm by 25 mm compact LED intent fixture;
- the AP2112K regulator intent fixture;
- the stock-library RC quickstart using the uncurated KiCad application roots;
- the public function-level status-driver example;
- `usb_c_i2c_sensor_3v3_protected`;
- the Class A and Class AB amplifier fixtures;
- all four power/interface synthesis fixtures: buffered ADC acquisition,
  Class AB power interface, protected power MOSFET load, and regulated MCU
  sensor subsystem.

Applicable promotion reports recorded clean ERC, strict DRC with zero real
findings, connectivity success, required-route completion, writer correctness,
and zero normalized round-trip differences. The power/interface rerun also
confirmed the generic ADC interface-conditioning operations after adding their
two generated usage names to the sorted private compatibility vocabulary.

Prism 0.5.0 reviewed the final staged Phase 7 change using
`gemini/gemini-3-flash-preview`. The final review reported zero high and zero
medium findings. Its remaining low notes were audited as non-blocking: the
Makefile already declares `GO_TEST_TIMEOUT ?= 20m`, and the matrix assertion
intentionally aggregates missing gates for complete diagnostics.

GitHub Actions run
[`29971550896`](https://github.com/dshills/KiCadAI/actions/runs/29971550896)
passed all five jobs:

- offline quality gates, including formatting, golangci-lint, bounded tests,
  and the coverage floor;
- open-set promotion corpus;
- adversarial multi-function promotion corpus;
- simulation-grounded promotion corpus;
- external-review regression ladder, executing the matrix twice from a clean
  checkout.

## Tools And Library Roots

- Go: `go1.26.5 darwin/arm64`.
- KiCad CLI: `10.0.3`.
- KiCad CLI: `/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli`.
- Stock symbols: `/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols`.
- Stock footprints: `/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints`.
- Prism: `0.5.0`; required command: `prism review staged`; final reviewer:
  `gemini/gemini-3-flash-preview`.

## Fixture Hashes

These hashes bind the audited request inputs; generated artifact hashes remain
in each run's `.kicadai/manifest.json`.

| Fixture | SHA-256 |
|---|---|
| compact LED | `786a5487b8014b760ad67b67708bc8874c54a5939607884913c12eb703ced6be` |
| AP2112K sensor | `8108ce524647a8d67563978c87f6c3d9a76708f684ca6aa387e2fc51bc1f33eb` |
| stock RC filter | `8fa204b4ca02ba36a0a92777a0b350d1ffdd03c2874109f8d050acd05ab51a0f` |
| multi-unit active filter equivalent | `90e5d6c62cd5f1b97e5251339543ff771b34808a0a426d0743c1bb12ec65d026` |
| split-supply amplifier equivalent | `d1f76fba8b800a4d6eefa6b053a1f4f3333f27c74e027082aff823dfbfc575a1` |
| public function-level example | `70071cdc4f72fa75d808cdfeff235fe9a147455dd60b10bb4af1e2dd6112e27d` |

One stock-library RC promotion run produced this shared core evidence set. The
manifest itself verifies the first five hashes; its own hash is recorded here.

| Core artifact | SHA-256 |
|---|---|
| `design-request.json` | `4a2d64ea7626a4e14dfb0f4252cb7e65a91cc769e4b8ccb99d8a8872c53cc632` |
| `transaction.json` | `97f4d850713a8a87126c0e69bb496ea439b0bc1ab1c6fdb0ea8c01e7a4e427fa` |
| `workflow-result.json` | `cf57b61f7244f5b20974bc8c98017c14808cadc248a00876ee4e5c2d5dfe59e2` |
| `validation-summary.json` | `e26b57c18ad273fa646909a9fdbdb93d0b52d932f4e639d1d5b9d78fd5d2e137` |
| `design-promotion.json` | `afaebd3825ec16669ab4084c07a234f5122c95caeb5a93ee08478da7f3ec1ca7` |
| `manifest.json` | `8cf76ffa998d75651fb8e517c18c95c8cee88bebcec4118c937631f15b71f263` |

## Known Limits And Recommended Next Goal

Installed KiCad and stock library promotion cannot run in the intentionally
offline GitHub-hosted workflow; those gates are required in configured
promotion environments and remain explicit, never silently passing when
skipped. The matrix proves the reviewed supported envelopes, not arbitrary
unmodeled circuits or analog performance outside their declared evidence.

The recommended next goal is a clean-checkout promotion orchestrator that
installs or discovers a pinned KiCad toolchain, runs this same manifest's
optional gates without hand-written commands, signs the resulting manifest,
and publishes the evidence as a release artifact. That closes the remaining
gap between offline regression safety and reproducible installed-tool release
qualification.
