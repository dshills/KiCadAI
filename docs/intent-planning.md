# Intent Planning And AI Workflow

Structured-intent generation, rationale reports, and semantic synthesis behavior.

Natural-language generation also has an explicit generic circuit-graph path.
With `--ai-profile generic-circuit-v1`, provider output is strict-decoded and
catalog-resolved before it is lowered into this design workflow. See
[AI Generation](ai-generation.md); the generic path does not bypass intent,
electrical, placement, routing, or KiCad validation gates.

### Behavioral Intent Compilation

`intent compile` is the fail-closed natural-language entry point for strict
open-set behavioral requirements. The prompt describes external interfaces,
observable behavior, operating ranges and corners, tolerances, safety limits,
and manufacturing-neutral board bounds. It must not choose topology, parts,
nets, coordinates, layers, or routes.

```sh
kicadai \
  --text "Filter a ground-referenced input at 18 kHz to 22 kHz with gain from 9.5 to 10.5 across a 10.8 V to 13.2 V supply." \
  --provider openai \
  --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-filter \
  intent compile
```

The command derives and hashes the installed semantic capabilities before the
provider call, strict-decodes provider output, verifies source and reverse
coverage, qualifies a ready proposal with deterministic architecture search,
and requires hash-bound trusted closed-loop simulation over every declared
corner. Its terminal statuses are `ready`, `needs_clarification`,
`unsupported`, and `invalid`. Only `ready` retains an executable strict v3
requirement and writes `behavioral-design-request.json`; unsupported behavior
produces stable semantic capability-gap records instead of a guessed design.

After a `ready` compilation, pass the persisted design request through the
normal project workflow and request the required external gates explicitly:

```sh
kicadai \
  --request ./out/behavioral-filter/.kicadai/behavioral-design-request.json \
  --output ./out/behavioral-filter-project --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

When clarification is required, the output directory contains
`.kicadai/behavioral-follow-up-template.json`. Fill only each `answer` field,
then rerun against the complete original text and the same output directory:

```sh
kicadai \
  --file ./original-request.txt \
  --provider openai \
  --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-filter \
  --follow-up ./out/behavioral-filter/.kicadai/behavioral-follow-up-template.json \
  --overwrite \
  intent compile
```

The follow-up hashes bind answers to the exact original source, capability
snapshot, proposal, and compilation. Answers may resolve only the named
clarification and uncertainty identities. Full proposal, compilation,
architecture-search, closed-loop, provider-attempt, and replay evidence is
persisted under `.kicadai/`; large simulation evidence is not emitted inline.

### AI Design Workflow

`design create` is the first deterministic AI-facing workflow. It accepts an
explicit request JSON, selects built-in circuit blocks by ID, generates a
schematic transaction, realizes PCB fragments, places footprints, optionally
routes, writes the KiCad project, runs structural/connectivity validation, and
returns stage-by-stage feedback.

```sh
kicadai \
  --request ./examples/design/led_indicator.json \
  --output ./out/led_indicator \
  --overwrite \
  design create
```

Useful flags:

- `--output`: project output directory for `design create` and generation
  commands; for `repair export-bundle`, an optional bundle output path inside
  the target root.
- `--target`: existing generated project directory or file for
  target-oriented repair commands.
- `--execute`: perform filesystem writes for repair export/apply commands that
  otherwise default to dry-run behavior.
- `--overwrite`: allow replacing generated output directories or existing
  repair bundle files.
- `--skip-routing`: skip board routing while still writing realized local PCB
  fragments.
- `--route-mode`: routing mode, one of `single_layer`, `two_layer`, or
  `validate_only`; `--mode` remains as a shorter alias.
- `--grid`, `--trace-width`, and `--clearance`: route planner controls in
  millimeters.
- `--allow-partial`: allow partial route results.
- `--placement-estimated-width`, `--placement-estimated-height`,
  `--placement-board-margin`: fallback placement geometry controls.
- `--strict-unrouted`, `--strict-zones`: make validation stricter.
- `--require-drc`, `--kicad-cli`, `--keep-artifacts`, `--artifact-dir`: require
  KiCad-backed DRC evidence.
- `--allow-missing-drc`: request optional DRC evidence without blocking when
  `kicad-cli` is unavailable.
- `--repair`: include the plan-only `validation_repair` stage in
  `design create`; it does not persist the replay bundle.
- `--repair-apply`: implies `--repair`, applies generated-project validation
  repair during `design create`, and writes `.kicadai/repair-bundle.json`.
- `--max-repair-attempts`: bound validation repair attempts when repair is
  enabled by the workflow or a repair command.

Examples live in `examples/design/`:

- `led_indicator.json`: structural acceptance, routing skipped.
- `active_low_led.json`: structural acceptance, routing skipped, active-low
  LED parameter path.

Optional KiCad-backed design examples live in
`examples/design/kicad-backed/`. They are run only when `KICADAI_KICAD_CLI` is
configured. Current pass fixtures include concrete BMP280, USB-C LED,
protected USB-C LED, protected USB-C/3.3 V/I2C, generic I2C breakout, exact
ESP32-WROOM-32E, Class-A, protected Class-AB headphone, and protected 10 W
Class-AB speaker designs. Simple LED and connector/LED smoke cases remain
candidate-level; the unprotected Class-AB skeleton and older op-amp headphone
buffer remain explicit expected failures. The authoritative list and manual
commands are in the
[KiCad-backed example README](../examples/design/kicad-backed/README.md).
Do not report a fixture as passing unless the optional test or manual command
produced successful KiCad report artifacts and its declared readiness is
`pass`.

The multi-block I2C sensor breakout is covered by block and intent fixtures
plus the optional KiCad-backed `i2c_sensor_breakout_candidate` design example.
It is now a reproducible KiCad-backed pass fixture when run with configured
required ERC/DRC evidence.

Concrete structured-intent examples are checked in at:

- `examples/intent/sensor_bmp280_breakout.json`;
- `examples/intent/sensor_sht31_breakout.json`.

Each example carries `sensor_component_id` through `intent plan` into the
generated `i2c_sensor` request. Component selection then writes the selected
manufacturer/MPN, component ID, symbol, footprint, confidence, and pinmap
evidence into generated operations. Use `intent plan` to inspect this handoff:

```sh
kicadai --request ./examples/intent/sensor_bmp280_breakout.json intent plan
kicadai --request ./examples/intent/sensor_sht31_breakout.json intent plan
```

Omitting `sensor_component_id` deliberately selects the backward-compatible
generic topology. Providing an unknown ID does not fall back; it blocks until
a verified concrete profile exists.

The BMP280 intent is also promoted through the optional KiCad-backed fixture
lane as `examples/design/kicad-backed/sensor_bmp280_breakout.json`. Its
metadata declares `pass` and requires real KiCad ERC and DRC evidence. See
`examples/design/kicad-backed/README.md` for the exact generation command and
artifact paths.

The BMP280 input also demonstrates automatic schematic-layout synthesis. It
contains no hand-authored `schematic_layout`. The input opts the generated
workflow into `auto_schematic_layout`; after block instantiation,
KiCadAI derives groups, lanes, rules, and `near`/`above`/`right_of` placements
from active component roles and non-ground net topology. Derived targets use
stable `instance__role` IDs such as `sensor__decoupling_capacitor`; generated
KiCad references and project names are deliberately not part of the contract.
KiCadAI validates the derived IR, runs the shared schematic layout engine,
centers the result on the selected page, and writes explicit symbol and
reference/value positions into the transaction. Ambiguous or disconnected
stage topologies fail closed with `schematic_layout.inference` diagnostics and
can be resolved by supplying explicit layout intent.

The top-level JSON result uses requested acceptance as the success contract. A
request for `structural` acceptance can return `ok: true` while still including
connectivity or KiCad-check issues for higher levels. Those issues are retained
in `issues[]` and grouped under `data.feedback.repairs[]` so an agent can decide
whether to revise the request, adjust placement/routing, or ask for external
tooling.

`design create` writes `<output>/.kicadai/design-promotion.json` when project
metadata can be written and includes a compact `data.promotion` summary when
that report artifact is written. Promotion is stricter than requested
acceptance: it evaluates fixture metadata,
workflow stages, writer correctness, connectivity, optional KiCad ERC/DRC
evidence, route completion, physical-rule evidence, and expected artifacts.
Agents should use the promotion report when deciding whether a generated
project can be treated as a promoted KiCad-backed fixture. A skipped KiCad gate
means external evidence was not produced; it is not proof of ERC/DRC success.
For the simple LED prompt, skipped optional KiCad evidence leaves the promotion
report at `candidate`; required clean KiCad ERC/DRC evidence promotes it to
`pass`, and real KiCad findings block promotion with exact issue codes.


### Intent Planner

The `intent` command family sits above `design create`. It is intended for AI
callers that can produce structured intent but should not need to know the
exact block IDs, validation defaults, or generated workflow request shape.

```sh
kicadai \
  --request ./examples/intent/sensor_breakout.json \
  --output ./out/intent_plan \
  --overwrite \
  intent plan

kicadai \
  --request ./examples/intent/sensor_breakout.json \
  intent explain

kicadai \
  --request ./examples/intent/sensor_breakout.json \
  --output ./out/intent_sensor \
  --overwrite \
  intent create
```

Commands:

- `intent plan`: validates the intent request, derives requirements,
  assumptions, known gaps, selected blocks, component policy, validation
  policy, semantic synthesis trace, and a generated `design create` request.
- `intent explain`: returns a compact AI-facing summary of requirements,
  selected blocks, assumptions, gaps, and blocking issues to stdout; with
  `--text` or `--file` before `intent`, it drafts prose first.
- `intent rationale`: returns a consolidated `design-rationale.json`-shaped
  report with source evidence, interpreted intent, planner decisions,
  assumptions, clarifications, known limits, validation summary, artifact
  references, and next actions.
- `intent create`: plans intent, refuses ambiguous or blocking plans, runs
  `design create`, and writes planner artifacts under the generated project's
  `.kicadai/` directory.
- `intent draft`: converts natural-language text into the structured intent
  request schema with extraction evidence, confidence, and clarifications.

Natural-language intake is CLI-first and deterministic:

```sh
kicadai --text "make a 3.3V I2C temperature sensor breakout" intent draft
kicadai --text "make a simple LED indicator board" --output ./out/ai_led --overwrite intent create
kicadai --file ./examples/intent_text/i2c_temperature_sensor_breakout.txt intent explain
kicadai --file ./examples/intent_text/i2c_temperature_sensor_breakout.txt intent rationale
kicadai --text "battery powered sensor" --strict intent draft
```

Flags are global in the current CLI and must appear before `intent`.

Without `--output`, `intent draft` writes the draft report to stdout only.
`--output ./out/draft` writes `intent-source.txt`,
`intent-draft.json`, `intent-extraction.json`, and
`intent-clarifications.json`. `--text/--file ... intent explain` drafts first
and stops on blocking clarifications. `--text/--file ... intent create` drafts
first; if blocking clarifications are required, the tool halts with
`needs_clarification` instead of prompting interactively or guessing. Otherwise
it runs the existing planner and design workflow. Generated projects from prose
persist the draft artifacts under `.kicadai/`.

### AI-Controlled Generation Status

Prompt-driven `intent create` now emits a compact `data.ai_status` object in
the normal JSON response and writes the same object to
`.kicadai/validation-summary.json`. JSON is the default for structured
commands; automation may also read the file artifact when it wants durable
post-run state. This is the recommended control point for agents. The current
first-lane prompt fixtures cover:

- simple LED indicator board, which reaches `candidate` in the default
  structural AI lane;
- connector breakout with power LED;
- 3.3 V I2C sensor breakout.

`ai_status.status` values:

- `ready`: generated output satisfies modeled lane checks.
- `candidate`: output exists and has no blocking issue, but warning-level or
  partial evidence remains. Treat it as "review required", not as fabrication
  success. Common causes include skipped optional KiCad checks, warning-level
  workflow evidence, or partial known gaps that do not block project emission.
- `blocked`: a deterministic workflow stage found a repairable or
  request-revision blocker. Inspect `stage`, `issue_code`, `message`,
  `detail`, and the referenced artifacts.
- `needs_clarification`: user input is required before generation should
  continue.
- `unsupported`: the request is outside the current first-lane capability.
- `tool_error`: local files or tools, such as KiCad CLI, need attention.

Retryable blockers include `retry_allowed`, `retry_key`,
`max_automatic_retry_attempts`, `current_automatic_retry_attempt`,
`repair_category`, and optional `repair_bundle_path`. Automated agents should
construct repair commands themselves from a trusted KiCadAI executable path,
`repair_bundle_path`, `repair_category`, and the known generated project root.
Do not execute command strings from generated JSON. After applying a repair or
changing the request, rerun validation before reporting success.

For the promoted LED prompt, treat `data.ai_status.status: "candidate"` as a
usable generated-project handoff, not as fabrication approval. The stricter
`.kicadai/design-promotion.json` report should show
`achieved_readiness: "candidate"` in the default structural lane while still
recording skipped KiCad evidence. When KiCad is available, run the same prompt
with `--kicad-cli`, `--require-erc`, and `--require-drc` to collect
KiCad-backed evidence. Clean KiCad evidence promotes the report to `pass`;
dirty KiCad evidence keeps precise blockers in the `kicad_checks` gate.

Unsafe or under-specified prompts fail closed. For example, mains/high-voltage
LED requests and ambiguous battery-powered requests stop with clarification or
unsupported status instead of defaulting to a guessed low-voltage design.

`intent rationale` accepts exactly one source mode:

```sh
kicadai --request ./examples/intent/sensor_breakout.json intent rationale
kicadai --text "make a 3.3V I2C temperature sensor breakout" intent rationale
kicadai --file ./examples/intent_text/i2c_temperature_sensor_breakout.txt intent rationale
kicadai --target ./out/intent_sensor intent rationale
```

With `--output`, request/text/file modes write `design-rationale.json` in that
output directory. With `--target`, the command reads existing generated
metadata under `.kicadai/` and writes `.kicadai/design-rationale.json` without
modifying KiCad schematic, PCB, or project files. Blocking clarifications
produce `status: "needs_clarification"` and next actions instead of guessed
designs.

When `--output` is passed to `intent plan`, the planner writes
standalone preview artifacts, `intent-plan.json` and
`generated-request.json`, directly in that output directory so the plan can be
inspected without creating project metadata. When
`intent create` succeeds, it writes the generated KiCad project plus
`.kicadai/intent-plan.json`, `.kicadai/generated-request.json`,
`.kicadai/workflow-result.json`, and `.kicadai/design-rationale.json` inside
the project output directory. The `.kicadai/` paths are the persistent
project-relative metadata locations once a KiCad project exists.

`intent-plan.json` now includes `synthesis`, a deterministic trace with:

- `decisions`: topology, bus resolution, voltage-domain choices, validation
  policy, and known topology gates.
- `evidence`: source intent fields, block capability records, generated net
  evidence, and workflow policy evidence.
- `constraints`: component confidence, acceptance, package, voltage, current,
  and fabrication constraints.
- `calculations`: policy-level value evidence for LED resistors, I2C pull-ups,
  regulator headroom, crystal load capacitors, and op-amp gain where inputs are
  known. Each calculation records whether it was `applied`, `deferred`, or
  `blocked`, any block params that were written, and calculated requirements
  that inform component selection or rationale output.
- `gaps`: unsupported peripherals, voltage-domain problems, target ambiguity,
  and other fail-closed synthesis limits.

For power rails that synthesize a voltage regulator, include `current_ma` when
the load current is known. The planner maps that value into the generated
component policy as a regulator `output_current` requirement and records the
input/output capacitor voltage classes needed for the selected input and output
rails. For a 3.3 V rail supplied from an input at or below 6 V and a load at or
below 150 mA, the planner selects the AP2112K SOT-23-5 profile and writes
`regulator_symbol`, `regulator_footprint`, `input_voltage_min`,
`input_voltage_max`, and `enable_mode: tied_input` into the generated
`voltage_regulator` block params. Higher-current rails fall back to the default
fixed-linear-regulator path and must still satisfy catalog selection and
workflow validation. The AP2112K catalog record carries a higher electrical
current rating, but the planner uses the lower automatic threshold until
package thermal evidence is modeled.

Regulator synthesis calculations now record the selected variant, modeled
dropout margin, headroom, minimum capacitor voltage policy, and explicit
thermal/stability review requirements. Insufficient modeled headroom blocks the
plan. These overrides and calculations are persisted in
`.kicadai/generated-request.json` and `.kicadai/intent-plan.json`, and the
selected regulator/capacitor evidence is persisted in
`.kicadai/workflow-result.json`. This is the preferred audit path for agents
checking that a generated 3.3 V breakout selected a real regulator rather than
a placeholder.

`--output` is used for new generated content, including intent planning and
project creation. `--target` is reserved for commands that inspect or repair an
existing generated project; after `intent create` or `design create`, pass that
generated project directory as `--target` to later target-oriented repair
commands. Commands that write to an existing output directory require
`--overwrite`.

Structured JSON examples live in `examples/intent/` and cover sensor breakout,
MCU programmer, power module, amplifier module, fabrication-oriented sensor
requests, MCU plus I2C sensor, MCU ISP programming, external-clock limitation,
multi-MCU ambiguity, explicit voltage-domain supply examples, AP2112K
regulator evidence, high-current regulator fallback, and an intentionally
blocked insufficient-headroom regulator case. Natural language text examples
live in `examples/intent_text/`.
Semantic synthesis fixtures are prefixed with `synthesis_` and cover explicit
MCU/I2C supply domains, UART programming, blocked unknown supply aliases, and
blocked external-clock topology.

Structured intent supports semantic target, bus, and supply fields:

- `functions[].target` and `interfaces[].target`: constrain support blocks or
  interfaces to a target role or instance when inference would be ambiguous.
- `functions[].interface` and `functions[].bus`: describe functional bus
  intent such as I2C.
- `interfaces[].bus`: groups connectors and devices onto the same logical bus.
- `functions[].supply`, `power.rails[].alias`, and
  `power.rails[].supplied_targets`:
  bind blocks to named voltage domains and emit plan evidence for selected
  supply sources. Legacy `power.rails[].supplies` input is still accepted as an
  alias.

Implemented semantic mappings:

- MCU plus I2C sensor/connector plans now connect SDA/SCL through the supported
  ATmega328P-A seed MCU template when exactly one compatible MCU target exists.
- Reset/programming support can connect ISP or UART headers to a resolved MCU
  target using semantic port roles and target-scoped signal nets.
- Voltage-domain planning records selected source/net evidence on affected
  requirements and refuses to silently fall back when an explicit supply alias
  is unknown.
- Multiple compatible MCU targets require explicit target metadata rather than
  guessed support wiring.
- External MCU clock intent now reports a precise topology limitation: target
  clock ports are known, but the current generated MCU block still only emits
  internal-clock topology.

Current intent-planner gaps:

- natural-language intake covers only the supported seed phrases and remains a
  deterministic draft adapter, not a general LLM parser;
- MCU semantic support is limited to the verified seed template and does not
  yet derive alternate functions from arbitrary KiCad symbols;
- external MCU clock generation is still blocked until the MCU block can emit a
  safe non-internal clock topology;
- design rationale reports explain current decisions and blockers, but they do
  not create new schematic/PCB topology beyond the deterministic planner;
- synthesis calculations now apply supported values to LED, I2C pull-up, and
  crystal blocks, and now map regulator current and capacitor voltage
  requirements into generated component policy for the verified
  linear-regulator slice. Broader analog synthesis remains limited to explicit
  requirement evidence until blocks expose safe parameters and catalog ratings
  cover the target checks;
- amplifier requests may use the bounded low-voltage headphone slice or the
  protected dual-rail 10 W RMS/8 ohm speaker slice with its exact reviewed
  component, load, SOA, thermal, protection, layout, simulation, KiCad, and
  fabrication evidence. Bridge outputs, mains supplies, materially higher
  power, arbitrary output families, unreviewed substitutions, and loads or
  heatsinks outside those envelopes remain blocked;
- fabrication-focused intent maps to stricter validation/component/routing
  policy, but external manufacturer acceptance remains a downstream
  fabrication-readiness concern.

Simulation-backed amplifier evidence is opt-in. The generated simulation stage
does not run unless a runner is configured by the caller or test harness. When
it does run, `.kicadai/amplifier-simulation.json` records normalized
measurements and `.kicadai/design-promotion.json` includes a `simulation` gate.
Treat `simulation: pass` as narrow headphone-slice evidence only; fabrication
or power-amplifier claims still require clean KiCad ERC/DRC, physical-rule,
SOA, thermal, and protection evidence.

Explicit `generic-circuit-v1` graphs use a separate always-deterministic trusted
model registry. Model compatibility and parameters resolve from the immutable
component catalog. Legacy analytic models accept only model ID, component-role
bindings, bounded scalar inputs, and metric bounds. Graph-derived MNA accepts
only bounded DC/AC analyses, source conditions, and structured node assertions;
it compiles topology from resolved nets and trusted resistor, capacitor,
independent-source, and op-amp primitive claims. Provider equations, matrices,
code, model files, and topology labels are rejected. Unsupported nonlinear,
unstable, singular, incompatible, or unbounded systems fail closed.

Current gaps for autonomous one-shot schematic + PCB generation:

- end-to-end autonomous design still depends on the planner's structured intent
  request and verified block/component coverage;
- placement is deterministic and conservative, not a full board-layout
  optimizer;
- routing is suitable for small known-good nets and local fragment routes, not
  dense production boards;
- writer correctness now reports generated-file gaps explicitly; remaining
  failures mostly reflect missing footprint assignments, resolver-backed
  pinmaps, and strict PCB reader/writer coverage;
- KiCad ERC/DRC requires `kicad-cli` from KiCad 7 or newer and is optional
  unless requested.
