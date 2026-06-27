# Circuit Block Library Readiness Review

Date: 2026-06-13

This review records the release-readiness status for the initial circuit block
library. The library is ready for AI-assisted schematic generation experiments.
It is not ready for unattended fabrication output.

## Verification Summary

| Block ID | Level | Fabrication-ready eligible | Notes |
|---|---|---:|---|
| `led_indicator` | `structural` | No | Checked-in example now passes KiCad schematic round-trip; broader parameter, ERC, and PCB evidence still missing. |
| `connector_breakout` | `experimental` | No | Checked-in example now passes KiCad schematic round-trip; generic connector pin naming is not electrically verified. |
| `voltage_regulator` | `structural` | No | Basic LDO topology; part-specific stability data is not modeled. |
| `i2c_sensor` | `structural` | No | I2C topology, pull-ups, interrupt, and decoupling are represented. |
| `opamp_gain_stage` | `structural` | No | Gain topology is represented; op-amp operating limits are not validated. |
| `usb_c_power` | `structural` | No | USB-C sink topology is represented; no-connect markers and connector variants remain gaps. |
| `mcu_minimal` | `structural` | No | Fixed ATmega328P-A role map; arbitrary MCU semantics remain unsupported. |

Fabrication-ready eligibility is intentionally restricted in code to
`roundtrip_verified`, `erc_drc_verified`, and `reference_verified`.

## Validation Run

Normal test suite:

```sh
go test ./...
```

Result: passed. In the isolated development environment this was run with
`GOCACHE` pointed at a writable temporary directory.

Block example inspection:

```sh
go build -o /tmp/kicadai ./cmd/kicadai
find examples/blocks -mindepth 1 -maxdepth 1 -type d \
  ! -name requests \
  ! -name reports \
  -print0 |
while IFS= read -r -d '' d; do
  /tmp/kicadai inspect project "$d" >/dev/null
done
```

Result: passed. Each project reports a warning for the intentionally omitted
`.kicad_pcb` file, and each `.kicadai/manifest.json` is non-stale after PCB
artifact references were removed.

Generated block schematic text checks:

```sh
! rg --fixed-strings -- '-0.0' examples/blocks
rg -n '"Datasheet"|"Description"' --glob '*.kicad_sch' examples/blocks
find examples/blocks -name '*.kicad_pcb' -print
```

Result: no `-0.0` matches, required `Datasheet` and `Description` properties
are present on generated symbols, and no generated `.kicad_pcb` files remain.

Structured CLI failures:

- `block show missing_block` returns structured JSON with `MISSING_FILE`.
- invalid block requests return structured JSON with `VALIDATION_FAILED`.
- missing compose request returns structured JSON with `INVALID_ARGUMENT`.

Result: covered by CLI tests.

## Opt-In Integration Run

KiCad CLI round-trip was run locally with KiCad CLI reporting `10.0.3`.
Configure the executable path for your system:

```sh
export KICADAI_KICAD_CLI=/path/to/kicad-cli
```

For this run on macOS, the exact executable was
`/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli`.

Focused block fixture command:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI="$KICADAI_KICAD_CLI" \
go test -v ./internal/kicadfiles/roundtrip -run TestKiCadRoundTripCheckedInBlockSchematics -count=1
```

Result: passed for the checked-in `led_indicator` and `connector_breakout`
block schematics.

Resolver integration was run against local KiCad library checkouts:

```sh
KICADAI_RUN_LIBRARY_INTEGRATION=1 \
KICADAI_KLC_ROOT=/path/to/klc \
KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols \
KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints \
KICADAI_TEMPLATES_ROOT=/path/to/kicad-templates \
go test ./internal/libraryresolver
```

Result: failed.

Observed blocker:

- `Device:Q_NPN_BEC` was not found in the local symbol library checkout during
  the `Package_TO_SOT_THT:TO-92_Inline` compatibility case.

The resolver can still support many common block references, but the current
local corpus does not satisfy all expected integration fixtures.

Round-trip integration tests were run:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI="$KICADAI_KICAD_CLI" \
go test ./internal/kicadfiles/roundtrip -count=1
```

Result: failed.

Remaining non-block blockers:

- `TestKiCadRoundTripCheckedInGeneratedSchematic` still changes an older
  non-block fixture.
- `TestKiCadRoundTripGeneratedLEDSchematic` still changes embedded
  `lib_symbols`; the helper emits simplified power/device symbol definitions
  that KiCad expands during save.

No block was promoted to `roundtrip_verified` in this pass. The focused
checked-in block examples pass, but verification levels should represent the
block implementation across parameter combinations, not just two fixtures.

## Gap Matrix

| Area | Current status | Blocker | Next action |
|---|---|---|---|
| Resolver | CLI and API exist; roots can be configured; many library records resolve. | Local integration fixture misses at least `Device:Q_NPN_BEC`; resolver semantics are not rich enough for arbitrary MCU alternate functions or USB-C variants. | Refresh resolver fixtures against the current KiCad libraries and add semantic role metadata for complex symbols. |
| Writer | Project and schematic writers generate parseable block examples; generated block schematics now canonicalize zero coordinates and emit required default symbol properties. | Older non-block schematic fixtures and the direct LED helper still have round-trip churn, especially simplified embedded `lib_symbols`. | Regenerate/fix non-block fixtures and teach helper-generated embedded symbols to match KiCad save output. |
| Validation | Inspect/evaluate/pinmap commands exist; block CLI failures are structured. | Block examples are not wired into a single readiness report command; ERC/DRC is not part of normal block validation. | Add a block readiness command or report that aggregates inspect, evaluate, pinmap, resolver, and round-trip results. |
| Electrical domains | Composition checks voltage-domain conflicts and block params validate basic units. | Regulator stability, USB-C no-connect policy, op-amp operating limits, MCU clock/reset/application constraints, and sensor-specific requirements are not modeled. | Add part-specific rule packs and conservative blockers for unknown electrical requirements. |
| Placement/routing | Blocks emit schematic locations and basic PCB placement hints. | No placement solver, no schematic routing quality rules, no PCB routing for composed designs, and no DRC-backed routing validation. | Add constraint-driven placement, net-class-aware routing, zones, and KiCad DRC integration. |
| Fabrication readiness | Code limits fabrication-ready eligibility to `roundtrip_verified` or stronger levels. | All current blocks remain `structural` or `experimental`; two checked-in schematic fixtures passing round-trip is not enough for fabrication eligibility. | Promote blocks only after broader round-trip, ERC/DRC, and reference-design evidence is recorded. |

## Release Decision

The initial circuit block library can be used for:

- deterministic schematic/project generation experiments;
- AI block selection and composition workflows;
- inspection/evaluation pipeline development;
- gathering round-trip and resolver gap evidence.

It must not be used to claim:

- fabrication-ready PCB output;
- ERC/DRC-clean designs;
- verified symbol-to-footprint compatibility for arbitrary libraries;
- full autonomous design without user review.
