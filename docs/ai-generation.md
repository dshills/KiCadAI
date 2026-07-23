# AI Generation

KiCadAI's provider-backed lane converts a natural-language prompt into strict
structured intent, then hands that validated intent to the deterministic
planner, schematic writer, placement/routing workflow, and KiCad checks. The
provider never emits KiCad S-expressions or route geometry directly.

## Provider Modes

KiCadAI exposes three fail-closed provider paths:

1. `behavioral-intent-v1` through `intent compile`, where the provider proposes
   a strict behavior contract and KiCadAI owns source coverage, uncertainty,
   capability validation, architecture search, and closed-loop qualification.
2. Bounded automatic `design create` profiles selected from prompt semantics.
3. `generic-circuit-v1`, where the provider emits either a strict explicit
   circuit graph or strict function-level intent. In the function form the
   provider supplies functions, named interfaces, operating domains, semantic
   connectivity, and bounded constraints—not pins, support parts, coordinates,
   layers, or routes. KiCadAI deterministically synthesizes the explicit graph
   and resolves every component, function, pin, pad, symbol, and footprint
   against trusted catalog and library evidence.

The two bounded production references remain:

- a protected USB-C-powered BMP280 I2C breakout with an AP2112 3.3 V
  regulator, pull-ups, decoupling, and external I2C connector;
- a protected USB-C-powered active-high LED indicator with fuse, TVS, bulk
  capacitance, and a 5 mA indicator path.

Their checked-in provider inputs are under
`examples/ai/usb_c_bmp280_breakout/` and
`examples/ai/usb_c_led_indicator_protected/`.

### Generation Capability Matrix

KiCadAI publishes its current provider/planner contract in
`internal/generationcapability`. Query the installed binary rather than relying
on repository source or documentation that may be older than the binary:

```sh
kicadai capability generation --json
```

The command returns the direct `design create` profile matrix, required
evidence, limitations, and the catalog-derived generic component/function
vocabulary. The generic provider
prompt uses this same serialized document, so the AI-facing contract and CLI
output cannot drift independently. It distinguishes the strict generic graph
path from bounded natural-language reference profiles rather than treating all
successful fixtures as arbitrary-circuit support.

`behavioral-intent-v1` uses a separate compiler-owned installed semantic
capability snapshot. `intent compile` validates and persists that snapshot
before provider execution, so it is not advertised as a direct graph-generation
profile by `capability generation`.

| Path | Input contract | Current boundary |
| --- | --- | --- |
| `behavioral-intent-v1` | `behavioralintent.Proposal` v1 compiled to `kicadai.open-set-requirement.v3` | Behavior/interfaces/conditions/tolerances/safety only. Minimum clarification or stable capability gaps replace guessed requirements. Executable output requires selected architecture and hash-bound closed-loop evidence. |
| `generic-circuit-v1` | `kicadai.circuit-graph.v1` explicit graph or function intent | Catalog-resolved explicit topology, or deterministic lowering from functions/interfaces/domains/constraints. Ambiguous resolution, missing support or unused-pin policy, unsupported functions, and incomplete intent fail closed. |
| `usb_c_bmp280` | `kicadai_bmp280_intent_v1` | Bounded USB-C BMP280 reference composition. |
| `usb_c_led_protected` | `kicadai_usb_c_led_intent_v1` | Bounded protected USB-C LED reference composition. |

All paths require the evidence appropriate to their requested acceptance level:
strict graph or request validation, trusted component resolution, schematic
electrical/readability checks, required-net routing, writer correctness,
round-trip preservation, and KiCad ERC/DRC where requested. This matrix is not
a claim that arbitrary dense, high-speed, RF, thermal, or analog-performance
requirements can be generated without engineering review.

### Behavioral Intent Compiler

Use the behavioral compiler when the user knows what the circuit must do but
should not be asked to choose its implementation:

```sh
kicadai \
  --file ./behavioral-request.txt \
  --provider openai \
  --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-request \
  intent compile
```

Every source statement must be accounted for as compiled behavior,
clarification, capability gap, or non-material context. Every generated
objective, operating case, and behavioral requirement must point back to source
evidence. The compiler exposes `ready`, `needs_clarification`, `unsupported`,
and `invalid`; only `ready` retains an executable requirement and writes
`.kicadai/behavioral-design-request.json`.

Clarification answers use the generated
`.kicadai/behavioral-follow-up-template.json` and are hash-bound to the exact
source, installed capabilities, prior proposal, and prior compilation. See
[Intent Planning](intent-planning.md#behavioral-intent-compilation) and the
[compiler audit](../specs/uncertainty-aware-behavioral-intent-compilation/AUDIT.md).

The offline generic-composition acceptance corpus exercises the checked-in RC
filter and transistor-switch graphs through strict decode, catalog resolution,
schematic/electrical checks, deterministic placement, routing, project writing,
and writer correctness. An unknown catalog component is rejected before a
project is written. KiCad ERC/DRC promotion remains separately optional and
environment-gated.

A second frozen, held-out corpus exercises function-level synthesis without
provider-supplied pins, support components, coordinates, layers, or routes. Its
eight circuits cover analog front ends and repeated active filters, a protected
USB-C/AP2112 supply, a transistor load driver, SHT31 and BME280 breakouts, an
ATmega328P ISP controller, and an ESP32/SHT31 controller. Every circuit has a
clean optional KiCad-backed promotion through strict ERC/DRC, complete routing
and connectivity, writer correctness, zero schematic/PCB round-trip
differences, and byte-identical replay. The hashes, selected components,
unused-pin decisions, derived constraints, and gate results are recorded in the
[function-level capability report](../specs/function-level-circuit-synthesis/CAPABILITY_REPORT.json).
The public operation registry and runnable, provider-free example are described
in [Function-Level Circuit Workflow](function-level-circuits.md). Query
`kicadai capability generation --json` and use
`data.function_level_contract`; do not derive the `usage` vocabulary from
internal corpus files.

Generic circuit graphs express schematic lanes explicitly. `power`, `signals`,
and `ground` are fixed to `top`, `middle`, and `bottom`. Provider output must set
`power_negative` to `lower` when any net has role `power_neg`, and to `null`
otherwise. Live provider output fails closed when a negative rail omits this
lane. Recorded legacy responses may infer `lower` only when exactly one distinct
negative rail exists; the run includes a compatibility warning, and multiple
implicit negative rails remain invalid.

The promoted generic references are:

- `examples/ai/generic_rc_filter/`;
- `examples/ai/generic_usb_c_led_indicator_protected/`;
- `examples/ai/generic_usb_c_bmp280_breakout/`;
- `examples/ai/generic_lmv321_ac_gain_stage/`;
- `examples/ai/generic_dual_lmv321_signal_conditioner/`.

They prove passive, multi-branch protected-power, and regulated I2C sensor
topologies plus single-stage and composed two-stage biased analog feedback
circuits through the common graph schema instead of topology-specific provider
schemas.

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

Recorded generic LMV321 gain-stage run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_lmv321_ac_gain_stage/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_lmv321_ac_gain_stage/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_lmv321 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

Recorded generic dual-LMV321 signal-conditioner run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_dual_lmv321_signal_conditioner/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_dual_lmv321_signal_conditioner/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_dual_lmv321 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

Recorded generic LM358 multi-unit signal-conditioner run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_lm358_buffered_signal_conditioner/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_lm358_buffered_signal_conditioner/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_lm358 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

### Multi-Unit Components

`generic-circuit-v1` models a multi-unit part as one physical component with a
single reference, catalog identity, footprint, and BOM identity. Its `units`
array declares the logical schematic units. Net endpoints and schematic
placements select those units explicitly. For the verified LM358 record, `A`
and `B` are amplifier units and required unit `P` owns the shared supply pins.

Unit qualifiers are absent for ordinary single-unit components. Relative
layout qualifiers such as `near_unit` are used only when their corresponding
relationship targets a declared unit on a multi-unit component. KiCadAI fails
closed on nonexistent or conflicting units, inconsistent shared-pin nets,
missing required units, duplicate physical footprints, and ambiguous
unit-to-pad mappings.

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

Live generic LMV321 gain-stage run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_lmv321_ac_gain_stage/prompt.txt \
  --provider openai \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/live_generic_lmv321 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

Live generic dual-LMV321 signal-conditioner run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_dual_lmv321_signal_conditioner/prompt.txt \
  --provider openai --ai-background \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/live_generic_dual_lmv321 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted --max-ai-attempts 2 \
  design create
```

The background option avoids the foreground response-body timeout on provider
requests that need more than two minutes. It does not weaken schema validation,
retry bounds, or any KiCad gate.

Live generic LM358 multi-unit run:

```sh
mkdir -p ./out
kicadai --prompt-file examples/ai/generic_lm358_buffered_signal_conditioner/prompt.txt \
  --provider openai --ai-background \
  --ai-profile generic-circuit-v1 --max-ai-attempts 2 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/live_generic_lm358 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

The live and recorded semantic projection test can verify the saved live graph
without another API call:

```sh
KICADAI_OPENAI_LIVE_TEST=1 \
KICADAI_OPENAI_LIVE_GRAPH=./out/live_generic_lmv321/.kicadai/circuit-graph.json \
  go test ./internal/aiprovider -run '^TestOpenAILiveGenericLMV321Graph$' -count=1 -v
```

For the two-stage fixture, compare its saved live graph with the checked-in
recorded critical projection without making another provider request:

```sh
KICADAI_OPENAI_LIVE_TEST=1 \
KICADAI_OPENAI_DUAL_LMV321_LIVE_GRAPH=./out/live_generic_dual_lmv321/.kicadai/circuit-graph.json \
  go test ./internal/aiprovider -run '^TestOpenAILiveGenericDualLMV321Graph$' -count=1 -v
```

For the LM358 fixture, verify one-package multi-unit identity and compare its
saved live graph with the recorded critical projection:

```sh
KICADAI_OPENAI_LIVE_TEST=1 \
KICADAI_OPENAI_LM358_LIVE_GRAPH=./out/live_generic_lm358/.kicadai/circuit-graph.json \
  go test ./internal/aiprovider -run '^TestOpenAILiveGenericLM358Graph$' -count=1 -v
```

Each live semantic-projection test uses a fixture-specific graph-path variable
so multiple saved live graphs can be tested in the same process. The single-stage
fixture retains the older generic `KICADAI_OPENAI_LIVE_GRAPH` name; the
two-stage fixture uses `KICADAI_OPENAI_DUAL_LMV321_LIVE_GRAPH`.
The LM358 fixture uses `KICADAI_OPENAI_LM358_LIVE_GRAPH`.

The LM358 reference has both recorded and live-provider evidence. Its saved
live graph is semantically equivalent to the checked-in recording and reaches
AI status `ready` plus KiCad-backed promotion `pass` through clean ERC, strict
DRC, required-net routing, writer correctness, and normalized round-trip
checks. Provider timeouts and transport failures occur before graph generation
and do not alter this deterministic evidence; rerun the optional live command
or replay the saved graph through the recorded provider.

Optional provider settings include `--model`, `--max-ai-attempts`,
`--ai-max-output-tokens`, and `--ai-background`. Bounded reference profiles
default to 8,192 output tokens; `generic-circuit-v1` defaults to 32,768 because
its strict graph envelopes are larger. The explicit token limit must be from
1,024 through 65,536. `--ai-max-output-tokens` overrides
`KICADAI_AI_MAX_OUTPUT_TOKENS`, which overrides the profile default. Increasing
the limit can increase provider cost and latency.

When OpenAI returns `incomplete_details.reason=max_output_tokens`, the
structured issue path is `provider.max_output_tokens`. Its message includes the
selected limit and reported output/total usage, and its suggestion provides an
explicit bounded retry flag. KiCadAI does not automatically retry token
exhaustion. Correction attempts remain bounded and restricted to explicit
schema/intent diagnostics. Authentication, refusal, unsupported topology, and
post-generation engineering failures are not sent back to the model for
unbounded retries.

### Capture Once, Replay Offline

Every successfully decoded provider envelope is immediately captured at
`.kicadai/ai-provider-replay.json`, before graph or intent preflight. The
versioned `kicadai.ai.replay.v1` artifact contains the selected profile,
sanitized provider metadata and usage, a canonical intent envelope, and an
integrity hash. It excludes raw user and correction prompts, credentials,
headers, and provider instructions. Invalid downstream graphs retain this
artifact so the same failure can be debugged without another provider call.

CLI JSON includes `data.provider.replay_artifact` and an exact
`data.provider.replay_command`. Integrations should execute
`data.provider.replay_argv` directly without a shell; the command string is
POSIX-shell escaped for interactive use. The replay uses the recorded provider,
requires no prompt, and performs no network request. A direct form is:

```sh
kicadai \
  --provider recorded \
  --provider-record ./out/live/.kicadai/ai-provider-replay.json \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/replay --overwrite \
  design create
```

The replay artifact supplies the original schema profile. Plain checked-in
`recorded-response.json` envelopes remain supported and still require an
explicit prompt/profile as shown in the fixture commands above.

## Catalog-Backed Trusted Simulation

`generic-circuit-v1` simulation requests select a KiCadAI-owned trusted model
ID. Legacy analytic models bind named roles and bounded scalar inputs. The
graph-derived linear MNA model accepts bounded DC/AC analyses. A distinct
nonlinear workflow accepts bounded DC operating points for reviewed Shockley
diode and NPN/PNP Ebers-Moll primitives. A separate transient workflow accepts
one exact bounded observation grid, constant or bounded pulse sources, and
voltage or trusted 10%-90% edge assertions for reviewed capacitors, diodes, and
BJTs. These workflows accept independent source
conditions and structured node assertions; topology is compiled only from
resolved circuit connectivity and catalog evidence. The provider cannot supply
topology classifications, device parameters, equations, matrices, stamps,
integration methods, initial states, solver settings, executable code, model
files, include paths, commands, or expressions. Strict decoding and
registry validation reject unknown fields and unsupported or incompatible
requests.

The registry supports catalog-parameterized ideal fixed linear regulators,
unloaded resistor-divider DC behavior, ideal first-order RC low-pass AC
magnitude, graph-derived linear MNA, bounded nonlinear DC, and fixed-step
transient analysis. Trusted MNA primitives cover resistors, capacitors,
independent voltage/current sources, a
finite-gain single-pole op-amp with catalog supply/output limits, reviewed
signal diodes, and reviewed NPN/PNP small-signal BJTs. Nonlinear analysis uses a
fixed source/gmin continuation schedule, bounded Newton iterations and voltage
updates, bounded exponential evaluation, deterministic convergence evidence,
and catalog-backed current/voltage operating limits. The deterministic
transient path uses backward Euler, a bounded nonlinear DC initial condition,
the previous accepted state as each Newton seed, a catalog-rated capacitor
companion model, and per-point capacitor/diode/BJT operating-limit checks.
`.kicadai/simulation.json` report records registry/catalog provenance, canonical
topology hash and devices, every analysis point and solved node, assertions,
and status. Singular, unstable, unsupported, nonconvergent, operating-limit,
incompatible, and numerically unbounded systems fail closed. This remains
bounded functional evidence, not arbitrary SPICE compatibility, parasitic,
thermal, tolerance, SOA, or fabrication signoff.

## Generic Placement And Routing Correction

After a `generic-circuit-v1` graph strict-decodes, resolves through the trusted
catalog, and lowers to a design request, KiCadAI automatically enters a separate
deterministic engineering correction loop when the initial route fails with
supported diagnostics. This is not a provider retry: board diagnostics are
never returned to the model. It is also not the persisted `repair apply`
workflow used after project generation.

The loop allows three total placement/routing attempts: the initial attempt and
at most two corrections. It normalizes placement, routing, route-tree, and
contact-graph diagnostics; derives stable retry keys; plans without mutation;
applies only authorized adjustments; reruns placement and the real router; and
retains the best electrically ranked attempt. It stops on success, exhausted
budget, repeated retry key or placement state, non-improvement, cancellation,
ambiguous/unsupported evidence, fixed-constraint conflict, or protected
invariant change.

V1 can apply validated relative-spacing, declared-region, endpoint-fanout, and
endpoint-distance adjustments before rebuilding affected routes. Route-tree
branch reordering and layer-transition insertion are represented in the
diagnostic taxonomy but remain unsupported automatic actions and fail closed.
The loop cannot change components, values, pin mappings, footprints, nets,
board outline, layer count, net classes, width/clearance rules, fixed placement,
or catalog resolution.

Here, fail closed means correction stops and retains the best ranked attempt;
if that attempt is still incomplete, connectivity, routing, and promotion gates
remain blocked. It does not silently treat an unsupported action as success.

Run any checked-in generic fixture through the compiled CLI. Set the two
library variables to your local KiCad library checkouts first:

```sh
mkdir -p ./out
# Replace both values with absolute paths to your local KiCad library checkouts.
KICAD_SYMBOLS_ROOT="/absolute/path/to/kicad-symbols"
KICAD_FOOTPRINTS_ROOT="/absolute/path/to/kicad-footprints"
kicadai --prompt-file examples/ai/generic_rc_filter/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_rc_filter/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root "$KICAD_SYMBOLS_ROOT" \
  --footprints-root "$KICAD_FOOTPRINTS_ROOT" \
  --output ./out/generic_rc_filter --overwrite \
  design create
jq . ./out/generic_rc_filter/.kicadai/autonomous-correction.json
```

The report uses schema `kicadai.autonomous-correction.v1`. `attempts` counts
routing executions, including the initial attempt. `plan_evaluations` counts
correction plans, including plans that fail closed without another routing
execution. Inspect `stop_reason`, `selected_attempt`, invariant fingerprints,
`selected_reason`, `all_attempt_invariants_preserved`, applied retry keys, and
`attempt_history`; do not infer success from `applied` alone. Final authority
remains route completion, connectivity, writer, round-trip, ERC/DRC, and
promotion evidence.

The live provider smoke test is opt-in:

```sh
KICADAI_OPENAI_LIVE_TEST=1 \
  go test ./internal/aiprovider -run '^TestOpenAILive(BMP280Intent|ProtectedLEDIntent|GenericRCGraph|GenericUSBCBMP280Graph|GenericLMV321Graph|GenericDualLMV321Graph|GenericLM358Graph)$' -count=1 -v
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
- `autonomous-correction.json`: generic-only bounded placement/routing attempt,
  retry-key, action, invariant, selection, and stop evidence;
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

These paths now follow the lane-neutral [creation evidence
contract](creation-evidence.md). Provider, intent, and circuit entry points
write the same six versioned core artifacts; provider responses, circuit
resolution, rationale, and retry records remain additive.

For the LMV321 commands, inspect
`./out/ai_generic_lmv321/.kicadai/` or
`./out/live_generic_lmv321/.kicadai/`. The recorded and live lanes preserve
the verified LMV321 symbol and SOT-23-5 footprint, required-net topology, and
explicit markers with value `review_required` for stability, gain-bandwidth,
output drive, noise, distortion, and load compatibility. Clean ERC/DRC and
promotion `pass` do not prove those analog performance properties.

For the two-stage commands, use
`./out/ai_generic_dual_lmv321/.kicadai/` and
`./out/live_generic_dual_lmv321/.kicadai/`. Both LMV321 instances carry
separate stage identity plus explicit `review_required` markers for stability,
gain-bandwidth, output drive, noise, distortion, and load compatibility. The
current reference also uses a high-impedance 100 kOhm VREF divider and routed
ground traces without a plane. Those choices are accepted ERC/DRC fixture
inputs, not proof of low-noise or low-distortion analog performance; revise and
simulate them before using the design as an engineering reference.

For the LM358 commands, use `./out/ai_generic_lm358/.kicadai/` and
`./out/live_generic_lm358/.kicadai/`. In `circuit-graph.json`, U1 has logical
units `A`, `B`, and `P`; in the lowered request and PCB artifacts, U1 remains
one physical SOIC-8 component with one footprint and one BOM identity. Inspect
`circuit-resolution.json`, `design-request.json`, `workflow-result.json`, and
`design-promotion.json` together when auditing shared supply pins and pad nets.
The fixture marks stability, gain-bandwidth, output swing, input common-mode
range, output drive, noise, distortion, and load compatibility as
`review_required`. Clean ERC, DRC, routing, writer, and round-trip evidence does
not establish those analog properties.

Repeated recorded runs are deterministic at the circuit graph, catalog
resolution, lowered design request, and transaction layers. Manifests contain
workspace-specific paths and are not expected to be byte-identical across
different output directories.

Plaintext prompts, API keys, authorization headers, and hidden provider
reasoning are not persisted. The normalized intent plus the KiCadAI version is
the reproducibility boundary.

## Failure Behavior

- Provider configuration, authentication, transport, refusal, malformed
  output, and strict-decode failures stop before project writes.
- Behavioral compilation with unresolved uncertainty writes a minimal
  clarification template, while unavailable semantics write stable capability
  gaps. Neither status releases an executable design request.
- Unsupported or ambiguous intent fails closed with structured diagnostics.
- Missing KiCad tooling cannot produce promotion `pass`.
- ERC, DRC, connectivity, routing, writer, or round-trip failures remain
  deterministic blockers; they are not suppressed because a model produced
  the initial intent.
- `candidate` means warning/skipped evidence still needs review. `ready` plus
  promotion `pass` means the requested validation gates produced real pass
  evidence.

## Current Limits

The behavioral compiler removes the need for users to prescribe implementation
details inside the supported semantic envelope. Its 24-prompt held-out corpus
contains 12 paraphrase groups: 12 ready prompts representing six unique
supported contracts, four clarification prompts, and eight unsupported
prompts. All six supported contracts pass deterministic replay and installed-
KiCad promotion.

For programmable-controller objectives, the catalog provider can now choose a
verified ATmega328P-A, ESP32-WROOM-32E, or STM32G031K8T6 from capability and
electrical requirements. It assigns compatible GPIO/UART/I2C/SPI/PWM/ADC/
interrupt bundles to physical functions deterministically and expands the
selected record's power, reset, boot, programming, and clock companions. A
three-request neutral corpus validates this lane without target names.

For power and mixed-signal interfaces, the architecture provider can now build
a deterministic rail graph, aggregate bounded loads and quiescent demand,
apply regulator dropout/efficiency/startup/thermal evidence, derive stability
and transient capacitance, and solve supported pull-up, translation,
termination, clock, and ADC-drive obligations. A neutral four-design corpus
covering MCU/sensor, power-MOSFET, buffered ADC, and Class-AB cases passes the
complete installed-KiCad promotion lane. Missing safety-relevant evidence and
incompatible constraints produce stable capability gaps instead of guessed
values or topology.

The generic contract removes the architectural requirement for one provider
schema per topology and its function form removes the requirement for the AI to
enumerate pins, support components, or physical implementation details. It does
not make every circuit routable or electrically proven. V1 is limited to
catalog-resolvable parts, functions, and reviewed companion/unused-pin policy;
flat synthesized topology; bounded physical constraints; deterministic
placement; and the current explicit-circuit router. Six promoted explicit
topology classes and eight held-out function-level circuits now pass. This is
meaningful breadth evidence, not proof of arbitrary electronics. Dense boards,
arbitrary hierarchy, high-speed and RF constraints, comprehensive analog
performance, thermal/safe-operating-area analysis, safety isolation, and
fabrication release still require additional evidence.

Automatic bounded dispatch remains available for the two promoted references.
Explicit generic mode must be selected with `--ai-profile generic-circuit-v1`.
Behavioral compilation must be selected with
`--ai-profile behavioral-intent-v1` and the `intent compile` command; it does
not silently replace `design create` dispatch.
Unknown parts, ambiguous catalog matches, invented pins, unsupported geometry,
and incomplete required routing fail closed. A KiCad-backed pass is
design-validation evidence, not a fabrication-ready claim.

To reproduce the complete supported installed-KiCad release corpus from an
unmodified checkout, use:

```sh
make promotion-bundle
```

This path discovers or bootstraps the locked KiCad toolchain and stock
libraries, runs each matrix scenario twice, and emits a verified
content-addressed bundle. It does not need hand-set library paths. Individual
provider or corpus test commands remain useful for focused diagnosis, but they
are not substitutes for the release bundle.
