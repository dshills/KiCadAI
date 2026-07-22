# Constraint-Driven Power-Tree And Interface Synthesis Completion Audit

Verified 2026-07-22 with Go 1.26.5 on darwin/arm64 and KiCad CLI 10.0.3.

## Result

The bounded catalog-backed milestone is complete. Four neutral, target-free
requirements pass deterministic architecture selection, offline workflow
replay, lowering, complete routing/connectivity, writer correctness, clean
installed-KiCad ERC, strict DRC, and zero normalized round-trip differences.
Ten failure-driven requests retain the same typed code and message after
semantically irrelevant input reordering.

This is not an arbitrary-electronics or fabrication-release claim. It proves
the reviewed component, model, analysis, and physical-synthesis envelope
described by `SPEC.md`.

## Ready corpus evidence

| Requirement | Request SHA-256 | Transaction SHA-256 | Installed-KiCad result |
|---|---|---|---|
| Buffered ADC acquisition | `f5eaa58d20992d329d27c9fd27c63e33dda25c3b51aea19bab48edef63cf8539` | `5c7253895c8810d0530805b34fec2a8be7e6c3e6f0053b0c909eb22c283fd273` | pass |
| Class-AB power interface | `d69518202b296f1c9a1debeb10610e8eaa60d389ce736c1ac0854ccf0957282a` | `58759000b62815ac9e06b8e8add9eb3aa3e09728e45ee1f7d735c63f035ec544` | pass |
| Protected power-MOSFET load | `61e06b72415ceee7815292097dce16d481dd33567de66ed84c26362729c671c1` | `2ec3f902301a5026f8509dee554929d26293b41e4ec212837e743d06656f8ee4` | pass |
| Regulated MCU/sensor subsystem | `25125c0b12e9fdb266ae149b1ef23ed3b80eae42eba40f690d61537132b2f31a` | `7518eef53dde9960397886f0a2a54fc739c63119013de665e41941c734fa628f` | pass |

The authoritative promotion test is
`TestPowerInterfaceSynthesisCorpusOptionalKiCadPromotion`. Its final post-review
run passed all four subtests in 303.62 seconds. Each subtest generates twice and requires
byte-stable transactions, normalized KiCad equality, clean ERC and strict DRC,
zero unconnected items, complete required-net routing/connectivity, and writer
correctness.

## Requirement-to-evidence map

| Specification area | Implementation and test evidence |
|---|---|
| Typed regulator dynamics and fabrication proof | `components.RegulatorDynamicEvidence`; `TestRegulatorDynamicEvidenceSupportsFabricationProof`; malformed/missing startup rejection |
| Translator, ADC, op-amp, and clock facts | typed component interface/clock evidence; deterministic normalization and incomplete-evidence tests |
| Rail source uniqueness and cycles | `validatePowerTreeTopology`; `TestValidatePowerTreeTopologyProvesUniqueAcyclicSources` and deterministic rejection tests |
| Current, quiescent, efficiency, dropout, and thermal demand | catalog power-demand calculations; `power_dynamic_integration_test.go`; MNA regulator/load tests |
| Stability and transient capacitance | `regulatorOutputCapacitor`; upward preferred-value rounding and unavailable/impossible-window tests |
| Sequence, monotonic startup, and inrush | `validatePowerSequenceConstraint`; typed startup calculation tests; startup/transient MNA analyses |
| Pull-ups, translation, termination, and clock conditioning | catalog interface expansion and deterministic whole-interface calculation tests |
| Passive and buffered ADC drive | passive settling and catalog-backed op-amp buffer/headroom tests |
| Thermal and dynamic simulation | fixed/adjustable regulator, startup, transient, periodic thermal, and missing-path fail-closed tests |
| Physical lowering and writer agreement | shared emitted-route clearance repair, physical through-via modeling, route compaction, and explicit/composed routing tests |
| Neutral physical promotion | four ready corpus fixtures plus the installed-KiCad promotion test |
| Stable unsupported behavior | ten-case reordered negative corpus in `TestPowerInterfaceNegativeRequestCorpus` |

## Stable negative corpus

The negative corpus proves these families without selecting a fixture-specific
repair or component family:

- missing rail source and rail cycle;
- unproven regulator capacitor stability and unavailable transient
  capacitance;
- unproven rail sequence;
- voltage-domain mismatch and empty pull-up window;
- unproven termination, clock conditioning, and ADC settling.

## Physical correction evidence

Final emitted copper is checked at a shared transaction boundary for workflows
that explicitly require KiCad DRC. The correction is generic and deterministic:
it can relocate transition vias, insert pad-clear doglegs, split a segment onto
an alternate routable layer with physical through vias, expand transition
spans, and compact duplicate/zero-length geometry. Validation models every
ordinary multi-layer writer via as the physical F.Cu-to-B.Cu through via that
the PCB writer emits.

Structural/offline workflows retain ordinary router validation but do not claim
writer-level DRC against conservative template pad geometry. Strict workflows
record delegated conservative findings and rely on mandatory KiCad DRC as the
authoritative physical gate. No fixture coordinates, identities, allowlists,
schemas, or new per-circuit block families were added.

## Preserved regressions

The final installed-KiCad regression run also passed:

- `usb_c_led_indicator_protected`;
- `usb_c_i2c_sensor_3v3_protected`;
- `esp32_wroom_32e_minimal_pass`;
- `class_ab_headphone_protected`.

## Remaining boundary

Next work should add genuinely unsupported mixed-signal and power-control
primitive/model families, followed by dynamic electrothermal and control-loop
evidence where static bounds cannot prove startup, SOA, stability, or transient
protection. Broader clock/fanout, programming-load, isolation, converter,
high-energy protection, arbitrary-part qualification, RF/high-speed, dense-board
routing, and fabrication signoff remain outside this milestone.
