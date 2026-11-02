# Source-reference boundary experiment 03

Status: **offline experiment; not a production selector or live acceptance**.
The default command still uses `typed-requirements-02`; its selector payload,
model and native generator are unchanged. The separate experimental
`--intent-protocol indexed-v3` mode requires an explicit budget, ledger and
independent evidence journal. The candidate records bounded request/response
bodies, supports read-only journal replay, and has a bounded offline-tested
collector and source-bound scorer. This is a new local build, not the frozen
evaluation runtime or a live acceptance result.

## Why this experiment exists

The [complete failed evaluation](../typed-evaluation-02/RESULTS-02.md) remains
5/14 faithful raw extractions, 7/14 correct application decisions, and 2/5 useful
boards. Six completed provider responses caused local validation errors. Exact
quote copying, duplicated clause records, and facts repeated against pronouns
were avoidable parts of that protocol burden. Incorrect meaning and invented
numeric defaults were separate failures; eliminating copied quotes does not
establish that these other failures are fixed.

The earlier publication turn finished that complete failure report and green
CI. The subsequent offline prototype removed copied quotes and clause-inventory
bookkeeping. This experiment is new offline development, not a resumption,
repair, rescore, or extension of the exhausted final-02 batch.

## Narrow hypothesis

Let the application keep the original request and its numbered source clauses.
An extractor emits a flat list of facts citing those clause IDs. It does not
copy evidence strings or reproduce an inventory of empty/background clauses.
Several source IDs can retain an antecedent together with a later pronoun,
negation, or temporal qualifier. IDs may be reused across facts, but duplicated,
missing, and out-of-range references within one fact are rejected.

The current revision also gives every recognized quantity occurrence its own
application-owned ID, exact byte span, original text, and compatible normalized
fields. The extractor references these IDs; it cannot supply a replacement
number or invent a numeric default. Quantity role and requirement state still
need semantic interpretation. A voltage token is not automatically a supply
requirement, and an accuracy tolerance in C is not an ambient operating bound.

Example synthetic input for `Please use SHT31. I have chosen that sensor.`:

```json
{
  "version": "3-indexed-quantities-experimental",
  "facts": [
    {"kind":"sensor","value":"SHT31","state":"required","sources":[0,1],"quantities":[]}
  ]
}
```

`DecodeReferencedIntent` in `internal/boardfamily/intent_reference.go` is a pure
local experimental entry point. It has no provider, ledger, native-generation,
or filesystem access. It validates fact shapes and source IDs, constructs
ephemeral application-owned evidence, and uses the existing fact validation and
deterministic admission engine. Original provider JSON is never modified or
accepted under the new version. The separate experimental provider contract
supplies a model-facing schema and context. Its explicit CLI mode is tested with
an in-memory HTTP transport, including real child-process execution, accounting,
native generation and evidence collection. It is not selected by the
default/released command; no new actual provider request is authorized.

For `Please use BMP280 with 400 kilohertz I2C clock.`, source quantity 0 carries
`clock_hz: 400000`. A synthetic numeric fact is:

```json
{"kind":"number","value":"clock_hz","state":"required","sources":[0],"quantities":[0]}
```

`PrepareReferencedRequest` regenerates the source table from the original
request. Decoding independently regenerates it; a caller-provided or altered
table is never trusted. Even a correct model-authored `number: 400000` property
is rejected under this protocol. The earlier unshipped source-only version is
not silently accepted as this indexed-quantity version.

## Guardrails retained and added

- Keep the entire original request, not a rewritten summary or closed vocabulary.
- Keep required, not-required, forbidden, and uncertain states distinct.
- Keep family/profile/measurement identity checks, numerical units and range
  endpoints, fixed electrical bounds, direct-source contradiction checks, and
  no-configuration-on-failure invariants.
- Coverage is by quantity occurrence ID, not matching text, magnitude or shared
  clause. One clock value cannot cover an omitted capacitance; one occurrence of
  `100 pF` cannot cover another occurrence. Feature/other/unclear facts must also
  explicitly cite any quantity they classify, rather than swallowing every
  number in a broad source clause.
- Every fact retains required/not-required/forbidden/uncertain state. Excluded
  numbers do not set configuration values. Clock/pull-up prohibitions become
  profile exclusions where the catalog proves the mapping; other forbidden or
  unresolved numeric bounds produce a specific question, not a positive value.
- Local conversion supports the tested Hz/kHz, ohm/kilohm, V/mV, A/mA, C and
  pF/nF/uF spellings, scientific notation, leading decimals, explicit scalar
  voltage/temperature values, and `to`, dash, or `between … and` ranges.
  Reversed ranges, mismatched units, overflow, underflow, and unsupported
  dimensional mappings retain their source evidence without usable fields.
- Request preparation preserves exact bytes and caps input at 2000 bytes,
  32 source clauses and 128 recognized quantities. Response facts remain capped
  at 64. The old clause splitter remains unchanged for historical replay; the
  candidate splitter additionally preserves leading decimals such as `.1 nF`.
- Missing numerical facts produce a specific question; they cannot hide an
  independently established unsupported constraint such as heater operation.
- Do not silently map the historical response format to this candidate. All 14
  actual response bytes replay through the old decoder with exactly their
  original decisions and the same six errors.

## What the tests can and cannot establish

Fourteen **new synthetic extractions**, explicitly authored by the implementing
agent for the 14 already-known prompts, exercise the candidate's local decisions.
They are separate from historical provider responses. A pass establishes local
handling of the supplied correct facts, not model accuracy, an unseen holdout,
a repaired live score, generation timing, or end-to-end acceptance.

The same synthetic responses now traverse the actual OpenAI request builder,
stream parser, reservation/settlement path and experimental decoder through an
in-memory transport. All five supported cases then reach the real deterministic
generator: its 111 output files are byte-identical to the corresponding reviewed
examples. The fixtures are also checked against their actual request-specific
schemas. None of this measures a model's ability to produce those fixtures.

The next integration revision records the application-visible request/response
body bytes, checks terminal metadata independently of successful fact decoding,
and exercises the existing command flow with the candidate injected for tests.
Both families passed the real KiCad validation/export handoff; see
[the evidence and command record](EVIDENCE-COMMAND-VERIFICATION.md). The later
[journal](JOURNAL-VERIFICATION.md), [explicit command mode](COMMAND-VERIFICATION.md)
and [collector](COLLECTOR-VERIFICATION.md) extend that checkpoint. The collector
is verified offline; final semantic evaluation preparation is still pending.

Negative cases cover foreign/duplicated source IDs, pronouns without an
antecedent, copied quote fields, unknown states/kinds, invented identities and
defaults, wrong units/magnitudes/range endpoints/numeric roles, missing quantities,
malformed/duplicate JSON, bounded inventory, input limits, source ordering, and
failure/configuration invariants. Race checks and bounded fuzzing supplement
these tests; they are not language-understanding evidence.

The suite also explicitly demonstrates a **remaining semantic counterexample**:
a syntactically valid `custom_geometry` fact citing the indoor-project clause
still produces the wrong refusal. That diagnostic is not counted as a successful
user outcome. Source IDs do not prove that a label is true, that all relevant
constraints were extracted, or that a negation/temporal condition was understood.
Feature/other/unclear classifications of numerical requirements also depend on
semantics. The quantity scanner is a bounded literal recognizer, not an
exhaustive parser of English quantities, word numbers, mathematical expressions,
tolerances, or every unit. It does not establish full coverage of all numerically
meaningful text. Successful source indexing and unit conversion do not establish
that a value was assigned the right role or that an exclusion is true. No claim
of comprehensive independent meaning validation is made.

## Decision and next engineering boundary

Keep this candidate offline. It is worth investigating as a simpler provenance
contract, but it is **not sufficient to replace the production selector** and
does not justify another paid evaluation by itself.

The indexed quantity experiment and model-facing contract are implemented
locally; see the [quantity checks](QUANTITY-VERIFICATION.md) and
[provider/generation checks and review](PROVIDER-VERIFICATION.md). The latter
checks actual encoded payload bounds and separates local-invalid extraction,
provider failure and accounting failure. The subsequent
[evidence/command integration](EVIDENCE-COMMAND-VERIFICATION.md) retains failed
response bodies and verifies the in-process command path. The subsequent
[journal verification](JOURNAL-VERIFICATION.md) adds independent, pre-generation
checkpoints and process-exit/storage-failure tests. The subsequent
[command verification](COMMAND-VERIFICATION.md) adds explicit experimental
protocol/journal flags and real terminal-process tests while preserving the
typed-v2 default. The [collector checkpoint](COLLECTOR-VERIFICATION.md) implements
and tests bounded collection plus local byte-replay auditing. The
[scoring checkpoint](SCORING-VERIFICATION.md) adds evaluator-only gold matching,
explicit source-bound review, replayed outcome/native comparisons, full 14-case
synthetic integration and an offline CI job. The [runtime record](RUNTIME-03.json)
now binds an offline-qualified production build, its compiler-selected inputs,
tests and evidence; the [candidate review](REVIEW-03.md) records its boundaries
and remaining risks. Exact-source CI and actual live acceptance are still
pending. Preserve the distinction between a measured bad
extraction and an unsafe-to-continue transport/accounting failure. Do not confuse
structural guarantees with semantic completeness, add a word allowlist, or force
every ordinary request through user-confirmed configuration as a substitute for
the current natural-language goal.

Before production adoption, finish exact-source CI and final live evaluation
with source-bound semantic review. Any later
live evaluation requires a separately frozen runtime/corpus and explicit new
request/dollar authority. No requests remain in final-02, and unused dollars do
not restore request slots. No new live, Gemini, firewall, fabrication, or physical
work is authorized here. PR #14 and the full goal remain incomplete.

Run the local candidate tests with credentials removed and the repository's
offline Go caches:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY \
  -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  GOCACHE="$PWD/.cache/go/build" GOMODCACHE="$PWD/.cache/go/mod" \
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOENV=off GOWORK=off \
  go test ./internal/boardfamily -run 'TestSourceQuantities|TestIndexedQuantity|TestReferencedIntent|TestReferencedProvider|Fuzz.*Referenced' -count=1
```
