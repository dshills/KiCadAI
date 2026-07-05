# KiCad-Backed Design Examples

These examples are optional `design create` fixtures for local runs with real
`kicad-cli` evidence. They are not part of the default design example set
because they currently document richer board-generation gaps.

Run commands from the repository root. Run the optional test tier:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test -v ./internal/designworkflow -run TestDesignExamplesOptionalKiCadBackedTier -count=1
```

`KICADAI_KICAD_CLI` must point to the `kicad-cli` executable, not the KiCad
application bundle or install directory.

The optional tier now writes a normalized promotion report at
`.kicadai/design-promotion.json` for each generated fixture. The report records
declared readiness, achieved readiness, gates, stage evidence, artifacts,
issues, and repair guidance. The compact promotion summary is also available in
`data.promotion` from `design create`.

Manual command shape:

This assumes the compiled `kicadai` binary is available on `PATH`.

```sh
OUT=./out/kicad-backed/led_indicator_kicad_smoke
mkdir -p "$OUT"
KICADAI_KICAD_CLI=/path/to/kicad-cli \
kicadai \
  --request examples/design/kicad-backed/led_indicator_kicad_smoke.json \
  --output "$OUT" \
  --overwrite \
  --require-erc \
  --require-drc \
  --keep-artifacts \
  --artifact-dir "$OUT/.kicadai/checks" \
  design create
```

## Fixtures

| Fixture | Readiness | Purpose |
| --- | --- | --- |
| `led_indicator_kicad_smoke` | `expected_fail` | Tracks the smallest design-level KiCad-backed smoke path with schematic electrical checks, block-local route contact proof, writer correctness, and board validation. It currently blocks on generated LED schematic label conflicts and required KiCad ERC pin connectivity evidence. |
| `connector_led_kicad_smoke` | `expected_fail` | Tracks connector-to-LED multi-block composition with KiCad-native net assignment and routed inter-block endpoint contact evidence. It currently blocks on required KiCad ERC pin/endpoint evidence. |
| `i2c_sensor_breakout_candidate` | `expected_fail` | Tracks the richer sensor breakout candidate after placement, local route contact proof, VCC/GND/SDA/SCL alias propagation, route-tree execution, contact graph evidence, project-write, writer-correctness, and structural validation. Generated schematic label stubs are now grid-safe; the current real-check blocker is required KiCad ERC pin/endpoint connectivity evidence, not route-tree or project-write evidence. |
| `class_ab_headphone_protected` | `expected_fail` | Tracks the protected Class AB headphone amplifier path with verified LMV321/op-amp and output transistor selections plus `headphone_output_protection`; current blockers are schematic label conflicts before PCB realization, placeholder fault protection, missing HP_RET/LOAD_REF policy and LOAD_REF/GND net-tie evidence, and unpromoted thermal/SOA evidence. KiCad ERC is required for future promotion but currently remains unreachable because schematic electrical validation blocks first; DRC is not required until PCB realization runs. |
| `opamp_headphone_buffer_kicad_candidate` | `expected_fail` | Tracks the draft op-amp headphone-buffer seed when promoted to fabrication-candidate requirements; current blockers are missing verified amplifier component evidence, migration to the protected Class AB headphone output path, active fault-protection proof, analog layout proof, and KiCad ERC/DRC promotion evidence. |

## Interpreting Results

The readiness level is stored in each fixture's `*.metadata.json` file under
the `readiness` field. Each fixture is a pair of files in this directory: a
request file such as `led_indicator_kicad_smoke.json` and a metadata file such
as `led_indicator_kicad_smoke.metadata.json`.

- `pass`: the fixture must complete configured ERC/DRC checks and expose report
  artifacts.
- `candidate`: the fixture is provisionally successful when it completes the
  full optional tier, but has not been promoted to stable pass evidence yet.
- `expected_fail`: the fixture must produce explicit blocked evidence. A silent
  skip or accidental clean pass is treated as an unexpected success.
- `blocked`: the fixture is documented but not run by default.

Tests for `expected_fail` fixtures are considered successful only when they
encounter the documented blockers. That is not the same as an ERC/DRC-clean
generated design. These fixtures document that generated design-level PCBs
can progress past writer correctness net-code assignment and block-local route
endpoint binding. The I2C fixture also exposes route-tree evidence for its
multi-endpoint VCC/GND/SDA/SCL nets, including managed nets, planned branches,
attempted branches, proven endpoints, graph components, and group completion
counts. The current run keeps the fixture in `expected_fail` only because
required KiCad ERC evidence still reports generated schematic pin/endpoint
connectivity findings:
route-tree execution owns the four I2C nets, emits all 8 route-tree branches,
proves all required connector and block-local endpoint contacts, and reaches
project-write, writer-correctness, and structural validation evidence. Generated
label stubs are grid-safe. A logic-only unit test also exercises the promotion
decision path with mocked clean KiCad results, but that mocked path is not
production evidence; real KiCad CLI ERC/DRC evidence is still required for
candidate/pass promotion. Route-tree contact
graph evidence now reports four complete contact-graph groups, local-route
merge evidence, same-net segment intersection/overlap merges, and via layer
transitions. Route-tree diagnostics also separate fixed-net preservation
notices and missing-net-class warnings from repairable blockers. The next
blocker is schematic readability/connectivity work sufficient to clear KiCad
ERC, then KiCad DRC/pass evidence.

Promotion gates currently include metadata, stages, writer correctness,
connectivity, KiCad checks, route completion, physical rules, and artifacts.
Missing `kicad-cli` evidence is recorded as skipped external evidence, but it
still blocks candidate/pass readiness when ERC or DRC is required. The current
optional KiCad-backed fixtures remain `expected_fail`; do not treat a generated
board as promoted until the promotion report achieves `candidate` or `pass` and
the configured KiCad ERC/DRC evidence gates pass.

## Promotion Policy

Fixture metadata is validated as a promotion queue:

- `expected_fail` and `blocked` fixtures must describe their known blockers.
- `candidate` fixtures must require ERC, must require DRC when PCB layout stages
  are expected, and must expect `.kicadai/design-promotion.json`.
- `pass` fixtures must meet the candidate evidence requirements with no
  `known_gaps` and no allowlists.

The `internal/designworkflow.SummarizePromotionFixtures` helper groups fixtures
by declared readiness and preserves tier, acceptance, required ERC/DRC evidence,
and known-gap counts. Use that summary shape for future CLI or documentation
views of the promotion queue.
