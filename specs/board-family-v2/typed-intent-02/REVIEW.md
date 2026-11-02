# Typed-intent successor review 02

Disposition: **offline implementation qualified; end-to-end release acceptance
still pending**. This review is by the implementing agent, not an independent
engineering reviewer or an additional Gemini review. No API was called.

## Outcome

The production selector no longer requires every user word to belong to a local
allowlist. It uses application-numbered clauses, source-quoted typed facts, and
application-derived admission. The original request is retained without requiring
the provider to reproduce it. The model no longer supplies a final configuration
that the application must reconcile with a separate model verdict.

All 25 wording/configuration combinations in the diagnostic now pass with
synthetic correct extractions: five canonical requests and 20 polite/question
variants. Before this change, the same 25 requests with ideal full-decision
proposals yielded 5 passes and 20 unnecessary clarifications. That earlier audit
is copied unchanged into `usability-01/`; its source helper is archived as
`main.go.txt` to avoid adding a runnable package to the product. These are distinct
synthetic response formats, not a comparison of live model outputs.

Additional local tests cover ordinary measurement paraphrases, all five supported
profiles, explicit engineering limits and unit conversion, excluded versus
forbidden features, genuine unresolved choices, contradictory sensor/profile
requests, heater conditions, malformed/duplicate JSON, invented source evidence,
numeric role/value/range errors, and failure/configuration invariants. Explicit
quantities omitted from polite requests now withhold generation. A development
test caught `I2C` being mistaken for `2 C`; the identifier boundary was corrected
before the complete qualification run.

## Qualification evidence

Source-bound run: `.cache/board-family-v2/typed-intent-02-run-01`.
The publication contains the complete 13 command logs, coverage file, exported
successor contract and receipt, not just this summary.

- 25/25 synthetic wording cases pass.
- 14/14 frozen prompts pass with **new, implementing-agent-authored synthetic
  typed extractions**. This is not a replay of old provider bytes through a new
  schema, and not a new live score.
- All 14 actual historical-response regressions still pass their retained legacy
  decoder tests. Original live scores remain **4/14 raw and 8/14 application**.
- The complete short Go regression passes across 174 packages (95.771 seconds;
  unaffected cached results are disclosed in the log).
- Target-package race checks pass. A bounded 15-second fuzz run executes 255,457
  inputs without breaking configuration/source-retention invariants. This is not
  a proof of natural-language understanding.
- Full lint: zero issues. Existing Node safeguards: 17/17 pass.
- The actual streamed provider-response parser and the ledger are exercised with
  an in-memory transport and fake key. Cases cover success, refusal by local
  requirements, invalid extraction, network failure, incomplete response,
  wrong returned model and exhausted allowance. No socket or DNS request occurs.
- A 2000-byte ordinary input produces a 15,551-byte request within the unchanged
  24,000-byte bound. Escape-heavy nested JSON is rejected before reservation when
  it exceeds that bound. No cap, retry policy or request allowance was raised.
- Fresh explicit-config BMP280-standard and SHT31-standard builds each pass all
  14 native/manufacturing gates and match 81 reviewed deliverables in total.
  Their command wall times are 4.028 and 3.999 seconds. They are **not** live-AI
  timing measurements. The earlier full five-profile qualification is reused
  because native templates and manufacturing generation are unchanged.
- Before/after historical authentication passes. Both exhausted ledgers, the
  frozen original runtime, final evaluation and prior correction publication
  remain byte-identical. New API requests/spend: **0 / USD 0**.

## Review boundaries and remaining risks

The source quote checks prove string membership, not the truth of a semantic
classification. The extractor can still misunderstand negation, omit a
non-numeric requirement, or label an actual feature request as excluded. Literal
heater/direct-power/mechanical checks and the retained fully recognized grammar
are independent defenses, but do not cover every paraphrase. Measurement/profile
identity anchors and mechanical checks may also be conservative for unusual
wording. These risks must be measured, not hidden by counting all clarifications
as safe successes.

Typed facts and supported JSON are retained separately from final decisions.
Provider transport/refusal/incomplete errors retain available error/usage/ledger
metadata under the existing provider behavior; this change does not add a
complete raw HTTP-stream archive. The exported request context/schema has changed
and is explicitly versioned; model, endpoint, token/byte bounds and accounting
have not changed. The old final-01 runner expects `raw_decision` and must not be
pointed at this new binary or silently adapted in place.

SHA-256 authenticates the recorded local bytes and their relationship to the
tested sources. It is not a provider signature, independent semantic review, or
proof of physical operation. There is no measured reduction in live failure rate,
latency or spend yet. No board has gained fabrication/bench certification.

## Next acceptance boundary

Prepare a separately versioned runner and freeze a small successor evaluation
covering all five useful profiles, ordinary paraphrases, true choices, numerical
limits and adversarial requirements. Assess **raw typed-fact completeness and
meaning separately from final admission and native outputs**. A harmless wording
clarification is a failure, not a success; an unsupported requirement must not
produce a board even if native gates pass. Keep every attempted case and error.

Bind that evaluation to the exported successor contract and a new runtime.
Obtain explicit new request/dollar authority before any live execution. Do not
reuse the exhausted 41-call v1 or 14-call v2 ledger, overwrite runtime-01, add a
firewall rule or perform recovery calls under this offline approval. Keep PR #14
draft until real acceptance and the exact-head CI quality gate are complete.
Physical bring-up remains a separate future authorization.
