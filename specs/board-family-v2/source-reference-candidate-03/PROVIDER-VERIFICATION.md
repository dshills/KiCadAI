# Indexed provider contract and generator checks — 2026-09-14

Historical component snapshot. The subsequent
[recorded-response/command integration](EVIDENCE-COMMAND-VERIFICATION.md) addresses
the in-memory response-loss and command-test gaps described below. Its remaining
durability and live-acceptance limits are separate; these earlier test results
are not silently reclassified as end-to-end acceptance.

Disposition: **offline component integration passed; not live or whole-command
acceptance**. The experimental files remain local and untracked on
`codex/board-family-v2`, based on `f8ac5099c2e2d21a72a2faf2bb2161ffbec8792a`.
The preceding goal work added the provider contract; this continuation observed
its pending test process complete successfully, added schema/fixture conformance
checks, and connected all five supported synthetic outcomes to the real native
generator. This is concrete offline progress, not another live evaluation.

## Contract and integration

`intent_reference_provider.go` adds the experimental schema, extraction context,
application-owned source preparation and a private provider integration point.
The released `InterpretWithPolicy` and `kicadai-board-family` command still use
the unchanged typed-v2 contract. No candidate flag, production switch, recovery
loop, alternate model or automatic live retry was added.

The candidate uses a closed object root and closed nested fact variants, with
every property required within its variant. Source and quantity bounds are
tightened to each request's application-owned inventory. A request without any
recognized quantity has no numeric-fact branch. Numbers remain application-owned;
the model may supply references and roles, not numeric magnitudes or defaults.
These schema choices align with the published Structured Outputs requirements;
the pinned `gpt-4.1-mini-2025-04-14` snapshot is documented as supporting Structured
Outputs and streaming. This is a documentation-based compatibility assessment,
not proof that the service accepted this new schema.
Sources: [Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs),
[GPT-4.1 Mini](https://developers.openai.com/api/docs/models/gpt-4.1-mini).

The context preserves explicit requirements, exclusions, prohibitions, unresolved
choices, temporal conditions, numeric roles and additional unsupported
requirements. Background language is not automatically a feature request. These
instructions address observed failures; they do not prove the model follows
them. The full unchanged catalog is included, and deterministic admission owns
the final family/profile/default selection.

The test transport receives the actual encoded request from the existing OpenAI
provider and returns synthetic SSE. Tests check endpoint, pinned model, strict
schema, streaming, `store:false`, `background:false`, one attempt and 1600 output
tokens. They compare source/context/schema to fresh preparation and verify that
the dummy credential is absent from the request body. No socket is opened.

## Observed local results

- The original pending focused test command finished with exit 0: **0.910 s**.
- After the additions in this continuation, all focused provider/schema tests
  and the example passed with exit 0: **0.960 s**.
- All **14 already-known prompts** with separately authored synthetic facts pass
  the in-memory provider path and expected local decision assertions. The 14
  fixtures also pass a test-only checker for every keyword used by this schema.
  Fifteen invalid-shape fixtures and the 0/1/64/65 fact-count boundaries exercise
  required fields, enum membership, types, references and cardinality. That
  checker is not a general JSON Schema engine or server compatibility test.
- All **five supported configurations** reach the real `Generate` function.
  BMP280 standard/fast/low-current each reproduce all **21** generator files;
  SHT31 standard/fast each reproduce all **24** generator files. The total is
  **111 byte-identical files** compared with the five reviewed examples under
  `examples/board-family-v2`. Configuration, electrical calculations, native
  projects, BOMs and bundled libraries are included. No output repair was used.
  This test does not run CLI orchestration, KiCad validation, previews or
  manufacturing export; unchanged qualification remains separate evidence.
- The actual encoded payloads for those prompts span **13,134–14,556 bytes**.
  An ordinary 2000-byte maximum-length input encodes to **16,945 bytes**.
  Escape-heavy and quantity-heavy inputs are rejected before the underlying
  transport callback, reservation or ledger creation, under the retained 24 KB
  bound. Payload rejection is not a successful user outcome or a language score.
- **Ten failure modes** check local-invalid extraction, malformed JSON,
  transport failure, incomplete response, provider refusal, wrong model,
  redirect, rate limit, excessive usage and settlement locking. Each attempted
  request reaches the test transport only once, retains its full reservation,
  and exposes no configuration on failure. Completed structured bytes remain
  available when a later model, local decoding or accounting check fails.
- **Eleven preflight/payload cases** check cancellation, absent key/transport,
  invalid policy, exhausted budget, budget-identity mismatch, ledger lock,
  invalid prompt and the three payload cases above. Unsent requests do not
  create or modify a ledger and have no reservation index.
- Final race checks passed with exit 0 and `-count=1` for both full target
  packages: `internal/boardfamily` **6.289 s**, `cmd/kicadai-board-family`
  **1.313 s**. Targeted `golangci-lint` completed with **zero issues**.
- Read-only final-02 publication authentication passed after the code edits:
  **203** original publication files plus **five** CI-addendum files are intact.
  It still records all **14** original attempts, raw **5/14**, application
  **7/14**, complete **5/14**, and two original native bundles matching **81**
  reviewed deliverables. The failed result and exhausted request budget did not
  change. Authentication includes the existing reviewed examples.

All test commands removed real provider credentials, disabled live-provider
tests, used the repository Go caches, and disabled module downloads. Tests use
only explicit dummy credentials and in-memory transports. There were no live
requests, new paid reviews, firewall changes, screen interactions, commits or
remote writes. The earlier interrupted broad-suite run remains disclosed in
[VERIFICATION.md](VERIFICATION.md); no new whole-repository or exact-head CI pass
is claimed here. The five generated bundles were temporary test outputs, not
newly published or physically qualified boards.

## Failure evidence and collection boundary

`extraction_outcome` distinguishes `no_request`, `decision`,
`invalid_extraction`, `provider_failed_or_unknown`, `model_mismatch` and
`accounting_failure`. An ordinary decoded clarification or unsupported result is
a `decision`, not a provider error. A completed, accounted but locally invalid
extraction remains a failed user outcome; it is not a reason to omit the case
from evaluation or retry it.

These labels are diagnostic only. A future collection runner must separately
establish that the child process is terminal, the response and usage join its
reservation, the frozen contract is unchanged, and the approved budget has
remaining capacity before advancing to the next planned case. It must not
automatically retry the failed case, treat an observation timeout as process
completion, or continue past unknown transport/accounting state.

Current limitations matter: the existing provider parser does not return raw
structured bytes for every malformed/incomplete/refusal response, and this
experiment does not archive the full HTTP/SSE response. The generic error text
can conceal a more specific preflight cause. A missing returned model can be
filled from the requested model by the shared provider; the mismatch check only
detects an explicitly different model. Thus this boundary is not complete raw
response evidence or independent provider attestation. Recording and verifying
all terminal outcomes still needs offline implementation and tests before a new
collection run.

## Review decision and next step

This is implementing-agent review, not an independent review. The retained
`custom_geometry`/indoor-background counterexample still yields a wrong refusal
despite valid references. Numeric role, negation, temporal meaning and omitted
requirements are not established merely by source IDs or schema conformance.
The numeric recognizer is bounded, not a full English/math parser. The 14 known
fixtures are development cases, not an unseen holdout or evidence of improved
live reliability, latency or usefulness.

Proceed offline with a tested whole-command candidate integration and complete
terminal-outcome recording. Preserve the existing production path and immutable
historical evidence while the candidate is unqualified. Review the actual
command's saved selection, decision/error disposition, credential clearing,
native/validation handoff and output inventory using in-memory provider inputs.
Do not substitute a required confirmation form, a keyword-only selector or
generic refusal for successful ordinary-language requests.

Before any later live run: finish the full-path review, freeze its exact runtime
and evaluation corpus, obtain explicit new request/dollar authority, and retain
all outcomes without manual resumption for a merely invalid extraction. No
request slots remain in final-02. Physical bring-up remains separately
authorized. The complete two-family natural-language goal is **not achieved**.
