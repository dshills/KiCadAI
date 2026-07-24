# Generic Standalone Clock Generation Audit

## Outcome

The frozen `clock_generation` benchmark is closed. Both behavior-only cases
pass with distinct, catalog-backed architectures, and the original held-out
`digital_clock_source` now passes the same closed-loop and installed-KiCad
pipeline. This advances the held-out benchmark from 11/12 to 12/12 without
changing the frozen specification, plan, baseline, corpus test, manifest, or
requirements.

| Case | Selected architecture | Result |
| --- | --- | --- |
| `precision_logic_clock` | SiTime SiT8008 fixed packaged oscillator plus qualified SN74LVC1G17 endpoint | pass |
| `relaxed_logic_clock` | Analog Devices LTC6906 resistor-programmed relaxation oscillator plus qualified SN74LVC1G17 endpoint | pass |
| held-out `digital_clock_source` | requirement-selected fixed packaged oscillator path | pass |

## Generic closure

- Architecture selection evaluates accuracy, jitter, startup, supply,
  temperature, load, fanout, edge, duty cycle, and frequency requirements. It
  uses typed deterministic rejection codes and does not inspect request
  identities.
- The relaxation architecture solves its timing resistance from the requested
  frequency. The fixed architecture uses a concrete frequency-qualified
  oscillator record.
- Each accepted candidate records eight supply/temperature/tolerance corner
  evaluations, output high/low bounds, edge time, load, fanout, supply current,
  bypass adequacy, component evidence, and reviewed model provenance.
- The simulation registry selects a transient workflow only when a connected
  primitive is an autonomous transient source. Optional transient models on
  capacitors, relays, and other components do not perturb unrelated circuit
  selection.
- Clock and timing nets carry generic preferred-layer and maximum-length
  contracts. Clock paths also carry a declared return net and maximum return
  distance. Acceptance measures those constraints from the final emitted
  segments and vias.
- The optional LTC6906 GRD output is explicitly unconnected. The catalog does
  not claim a bare-copper guard ring that the writer did not generate.
- A bundled LTC6906 symbol and deterministic installed-library footprint
  hydration make the result reproducible without fixture-specific library
  substitutions.

## Evidence

The final installed-KiCad run executed each case twice and passed ERC, strict
DRC, connectivity, route completion, writer correctness, and zero-difference
schematic and PCB round trips:

```text
TestStandaloneClockGenerationCorpusOptionalKiCadPromotion
  precision_logic_clock.json  PASS
  relaxed_logic_clock.json    PASS
TestHeldOutClockGenerationCorpusOptionalKiCadPromotion
  digital_clock_source.json   PASS
```

The protected regression remained green:

```text
TestDesignExamplesOptionalKiCadBackedTier/usb_c_i2c_sensor_3v3_protected PASS
```

The two-family clean-checkout input is
`PROMOTION_MATRIX.json`. It uses the existing generic `requirement` lane and
the frozen behavior corpus, runs each scenario twice, requires all internal
and installed-KiCad gates, and feeds the existing content-addressed bundle
builder and verifier.

The checked-in component coverage, function-level capability report,
schematic provenance goldens, tolerance capability report, and model
provenance hashes were regenerated through their supported derivation paths.
`FINAL_REPORT.json` and `FINAL_REPORT.sha256` record the milestone result.

## Scope

This closes standalone fixed packaged and resistor-programmed logic clocks. It
does not claim PLL, differential, spread-spectrum, RF phase-noise, or
fabrication-release capability.
