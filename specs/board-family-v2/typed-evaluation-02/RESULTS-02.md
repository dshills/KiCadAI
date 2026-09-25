# Typed-intent live evaluation 02: acceptance failed

The complete frozen batch produced **5/14 faithful raw extractions and 7/14 correct application outcomes**. Only **2/5 useful requests generated boards**. All 14 cases remain in the denominator. The practical natural-language milestone is not complete; keep PR #14 draft and do not use this language workflow unattended.

Collection ran on September 14, 2026, 15:42:31.253–15:50:19.002 UTC. This is one batch of 14 first physical attempts, with six verified continuations of never-attempted cases under the explicitly approved continuation scope. No retry, recovery baseline, probe, output repair, schema/prompt/model change or ledger reset occurred.

## Results by frozen case

| Case | Raw meaning/contract | Application | Wall seconds | Finding |
| --- | --- | --- | ---: | --- |
| useful-01 | Pass | Pass | 6.644 | BMP280 standard: complete validated board |
| useful-02 | Fail | Fail | 3.930 | Invented ellipsis in humidity quote; useful request rejected |
| useful-03 | Fail | Fail | 3.455 | Invented default numeric facts; not-required power/battery constraints strengthened to forbidden |
| useful-04 | Fail | Fail | 5.239 | Non-verbatim humidity quote; useful request rejected |
| useful-05 | Pass | Pass | 6.490 | SHT31 fast with heater forbidden: complete validated board |
| choice-01 | Fail | Fail | 1.150 | Invented custom geometry from indoor-project context; irrelevant refusal |
| choice-02 | Fail | Fail | 2.630 | Repeated named-sensor facts against pronoun-only clause; generic error instead of targeted question |
| choice-03 | Pass | Pass | 1.853 | Targeted question about the unresolved profile |
| refuse-01 | Fail | Fail | 3.323 | Duplicate clause ID and absent source quotes; generic error instead of combined-sensor refusal |
| refuse-02 | Fail | Fail | 2.700 | Repeated identities/quantities grounded only in pronouns; generic error instead of capacitance refusal |
| refuse-03 | Pass | Pass | 2.136 | Correctly refuses SHT31 low_current/10k profile |
| refuse-04 | Fail | Pass | 1.581 | Correctly refuses later heater use, but weakens startup-off prohibition in raw facts |
| refuse-05 | Fail | Pass | 1.822 | Correctly refuses USB and wireless; raw facts invent a GPIO-load prohibition |
| refuse-06 | Pass | Pass | 2.274 | Correctly refuses unmeasured assembled-board accuracy guarantee |

The six execution stops were local extraction-contract validation failures after completed provider responses, not transport, authentication, quota or unknown-accounting failures. Their raw outputs, final fallback decisions and errors are preserved. No unsupported case generated a board in this batch; this observation is not a general safety guarantee.

## Why there are three different raw counts

The runner records **8 automatic raw passes**, because it does not score after a child exits nonzero. The read-only audit scores all saved selections and obtains **10 automatic raw passes**. Source-bound semantic review yields **5 actual raw passes**. The extra automatic passes include wrong feature meanings and source-identity defects that shape/substring checks alone cannot establish. The report uses the semantic score, not the most favorable number. Correct application refusal never repairs the raw score.

Application success means the expected exact configuration or a genuinely targeted, truthful clarification/refusal plus the required output behavior. A generic extraction-error question is not success. Application success is **7/14**: two useful, one clarification, four refusal cases. Complete raw-and-application success is **5/14**.

## Time and cost

- All five useful attempts, including three failures: median **5.239 s**, maximum **6.644 s**, meeting the frozen under-60/under-120-second targets. Most failed attempts never reached native generation; this is not a five-board generation-speed result.
- Successful boards: extraction **2.393 / 2.380 s**; deterministic generation **0.018 / 0.013 s**; validation **4.198 / 4.065 s**; full child wall time **6.644 / 6.490 s**.
- Sum of all 14 child wall times: **45.228 s**. Total collection elapsed time including stop review, checkpointing and verified continuations: **467.749 s**. Do not hide this operational overhead when discussing time-to-completion.
- **14/14 physical requests**, 14 distinct completed response IDs, **34,614 input / 1,978 output tokens**. No unknown outcomes. Estimated usage cost: **USD 0.017016**, summing each request's upward-rounded micro-USD estimate. This is not an invoice or independently signed provider receipt.
- Permanent reservations: **USD 0.70** under the approved **USD 1.00** ceiling. **Zero request slots remain**. Unused dollars do not authorize additional requests. Prior v1 and final-01 budgets remain exhausted and unchanged.

## Native output evidence

Both generated useful boards pass all **14 native/manufacturing gates**, including KiCad 10.0.3 ERC, strict DRC/parity, round trips, writer/connectivity checks, previews and manufacturing exports. The complete native/BOM/electrical/library/preview/manufacturing deliverables match the reviewed examples: **39 BMP280 + 42 SHT31 = 81 compared files**, with only the predeclared timestamp normalization. No generated file was repaired. All 177 final evidence files are copied byte-identically, including the failed responses, logs, per-case ledgers, full state and final ledger.

The other 12 cases contain only selection.json in their output directories. Empty stderr/stdout where applicable is preserved as evidence, not replaced by invented output. The two native successes demonstrate both existing families but do not rescue the failed language acceptance.

## Authentication and review

- Evaluated source head: `9db09dfc29244c4160e83867698e100d5221f55a`. Its [standard CI](https://github.com/dshills/KiCadAI/actions/runs/34862239873) passed 25/25 jobs and its [runner safeguard workflow](https://github.com/dshills/KiCadAI/actions/runs/34862239958) passed 1/1 before execution. Actual coverage merge: 10 shards, 6,675 tests, 174 packages; 79.20% generated-excluded coverage versus unchanged 75.00% floor (raw 75.3%).
- The frozen runtime is a byte-identical copy of the qualified selector, not a new build. Full compiler-selected closure was authenticated: 1,191 files, including 46 successor-qualified files and 1,145 unchanged original-runtime files. No production code changed during collection/publication.
- [APPROVAL-02.json](APPROVAL-02.json) binds actual new user approval, exact budget, model, key-reuse choice, freeze/runtime hashes and untouched-only continuation. [FIREWALL-02.json](FIREWALL-02.json) records the approved exact-executable api.openai.com:443 rule; no broad firewall change.
- [SEMANTIC-REVIEW-02.json](SEMANTIC-REVIEW-02.json) binds each original prompt and selection and every frozen requirement to raw clause/fact references. [AUDIT-RESULT-02.json](AUDIT-RESULT-02.json) preserves the recomputed conclusion. Six pre-resume state/ledger checkpoints remain under [checkpoints](checkpoints).
- Run `env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS node specs/board-family-v2/typed-evaluation-02/authenticate-final.mjs --check` from the repository root for portable, read-only publication verification. It verifies hashes, all 14 attempt/response joins, prefix ledgers, terminal observations, semantic bindings, artifacts, historical examples and complete inventory. A successful authenticator exit means **the failed evidence is intact**, not acceptance passed.

## Limitations and next decision

This was an implementing-agent-authored targeted set after development, not an independent holdout, representative user sample or reliability estimate. Cases and extraction contracts differ from final-01; do not infer statistically meaningful reliability or speed improvement. Review is by the implementing agent, not an independent human or engineering certification. Local hashes authenticate consistency, not provider signatures; the existing provider error path does not archive every raw HTTP-stream byte. Firmware, fabrication, calibration, thermal/power measurements and physical bring-up were not performed or authorized.

The deterministic CAD path remains useful and fast. The brittle part is the AI requirement-extraction contract and its mismatch with local literal grounding. The next engineering decision should be offline: simplify that boundary using these preserved counterexamples, or explicitly choose a structured user-confirmed configuration workflow with a different product goal. Do not silently redefine the current natural-language goal, loosen safety gates, repair evaluated responses or buy another batch to chase a pass.
