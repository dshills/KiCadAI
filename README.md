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
- The provider-backed natural-language lane retains two promoted bounded
  profiles and now adds an explicit catalog-resolved circuit graph contract.
- The bounded USB-C BMP280 and protected LED profiles remain KiCad-backed
  `pass`; a generic RC filter also reaches recorded and live KiCad-backed
  `pass` without a topology-specific provider schema.
- Arbitrary electronics generation is not yet guaranteed. Generic graphs fail
  closed on unknown parts, pins, ratings, placement, or routing capability.
- Generated `pass` evidence is not automatically a fabrication-release claim.

See [Project Status](docs/project-status.md) for capability boundaries and
[Roadmap](specs/ROADMAP.md) for remaining work.

## Requirements

- Go 1.22 or newer.
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

See [AI Generation](docs/ai-generation.md) for bounded and generic modes, live
commands, evidence files, failure behavior, and current limits. AI agents
should also follow the [KiCadAI Agent Skill](docs/kicadai-agent-skill.md).

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
| Structured intent and planning | [Intent Planning](docs/intent-planning.md) |
| Circuit blocks | [Circuit Blocks](docs/circuit-blocks.md) |
| Components, symbols, and footprints | [Libraries And Components](docs/libraries-and-components.md) |
| Placement and routing | [Placement And Routing](docs/layout-routing.md) |
| Validation, writer checks, and round-trip | [Validation And Analysis](docs/validation-and-analysis.md) |
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
