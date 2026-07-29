# Behavior-Driven MCU Power-Integrity Completion Audit

Date: 2026-07-28

## Outcome

The verified ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6 synthesis path no
longer relies on fixed-value MCU decoupling companions. Architecture expansion
now derives deterministic local and bulk capacitor networks from behavior-level
startup, transient, ripple, noise, brownout, source-impedance, supply-range,
and temperature constraints.

This closes a bounded catalog-backed envelope. It does not claim unrestricted
power-distribution-network analysis, high-speed/RF layout, arbitrary MCU
support, or fabrication readiness beyond the checked evidence and gates.

## Requirement Audit

| Requirement | Result | Evidence |
|---|---|---|
| Per-rail MCU evidence | Pass | Every verified MCU rail group has startup current, transient step and durations, ripple/noise limits, brownout threshold, source impedance, and local/bulk placement bounds. Catalog validation fails closed when any record is incomplete. |
| Calculation-backed sizing | Pass | Source drop, brownout headroom, ESR droop, capacitive droop, effective capacitance, voltage derating, ripple current, and temperature bounds are recorded in finalized hashed calculations. |
| Local and bulk topology | Pass | One local capacitor is emitted for every normalized MCU supply domain and one bulk capacitor for every normalized rail group. ATmega proves two local domains with one shared bulk network. |
| Concrete capacitor qualification | Pass | Candidate selection requires verified concrete catalog identity, fabrication and pin-map evidence, tolerance, effective capacitance, ESR, ripple rating, voltage rating, a proven typed per-capacitor maximum voltage-use ratio, and temperature coverage. |
| Determinism | Pass | Search replay is byte-identical; capacitor catalog order and request-constraint order do not change expansion output. |
| Stable fail-closed behavior | Pass | Missing MCU/transient evidence, capacitor ESR, capacitor voltage-derating evidence, or supply-domain mapping, as well as exceeded brownout budgets and unqualified temperature ranges, return stable bounded diagnostic codes. |
| No fixed-recipe duplication | Pass | Typed calculated companions suppress the former static MCU power companions; expansion-count tests require exactly one local network per domain and one bulk network per group. |
| Physical evidence | Pass | The selected KEMET T-case footprint has checked-in pad/courtyard geometry matching the installed KiCad footprint and a matching transfer fallback. |
| Preservation | Pass | Prior MCU/ESP32, clock/programming, protected USB-C, sensor, Class-A, and Class-AB lanes remain green locally. |

## Held-Out Corpus

The frozen behavior-only corpus contains:

- an ESP32 wireless single-domain transient case;
- an STM32 SWD single-domain transient case;
- an ATmega ISP mixed-domain transient case.

No case names a selected MCU, capacitor, topology, pin, net, coordinate, layer,
or route. The manifest pins every requirement by SHA-256 and defines
adversarial mutations for unreviewed transient demand, brownout-budget
exhaustion, and unavailable temperature qualification. Focused adversarial
tests additionally remove ESR, typed voltage-derating, and supply-domain
evidence and assert their stable rejection codes.

## Local Verification

The following gates passed from the working tree:

- `go test -short ./... -count=1`;
- MCU evidence, calculation, fail-closed, order-invariance, and held-out corpus
  tests;
- complete offline lowering for all three new cases;
- installed-KiCad promotion for all three new cases, requiring clean ERC,
  strict DRC, connectivity, route completion, writer correctness, zero
  normalized round-trip differences, and deterministic replay;
- installed-KiCad neutral MCU promotion: 3/3;
- installed-KiCad clock/programming promotion: 2/2;
- installed-KiCad power/interface promotion: 4/4, including the Class-AB case
  and regulated MCU/sensor case;
- selected installed-KiCad design fixtures: Class-A preamplifier, Class-AB
  headphone driver, ESP32 minimal system, protected USB-C LED, and protected
  USB-C I2C sensor: 5/5.

The unrestricted non-short aggregate was also exercised. It initially exposed
the expected catalog-fingerprint golden updates, provider capability-size
pressure, and missing model-provenance records for the newly selected
capacitor. Those issues were corrected and their focused regressions pass. The
aggregate long corpus exceeded Go's default ten-minute package timeout while
continuing unrelated promotion cases; the authoritative complete local short
suite and every affected long/installed-KiCad lane listed above pass.

## Source And Model Evidence

The added polymer tantalum record is KEMET
`T520T476M006ATE040`, using the official manufacturer specification at
`https://search.kemet.com/download/specsheet/T520T476M006ATE040` and the stock
KiCad `Capacitor_Tantalum_SMD:CP_EIA-3528-12_Kemet-T` footprint. Reviewed
provenance covers the registered static and transient capacitor model
definitions over the part's declared temperature range. Every
fabrication-proof capacitor now also carries a validated voltage-derating
review and maximum operating-voltage ratio; MCU synthesis consumes that typed
record rather than an implicit global factor.

## Remaining Boundary

Power-integrity synthesis remains intentionally fail-closed outside reviewed
MCU rail evidence and qualified capacitor records. Inductance-aware PDN
impedance, package/plane resonance, regulator control-loop interaction,
high-frequency electromagnetic behavior, arbitrary multi-rail sequencing, and
unreviewed components require future evidence and analysis capabilities.
