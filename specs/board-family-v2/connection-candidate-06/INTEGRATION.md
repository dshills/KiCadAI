# Connection-v5 integration — offline only

The candidate exposes a symmetric wired/wireless fact through the existing
command, admission pipeline, private journal, deterministic generator and
validation boundary. It does not change the default `typed-v2` route, any frozen
owned-v4 request, or the failed live evaluation. The implementing agent reviewed
this change; this is not an independent review.

## Contract and execution boundary

- Explicit selection: `--intent-protocol connection-v5`.
- Admission version: `5-connection-evidence-experimental`.
- Request revision: `connection-request-06`.
- Schema name: `board_family_connection_requirements_v5`.
- Journal / audit versions: `connection-evidence-journal-1` /
  `connection-journal-audit-1`.
- Provider configuration is unchanged: Responses API, pinned
  `gpt-4.1-mini-2025-04-14`, streaming strict JSON schema, 1,600 maximum output
  tokens, no background execution or response storage. Requests are bounded at
  65,536 HTTP body bytes and use the existing per-request reservation.
- Only the original prompt, source clauses, literal quantity table, extraction
  instructions and schema enter model input. There are no gold answers, board
  source files, geometry, catalog, or environment values in that input.
- A separate budget policy, ledger and new private evidence journal are required
  for prompt execution. A policy file is not user spending authorization.
- The command exports a source-specific contract without a key or network call.
  `--inspect-connection-journal` likewise performs local replay only.
- A first-attempt failure is terminal. The new path does not add retries,
  fallbacks, recovery calls or reuse of a journal. Credentials are removed before
  CAD generation and validation, as on the existing experimental paths.

Connection facts can reference source clauses but cannot classify or consume
numeric occurrences. Additional protocol, connector, range, electrical, firmware
and loading requirements remain separate constraints. Required radio remains
unsupported. A supported wired fact cannot cancel it, nor can an exclusion
become a prohibition.

The original transport stream remains in the journal. Indentation of the JSON
selection can change whitespace around its embedded extraction; it must not
change facts. Journal replay checks the stored request against the exact
protocol-specific request and replays the original response. These are local
integrity checks, not independent provider signatures or semantic entailment.
Legacy, prototype and successor evidence are not interchangeable.

## Offline checks

Final verification records are retained in
`.cache/connection-06-integration-01/verification.json`, with source hashes,
commands, actual terminal results, and hashes of their stdout/stderr/process
records. The cached Go 1.26.8 toolchain was used with dependency downloads
disabled and provider keys removed. Tests inject in-memory HTTP responses.

Recorded results:

- Focused race suite: exit 0, 131 passing named test events including subtests,
  no failures or skips, 12.569 seconds including startup.
- Affected-package lint: exit 0, zero issues, 3.063 seconds.
- Broad repository short suite: exit 1, 271.630 seconds. Both affected packages
  passed in full (`internal/boardfamily` and `cmd/kicadai-board-family`).
  `internal/opentopologysynthesis` exceeded the invocation's aggregate
  three-minute package timeout while a simulation test was running. No failed
  assertion was reported. That package's source is unchanged; the Makefile
  normally allows twenty minutes and CI splits its coverage into six shards.
  This observation does **not** establish a completely green repository suite
  or replace CI at this candidate's source.
- Request fixtures: 8,706–65,261 HTTP body bytes, under the 65,536-byte guard.
- Supported synthetic outputs: 111 generated files compared across five
  family/profile configurations (21 for each BMP280 profile, 24 for each SHT31
  profile), all equal to the reviewed generated source.

During development, two new test assertions initially failed: one marshaled a
byte slice as base64 while comparing schemas; another compared indented
selection JSON byte-for-byte with compact JSON. The assertions now compare
decoded schemas and compacted extraction JSON respectively, retaining equality
of facts and checking that caller bytes are unchanged. The original failures
were offline test failures, not provider requests or repaired live evidence.

The request fixtures include the unchanged 14-case corpus and six input-size
boundaries. The CLI corpus uses hand-authored synthetic extractions, never
repaired model responses. Five supported cases run the actual generator and
compare all generated source files against reviewed family/profile outputs.
Their validation step is a stub: this does not constitute new KiCad
qualification, manufacturing validation or live language success.

Negative tests cover provider refusal, provider/server/transport errors,
incomplete or wrong-model results, invalid/prototype extractions, accounting,
journal reuse, byte bounds, cross-version inspection, altered decisions,
coherently rehashed requests, duplicate JSON, unsafe files, missing files,
generation/validation failures and incompatible CLI modes.

## Remaining decision

`TestConnectionStillRequiresSemanticEvaluation` intentionally preserves the
counterexample: a wrong wireless fact for a wired request remains schema-valid
and causes a false refusal. Better representation and explicit extraction
instructions are a hypothesis, not measured accuracy or latency improvement.
OpenAI's [Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs)
distinguishes valid output structure from correct content.

The previous approved batch remains stopped after two requests, with twelve
unattempted prompts. This candidate does not reuse its remaining slots, budget,
firewall authority, raw evidence or acceptance results. No successor live
collector or frozen evaluation package is claimed here. A reviewed successor
package and fresh, explicit authority are required before any new transmission.
No PR publication, merge, fabrication or bench work is included in this local
integration checkpoint. The full natural-language-to-board goal is incomplete.
