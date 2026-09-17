# Semantic boundaries: integrated candidate, live acceptance pending

Checkpoint: 2026-09-17, on `codex/semantic-boundaries-13`. The prior offline
prototype is now an explicit command mode, **not an accepted release**. The
latest actual live result remains v9: **7/14 complete cases and 2/5 required
useful bundles**. Neither the synthetic results below nor green software checks
replace that failed result. The two-family natural-language goal is incomplete.

The closed v9 batch had 14 unique completed responses, no retries, **9/14 raw
fidelity passes**, **9/14 application passes**, and only BMP280 standard/fast
native bundles. All 14 request slots are consumed. Estimated recorded cost is
$0.058352 (19,368 input / 2,452 output tokens), not unused spending authority.
The read-only closure replay was rerun during this checkpoint: 20 review files,
383 evidence inputs and 1,080 historical pre-live files authenticate unchanged.
Raw v9 evidence remains local; this source checkpoint publishes its summary,
not a new raw-evidence archive. Result SHA-256:
`c10ce95eb807f93e33c576f4aee025692af58f4793a239f3fafc3ac06bf268a0`;
QA SHA-256:
`bd851a7adb85cc19242e146c25d5f9da51a44d45b4d736fd9b00f359328488ab`.

## Implemented change

The model no longer paraphrases unfamiliar requirements classified as `other`.
It supplies a residual-span reference, state and context; the application
preserves the full original residual text. Numeric `other` facts preserve their
original clause and distinct quantity occurrence. Genuine `unclear` facts still
require a targeted question. This prevents a model-written detail from narrowing
“no external adapter or change to either requirement” or dropping calibration
conditions from a stated accuracy guarantee. Raw provider bytes remain intact.

This builds on the already tested defaults-control spans and narrow polarity
guards, without introducing another parser architecture. The complete original
request remains present. A control cannot consume a quantity, a compound
sentence's remaining hardware request, or an explicit conflicting bound.

The same command accepts `--intent-protocol semantic-boundaries-v10`. The default
remains `typed-v2`; no prior protocol, native generator, capability catalog,
qualification, evaluation prompt, expected outcome or acceptance threshold is
changed. The new adapter reuses the one-request/no-retry transport and durable
evidence capture. Its identities are separate:

| Purpose | Identity |
| --- | --- |
| Admission | `10-semantic-boundaries-experimental` |
| Request | `semantic-boundary-request-13` |
| Schema | `board_family_semantic_boundaries_v10` |
| Journal | `semantic-boundary-evidence-journal-1` |
| Audit | `semantic-boundary-journal-audit-1` |
| Accounting | `semantic-boundary-v10-gpt-4.1-full-standard-2026-09-17` |

The model is unchanged: `gpt-4.1-2025-04-14`, with 1,600 output tokens and a
16,000-byte request cap. The historical full-model standard-rate estimator and
50,000-microdollar per-request reservation remain unchanged. These are accounting
bounds, not authorization. Old ledger balances and consumed request allowances
cannot be used for this candidate.

`--export-live-contract` with the new protocol and one prompt writes the exact
non-secret contract without a key or network. It explicitly grants no spending
authority. `--inspect-semantic-boundary-journal DIRECTORY` replays only local
bytes. Live mode requires an explicit separate policy, ledger and new journal.

## Verification

Tests used Go 1.26.8, cached dependencies, `GOPROXY=off`, `GOSUMDB=off` and a clean
environment excluding real provider credentials. Injected transports use marked
synthetic responses and placeholder keys, never a network socket.

- All 14 unchanged corpus cases pass through the synthetic command path, with
  expected family/profile/configuration or targeted clarification/refusal.
- Exact fake-provider request bodies for all 14 prompts measure **8,279–12,173
  bytes**, below the 16,000-byte cap. This is wire-byte measurement, not a token,
  cost or live-latency estimate.
- Preflight, no-retry, unknown outcome, model mismatch, request/cost limits,
  bidirectional ledger isolation and nine journal-tampering cases pass.
- Full race tests pass for `internal/boardfamily` (61.439 s),
  `cmd/kicadai-board-family` (54.943 s) and `internal/aiprovider` (2.140 s).
- Repository-wide `golangci-lint run ./...`: zero issues.
- Source-owned-text compiler fuzzing: 132,600 mutation executions, passing in
  11.379 s. This checks compiler/schema invariants, not model reliability.
- All 14 historical v9 production journals replay through the unchanged v9
  inspector and are rejected by the v10 inspector. This does not rescore them.
- The standalone binary exports all 14 exact contracts without credentials.
  Its replay of all five new synthetic native journals passes, and exported
  request input/schema match the retained actual fake-transport wire bytes.

An extra non-short repository run with a 180-second per-package limit did not
pass: two historical Git-output comparisons captured Apple's sandbox `confstr`
warning, and the unchanged `compositionlowering` and `opentopologysynthesis`
exhaustive suites exceeded that limit. The two Git tests pass when `TMPDIR` is
explicitly supplied (0.699 s); no frozen expectation was edited. The timed-out
tests and their source packages are unchanged from v9. That run is not claimed
as a full-suite pass. The repository's normal bounded tier uses `-short`; its
separate result must not be confused with exhaustive simulation coverage.
The repository-wide short run passed the other packages but also exceeded the
180-second limit in `opentopologysynthesis`. This package does not depend on the
changed board-family packages. Its isolated rerun passed in **442.790 s** using
the CI-configured 12-minute timeout. This completes the short tier across the
initial run and isolated rerun, not a single successful full-repository process
or a retroactive pass for either timed-out run.

The real command, deterministic generator and KiCad 10.0.3 completed all five
useful cases from synthetic extractions in **22.65 s test-body time / 23.205 s
package time**. BMP280 standard/fast/low-current and SHT31 standard/fast each pass
all 14 native/export gates with no manual output repair. Their native project,
schematic and PCB hashes match the reviewed examples. All 19 manufacturing
manifest entries per case were separately re-hashed, with each manifest tied to
the generated PCB. These include logs/reports, not 19 Gerber layers.

Retained local artifacts: `.cache/semantic-boundaries-13-integration-01/`.
The binary there is an **offline integration build**, not a frozen authorized
live executable. Its SHA-256 is
`4e32228440a62209ae9af1129b3f56e38d7e352e72be4bdabd7153f59004ce84`.
The Go toolchain embedded the primary checkout's revision in this worktree
build; that metadata does not authenticate this candidate. A release build must
come from a clean candidate checkout with its actual revision verified.
KiCad SHA-256 is
`cc5433d3b41421a4cba065f291aeeb1c5fa69c7a10305a3dc28854f4e6d69534`.
No board qualification or manufacturing-performance claim is inferred from
synthetic language tests. Software validation is not bench validation.

The original corpus and reviewed-example manifest remain unchanged:

```text
90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867  typed-evaluation-02/cases-02.json
aa2279629d361cce7c22e2c73969c69490147f342da0e768b193ea7b26a68705  evidence/examples-01.json
```

## Review and remaining gates

Review is by the implementing agent, not an independent engineering review or
provider attestation. The review checks source ownership, exact caller-byte
preservation, closed schema fields, explicit protocol selection, accounting
isolation, historical replay, command failure boundaries and native output
identity. It does not establish live semantic completeness.

Known limitations remain: the model can omit unfamiliar requirements, select an
incorrect state/context outside the bounded guards, or mark a real requirement
as background. Source-owned text prevents paraphrase loss only for facts actually
emitted. This known corpus is not an independent holdout. [Official Structured
Outputs documentation](https://developers.openai.com/api/docs/guides/structured-outputs)
distinguishes structural conformance from semantic correctness.

There have been **zero new live requests**, firewall changes or merges in this
checkpoint. The primary checkout and frozen v9 worktree remain clean at their
original commits; the v9 binary, result and QA hashes are intact. The user
approved publication to the existing draft PR, and candidate `a35e96ce` plus
the accurate closed-v9 summary were published to [PR #14](https://github.com/dshills/KiCadAI/pull/14).
The PR remains draft; publication does not authorize a merge or live run.

The candidate's main [CI run](https://github.com/dshills/KiCadAI/actions/runs/35255734657)
passed all 25 jobs, including quality gates. Six auxiliary workflows passed,
but [owned offline rehearsal](https://github.com/dshills/KiCadAI/actions/runs/35255734516)
failed its older log-overflow test before recording the healthy first case
(0 recorded outcomes instead of 1). That log does not retain the subprocess
record, so the exact runner-level cause cannot be independently recovered.
The test applied a 500 ms deadline to every case, including the healthy one.

A test-only 750 ms healthy-process delay reproduces the same failure locally:
the first child is killed at 506.6 ms with `timed_out=true`, while
`log_overflow=false`, before the overflow fixture is reached. The correction
uses the normal 60-second ceiling for overflow tests and a 5-second deadline
only for the timeout fixture (which sleeps 30 seconds). Both collector suites
retain the delayed healthy predecessor and now additionally require the named
failure: overflow must not be a timeout, its retained log must be exactly 1 MiB,
and timeout must not be overflow. Stop/no-retry, observed termination and the
unattempted denominator checks remain unchanged. No production collector,
evaluation case, expected outcome, acceptance threshold or frozen evidence is
modified. The combined offline Node suites pass **28 tests, one native-only
skip**, in 14.463 seconds. Exact-head CI for this test-only correction remains
the publication gate; the earlier failed workflow is not relabeled a pass.

After release-preflight binding, at most one separately approved final live
batch should lead to a release/no-release decision. Do not start an automatic
next-parser cycle, reuse exhausted budgets, count retries as fresh cases, or
weaken the requirement for faithful extraction and all five useful bundles.
Failure means the natural-language goal remains unmet and needs user direction;
it is not permission to relabel the deterministic-only capability as completion.
