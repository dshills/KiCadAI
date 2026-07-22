# External Review Mitigation Baseline

Date: 2026-07-22
Implementation baseline: `cfa5cf23`
Specification baseline: `77f72cd`

## Purpose

Freeze the confirmed behavior from `../FEEDBACK.md` before mitigation code is
changed. Paths below use environment variables instead of machine-specific
absolute library paths. The durable fixtures do not depend on the sibling
`test-kicadai` repository.

## Environment

- Go commands use an isolated writable `GOCACHE`.
- Stock-library reproduction uses the installed KiCad 10 symbol and footprint
  roots.
- Curated-library reproductions use configured `KICADAI_SYMBOLS_ROOT` and
  `KICADAI_FOOTPRINTS_ROOT` values.
- The external compact LED request was sanitized and checked in as
  `examples/intent/led_indicator_compact.json`.
- User paths, author identity, and external project metadata were not copied.

## F1: Regulator Composition Placement

Command shape:

```sh
kicadai \
  --request ./examples/intent/regulator_ap2112k_sensor.json \
  --output "$OUTPUT" --overwrite intent create
```

Observed result:

- intent planning reports ready with score 100;
- creation blocks in placement;
- fixed regulator and sensor members are reported outside the usable board or
  in conflict with the USB placement keepout;
- rigid groups then report that no legal translation preserves relative
  placement;
- dependent proximity checks report missing anchors.

The current unit ordering places individual components before calling the
post-placement rigid-group preservation path.

## F2: Unrelated Stock-Library Diagnostics

Command shape:

```sh
kicadai \
  --symbols-root "$KICADAI_STOCK_SYMBOLS_ROOT" \
  --footprints-root "$KICADAI_STOCK_FOOTPRINTS_ROOT" \
  circuit create \
  --request ./examples/circuit-graph/rc_filter.json \
  --output "$OUTPUT" --overwrite
```

Observed result:

- creation blocks in `library_context`;
- reported symbols are not referenced by the RC request;
- examples include hidden-power and duplicate-pin findings from unrelated
  installed symbol files;
- stock footprint inventory warnings are also returned;
- the merged report was approximately 21 MB in this reproduction.

The synthetic skipped regression adds an unrelated hidden-power symbol to an
otherwise valid RC library fixture. Phase 3 must make that test pass while
retaining a negative test for a referenced invalid symbol.

## F3: Compact LED Placement

Command shape:

```sh
kicadai \
  --request ./examples/intent/led_indicator_compact.json \
  --output "$OUTPUT" --overwrite design create
```

Observed result on the declared 40 mm by 25 mm board:

- fixed indicator members are reported outside the usable board;
- `status.inline_indicator` reports no legal shared translation;
- the required proximity anchor is then reported missing.

The same external request passed only after its board was expanded. Increasing
the checked-in board is not an accepted mitigation.

## F4: JSON Transport And Volume

Current JSON stdout parsed successfully when stdout and stderr were captured
separately. The original non-JSON interleaving was not reproduced on this
baseline.

Confirmed remaining problem:

- stock-library failure output includes the complete unrelated diagnostic set
  and resolved graph evidence, making it unnecessarily large for an agent.

Phase 4 retains a parse regression, adds concurrent/logging stress, and bounds
representative failure output after Phase 3 removes unrelated blockers.

## F5: Circuit Help And Function Discovery

Command:

```sh
kicadai circuit --help
```

Observed result:

- exits nonzero;
- reports `unsupported circuit subcommand --help`;
- suggests only `preflight` and `create`;
- no complete public function-level request exists under
  `examples/circuit-graph`.

The desired help behavior is frozen as a skipped CLI regression until Phase 4.

## F6: Circuit-Lane Evidence

A successful function-level circuit creation currently writes these core
`.kicadai` files:

- `manifest.json`;
- `simulation.json`;
- `transaction.json`.

It does not write the lane-neutral promotion, validation-summary, or workflow
artifacts used by provider and intent workflows. The desired shared artifact
inventory is frozen as a skipped regression until Phase 6.

## Known-Failure Test Policy

Desired behavior that would make the committed baseline red is represented by
tests with a `known F<n>` skip message. A phase is not complete until it removes
the corresponding skip and the test passes for the desired behavior.

The skip messages are deliberately searchable:

```sh
rg -n 'known F[1-6]' cmd internal
```

The final promotion phase must find no remaining known-finding skips.

## Baseline Test Execution

Focused packages after adding the reproduction fixtures:

```text
go test ./internal/placement ./cmd/kicadai -count=1
ok   kicadai/internal/placement
ok   kicadai/cmd/kicadai
```

The unrestricted full suite was interrupted after
`internal/compositionlowering` ran for approximately 309 seconds without
finishing. A focused diagnostic run with `-timeout=120s` timed out while the
following pre-existing simulation-heavy corpus cases were active:

- `class_ab_power_interface.json`;
- `protected_power_mosfet_load.json`;
- `regulated_mcu_sensor_subsystem.json`.

The timeout is not attributed to the Phase 0 documentation and skipped
regression additions. It remains a baseline limitation that must be accounted
for when interpreting full-suite duration; focused mitigation packages and the
final configured CI result remain required.
