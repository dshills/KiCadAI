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

On macOS, the app-bundled `kicad-cli pcb drc` may abort when run inside the
Codex restricted command sandbox or similarly isolated CI environments because
the DRC job path touches wx/macOS application registration. If direct DRC
crashes with exit `134` or the workflow reports
`tool_error_kind: no_output_crash`, rerun the same command outside that
sandbox. This is an execution-environment issue when the same board passes DRC
outside the sandbox.

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
| `led_indicator_kicad_smoke` | `candidate` | Tracks the smallest design-level KiCad-backed smoke path with schematic electrical checks, block-local route contact proof, writer correctness, board validation, and warning-only KiCad evidence. |
| `connector_led_kicad_smoke` | `expected_fail` | Tracks connector-to-LED multi-block composition with KiCad-native net assignment and routed inter-block endpoint contact evidence. It currently blocks on required KiCad ERC pin/endpoint evidence. |
| `i2c_sensor_breakout_candidate` | `candidate` | Tracks the richer sensor breakout candidate after placement, local route contact proof, VCC/GND/SDA/SCL alias propagation, route-tree execution, contact graph evidence, project-write, writer-correctness, structural validation, and warning-level optional KiCad evidence. Pass promotion still requires clean required KiCad DRC evidence from a stable `kicad-cli`. |
| `sensor_bmp280_breakout` | `pass` | Reproduces the concrete BMP280 structured-intent result through the environment-gated design fixture lane with verified Bosch identity, LGA-8 footprint/pad mapping, complete required-net routing, and clean KiCad ERC/DRC evidence. |
| `usb_c_led_indicator_pass` | `pass` | Tracks a USB-C powered LED indicator generated from natural-language intent using `usb_c_power` plus `led_indicator`, project-local USB-C symbol export, verified USB4125 pad transfer, routed VBUS/GND connectivity, and clean required KiCad ERC/DRC evidence. |
| `usb_c_led_indicator_protected` | `pass` | Tracks the protected USB-C LED variant with fuse, TVS, and bulk capacitance enabled. Its schematic layout is inferred from component roles and non-ground topology, with no hand-authored layout coordinates. The checked-in metadata promotes it through the optional KiCad-backed fixture lane; the latest reproduced run is documented below. |
| `usb_c_i2c_sensor_3v3_protected` | `pass` | Tracks the medium-complexity AI-generated target fixture: protected USB-C input, AMS1117 5 V to 3.3 V regulation, I2C sensor, pull-ups, decoupling, and header. It has clean required KiCad ERC/DRC, route-completion, writer-correctness, and zero-diff round-trip evidence for `erc-drc` acceptance. Fabrication readiness remains a separate, stricter gate. |
| `class_ab_headphone_protected` | `expected_fail` | Tracks the protected Class AB headphone amplifier path with verified LMV321/op-amp and output transistor selections plus `headphone_output_protection`; schematic electrical validation, PCB realization, placement, endpoint binding, project write, and writer-correctness evidence now run. The current blocker is structural validation evidence for schematic label/connectivity issues and unrouted or partially routed PCB nets before KiCad ERC/DRC promotion. Fabrication promotion still also requires active output fault protection, speaker/bridge/power-amplifier load safety, LOAD_REF/GND net-tie evidence, promoted thermal/SOA evidence, and analog stability/layout proof. |
| `opamp_headphone_buffer_kicad_candidate` | `expected_fail` | Tracks the draft op-amp headphone-buffer seed when promoted to fabrication-candidate requirements; current blockers are missing verified amplifier component evidence, migration to the protected Class AB headphone output path, active fault-protection proof, analog layout proof, and KiCad ERC/DRC promotion evidence. |

## Protected USB-C LED Pass Evidence

The protected USB-C LED variant is a checked-in KiCad-backed `pass` fixture.
It was last reproduced in this repository by running the workflow outside the
restricted sandbox. The ERC/DRC artifacts below record the local KiCad version
and report timestamps.

Run from the repository root:

```sh
make build
KICADAI_KICAD_CLI=/path/to/kicad-cli
./bin/kicadai \
  --request examples/design/kicad-backed/usb_c_led_indicator_protected.json \
  --output examples/.generated/usb_c_led_indicator_protected \
  --overwrite \
  --kicad-cli "$KICADAI_KICAD_CLI" \
  --require-erc \
  --require-drc \
  --require-kicad-roundtrip \
  --keep-artifacts \
  --artifact-dir examples/.generated/usb_c_led_indicator_protected/.kicadai/checks \
  design create
```

Observed promotion status:

- `promotion.status`: `pass`
- `promotion.achieved_readiness`: `pass`
- `acceptance.achieved`: `erc-drc`
- KiCad ERC report:
  `examples/.generated/usb_c_led_indicator_protected/.kicadai/checks/kicadai-check-erc-2411734681/erc.json`
- KiCad DRC report:
  `examples/.generated/usb_c_led_indicator_protected/.kicadai/checks/kicadai-check-drc-1509461091/drc.json`
- PCB round-trip normalized diff:
  `examples/.generated/usb_c_led_indicator_protected/.kicadai/checks/pcb-roundtrip-3229415959/normalized.diff`
- Schematic round-trip normalized diff:
  `examples/.generated/usb_c_led_indicator_protected/.kicadai/checks/usb_c_led_indicator_protected-259474249/normalized.diff`

The ERC report has no violations. The DRC report has no violations and no
unconnected items. Both normalized round-trip diffs are zero bytes. The
checked-in request opts into `auto_schematic_layout`; inference places the
USB-C connector, fuse, indicator resistor, and LED in left-to-right power-flow
order while keeping CC pull-downs, TVS protection, and bulk capacitance below
and near their owning power stages.

## Protected USB-C I2C 3.3 V Pass Evidence

The medium-complexity protected USB-C I2C fixture is a checked-in KiCad-backed
`pass` fixture for `erc-drc` acceptance. Run from the repository root:

```sh
make build
KICADAI_KICAD_CLI=/path/to/kicad-cli
./bin/kicadai \
  --request examples/design/kicad-backed/usb_c_i2c_sensor_3v3_protected.json \
  --output examples/.generated/usb_c_i2c_sensor_3v3_protected \
  --overwrite \
  --kicad-cli "$KICADAI_KICAD_CLI" \
  --require-erc \
  --require-drc \
  --require-kicad-roundtrip \
  --keep-artifacts \
  --artifact-dir examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/checks \
  design create
```

Observed evidence:

- promotion status and achieved readiness: `pass`;
- KiCad ERC report:
  `examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/checks/kicadai-check-erc-<run-id>/erc.json`;
- KiCad DRC report:
  `examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/checks/kicadai-check-drc-<run-id>/drc.json`;
- PCB normalized round-trip diff:
  `examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/checks/pcb-roundtrip-<run-id>/normalized.diff`;
- schematic normalized round-trip diff:
  `examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/checks/usb_c_i2c_sensor_3v3_protected-<run-id>/normalized.diff`.

Run IDs are generated for each execution. Read
`examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/design-promotion.json`
for the exact ERC, DRC, and round-trip artifact paths produced by a run.

ERC and DRC contain no violations, DRC reports no unconnected items, required
route groups are complete, writer correctness passes, and both normalized
round-trip diffs are zero bytes. The selected AMS1117 and 10 uF capacitors have
verified connectivity-level identity and pin/pad evidence suitable for this
acceptance level. Manufacturer-specific ESR, effective-capacitance, and thermal
review are still required before claiming fabrication readiness.

## BMP280 Structured-Intent Pass Evidence

The authoritative input is
`examples/intent/sensor_bmp280_breakout.json`. The optional fixture lane checks
the deterministic synthesized request at
`examples/design/kicad-backed/sensor_bmp280_breakout.json` with readiness
declared by `sensor_bmp280_breakout.metadata.json`.

Neither checked input carries hand-authored `schematic_layout` coordinates or
relationships. The intent input opts into `auto_schematic_layout`, that policy
is retained in the generated design request, and block planning derives functional groups and
ranks, power/signal/ground lanes, page-centering and spacing rules, and
`near`/`above`/`right_of` relations from active component roles and non-ground
net topology. Derived targets use stable `instance__role` IDs. The generated
schematic therefore does not depend on the project name, generated reference
hashes, or fixture-specific coordinate tables.

Run the full intent workflow from the repository root with the compiled binary
on `PATH`:

```sh
export KICADAI_KICAD_CLI=/path/to/kicad-cli
kicadai \
  --request examples/intent/sensor_bmp280_breakout.json \
  --output examples/.generated/sensor_bmp280_breakout \
  --overwrite \
  --kicad-cli "$KICADAI_KICAD_CLI" \
  --require-erc \
  --require-drc \
  --require-kicad-roundtrip \
  --keep-artifacts \
  --artifact-dir examples/.generated/sensor_bmp280_breakout/.kicadai/checks \
  intent create
```

Re-run only the checked-in environment-gated fixture:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test -v ./internal/designworkflow \
  -run 'TestDesignExamplesOptionalKiCadBackedTier/sensor_bmp280_breakout' \
  -count=1
```

Evidence is written under
`examples/.generated/sensor_bmp280_breakout/.kicadai/`:

- promotion: `design-promotion.json`;
- workflow and route-completion evidence: `workflow-result.json`;
- generated transaction and identity provenance: `transaction.json`;
- KiCad ERC: `checks/kicadai-check-erc-*/erc.json`;
- KiCad DRC: `checks/kicadai-check-drc-*/drc.json`;
- PCB round-trip diff: `checks/pcb-roundtrip-*/normalized.diff`;
- schematic round-trip diff: `checks/sensor_bmp280_breakout-*/normalized.diff`.

A passing run reports promotion readiness `pass`, zero KiCad ERC/DRC
violations, zero DRC unconnected items, all 16 required PCB endpoints proven,
and zero-byte normalized round-trip diffs.

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
counts. The fixture now promotes to structural `candidate` with optional
warning-level KiCad evidence:
route-tree execution owns the four I2C nets, emits all 8 route-tree branches,
proves all required connector and block-local endpoint contacts, and reaches
project-write, writer-correctness, and structural validation evidence. Generated
label stubs are grid-safe. A logic-only unit test also exercises the promotion
decision path with mocked clean KiCad results, but that mocked path is not
production pass evidence; real clean KiCad CLI DRC evidence is still required
for `pass` promotion. Route-tree contact
graph evidence now reports four complete contact-graph groups, local-route
merge evidence, same-net segment intersection/overlap merges, and via layer
transitions. Route-tree diagnostics also separate fixed-net preservation
notices and missing-net-class warnings from repairable blockers. The protected
USB-C LED variant is now part of the optional promotion lane and has stable
required KiCad DRC/pass evidence when run outside the restricted sandbox.
Sandboxed macOS DRC aborts are execution-environment failures to rerun outside
the sandbox before classification, not fixture `known_gaps` or board
violations.

Promotion gates currently include metadata, stages, writer correctness,
connectivity, KiCad checks, route completion, physical rules, and artifacts.
Missing `kicad-cli` evidence is recorded as skipped external evidence, but it
still blocks readiness when ERC or DRC is required. LED and I2C currently
promote to `candidate`; amplifier optional KiCad-backed fixtures are still
`expected_fail`. Do not treat a generated board as `pass` until the promotion
report achieves `pass` and the configured KiCad ERC/DRC evidence gates pass.

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
