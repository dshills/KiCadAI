# AI Generation

KiCadAI's provider-backed lane converts a natural-language prompt into strict
structured intent, then hands that validated intent to the deterministic
planner, schematic writer, placement/routing workflow, and KiCad checks. The
provider never emits KiCad S-expressions or route geometry directly.

## Supported Reference Lanes

The provider has exactly two promoted reference profiles:

- a protected USB-C-powered BMP280 I2C breakout with an AP2112 3.3 V
  regulator, pull-ups, decoupling, and external I2C connector;
- a protected USB-C-powered active-high LED indicator with fuse, TVS, bulk
  capacitance, and a 5 mA indicator path.

Their checked-in provider inputs are under
`examples/ai/usb_c_bmp280_breakout/` and
`examples/ai/usb_c_led_indicator_protected/`.

Recorded, strict KiCad-backed run:

```sh
kicadai --prompt-file examples/ai/usb_c_bmp280_breakout/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/usb_c_bmp280_breakout/recorded-response.json \
  --output ./out/ai_usb_c_bmp280 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-kicad-roundtrip --strict-diffs \
  design create
```

Replace the `kicad-cli` path for the local installation. A successful strict
run returns `data.ai_status.status: "ready"`; its
`.kicadai/design-promotion.json` has `status: "pass"`.

The CLI response wraps status at `data.ai_status.status`. The persisted compact
summary `.kicadai/validation-summary.json` stores the same value at its root
`status` property.

Recorded protected LED run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/usb_c_led_indicator_protected/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/usb_c_led_indicator_protected/recorded-response.json \
  --output ./out/ai_usb_c_led_protected --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-kicad-roundtrip --strict-diffs \
  design create
```

## OpenAI Provider

Load `OPENAI_API_KEY` into the process environment from the user's shell
configuration or secret manager; do not type the key directly into a command,
or place it in requests, examples, or generated evidence:

```sh
kicadai --prompt-file examples/ai/usb_c_bmp280_breakout/prompt.txt \
  --provider openai \
  --output ./out/live_ai_usb_c_bmp280 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-kicad-roundtrip --strict-diffs \
  design create
```

Live protected LED run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/usb_c_led_indicator_protected/prompt.txt \
  --provider openai \
  --output ./out/live_ai_usb_c_led_protected --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-kicad-roundtrip --strict-diffs \
  design create
```

The selected schema depends only on bounded prompt semantics, never project
names, output paths, fixture paths, or model output.

Optional provider settings include `--model`, `--max-ai-attempts`, and
`--ai-background`. Correction attempts are bounded and restricted to explicit
schema/intent diagnostics. Authentication, refusal, unsupported topology, and
post-generation engineering failures are not sent back to the model for
unbounded retries.

The live provider smoke test is opt-in:

```sh
KICADAI_OPENAI_LIVE_TEST=1 \
  go test ./internal/aiprovider -run '^TestOpenAILive(BMP280|ProtectedLED)Intent$' -count=1 -v
```

## Reproducible Promotion Test

The default suite validates the recorded fixture and metadata without network
or KiCad. To run the real KiCad-backed promotion lane:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
  go test ./cmd/kicadai -run '^TestAIProviderOptionalKiCadPromotion$' -count=1 -v
```

The test uses a unique ignored workspace under `examples/.generated` because
the tested macOS KiCad CLI has been unreliable when its working directory is
removed from the system temporary tree. `t.Cleanup` removes the workspace after
the test; an interrupted process may leave an ignored uniquely named directory.

## Evidence

The generated `.kicadai/` directory includes:

- `ai-request.json`: sanitized provider/model/schema metadata and prompt hash;
- `ai-response.json`: validated normalized intent and response metadata;
- `ai-attempts.json`: bounded attempt history;
- `intent-plan.json` and `generated-request.json`;
- `workflow-result.json` and `design-promotion.json`;
- `validation-summary.json` and `retry-state.json`;
- writer, route-completion, ERC/DRC, and round-trip evidence referenced by the
  workflow and promotion reports.

For an output such as `./out/live_ai_usb_c_led_protected`, inspect
`./out/live_ai_usb_c_led_protected/.kicadai/validation-summary.json`,
`./out/live_ai_usb_c_led_protected/.kicadai/workflow-result.json`, and
`./out/live_ai_usb_c_led_protected/.kicadai/design-promotion.json`. The
workflow result embeds the exact KiCad ERC/DRC commands, versions, finding
counts, and strict writer summary; promotion `pass` is the checked gate result.

Plaintext prompts, API keys, authorization headers, and hidden provider
reasoning are not persisted. The normalized intent plus the KiCadAI version is
the reproducibility boundary.

## Failure Behavior

- Provider configuration, authentication, transport, refusal, malformed
  output, and strict-decode failures stop before project writes.
- Unsupported or ambiguous intent fails closed with structured diagnostics.
- Missing KiCad tooling cannot produce promotion `pass`.
- ERC, DRC, connectivity, routing, writer, or round-trip failures remain
  deterministic blockers; they are not suppressed because a model produced
  the initial intent.
- `candidate` means warning/skipped evidence still needs review. `ready` plus
  promotion `pass` means the requested validation gates produced real pass
  evidence.

## Current Limits

This milestone proves two production reference profiles, not arbitrary
electronics generation. The provider schemas are intentionally constrained to
the USB-C BMP280 and protected USB-C LED designs. Prompts matching neither
profile, or requesting both profiles as one composite design, fail before a
provider call or project write. New circuit categories need reviewed strict
schemas and promotion fixtures before they can make the same claim. A
KiCad-backed pass is design-validation evidence, not a claim that the board is
fabrication-ready without manufacturing review.
