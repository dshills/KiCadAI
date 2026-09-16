# Partitioned requirement extraction: experimental candidate

Candidate 08 is **not a completed goal, release, live-tested improvement, or spending authorization**. Candidate 07 remains a completed failed evaluation: 8/14 complete passes and 4/5 useful native boards. Its source, runtime, provider responses, judgments and evidence remain unchanged.

## What changes

The experimental version `7-partitioned-evidence-experimental` separates nonnumeric requirements from a mandatory, source-owned quantity inventory. Each original qN is a required object property containing a classification. A feature assertion cannot consume a quantity. Numeric values and owning clauses still come exclusively from the original application-owned source; the model chooses a semantic role, not a magnitude.

Requirement states use explicit wire labels: `requested`, `unnecessary_but_allowed`, `must_not_occur`, and `unresolved_choice`. Compilation maps these one-to-one to the existing states without correcting model output. Quantity roles without a faithful supported field use a precise `other` description or a genuinely targeted `unclear` question; they are not forced into a feature enum. The application remains responsible for engineering feasibility.

Shared JSON Schema definitions keep the schema bounded even at 32 clauses and 128 quantity occurrences. Source preparation preserves the complete request and clause/quantity tables. Model selection, direct JSON framing, attempt 1, 1600 output tokens, engineering rules, native geometry and operating limits are unchanged.

The opt-in CLI flag is `--intent-protocol partitioned-v7`; the default remains `typed-v2`. Experimental generation requires a separately approved budget, ledger and new private evidence journal. `--inspect-partitioned-journal` replays completed evidence locally without a key or request. The journal and audit have distinct version identifiers; legacy inspectors reject these journals, and the new inspector rejects legacy journals. Contract export is key-free and grants no sending or spending authority.

The new internal assertion ceiling is bounded at 320: at most 64 ordinary requirements and two endpoint roles for each of 128 source quantities. Historical public decoders still enforce their original 64-fact ceiling. This fixes an offline-reproduced regression where a valid 128-quantity request could never be admitted. It does **not** increase source limits, byte limits, the output-token cap, or any electrical limit. Schema alternatives also prevent mixing numeric and free-text roles within one quantity slot; duplicate numeric fields are still rejected locally.

## Offline checks completed

### Initial prototype checkpoint

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

### Integrated candidate checkpoint

The subsequent `.cache/partitioned-08-integration-checks/verification.json` records the new source snapshot and checks, again with credentials removed and Go dependency fetching disabled. It supersedes the initial checkpoint for the integrated source; the earlier receipt is preserved unchanged.

- Three-package short regression: 1,639 test/subtest pass events, three package passes, zero failures.
- Candidate tests under the race detector: 141 test/subtest pass events, two package passes, zero failures.
- Five-second bounded fuzz test, three-package `go vet`, and three-package repository lint all passed.
- The complete frozen 14-case inventory is now covered by hand-authored **synthetic** responses: all 47 original gold assertions and all 15 source quantity occurrences are represented, with the expected dispositions and all five supported configurations. This is format/engineering expressiveness, not model accuracy.
- The same synthetic corpus exercises the actual CLI selection, in-memory provider, journal persistence/replay and deterministic generation. Validation is stubbed in these orchestration tests and is not new native qualification.
- Nine failure modes cover invalid/malformed extraction, refusal, server error, missing model, broken stream, output-token exhaustion, generation failure and validation failure. They retain evidence, do not retry and do not cross inappropriate native-execution boundaries.
- A terminal output-token-limit failure records all 1,600 synthetic output tokens and one ledger entry, produces no board, and is deliberately rejected by the completed-response inspector. Retention is not certification as a complete response.
- Nine journal cases include an unchanged control and rejection of forged decisions/outcomes, coherent request rehashing, duplicate JSON, public permissions, extra/missing files and symlinks. Cross-version rejection is checked in both directions.
- Twenty-one in-memory provider/journal fixtures span 6,833–54,116 request bytes under the unchanged 65,536-byte ceiling. The simultaneous maximum inventory still uses 108 enum members and 171 object properties.
- Dense synthetic inventories retain 130 assertions and the full 320-assertion maximum; a 65th ordinary requirement is rejected. Historical decoder limits and over-size transport/replay guards remain enforced.

## Limitations and review findings

A subsequent [offline review](REVIEW.md) closes the real native-handoff gap: all five synthetic supported selections now run the actual KiCad 10.0.3 validation/export path, pass 14 gates each and match 201 reviewed deliverables. It authenticates all 256 published example files and checks the 14 schemas with Ajv plus 58 negative controls. Exact synthetic JSON sizes are recorded, but no provider token count or live accuracy result is claimed. The earlier stubbed-integration checkpoint remains unchanged as historical evidence.

Mandatory quantity slots prove inventory presence, not meaning. The model can still assign the wrong eligible role, use an inaccurate free-text description, omit a nonnumeric requirement, or claim a forbidden state when the source says merely unnecessary. An explicit test preserves a schema-valid synthetic wired-to-wireless false refusal. Do not call these tests an accuracy improvement, independent review or general reliability estimate.

The 1,600-output-token cap is unchanged. A source inventory can be valid and fit the request-size and internal assertion bounds while requiring too many output tokens to complete. Dense synthetic compiler tests prove representation only, not provider completion at that cap. Output-token feasibility for the proposed live corpus still needs explicit review; no token or spending increase is implied.

The test-only schema checker expands local acyclic definitions; it is not a general JSON Schema validator and does not prove that the provider will accept the schema. Existing engineering rules are reused through private bounded decoder helpers; historical public entry points retain their limits. Deterministic outputs are regenerated by synthetic CLI tests and compared with reviewed examples, but complete KiCad validation is not rerun here. This checkpoint is an implementing-agent review, not independent semantic or engineering certification, and these package checks are not a whole-repository CI run.

## Remaining work toward the original goal

1. Prepare and freeze a separate candidate-08 runtime and runner with the unchanged 14-case acceptance criteria, accounting boundaries and no-retry policy. Do not reuse or relabel candidate-07 evidence.
2. Preserve the completed offline review, size-screen limitations and native-equivalence evidence. Exact fixture sizes do not bound free-form model output or prove reliability; do not silently raise limits.
3. Request explicit authority for the frozen bounded evaluation and any genuinely required executable-specific network rule. No additional request allowance or budget exists for this candidate.
4. Assess full raw fidelity and complete native outcomes honestly; obtain separate publication authority for candidate-head CI and updated PR results. The goal remains open until its original gates are satisfied.

The full goal remains two distinct reviewed board families through a reliable natural-language workflow, complete native/manufacturing bundles, no manual output repair, and the original complete acceptance gate. Fabrication and physical bring-up remain separate.

## Guidance used

The OpenAI Docs skill informed use of shared schema definitions and the distinction between structural validity and semantic correctness. The [official Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs) supports local definitions and warns that structured outputs can still contain mistakes. The API-key skill preserved the existing approved credential choice; all work here was offline with no live API requests, spend, firewall changes, remote publication, or primary-checkout changes.
