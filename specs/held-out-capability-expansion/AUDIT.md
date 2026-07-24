# Held-Out Capability Expansion Completion Audit

Audit date: 2026-07-24

## Result

The milestone satisfies the behavior-only expansion contract in
[`SPEC.md`](SPEC.md). The frozen twelve-case benchmark improved from 5/12 to
11/12 complete passes using the same evaluator, manifest, and gate profile.
All six controls pass. The three constant-current cases and both
precision-rectification cases now pass trusted simulation, internal
validation, physical realization, writer, installed-KiCad, round-trip, and
replay gates.

The only remaining benchmark gap is standalone `clock_generation`. It remains
fail-closed at architecture selection with
`ARCHITECTURE_CAPABILITY_UNSUPPORTED`; no acceptance gate was weakened or
converted into a skip.

## Frozen Benchmark And Baseline

The benchmark was frozen before production support for either promoted family:

| Artifact | Commit | SHA-256 |
| --- | --- | --- |
| Version 1 corpus and manifest | `6e4d2209c6aae04994febca30c9ec38a40051cc2` | manifest `e0d55f484c749eba7d3279da13c21380f5b52f9953f333adf1916409620eb442` |
| [`BASELINE_REPORT.json`](BASELINE_REPORT.json) | `6006970e` | `ba47d125141a56deb54127c3e51ae36f2ce5df2efbd875334b7433d9ee63582c` |
| [`FINAL_REPORT.json`](FINAL_REPORT.json) | `fa7f60ba8ed70b1fd921f76e29d13222642880b4` | `9541502f81f2748aded5f00383ba0dd927591c102a61ec980e4e6f17d53047c2` |
| [`PROMOTION_MATRIX.json`](PROMOTION_MATRIX.json) | `fa7f60ba8ed70b1fd921f76e29d13222642880b4` | `765bedafc4cda2484d2b68e92bd2cf3f3f2a975bb784d0ada268f8d5d8f358d0` |

Both reports use evaluator `held-out-capability-stage-v1`, the same manifest
hash, and the same required gate profile. The final report regenerated
byte-for-byte from the completed implementation and installed KiCad 10.0.3.

## Measured Improvement

| Measure | Frozen baseline | Final |
| --- | ---: | ---: |
| Complete passes | 5 | 11 |
| Blocked cases | 7 | 1 |
| Cumulative stage reach | 105 | 168 |
| Controls passing | 5/6 | 6/6 |
| Held-out cases passing | 0/6 | 5/6 |

The baseline's two largest ranked unsupported clusters were:

1. `constant_current_regulation`: three cases across the power, MCU, and sensor
   domains; and
2. `precision_rectification`: two cases across the analog and mixed-signal
   domains.

They are electrically distinct. Constant-current regulation controls delivered
energy against compliance, load, tolerance, and thermal bounds.
Precision rectification implements a nonlinear signal transfer with
op-amp/diode operating-region, accuracy, recovery, and downstream-drive
obligations. The final report contains no blocker for either family.

## Generic Capability Closure

Constant-current support adds catalog-driven regulator selection, typed
behavioral contracts, deterministic set-resistor and rating selection,
component/model provenance, simulation assertions, lowering, and reusable
physical semantics. It is exercised independently as a bounded power output,
an MCU-controlled peripheral supply, and a sensor excitation source.

Precision-rectification support adds a reusable provider and component
evidence, deterministic preferred-value selection, reviewed op-amp and diode
simulation bindings, bounded nonlinear solving, accuracy/corner assertions,
and composition with an existing ADC acquisition chain.

Physical closure is generic rather than corpus-aware. The implementation:

- routes constrained leaves before shared trunks;
- removes redundant disconnected route leaves;
- discovers required layer transitions from segment interiors;
- batches transaction-owned layer-junction vias deterministically;
- avoids redundant vias at already connected plated junctions;
- applies physical-clearance repair to generated junction vias; and
- preserves route-operation provenance through tracks and vias.

Current-sense simulation also omits a genuinely unexposed measurement-only
interlock while rejecting partially specified interlock roles. Catalog
parameter selection accepts finite zero-valued non-negative model bounds, and
the nonlinear centered op-amp seed remains bounded without an arbitrary
four-device limit.

Production shortcut search found no benchmark case ID, corpus path,
`fixture_role`, baseline/final report identity, or promotion-matrix identity in
non-test Go code. No fixture coordinate, request hash, family label, allowlist,
schema exception, or identity-aware fallback was added.

## Implementation History

| Boundary | Commit | Subject |
| --- | --- | --- |
| Benchmark freeze | `6e4d2209` | Freeze held-out capability expansion benchmark |
| Untouched baseline | `6006970e` | Freeze held-out capability baseline |
| First ranked family | `4a634db4` | Promote held-out constant-current capability |
| Second family, physical closure, reports, and promotion tooling | `fa7f60ba` | Complete held-out capability expansion |

## Local Verification

The final committed implementation passed:

```sh
make GO_TEST_FLAGS=-count=1 GO_TEST_TIMEOUT=45m test
make lint
```

The uncached full suite passed every package. The slow
`internal/compositionlowering` package completed in `1763.639s`; lint reported
zero issues.

After hosted CI demonstrated that the simulation-grounded lane's former
20-minute alarm was too short, the exact named-test command was rerun locally
with the approved 40-minute budget. All ten cases passed in `497.282s`,
including `class_a_amplifier.json` in `214.57s` and
`class_ab_amplifier.json` in `496.68s`. The correction changes only the Go
alarm and outer job deadline; it does not skip, remove, or weaken any test.

Installed KiCad 10.0.3 passed:

- `usb_c_i2c_sensor_3v3_protected`;
- `usb_c_led_indicator_protected`;
- all three constant-current held-out cases;
- both precision-rectification held-out cases; and
- the ordered adjacent-family control regression.

The two protected USB-C fixtures completed together in `10.68s`. The
constant-current corpus completed in `61.04s`, the precision-rectification
corpus in `166.59s`, and the ordered controls in `102.15s`.

The final twelve-case evaluator completed in `429.26s` and reproduced report
SHA-256
`9541502f81f2748aded5f00383ba0dd927591c102a61ec980e4e6f17d53047c2`.
Every one of its eleven passing rows requires clean ERC, strict DRC,
connectivity, complete routing, writer correctness, zero round-trip
differences, and byte-identical replay.

## Clean-Checkout Promotion Receipts

Both commands ran from clean commit
`fa7f60ba8ed70b1fd921f76e29d13222642880b4` with the locked Darwin ARM64
KiCad 10.0.3 toolchain:

```sh
make PROMOTION_ROOT=/tmp/kicadai-promotion-fa7f60ba promotion-bundle
make HELD_OUT_PROMOTION_ROOT=/tmp/kicadai-held-out-promotion-fa7f60ba \
  held-out-promotion-bundle
```

The outputs were outside the worktree. Each command ran its matrix twice,
required one content-addressed bundle, and invoked the standalone verifier on
that copied output.

| Matrix | Executions | Bundle and manifest SHA-256 | Files | Verifier |
| --- | ---: | --- | ---: | --- |
| Existing promotion matrix | 12 | `09d64b8ce1949bbb6d8fedd769cd6a47d8f5940f98c3ba254e067154a88044f6` | 286 | `pass` |
| Five newly promoted held-out cases | 10 | `20cc58ffa9286b3301ede547b7203a462f5c19ab87798b514236a7412714b425` | 320 | `pass` |

The held-out bundle contains all three constant-current and both
precision-rectification requirements and their normalized requirements,
architecture/component evidence, simulation evidence, generated projects,
ERC/DRC reports, connectivity/routing/writer/round-trip results, replay
comparisons, tool identities, inventories, manifests, and checksums.

## Prism Review

Repeated staged Prism review drove generic corrections for multi-transition
route operations, redundant decoding, generated-via clearance repair,
arbitrary nonlinear-seed limits, and zero-valued catalog parameters. The final
review reported zero high and zero medium findings.

Two low performance observations remain non-blocking:

- route-tail cleanup uses bounded linear pad scans rather than adding a spatial
  index to this milestone; and
- the MNA scratch `sync.Pool` retains the largest previously used buffer.

Neither changes correctness, determinism, acceptance gates, or the measured
benchmark result. The full routing, simulation, and end-to-end suites pass.

## GitHub Actions

Implementation commit `fa7f60ba8ed70b1fd921f76e29d13222642880b4`
is bound to:

- ordinary CI run
  [`30085430213`](https://github.com/dshills/KiCadAI/actions/runs/30085430213);
  and
- installed-KiCad publication run
  [`30085458955`](https://github.com/dshills/KiCadAI/actions/runs/30085458955).

The installed-KiCad publication run completed successfully. It reproduced and
verified content-addressed bundle
`sha256-b414fac72252ec66a76247931b43e9324e9db3152193c0a477de1bed62e35c76`
with 286 inventoried files, then uploaded:

- artifact ID `8593780582`;
- artifact name
  `kicadai-promotion-fa7f60ba8ed70b1fd921f76e29d13222642880b4`;
- uploaded ZIP SHA-256
  `5cb76f5de0c41a43f718a363f0ebce298d26a6747fb162f262880e91f093e605`;
  and
- [artifact download](https://github.com/dshills/KiCadAI/actions/runs/30085458955/artifacts/8593780582).

The initial ordinary CI run passed formatting, lint, the clean-checkout
contract, bounded tests, the coverage floor, open-set and adversarial corpora,
and the external-review ladder. Its simulation-grounded job reached Go's
20-minute alarm while the Class A and Class AB cases were still actively
solving. The approved delivery correction assigns that unchanged corpus a
40-minute Go alarm and a 45-minute job deadline. The exact pushed delivery
commit is accepted only after its complete ordinary CI run is green.

## Remaining Boundary And Next Goal

This milestone proves measurable generic expansion over one frozen twelve-case
sample. It does not prove arbitrary circuit generation, unrestricted part
qualification, dense-board routing, RF/high-speed correctness, mains safety,
or fabrication release.

The next measured gap is generic standalone clock generation. The next
milestone should implement and hold out at least two electrically distinct
clock-source families, with deterministic architecture/component evidence,
startup and frequency proof, clock-aware physical design, the same strict
promotion gates, and no regression in this benchmark.
