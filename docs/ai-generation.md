# AI Generation

KiCadAI's provider-backed lane converts a natural-language prompt into strict
structured intent, then hands that validated intent to the deterministic
planner, schematic writer, placement/routing workflow, and KiCad checks. The
provider never emits KiCad S-expressions or route geometry directly.

## Supported Reference Lane

The demonstrated v1 reference is a protected USB-C-powered BMP280 I2C
breakout with an AP2112 3.3 V regulator, pull-ups, decoupling, and an external
I2C connector. Its checked-in provider inputs are under
`examples/ai/usb_c_bmp280_breakout/`.

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

Optional provider settings include `--model`, `--max-ai-attempts`, and
`--ai-background`. Correction attempts are bounded and restricted to explicit
schema/intent diagnostics. Authentication, refusal, unsupported topology, and
post-generation engineering failures are not sent back to the model for
unbounded retries.

The live provider smoke test is opt-in:

```sh
KICADAI_OPENAI_LIVE_TEST=1 go test ./internal/aiprovider -run TestOpenAILiveBMP280Intent -count=1 -v
```

## Reproducible Promotion Test

The default suite validates the recorded fixture and metadata without network
or KiCad. To run the real KiCad-backed promotion lane:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
  go test ./cmd/kicadai -run TestAIBMP280OptionalKiCadPromotion -count=1 -v
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

This milestone proves one production reference lane, not arbitrary electronics
generation. The provider capability/schema is intentionally constrained to the
USB-C BMP280 design and existing verified blocks. New circuit categories need
verified component/block semantics and promotion fixtures before they can make
the same claim. A KiCad-backed pass is design-validation evidence, not a claim
that the board is fabrication-ready without manufacturing review.
