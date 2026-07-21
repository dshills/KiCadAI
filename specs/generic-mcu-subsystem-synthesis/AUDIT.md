# Generic MCU Subsystem Synthesis Completion Audit

Date: 2026-07-21

## Outcome

The initial three-target milestone is complete. Behavior-level requirements can
select a verified ATmega328P-A, ESP32-WROOM-32E, or STM32G031K8T6 without naming
the target, allocate compatible peripheral bundles to physical functions, add
catalog-declared support networks, lower the resulting subsystem, and pass the
same deterministic physical/KiCad gates as the existing promoted designs.

This is generic within verified catalog evidence. It is not a claim that an
arbitrary MCU, RF design, dense board, or undocumented electrical condition can
be synthesized safely.

## Requirement Evidence

| Requirement | Implementation and proof |
| --- | --- |
| Typed MCU evidence | `components.MCUEvidence` models supply domains, physical pins, alternate functions, programming, clocks, boot constraints, and current budgets. `TestValidateMCUEvidenceAcceptsMappedResources`, `TestValidateMCUEvidenceRequiredForVerifiedMCU`, invalid-mapping tests, and shuffled-order normalization cover the contract. |
| Three verified families | The checked-in catalog contains normalized ATmega328P-A TQFP-32 and ESP32-WROOM-32E evidence plus the new STM32G031K8T6 LQFP-32 record. Catalog, symbol, footprint, and pinmap validation pass. |
| Deterministic target selection | `selectAssignableMCU` evaluates verified candidates and orders feasible results by generic capability/package/resource/identity evidence. The neutral corpus and shuffled-evidence tests prove stable selection. |
| Complete pin bundles | The bounded backtracking solver handles GPIO, UART, I2C, SPI, PWM, ADC, interrupt, programming, and clock reservations. Mixed-peripheral, bundle-instance, exhaustion, and shuffled-order tests cover the solver. |
| Electrical rejection | Stable MCU codes cover voltage domain, pin current, aggregate current, clock availability/frequency, and modeled I2C pull-up loading. Boot, programming, and clock pins are reserved before user peripherals are allocated. |
| Support networks | Catalog companions supply per-target decoupling/bulk, reset, boot, programming/debug, and conditional external-clock networks. Assignment-dependent I2C pull-ups resolve through the selected physical SDA/SCL functions, validate sink/rise-time evidence when supplied, and remain idempotent with existing sensor pull-up policy. External interfaces are exported from otherwise unbound participant roles; existing translator/protection providers remain request-driven. |
| Stable failure output | Provider rejection codes survive architecture search and map to stable behavioral capability gaps, including `mcu_pin_assignment_impossible`, with deterministic candidate detail. |
| Provenance stability | `ResolutionHash` retains full catalog/library provenance. `GenerationHash` binds physical generation only to selected circuit content, so unrelated catalog additions do not perturb placement or routes. `TestGenerationHashIgnoresUnselectedCatalogRecords` protects this boundary. |
| Neutral held-out evidence | Three requests under `internal/architecturesearch/testdata/mcu_synthesis_corpus` describe voltage, wireless, programming, and peripheral behavior without target IDs. They select ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6 respectively and replay deterministically. |
| Full physical promotion | `TestNeutralMCUSynthesisCorpusPassesOfflineWorkflow` and `TestNeutralMCUSynthesisCorpusOptionalKiCadPromotion` require schematic/electrical, placement, routing, project write, writer correctness, validation, KiCad ERC/DRC, complete routing/connectivity, strict round trip, and two-run normalized equality. |
| Existing evidence preserved | The optional installed-KiCad tier remains green for `esp32_wroom_32e_minimal_pass`, `usb_c_i2c_sensor_3v3_protected`, and `usb_c_led_indicator_protected`. The complete hermetic repository suite passes. |

## Reproducible Commands

Hermetic repository gate:

```sh
GOCACHE=/tmp/kicadai-go-cache go test ./... -timeout 900s
```

Installed-KiCad MCU promotion:

```sh
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
GOCACHE=/tmp/kicadai-go-cache \
go test -v ./internal/compositionlowering \
  -run '^TestNeutralMCUSynthesisCorpusOptionalKiCadPromotion$' \
  -count=1 -timeout 300s
```

Installed-KiCad regression fixtures:

```sh
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
GOCACHE=/tmp/kicadai-go-cache \
go test -v ./internal/designworkflow \
  -run 'TestDesignExamplesOptionalKiCadBackedTier/(usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected|esp32_wroom_32e_minimal_pass)$' \
  -count=1 -timeout 300s
```

The recorded local tool was KiCad CLI 10.0.3. The final MCU promotion completed
in 33.02 seconds; all three subtests passed.

## Frozen Input And Generated Artifact Hashes

The target-free corpus inputs are:

| Case | Input SHA-256 |
| --- | --- |
| debuggable mixed-peripheral controller | `b35d3fef22d6301478bc5f0c8126d373aeb79884ccf5200a59f98ba50910b365` |
| five-volt serial controller | `d23d8fcf7db31d6e524c1bb228c40f5b823f7a09010854282a60e8ac84c7a524` |
| wireless sensor controller | `32648a4fc1d1aafbf0a001a16e3efccea8fcfc4993855f0515f5fca59c6d3f08` |

The first generated run in the final KiCad-backed replay produced:

| Case | Schematic SHA-256 | PCB SHA-256 |
| --- | --- | --- |
| debuggable mixed-peripheral controller | `58225cd4bda28ffe74a6a5d1cbe98b9f7beaa3da5d9521daf0c100d9d7dae376` | `d03a69ccc6255c4d160e6a5117059ce52f0beb369247eaed1165e75a44cdaf18` |
| five-volt serial controller | `20630bb6d5f360ab80478d864405cedc199356dd32aade95628dd0bccf6ff3e4` | `d1dee0046e4e8cc85439634c6878d3eccb17d848c7da7974a69b4b4f68282033` |
| wireless sensor controller | `7c4f743a3a1e6540a48503aae0b9ba17c5cbcbcf4989921474160432d0e327f1` | `dcd60be4dbde21a610f3955be1ccdd48f7a39c1e6df030d7c695ecc36f438b80` |

The harness compares normalized schematic and PCB bytes across both generated
runs and requires strict zero-diff KiCad round trips. Temporary validation and
round-trip working directories are intentionally excluded from these hashes.

## Boundaries And Next Goal

The verified slice deliberately does not infer MCU evidence from arbitrary
KiCad symbols. UART/SPI fanout, ADC source impedance/settling, programming-pin
external loading, RF supply dynamics, and startup sequencing are enforced only
where the request and catalog expose modeled evidence. The fixed reviewed I2C
pull-up recipes are validated against supplied speed/capacitance evidence; the
catalog does not yet choose among a broad resistor range or solve a whole-bus
signal-integrity model.

The next goal should therefore be deterministic power-tree and signal-integrity
synthesis for composed digital subsystems: size rails and transient
decoupling, derive bus bias and supported level transitions from whole-bus
loads, validate startup/boot/programming interactions, and promote neutral
failure-driven cases through the same KiCad-backed gates.
