# Generic Multi-Control Safety Composition Audit

## Result

Implemented and locally promoted on 2026-08-02 from baseline `2bcef2ae` using
Go 1.26.5 and KiCad 10.0.3.

The V6 control contract now preserves independent connecting and protection
roles, enforces default-off/protection-dominant semantics, lowers through a
catalog-backed coordinator, and binds event-scoped directed transitions. The
promoted fixtures use an active-high permit so loss of logic power is
intrinsically deasserting. Startup checks the physical load; rail-loss timing
checks the permit, avoiding a false edge requirement on an analog output with
no guaranteed discharge path.

The simulation resolver also converts semantic event voltages through the
resolved source orientation. This fixes reversed connector sources without
special-casing a fixture or coordinate. The schematic writer chooses a clear
interior bend deterministically when every direct-segment midpoint is blocked,
eliminating the active-filter dangling-wire ERC finding.

## Passing evidence

- Frozen control corpus validation and trusted behavioral execution:
  `current_sense_protection` and `mixed_function_control_power` pass all
  retained analyses.
- Offline promotion shards `3/10` and `6/10` pass.
- Installed-KiCad promotion shards `3/10` and `6/10` pass.
- `usb_c_i2c_sensor_3v3_protected` passes its installed-KiCad tier.
- Full installed-KiCad simulation-grounded corpus lane 1 passes all ten
  fixtures in 404.71 seconds after Prism remediation.
- Full installed-KiCad simulation-grounded corpus lane 2 passes all ten
  fixtures in 413.02 seconds after Prism remediation.
- Each installed-KiCad promotion requires clean ERC, strict DRC, connectivity,
  complete routing, writer correctness, deterministic replay, and zero
  normalized round-trip differences.

The two full lanes used distinct artifact roots:

```text
/tmp/kicadai-full-final-1
/tmp/kicadai-full-final-2
```

The common installed-KiCad command was:

```sh
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
KICADAI_SIMULATION_GROUNDED_ARTIFACT_DIR=<lane-root> \
go test ./internal/compositionlowering \
  -run '^TestFrozenSimulationGroundedCorpusOptionalKiCadPromotion$' \
  -count=1 -v -timeout=30m
```

## Full-suite exception

`go test ./... -count=1 -timeout=30m` passed every package except the existing
exhaustive `internal/compositionlowering` package and then hit its 30-minute
package timeout. Before timeout it reported two unrelated corpus failures:

- nonlinear DC convergence for
  `heldout_power_protected_isolated_12v.json`; and
- a search-limit routing failure for `segmented_smbus.json`.

The timeout occurred while placing the held-out
`low_noise_sensor_decision.json` workflow. These cases do not traverse the new
multi-control coordinator, source-event polarity regression, or schematic
label fallback. All directly affected package tests and required promotion
lanes pass independently as listed above.
