# Results — board-family milestone passes staged functional acceptance

September 13, 2026. The deterministic-family strategy produced ten correct complete boards from ten fresh supported language requests, with a **5.816 s median** end-to-end runtime. A separately approved six-request live follow-up passed **4/4 unsupported refusals and 2/2 targeted clarifications** under the corrected implementation. Together with unchanged-source replay and complete-board revalidation, the evidence meets this bounded milestone's functional targets. This is **staged acceptance, not a newly executed single-binary 16/16 trial**. Earlier failed suites remain failed. The single scoped PR is reviewed by its implementing agent and remains unmerged; inherited CI failures and missing hardware testing are not waived.

| Criterion | Evidence-backed result |
|---|---|
| Supported numerical configurations | **10/10** complete projects; no manual output repair |
| Meaningful electrical profiles | **3**: 4.7 kΩ/100 kHz, 2.2 kΩ/400 kHz, 10 kΩ/100 kHz |
| Clean deterministic replays | **3/3 byte-identical** native/config/BOM outputs |
| Current offline end-to-end runtime | Median **3.304 s**, maximum **3.581 s** after CI error-handling fixes |
| Fresh supported first selections / completed boards | **10/10 / 10/10** |
| Earlier fresh unsupported / ambiguous clean command passes | **2/4 / 1/2** — that trial remains failed |
| Earlier strict fresh live suite | **13/16**; failures remain in their denominators |
| Corrected offline decision replay | **16/16** retained real responses; **0 API calls**, no native generation |
| Separately approved fresh live guardrail follow-up | **4/4 unsupported / 2/2 clarifications**; null configurations and no native outputs |
| Fresh supported end-to-end runtime | All ten: median **5.816 s**, maximum **8.476 s** |
| Fresh supported generation / validation | Median **0.021554 s / 3.113777 s**, measured separately |
| Native validation | Electrical/reference/BOM, writers, connectivity, KiCad 10.0.3 ERC, strict DRC/parity, round trips, previews and unchanged-input checks passed for every accepted project |
| Full bounded repository regression | Passed in **38.144 s** after the final cap-only change; unchanged packages may use Go test cache |
| Goal-wide physical API requests | **41/41** approved requests; no slots remain |
| Conservative API accounting | **$0.030054** known estimate + **$0.05** unknown reserve = **$0.080054** |
| Review identity | Implementing Codex agent; no independent engineer, external provider review or bench testing |

## Evidence and reproducibility

The [original offline summary](evidence/offline/summary.json) identifies the Apple M4 Pro / ARM64 machine, base/binary/runner/source hashes, ten configuration cases and three clean replays. Offline timing includes generation, installed-KiCad validation and process startup, but not AI inference. Generator, reference assets and electrical calculations retain their original hashes. After CI found unchecked cleanup errors, command/ledger/validator error handling was corrected without changing decision or design behavior. [Current offline revalidation](evidence/integration/summary.json) repeats all ten cases and three clean replays; **every native/configuration/electrical/BOM/library hash is identical to its original counterpart**. Its 43-file [integration manifest](evidence/integration/manifest.json) preserves actual reports, current source identities and the new full regression. Original qualification and live evidence remain unchanged.

The [live/correction manifest](evidence/acceptance/manifest.json) contains **221 file hashes**, supplementing the unchanged **382** original offline/live hashes. It retains all eighteen calls from that extension, source/binary identities per run, stdout/stderr, raw decisions, response IDs, native reports, the complete 35-entry ledger, corrected offline replay and its regression. With 43 offline integration records and 31 [live guardrail records](evidence/guardrails/manifest.json), the audit checks **677 hashes**. The new records retain all six prompts/responses, the executed contract, before/after ledgers and final regression. Three [delivered examples](../../examples/board-family-v1/README.md) include byte-identical representative language and holdout records. Every newly generated supported project's native files and BOM match the reviewed profile example; its numerical configuration and electrical calculation are separately retained.

Hashes establish local integrity and provenance, **not third-party authentication**. Original execution paths in native reports are preserved. The incorrectly generated resize is recorded as rejected, not offered as a successful request. No evaluated native output was manually repaired.

Run the read-only audit from the repository root:

```sh
node specs/board-family-v1/evaluation/verify-evidence.mjs
```

The audit checks original and new manifests, response/ledger bindings, configuration/disposition counts, costs, replay provenance and the final cap-only source lineage. Its successful exit means **the evidence reconciles**; acceptance also relies on the recorded actual outcomes and disclosed semantic/readability review. `development/replay-decisions` reproduces the current decoder test offline from retained local raw runs; it is not a provider accuracy measurement.

## Live attempts, failures and repairs

The implementing agent authored each evaluation set before its first call. Literal prompts were unseen by the selector; expected answers, other cases, native geometry and source files were never sent. This is not an independently authored benchmark and does not replace the historical practical-board corpus.

The original set consumed seventeen calls including an explicit recovery. It remains **8/10 correct first selections, 7/10 first complete boards, 4/4 refusals and 1/2 clarifications**. Failures were a legacy-envelope integration error, an incorrect refusal of an excluded requirement, clause-boundary whitespace rejection, and a whole-board energy request incorrectly treated as pull-up-current optimization. Eight supported projects ultimately completed; conditional median/max time was 6.105/7.315 s. The first call lost selection/usage metadata and retains its entire reserve. See the unchanged [original assessment](evidence/live/assessment.json).

The [approved extension](evaluation/AUTHORIZATION.md) allowed two recorded development checks and sixteen fresh cases with the exact revised contract and holdout. Both development checks passed but do not count as unseen attempts. All ten fresh supported boards passed under source `b5731a56`; later local corrections mean the entire live suite is **not a single-binary trial**.

- **`fresh-unsupported-02`:** correct overall refusal of direct 5 V power, null configuration, but inconsistent clause tags caused a local command error. No board was generated. Null-configuration refusal/clarification responses now preserve the whole original request in code, while retaining raw model annotations separately.
- **`fresh-unsupported-04`:** the model falsely accepted an 80×60 mm resize and generated the unchanged 120×80 mm reference. Native validity did not establish request validity. A deterministic guard now rejects common explicit conflicting dimension/layer forms. This is not a general natural-language parser; unusual paraphrases can escape the guard and negated dimensions may be conservatively refused.
- **`fresh-ambiguous-01`:** correct targeted clarification, null configuration, but omitted request text caused a local command error. Whole-request preservation fixes this without inventing a design or asserting that every raw model clause was correct.

The final unseen ambiguous request passed under source `0e791b14`. Replaying all sixteen actual responses through that source passes **10 supported configurations, 4 refusals and 2 clarifications offline**. Supported configurations remain identical to those originally generated. The replay neither regenerates/repairs native boards nor retroactively converts the three live failures into passes.

The user then approved exactly [six new predefined guardrail prompts](evaluation/guardrail-followup-proposed.json), keeping the contract/key/ledger/$10 limit and increasing only the total request ceiling to 41. Source `2b85d7698f2fb5c8550c59621e7f81ce666611e9` changes production only in the request-cap literal and its help text. The runner and read-only audit verify that lineage against `faca271b`; decision, generation, electrical and validation behavior stay frozen. All six first calls passed, with no retries or native outputs. The four explanations reject Bluetooth, direct 12 V, a pump relay and a 90×50 mm outline. The two questions clarify speed versus pull-up-current priority and unspecified measurement interval/active loads. The implementing agent inspected the raw responses, final decisions and complete original prompts. See the [separate assessment](evidence/guardrails/assessment.json); it explicitly preserves both older failed trial flags.

## Cost, review and integration limitations

Known usage across 40 responses is **44,274 input / 7,704 output tokens**. Estimates use full input price, ignore cache discounts and round each call upward to a micro-dollar at [published model pricing](https://developers.openai.com/api/docs/models/gpt-4.1-mini): $0.40/million input tokens and $1.60/million output tokens. One response remains unknown with a $0.05 reserve. **$0.080054 is conservative goal accounting, not an invoice or account-wide spend.** All 41 approved calls are consumed. The original 35 ledger records are unchanged; no further call or replacement ledger is authorized.

The complete bounded integration tier (`go test -short -p=1 -timeout 20m ./...`) passed; this matches `make test` / `make test-bounded`, not the exhaustive release tier. New/affected-scope lint passes. Repository-wide lint still reports two unchecked-close findings in unchanged historical files, independently verified in the already-failed [main-branch CI run](https://github.com/dshills/KiCadAI/actions/runs/34755942929). Their bytes and lint rules were not changed or waived. [Review](REVIEW.md) records source review, focused tests, recorded-response regression and actual schematic/PCB readability review. [Family limits](FAMILY.md) disclose four default-ignored ERC and five default-ignored DRC categories, crowded grouped-ground labels, incomplete assembly silkscreen, electrical assumptions and missing firmware/hardware measurements.

[CI run 34765619297](https://github.com/dshills/KiCadAI/actions/runs/34765619297) on `faca271b` completed with **23 passing jobs, one failed static job and one skipped dependent Offline quality gates job**. All ten coverage shards, eleven frozen-corpus jobs, the reachable-vulnerability check and external regression ladder passed. The CI coverage merge/floor did **not** execute. This is not green CI or merge approval; check [PR #13](https://github.com/dshills/KiCadAI/pull/13) for subsequent head status. The inherited historical findings are a separate repository-integration issue, not a board-family test failure, and no frozen source or lint policy was changed to suppress them.

## Completion audit

The family definition supplies one integrated, manufacturer-backed reference and three electrical profiles. The deterministic command and native examples satisfy complete-board generation, all thirteen applicable validation checks, ten numerical cases and three clean replays. The ten fresh supported language first attempts exceed the 9/10 selection target; the separate six-request live follow-up closes the refusal/clarification requirement without rerunning those successes. Measured supported times are far below the 300/600-second limits. The current full bounded regression, integrity/accounting audit and implementing-agent code/readability review pass. Commands, operating limits, projects, previews, BOMs, results and review are delivered in the one scoped PR, with no merge.

Acceptance applies only to this small, agent-authored board-family evaluation and its source-linked stages. It is not independent model-accuracy certification, a pass for the historical general-purpose benchmark, universal request understanding or proof of manufactured-board performance. Further live use needs new request authorization; deterministic configuration-file generation remains available without paid calls.
