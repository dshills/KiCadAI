# Recorded responses and command integration — 2026-09-14

Status: **offline implementation and command-path checks; not live acceptance**.
This follows the completed [provider/generator checks](PROVIDER-VERIFICATION.md).
That earlier turn was progress, not a blocked wait. This continuation adds
bounded response evidence, explicit metadata validation, and in-process testing
of the candidate through the actual board-family command flow.

The work remains local on `codex/board-family-v2`, based on published head
`f8ac5099c2e2d21a72a2faf2bb2161ffbec8792a`. The command source changed to permit
test injection. Its default factory still selects the existing v2 interpreter,
generator and validator; exported default live contract and CLI flags remain
unchanged. The historical source/runtime/evaluation archives were not edited.
This is not the exact published binary or a new live-qualified successor.

## What changed

- The candidate transport records the actual encoded request body and the body
  bytes consumed from its HTTP response. Request headers (including the key) and
  arbitrary response headers are not recorded. Body fields are base64 in JSON,
  preserving malformed JSON and invalid UTF-8 exactly. SHA-256 fields bind those
  local byte arrays; they are not provider signatures.
- Response retention is bounded at the existing **2 MiB** candidate response
  limit. EOF, truncation, bytes consumed, transport/read/close errors and HTTP
  status remain distinct. Over-limit responses keep a prefix explicitly marked
  truncated; interrupted reads never claim a complete response.
- `RecordedOpenAITerminal` is a no-I/O wrapper around the existing provider's
  stream validator. The candidate additionally rejects duplicate JSON fields in
  all recorded events and the terminal response before inspecting metadata.
  Missing model, response identity, usage fields or inconsistent usage cannot
  inherit defaults or become an accepted configuration.
- Completed malformed extraction and provider refusal now retain verified
  terminal model/ID/usage and completed accounting, while staying failed user
  outcomes. Genuine incomplete responses retain metadata and have a distinct
  outcome; their ledger status remains `failed_or_unknown`. Invalid/partial
  response evidence keeps the complete reservation without treating missing
  usage as a known cost. There is no retry, budget refund or collection loop.
- The actual CLI flow has an injected pipeline for offline tests. The default
  production factory is unchanged. Tests inject the candidate, exercise prompt
  file/policy/ledger handling, clear credentials before native work, and save the
  full candidate record to `selection.json`. A defensive guard rejects a
  supported selection with a nil configuration rather than dereferencing it.

This uses the official distinction between stream deltas and a terminal response
event. Stream termination and successful extraction are separate facts; receiving
some text does not establish a completed result.
[OpenAI streaming guide](https://developers.openai.com/api/docs/guides/streaming-responses),
[Responses API examples](https://developers.openai.com/api/reference/typescript/resources/responses/methods/create).

## Observed tests

- Existing provider tests passed after adapting their expected diagnostic
  outcomes: **0.944 s**. The first run failed three now-stale outcome
  expectations (exit 1); they were updated to reflect retained
  completed metadata. The old `incomplete` fixture had emitted a completed event
  with an incomplete status; it was corrected to a genuine incomplete event.
  A separate new status-mismatch case preserves coverage of that invalid stream.
- **21 recorded-response cases** pass, including completed/non-streaming JSON,
  malformed output, refusal, incomplete/failed responses, partial reads, close
  failure, excessive bytes, HTTP/transport failures, missing metadata,
  inconsistent usage, duplicate keys/event types, extra terminal data, mismatched
  status and invalid UTF-8. Each case reaches the in-memory transport once,
  preserves the expected bytes/prefix through JSON serialization, and verifies
  the resulting reservation/usage/outcome. Five additional tamper cases reject
  altered request/body hashes, byte length, EOF and close status.
- Combined candidate provider and evidence tests passed: **1.492 s**, exit 0.
- **13 command-path cases** passed: all five supported family/profile pairs,
  clarification, unsupported heater, malformed output, invalid extraction,
  refusal, missing model, broken stream and native-validation failure. The test
  checks credential clearing, saved evidence, JSON stdout, single request, and
  whether generation/validation were called. Non-design and provider-error paths
  emit only `selection.json`, not a board. **0.690 s**, exit 0. These tests use a
  validation stub; the separate native check below does not.
- Installed KiCad reports **10.0.3**. The native opt-in command test passed for
  BMP280 standard (**4.39 s**) and SHT31 standard (**3.94 s**), using the real
  generator and validator/exporter: **14/14 gates per family**, selection evidence
  saved before validation, non-empty BOM/previews/manufacturing manifest, and no
  manual repair. Total test command: **8.550 s**, exit 0. Provider responses remain
  synthetic. Temporary test bundles were not published as a new qualification.
- Race tests passed for all three affected packages: boardfamily **7.837 s**,
  aiprovider **2.363 s**, command **2.748 s**. Targeted lint passed with **zero
  issues**. Full repository lint (`./cmd/... ./internal/...`) also passed with
  **zero issues**, and tracked diff whitespace checks passed.
- The broad `go test ./... -short -count=1 -p 4` regression used credentials
  removed and module downloads disabled. Session **89919 terminated, exit 1**:
  the unchanged `internal/opentopologysynthesis` package hit Go's default
  aggregate ten-minute timeout (600.393 s), while
  `TestPowerTransferCandidatesReachTrustedSimulation/efficient_audio_power_stage`
  was running. This is not a full-suite pass or proof of a functional regression
  in the changed packages. The command omitted the repository's twenty-minute
  test timeout; it was not restarted or silently reclassified as successful.
- Read-only frozen final-02 authentication passed after the command refactor.
  The original failed scores, all 14 attempts, 203 publication files, five CI
  addendum files and the two historical native bundles remain unchanged.

All commands removed real provider credentials and disabled live-provider tests.
The API tests supply explicit dummy keys and in-memory RoundTrippers; module
downloads were disabled and existing repository caches reused. No OpenAI/Gemini
request, screen interaction, firewall change, commit, push or PR mutation occurred.
The actual native checks used only the existing local KiCad installation.

## Remaining boundaries and review

The subsequent [journal checkpoint](JOURNAL-VERIFICATION.md) implements and tests
independent pre-generation persistence. The following describes the earlier
in-memory checkpoint and why that follow-up was required.

Implementing-agent review only. These checks exercise the existing command's
in-process control flow, not a separately launched candidate executable and its
OS exit/signal behavior. There is no user-visible candidate switch yet. A future
versioned runtime must deliberately select it; the old frozen runtime must not
be reused for the new payload.

The recorder currently retains bytes in memory until the command saves its
selection. A crash, output-creation failure or generation error before that save
can still prevent durable capture. Filesystem write/sync/close failure and
ownership of partial generation outputs need an explicit durable evidence design
and fault-injection tests before unattended collection. Do not claim the new
recording is crash-safe, complete on disk for every terminal process, or ready to
auto-resume evaluation. The shared ledger does not by itself decide whether the
next case may run after an unknown outcome; a collector must enforce that rule.

Storage review identified existing helpers to evaluate, not blindly reuse:
`internal/atomicdir.Publish` performs no-replace publication but deletes failed
staging trees, so it must not own the only copy of failed provider evidence.
`internal/atomicfile.Write` syncs data and its parent directory but replaces the
destination, so it belongs only behind explicit journal ownership/inventory
checks. Keep the evidence journal independent of disposable build staging. A
completed-evidence marker must follow successful body/metadata persistence, not
just an in-memory provider return; interrupted captures remain visibly partial.

The generic provider parser's earlier data-loss behavior remains unchanged for
the released v2 path. The candidate recovers verified metadata from its own
recorded response; raw malformed extraction remains in the response bytes rather
than being forced into `json.RawMessage`. Invalid response evidence is not used
for configuration. These are application-visible/decompressed body bytes, not
TLS or on-the-wire packet archives, and local hash consistency is not independent
provider attestation.

Semantic fidelity remains unproven. The known indoor-background/custom-geometry
counterexample is still retained. Correct references and valid JSON do not prove
meaning, exclusion strength, temporal requirements or complete extraction. These
known synthetic fixtures are not an unseen live holdout or a statistical estimate
of reliability, speed or useful-board success.

Next: make evidence durable before native generation, add terminal-process and
storage-failure checks, and finish a separately versioned command/collector
contract. Then complete exact-source regression/review/CI, freeze the candidate
and evaluation, and obtain explicit new request/dollar authority for any live
run. No old budget has remaining request slots. Physical bring-up still needs a
separate authorization. The full two-family natural-language goal remains active
and **not achieved**.
