# V22 implementation validation

Date: 2026-09-09. Scope: the additive implementation sealed by
`V22_IMPLEMENTATION.sha256`, before the public successor evaluation is frozen.
This is not a replacement of any historical corpus report or evidence seal.

## Passing local checks

- `make test-bounded GO_TEST_FLAGS=-count=1`: complete uncached bounded suite
  passed, including V20/V21 and diagnostic-publication contracts. The synthesis
  package took 439.469 seconds. Later V22-only review changes received affected
  regression/race reruns; historical source remained unchanged.
- `make coverage-check`: ten authenticated shards covering the bounded test
  inventory; generated-code-excluded total **80.1%**, threshold **75.0%**.
- `make lint`: formatting, vet and full-project lint passed with zero issues,
  including a final rerun after predecessor authentication was added.
- `make race-short`, plus focused V22 certificate, admission, control-path,
  bounded-search, predecessor-authentication and synthesis race suites: passed.
- `make review-matrix`: both repetitions of the five-package external-review
  regression matrix passed. This is local regression execution, not another
  external provider or corpus evaluation.
- Two repetitions of the five educational source/layout checks with installed
  KiCad symbol/footprint libraries passed, with no blocking layout diagnostics.
- `TestDesignExamplesOptionalKiCadBackedTier`: all 13 declared success/refusal
  expectations passed under native macOS execution. Includes protected USB-C,
  amplifier, ESP32 and sensor examples; not a claim of thirteen passing designs.
- V20/V21 contract/evaluator seals and the immutable electrical diagnostic and
  root-cause evidence seals authenticated unchanged.
- Three staged Prism reviews and dispositions are recorded in `V22_REVIEW.md`.

## New independent electrical and physical proofs

The single monitor requires one control edit and two complete electrical
assertions. The dual monitor requires two edits and three assertions. The new
bounded continuation selects and lowers both; each then passes the existing
installed-KiCad promotion path twice in separate clean roots. All required ERC,
strict DRC, route completion, connectivity, writer correctness, zero round-trip
differences and raw project replay checks pass.

| Fixture | Exact generated-project SHA-256 |
| --- | --- |
| Single monitor | `70e8442b8cc96472fe6d5a3abde382e2dafb3ec258c01c4c383adb131b3652b6` |
| Dual monitor | `207ae84c8d11a306e0c8b81355e0e81201bf843f919181500aafcc06f72a881a` |

The pair passed again after review with the same project hashes. The two-run
test took 23.18 seconds initially and 28.20 seconds in the review rerun. These
single development timings are not controlled performance comparisons.

## V18 preservation and environment failure

The unchanged public V18 low-voltage multi-output threshold promotion passed
with native macOS access in 31.07 seconds, including both clean-root KiCad runs.
A preceding sandboxed preservation attempt aborted in KiCad's zone-refill CLI
before ERC/DRC. That attempt is a recorded environment failure, not a pass or a
circuit regression. No V18 source, bounds, graph, model or validation gate was
changed. The fresh native-access run used the same established regression test.
Future installed-KiCad gates that refill zones need native macOS access here.

## Remaining milestone work

The public corpus still has only its established **one complete pass out of
24**. Independent development fixtures do not increase that count. Commit the
exact successor selection, source/environment seals, limits and 24-case/two-replay
protocol before execution. Publish the aggregate result and preservation check
even if no new full pass is obtained; do not retune or rerun a frozen cohort in
response to outcomes. Final clean-checkout promotion/release verification and
the goal's completed PR handoff remain separate pending work.
