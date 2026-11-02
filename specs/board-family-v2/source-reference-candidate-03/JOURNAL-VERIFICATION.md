# Independent evidence journal — offline verification

Date: 2026-09-14. Local, uncommitted experimental indexed-selector work. This
checkpoint does not adopt the candidate, change the frozen evaluation, or grant
live request authority. The released command still uses the typed-v2 selector.

## Result

The experimental selection path can now retain request/response evidence and a
settled selection independently of native generation. The command integration
test uses this path for both board families. A generation error cannot remove
that separate journal; a journal error withholds the configuration and returns
an error before native work.

This is process-failure evidence retention, not evidence that the model's
interpretation is correct, that a manufacturing job completed, or that a PCB
works physically. The known semantic counterexample remains in the suite.

## Checkpoint contract

`InterpretReferencedWithJournal` requires an explicit transport and a new,
separately owned evidence directory. Existing files, directories and symlinks
are not replaced. The one-request reservation, exact endpoint/model, bounded
payload, timeout, no-redirect and no-retry guards remain in force.

| Checkpoint | Meaning and limitation |
| --- | --- |
| `start.json` | Original request, experiment identity and budget policy were recorded. This is not transport authority or evidence of a request. |
| `request/body.bin`, `request/receipt.json` | Exact request body and hash were published and synced before the base transport was called. No authorization header is retained. |
| `response.bin`, `response/receipt.json` | The bounded response prefix was written, synced, closed and compared with the in-memory bytes. Receipt flags distinguish EOF, truncation and transport/read/close errors. A receipt does not imply provider success. |
| `selection/selection.json`, `ledger.json`, `receipt.json` | Selection, available wire evidence and accounting snapshot were retained before generation. The receipt explicitly says `selection-recorded-not-process-terminal`; it does not certify child exit or a completed board. |

Response bytes remain base64 in selection JSON, including malformed JSON and
invalid UTF-8; `response.bin` preserves the same raw bytes or explicitly partial
prefix. The response limit is 2 MiB. Source quantities and raw structured output
remain distinct from the original HTTP bytes. Request headers, credentials and
arbitrary response headers are not archived.

Directory identity and private permissions are checked. Journal files are 0600
under a 0700 root. Checkpoint publication uses existing no-replace atomic
directory publication, but the response spool is outside disposable staging.
This is not a claim of protection against an actively hostile same-user process
mutating filesystem paths concurrently.

## Failure behavior

- Request-recording failure stops before the injected base transport. Because
  reservation occurs first, the attempt can remain fully reserved even when
  `transport_started` is false. There is no automatic refund or retry.
- Response write, short-write, sync, close and checkpoint-publication failures
  return `evidence_write_failure`, withhold the configuration and preserve
  whatever spool/checkpoints were already written. No successful receipt is
  fabricated for a failed spool write.
- Where complete in-memory terminal metadata exists, accounting can still retain
  known usage even if disk persistence subsequently fails. Missing/unverifiable
  usage is unknown, not proof of zero cost. Full reservations remain unchanged.
- A disputed ledger snapshot is retained when readable, alongside a failed
  selection and `ledger_joined: false`. It is not silently discarded or used to
  authorize generation. Snapshot reads reject symlinks, nonregular files,
  oversized/ambiguous JSON and inconsistent policy, identity, status or cost.
- Abrupt process exit leaves prior checkpoints and already-written spool bytes.
  Unsynced partial spool bytes are **not guaranteed to survive power loss**.
  A missing response/selection receipt means incomplete evidence, not a
  completed request or permission to resume spending.
- Selection-write failure can occur after an atomic rename but before its
  directory sync reports success. A visible file alone is therefore never a
  terminal-process or successful-persistence certificate. A collector must use
  the actual process result and independently verify evidence joins.

## Verification observed

All provider tests use dummy keys and explicit in-memory RoundTrippers, not
sockets. Real provider credentials and live-test enablement were removed from
test commands. Go module downloads were disabled; repository caches were used.

- New journal tests cover a successful joined selection; ten storage/accounting
  faults; seven ownership/preflight cases; and nine ledger snapshot cases,
  including a valid baseline plus independent status/cost tampering.
- The existing 21 response modes now run both with and without the journal.
  This checks actual retained bytes for success, malformed extraction, refusal,
  incomplete/failed provider responses, HTTP/transport/read/close failures,
  missing/disputed metadata, duplicate fields, oversized/trailing streams and
  invalid UTF-8. Successful disk capture does not turn those failures into passes.
- Three real subprocess tests exited with expected terminal codes: 71 after
  request publication, 72 after a partial response write, and 73 after settled
  selection publication. Parents verified prior files and ledger reservations.
  These are controlled process exits, not simulated power cuts.
- Fourteen in-process command cases cover all five supported family/profile
  combinations, clarification, refusal/unsupported paths, extraction/evidence
  errors, generation failure and validation failure. Generation checks that the
  journal already exists and that all four provider-key variables are absent.
- Full short tests for affected packages passed: boardfamily **6.343 s**,
  aiprovider **0.462 s**, command **2.797 s** (exit 0).
- Race checks passed: boardfamily **11.635 s**, aiprovider **3.028 s**, command
  **3.852 s** (exit 0). The last small test-only change adds the valid ledger
  baseline and independent status/cost cases; all nine cases separately passed
  in **0.930 s**, exit 0. They are not retroactively included in these timings.
- Real KiCad command checks passed BMP280 standard **6.15 s** and SHT31 standard
  **4.28 s**, **14/14 native/export gates each**, no output repair; total command
  **10.668 s**, exit 0. Responses were synthetic; temporary bundles were not
  republished as a new historical qualification.
- Repository lint returned **0 issues**. The first invocation returned exit 0
  but emitted sandbox cache-write warnings; a terminal repeat using
  `GOLANGCI_LINT_CACHE=$PWD/.cache/golangci-lint` returned **0 issues** without
  those warnings. No permissions were broadened.
- Read-only final-02 authentication passed: all 14 historical attempts, 203
  publication files, five CI addendum files and 81 native deliverables remain
  unchanged. Historical scores remain raw **5/14**, application **7/14**,
  complete **5/14**. Complete acceptance is still not met.

The initial journal test run had five failed equality assertions because
`json.MarshalIndent` reformatted the `json.RawMessage` structured field. The
test now compares that field as JSON, while separately checking byte-exact
request/response capture. No raw HTTP byte assertion was relaxed.

The earlier unfiltered short repository test run (session 89919) ended exit 1
at the unchanged open-topology package's default ten-minute aggregate timeout;
see [the corrected earlier record](EVIDENCE-COMMAND-VERIFICATION.md). It is not a
pass. A separate existing `make test-fast` tier was launched with `-count=1`,
four package workers and the repository's twenty-minute timeout. That tier
explicitly excludes its six pre-existing heavy tests; it is not full acceptance.
Session **72228 terminated with exit 0**; the open-topology package completed in
**263.091 s**. This verifies the pre-command-mode journal checkpoint under that
fast tier, not subsequent command/collector changes or the six excluded tests.
It was not a restart of the timed-out full run.

## Remaining goal work

The subsequent [command checkpoint](COMMAND-VERIFICATION.md) adds the explicit
experimental mode and actual entrypoint/process checks. The remaining collector
requirements below are still open.

The immutable journal closes one evidence-loss gap, not the overall goal.
Next qualify a standalone experimental executable and a bounded collector that
records terminal process outcomes, preserves every measured bad extraction,
stops on unsafe transport/accounting/persistence states, and measures total wall
time. It must not infer safe continuation from a lock file or checkpoint alone.
The released one-command selector and published CI do not yet include this
local candidate. Complete review and fresh exact-source verification remain.

Any final live evaluation still needs a separately frozen runtime/corpus and
explicit new request/dollar approval. Final-02 has no remaining request slots;
unused dollars are not new request authority. Physical fabrication and bench
bring-up remain separate. No API, Gemini, firewall, screen-control, commit,
push or PR mutation occurred during this checkpoint.
