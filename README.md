# KiCadAI

KiCadAI is a Go toolkit for AI-assisted KiCad design work. It combines:

- a Go client for probing KiCad's live IPC API;
- direct KiCad project, schematic, and PCB file writers;
- a CLI for generation, inspection, validation, repair planning, component intelligence, placement/routing evidence, fabrication readiness previews, and structured intent planning.

The near-term goal is to let agents build and review KiCad-native projects from structured intent while keeping the lower-level writer strict, deterministic, and testable.

## Status

The direct-file workflow is the main functional path today. KiCadAI can generate KiCad project directories, write root schematics and PCBs, inspect and evaluate projects, run writer and board validation, select catalog-backed components with rating evidence, write selected component identity properties into generated schematic symbols, perform block-aware placement/routing, run optional KiCad ERC/DRC checks, and produce deterministic intent plans and rationale reports.

Live KiCad IPC support is useful for connection probes, version checks, document discovery, and capability reporting. Live schematic/PCB mutation through IPC remains limited by the write commands exposed by the current KiCad API surface, so design generation is done by writing KiCad files directly.

Imported project review is now read-only by default and includes structured
preservation evidence. `inspect` and `evaluate` report preservation-only KiCad
content, transaction planning returns operation-level preservation reviews, and
`transaction apply` blocks existing-project writes unless `--allow-imported-apply`
is explicitly supplied after reviewing the plan.

Generated schematic workflows now emit schematic readability and electrical-rule
evidence. The foundation includes deterministic role/stage/lane classification,
conservative component placement rules, orthogonal schematic routing, label
fallback for long/shared nets, geometry overlap diagnostics, and workflow
summary metrics. Generated schematic checks also expose structured evidence for
duplicate refs, floating/conflicting labels, required/no-connect pins, power
rails, decoupling/value/rating policy hooks, pin anchors,
wire/label/no-connect attachment, and power-driver policy before optional KiCad
ERC runs. `design create` includes a `schematic_electrical` stage, and
`evaluate schematic` / `evaluate project` include a `schematic_electrical`
check.
The first generator improvements also spread the op-amp gain-stage block and
prevent design API schematic connections from being emitted as diagonal-wire
segments.
Checked-in simple schematic examples now have standard readability regression
gates, and the Class AB, Class A, and op-amp headphone-buffer examples have
strict amplifier readability gates for left-to-right signal flow, feedback
placement, rail/return placement, and orthogonal wiring. This is a foundation,
not a full schematic editor. Future work still includes imported schematic
mutation, automatic hierarchy/page splitting, exact KiCad text justification
geometry, and broader example regeneration.

Generated design PCB net assignment now propagates pad and copper net names
through placement/project write, resolves KiCad 10 name-only net references
during PCB readback, and reports net-assignment evidence in the `design create`
workflow. The simple LED prompt now reaches strict promotion `candidate` in the
default structural lane without requiring local KiCad, and promotes to `pass`
when required KiCad ERC/DRC and writer round-trip evidence are clean. The
optional KiCad-backed I2C fixture is now a checked-in `pass` fixture when
required KiCad ERC/DRC evidence is clean; the LED fixture remains
candidate-level, while connector/LED and amplifier fixtures remain `expected_fail` until their
required KiCad ERC/DRC evidence is clean. Generated block-local routes now
bind to physical same-net pad anchors and report route-connectivity evidence.
Generated inter-block route candidates now promote connector/LED and I2C
breakout request connections into placement/routing evidence. Routing summaries
now distinguish candidates, attempted routes, endpoint-contact evidence,
graph-connected routes, partial routes, and unrouted nets. Multi-endpoint
route-tree execution now manages the I2C fixture's VCC/GND/SDA/SCL nets and
reports branch-level path/contact evidence. Route-tree branch execution now
ranks pad and local-route access candidates, tries bounded access pairs, keeps
access-selected route endpoints out of post-route pad snapping, and records
selected access roles in branch evidence. Route-tree contact graph evidence now
includes local-route anchors, same-net segment intersection/overlap merges,
same-net copper merge evidence, and via layer transitions. The I2C
fixture now emits all 8 route-tree branches, proves all 12 required contacts,
reports four graph-complete route-tree nets, and is reproducible as a checked-in
KiCad-backed `pass` fixture with clean required ERC/DRC evidence. Route-tree repair classifies contact blockers, feeds repairable hints
into bounded placement retry, and ranks selected attempts by route-tree
completion evidence. Route-tree diagnostics now separate fixed-net skip notices
and missing-net-class warnings from repairable blockers. The next blockers are
generated schematic connectivity/readability sufficient to clear KiCad ERC,
broader rich-board routing coverage, and KiCad DRC-clean pass evidence.

Fabrication readiness now includes expanded deterministic physical-rule
evidence for annular rings, copper feature widths, polygonal copper width and
edge-clearance checks, polygonal solder-mask web checks,
edge-plating/castellation policy, impedance/differential-pair evidence gaps,
and basic fabrication metadata. Physical-rule thresholds can now come from
built-in or local fabrication profiles, with profile provenance recorded in
readiness reports and package manifests. These checks improve local DFM
visibility, but they are still conservative evidence, not manufacturer
acceptance.

KiCadAI is not yet a general autonomous "make me any board" system. It works best with supported structured intent, verified circuit blocks, and catalog-backed components. Broader component coverage, topology synthesis, validation feedback, and production layout proof are still active roadmap areas.

Amplifier-focused coverage now includes checked-in Class AB, Class A, and
op-amp headphone-buffer schematic fixtures, amplifier semantic tests, structured
intent fixtures, a draft generated op-amp headphone-buffer design request, and
an optional KiCad-backed `expected_fail` fabrication-candidate fixture. These
fixtures are regression and evidence tools only. The generator now has a
connectivity-level Class AB headphone output-stage path with deterministic
LMV321 op-amp selection, MMBT3904/MMBT3906 output-device selection,
diode-string biasing, and single-supply AC output coupling through
`headphone_output_protection`. That block models the DC-blocking capacitor,
required bleed/reference policy, optional series output resistor, connector
return/reference diagnostics, and blocked speaker/bridge/power-amplifier
scope. The protected KiCad-backed fixture now passes schematic electrical
checks, PCB realization, placement, endpoint binding, and routing enablement.
It now stops at route completion before project write: all 20 block-local
routes bind to physical same-net pad anchors, six required inter-block nets are
graph-complete, and the remaining blocking evidence is a partial VCC route-tree
contact group with `output.3` unproven. Routing summaries now include compact
missing-endpoint trace evidence plus a repairable `graph_split` VCC hint that
points AI callers at the same-net contact graph repair instead of a generic
failure. Generated amplifier designs are not fabrication-ready until that
required-net blocker, active output fault protection,
speaker/bridge/power-amplifier load safety, SOA and thermal evidence, analog
stability/layout rules, and KiCad ERC/DRC-clean proof are available.

Amplifier simulation support is now available as an opt-in evidence layer for
Go-level and test-harness integrations; a stable CLI flag or environment
variable for external simulator execution is not exposed yet. Runners are
currently injected programmatically through the Go API or exercised by the test
suites. The Class AB headphone simulation module can:

- emit a deterministic SPICE3/ngspice-oriented netlist;
- run through an injected simulation runner;
- normalize gain, high-pass cutoff, output DC offset, quiescent current, load
  current, output swing, and stability margin measurements;
- write `.kicadai/amplifier-simulation*` artifacts.

Passing simulation evidence can feed the promotion report through a
`simulation` gate, but it is still evidence for the narrow low-voltage
headphone slice, not a power-amplifier or fabrication-readiness claim.

## Requirements

- Go 1.22 or newer.
- KiCad 9 or newer is the target for current file output and IPC protobufs.
- `kicad-cli` is optional, but recommended for round-trip checks and external KiCad ERC/DRC validation.
- `protoc` is only needed when regenerating vendored protobuf bindings.

The repository vendors KiCad API protobuf definitions under `third_party/kicad/api/proto` and commits generated Go bindings under `internal/kiapi/gen`.

## Quick Start

Run tests:

```sh
make test
```

Build a local binary:

```sh
go build -o bin/kicadai ./cmd/kicadai
```

Run the compiled CLI:

```sh
kicadai --help
```

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

## Common Commands

Probe a running KiCad instance:

```sh
kicadai config
kicadai ping
kicadai version
kicadai documents
kicadai capabilities
```

Inspect and validate generated projects:

```sh
kicadai inspect project ./examples/07_generated_pcb
kicadai evaluate project ./examples/07_generated_pcb
kicadai writer check ./examples/07_generated_pcb
kicadai validate board ./examples/07_generated_pcb
```

Plan imported-project edits before applying them:

```sh
kicadai transaction plan ./project ./tx.json
kicadai --allow-imported-apply transaction apply ./project ./tx.json
```

Use component intelligence:

```sh
kicadai component list
kicadai component show resistor.generic.0805
kicadai component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai --request examples/components/select_concrete_resistor.json component select
kicadai component validate
kicadai --source-dir ./data/component-sources component coverage
```

The checked-in catalog now includes a verified first-slice component set for
common generated designs:

- 0603/0805 resistor seeds and voltage-rated Murata ceramic capacitor seeds;
- 1x02 through 1x06 2.54 mm pin headers and indicator LEDs;
- Signal and Schottky diodes plus a SOD-323 ESD/TVS protection diode;
- AMS1117 SOT-223 and AP2112K SOT-23-5 fixed 3.3 V LDOs;
- onsemi MMBT3904/MMBT3906 SOT-23 BJT amplifier seeds;
- blocked-by-default NPN TO-220 power-output placeholders for unsupported
  power-device scenarios.

Source snapshots are local curated evidence fixtures; they are not live
availability, pricing, or distributor data. AMS1117 fabrication-candidate use
still requires output-capacitor ESR/stability review; MLCC DC-bias review
applies broadly to Class II X5R/X7R ceramic-capacitor designs.

Generated schematic symbols include hidden KiCadAI component identity
properties when selected evidence is available, including component IDs,
variant IDs, roles, block IDs, manufacturer/MPN, class, confidence, source,
lifecycle, availability, and pinmap evidence. BOM and fabrication exports
consume those schematic properties first, with legacy property names retained
as fallback. The full property contract is documented in
`specs/component-symbol-properties/SPEC.md`.

Inspect generated identity evidence:

```sh
kicadai --request examples/design/led_indicator.json --output ./out/led_indicator --overwrite design create
kicadai inspect schematic ./out/led_indicator/led_indicator.kicad_sch
kicadai export bom ./out/led_indicator
```

Inspect fabrication profiles:

```sh
kicadai fabrication profile list
kicadai fabrication profile show generic_assembly
kicadai fabrication profile validate ./profiles/my-board-house.json
```

Use `--manufacturer-profile` to select a built-in or local physical-rule
profile, and `--manufacturer-profile-dir` or
`KICADAI_FABRICATION_PROFILE_DIR` to load trusted local JSON profile
snapshots.

KiCadAI normalizes the request `name` to a safe basename inside the output
directory when choosing generated KiCad filenames, with fallback naming for
invalid or empty names.

Plan or create from structured intent:

```sh
kicadai --request ./examples/intent/sensor_breakout.json --output ./out/intent_plan --overwrite intent plan
kicadai --request ./examples/intent/sensor_breakout.json intent explain
kicadai --text "make a 3.3V I2C temperature sensor breakout" intent rationale
kicadai --text "make a simple LED indicator board" --output ./out/ai_led --overwrite intent create
kicadai --request ./examples/intent/sensor_breakout.json --output ./out/intent_sensor --overwrite intent create
```

Use `intent create --overwrite` only for disposable output directories; it
authorizes KiCadAI to replace generated output at that path. Do not use
`intent create --overwrite` as the retry-loop default if you need to preserve
`.kicadai/retry-state.json`; keep separate agent-side retry bookkeeping or
rerun against the same generated output when tracking automatic retry budgets.
This warning is about project generation. `repair apply --overwrite` is a
separate repair command authorization for KiCadAI-managed generated files.

### AI-Controlled Generation Lane

The shortest current path for an AI agent is prompt-driven `intent create`.
First-lane prompts include simple LED indicator, connector breakout with power
LED, and 3.3 V I2C sensor breakout requests. "First-lane" means deterministic,
instrumented, and fail-closed; it does not guarantee `ready` status yet. The
simple LED prompt now emits a KiCad project with `data.ai_status.status` set to
`candidate` in the default structural lane, and
`.kicadai/design-promotion.json` now records `achieved_readiness: candidate`
with explicit missing-KiCad evidence. When rerun with `--kicad-cli`,
`--require-erc`, and `--require-drc`, clean KiCad evidence promotes that report
to `pass`; KiCad findings remain precise blockers. Broader prompts can still
stop at placement, routing, validation, clarification, unsupported, or tool
blockers. The command drafts the
intent, plans supported blocks, runs the deterministic design workflow, and
writes `.kicadai/` evidence artifacts even when blocked:

```sh
kicadai --text "make a simple LED indicator board" --output ./out/ai_led --overwrite intent create
```

Read `data.ai_status.status` in the CLI JSON response. The file
`.kicadai/validation-summary.json` contains the `ai_status` object itself, so
the status field is at the root JSON property `status`; this file is intended
as the compact persisted status, not a full CLI response wrapper. In short:
stdout uses `data.ai_status.status`; the file uses `status`. Important statuses
are `ready`, `candidate`, `blocked`, `needs_clarification`, `unsupported`, and
`tool_error`. Retryable blockers
include `retry_allowed`, `retry_key`, `max_automatic_retry_attempts`,
`current_automatic_retry_attempt`, `repair_category`, optional
`repair_bundle_path`, and artifact paths. Revalidate after any repair.
Mains/high-voltage and ambiguous battery prompts fail closed instead of
guessing.

Run KiCad-backed checks when `kicad-cli` is available:

```sh
kicadai check erc ./examples/checks/erc_fail/erc_fail.kicad_sch
kicadai check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
kicadai --kicad-cli /path/to/kicad-cli --require-erc --require-drc --text "make a simple LED indicator board" --output ./out/ai_led_kicad --overwrite intent create
```

## Intent Planning

The intent planner is the higher-level AI orchestration layer. It accepts structured intent requests, derives requirements and constraints, maps supported goals to circuit blocks, emits assumptions and known gaps, and can hand the generated request to `design create` for project generation.

Planner synthesis traces include topology decisions, bus and voltage-domain evidence, component policy constraints, value calculations, applied/deferred/blocked calculation status, and fail-closed gaps. Supported calculations can now write safe generated block parameters for LED resistors, I2C pull-ups, crystal load capacitors, and the verified AP2112K 3.3 V LDO slice. Regulator current, capacitor voltage policy, dropout/headroom, thermal review, and stability review evidence are persisted in generated planner artifacts; op-amp gain remains explicit requirement evidence unless a block exposes safe direct mutation.

Amplifier intents currently produce explicit partial or blocked evidence unless
they map to supported low-voltage headphone slices. The Class AB headphone
output-stage path is connectivity-level only: it can select the seeded
MMBT3904/MMBT3906 pair, realize the diode-biased output stage, require
single-supply AC output coupling through `headphone_output_protection`, and
explain bleed/reference, connector-return, optional series-resistor, and fault
protection assumptions. Simulation-backed promotion can now be attached when a
local runner is configured and the generated evidence stays inside the modeled
headphone limits. Class A output stages, speaker loads, active fault
protection, and power-amplifier thermal/current proof remain roadmap work.

See [Intent Planning And AI Workflow](docs/intent-planning.md) for details.

## Documentation

Detailed reference material lives in [docs/](docs/README.md):

- [KiCadAI Agent Skill](docs/kicadai-agent-skill.md)
- [CLI Reference](docs/cli-reference.md)
- [Intent Planning And AI Workflow](docs/intent-planning.md)
- [Circuit Blocks](docs/circuit-blocks.md)
- [Libraries And Components](docs/libraries-and-components.md)
- [Placement And Routing](docs/layout-routing.md)
- [Validation And Analysis](docs/validation-and-analysis.md)
- [Fabrication Export And Readiness](docs/fabrication.md)
- [Development Reference](docs/development.md)

Focused subsystem docs also live in `docs/`, including direct file writers, component intelligence, circuit block readiness/verification, the library resolver, and validation repair.

## Repository Map

- `cmd/kicadai`: CLI entrypoint.
- `internal/intentplanner`: structured intent planner and synthesis trace.
- `internal/designworkflow`: generated project workflow orchestration.
- `internal/blocks`: reusable circuit block registry and realization logic.
- `internal/components`: component catalog, selection, ratings, and evidence checks.
- `internal/kicadfiles`: project, schematic, PCB, and lower-level KiCad file writers.
- `internal/schematiclayout`: schematic readability classification, placement, routing, and diagnostics.
- `internal/placement` and `internal/routing`: layout and routing engines.
- `internal/boardvalidation`, `internal/writercorrectness`, and `internal/repair`: validation and deterministic repair foundations.
- `data/components`: checked-in component catalog.
- `examples`: generated projects, intent requests, checks, repair fixtures, and CLI examples.
- `specs`: roadmap, specifications, and phased implementation plans.

## Development

Run all tests:

```sh
go test ./...
```

Run formatting before committing Go changes:

```sh
gofmt -w <changed-go-files>
```

Build the CLI into `bin/`:

```sh
make build
```

Install the CLI with the project Makefile:

```sh
make install
```

See [Development Reference](docs/development.md) for package details, test coverage notes, protobuf maintenance, current limitations, troubleshooting, and design direction.

## License

KiCadAI is licensed under the [MIT License](LICENSE). Vendored KiCad API
materials under `third_party/kicad/` retain their upstream licenses.
