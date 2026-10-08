# Publication CI setup correction 03

The [publication-02 Linux CI run](https://github.com/dshills/KiCadAI/actions/runs/34865770709) passed all eight new tamper tests and authenticated the complete new publication, preserving raw 5/14 and application 7/14. Its additional historical final-01 audit then failed because `.cache/board-family-v1/live-ledger.json` does not exist in a fresh checkout. Standard CI already creates this exact exhausted-ledger fixture; the new separate job omitted that setup.

The correction adds the same CI-only, exclusive copy from the published historical ledger, checking its frozen SHA-256 and 41 consumed entries first. It refuses to overwrite existing history. This is a disposable CI fixture, not a live ledger reset, new budget, weakened check or provider request. The historical audit remains enabled.

Original publication manifest 02, its 203 hashed files, all evaluated inputs, responses, ledgers, stop checkpoints, native outputs, reviews and scores remain byte-identical. The original workflow is archived at `checkpoints/publication-02-workflow.yml` and checked against manifest 02's original hash. `PUBLICATION-CI-03.json` binds the corrected workflow, preserved workflow and successor read-only checker. No production code, frozen evaluation/runtime workflow, API model/schema or paid behavior changes.

For the current checkout use:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  node specs/board-family-v2/typed-evaluation-02/authenticate-publication-03.mjs --check
```

The original `authenticate-final.mjs --check` remains unchanged for publication commit b0735ebf, whose current-workflow binding is intentionally historical. The successor authenticates that old workflow from its preserved copy while binding the corrected current workflow separately. It retains the entire original evidence verification, all six checkpoint-prefix checks and the failed conclusion. This addendum supersedes only the current-checkout command in RESULTS-02.md, not any evaluation claim.

The Linux failure is a publication setup defect, not a different live result. No retry of any evaluated request or extra API call is authorized or performed. The failed workflow log is retained at `checkpoints/publication-02-ci-failure.log`. Native whitespace and original logs remain untouched.
