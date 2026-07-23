# External Review Regression Ladder

The independent six-circuit review from 2026-07-21 is a release-blocking,
offline regression ladder. Its machine-readable inventory is
`testdata/external-review-mitigation/matrix.json`. The checked-in cases are the
review requests or identity-neutral equivalents; no case depends on the
reviewer's checkout or user-specific paths.

Run the complete offline ladder twice:

```sh
make review-matrix
```

The JSON manifest is the scenario-requirements source of truth; the Make target
is the executable test source of truth, avoiding a second test-name registry.
The target validates the matrix contract and runs the corresponding scenario
and negative tests twice. It covers compact rigid-group placement, intent planning, stock
library diagnostic scoping, explicit-graph creation, multi-unit and
split-supply behavior, function-level synthesis, JSON transport, shared
creation evidence, deterministic replay, and the four required fail-closed
cases. GitHub Actions runs the same command in the `External review regression
ladder` job.

The stock-library quickstart uses the installed KiCad roots rather than a
curated subset:

```sh
go run ./cmd/kicadai \
  --symbols-root "$KICADAI_STOCK_SYMBOLS_ROOT" \
  --footprints-root "$KICADAI_STOCK_FOOTPRINTS_ROOT" \
  --kicad-cli "$KICADAI_KICAD_CLI" \
  --require-erc --require-drc --require-kicad-roundtrip --strict-diffs \
  --request ./examples/circuit-graph/rc_filter.json \
  --output "$OUTPUT" --overwrite circuit create
```

Release promotion from a clean checkout does not require manually configured
KiCad paths:

```sh
make promotion-bundle
```

The command discovers or bootstraps the locked KiCad release and matching stock
libraries, runs the whole positive matrix twice, and independently verifies the
content-addressed bundle. It is also the command used by the separate
`Installed KiCad Promotion` GitHub Actions workflow.

For focused debugging only, individual optional Go harnesses can still be run
against explicitly selected local paths:

```sh
export KICADAI_KICAD_CLI=/path/to/kicad-cli
export KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols
export KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints

go test ./cmd/kicadai \
  -run '^(TestRunIntentCreateSensorBreakoutPersistsRegulatorEvidence|TestRunIntentCreateCompactLEDOptionalKiCadPromotion|TestPublicFunctionLevelExampleOptionalKiCadCLI)$' \
  -count=1 -v
go test ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier$/(usb_c_i2c_sensor_3v3_protected|class_ab_headphone_driver|class_a_bjt_line_preamplifier)$' \
  -count=1 -v
go test ./internal/compositionlowering \
  -run '^TestPowerInterfaceSynthesisCorpusOptionalKiCadPromotion$' \
  -count=1 -v
```

Those environment variables are a targeted diagnostic interface, not the
clean-checkout release reproduction procedure.

For every applicable generated project, promotion requires clean ERC, strict
DRC, connectivity, required-route completion, writer correctness, and zero
normalized round-trip differences. A skipped KiCad gate is not promotion-pass
evidence.

The complete milestone evidence and remaining limitations are recorded in
[`../specs/external-review-mitigation/AUDIT.md`](../specs/external-review-mitigation/AUDIT.md).
