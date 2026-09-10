# Clean-checkout V22 validation

Date: 2026-09-09. The primary local validation checks below ran from the clean,
detached checkout of
`d3b6088fbaa4f318b5685d62c4fc83eeba3eb4a2` at
`/tmp/kicadai-v22-final-clean`. This is the committed implementation and public
evaluator freeze. No evaluated source was edited during these checks.

The toolchain was Go 1.26.8 with `GOENV=off`, `GOWORK=off`, empty `GOFLAGS` and
`GOEXPERIMENT`, and the repository's authenticated dependency cache. Native
checks used the installed KiCad 10.0.3 CLI and libraries. The public evaluation
was running concurrently; these wall times are observations, not controlled
performance comparisons. No held-out data or keys were accessed and no GitHub
Actions workflow was manually dispatched.

## Quality and preservation

- `make lint`: formatting, vet and full-project configured lint passed; zero
  lint issues.
- `make test-bounded GO_TEST_FLAGS=-count=1`: the complete uncached bounded
  suite passed. The synthesis package took 509.081 seconds.
- `make coverage-check COVERAGE_MAX_WORKERS=2`: all ten authenticated shards
  completed and merged. Generated-code-excluded coverage was **80.1%**, above
  the **75.0%** threshold. No source/index edits occurred during coverage.
- `make race-short`: every package passed.
- `make review-matrix`: both repetitions of the five-package external-review
  regression matrix passed. This is separate from the external Prism review.
- Focused V22 race tests passed in the synthesis package (12.859 seconds),
  executor package (6.267 seconds) and command package (2.205 seconds).
- Both rounds of the five educational examples' source, conventional-topology
  and installed-library layout checks passed (4.718 seconds in total).
- All 13 declared success/refusal expectations in the installed-KiCad design
  example tier passed (75.046 seconds), including protected USB-C, amplifier,
  ESP32 and sensor cases. This is not a claim of thirteen passing circuits.
- The unchanged admitted V18 public multi-output threshold regression passed
  installed-KiCad promotion and exact project replay (26.738 seconds).
- The full bounded suite authenticated the diagnostic, implementation,
  historical/current evaluator and corpus seals, including the new V22 freeze.
- The V22 command, executor and frozen-audit test binaries cross-compiled for
  Linux AMD64. This is compilation evidence, not Linux runtime testing.

## Independent V22 fixtures

The single- and dual-monitor development fixtures again passed the unchanged
installed-KiCad promotion path (30.749 seconds total), including clean-root
replay, ERC, strict DRC, complete routing, connectivity, writer correctness and
zero round-trip differences. Their generated-project hashes match the earlier
implementation reviews exactly:

| Fixture | Project SHA-256 |
| --- | --- |
| Single monitor | `70e8442b8cc96472fe6d5a3abde382e2dafb3ec258c01c4c383adb131b3652b6` |
| Dual monitor | `207ae84c8d11a306e0c8b81355e0e81201bf843f919181500aafcc06f72a881a` |

These are independent development fixtures. They do not add passes to the
frozen public corpus.

## Promotion bundle and release verification

The six-scenario clean-checkout bundle completed twelve passing command and
promotion results. Its verification receipt reports `pass` and authenticates
all 286 files in bundle
`sha256-3ad771fdc66c1dc2326908a11853dd8af463277fb408d5c0ef3d64a8f58141e8`.
The retained bundle and receipt are under
`/tmp/kicadai-v22-clean-promotion-bundle`.

`make release-reproducibility` produced byte-identical macOS/Linux AMD64/ARM64
artifacts across two builds. A retained verification build is under
`/tmp/kicadai-v22-clean-release-artifacts`; the host clean-install/first-run
smoke check passed, including help, application version, capability boundary,
public example and representative fail-closed refusal.

These binaries report the unchanged application version 1.0.1 and the exact
frozen commit above. They are **verification builds**, not new release assets
or replacements of any published tag or artifact.

## Evaluator executable reproduction

A separate clean local clone at `/tmp/kicadai-v22-evaluator-source-rebuild`
checked out the exact frozen commit and rebuilt the evaluator with the original
`go build -trimpath` invocation and pinned environment. The resulting
`/tmp/kicadai-v22-evaluator-rebuilt-clone` is byte-identical (`cmp` exit 0) to the
original running evaluator. Both SHA-256 digests are
`b391c07b0ff624b0d4042c3c25ac88061aab0506f375750a9826f3f7b2af885e`.
Both binaries record Go 1.26.8, the frozen Git revision and commit timestamp,
`vcs.modified=false`, identical dependency digests, and identical build settings.
This check compiled the evaluator only; it did not execute the corpus again.

An earlier rebuild from the linked clean worktree was not byte-identical: it
omitted the Git settings and used `(devel)` module metadata. Native access and
explicit `-buildvcs=true` did not change that result. Inspection of the installed
Go 1.26.8 VCS detector showed that its Git root declaration accepts a `.git`
directory, whereas the linked worktree has a `.git` file. The ordinary local
clone restored the original metadata and exact executable bytes. Reproduction
instructions therefore require a normal clone for this evaluator/toolchain;
the failed linked-worktree comparisons are not claimed as passing checks.

## Retained logs and remaining work

Machine-local logs use `/tmp/kicadai-v22-clean-` followed by `lint.log`,
`bounded.log`, `coverage.log`, `race.log`, `review-matrix.log`,
`focused-race.log`, `educational.log`, `design-examples.log`, `independent.log`,
`v18.log`, `promotion-bundle.log`, `release-repro.log`, `release.log` and
`release-smoke.log`. Coverage profiles and the shard inventory are retained in
the clean checkout's `.coverage` directory. Both source checkouts were clean
after the validation jobs completed, before this audit record was added.

The frozen corpus run subsequently completed all 24 cases twice and its
post-run source and preservation checks passed. Independent publication tests
authenticate every retained replay, passing promotion, and the exact checksummed
artifact inventory, including rejection of changed and rehashed evidence.
Post-publication full formatting/vet/lint passed; focused race tests passed in
the V22 synthesis, executor, command, frozen-audit, and publication packages.
After review remediation, both audit race suites and scoped lint passed again,
as did the staged whitespace check and historical missing-replay/promotion
negative regressions. The [Prism receipt](../publication-v22/REVIEW.md) records
no unresolved valid findings. V22 produced zero additional complete passes.
Review and PR handoff are integration gates, not evidence of electrical
improvement. The existing supported v1 boundary is unchanged.
