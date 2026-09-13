# Reliable board family v1 — reviewed candidate

This new lane configures one engineered ESP32-WROOM-32E-N4 / BMP280 wired pressure-controller board. It does not search for a circuit, place parts, or autoroute during normal generation. The three options change installed I²C pull-up resistors and their permitted clock/loading envelope.

**Status: 10/10 configuration cases and 10/10 fresh supported language requests generated complete validated boards. The fresh strict live suite still failed rejection/clarification gates (2/4 and 1/2). Current decision-code replay passes all 16 retained responses offline; this does not change live scores.** The autonomous goal is not complete. The one scoped PR is a draft pending acceptance; no hardware-performance claim is made.

## Use the deterministic command

Run from the repository root with its pinned Go toolchain and KiCad **10.0.3** installed:

```sh
make board-family
bin/kicadai-board-family \
  --config specs/board-family-v1/development/standard.json \
  --output .cache/my-pressure-board \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```

The output directory must not exist. `fast.json` and `low_current.json` select the other electrical profiles. Linux users can pass their installed `kicad-cli` path, but Linux runtime behavior has not been evaluated here. Other KiCad versions require requalification; the command fails the version gate instead of silently accepting them.

A successful command writes a native project, schematic, fully routed PCB, local symbol/footprint libraries, BOM CSV/JSON, configuration, calculated electrical margins, native ERC/DRC reports, previews and `validation.json`. Open `board.kicad_pro` in KiCad. Exit code zero alone is not a design-pass signal: require stdout `passed:true` and `validation.json` `passed:true`.

## Constrained natural-language command

The command below demonstrates usage. **All 35 approved goal-wide requests are consumed; do not execute another live call, reset the ledger or use a replacement ledger without new authorization.** Configuration-file generation remains available without API use. Do not count offline replay/unit tests as fresh model accuracy.

```sh
bin/kicadai-board-family \
  --prompt 'Build a wired pressure controller with the fast I2C profile.' \
  --ledger .cache/board-family-v1/live-ledger.json \
  --output .cache/my-ai-pressure-board \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```

The existing `OPENAI_API_KEY` is read from the process environment. Never paste it into a prompt, command argument or repository file. Every live invocation for this goal must use the **same ledger path**; do not delete/reset it or create another ledger to recover a failed attempt.

The pinned model is `gpt-4.1-mini-2025-04-14`. Only the literal request and bounded contract/schema are model input; native geometry and source files are not uploaded. Provider credentials are cleared before KiCad subprocesses. Unsupported and ambiguous requests produce `selection.json` with an explanation or question and **no native board**. There are no automatic API retries, redirects or other-provider fallbacks.

User-approved goal-wide ceilings: **35 physical requests / $10** (extended from 20; see [authorization](evaluation/AUTHORIZATION.md)). Each request permanently consumes one count and reserves $0.05 before the HTTP call. Recorded input/output usage gives a conservative estimate at full input price; unknown outcomes retain their reserve. Accounting anomalies persistently halt the ledger. This ledger controls this tool, not other applications or account-level spend. The API provider's actual billing is authoritative. [Official model/pricing](https://developers.openai.com/api/docs/models/gpt-4.1-mini).

The [original payload](evaluation/LIVE_CONTRACT.json) and [original cases](evaluation/language.json) remain unchanged and failed. The [subsequently approved revised instructions](evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json) and [fresh holdout](evaluation/language-holdout-proposed.json) retain their historical proposed names/status text to preserve approved bytes. Two recorded development checks and sixteen fresh cases consumed the remaining eighteen slots. **35/35 requests; $0.076006 conservative accounting**, including the unknown call's reserve. [Results](RESULTS.md) distinguish live failures from offline corrections.

For supported decisions the decoder preserves raw output, restores only original boundary whitespace and rejects omitted words/punctuation. For non-design decisions with no configuration, code retains the entire original prompt instead of relying on model transcription; raw annotations remain available. Deterministic guards reject commonly expressed conflicting dimensions/layer counts and clarify a proposed `low_current` selection without explicit pull-up scope. These are narrow gates, not complete semantic understanding: negated dimensions may conservatively be refused, and unrecognized paraphrases still depend on the model.

## Scope and evidence

- [Family and integrated engineering review](FAMILY.md): components, pin map, envelope, assumptions and unresolved hardware checks.
- [Attempt log](WORK.md): disclosed reference engineering; historical campaigns remain unchanged.
- [Configuration acceptance cases](evaluation/configurations.json) and [runner](evaluation/run-configurations.mjs): ten cases, three clean replays, generation/validation timing and hashes.
- [Current results](RESULTS.md), [original offline](evidence/offline/manifest.json), [original live](evidence/live/manifest.json), and [final evidence](evidence/acceptance/manifest.json) integrity manifests, plus [three complete example projects](../../examples/board-family-v1/README.md).
- `development/` programs engineer new reference copies. They are not production repair steps and must never be run on acceptance outputs.

The implementing Codex agent authored and reviewed this reference. There is no independent human electrical-engineering review or manufactured-board testing. This separate milestone does not pass or replace the historical practical-board benchmark.
