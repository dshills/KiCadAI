# Source-eligibility candidate 11 (offline-tested experimental integration)

This successor candidate addresses two failure mechanisms from the closed
GPT-4.1 comparison: invented feature assertions, and numeric accuracy tolerances
misclassified as operating limits. It does not alter the frozen v7 contracts,
captured model output, evaluation corpus, acceptance thresholds or native board
qualification. The real evaluated result remains **12/14**, not 14/14.

## New contract

- A feature label must have a lexically compatible mention in the original
  request. Its v8 assertion carries an `anchor` clause selected from the
  application-owned eligible clauses, and that anchor must also be cited in
  `evidence`. Firmware cannot be asserted from USB power alone. An adapter ban
  is not recast as a general extra-peripheral ban.
- A precision, tolerance, resolution, repeatability, error, offset or drift
  phrase directly attached to a quantity removes operating-value choices for
  that exact occurrence. The original quantity, units, byte positions and
  source clause remain intact. Such a quantity must be retained as `other` with
  its precise meaning, or `unclear` if genuinely ambiguous.
- Unfamiliar wording remains representable through `other`/`unclear`; lexical
  eligibility is not permission to omit it. All v7 state distinctions,
  quantity-inventory limits and engineering admission rules remain in place.
- The compiler validates every new constraint before constructing a separate
  internal admission object. It rejects the entire extraction on a failed
  guard; it never deletes a bad assertion and then proceeds with a board.

The public development entry points are `SourceEligibleEvidenceSchema`,
`SourceEligibleEvidenceContract`, `CompileSourceEligibleEvidence` and
`DecodeSourceEligibleEvidenceIntent`. It now has an explicit experimental
`--intent-protocol source-eligible-v8` command path, with provider/journal APIs
`InterpretSourceEligibleWithJournal` and `InspectSourceEligibleJournal`.
The default remains `typed-v2`. Its model is the same pinned GPT-4.1 snapshot;
**no live request is authorized or claimed for v8**.

The direct provider input contains `source` (unchanged original text, clauses
and quantities) and a separate application-owned `eligibility` table. Key-free
`--export-live-contract FILE --intent-protocol source-eligible-v8 --prompt TEXT`
exports that exact input, schema, instructions and accounting identity. Export
does not reserve funds, contact a provider or grant spending authorization.
`--inspect-source-eligible-journal DIRECTORY` replays local bytes only.

Generation requires a new evidence journal, explicit separate budget and ledger.
A distinct version-3 accounting profile prevents v8 and earlier modes from
reusing each other's ledgers even for the same model, goal and limits. The full
model's 16,000-byte request bound, 1,600-token output cap, $0.05 reservation,
$2/$8-per-million uncached input/output accounting and no-retry behavior remain.
An incomplete response, unknown outcome or model mismatch prevents another
request using that ledger. Export and journal consistency are not semantic
accuracy or provider-attestation claims.

## What this does not prove

Lexical eligibility is necessary, not sufficient, for semantic support. A
mentioned heater can still be assigned the wrong state or temporal scope.
The test suite intentionally retains this counterexample. Broad words such as
"programming" can have multiple meanings; uncommon paraphrases may require the
free-form path. Neither schema conformance nor passing synthetic fixtures proves
that a model will extract all requirements correctly.

This limitation agrees with the official OpenAI guidance that structured
outputs can still contain semantic mistakes; simpler tasks and instruction
changes require evaluation rather than assumed correctness:
[Structured outputs: handling mistakes](https://developers.openai.com/api/docs/guides/structured-outputs#handling-mistakes).

## Offline verification and continuation

Tests cover every feature rule and all four states, unsupported inventions,
wrong/uncited anchors, word boundaries, both sides of precision wording, repeated
values, UTF-8 offsets, operating range plus tolerance in one clause, unknown
requirements, malformed envelopes, source limits, dense quantity inventories,
historical-version isolation and nonmutation. Existing hand-authored synthetic
fixtures for all 14 cases must compile to exactly the same internal evidence and
engineering decisions as v7. They are explicitly not corrected live outputs.

Offline integration tests additionally check exact exported/wire input,
one-request dispatch, all 14 hand-authored command fixtures, actual deterministic
generation for their five supported configurations (validation stubbed, not new
native qualification), failure-before-generation, journal tampering, protocol
and accounting isolation, request/cost caps, model mismatch and unknown-outcome
halts. Optional `KICADAI_ELIGIBLE_FULL_BASELINE` joins replay tests to the original
completed full-model journals without writing them or calling the provider.

`TestSourceEligibleCommandNative`, when explicitly given the offline KiCad CLI,
also performs real generation and all 14 native validation gates for the five
supported fixtures across both families. Retained artifacts are labelled
synthetic extraction, with no manual output repair and no physical qualification.
They use a separate `source-eligible-native` output directory so existing v7
native checks can coexist in the same review root.

Next: finish review and qualification of this integration, preserving the
unchanged native qualification and failed evaluation. A later frozen live
evaluation requires a newly approved payload/budget and exact executable network
rule; the completed batch cannot be reused, retried or resumed. Offline tests
are not a new live score, so the evaluated result remains 12/14.
