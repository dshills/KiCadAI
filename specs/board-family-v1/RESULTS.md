# Results — supported boards pass; live refusal/clarification acceptance remains open

September 13, 2026. The deterministic-family strategy produced ten correct complete boards from ten fresh supported language requests, with a **5.816 s median** end-to-end runtime. The strict live suite nevertheless **failed**: two unsupported cases and one ambiguous case failed acceptance. Current decision-code replay passes all sixteen recorded responses offline; this does not change the live scores or complete the goal. The single scoped PR remains a draft; no merge or manufactured-hardware claim is authorized.

| Criterion | Evidence-backed result |
|---|---|
| Supported numerical configurations | **10/10** complete projects; no manual output repair |
| Meaningful electrical profiles | **3**: 4.7 kΩ/100 kHz, 2.2 kΩ/400 kHz, 10 kΩ/100 kHz |
| Clean deterministic replays | **3/3 byte-identical** native/config/BOM outputs |
| Offline end-to-end runtime | Median **3.273 s**, maximum **3.564 s** |
| Fresh supported first selections / completed boards | **10/10 / 10/10** |
| Fresh unsupported / ambiguous clean command passes | **2/4 / 1/2** — acceptance failed |
| Strict fresh live suite | **13/16**; failures remain in their denominators |
| Corrected offline decision replay | **16/16** retained real responses; **0 API calls**, no native generation |
| Fresh supported end-to-end runtime | All ten: median **5.816 s**, maximum **8.476 s** |
| Fresh supported generation / validation | Median **0.021554 s / 3.113777 s**, measured separately |
| Native validation | Electrical/reference/BOM, writers, connectivity, KiCad 10.0.3 ERC, strict DRC/parity, round trips, previews and unchanged-input checks passed for every accepted project |
| Full bounded repository regression | Passed in **33.535 s** after final decision-code corrections; unchanged packages may use Go test cache |
| Goal-wide physical API requests | **35/35** approved requests; no slots remain |
| Conservative API accounting | **$0.026006** known estimate + **$0.05** unknown reserve = **$0.076006** |
| Review identity | Implementing Codex agent; no independent engineer, external provider review or bench testing |

## Evidence and reproducibility

The [offline summary](evidence/offline/summary.json) identifies the Apple M4 Pro / ARM64 machine, base/binary/runner/source hashes, ten configuration cases and three clean replays. Offline timing includes generation, installed-KiCad validation and process startup, but not AI inference. The current generator, reference assets, electrical calculations and native validator match the relevant original source hashes; reused qualification is identified explicitly in the [final assessment](evidence/acceptance/assessment.json).

The [final manifest](evidence/acceptance/manifest.json) contains **221 file hashes**, supplementing the unchanged **382** original offline/live hashes. It retains all eighteen new calls, source/binary identities per run, stdout/stderr, raw decisions, response IDs, native reports, the complete 35-entry ledger, corrected offline replay and final regression. Three [delivered examples](../../examples/board-family-v1/README.md) include byte-identical representative language and holdout records. Every newly generated supported project's native files and BOM match the reviewed profile example; its numerical configuration and electrical calculation are separately retained.

Hashes establish local integrity and provenance, **not third-party authentication**. Original execution paths in native reports are preserved. The incorrectly generated resize is recorded as rejected, not offered as a successful request. No evaluated native output was manually repaired.

Run the read-only audit from the repository root:

```sh
node specs/board-family-v1/evaluation/verify-evidence.mjs
```

The audit checks original and new manifests, response/ledger bindings, configuration/disposition counts, costs and replay provenance. Its successful exit means **the evidence reconciles**, not that live acceptance passed. `development/replay-decisions` also reproduces the current decoder test offline from retained local raw runs; it is not a provider accuracy measurement.

## Live attempts, failures and repairs

The implementing agent authored each evaluation set before its first call. Literal prompts were unseen by the selector; expected answers, other cases, native geometry and source files were never sent. This is not an independently authored benchmark and does not replace the historical practical-board corpus.

The original set consumed seventeen calls including an explicit recovery. It remains **8/10 correct first selections, 7/10 first complete boards, 4/4 refusals and 1/2 clarifications**. Failures were a legacy-envelope integration error, an incorrect refusal of an excluded requirement, clause-boundary whitespace rejection, and a whole-board energy request incorrectly treated as pull-up-current optimization. Eight supported projects ultimately completed; conditional median/max time was 6.105/7.315 s. The first call lost selection/usage metadata and retains its entire reserve. See the unchanged [original assessment](evidence/live/assessment.json).

The [approved extension](evaluation/AUTHORIZATION.md) allowed two recorded development checks and sixteen fresh cases with the exact revised contract and holdout. Both development checks passed but do not count as unseen attempts. All ten fresh supported boards passed under source `b5731a56`; later local corrections mean the entire live suite is **not a single-binary trial**.

- **`fresh-unsupported-02`:** correct overall refusal of direct 5 V power, null configuration, but inconsistent clause tags caused a local command error. No board was generated. Null-configuration refusal/clarification responses now preserve the whole original request in code, while retaining raw model annotations separately.
- **`fresh-unsupported-04`:** the model falsely accepted an 80×60 mm resize and generated the unchanged 120×80 mm reference. Native validity did not establish request validity. A deterministic guard now rejects common explicit conflicting dimension/layer forms. This is not a general natural-language parser; unusual paraphrases can escape the guard and negated dimensions may be conservatively refused.
- **`fresh-ambiguous-01`:** correct targeted clarification, null configuration, but omitted request text caused a local command error. Whole-request preservation fixes this without inventing a design or asserting that every raw model clause was correct.

The final unseen ambiguous request passed under source `0e791b14`. Replaying all sixteen actual responses through that source passes **10 supported configurations, 4 refusals and 2 clarifications offline**. Supported configurations remain identical to those originally generated. The replay neither regenerates/repairs native boards nor retroactively converts the three live failures into passes. A fresh live test of the repaired rejection/clarification handling remains outstanding.

## Cost, review and remaining barrier

Known usage across 34 responses is **37,443 input / 6,883 output tokens**. Estimates use full input price, ignore cache discounts and round each call upward to a micro-dollar at [published model pricing](https://developers.openai.com/api/docs/models/gpt-4.1-mini). One response remains unknown with a $0.05 reserve. **$0.076006 is conservative goal accounting, not an invoice or account-wide spend.** The request ceiling, not the dollar ceiling, is exhausted. No further call or replacement ledger is authorized.

The complete bounded integration tier (`go test -short -p=1 -timeout 20m ./...`) passed; this matches `make test` / `make test-bounded`, not the exhaustive release tier. [Review](REVIEW.md) records source review, focused tests, recorded-response regression and actual schematic/PCB readability review. [Family limits](FAMILY.md) disclose four default-ignored ERC and five default-ignored DRC categories, crowded grouped-ground labels, incomplete assembly silkscreen, electrical assumptions and missing firmware/hardware measurements.

Unaffected implementation, documentation, examples and review are ready in the draft PR. Completion still needs a passing live rejection/clarification check under the repaired implementation or an **explicit user decision** to accept the disclosed mixed live/offline evidence. Neither is silently assumed. Original failures remain permanently recorded; the goal is not complete merely because this report or the PR exists.

The smallest proposed follow-up is [six new rejection/clarification prompts](evaluation/guardrail-followup-proposed.json): four unsupported and two ambiguous, exactly one call each, unchanged contract/key/ledger, no rerun of the successful ten supported cases. This would require **41 total requests, still $10 total**. It is **not authorized or executed**. Only the request cap and targeted runner would change; decision/generation behavior remains frozen for that check.
