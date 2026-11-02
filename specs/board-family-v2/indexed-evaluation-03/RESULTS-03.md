# Indexed-v3 final evaluation: stopped, acceptance not met

Date: 2026-09-15. Evaluated source:
`8744d7fc032452b2683afac0e5f9082f963de164`.
Reviewer: implementing Codex agent, not an independent reviewer.

The approved final batch made **one physical request**, then stopped without a
retry. Thirteen of fourteen planned cases were not attempted. No native board
bundle was produced. Complete acceptance is **not met**; do not promote
indexed-v3, merge on the basis of this evaluation, or claim the full goal done.

## Outcome and accounting

| Measure | Recorded result |
| --- | --- |
| Approved limit | 14 physical requests; USD 1 total; no retries, probes, or recovery batch |
| Actual launches / reserved requests | 1 / 1 |
| HTTP result | 200; 45,096 response bytes; EOF observed; no transport/read/close error or truncation |
| Frozen evidence-auditor result | Failed: `journal has no complete verified pinned response` |
| Collector | `stopped-no-retry`; terminal exit 1; 5.371 seconds |
| Accepted recorded outcomes | 0 |
| Unattempted cases | 13, retained in the planned denominator |
| Complete passes / native bundles | 0/14 / 0 |
| All-five-useful timing target | Not established; four useful cases were not attempted |
| Settled usage / actual charge | Unverified / not established |
| Conservative retained reservation | USD 0.05; no refund or ledger rewrite |

Zero complete passes is the frozen full-denominator result, **not a claim that
the model answered all fourteen cases incorrectly**. Only one response exists.
Unused request slots and dollars do not authorize resuming this stopped batch.

## Findings

1. **High severity, high confidence: the evidence validator used the wrong
   nesting boundary.** The API echoed the supplied JSON schema inside
   `response.created`, `response.in_progress`, and `response.completed`. Each
   reaches JSON value depth 13 at
   `$.response.text.format.schema.properties.facts.items.anyOf.0.properties.kind.enum.0`.
   `checkReferencedStreamJSON` calls `validateIntentJSON`, whose depth limit is
   12. The first event therefore fails with the generic stream-validation error.
   This is a local compatibility defect, not a firewall, authentication, quota,
   or missing-response failure. Synthetic fixtures did not exercise the full
   schema echoed inside the real event envelope. A future offline correction
   should separate bounded transport-envelope validation from the stricter
   intent-output boundary while retaining duplicate-key and resource checks.

2. **High severity, high confidence: the retained extraction is also
   semantically wrong.** The prompt asks for a *wired pressure monitor* and the
   standard profile. Pressure and standard-profile facts are supported, but
   the model invents an affirmative `wireless_operation` requirement and emits
   a named `BMP280` sensor fact although the user never named that sensor.
   The extraction contract explicitly leaves family inference to the
   application. Fixing stream validation alone would not make this answer
   faithful. The extra facts remain in the evidence and fail source-bound review.

3. **The safety stop worked, but the user task failed.** The application
   withheld configuration and the collector did not continue after its
   independent auditor failed. No evaluated output was manually repaired.
   This protects against using unverified evidence; it does not demonstrate
   reliable natural-language board creation.

The raw terminal event claims the pinned model and 2,596 input / 100 output /
2,696 total tokens. These fields are retained as **diagnostic claims only**:
the frozen validator did not accept them. They are not substituted into the
ledger, treated as billing proof, or used to release the reservation.

## Evidence, checks, and limits

- [Approval](approval.json) records the user's actual `approved` reply to the
  explicit bounded evaluation and exact-path hostname/port LuLu rule request.
- [Frozen runtime manifest](runtime-manifest.json) and
  [qualification receipt](runtime-qualification.json) are unchanged copies;
  source/runtime hashes identify the evaluated build, not a corrected successor.
- [Complete batch record](batch/result.json) preserves the one attempt and all
  thirteen unattempted IDs. All **22 original batch files** are copied byte for
  byte under `batch/`, including process observations, raw request/response
  bodies, journals, ledger, and both failure logs.
- [Source-bound review](review.json), [frozen scoring result](results.json), and
  [offline diagnosis](diagnosis.json) keep data integrity, semantic correctness,
  accounting confidence, and completion separate.
- [Reproducible record/check script](record-failure-03.mjs) authenticates the
  manifest, plan, case order, exact inventory, process receipts, every recorded
  file hash, original/copy equality, body/base64 equality, ledger snapshot join,
  actual failure state, review bindings, and the full-denominator score. It
  verifies the stopped record; it **does not override the failed Go auditor** or
  claim accepted provider evidence. `JSON.parse` in the diagnostic depth check
  is not a substitute for strict Go JSON validation.
- [Prior CI completion](../source-reference-candidate-03/CI-03.md) covers the
  evaluated source. These later publication files were not part of that run.

To verify offline from the repository root, with the frozen local runtime and
compiler-source closure still available:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY \
  -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  node specs/board-family-v2/indexed-evaluation-03/record-failure-03.mjs --check
```

The original runtime remains under `.cache/board-family-v2/indexed-runtime-03-01`
and the original batch under `.cache/board-family-v2/indexed-final-03`. This
publication preserves the response and failed result, but does not package a
portable copy of the entire compiler/tool/runtime closure. Local hashes are
consistency checks, not provider signatures or tamper-proof attestations.

The dataset is one attempted case within fourteen known regression cases, not
an independent holdout or representative traffic sample. It supports the two
concrete failure findings above, not a statistical model-quality trend estimate.
Historical typed-v2 results remain unchanged. Software-qualified reference
boards remain separate from this failed live run; no fabrication, calibration,
or bench operation is claimed or authorized.

## Next boundary

Keep this failed result immutable. Use the retained full response to develop a
separate offline transport-envelope regression and review the semantic failure
before proposing any further live work. Do not weaken the intent-output depth
limit globally, relax semantic acceptance, rerun the stopped batch, or treat the
remaining budget as recovery authority. The default command remains typed-v2;
its historical live acceptance also failed. The goal remains incomplete.
