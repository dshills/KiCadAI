# Source-eligibility candidate 11 (offline only)

This successor prototype addresses two failure mechanisms from the closed
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
`DecodeSourceEligibleEvidenceIntent`. The contract is explicitly `offline_only`.
It is **not connected to CLI selection, a provider, accounting or journals**.
Its proposed model is the same pinned GPT-4.1 snapshot; no request is authorized.

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

Next: review guard limitations, then integrate this version as an explicit
experimental CLI/provider protocol with isolated accounting and journals and
offline transport tests. Reuse unchanged native qualification. A later frozen
live evaluation requires a newly approved payload/budget and exact executable
network rule; the completed batch cannot be reused, retried or resumed.
