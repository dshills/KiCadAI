# Partitioned requirement extraction: offline prototype

Candidate 08 is **not a completed goal, release, live-tested improvement, or spending authorization**. Candidate 07 remains a completed failed evaluation: 8/14 complete passes and 4/5 useful native boards. Its source, runtime, provider responses, judgments and evidence remain unchanged.

## What changes

The experimental version `7-partitioned-evidence-experimental` separates nonnumeric requirements from a mandatory, source-owned quantity inventory. Each original qN is a required object property containing a classification. A feature assertion cannot consume a quantity. Numeric values and owning clauses still come exclusively from the original application-owned source; the model chooses a semantic role, not a magnitude.

Requirement states use explicit wire labels: `requested`, `unnecessary_but_allowed`, `must_not_occur`, and `unresolved_choice`. Compilation maps these one-to-one to the existing states without correcting model output. Quantity roles without a faithful supported field use a precise `other` description or a genuinely targeted `unclear` question; they are not forced into a feature enum. The application remains responsible for engineering feasibility.

Shared JSON Schema definitions keep the schema bounded even at 32 clauses and 128 quantity occurrences. Source preparation preserves the complete request and clause/quantity tables. Model selection, direct JSON framing, attempt 1, 1600 output tokens, engineering admission, native geometry and operating limits are unchanged. This prototype has no production CLI, journal or live-transport dispatch path yet.

## Offline checks completed

Using the repository-pinned Go 1.26.8 toolchain and local dependency cache, with provider secrets removed and Go network fetching disabled:

- Board-family, CLI and provider short regression suites: 3 packages passed, 1,550 test/subtest pass events, zero failure events. This is not the whole-repository CI suite.
- Candidate race checks: 53 test/subtest pass events, zero failures.
- Bounded fuzzing: 203,461 executions in a 5-second fuzz interval; no panic or caller-byte mutation.
- `go vet ./internal/boardfamily` passed; package-scoped repository lint reported zero issues.
- All five existing family/profile configurations retain exactly the prior engineering decisions across 25 hand-authored synthetic wording cases.
- Negative checks reject missing/empty/foreign quantity slots, CRC feature substitution in a resistance slot, incompatible numeric roles, historical envelopes and non-clause evidence on ordinary requirements.
- Range endpoints, repeated quantity occurrences, source ownership, excluded versus prohibited requirements, accuracy/load quantities, and startup-versus-later heater conditions are represented in tests.
- All 14 original prompts retain identical source tables and prior direct-v6 request preparation. This checks contract/source preservation, **not** new model accuracy on those prompts.
- Six in-memory provider-envelope fixtures ranged from 7,439 to 53,854 bytes under the unchanged 65,536-byte request ceiling. The simultaneous maximum fixture uses 108 enum members and 171 object properties. No socket was opened by these fixture transports.

The local `.cache/partitioned-08-checks/verification.json` retains exact commands, timings, source hashes, result counts and output hashes. Logs and the offline runner are beside it. Early environment setup failures are not passing checks: the default Go cache was sandbox-restricted, the default module cache lacked pinned dependencies, and mixing the installed Go 1.27.1 with the pinned GOROOT caused version errors. The recorded final checks use the matching Go 1.26.8 binary, GOROOT and local modules. Lint also required that pinned binary first in PATH.

## Limitations and review findings

Mandatory quantity slots prove inventory presence, not meaning. The model can still assign the wrong eligible role, use an inaccurate free-text description, omit a nonnumeric requirement, or claim a forbidden state when the source says merely unnecessary. An explicit test preserves a schema-valid synthetic wired-to-wireless false refusal. Do not call these tests an accuracy improvement, independent review or general reliability estimate.

The existing 64-assertion admission bound remains enforced after compilation. A source inventory can be valid and fit the request-size limit yet require too many assertions to admit; dense quantity requests therefore need explicit expressiveness/budget review before live integration. No source input limits were reduced to make the size fixtures pass, and no admission limit was increased to hide that issue.

The test-only schema checker expands local acyclic definitions; it is not a general JSON Schema validator and does not prove that the provider will accept the schema. Native outputs were not regenerated because generation and admission code are unchanged; this turn does not claim new complete-board qualification.

## Remaining work toward the original goal

1. Complete a source-bound synthetic audit of all 14 acceptance cases and additional adversarial wording, checking full extraction fidelity and truthful decisions separately. Preserve the original criteria and historical failed results.
2. Resolve dense-inventory expressiveness, output-token feasibility, and schema/decoder mismatch risks before selecting this protocol for a live path.
3. Add explicit CLI selection, isolated journal versioning, replay authentication and in-memory end-to-end tests, retaining the existing default and all transport/accounting safeguards.
4. Review the complete candidate and reuse unchanged native qualification. Only then consider a separately authorized fixed live evaluation. No additional request allowance or budget exists for this candidate.

The full goal remains two distinct reviewed board families through a reliable natural-language workflow, complete native/manufacturing bundles, no manual output repair, and the original complete acceptance gate. Fabrication and physical bring-up remain separate.

## Guidance used

The OpenAI Docs skill informed use of shared schema definitions and the distinction between structural validity and semantic correctness. The [official Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs) supports local definitions and warns that structured outputs can still contain mistakes. The API-key skill preserved the existing approved credential choice; all work here was offline with no live API requests, spend, firewall changes, remote publication, or primary-checkout changes.
