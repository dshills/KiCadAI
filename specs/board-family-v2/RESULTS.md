# Two-family final evaluation: complete evidence, acceptance failed

The frozen September 14, 2026 batch completed all **14 first attempts**. The application met **8/14** cases; **4/14** raw provider decisions met the complete frozen contract. The required 14/14 acceptance was **not achieved**. In particular, an unsupported continuous-heater request produced a software-valid SHT31 board. **Do not release this natural-language workflow for unattended use.**

Deterministic construction remains fast and repeatable for explicitly supported configurations. This result does not justify changing the frozen criteria, repairing outputs, rerunning failed cases, or declaring the goal complete.

## Scope and accounting

- Evaluation: [14 frozen cases](evaluation/cases-01.json), [protocol](evaluation/PROTOCOL-01.md), [qualified runtime](evaluation/runtime-01.json). Executed production code is the `ab1a224d` checkpoint, unchanged throughout collection.
- [Actual API approval](evaluation/APPROVAL-01.json): existing key, at most 14 physical requests and USD 1.00. [Separate firewall approval](evaluation/FIREWALL-APPROVAL-01.json): only the qualified evaluator → `api.openai.com:443` Allow rule; no other settings changed.
- Pinned model: `gpt-4.1-mini-2025-04-14`; 14 completed provider responses, 14 unique response IDs and 14 ledger entries, no probe, retry, model/prompt/runtime change or recovery baseline. All 14 request slots are exhausted.
- Recorded usage: **21,952 input tokens**, **3,629 output tokens**. Sum of per-request, upward-rounded estimates: **USD 0.014593**. Pricing uses the [official model page](https://developers.openai.com/api/docs/models/gpt-4.1-mini), rechecked September 14: USD 0.40/M input and 1.60/M output, without cached-input discounts. This is an estimate from returned usage, not a billing reconciliation.
- Permanent budget reservations: **USD 0.70**. Low actual cost does not restore a request slot or authorize another batch.
- Collection ran **11:40:51.712–11:46:05.002 UTC**, 313.290 seconds including inspection between stops. Summed case execution was 74.980 seconds. Five application errors caused stops; four verified resumptions collected only unseen suffixes ([01](evaluation/RESUME-01.json), [02](evaluation/RESUME-02.json), [03](evaluation/RESUME-03.json), [04](evaluation/RESUME-04.json)). The final failed case was already the last case; raw state correctly remains `stopped`.

## Case-by-case result

“Raw” requires exact original clause concatenation, consistent dispositions, correct configuration/nullability, and truthful meaning. “Application” additionally requires successful execution and the required output/no-output behavior. Local clause restoration cannot turn a raw miss into a raw pass. A saved decoded candidate from a failed command is not an admitted response.

| Case | Raw | Application | Observed result |
|---|---|---|---|
| supported-01: BMP280 standard | Fail | Pass | Correct configuration; raw boundary spaces restored locally; validated board. |
| supported-02: BMP280 fast, exclusions | Fail | Fail | Duplicated exclusion clause and contradictory unsupported label; app rejected, no board. |
| supported-03: BMP280 low_current | Fail | Pass | Correct pull-up-only scope; boundary spaces restored; validated board. |
| supported-04: SHT31 standard | Pass | Pass | Correct 70 pF configuration and explicit exclusions; validated board. |
| supported-05: SHT31 fast | Fail | Pass | Correct 100 pF, heater-off configuration; boundary spaces restored; validated board. |
| clarify-01: unspecified measurement | Fail | Fail | Relevant question, but non-null BMP280 configuration and supported clause; rejected. |
| clarify-02: either sensor | Pass | Pass | Targeted measurement/family choice, null configuration, no board. |
| clarify-03: either profile | Fail | Pass | Targeted standard/fast question; reordered raw clauses replaced locally with original prompt; no board. |
| unsupported-01: both sensors | Pass | Pass | Correct combined-sensor refusal; no substitution or board. |
| unsupported-02: SHT31 standard 100 pF | Fail | Fail | Model treated request as permission to exceed 70 pF; local numeric guard rejected. |
| unsupported-03: SHT31 low_current | Pass | Pass | Correct wrong-family-profile refusal; no board. |
| unsupported-04: continuous heater | Fail | Fail | Incorrectly accepted as firmware behavior; generated a board despite heater-off qualification. |
| unsupported-05: direct 5 V USB + wireless | Fail | Fail | Refused wireless but invented regulation for direct USB; malformed clauses/non-null config; rejected. |
| unsupported-06: ±0.1 C ambient guarantee | Fail | Fail | Relevant refusal message contradicted by supported clause/non-null config; rejected. |

Category totals are **4/5 supported builds**, **2/3 clarifications**, and **2/6 proper refusals**. Five of six unsupported requests emitted no board, but three did so through application errors rather than the required correct refusal. The sixth emitted a board. None of these distinctions is hidden inside an aggregate safety claim.

## Speed and native evidence

All five supported attempts, including the failed attempt, remain in the timing population:

| Case | End-to-end seconds | Native bundle |
|---|---:|---|
| supported-01 | 9.602 | Present, 14/14 native gates |
| supported-02 | 6.964 | Absent: rejected before generation |
| supported-03 | 9.514 | Present, 14/14 native gates |
| supported-04 | 8.207 | Present, 14/14 native gates |
| supported-05 | 8.653 | Present, 14/14 native gates |

Median **8.653 seconds**, maximum **9.602 seconds**: timing thresholds pass, but timing does not override failed correctness. These times include interpretation and command execution; they do not include the separate offline qualification or human/agent review.

The four legitimate generated bundles match **162 reviewed deliverable files**, with only the frozen timestamp normalization. The wrongly admitted heater case also passes all 14 native gates and matches the SHT31 standard reference's **42** deliverables. Thus all five actually generated bundles have **204** matching deliverables, but only four satisfy their prompts. The heater artifact is retained exclusively as failed evidence, not a new accepted example or fabrication candidate.

All **677 historical publication files**, **256 existing v2 example files**, the exhausted v1 ledger, the frozen payload/cases and qualified runtime identities were rechecked. Full raw outputs, logs, prompts, structured decisions, ledger and four stop checkpoints are included in the [publication manifest](evidence/final-01/manifest.json). No generated output was manually repaired. The [existing electrical/assembly review](ELECTRICAL_ASSEMBLY_REVIEW.md) remains version-bound and unchanged; byte/content comparison permits review reuse, not a claim of a new independent visual inspection.

## Reproduce the evidence check

From the repository root, with the original frozen ledger fixture available:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY \
  -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  node specs/board-family-v2/evaluation/audit-final.mjs --check
```

This performs no API request. It authenticates published files, all 14 outcomes (including entries the stopped runner did not grade), response/usage accounting, exact case order, unchanged stop prefixes, native manifests, reviewed-output comparisons, manual review bindings and independently recomputed metrics. The original local-runtime preflight separately verified all 1,485 runtime source/tooling/output identities before publication; portable publication checking does not pretend those local executables are present on a fresh machine.

## Decision and limitations

Keep [PR #14](https://github.com/dshills/KiCadAI/pull/14) in draft. [Final review](REVIEW.md) identifies the missing deterministic requirement-admission check and inconsistent decision structure as the next engineering work. No successor batch or extra spending is authorized by these results.

These are implementing-agent-authored targeted tests and implementing-agent review, not an independent holdout, statistical reliability estimate, arbitrary circuit synthesis proof, or independent engineering signoff. Hashes authenticate retained bytes and their relationships; they are not cryptographic provider attestations. `selection.json` retains structured adapter output, not raw HTTPS bytes or the full provider envelope. Physical fabrication, assembly, firmware, thermal behavior, power integrity and bench accuracy remain untested and separately authorized.
