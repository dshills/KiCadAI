# Verified Component Family Expansion Audit

Date: 2026-07-02

## Baseline

The checked-in component catalog currently validates and reports deterministic
coverage, but it is still a seed catalog. The current golden coverage snapshot
contains:

- `record_count`: 29
- `family_count`: 12
- verified records: 21
- rule-inferred records: 4
- placeholder records: 4
- concrete records: 11
- generic fallback records: 18
- equivalence groups: 4

## Current Family Coverage

| Family | Records | Concrete | Generic | Equivalence Groups | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| capacitor | 4 | 1 | 3 | 1 | Only one concrete 100 nF 0805 ceramic alternative. Electrolytic remains placeholder. |
| connector | 5 | 1 | 4 | 1 | Generic 1x02-1x05 headers plus one concrete 1x04 Samtec header. |
| crystal | 2 | 1 | 1 | 0 | One concrete 16 MHz 5032 crystal plus generic fallback. |
| diode | 3 | 0 | 3 | 0 | Generic signal, Schottky, and TVS records only. |
| led | 2 | 1 | 1 | 1 | One concrete green 0805 LED plus generic fallback. |
| mcu | 2 | 1 | 1 | 0 | One ATmega328P-A seed record plus placeholder fallback. |
| opamp | 2 | 1 | 1 | 0 | One LMV321 seed record plus generic fallback. |
| protection | 1 | 0 | 1 | 0 | Generic USB/power TVS protection record only. |
| regulator | 2 | 2 | 0 | 0 | AMS1117 3.3 V and AP2112K 3.3 V seed records. |
| resistor | 3 | 1 | 2 | 1 | Generic 0603/0805 plus one concrete 10 kOhm 0805 Yageo record. |
| sensor | 2 | 1 | 1 | 0 | BME280 seed record plus I2C interface fallback. |
| usb_c | 1 | 1 | 0 | 0 | One USB-C power-only seed connector. |

## Target Matrix

The first expansion target matrix is intentionally bounded and is now mirrored
in `internal/components/coverage_test.go`:

- resistor: 0603/0805 values for bias, pull-up, current-limit, and feedback
  uses;
- capacitor: 0603/0805 ceramic values plus radial bulk values for decoupling,
  filtering, and bulk storage;
- connector: 1x02 through 1x06 headers plus audio/programming uses;
- diode: signal, Schottky, and reverse-polarity use cases;
- LED: 0603/0805 common indicator colors;
- protection: ESD/TVS/power-entry protection packages;
- op-amp: SOT-23-5 and SOIC-8 seed audio buffer/gain-stage coverage;
- USB-C: existing power-entry seed coverage.

## Key Gaps

- Passive alternatives do not yet cover common resistor and capacitor
  value/package combinations.
- Diode and protection records are generic despite being polarity-sensitive.
- Connector coverage lacks concrete alternates for most header sizes.
- LED coverage has only one concrete color/package.
- Amplifier-relevant BJT pairs are absent.
- Op-amp coverage is too narrow for audio examples.
- Local lifecycle/availability source snapshots exist for only a small seed
  subset.

## Implementation Notes

- Expansion should prefer small, reviewable catalog slices by family.
- Do not add live supplier claims.
- Do not mark pinout-sensitive parts verified unless symbol, footprint, and
  pad/function evidence are present.
- Generic fallbacks should remain available only where the requested acceptance
  level permits them.
