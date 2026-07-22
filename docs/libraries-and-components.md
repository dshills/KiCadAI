# Libraries And Components

Component catalog, pinmap, and KiCad library resolver reference.

### Component Intelligence

Component intelligence provides a deterministic catalog and selection layer for
AI-facing generation. The checked-in default catalog lives in `data/components/`
and is embedded in released binaries, so it does not depend on the caller's
working directory. It can be overridden with `--catalog-dir`. Records include
KiCad symbol IDs, package variants, footprint IDs, function pins, pad functions,
ratings, values, and verification confidence.

Confidence levels are:

- `verified`: explicit evidence is available.
- `library_derived`: derived from KiCad library metadata.
- `rule_inferred`: limited safe inference, mainly symmetric passives.
- `placeholder`: draft-only structural stand-in.
- `blocked`: known unsafe or incomplete.

Selection is acceptance-gated. Draft requests may use placeholders with
warnings. Connectivity, ERC/DRC, and fabrication-candidate requests reject
placeholder active components and require verified evidence except for narrowly
allowed passive rule-inferred records.

Examples:

```sh
kicadai component list
kicadai component show resistor.generic.0805
kicadai component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai --request examples/components/select_resistor.json component select
kicadai --request examples/components/select_concrete_resistor.json component select
kicadai component validate
```

When symbol or footprint roots are configured, `component validate` also
resolves every selectable catalog binding against those KiCad libraries. It
fails before provider execution on missing symbols, units, pins, footprints,
or pads and reports a deterministic `library_validation` summary. Explicitly
blocked records remain excluded.

```sh
kicadai --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  component validate
```

The catalog includes a verified first-slice alternative set for common
generated parts, plus explicit blocked placeholders for unsupported power
devices:

- 0603/0805 resistor and capacitor seeds;
- Panasonic EEUFR1C221 220 uF/16 V polarized radial capacitor with verified
  KiCad pin/pad polarity; fabrication use still requires ESR, ripple-current,
  lifetime, and derating review;
- 1x02 through 1x06 Samtec pin headers;
- 0603/0805 Lite-On indicator LEDs;
- Signal and Schottky diodes plus a SOD-323 ESD/TVS protection diode;
- fixed 3.3 V LDO records;
- verified ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6 MCU/module records
  with physical pins, alternate functions, supplies, programming, clock, boot,
  and current-budget evidence;
- onsemi MMBT3904/MMBT3906 SOT-23 BJT amplifier seeds;
- blocked-by-default NPN TO-220 power-output placeholder.

Connectivity and stronger selection prefer concrete alternatives when they
satisfy the request; draft and structural selection can still use generic
fallbacks. Local source snapshots are curated evidence fixtures, not live
availability or pricing data.

High-value MLCC selections, such as 10 uF 0603/0805 ceramic capacitors, now
carry structured `capacitor_evidence` fields for dielectric, DC-bias review,
effective-capacitance review, and ESR review. AMS1117 and AP2112K regulator
records carry structured `regulator_evidence`. Connectivity workflows surface
these as warnings; fabrication-candidate selection blocks until the required
regulator stability, MLCC derating, and thermal evidence is proven or marked
not applicable.

Reviewed records used by power/interface synthesis also carry typed regulator
startup, transient, quiescent-current, efficiency, dropout, and thermal facts;
driver/receiver impedance and current limits; translator mode/channel/speed
facts; ADC acquisition limits; op-amp capacitive-load/isolation evidence; and
clock amplitude, common-mode, edge, jitter, startup, and frequency evidence.
Normalization and fabrication-level validation fail closed when a record
claims one of these analyses without the required fields.

`design create` includes a `component_selection` stage after block planning and
before schematic or PCB writes. Request JSON can include `component_policy` to
set a catalog directory, minimum confidence, package preferences, per-role
component overrides, and component-specific acceptance. See
`docs/component-intelligence.md` and `examples/components/`.


### Pinmaps

Pinmap validation checks whether schematic symbol-to-footprint assignments have
human-verified pin mappings before fabrication readiness is claimed.

List built-in pinmaps:

```sh
kicadai pinmap list
```

Validate a project:

```sh
kicadai pinmap validate ./examples/01_led_indicator
```

Current built-in mappings include common resistors, capacitors, LEDs, simple
headers, and verified `Transistor_BJT:Q_NPN_BEC` mappings. Missing mappings,
pin-count mismatches, pin-name mismatches, and unflattened hierarchical sheets
block pinmap fabrication readiness.


### Library Resolver

The `library` command indexes local KiCad symbol, footprint, and template
repositories so generators and transactions can use real library IDs.

```sh
export KICADAI_KLC_ROOT=/path/to/klc
export KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols
export KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints
export KICADAI_TEMPLATES_ROOT=/path/to/kicad-templates

kicadai library symbol Device:R
kicadai library footprint Resistor_SMD:R_0805_2012Metric
kicadai library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric
kicadai library pinmap-candidate Device:R Resistor_SMD:R_0805_2012Metric
kicadai library templates
```

Hardened symbol inspection commands expose resolver evidence without requiring
agents to read raw `.kicad_sym` files:

```sh
kicadai library symbols list
kicadai library symbols show Device:R
kicadai library symbols pins Device:R
kicadai library symbols validate Device:R
```

These commands report parsed units, common pins, electrical types, power-symbol
flags, inherited metadata, rough graphics bounds, and resolver diagnostics.
`writer check` can use the same resolver evidence when symbol roots are
configured, and reports a `library_resolver` check when resolution is attempted.

Use `--library-cache .kicadai/library-index.json` for faster repeated loads and
`--refresh-library-cache` to rebuild it. See
[docs/library-resolver.md](library-resolver.md) for setup, command
examples, cache behavior, compatibility statuses, and opt-in integration tests.
