# AI Generation

KiCadAI's provider-backed lane converts a natural-language prompt into strict
structured intent, then hands that validated intent to the deterministic
planner, schematic writer, placement/routing workflow, and KiCad checks. The
provider never emits KiCad S-expressions or route geometry directly.

## Provider Modes

KiCadAI exposes two provider contracts:

1. Bounded automatic profiles selected from prompt semantics.
2. Explicit `generic-circuit-v1`, where the provider emits a strict circuit
   graph and KiCadAI resolves every component, function, pin, pad, symbol, and
   footprint against the trusted catalog.

The two bounded production references remain:

- a protected USB-C-powered BMP280 I2C breakout with an AP2112 3.3 V
  regulator, pull-ups, decoupling, and external I2C connector;
- a protected USB-C-powered active-high LED indicator with fuse, TVS, bulk
  capacitance, and a 5 mA indicator path.

Their checked-in provider inputs are under
`examples/ai/usb_c_bmp280_breakout/` and
`examples/ai/usb_c_led_indicator_protected/`.

The promoted generic references are:

- `examples/ai/generic_rc_filter/`;
- `examples/ai/generic_usb_c_led_indicator_protected/`;
- `examples/ai/generic_usb_c_bmp280_breakout/`.

They prove passive, multi-branch protected-power, and regulated I2C sensor
topologies through the common graph schema instead of topology-specific
provider schemas.

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

Recorded generic RC run:

```sh
kicadai --prompt-file examples/ai/generic_rc_filter/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_rc_filter/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_rc --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

Recorded generic USB-C BMP280 run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_usb_c_bmp280_breakout/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_usb_c_bmp280_breakout/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_usb_c_bmp280 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

Recorded generic protected USB-C LED run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_usb_c_led_indicator_protected/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_usb_c_led_indicator_protected/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_usb_c_led --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
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

For a live generic run, use its recorded command and replace the two
recorded-provider options with `--provider openai`. For example, the generic
BMP280 command becomes:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_usb_c_bmp280_breakout/prompt.txt \
  --provider openai \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/live_generic_usb_c_bmp280 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
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
  go test ./internal/aiprovider -run '^TestOpenAILive(BMP280Intent|ProtectedLEDIntent|GenericRCGraph|GenericUSBCBMP280Graph)$' -count=1 -v
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
- `intent-plan.json` and `generated-request.json`: bounded-profile planning
  and lowered request evidence;
- `circuit-graph.json`, `circuit-resolution.json`, and `design-request.json`:
  generic graph, trusted resolution, and lowered request evidence;
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

For the generic BMP280 commands above, the corresponding stable evidence paths
are `./out/ai_generic_usb_c_bmp280/.kicadai/` or
`./out/live_generic_usb_c_bmp280/.kicadai/`. Start with
`design-promotion.json`, `workflow-result.json`, `validation-summary.json`,
and `circuit-resolution.json`. The workflow report embeds writer and round-trip
summaries and records the exact temporary KiCad ERC/DRC report paths used by
that run.

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

The generic graph removes the architectural requirement for one provider
schema per topology; it does not make every circuit routable or electrically
proven. V1 is limited to catalog-resolvable parts and functions, flat graph
topology, bounded relative layout intent, deterministic placement, and the
current explicit-circuit router. The promoted generic fixtures prove three
specific topology classes, not arbitrary electronics. Dense boards, arbitrary
hierarchy, analog performance, thermal/safe-operating-area analysis, safety
isolation, and fabrication release still require additional evidence.

Automatic bounded dispatch remains available for the two promoted references.
Explicit generic mode must be selected with `--ai-profile generic-circuit-v1`.
Unknown parts, ambiguous catalog matches, invented pins, unsupported geometry,
and incomplete required routing fail closed. A KiCad-backed pass is
design-validation evidence, not a fabrication-ready claim.
