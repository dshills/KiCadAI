# V23 clean-checkout validation

Validation source: `16013c7d3f4e44ae8cfbda18be655c853ef8f111`, the committed
V23 implementation and evaluator freeze. Checks run in the ordinary clean local
clone `/tmp/kicadai-v23-final-clean`, using Go 1.26.8, `GOENV=off`, `GOWORK=off`,
empty `GOFLAGS` and `GOEXPERIMENT`, and the authenticated repository dependency
cache. Native checks use installed KiCad 10.0.3. No evaluated source is edited.

These preservation checks overlap the one frozen public evaluation. Their
elapsed times are observations, not controlled performance comparisons. They
are not additional V23 corpus executions and do not increase its pass count.
No held-out keys or content are accessed; no Actions workflow is dispatched.

## Completed checks

- Full `make lint`: formatting, vet and configured lint pass with zero issues.
- `make test-bounded GO_TEST_FLAGS=-count=1`: the complete uncached bounded
  suite passes. The synthesis package takes 720.581 seconds during the
  overlapping validation workload. Writer, routing, simulation, round-trip,
  corpus, historical/current seal and source-authentication tests are included.
- `make coverage-check COVERAGE_MAX_WORKERS=2`: all ten authenticated shards
  complete and merge successfully. Generated-code-excluded coverage is
  **80.0%**, above the **75.0%** threshold; raw coverage is 76.0%. Profiles and
  inventory are retained in the clean clone's `.coverage` directory.
- `make race-short`: all ten configured packages pass.
- Focused V23 races pass in synthesis (41.314 seconds), executor (6.797),
  command (1.939) and evaluator-freeze (2.949). The diagnostic package matched
  no tests in this focused invocation; its complete tests are covered by the
  bounded suite. The clean clone also passes `go mod verify`.
- `make review-matrix`: the five-package external-review regression matrix
  passes both repetitions. This is separate from the external Prism review.
- All thirteen declared success/refusal expectations in
  `TestDesignExamplesOptionalKiCadBackedTier` pass (98.459 seconds), including
  protected USB-C, amplifier, ESP32 and sensor examples. This does not mean
  thirteen circuits are supported or passing.
- The five educational schematic source, conventional-topology and
  installed-library layout checks pass twice (10.427 seconds total).
- The admitted V18 low-voltage multi-output threshold case passes its unchanged
  two-clean-project installed-KiCad promotion (42.09 seconds).
- Both independent V23 monitor fixtures pass simulation, lowering and
  installed-KiCad promotion from two clean projects each (35.52 seconds total).
  The single-monitor project hash remains
  `156dfc12bdaeadba01cc462ae5e74e97d69947441c7cfb49d2df1dd372b8a7aa`;
  the dual-monitor hash remains
  `207ae84c8d11a306e0c8b81355e0e81201bf843f919181500aafcc06f72a881a`.
  These exactly preserve the committed independent-fixture evidence; they are
  not new corpus passes.
- The V23 command, executor and frozen-audit test binaries cross-compile for
  Linux AMD64. This is compilation evidence, not Linux runtime testing.
- The six-scenario clean-checkout promotion bundle passes. Its independent
  verification receipt authenticates all 286 files in
  `sha256-42b98cb802866937516597f7ccf13bc704930977ffb7ef508891646b15e8b475`.
  The bundle and receipt are retained at
  `/tmp/kicadai-v23-clean-promotion-bundle`.
  All twelve recorded command results and twelve promotion results are `pass`.
- `make release-reproducibility`: macOS/Linux AMD64/ARM64 binaries, release
  manifest and checksums are byte-identical across two builds.
- A retained verification build at `/tmp/kicadai-v23-clean-release-artifacts`
  passes every SHA-256 check. Its manifest binds version `1.0.1`, the exact
  frozen commit, Go 1.26.8, CGO disabled and commit-derived build timestamp
  `2026-09-10T06:05:20-05:00`.
- The host release clean-install/first-run smoke test passes, including help,
  application version, capabilities, a supported public example and a
  representative fail-closed refusal. These are verification builds, not new
  release assets, tags, or replacements of the published v1.0.1 release.

## Evaluator reproduction

The exact `go build -trimpath` invocation in the clean ordinary clone produces
an evaluator byte-identical to the binary executing the original public run
(`cmp` exits zero). Both SHA-256 digests are
`1939031aa7c2ff025b52c857b3d10d816ba60d6ae644dcbd6155527430acaff1`.
The rebuilt binary is `/tmp/kicadai-v23-evaluator-rebuilt`; the original is
`/tmp/kicadai-v23-public-evaluation-1/evaluator`. This check builds only; it
does not execute the corpus again.

## Completed evaluation and publication authentication

The listed clean-checkout local gates completed while the clone was clean at
the frozen commit. The same clone was subsequently used to stage a copy of
the separate publication audit for review; that does not change which source
the completed quality gates validated.

The original public evaluation completed all 24 cases / 48 replays on September
10 at approximately 16:03 UTC. Its original launcher emitted the completion
marker after checking the unchanged Git revision, unchanged tracked sources,
and successful frozen assessment. The final preservation assessment is true,
with twenty unselected cases preserved and zero regressions. The improvement
assessment is false: no additional complete simulation-and-KiCad pass occurred.

The complete separate retained-evidence and pinned inventory audits pass. The
optional native-byte audit independently authenticates all four original clean
KiCad projects against their recorded project hashes, without running KiCad.
Publication audit race tests pass, and scoped lint reports zero issues. The
new audit was untracked throughout the original run; no evaluated source or
original result was edited or restarted. See [results](RESULTS.md) for the
report and manifest identities and [review](REVIEW.md) for review disposition.

After the complete publication review, required-report skips were removed from
the new audit and identity-marshalling diagnostics were improved. An isolated
clone with no V23 report correctly fails both required publication tests; after
copying the exact retained publication, both complete packages pass. The final
corrected publication race test passes (1.634 s), and repository-wide lint again
reports zero issues. These are non-numerical audit checks, not corpus retries.

One later read-only checkpoint-audit launch stopped before its tests started
because the sandbox could not reach the Go checksum service to authenticate
the pinned toolchain. Repeating only that non-numerical audit with permitted
checksum-service access passed. The original evaluator remained running;
no corpus case, numerical analysis or physical promotion was restarted.

During case 022, an interrupted monitoring session lost its tool handle. Native
process inspection confirmed the original launcher (PID 30564), timing wrapper
(PID 30741), and evaluator (PID 30742) were still running with the original
command and increasing evaluator CPU time. A separate read-only observer was
attached to those same processes; it polls without starting synthesis or writing
evaluation artifacts. No evaluation, case, numerical analysis, or promotion was
restarted. Because the original tool-session exit status is no longer available,
terminal verification therefore uses the original launcher's completion marker,
original assessment log, and retained report/resource evidence—not the observer's
exit status.

The separate retained-evidence audit additionally checks each replay's exact
predecessor digest against its immutable V22 sidecar. Synthetic tests reject
self-consistently rehashed predecessor substitutions for selected and unselected
cases. The updated audit passes against all twelve completed cases (24 replays),
its race tests pass (1.224 seconds), and scoped lint reports zero issues. These
are non-numerical evidence checks, not additional corpus executions.

Machine-local logs use `/tmp/kicadai-v23-clean-` followed by `lint.log`,
`bounded.log`, `coverage.log`, `race.log`, `review-matrix.log`, `educational.log`,
`design-examples.log`, `independent.log`, `promotion-bundle.log`,
`release-repro.log`, `release.log`, `release-smoke.log` and `focused-race.log`.
