# Typed requirement extraction 02

This is a successor implementation, not a rewritten result for final-01.
It addresses the ordinary-language admission defect documented in the retained
`usability-01/` audit: only 5/25 synthetic ideal proposals passed the previous
closed-vocabulary gate. Harmless wording caused 20 unnecessary clarifications.

The production natural-language command now asks the pinned model for typed
requirements, not a verdict/configuration or a verbatim transcription. The
application keeps the original request and numbered clauses, validates quoted
facts, derives family/profile/operating bounds, then runs the existing deterministic
generation and native validation. Unknown harmless words are not a veto.

## Contract and safety boundaries

- Admission version: `typed-requirements-02`; schema name:
  `board_family_requirements_v2`; extracted document version: `2`.
- Every application clause is represented exactly once. Facts distinguish actual
  requirements, exclusions from requirements, prohibitions and unresolved choices.
  The application owns the final decision and configuration; provider facts stay
  in `raw_intent`, separately from `original_request` and `request_clauses`.
- Exact object shapes, duplicate keys, clause IDs, short source quotes, numeric
  magnitude/units/range endpoints, finite values and catalog bounds are checked.
  Known numeric quantities cannot simply disappear from a polite request.
- Family/profile identities are source anchored. Combined required families,
  contradictory profiles, out-of-envelope values and required unsupported features
  withhold generation. Genuine uncertainty produces a targeted question.
- Narrow independent literal checks retain demonstrated heater/direct-power and
  mechanical contradictions, plus the older grammar's fully recognized constraints.
  They are defense in depth, not a general English proof.
- The pinned model remains `gpt-4.1-mini-2025-04-14`, Responses endpoint only,
  streamed foreground mode, 1600 output tokens, 24,000 request bytes, one physical
  request with no automatic retry. The existing ledger reservation/settlement
  implementation and all prior exhausted allowances are unchanged. Escape-heavy
  inputs can hit the unchanged byte cap before reservation, even below 2000 bytes.

The provider still interprets meaning, negation, context and requirements outside
the typed numeric checks. It can omit or misclassify facts despite schema validity.
The independent checks can also miss paraphrases or conservatively reject wording.
Offline synthetic success is therefore **not** end-to-end language acceptance.

## Evidence and verification

`verify.mjs` runs credential-free local tests, race checks, bounded fuzzing, full
short regressions, lint, existing Node safeguards, contract export and two native
bundle comparisons. It requires a new cache directory and never overwrites a run.
`publish.mjs` copies completed evidence with exclusive creation; `check.mjs`
authenticates the publication. `check.mjs --current` additionally authenticates
the qualified source revision and workspace-only binary/native artifacts.

The old grammar decoder and its actual recorded-response tests remain available
for historical replay. The new tests using the same 14 prompts inject explicitly
synthetic typed facts: they do not convert old provider responses into new-model
results or change either old score. Native outputs are never hand repaired.

Run, from the repository root:

```sh
node specs/board-family-v2/typed-intent-02/verify.mjs .cache/board-family-v2/typed-intent-02-run-01
node specs/board-family-v2/typed-intent-02/check.mjs
```

No live call, API spend, additional provider review, firewall change, fabrication,
firmware delivery or bench operation is authorized by these scripts or this
document. The successor payload/runtime and a new frozen evaluation need a new,
explicit request/dollar authorization before live acceptance. PR #14 remains draft
until the complete goal, including the real language workflow, is demonstrated.

The implementation follows the closed-object/required-field/nested-union
restrictions in [OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs).
This is schema guidance, not evidence of model reliability or physical board performance.
