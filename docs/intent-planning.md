# Intent Planning And AI Workflow

Structured-intent generation, rationale reports, and semantic synthesis behavior.

### AI Design Workflow

`design create` is the first deterministic AI-facing workflow. It accepts an
explicit request JSON, selects built-in circuit blocks by ID, generates a
schematic transaction, realizes PCB fragments, places footprints, optionally
routes, writes the KiCad project, runs structural/connectivity validation, and
returns stage-by-stage feedback.

```sh
kicadai \
  --json \
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
- `sensor_breakout.json`: connectivity-oriented request that may produce
  actionable routing/validation feedback as the workflow matures.

The top-level JSON result uses requested acceptance as the success contract. A
request for `structural` acceptance can return `ok: true` while still including
connectivity or KiCad-check issues for higher levels. Those issues are retained
in `issues[]` and grouped under `data.feedback.repairs[]` so an agent can decide
whether to revise the request, adjust placement/routing, or ask for external
tooling.


### Intent Planner

The `intent` command family sits above `design create`. It is intended for AI
callers that can produce structured intent but should not need to know the
exact block IDs, validation defaults, or generated workflow request shape.

```sh
kicadai \
  --json \
  --request ./examples/intent/sensor_breakout.json \
  --output ./out/intent_plan \
  --overwrite \
  intent plan

kicadai \
  --json \
  --request ./examples/intent/sensor_breakout.json \
  intent explain

kicadai \
  --json \
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
kicadai --json --text "make a 3.3V I2C temperature sensor breakout" intent draft
kicadai --json --file ./examples/intent_text/i2c_temperature_sensor_breakout.txt intent explain
kicadai --json --file ./examples/intent_text/i2c_temperature_sensor_breakout.txt intent rationale
kicadai --json --text "battery powered sensor" --strict intent draft
```

Flags are global in the current CLI and must appear before `intent`.

Without `--output`, `intent draft` writes the draft report to stdout only.
`--output ./out/draft` writes `intent-source.txt`,
`intent-draft.json`, `intent-extraction.json`, and
`intent-clarifications.json`. `--text/--file ... intent explain` drafts first
and stops on blocking clarifications. `--text/--file ... intent create` drafts
first, refuses blocking clarifications, then runs the existing planner and
design workflow. Generated projects from prose persist the draft artifacts
under `.kicadai/`.

`intent rationale` accepts exactly one source mode:

```sh
kicadai --json --request ./examples/intent/sensor_breakout.json intent rationale
kicadai --json --text "make a 3.3V I2C temperature sensor breakout" intent rationale
kicadai --json --file ./examples/intent_text/i2c_temperature_sensor_breakout.txt intent rationale
kicadai --json --target ./out/intent_sensor intent rationale
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
- fabrication-focused intent maps to stricter validation/component/routing
  policy, but external manufacturer acceptance remains a downstream
  fabrication-readiness concern.

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
