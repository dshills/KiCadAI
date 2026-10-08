# Direct-source extraction candidate 07

Status: offline candidate, not a successful live evaluation or release approval.

## Why this candidate exists

The frozen connection-v5 run at source d1a66d4a5297be100a3d5e42e5097dcbb8d23dc7 completed all 14 requests but failed its acceptance gate: 7 complete passes and only 2 of 5 requested native boards, both BMP280. Its original request, response, review and budget records remain in development-06 and are not changed here.

Inspection of useful-03's captured request/body.bin showed that the application-owned source JSON was encoded inside the provider wrapper's prompt string. The extraction instructions were another field in that same user-input wrapper. This is a concrete representation issue, not proof that it caused the semantic failures.

Candidate 07 changes only message framing. The hypothesis is that a single source object and a distinct instruction field will be easier for the same pinned model to interpret faithfully. No live result establishes that hypothesis yet. Schema-valid invented features can still pass structural admission; the negative wired-to-wireless counterexample is deliberately retained.

The design follows the documented distinction between instructions and input in [OpenAI's prompt engineering guide](https://developers.openai.com/api/docs/guides/prompt-engineering). [Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs) explicitly cautions that schema-conforming outputs can still contain mistakes. These references support the request structure, not a claim of improved accuracy.

## Contract and invariants

- Explicit opt-in command: `--intent-protocol direct-v6`. The default stays `typed-v2`.
- Version: `6-direct-source-experimental`; request revision: `direct-request-07`.
- Model remains `gpt-4.1-mini-2025-04-14`. One first attempt, 1600 maximum output tokens, the same 65,536-byte HTTP bound and budget reservation rules.
- The user input is the exact application-owned source JSON object: complete original request plus unchanged clause and literal-quantity tables. It is decoded once, without an inner prompt string.
- Only static application extraction instructions move to the Responses instructions field. The original request remains untrusted user input. Tests include an injection-shaped request and check that its text never enters instructions.
- The requirement schema is semantically identical to connection-v5, except for its distinct version enum. No supported family, profile, quantity occurrence, unknown requirement, negation state or temporal constraint is removed.
- Decoding uses a bounded pure version lowering before unchanged connection-v5 admission. Original provider bytes remain in the new journal; old live responses are not rewritten or scored as new evidence.
- Journal and audit versions are separate. Legacy inspectors reject new journals, and the new inspector rejects legacy journals.
- No automatic repair, retry, model switch, capability-based requirement rewrite, or manual native-output repair is introduced.
- No CAD generation, placement, routing, engineering limits or native validation logic changes.
- A budget policy file or exported contract is not spending authority. The prior batch is closed; unused dollars do not authorize this candidate.

## Offline checks

The saved terminal records in `.cache/direct-07-checks-01` distinguish test mechanics from model accuracy:

- `focused-01`: race-enabled provider/admission checks; 275 passing test events, 7 opt-in skips, no failures. This preceded the extra combined maximum-inventory/padding case, which is covered by the later full package run.
- `command-01`: race-enabled command checks; 117 passing test events, 2 native-regeneration skips, no failures.
- `packages-01`: full race-test suites for internal/aiprovider, internal/boardfamily and cmd/kicadai-board-family passed. The command tests use in-memory transport, real generation and a validation stub. Generated files are compared to existing qualified examples; the stub is not new native qualification.
- `lint-01`: configured lint checks for the three affected packages, 0 issues.
- Later checks and the source fingerprint are recorded in `.cache/direct-07-checks-01/verification.json` when complete.

The direct-source bound tests cover all 14 unchanged corpus prompts, maximum text, escaped input, quotes, Unicode, maximum clause/quantity inventory, combined inventory plus escaped padding, and instruction-like source text. They verify source/schema parity, exact HTTP framing, bounded accounting and read-only journal replay using synthetic responses only.

Tests also cover malformed input, prohibited retry/diagnostic use, cross-version rejection, tampered journal records, provider/stream failures, generation/validation failures, missing budget/ledger/journal, output overlap, and mixed CLI modes.

## Required before another live claim

1. Finish source review and freeze the candidate executable, contracts, runner dependencies and unchanged acceptance corpus.
2. Preserve and authenticate the unchanged native qualification; do not repeat native work solely because extraction changed.
3. Obtain fresh explicit authorization for publishing this candidate and for any separate paid evaluation. No remote update or paid request occurred in this development step.
4. Run the originally scoped complete evaluation with the original semantic and native acceptance gates. A passing synthetic pipeline is not a substitute.
5. Review every raw output against the original source and retain all failures. Do not mark the overall two-family goal complete unless real evidence establishes it.

Physical fabrication, firmware bring-up and bench validation remain outside this goal's authorization.
