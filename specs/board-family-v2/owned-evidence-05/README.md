# Owned-evidence protocol: offline prototype 05

Status: **offline prototype, not a live-evaluated or production-ready route**.
Developed on `codex/owned-evidence-05`, based on
`87d05b352dfffd96ca6b951ce6b195a2c6463fbd`, on 2026-09-15.
The primary working tree, PR #14, frozen evaluator, prior provider responses,
historical scores, request ledgers and native qualification are unchanged.

## Outcome and boundary

The application now owns structural joins that it can derive exactly from the
original request. A new, pure experimental compiler accepts numeric references
such as `q0/pullup_ohms`, binding one quantity occurrence, a dimensionally eligible
field, its converted value and its owning source clause together. A feature's
`qN` evidence similarly contributes that quantity's owning clause automatically.
The model cannot supply a replacement quantity table, invent a numeric value,
or independently join a quantity to a dimensionally incompatible field.

This addresses the **representation mechanism** behind the uncited-quantity
failures, not the correctness of a model's meaning. The live evaluation remains
**9/14 complete, 11/14 application, 9/14 raw**, with only 3/5 useful native bundles.
See [the unchanged live result](../indexed-evaluation-04/README.md). No historical
response was converted and called repaired, and no new live request was sent.

The compiler is not connected to a provider, command-line flag or default,
journal, ledger, evaluator or native generator. Its version is
`4-owned-evidence-experimental`. The compiled v3-shaped value is an internal
admission input only, never a captured response or a historical migration.

## Representation contract

| Model-owned selection | Application-owned interpretation |
| --- | --- |
| `choice: "qN/field"` on a numeric fact | Exact source occurrence, compatible dimension, converted value and owning clause |
| `evidence: ["qN", "cM", ...]` on feature/other/unclear | Each quantity plus its owner; any additional contextual clauses |
| `evidence: ["cN", ...]` on an identity | Source clauses only; identities cannot consume quantities |
| A fact's state | Preserved as required, not-required, forbidden or uncertain, without weakening it |

Additional clause context, multiple quantities on a feature, repeated numeric
occurrences, unknown dimensions, temporal requirements and negative requirements
remain representable. Reference arrays have bounded **set-union semantics**:
duplicate aliases within the existing array bound have no extra meaning. The
compiler emits canonical source order but never modifies the raw input bytes.
This is an explicit new contract, not cleanup of malformed historical v3 output.

Every source table is regenerated from the original request. The original
1–2000-byte UTF-8 input, 32-clause and 128-quantity limits remain unchanged. There
is no new mandatory form, keyword-only input language, or routine human approval
step. The wire keeps the existing 64-fact and 65,536-byte safety bounds; the byte
bound and semantic admission checks remain separate from JSON Schema shape.

Compilation fails without returning partial facts. The existing deterministic
admission engine still decides supported/clarify/unsupported, checks narrow
numeric-role rules and detail validity, and accounts for omitted quantities.
A schema-valid response is **not** a guarantee of successful semantic admission.

## Offline verification

All commands used the cached Go 1.26.8 toolchain, offline module resolution and
removed `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY` and
`KICADAI_LIVE_PROVIDER_TESTS` from their process environment. No new key, network
permission, API budget, provider request, native qualification or board repair
was needed.

- All 14 existing **hand-authored synthetic fixtures** retain exact full-decision
  and configuration parity, including family, profile, operating bounds,
  refusal/clarification messages and every clause result. These are not new AI
  accuracy observations or held-out cases.
- 192 quantity/field/state combinations agree between the schema and compiler
  on dimensional eligibility and immutable source ownership.
- Negative tests cover invented numbers, incompatible fields, foreign aliases,
  wrong evidence kinds, caller-owned overrides, null fields, duplicate JSON
  keys, output/input bounds, historical-version rejection and fail-closed errors.
- Multiple quantities, repeated occurrences, UTF-8 source spans, omitted values,
  all four states, temporal heater requirements and unsupported dimensions are
  retained. Two real archived v3 extracts are rejected as the wrong wire version.
- `go test -short -p=1 -count=1 ./internal/boardfamily ./cmd/kicadai-board-family`
  passed. The same packages passed with `-race`.
- `golangci-lint run ./internal/boardfamily/... ./cmd/kicadai-board-family/...`
  reported **0 issues**.
- A 10-second `FuzzOwnedEvidenceCompiler` run completed **241,395 executions**,
  with five initial seeds, two workers and no failing input. Fuzzing is bounded
  test evidence, not an exhaustive proof or a language evaluation.

On the 14 known prompts, serialized schemas are 2,421–3,313 bytes and prepared
source tables are 253–1,171 bytes. At the simultaneous 32-clause/128-quantity
boundary, the schema is 11,908 bytes with 922 enum values; the prepared table is
39,465 bytes and offers 256 numeric choices. These are **not full API request
sizes**, token counts, costs or measured model latency. The tests check the
documented enum/property/string limits; actual provider compatibility remains
unverified. [Official Structured Outputs limits](https://developers.openai.com/api/docs/guides/structured-outputs)

The hand-authored response fixtures total 4,521 bytes versus 5,166 bytes in the
older representation. This reduction alone does not establish faster model
completion: input overhead and semantic accuracy have not been measured live.

## Review and remaining work

The implementing agent reviewed the compiler boundary, schema/decoder agreement,
mutable caller-table isolation, fail-closed behavior, raw-byte preservation,
legacy isolation and test assumptions. This is **self-review**, not an independent
Gemini review. The full affected-package tests, race checks and lint passed.

A deliberately wrong extraction of "wired pressure monitor" as a required
wireless feature still fits the new schema and causes a false refusal. The test
preserves this counterexample explicitly. Atomic references do not prevent wrong
polarity, invented features, inappropriate quantity roles or omitted nonnumeric
requirements. Adding an automatic owner also must not be described as proof that
a feature really refers to that quantity.

The next useful work is offline: review the semantic failure cases; design a
minimal provider instruction and request builder for this distinct protocol;
exercise the complete transport, journal, ledger and audit path using synthetic
responses; and verify the frozen historical routes remain intact. Do not route
the new protocol through old protocol identifiers or rewrite captured evidence.

Only after that offline integration and review should a separately versioned
runtime be frozen and a specific new live evaluation proposed. The previous
14-request authority is exhausted. This prototype authorizes **no retry, probe,
recovery batch, Gemini request, firewall expansion or additional API spending**.
The overall two-family reliability goal is still active and not achieved;
physical fabrication and bench bring-up remain separate milestones.
