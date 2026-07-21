# KiCad-Backed Design Examples

These examples are optional `design create` fixtures for local runs with real
`kicad-cli` evidence. They are separate from the default design example set so
the normal Go test suite remains independent of a local KiCad installation.

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
| `connector_led_kicad_smoke` | `candidate` | Tracks connector-to-LED multi-block composition with clean required KiCad ERC/DRC, KiCad-native net assignment, and routed inter-block endpoint contact evidence. Writer round-trip evidence remains warning-only when the fixture does not configure a separate writer-side KiCad CLI invocation. |
| `i2c_sensor_breakout_candidate` | `pass` | Tracks the generic I2C sensor breakout with VCC/GND/SDA/SCL alias propagation, complete route-tree/contact-graph evidence, writer correctness, and clean required KiCad ERC/DRC. The historical fixture name is retained for compatibility. |
| `sensor_bmp280_breakout` | `pass` | Reproduces the concrete BMP280 structured-intent result through the environment-gated design fixture lane with verified Bosch identity, LGA-8 footprint/pad mapping, complete required-net routing, and clean KiCad ERC/DRC evidence. |
| `usb_c_led_indicator_pass` | `pass` | Tracks a USB-C powered LED indicator generated from natural-language intent using `usb_c_power` plus `led_indicator`, project-local USB-C symbol export, verified USB4125 pad transfer, routed VBUS/GND connectivity, and clean required KiCad ERC/DRC evidence. |
| `usb_c_led_indicator_protected` | `pass` | Tracks the protected USB-C LED variant with fuse, TVS, and bulk capacitance enabled. Its schematic layout is inferred from component roles and non-ground topology, with no hand-authored layout coordinates. The checked-in metadata promotes it through the optional KiCad-backed fixture lane; the latest reproduced run is documented below. |
| `usb_c_i2c_sensor_3v3_protected` | `pass` | Tracks the medium-complexity protected USB-C, AMS1117, and I2C sensor composition with complete physical VCC_3v3 route-tree contact proof, clean strict KiCad ERC/DRC, writer correctness, and normalized round-trip evidence. |
| `esp32_wroom_32e_minimal_pass` | `pass` | Tracks the exact ESP32-WROOM-32E-N4 minimal-system block with 3.3 V power conditioning, EN/BOOT straps and buttons, UART/I2C/SPI/GPIO headers, a hard all-copper antenna keepout, complete local/inter-block route contact proof, clean strict KiCad ERC/DRC, writer correctness, and zero normalized round-trip diffs. |
| `class_a_bjt_line_preamplifier` | `pass` | Tracks the bounded Class-A line-level BJT preamplifier through electrical, physical, simulation, KiCad ERC/DRC, routing, writer, and round-trip gates. |
| `class_ab_headphone_driver` | `expected_fail` | Retains the unprotected Class-AB skeleton as an explicit negative fixture pending the protected output and proof requirements. |
| `class_ab_headphone_protected` | `pass` | Tracks the protected Class-AB headphone amplifier through verified component selection, protection behavior, simulation, layout, routing/connectivity, clean required KiCad ERC/DRC, writer correctness, and zero-diff round trip. |
| `class_ab_speaker_10w_protected` | `pass` | Tracks the bounded protected dual-rail 10 W RMS/8 ohm speaker amplifier fabrication candidate, including electrothermal, SOA, current-limit, DC-fault, mute, star/Kelvin/high-current, package, KiCad, writer, and round-trip evidence. |
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

## ESP32-WROOM-32E Minimal-System Pass Evidence

The exact ESP32-WROOM-32E-N4 profile is a checked-in KiCad-backed `pass`
fixture. Reproduce its strict ERC, DRC, connectivity, writer, route-completion,
and round-trip gates with:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test -v ./internal/designworkflow \
  -run 'TestDesignExamplesOptionalKiCadBackedTier/esp32_wroom_32e_minimal_pass$' \
  -count=1
```

The block uses the native `RF_Module:ESP32-WROOM-32E` symbol and footprint and
the cataloged `ESP32-WROOM-32E-N4` identity. Support is intentionally exact;
other ESP32 modules, package variants, flash sizes, RF layouts, and arbitrary
GPIO remapping remain unsupported until separately cataloged and verified.

## Protected USB-C I2C 3.3 V Pass Evidence

The medium-complexity protected USB-C I2C fixture is a checked-in
KiCad-backed `pass` fixture. Run it from the repository root outside the
restricted sandbox to reproduce strict ERC, DRC, and normalized round-trip
evidence:

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

The route tree binds `rail3v3.vout` to the regulator's physical VOUT pad shape
and proves every required VCC_3v3 endpoint in one contact-graph component.
The regulator realization emits only physical bypass copper; inter-block route
trees provide the rail connections without virtual anchor copper. On the
declared 100 mm board, fragment placement uses deterministic left-to-right
flow, which preserves clearance around the regulator rail.

Read `examples/.generated/usb_c_i2c_sensor_3v3_protected/.kicadai/design-promotion.json`
after a run. A current strict KiCad 10.0.3 run reports promotion `pass`, zero
ERC violations, zero DRC violations, zero unconnected items, writer correctness
pass, and zero normalized schematic and PCB round-trip diffs.

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
generated design. The remaining expected failures are the unprotected
Class-AB skeleton and the older op-amp headphone-buffer seed.

The generic I2C fixture is now a real `pass` lane. Route-tree execution owns
VCC/GND/SDA/SCL, emits all eight branches, proves all required connector and
block-local endpoint contacts, and reports four complete contact-graph groups,
including local-route, same-net intersection/overlap, and via-transition merge
evidence. Project write, writer correctness, structural validation, and
required real KiCad ERC/DRC evidence must all pass. Logic-only mocked KiCad
tests cover promotion decisions but are not production pass evidence. The
protected USB-C LED fixture likewise has stable required KiCad pass evidence
when run outside the restricted sandbox.
Sandboxed macOS DRC aborts are execution-environment failures to rerun outside
the sandbox before classification, not fixture `known_gaps` or board
violations.

Promotion gates currently include metadata, stages, writer correctness,
connectivity, KiCad checks, route completion, physical rules, and artifacts.
Missing `kicad-cli` evidence is recorded as skipped external evidence, but it
still blocks readiness when ERC or DRC is required. The LED and connector/LED
smoke fixtures remain `candidate`; the generic I2C and protected amplifier
fixtures are `pass`; only the two draft amplifier seeds remain
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
