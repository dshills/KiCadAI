# Protocol v2: approved five-minute provider request deadline

The user approved a versioned timeout-only revision and one new recorded
baseline on 2026-09-11. The approval is recorded in
`protocol-v2-authorization.json` before any v2 live request.

## Sole behavioral change

The experimental evaluator's HTTP request timeout changes from 120 to
300 seconds. This applies to its existing initial, permitted correction and
clarification requests. The same fixed model, body, output-token limit and
provider request semantics are retained. Production OpenAI client defaults
are not changed.

The inline client construction is factored into a small tested helper without
altering its recording transport, redirect refusal or cookie behavior. The
original timeout was explicitly supplied by the evaluator, not merely inherited
from the production default. No compiler, electrical, schematic, placement,
routing or native-validation logic changes.

The v2 supervisor routes the existing worker's `--freeze` option to the
versioned seal, requires coverage of the added protocol files, and records
`protocol=v2` and `provider_request_timeout_seconds=300`. Its serial execution,
correction policy, case order, resource monitors and stopping rules are unchanged.

## Versioning and historical preservation

`freeze-v2.json` is a new seal; `freeze.json` is not overwritten. The seal
serialization keeps the existing `kicadai.practical-board-freeze.v1` schema
because its format is unchanged; protocol versioning is explicit in its filename,
this document, authorization and campaign metadata.

The v1 source, original evaluator and prior authentication scripts are preserved
at commit `ea3eef2c5aeed972d6ded2cacca0eaf2ece1360c` and in the detached checkout
`/tmp/kicadai-practical-protocol-v1-source`. Run the original
`authenticate-recovery-1.mjs` and `authenticate-recovery-2.mjs` from that checkout:
they intentionally verify v1 engine bytes, not the revised current engine.

All original acceptance documents, corpus and environment files, original seal
and supervisor, historical V18-V23 evidence, three interrupted raw cohorts,
archives, receipts and reports remain unchanged. The revised current evaluator
does not claim to satisfy the old seal. A worker built with the v2 seal rejects
the old default seal before a live request.

## Identical acceptance and remaining budgets

The fixed eight positives, four refusals, two clarifications and two paraphrases
retain their exact bytes and all 108 acceptance clauses. Public-frozen reserved
cases remain excluded from implementation tuning. The six-complete-positive,
two-distinct-uplift, preservation, refusal, clarification, readability,
electrical, native and deterministic-replay criteria are unchanged.

Other bounds remain exactly as frozen: gpt-5.6-sol; 16,384 output tokens;
131,072 request bytes; no automatic provider retries; only the existing bounded
structured-output correction; serial cases; 20-minute case / two-hour phase
caps; 16 GiB sampled process-tree RSS; 10 GiB raw evidence; 36 baseline / 72
total provider requests; USD 50 estimated/reserved spend.

The new evidence root is
`/tmp/kicadai-practical-sensor-controller-public-1-protocol-v2`.
Copy receipts 001, 002 and 003 and their usage records byte-for-byte from
recovery 2 into its cumulative journal, exactly once. Retained estimates total
USD 1.368288; actual billing remains unknown. Remaining headroom is 33 baseline
requests, 69 total requests and USD 48.631712. The first new reservation is 004.
No prior raw directory or journal is reopened or overwritten.

## Evidence and comparison

The original three interrupted cohorts remain separately disclosed: two failed
before HTTP and one received an incomplete HTTP-200 stream before the old request
deadline. None is a complete design baseline; no partial JSON is repaired or
promoted into an accepted requirement.

Only the authenticated v2 baseline can drive a production scope decision. Any
later originally planned final and paired comparison must use this same v2
evaluator seal and cumulative journal. No production fixes are selected until
nonreserved baseline failures demonstrate a credible path to two distinct
complete uplifts under at most three generic changes.

Tests prove the approved helper's timeout/transport/redirect configuration,
reconstruct the original engine bytes after removing the timeout-only patch,
and verify that every other original frozen file is byte-identical. A second
test reconstructs the v2 supervisor solely from its declared seal-routing,
coverage and metadata changes. These are protocol checks, not corpus passes.

## Reproduction and stopping

Build the worker from a clean recorded source commit with the SHA-256 of
`freeze-v2.json` embedded in
`kicadai/internal/practicalboardeval.FreezeSHA256`, using the same pinned Go
toolchain. Run `run-campaign-v2.mjs <binary> <root> <campaign-root> baseline`
with the original pinned native environment. The frozen supervisor enforces
identity and resource checks before cases. Do not use the old v1 supervisor
with a v2 worker.

This approval authorizes one newly recorded baseline, not an unlimited
recovery loop. Five minutes is not a guarantee of completion. Any terminal
provider/resource failure stops the campaign. No additional run, acceptance
relaxation, budget increase, firewall modification, merge, release, fabrication
approval or stable support expansion is authorized.
