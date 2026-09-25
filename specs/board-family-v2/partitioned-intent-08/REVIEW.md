# Candidate 08 offline review — live acceptance remains unproven

Reviewed September 16, 2026 by the implementing agent. Production checkpoint: `614de2247fa5bc81b4ed8b86b9c6c49e2dd4a78b`; this follow-up adds review tests and a read-only verifier, not a different extraction contract. No provider calls, new budget, firewall change, publication, merge or physical work occurred.

## Outcome

The new command's offline handoff is verified for **all five existing configurations across both board families**. Each uses a clearly labelled hand-authored synthetic extraction, the real deterministic generator, and the installed KiCad 10.0.3 validator/exporter. Every run passes all 14 gates. No generated output was repaired.

The review authenticates all 256 published example files against their retained publication manifest. It compares all 201 fresh native/library/BOM/configuration/electrical/preview/manufacturing deliverables against the previously reviewed examples, using only the original timestamp-normalization recipe in memory. New native-file hashes are identical; each manufacturing manifest authenticates its 19 files. Raw timestamps and complete generated directories are retained unchanged.

This closes the **synthetic command-to-complete-bundle** gap, not the model-accuracy gap. Candidate 07 remains a failed completed live evaluation (8/14 complete passes, 4/5 useful boards). Candidate 08 has no live results and the goal remains open.

## Extraction and response-size review

All 14 original corpus fixtures validate using installed Ajv 8.18.0 in strict JSON Schema 2020 mode, independently of the repository's small test-only schema checker. Fifty-eight mutations covering wrong versions, extra fields, null requirements, missing quantity slots and feature substitution into quantity slots are rejected. Passing a local standards validator does **not** prove acceptance by the provider's supported schema subset.

The original gold-assertion tests preserve all 47 acceptance assertions and 15 quantity occurrences; these are synthetic representation checks, not corrected or relabelled provider answers. The source prompt, dimensional quantity tables and fixed engineering rules remain authoritative.

For these 14 exact synthetic responses, compact JSON ranges from 83 to 711 UTF-8 bytes; two-space-indented JSON ranges from 96 to 1,131 bytes. This is a useful visible-payload size screen, **not a measured token count or completion guarantee**. No local model tokenizer was installed, and no token-count endpoint was called. Free-text details, additional legitimate requirements and formatting chosen by the model can differ from these fixtures. Dense maximum-inventory tests must not be presented as feasible 1,600-token generations.

The 1,600-token cap, model snapshot, one-request policy and no-retry behavior are unchanged. A synthetic terminal token-limit failure is retained and accounted, emits no board, and is rejected by the completed-response inspector. The OpenAI Docs guidance distinguishes visible text tokenization from API framing and schema overhead; the API limit covers generated output, including any reasoning tokens. These facts do not justify increasing this candidate's cap. See [Counting tokens](https://developers.openai.com/api/docs/guides/token-counting) and the [Responses output limit](https://developers.openai.com/api/reference/cli/resources/responses/methods/create).

## Residual findings

1. **Release-blocking evidence gap: live fidelity is unknown.** Mandatory quantity keys prevent omission of keys, not wrong roles, states or free-text meaning. Nonnumeric requirements can still be omitted. A schema-valid wired-to-wireless false refusal remains deliberately covered by a negative counterexample. Do not promote this candidate on synthetic scores.
2. **Bounded internal capacity is not a broad semantic relaxation.** The private v7 path permits 320 assertions to represent 64 ordinary requirements plus two roles for each of 128 quantities. Public historical v3/v4/v5 entry points keep their original 64-fact cap. No source, raw-byte or electrical limit was widened. Numeric duplicate roles and several semantic constraints remain application checks beyond JSON Schema.
3. **Review independence is limited.** Fixtures and this review were authored by the implementing agent after development. They are not a hidden holdout, independent engineering review, statistical reliability estimate or provider-signed attestation.
4. **Physical limitations are unchanged.** The reused [electrical/assembly review](../ELECTRICAL_ASSEMBLY_REVIEW.md) covers bounded software generation, not proven board operation. Firmware, rail integrity, thermal/ambient accuracy, assembly orientation/process, fabrication and bench qualification remain separate.

No additional implementation defect was established by this scoped follow-up. That is not a claim of exhaustive correctness.

## Goal requirement audit

| Requirement | Current authoritative evidence | Status |
|---|---|---|
| Restore green CI without changing frozen evidence | Published head `bda95356`: eight PR-triggered workflows successful; [standard CI 35084038501](https://github.com/dshills/KiCadAI/actions/runs/35084038501) has 25 successful jobs, including historical-source guards, lint, and the executed coverage merge/floor step | Proven for the published candidate-07 head, **not** unpublished candidate 08 or merged main |
| A second genuinely distinct sensor/controller board | Reviewed BMP280 pressure and SHT31 temperature/humidity designs; five native builds and exact native hashes | Verified for these two bounded board families; not arbitrary circuit synthesis |
| One reliable natural-language-to-KiCad workflow | Experimental `partitioned-v7` command, 14 synthetic cases and five real native handoffs | Integration verified; **live language reliability unproven** |
| Complete native/BOM/preview/Gerber/drill/placement bundles | Five fresh 14-gate results, 201 compared deliverables and 256 authenticated published files | Verified for synthetic supported selections |
| No manual output repair | Raw outputs and manifests retained; comparison normalizes only declared timestamps in memory | Verified for this checkpoint |
| Final reviewed results | This source-bound implementing-agent review and preserved failed historical results | Offline review complete; live acceptance and candidate-head CI/publication remain |
| New live evaluation only with explicit budget approval | No new API requests; all old evaluation allowances preserved | No execution authority for candidate 08 |

PR #14 was re-read and remains open/draft, unmerged, at `bda9535651d4ffc12cfa1a8c4f966d0aa79be638`. Its body describes older evaluations; this local checkpoint does not update it. The current local branch remains isolated from the user's primary checkout.

## Reproduce or authenticate

All following commands are offline. Use the pinned Go 1.26.8 toolchain and remove provider credentials. Set `KICADAI_OFFLINE_NATIVE_CLI` to the existing KiCad 10.0.3 executable. Tests normally use temporary directories. To retain new native artifacts, first create a **new** absolute directory and set `KICADAI_PARTITIONED_REVIEW_ROOT` to it. The tests refuse existing per-run directories.

```sh
go test ./cmd/kicadai-board-family \
  -run '^TestGrounded(OfflineReviewFixtures|CandidateCommandNative)$' -count=1 -json
```

The test command alone does not write a capture receipt. The local recorded runner `.cache/partitioned-08-review-01/run.mjs` produced `native-receipt.json` and both streams in addition to fixtures and native directories; its fixed output directory must not be reused. That receipt is required by the verifier. The following commands authenticate this **existing** checkpoint without repeating native work. The installed Ajv dependency is resolved locally; this review does not install packages or download a tokenizer.

```sh
node specs/board-family-v2/partitioned-intent-08/review-offline.mjs \
  .cache/partitioned-08-review-01 --check
KICADAI_PARTITIONED_REVIEW_ROOT="$PWD/.cache/partitioned-08-review-01" \
  node --test specs/board-family-v2/partitioned-intent-08/review-offline.test.mjs
```

`review.json` binds 92 Go source/test files and 334 local evidence files, plus the corpus, example publication, verifier, normalization recipe and Ajv entry-point identities. Guard tests copy evidence to temporary directories and reject missing/mutated fixtures, altered native bytes, coherent local manifest rehashing, extra deliverables/evidence and symlinks. These copied negative controls do not modify retained outputs.

The final `.cache/partitioned-08-review-checks/verification.json` binds 97 source/recipe inputs and seven successful commands. It records 1,654 three-package short-regression test/subtest passes, 156 candidate race-check passes, bounded fuzzing, three-package vet/lint, all eight review-guard tests, and successful replay of the completed offline review. No package check here is represented as candidate-head remote CI.

## Next execution boundary

Prepare a separate candidate-08 evaluation runtime and fixed runner offline; the candidate-07 runner/contract must not be relabelled or reused. Freeze the exact request bodies, schema, executable, corpus, accounting safeguards and original acceptance criteria. Then request one explicit authorization package for that bounded run, including any genuinely required executable-specific network rule. No approval is inferred from earlier budget or firewall approvals. Publication and merge remain separate actions.

The API-key skill preserved the user's existing reuse decision while all tests removed real credentials and substituted only in-memory synthetic transports. The OpenAI Docs skill informed the output-size caveats above. Neither guidance changes the original success criteria.
