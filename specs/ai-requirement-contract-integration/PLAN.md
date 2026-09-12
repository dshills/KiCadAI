# Offline AI-to-requirements integration

User approval: September 11, 2026, in response to the separately scoped
integration milestone in `specs/practical-sensor-controller-boards/SCOPE-DECISION.md`.
Starting commit: `5cd36155bb412798edc8c6c05874ecfbf69dc316`.
Work branch: `codex/ai-requirement-contract-integration`.

## Objective and limits

Make the existing AI-facing v3 requirement contract internally consistent and
demonstrate valid generation, refusal and bound clarification workflows using
independently authored synthetic data. Keep compiler validation authoritative.
This is interface reliability work, not proof of complete new-board capability.

The user approved offline implementation only. Provider credentials are removed
from test processes. No provider calls, model change, response-size increase,
additional recovery baseline, acceptance relaxation, merge, release, fabrication
or stable-support expansion is authorized. Historical/frozen inputs, evaluation
tools, result files and raw evidence are preserved byte-for-byte. PR #11 remains
the separate immutable baseline publication.

## Implementation and verification

1. Align schema emission with its advertised v3 contract: constrain later-version
   fields to absent/empty forms, use the compiler's semantic vocabulary, encode
   exclusive binding forms, and represent numeric and relation/value bounds.
   Keep schema objects strict and fully required; preserve genuine optional
   values through explicit null/empty forms. Do not broaden compilation to a
   different contract version or silently reinterpret invalid proposals.
2. Describe source coverage, references, uncertainty ownership and exclusive
   refusal/clarification outcomes at the provider boundary. Test complete new
   synthetic ready/refusal/clarification examples against both emitted schema
   and compilation, with adversarial mutations and bound-answer replay checks.
3. Exercise provider stream completion, opaque metadata, parser limits, output
   consistency and usage through synthetic transports. Harden invalid-stream
   rejection where supported by those tests; retain existing byte/token limits
   and request/retry policy. Rejected outputs must not become accepted inputs.
4. Run focused and affected downstream offline tests, race checks and lint;
   verify frozen-file/evidence preservation. Publish results, review findings
   and remaining limitations in this directory. No live success rate is inferred
   from authored inputs or mocks.
5. Propose, but do not execute, a separately frozen live protocol/resource plan
   after offline checks. That future decision must distinguish interface success
   from full electrical/native/replay board acceptance and cannot rewrite the
   protocol-v2 negative baseline.

## Documentation basis

The schema must use the supported strict Structured Outputs subset: required
object fields, no additional properties, and nested alternatives where needed.
Cross-reference identity and engineering correctness remain compiler checks.
Checked September 11, 2026: [official Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs).

## Progress

- Baseline branch preserved; separate implementation branch created.
- Contract/source inspection confirms the v3/later-field mismatch and missing
  constrained semantic vocabulary. Schema, context and compiler alignment are
  implemented without weakening engineering validation.
- Independent synthetic contract and stream regressions pass, including bound
  clarification answers and replay rejection. Affected-package race tests and
  lint pass; historical evidence authentication passes.
- Results, local review and a proposed separately authorized live protocol are
  recorded in [RESULTS.md](RESULTS.md), [REVIEW.md](REVIEW.md) and
  [LIVE-PROTOCOL-PROPOSAL.md](LIVE-PROTOCOL-PROPOSAL.md).
- GitHub reported PR #11 merged at the unchanged baseline head when checked on
  September 11, 2026. This implementation does not modify that publication and
  has not been pushed or submitted as another PR. No live calls were made.
