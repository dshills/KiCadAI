# Adversarial Multi-Function Composition Audit

Date: 2026-07-19

Status: complete for the bounded `kicadai.open-set-requirement.v2` envelope.
This audit does not claim arbitrary-circuit generation.

## Frozen Coverage

- 10 SHA-256-pinned, behavior-only circuits.
- 35 objectives and 3 abstract participants: 38 terminal search obligations.
- 18 distinct objective capabilities spanning analog, digital, power,
  protection, isolation, sensing, and amplifier behavior.
- 23 whole-circuit constraints.
- Every selected search reports all terminal obligations as selected, retains
  at least one distinct complete alternative, identifies the selected
  fingerprint in its rationale, and replays byte-identically.
- The checked-in catalog now contains 95 records across 24 families.

The freeze test rejects fixture implementation details and unmanifested or
modified corpus files. Production code contains no corpus IDs, paths, expected
parts, coordinates, route allowlists, private request schemas, or
corpus-specific block families.

## Global Proof Coverage

Every complete v2 candidate is checked before scoring. The structured evidence
includes:

- compatible voltage windows at shared anchors;
- aggregate explicit and impedance-derived loading against proven source
  capacity;
- supply/rail current budgets and required headroom;
- per-rail selected-part supply-current ratings, explicit operating current,
  domain-matched output obligations, converter efficiency, and solved static
  resistor networks;
- MCU GPIO allocation filtered by declared ADC, DAC, PWM, bus, and digital
  pin capabilities, with physical-pin alias reuse rejected;
- topology-backed startup defaults for loads, outputs, mute paths, and
  open-drain buses;
- composed current-sensor, comparator, and switch response delay;
- catalog/model-backed loop phase margin;
- quadrature integration of selected input-noise densities over the composed
  filter bandwidth using the realized Butterworth order;
- power-device junction-temperature margin at the catalog evidence condition;
- galvanically distinct reference domains;
- passing value, tolerance, and rating calculations;
- component count and catalog-footprint area against the declared board
  envelope.

An unknown system constraint, missing evidence, incompatible contract, unsafe
startup state, exceeded budget, or failed bound rejects the complete candidate
with `ARCHITECTURE_GLOBAL_CONSTRAINT_UNPROVEN` or the more specific current
diagnostic. It never reaches selection or lowering.

## Requirement-By-Requirement Promotion

| Frozen requirement | Composition focus | Offline workflow | Installed KiCad |
|---|---|---:|---:|
| `battery_window_disconnect` | dual thresholds, safety interlock, load switch | pass | pass |
| `current_sensed_load_protection` | current measurement/interlock, threshold, switch, indication | pass | pass |
| `filtered_amplifier_protected_output` | active filter, gain, protected output | pass | pass |
| `hysteretic_load_fault_driver` | hysteresis, inductive switch protection, indication | pass | pass |
| `isolated_protected_gateway` | isolated regulation/bus, translation, ESD protection | pass | pass |
| `mcu_managed_class_ab_output` | MCU GPIO allocation, bias, mute, Class-AB output/protection | pass | pass |
| `precision_sensor_decision_chain` | instrumentation gain, filtering, threshold, indication | pass | pass |
| `protected_sensor_controller` | reverse-blocked input, regulation, I2C translation | pass | pass |
| `regulated_class_a_line_driver` | regulation, Class-A gain, stable protected output | pass | pass |
| `split_supply_analog_frontend` | split rails, active filtering, amplification | pass | pass |

For every row, “offline workflow” means component/rating/value resolution,
complete lowering, two deterministic writes, readable schematic and PCB,
writer correctness, zero normalized round-trip differences, complete
connectivity, and routing. “Installed KiCad” additionally means clean ERC and
strict DRC for both deterministic writes.

The authoritative post-review installed-KiCad run completed all ten in 126.34
seconds. Its artifacts were written under `/tmp/kicadai-adversarial-final-v3`
and are ephemeral CI-style evidence
rather than committed generated projects.

## Preserved Evidence

- The original five-circuit open-set corpus passes, including the protected
  inductive switch's explicit logic-supply endpoint.
- `usb_c_i2c_sensor_3v3_protected` passes its installed-KiCad design-example
  tier.
- `usb_c_led_indicator_protected` passes its installed-KiCad design-example
  tier.
- Component, circuitgraph, architecture-search, lowering, writer, routing,
  round-trip, amplifier, MCU/ESP32, sensor, USB-C, fabrication, and repair
  regressions are covered by `go test ./...`.

## Reproduction

```sh
GOCACHE=/tmp/kicadai-go-cache go test ./...

KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
GOCACHE=/tmp/kicadai-go-cache \
go test ./internal/compositionlowering \
  -run TestFrozenAdversarialMultiFunctionCorpusOptionalKiCadPromotion -count=1
```

## Remaining Boundary

This milestone proves deterministic composition inside 18 registered
capabilities and the 95-record checked-in catalog. It does not prove arbitrary
topologies, arbitrary parts, RF/high-speed behavior, general thermal/mechanical
design, mains safety, dense-board autorouting, or unrestricted natural-language
intent. Those remain fail-closed expansion work.
