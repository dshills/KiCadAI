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
| `led_indicator_kicad_smoke` | `expected_fail` | Tracks the smallest design-level KiCad-backed smoke path after pad/copper net-code assignment and physical local-route endpoint binding; current blocker is full validation plus KiCad ERC/DRC-clean evidence. |
| `connector_led_kicad_smoke` | `expected_fail` | Tracks connector-to-LED multi-block composition after local route endpoint binding and routing-enabled inter-block contact evidence; current blocker is graph-connected route completion for all required contacts plus DRC-clean promotion. |
| `i2c_sensor_breakout_candidate` | `expected_fail` | Tracks the richer sensor breakout candidate after VCC/GND/SDA/SCL inter-block candidate and contact-evidence promotion; current blocker is routed same-net completion and DRC-clean promotion. |
| `opamp_headphone_buffer_kicad_candidate` | `expected_fail` | Tracks the draft amplifier seed when promoted to fabrication-candidate requirements; current blockers are missing verified amplifier component evidence, output DC-blocking/protection realization, analog layout proof, and KiCad ERC/DRC promotion evidence. |

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
generated design. These fixtures now document that generated design-level PCBs
can progress past writer correctness net-code assignment and block-local route
endpoint binding. The next layout-quality blockers are full inter-block routing
coverage for richer boards and KiCad ERC/DRC-clean evidence.

Promotion gates currently include metadata, stages, writer correctness,
connectivity, KiCad checks, route completion, physical rules, and artifacts.
Missing `kicad-cli` evidence is recorded as skipped external evidence, but it
still blocks candidate/pass readiness when ERC or DRC is required. The current
fixtures remain `expected_fail`; do not treat a generated board as promoted
until the promotion report achieves `candidate` or `pass` and the configured
KiCad ERC/DRC evidence gates pass.

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
