# KiCadAI

KiCadAI is a Go toolkit and CLI for AI-assisted KiCad design. It provides
deterministic project, schematic, and PCB generation; structured intent and
schematic IR inputs; component and circuit-block selection; placement and
routing; inspection, validation, and repair evidence; and optional KiCad
ERC/DRC and round-trip checks.

The primary workflow writes KiCad-native files directly. Live KiCad IPC is
available for connection probes, version checks, document discovery, and
capability reporting, but current KiCad IPC write support is not the generation
path.

## Current State

- Direct project, schematic, and PCB writers are functional and extensively
  tested.
- Structured intent can generate supported designs through planning, component
  selection, schematic/PCB realization, placement, routing, and validation.
- Every normalized creation request is deterministically classified as
  `supported`, `experimental`, or `unsupported` before project mutation.
  Experimental generation requires explicit `--experimental` opt-in and is
  barred from fabrication-ready and promotion-pass claims; unsupported
  requests fail closed with evidence-linked diagnostics. Assessments are
  re-evaluated through later workflow stages and embedded in workflow,
  promotion, and manifest artifacts.
- Unsupported assessments can now be converted into deterministic,
  content-addressed expansion plans. Source-backed component, model, physical,
  verification, and declarative architecture additions remain quarantined as
  `experimental`; automatic representative/adversarial cases and the complete
  workflow/KiCad gate contract produce a review-ready bundle only after every
  result passes. An exact-hash approval and explicit mutation command are
  required to update the supported-capability registry, which behavior-only
  requirement generation can load for fresh requests.
- Ordinary behavior-first requests can enter through the uncertainty-aware
  `intent compile` trust boundary. It accounts for every source statement,
  asks the minimum blocking clarification, records stable capability gaps, and
  releases a strict v3 requirement only after deterministic architecture
  search and trusted all-corner closed-loop evidence pass.
- Exact `ESP32-WROOM-32E-N4` minimal-system generation is supported through the
  built-in `esp32_wroom_32e_minimal` block, with power conditioning, EN/BOOT
  straps and buttons, UART/I2C/SPI/GPIO headers, and an antenna copper keepout.
- Behavior-driven MCU subsystem synthesis can select verified ATmega328P-A,
  ESP32-WROOM-32E, or STM32G031K8T6 targets from catalog evidence, assign real
  alternate-function pins deterministically, and add catalog-declared power,
  reset, programming, boot, and optional clock support. Three neutral requests
  reach the full installed-KiCad promotion lane without naming their target.
- Behavior-driven clock/programming synthesis now calculates verified external
  crystal load networks, bounds crystal drive/stability/startup, and records
  programming frequency, loading, voltage, reset/boot entry, and shared-pin
  isolation evidence. External-crystal ISP and integrated-clock UART cases pass
  the local installed-KiCad lane; unsupported unpowered SWD fails closed.
- Behavior-driven MCU power-integrity synthesis now replaces fixed decoupling
  recipes for verified ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6
  selections. It derives source drop, brownout headroom, ESR and capacitive
  droop from startup/transient behavior, qualifies concrete capacitors for
  tolerance, voltage derating, ripple, and temperature, emits one local
  network per supply domain plus shared bulk support per rail group, and
  records finalized calculation evidence. Three target-free held-out cases
  pass the complete local installed-KiCad lane; evidence, budget, and
  temperature gaps fail closed.
- The provider-backed natural-language lane retains two promoted bounded
  profiles and adds a catalog-resolved circuit graph contract with either an
  explicit graph or strict function-level intent. Function intent names
  functions, interfaces, operating domains, and bounded constraints; KiCadAI
  deterministically supplies verified parts, support networks, unused-pin
  policy, physical defaults, and routes.
- Generated `generic-circuit-v1` designs now have a deterministic correction
  loop with at most three placement/routing attempts, protected-invariant
  checks, stable retry evidence, and fail-closed unsupported actions.
- The ESP32-WROOM-32E minimal-system fixture and the bounded USB-C BMP280 and
  protected LED profiles are KiCad-backed `pass`. Six generic fixtures use the
  shared catalog-resolved graph contract:
  RC filter, protected USB-C LED, protected USB-C BMP280, single-stage LMV321,
  dual LMV321, and multi-unit LM358. Their recorded/live evidence varies by
  fixture, while each checked-in optional KiCad promotion lane is `pass`. None
  requires a topology-specific provider schema. The LM358 fixture additionally
  proves one-package, multi-unit schematic lowering with one footprint and BOM
  identity.
- A frozen held-out function-level corpus adds eight independently authored
  analog, power/protection, transistor, sensor/interface, ATmega328P, and
  ESP32/SHT31 circuits. All eight pass deterministic lowering, catalog and
  KiCad library resolution, support expansion, placement, complete routing,
  clean ERC and strict DRC, connectivity, writer correctness, zero round-trip
  differences, and byte-identical replay. The checked-in
  [capability report](specs/function-level-circuit-synthesis/CAPABILITY_REPORT.json)
  records the authoritative hashes and per-circuit gates.
- A second frozen adversarial corpus composes 10 unfamiliar multi-function
  circuits from 35 objectives and 3 abstract participants. It exercises 18
  registered capabilities and 23 whole-circuit voltage, current, startup,
  loading, stability, tolerance, isolation, thermal, response-time, noise, and
  board constraints. All 10 pass deterministic replay, lowering, writer,
  routing/connectivity, zero-diff round trip, and installed-KiCad ERC and strict
  DRC. See the [composition audit](specs/adversarial-multi-function-composition/AUDIT.md).
- A frozen 24-prompt behavioral-intent corpus adds 12 paraphrase groups across
  amplifiers, filters, power, protection, sensors, and MCU interfaces. Twelve
  prompts compile to six unique supported contracts, four require targeted
  clarification, and eight produce stable unsupported outcomes. Every supported
  contract passes deterministic replay and the installed-KiCad promotion lane.
  See the [behavioral compiler audit](specs/uncertainty-aware-behavioral-intent-compilation/AUDIT.md).
- Constraint-driven power-tree and interface synthesis now has a four-design
  neutral promotion corpus: regulated MCU/sensor, protected power-MOSFET load,
  buffered ADC acquisition, and Class-AB power/interface cases. All four pass
  deterministic replay, complete routing/connectivity, writer correctness,
  clean installed-KiCad ERC and strict DRC, and zero-difference round trip.
  Ten reordered negative cases prove stable fail-closed power and interface
  diagnostics. See the [completion audit](specs/constraint-driven-power-tree-interface-synthesis/AUDIT.md).
- A versioned twelve-case held-out capability benchmark spans analog, power,
  digital, MCU, sensor, and mixed-signal requirements without prescribing
  topology, components, nets, pins, or coordinates. The frozen installed-KiCad
  baseline passed 5/12 cases. Constant-current regulation and precision
  rectification advanced the original expansion report to 11/12, and generic
  fixed-frequency and resistor-programmed clock generation subsequently closed
  the benchmark at 12/12. Every row now passes simulation,
  routing/connectivity, writer, ERC, strict DRC, zero-diff round trip, and
  deterministic replay. See the
  [specification](specs/held-out-capability-expansion/SPEC.md),
  [baseline](specs/held-out-capability-expansion/BASELINE_REPORT.json), and
  [clock closeout audit](specs/standalone-clock-generation/AUDIT.md).
- A frozen six-system V4 corpus now measures hierarchical synthesis across
  protected Class-AB, precision analog, regulated MCU/sensor, isolated
  mixed-voltage, high-current switched-load, and split-supply monitor systems.
  Production code derives subsystem/block hierarchy, typed boundary contracts,
  shared-resource plans, global backtracking evidence, physical partitions,
  and requirement-to-KiCad traceability. All six pass the complete offline and
  installed-KiCad two-run promotion gates. This is a bounded catalog-backed
  envelope, not unrestricted arbitrary-circuit generation.
- A frozen six-circuit V5 corpus now adds dynamic electrothermal and
  control-loop synthesis for reactive-load feedback amplification, analog
  servo regulation, switching conversion, protected inductive switching,
  Class-AB load/fault behavior, and sequenced dual rails. Reviewed catalog
  models drive deterministic return-ratio, corner, event, electrothermal,
  transient-SOA, protection, candidate-selection, and bounded-repair evidence.
  Every case passes the complete local KiCad promotion lane, and two
  independent clean roots produce the same 418-file content-addressed bundle.
  See the
  [completion audit](specs/dynamic-electrothermal-control-loop-synthesis/AUDIT.md).
- A separate identity-neutral open-world evaluation freezes discovery and
  held-out behavior-only requirements across six domains, records stable
  failure clusters, and uses those clusters to drive reusable capability
  expansion. Held-out readiness improves from 1/12 to 6/12 without changing
  corpus membership: five newly ready cases cover clock fanout, translated MCU
  debug, push-pull sensor translation, functional isolation, and protected
  isolated power. All five pass the complete local installed-KiCad lane and
  deterministic replay. See the
  [open-world audit](specs/open-world-capability-evaluation/AUDIT.md).
- A frozen four-case protocol-aware bus corpus now covers I2C, SMBus, SPI, and
  UART translation, including whole-bus loading, solved pull-ups, six
  independently isolated branches, mixed push-pull directions, reversed
  voltage domains, inactive startup, partial-power, hot-plug, and contention
  requirements. All four pass the complete local installed-KiCad two-run lane;
  eleven unsafe or incomplete variants fail closed. See the
  [specification](specs/protocol-aware-bus-synthesis/SPEC.md) and
  [promotion matrix](specs/protocol-aware-bus-synthesis/PROMOTION_MATRIX.json).
- Arbitrary electronics generation is not yet guaranteed. Generic graphs fail
  closed on unknown parts, pins, ratings, placement, or routing capability.
- MCU synthesis is limited to verified catalog records and modeled electrical
  constraints. It does not infer arbitrary MCU pin data from KiCad symbols, and
  ESP32 variants, flash choices, RF optimization, and unverified external-bus
  loading remain fail-closed boundaries.
- Generated `pass` evidence is not automatically a fabrication-release claim.
- A clean checkout can discover the locked KiCad 10.0.3 installation or
  bootstrap its checksum-pinned distribution, run the release promotion matrix
  twice, compare normalized projects, and emit an independently verifiable,
  content-addressed evidence bundle with one command.

See [Project Status](docs/project-status.md) for capability boundaries and
[Roadmap](specs/ROADMAP.md) for remaining work.

## Requirements

- Go 1.23 or newer, matching `go.mod`.
- KiCad 9 or newer for current file output and IPC protobuf compatibility.
- `kicad-cli` is optional but recommended for ERC, DRC, and round-trip evidence.
- `protoc` is needed only when regenerating vendored protobuf bindings.

## Install

Build into `bin/`:

```sh
make build
```

Install with the project Makefile:

```sh
make install
```

Confirm the CLI:

```sh
kicadai --help
```

## Quick Start

Generate a simple LED project without contacting KiCad:

```sh
kicadai \
  --output /tmp/led_indicator \
  --name led_indicator \
  --seed demo \
  --with-pcb \
  generate-led-demo
```

Open `/tmp/led_indicator/led_indicator.kicad_pro` in KiCad.

Generate from a checked-in structured design request:

```sh
kicadai --request examples/design/led_indicator.json \
  --output ./out/led_indicator --overwrite \
  design create
```

Inspect and validate the generated project:

```sh
kicadai inspect project ./out/led_indicator
kicadai evaluate project ./out/led_indicator
kicadai writer check ./out/led_indicator
kicadai validate board ./out/led_indicator
```

## AI Generation

Compile an ordinary behavior-first request without allowing the provider to
choose topology, parts, nets, or layout:

```sh
kicadai \
  --file ./behavioral-request.txt \
  --provider openai \
  --ai-profile behavioral-intent-v1 \
  --output ./out/behavioral-request \
  intent compile
```

Only a `ready` result writes
`./out/behavioral-request/.kicadai/behavioral-design-request.json`. A
clarification result writes a hash-bound follow-up template; an unsupported
result writes stable capability-gap evidence and no executable design request.
See [Intent Planning](docs/intent-planning.md#behavioral-intent-compilation) for
the follow-up and project-creation flow.

Run the recorded protected USB-C LED profile with KiCad-backed checks:

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

For a live request, load `OPENAI_API_KEY` from the shell or a secret manager and
replace the recorded-provider flags with `--provider openai`. Provider output is
strict-decoded and remains untrusted until deterministic and KiCad-backed gates
pass.

Agents that already have a valid `generic-circuit-v1` graph can avoid a provider:

```sh
kicadai capability generation --json
kicadai --request ./graph.json circuit preflight
kicadai --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  circuit create --request ./graph.json --output ./out/project --overwrite
```

For rejected generic graphs, run `circuit repair-plan` first. It selects an
executable patch only when one safe correction is fully derived; otherwise it
stops for review. See the [CLI reference](docs/cli-reference.md#generic-circuit-repair-plan)
for the strict patch contract and evidence boundary.

See [AI Generation](docs/ai-generation.md) for bounded and generic modes, live
commands, evidence files, failure behavior, and current limits. AI agents
should also follow the [KiCadAI Agent Skill](docs/kicadai-agent-skill.md).

## Reproduce Promotion Evidence

From an unmodified checkout, run:

```sh
make promotion-bundle
make held-out-promotion-bundle
```

The command builds the repository CLIs, resolves the version and stock
libraries locked by `toolchain/kicad-promotion.lock.json`, bootstraps the
checksum-pinned distribution only when needed, executes every required scenario
twice, verifies all promotion gates and deterministic comparisons, and writes
one content-addressed bundle below
`.tmp/clean-checkout-promotion/bundles/`. No manually configured KiCad or
library paths are required. The output directory must not already exist.
The held-out target uses the same locked toolchain and verifier with the
versioned five-scenario matrix for the two newly supported families, writing
below `.tmp/held-out-capability-promotion/`.

Use `bundle-path.txt` to locate the bundle. Its included files and semantic
promotion claims can be verified offline:

```sh
.tmp/clean-checkout-promotion/bin/kicadai-promotion verify \
  --bundle "$(cat .tmp/clean-checkout-promotion/bundle-path.txt)"
```

This is release-validation evidence for the supported corpus, not a claim that
arbitrary designs or fabrication outputs are automatically qualified.

## Schematic IR

The schematic design/layout IR is a strict JSON handoff for circuit intent,
layout intent, and repair policy. It is not free-form natural language or KiCad
S-expression syntax.

```sh
kicadai --request examples/schematic-ir/led_indicator.json schematic-ir validate
kicadai --request examples/schematic-ir/led_indicator.json schematic-ir normalize
kicadai --request examples/schematic-ir/led_indicator.json \
  --output ./out/ir_led --overwrite schematic-ir write
```

See [Intent Planning And AI Workflow](docs/intent-planning.md) and the
[CLI Reference](docs/cli-reference.md).

## Documentation

Start with the [documentation index](docs/README.md).

| Topic | Reference |
|---|---|
| Current capabilities and limits | [Project Status](docs/project-status.md) |
| Commands and live IPC | [CLI Reference](docs/cli-reference.md) |
| Natural-language provider workflow | [AI Generation](docs/ai-generation.md) |
| Behavioral compilation, structured intent, and planning | [Intent Planning](docs/intent-planning.md) |
| Circuit blocks | [Circuit Blocks](docs/circuit-blocks.md) |
| Components, symbols, and footprints | [Libraries And Components](docs/libraries-and-components.md) |
| Placement and routing | [Placement And Routing](docs/layout-routing.md) |
| Validation, writer checks, and round-trip | [Validation And Analysis](docs/validation-and-analysis.md) |
| Clean-checkout release evidence | [Validation And Analysis](docs/validation-and-analysis.md#reproducible-promotion-bundles) |
| Fabrication evidence | [Fabrication](docs/fabrication.md) |
| Direct KiCad file writers | [KiCad File Writers](docs/kicad-file-writers.md) |
| Tests, packages, and troubleshooting | [Development Reference](docs/development.md) |

## Development

```sh
make test
make lint
make build
```

See [Development Reference](docs/development.md) for focused tests, coverage,
protobuf maintenance, package boundaries, and troubleshooting.

## License

KiCadAI is licensed under the [MIT License](LICENSE). Vendored KiCad API
materials under `third_party/kicad/` retain their upstream licenses.
