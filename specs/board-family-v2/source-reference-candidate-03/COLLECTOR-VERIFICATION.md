# Bounded complete-outcome collector — offline verification

Date: 2026-09-14. Local implementation checkpoint, not final evaluation approval.
The candidate remains experimental; the default selector is still typed-v2.

## Result

The new collector completed a seven-case offline integration run through the
real Go command entrypoint, request builder, journal, read-only auditor and
ledger. It preserved three bad model answers as failed cases, recorded all seven
outcomes, and compared both real native board bundles with their reviewed
examples. Collection wall time before final result publication was **9.820 s**.

Only HTTP transport was replaced in the Go integration helper. These were
synthetic responses and test ledgers, **not live requests, measured AI accuracy,
provider attestation, actual expenditure or new physical qualification**.

## What changed

`collector.mjs` implements one fresh, sequential, bounded batch. Importing it
does not read credentials, spawn a child or start an evaluation. Its CLI offers
read-only `--check`, credential-stripped `--offline`, and a separately guarded
`--live` path. No live invocation was made.

The manifest binds the command binary, Node runtime, collector and its imported
source dependencies. It fixes the ordered case prompts, request/dollar policy,
per-case and total deadlines, and qualified native tools/examples. Live mode
rejects test prefixes and synthetic qualification. It also requires a matching
explicit approval record bound to the manifest hash and exact request/dollar
limits; a policy file alone does not authorize spending. An approval file is
not cryptographic proof of user intent: the operating agent must still obtain
and accurately record the user's actual new approval before invoking live mode.

Each case records its prompt and attempt identity before launch. The collector
then waits for the actual child `close` event, retaining exit code, signal,
timeout/spawn/log-overflow flags and elapsed time. Logs are bounded to 1 MiB per
stream. Timeout or overflow kills the owned POSIX process group and still waits
for its terminal event. Log persistence errors stop continuation. Existing logs,
batch roots and outcomes are not overwritten or automatically removed.

After a clean command termination, a second credential-stripped child runs
`kicadai-board-family --inspect-indexed-journal DIR`. The Go auditor:

- requires the expected private regular-file inventory and bounded reads;
- verifies receipts, hashes, source inventory and request/response metadata;
- reconstructs the exact request with the current runtime and replays only the
  recorded response through an in-memory transport;
- recomputes the local extraction outcome and decision, rather than trusting
  saved success flags;
- checks complete, unique, bounded accounting history and policy joins.

The audit is local byte/contract consistency checking. It makes no network
request, reads no real key, and cannot prove provider authenticity or English
semantics. It does not infer that the original process exited or that native
generation succeeded; the collector checks those separately.

## Continuation rules now implemented

| Verified case outcome | Collector behavior |
| --- | --- |
| Complete accounted response, `invalid_extraction` or provider refusal, expected exit 1, no board | Record a model failure and advance to the next unattempted case. Never retry or count it as correct. |
| Decision, exit 0, correct output shape; supported output matches a reviewed native bundle | Retain for semantic review. A structurally valid but wrong answer is not automatically a pass. |
| Disputed/incomplete transport, model/usage identity, journal, runtime, accounting or process state | Stop without retry; retain partial evidence and list every unattempted case. |
| Generation/validation failure after a supported selection | Preserve the journal and partial output; stop. A completed API response is not a completed board. |

The ledger prefix must remain unchanged and grow by exactly one verified
reservation for an accepted case record. Response IDs must be unique. Full
reservations are not refunded. A fresh batch root is exclusive, so repeating an
invocation cannot reset history or silently resume after a crash. No resume mode
was added. A leftover file or PID is never treated as proof that a child stopped.

Metrics distinguish prepared cases, observed launched children, recorded
outcomes and recorded model failures. All planned cases remain explicit, even
when stopped. The result is always marked `not-established-by-collection` for
acceptance. `total_wall_seconds` in `result.json` ends immediately before final
result publication; CLI stdout timing also includes that publication, and the
outer process observation includes the complete CLI lifecycle.

## Verification observed

All commands removed real provider keys and live-test enablement. Go downloads
were disabled and workspace caches reused. The credential safety skill reused
the existing user decision without revealing or persisting the key.

- **15 Node collector tests passed**, 0 failures, **5.265 s**. These cover
  continuation after recorded failures, eight stop classes, changed binaries,
  rejection of live use with synthetic qualification, real spawn failure,
  existing-log protection, the standalone collector CLI and read-only preflight.
  Their dedicated JavaScript child is explicitly synthetic and cannot pass the
  real Go journal auditor; these tests alone are not end-to-end evidence.
- **7 real-Go collector integration tests passed**, 0 failures, **12.994 s**.
  One batch recorded five non-design/failed-extraction outcomes. Five separate
  runs stopped correctly on transport error, missing model, abrupt exit,
  generation conflict and validation failure. The seven-case native run recorded
  **7/7 outcomes**, **3 model failures**, and both reviewed native bundles in
  **9.820 s** before final publication. There were no manual resumes or repairs.
- The integration executable was built locally with `go test -c` at
  `.cache/board-family-v2/indexed-collector-03.lNm1wb/board-family.test`.
  Tests call the real `main()` and replace only HTTP transport. This is not a
  production binary making real provider calls. The production CLI has no
  fixture-response option; live collection forbids the test entrypoint prefix.
- Go audit tests cover **nine tamper cases** and **two prior-response identity
  cases**, including a valid baseline, invented decision/outcome, consistently
  rehashed but changed request, duplicate JSON, public/missing/extra/symlinked
  files and repeated response IDs. The existing 21 response-mode matrix also
  exercises the auditor for both acceptable recorded outcomes and unsafe ones.
- Affected-package short tests passed: boardfamily **9.399 s**, command
  **5.280 s**, aiprovider **0.462 s**, exit 0.
- Race checks passed: boardfamily **12.782 s**, command **7.692 s**, aiprovider
  **2.616 s**, exit 0. Full repository lint: **0 issues**, exit 0.
- Frozen final-02 publication authentication passed unchanged: **14 historical
  attempts**, **203 publication files**, **five CI addendum files**, two native
  bundles and **81 compared deliverables**. Historical complete acceptance is
  still **5/14**, not rescored by this new implementation.

The earlier `make test-fast` pass and unfiltered test timeout remain separately
documented. They predate this collector/auditor revision and are not presented
as a full exact-source acceptance run for these changes.

## Review and remaining work

This is an implementing-agent review and offline verification checkpoint, not
an independent external review. No API/Gemini request, firewall/screen action,
commit, push or PR mutation occurred. No historical evidence was modified.

The next step is to finish final evaluation preparation: the candidate-specific
source-bound semantic scoring/review artifacts, a separately frozen production
runtime/corpus, complete review and exact-source qualification/CI integration.
Then obtain explicit new request/dollar approval for one small live evaluation.
No request slots remain in final-02; unused dollars do not restore them.

The known semantic counterexample remains. Reliable collection makes failures
observable and avoids unnecessary manual restarts; it does not make those
answers correct. Production adoption and the overall two-family goal remain
unproven until actual language outcomes meet the full acceptance requirements.
Physical fabrication and bench bring-up remain separately authorized.
