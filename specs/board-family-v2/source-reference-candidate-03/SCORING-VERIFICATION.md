# Indexed evaluation scoring and review checkpoint

Date: 2026-09-14. Status: offline implementation and tests; **not live model
acceptance, production adoption, independent review, or bench qualification**.
The existing key reuse choice remains recorded, but all real provider keys were
removed from these test processes. No real API request, Gemini review, firewall
change, commit, push, PR mutation or screen interaction occurred in this work.

## What is now implemented

`evaluation-plan.json` reuses the exact 14 known regression prompts and gold
requirements from typed evaluation 02, without modifying that historical file.
The original SHA-256 is
`90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867`.
The proposed new policy is 14 physical requests and USD 1.00, but the plan is
explicitly **not approval**. It does not restore any exhausted old allowance.

`scoring.mjs` separately measures raw extraction and application correctness.
Numbers match through the application-owned quantity table, including required
polarity when an older numeric gold requirement has no explicit state. IDs,
units, field roles and matching gold are structural evidence, not a guarantee
that the English request was understood completely.

Review templates start pending with false judgments and no reviewer signature.
They include the full original prompt, emitted facts, complete cited clauses,
quantity records, actual decision, gold requirements and per-case review rubric.
Checking a completed review binds those contexts to the authenticated selection
bytes and requires an explicit rationale for every fact and requirement, plus
completeness and targeted-question/truthful-refusal judgments. An invented fact
can therefore fail raw review even when all minimum gold facts are present.
The recorded reviewer kind must disclose implementing-agent versus independent
review; this is attribution, not cryptographic proof of authorship.

Before scoring, the verifier checks exact case order and inventory, prompts,
process records, plan and scorer dependency hashes, and all saved byte hashes.
It re-exports the schema/context from the pinned executable with no API key and
rejects a repinned contract that differs from that executable. It reruns the Go
journal auditor offline, then independently recomputes collector classification,
ledger-prefix joins, summary/exit expectations and reviewed native comparison.
It does not trust an edited `advance`, `model_correct` or `bundle` flag merely
because an attacker also recomputed file hashes.

All 14 planned cases remain in the denominator, including invalid extractions,
unsafe attempts and unattempted cases. All five useful-case elapsed times are
required, including failures; median must be below 60 seconds and maximum below
120 seconds. Full acceptance still requires all 14 raw and application passes,
all required native bundles, completed source-bound review and a separately
authorized live batch. An offline result can never establish live acceptance.

These checks prove local consistency, not provider signatures or tamper-proof
historical process attestation. Actual child termination is observed by the
collector when it runs. A malicious author with control over code, binaries and
all evidence cannot be defeated by local hashes alone.

## Tests and corrections

- Collector/scorer unit suite: **49 passed**, 3.030 seconds. This includes pure
  approval checks; synthetic approval-shaped objects are never saved as actual
  approval and never sent to a live collector.
- Fourteen hand-authored corpus fixtures pass the real pure admission path.
  These fixtures are in a Go `_test.go` file, not a new production vocabulary
  parser, canned-answer path, or model quality measurement.
- Real-command scoring integration: **11 tests passed**, 26.221 seconds,
  including nested tamper tests. Only HTTP transport was synthetic.
- One batch retained all **14 completed invalid extractions**, independently
  authenticated all outcomes and scored **0/14**, without retries or manual
  continuation. A pending review file is rejected by the scorer CLI.
- A separate synthetic batch completed all **14 expected decisions** and all
  **five actual native configurations**. Its raw gold/structural and application
  checks passed; semantic review remains deliberately pending. Native outputs
  passed the complete existing validation/export checks and compared against
  **201** reviewed deliverables: 39 for each of three BMP280 profiles and 42 for
  each of two SHT31 profiles. Collector wall time was **21.588 seconds**; this
  excludes semantic review and is not live-model latency.
- Tamper tests reject coherently rehashed outcome classes, child/auditor exits,
  appended fake bundle proof, edited final ledger history, false failure counts,
  omitted batch files, a repinned invented schema, and changed fabrication output
  even after its manufacturing manifest is rehashed.
- The first native integration assertion incorrectly expected 81 comparisons
  per bundle. It failed after all 14 cases completed. Inspection of the unchanged
  qualification confirmed 39/42 per family (81 for one of each), and the exact
  five-profile assertion `[39,39,39,42,42]` then passed. No generator output or
  historical example was repaired to obtain that pass. Tests clean up only
  their own synthetic scratch directories, not live evaluation evidence.
- CI-shaped run without a native CLI: **15 passed, 2 explicitly skipped**,
  7.189 seconds. Only the two optional real-KiCad tests skip; the 14-invalid-case
  scoring integration and real Go failure-path collector tests execute.
- Affected short Go packages passed: boardfamily 11.667 s, command 7.028 s,
  aiprovider 0.672 s. Race checks passed: 12.916 s, 7.383 s and 2.749 s respectively.
- Full repository lint: **0 issues**. The historical publication authenticator
  passed unchanged: 203 publication files, 177 evidence files, five CI-addendum
  files, and the same failed 5/14 raw, 7/14 application, 5/14 complete result.
- Current-source `make test-fast`, four workers, count 1 and 20-minute package
  deadline: **terminal exit 0**. The formerly long-running
  `internal/opentopologysynthesis` package passed in 257.708 s. This existing
  tier omits six named heavy tests and is not the unfiltered/full acceptance
  tier. It ran with the machine's Go 1.27.1 default, not the pinned release
  compiler; see the additional compiler qualification below.

## Runtime identity and release boundary

The test binary used for these new integration tests is
`.cache/board-family-v2/indexed-scoring-03.FhpM9x/board-family.test`, SHA-256
`801b5111ad5a50a4c997ddc522af36a38a416a7ff19106f8b6e987df0be88fe3`.
The separately built production binary in the same directory is
`kicadai-board-family`, SHA-256
`33169973bdcaaa37f0aca9f07c747b62eb039429342c6c6f29fcb2b7451dd235`.
Both were built offline with Go 1.27.1 on darwin/arm64. The production binary is
not yet bound to a final compiler-closure/qualification manifest. Its build alone
does not inherit the test binary's synthetic-HTTP execution as a live result.

New `.github/workflows/indexed-intent-evaluation.yml` runs unit safeguards, the
synthetic corpus admission tests, and real-Go non-native integration without
provider credentials. It is additive; frozen historical workflows/evidence are
unchanged. It has been exercised locally in the no-native-CLI shape, but has not
yet run as a published GitHub workflow at these source bytes.

Read-only GitHub verification in this turn confirmed PR #14 is open and draft at
`f8ac5099c2e2d21a72a2faf2bb2161ffbec8792a`. All three existing workflows at that
published head report success. These new uncommitted changes are not covered by
that remote success. Do not merge or claim the full goal complete.

Next: bind a fresh production runtime to the compiler-selected source/dependency
closure and current evaluator inputs, record qualification/review against those
exact bytes, and publish/check the exact-source CI candidate. Only after that
readiness check may a new explicit request/dollar approval authorize the single
final live evaluation. No recovery/probe allowance or physical bring-up follows
implicitly from this checkpoint.

## Commands

With the four provider keys and `KICADAI_LIVE_PROVIDER_TESTS` unset:

```sh
node --test specs/board-family-v2/source-reference-candidate-03/collector.test.mjs specs/board-family-v2/source-reference-candidate-03/scoring.test.mjs
KICADAI_INDEXED_TEST_BINARY="$PWD/.cache/board-family-v2/indexed-scoring-03.FhpM9x/board-family.test" \
KICADAI_OFFLINE_NATIVE_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
node --test specs/board-family-v2/source-reference-candidate-03/scoring.integration.test.mjs
```

For a future authenticated batch, `scoring.mjs --template PLAN MANIFEST BATCH
REVIEW [APPROVAL]` creates an exclusive pending template. After actual review,
`--check` authenticates and scores it. The optional approval path is required for
live evidence; it is rejected for offline evidence. Neither command makes a
provider request. Gold/review data stays evaluator-only and never enters the
model request.

## Compiler-closure follow-up

A read-only compiler dependency inventory found 1,211 selected nonstandard,
assembly/embed and package-test files under Go 1.27.1: 1,187 match the old frozen
runtime and 24 are new or changed relative to it. Besides the indexed-command
work, this includes atomic publication dependencies and two Unicode 17 files
selected by the newer compiler. It would therefore be incorrect to inherit the
old runtime's compiler/dependency qualification wholesale.

`go.mod` and the old runtime pin Go 1.26.8, whose compiler is already cached
locally. A new production and test build explicitly used that compiler, without
a download, toolchain auto-upgrade or module update. The output directory is
`.cache/board-family-v2/indexed-go126-03.CVpc3a`:

- `kicadai-board-family`: SHA-256
  `94f8cd780dbbdfe12a0503164686353671c5732b6484a80ecdd4e0560f291e20`.
- `board-family.test`: SHA-256
  `2ea3007570247fbd8e89fdc5fe949a3f14760f9f577626e145ff9dabda3870be`.
- The production command exported the expected experimental indexed-v3 contract
  and pinned model offline. Affected short Go tests passed on Go 1.26.8:
  boardfamily 9.414 s, command 4.694 s, aiprovider 0.871 s.
- The combined real-command collector/scoring integration passed **18/18**, no
  skips, in 28.445 s on this pinned test binary. This includes the 14-invalid-case
  batch and the 14-decision/five-native-configuration batch, all 201 comparisons,
  and the fabrication-tamper negative control. The latter collector batch took
  22.889 s. These remain synthetic-HTTP observations, not live AI scores.

All processes started for this checkpoint are terminal. The pinned build and
this inventory are preparation evidence, not a completed final runtime manifest.
The broader fast-tier and race/lint observations above retain their disclosed
Go 1.27.1 scope; they are not silently relabeled as Go 1.26.8 runs.
