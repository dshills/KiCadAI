# Results — deterministic lane passes; language acceptance does not

September 13, 2026. This family generates useful native boards quickly, but the original language evaluation **does not meet the goal**. All failures remain in the denominator. No final PR has been opened and the goal is not complete. This does not replace the historical practical-board corpus.

| Criterion | Evidence-backed result |
|---|---|
| Complete supported configuration projects | **10/10** passed without output repair |
| Meaningful electrical profiles | **3**: 4.7 kΩ/100 kHz, 2.2 kΩ/400 kHz, 10 kΩ/100 kHz |
| Clean native/config/BOM replays | **3/3 byte-identical** |
| End-to-end offline wall time | Median **3.273 s**, maximum **3.564 s** |
| Native validation | All 13 original/replay runs passed electrical contract, reference/BOM, writers, connectivity, KiCad 10.0.3 ERC, strict DRC/parity, round trips and unchanged-native-input hashes |
| Correct first-attempt language selections | **8/10**; target ≥9/10 — **fail** |
| First-attempt completed language projects | **7/10**; one additional correct selection failed decoding |
| Latest complete supported language projects | **8/10**, including the explicit `nl-01` recovery |
| Unsupported / ambiguous requests | **4/4 refused / 1/2 clarified** — ambiguity target **failed** |
| Completed supported language timing | **8 cases only:** median **6.105 s**, maximum **7.315 s** |
| Generation / validation time | For those 8 completions: median **0.017 s / 3.256 s** |
| Live usage | **17/20 physical requests**, 16 known usage records plus 1 unknown |
| Conservative API accounting | **$0.011711** estimated known usage + **$0.05** unknown reserve = **$0.061711**; not an invoice |
| Review | Implementing-agent code/reference/readability review; no independent hardware reviewer or bench testing |
| Final PR | Not opened; final live evidence/review still required |

[The offline summary](evidence/offline/summary.json) records the Apple M4 Pro / ARM64 machine, source/base/binary/runner hashes, all ten configurations and three replays. Its timing includes generation, installed-KiCad validation and subprocess startup, but not AI inference. The generator, reference geometry, numerical checks and native validator have not changed since this acceptance run; that evidence is reused under its original identities, not represented as testing later language corrections.

[The manifest](evidence/offline/manifest.json) verifies unchanged copies of the run evidence and [three example projects](../../examples/board-family-v1/README.md). These hashes establish local integrity and provenance, not third-party authentication. Original absolute execution paths in KiCad reports are preserved honestly. The remaining seven original projects and three replay projects are retained in the local source-run directory; their check records and complete artifact hashes are included here.

Repository integration uses the complete **bounded** tier (`go test -short -p=1 -timeout 20m ./...`), matching `make test` / `make test-bounded`. The initial run took 996.634 s; after provider changes it passed in 74.174 s. Subsequent unchanged packages used the test cache. Current-source focused tests, recorded-response replays and `go vet` pass. Integration receipts are under `evidence/regression/`. The exhaustive release tier is not implied.

The implementing agent checked actual rendered schematics for all profiles and the revised PCB. [Family limits and review caveats](FAMILY.md) cover crowded ground-pin labels, incomplete assembly silkscreen, electrical assumptions, assembly/firmware obligations and absent hardware-performance proof.

## Original language failures and corrections

The sixteen literal prompts and expected values were declared before the first call. They were authored by the implementing agent and unseen by the selector, not an independent benchmark. Expected answers, other cases, geometry and source files were never model input. All live runs used the same approved contract/schema and pinned model; local code changed between interrupted runs, so this is not a single-binary reliability trial.

- **`nl-01`, original attempt:** the provider returned a response, but the decoder expected a legacy intent envelope instead of the requested decision object. Selection and usage were lost. The unknown first attempt retains its full reserve. A separate structured-object decoding path fixed the integration without changing the outgoing payload. The explicit recovery passed in 6.063 s, not a new first-shot success.
- **`nl-03`:** the model incorrectly refused an explicit exclusion of whole-board power/battery requirements. No board was produced. Revised instructions distinguish exclusions from positive requirements; that change is **not live-tested**.
- **`nl-05`:** correct fast configuration, but a missing space between otherwise verbatim clauses caused local rejection before generation. The decoder now restores only original boundary whitespace, never words or punctuation. Offline replay of this recorded response passes decoding; the original failure remains.
- **`ambiguous-02`:** a general low-power goal was wrongly substituted with reduced I²C pull-up current, and a board was generated. It passed native checks but failed request acceptance. Its output is retained as rejected evidence, not a successful example. A deterministic gate now requires explicit low-current/pull-up scope; offline replay of the original response becomes a targeted clarification. Fresh live accuracy remains unmeasured.

## Evidence audit and limitations

The [live assessment](evidence/live/assessment.json), [raw-run manifest](evidence/live/manifest.json), and [ledger snapshot](evidence/live/ledger.json) retain all seventeen attempts, available response IDs/usage, original stdout/stderr, native reports and first/latest outcomes. All eight successful supported projects passed thirteen checks and match the reviewed profile examples' native hashes. Three examples include unchanged `language-selection.json` and `language-validation.json` from successful language runs. Hashes establish local integrity/provenance, not independent authentication or hardware performance.

Run the read-only audit from the repository root:

```sh
node specs/board-family-v1/evaluation/verify-evidence.mjs
```

It recomputes selection/acceptance counts from original selections, reconciles request indices and usage with the ledger, and verifies **382 file hashes**. A first-selection match means exact numerical configuration agreement; complete-board acceptance additionally requires a successful command and full validation. Unknown initial selection is conservatively a miss. All ten supported requests remain in reliability denominators. Only actual successful completions enter the explicitly conditional timing statistics; these exclude debugging and cannot establish the all-ten-case timing target.

Known usage is **16,846 input / 3,102 output tokens** across sixteen calls. Estimates use full input price, ignore cache discounts, and round each call upward to a micro-dollar at [published model pricing](https://developers.openai.com/api/docs/models/gpt-4.1-mini). The other call's usage is unknown. The ledger does not monitor other applications or account-level spend; provider billing is authoritative.

Native checks use all severities, all track errors and schematic parity, without adding per-finding exclusions. Reports still list **four default-ignored ERC and five default-ignored DRC categories**; [FAMILY.md](FAMILY.md) names them. Earlier absolute wording that no check was disabled was too broad and is corrected. No manufactured hardware, firmware, assembly approval or general semantic guarantee is inferred.

## Remaining authority and acceptance barrier

Only **three requests** remain under the original ceiling. Retrying the same ten prompts cannot repair their first-attempt result, and that set has no unseen cases left.

Proposed next validation: up to **two recorded development checks** (`nl-03`, `ambiguous-02`), followed by **one fresh 16-case holdout** (10 supported, 4 unsupported, 2 ambiguous), using the [revised instructions](evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json) and [proposed prompts](evaluation/language-holdout-proposed.json). This requires a **35-request total ceiling**, retaining **$10 total**, the existing key, the same ledger and every original failure. These calls are **not authorized or executed**. No other provider, firewall change, fabrication or purchase is proposed. The one final PR remains pending successful acceptance and final review.
