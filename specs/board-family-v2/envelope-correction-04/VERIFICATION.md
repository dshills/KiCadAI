# Offline envelope-boundary correction 04

Date: 2026-09-15. Status: **local offline correction; not live acceptance**.

The successor parser accepts the unchanged captured API envelope that stopped
indexed evaluation 03, while the existing admission code still rejects its
invented sensor fact. No new provider call, recovery batch, ledger repair,
native board output, default-protocol switch, or semantic-success claim follows.

## Changes

- Keep `validateIntentJSON` at depth **12**, with its existing error text and
  duplicate-key/UTF-8/trailing-content checks.
- Share that strict token walker with a separate provider-envelope entry point,
  bounded at depth **64**. Use the envelope entry point for stream events and
  terminal response objects. The recording transport's response-byte bound is
  unchanged. This handles schemas echoed inside the surrounding API envelope
  without widening the model-authored intent-output contract.
- Preserve the underlying validation cause in envelope errors instead of
  reporting only an ambiguous generic error.
- Add a regression using the **exact captured request and response**, pinned
  by SHA-256. The fixture is read from the committed failed-batch archive;
  it is never edited, stripped down, or turned into a corrected model answer.

The [official streaming-event reference](https://developers.openai.com/api/reference/resources/responses/streaming-events)
confirms that created, in-progress, and completed events contain a surrounding
`response` object. The observed depth of 13 comes from this captured schema,
not a documented universal API depth. The limit of 64 is this application's
bounded envelope policy, not an OpenAI guarantee.

## Verification

The new captured-envelope tests first failed against the unchanged parser with
`recorded stream event JSON is ambiguous or invalid`; the existing intent-depth
test passed. After correction, the focused run passes all four top-level tests
and seven negative subtests (11 test/subtest results):

- Captured full stream and terminal metadata: accepted by the successor parser.
- Intent depth 12 accepted; depth 13 still rejected.
- Envelope depths 12, 13, and 64 accepted; depth 65 rejected.
- Duplicate root, nested, and deeper-than-12 keys, invalid UTF-8, trailing JSON,
  and truncated JSON remain rejected.
- A fresh **offline, in-memory transport replay** traverses selection, temporary
  ledger, durable journal, and journal audit. It makes exactly one synthetic
  transport invocation. Outcome is `invalid_extraction`, with no configuration:
  `fact 0: sensor identity is not grounded in the quoted request`.
  All four original facts, including the incorrect wireless requirement,
  remain unchanged. Replaying saved bytes is not a new model trial or a repair
  of the historical result.

Affected packages passed using cached Go **1.26.8**, with network/module
downloads disabled and real provider credentials removed:

| Package | Short tests | Race + short tests |
| --- | ---: | ---: |
| `internal/boardfamily` | 10.326 s | 14.640 s |
| `internal/aiprovider` | 0.644 s | 2.581 s |
| `cmd/kicadai-board-family` | 5.396 s | 7.646 s |

Full `golangci-lint run ./cmd/... ./internal/...` completed with **0 issues**
after both `PATH` and `GOROOT` were set to the pinned compiler. An initial lint
invocation mixed the machine's Go 1.27.1 executable with Go 1.26.8's root and
failed type checking; that environment failure is not a source-code finding.
A corrected environment invocation is the passing result reported here.
The initial test scaffold's nonexistent ledger `Usage` field was corrected to
the actual input/output counters before the red/green regression run.

`gofmt` and `git diff --check` passed. Native generator, family catalog, board
geometry, footprints, routing, reviewed examples, model, extraction prompt,
and output schema are unchanged. No blanket native requalification was rerun
or claimed for this JSON-boundary-only correction.

## Historical boundary and location

The failed batch, its source-bound review, and CI completion addendum were
committed locally as `a0d719cce11c51fa6c510058a73869900a5bae7f` on
`codex/board-family-v2`. The frozen runtime still evaluates source `8744d7fc`;
the later commit contains evidence only, not a changed evaluated executable.

This correction is developed separately on `codex/indexed-envelope-04`, rooted
at `.cache/board-family-v2/development-04`. The original checkout and frozen
runtime/source dependencies are untouched. Its original offline failure
authenticator still passes: 22 archived batch files, one physical request,
13 unattempted cases, 0/14 complete passes, no native bundle, and the unchanged
USD 0.05 reservation. No historical usage was settled or refunded.

The original manifest references the original checkout's source paths. Keep
that checkout intact until historical source verification is made independent
of the development checkout; an isolated worktree prevents this correction
from invalidating those existing checks. This is not a claim that the current
archive is fully portable to another machine.

These new changes have local test/lint evidence, **not published exact-source
CI or live semantic acceptance**. Prior green CI at `8744d7fc` does not cover
the successor code. No new PR, remote push, or merge is included in this record.

## Remaining goal work

The independent semantic defect remains: feature validation recognizes allowed
feature names, but does not establish that an affirmative wireless requirement
is true of a wired-monitor request. Rejecting the invented sensor safely does
not satisfy the supported request, and accepting the transport does not fix
model fidelity. Do not repair facts, loosen the frozen scoring criteria, or
restart the failed batch. Address semantic reliability and integration/review
before proposing any further live evaluation. Physical bring-up remains
separately authorized work. The overall goal is not complete.
