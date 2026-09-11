# Recovery baseline 2: authenticated stream, incomplete generation

Date: 2026-09-11. Status: **incomplete; no design-capability score**.

The approved second recovery reached OpenAI and received HTTP 200 for the fixed
`gpt-5.6-sol` model. Unlike the two earlier TCP failures, this request confirms
the existing key and model were accepted for this recorded call. It streamed
output, but the unchanged two-minute client deadline canceled the request before
a terminal response event or complete structured requirement arrived.

The frozen supervisor stopped the campaign. No provider retry, correction,
timeout change, model change, acceptance change or firewall modification occurred.
This is a provider-completion failure, not a compiler, electrical or PCB failure.

## Observations

- Request 003 started at 2026-09-11T10:33:51.769367Z. Generation-call time was
  120.023462833 s, ending with `net/http: request canceled`; the provider wrapper
  reported `ai_provider_timeout`. The existing production client defaults to
  `openAIHTTPTimeout = 2 * time.Minute` in `internal/aiprovider/openai.go`.
- Retained HTTP response: 1,060,419 bytes of server-sent events, with status 200
  and content type `text/event-stream; charset=utf-8`. The request identifier is
  retained in the hash-bound response metadata for troubleshooting.
- The captured stream contains 3,830 contiguous events, sequence 0 through 3829,
  including 3,818 output-text deltas totaling 16,811 bytes. Both response
  envelopes identify `gpt-5.6-sol`, status `in_progress`, with no usage or error.
- There is no `response.completed`, `response.failed`, `response.incomplete` or
  error event. Every retained event parses, but the assembled output text is
  incomplete JSON. The capture is an incomplete stream, not a completed answer
  merely missing an archive file. Partial output is not repaired, accepted,
  compiled or used to select production improvements.
- P01 worker exited 1 after 120.127 s. Peak sampled process-tree RSS was
  34,095,104 bytes at 1 s intervals, with zero sampling errors and no resource-cap
  kill. Campaign wall time, including the environment snapshot, was 132.993 s;
  its terminal record was written at 2026-09-11T10:35:51.843Z.
- P02–P08, N01–N04, C01–C02 and W01–W02 are all `not_run`. No complete provider
  requirement, design proposal, electrical analysis, schematic, board, native
  corpus validation or downstream replay was produced. No final or paired
  campaign occurred.

The denominator remains eight positives. Complete baseline success rate is
unavailable, not a measured 0/8 design-engine result. All 108 frozen acceptance
clauses and all applicable design gates remain `not_run`. The six-complete-board
and two-distinct-uplift criteria cannot be assessed from this partial campaign.
No production improvement scope is selected, and reserved cases remain unused
for tuning.

## Lineage, resources and assistance

Authorization was committed before dispatch as `2924c13e`, with full scope in
`RECOVERY-2.md` and `recovery-2-authorization.json`. The raw tree contains an
identical authorization copy. The campaign used the exact previously
authenticated binary, SHA-256
`45e6c24392f1ba3469e5a6b9d4cd6857eb617bfa39de59c1ee87294660fe0ee3`,
against its matching clean detached source commit
`4341df799c25c811cd05e80f5d860c009ea50585`. Both current and detached source
copies match all 18 frozen files; baseline production remains merged V23.
The unchanged supervisor verified the original tool, library and policy identities.

Receipts 001 and 002 were carried byte-for-byte from recovery 1, counted once
each. Request 003 retained a further USD 0.456096 because final token usage was
not received. Cumulative estimated/reserved spend is USD 1.368288 across three
requests. **Actual billing and token usage are unknown**; HTTP 200 and partial
generation must not be represented as free or unbilled.

Original limits remain 36 baseline requests, 72 total requests and USD 50
estimated/reserved spend. Remaining headroom is 33 baseline requests, 69 total
requests and USD 48.631712. This headroom does not authorize another attempt.
There were no manual circuit/layout repairs. Two separate recovery approvals
and the initial key-reuse approval are disclosed; human-active minutes and
separately timed review minutes remain unmeasured.

## Authentication and retention

`recovery-2-evidence-inventory.json` binds all 31 raw files, totaling 62,965,699
bytes, to SHA-256 and byte count. The P01 internal inventory exactly matches its
13 payload files. The 28,008-byte request is byte-identical to requests 001/002
and matches receipt 003. The strict schema, model, output cap, streaming,
`store=false`, `background=false` and absence of tools were verified.

`recovery-2-audits.json` binds every outcome to the relevant evidence and retains
all 108 full frozen acceptance clauses. Its HTTP, authentication and model-access
fields now reflect this response rather than inheriting the earlier no-response
classification. An exact-secret scan of all raw files, including the partial
stream and decompressed library index, found no environment API-key value.
Authorization headers were never captured.

The read-only `authenticate-recovery-2.mjs` reproduces the file, journal, stream,
cohort and clause checks without any API request or file write. It also verifies
the unchanged 25-file original baseline and 28-file recovery-1 trees. Review is
disclosed self-review and local hash authentication, not third-party certification.

Raw tree: `/tmp/kicadai-practical-sensor-controller-public-1-recovery-2`.
The separate ignored archive is described by `recovery-2-archive.json`:
63,031,296 bytes, SHA-256
`58aad6b57e9b077b4e0724b36f32ca40bf31132333053ecbdb5b57fba97f6c7f`.
All 31 archived payloads verify exactly; there are no duplicate, extra or missing
payloads. Original trees and archives remain retained. Neither the evidence bundle
nor its archive has been uploaded for remote review, so retention is local,
not a remote reproducibility guarantee.

## Scope decision: protocol amendment requires approval

Recovery 2 is terminal. Connectivity, key acceptance and access to the fixed
model are demonstrated for this call; completing generation within the frozen
two-minute request cap is not. Replacing the key or rerunning the identical
protocol without a new scope is not justified.

A concrete next proposal is a separately preregistered protocol revision that
raises only the provider request timeout from 120 to 300 seconds, while retaining
the corpus, model, output-token and attempt limits, electrical/physical/readability
gates, 20-minute case cap, two-hour phase cap and original cumulative spend limit.
This is **not yet authorized or implemented**, and five minutes is not a promise
of completion. It would require a new versioned evaluator seal and one explicitly
approved new baseline; any later final comparison must use that same revised
protocol. All three interrupted cohorts and their reservations must remain visible.
No partial output from this run may be promoted into an accepted input.

The requested capability milestone remains unachieved. No production or stable
support boundary changed, and no PR, merge, release or fabrication approval was
created by this recovery. Existing offline regression and native reference results
remain separate controls, not substitutes for the missing live corpus results.
