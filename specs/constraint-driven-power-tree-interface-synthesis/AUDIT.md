# Constraint-Driven Power-Tree and Interface Synthesis Audit

## Status

The bounded milestone is implemented and reproducible. Clock support remains
deliberately limited to frequency- and impedance-bounded source damping;
requests needing unmodeled amplitude, common-mode, edge, or jitter correction
remain unsupported rather than guessed.

## Implemented evidence

| Requirement | Evidence | Status |
|---|---|---|
| One selected source per generated rail | `power_tree.go`; missing and ambiguous producer tests | pass |
| Acyclic rail graph | deterministic sorted graph walk and shuffled-input cycle test | pass |
| Current/headroom/load aggregation | existing candidate-global domain current and headroom checks | pass |
| Regulator voltage, current, dropout, thermal selection | existing catalog ratings, simulation headroom, and thermal calculations | pass |
| Stability and transient output capacitance | catalog stability window plus `C >= I*dt/dV`, upward E12 selection, hashed calculation evidence | pass |
| Rail startup order and delay | selected producer startup evidence with stable `POWER_SEQUENCE_UNPROVEN` failure | pass |
| Monotonic startup and inrush | fail closed until a selected model supplies the missing evidence | pass/unsupported |
| Whole-bus I2C pull-up sizing | bounded rise-time/sink-current window and stable empty-window rejection | pass |
| Level translation | existing reviewed bidirectional open-drain translator selection | pass |
| Source termination | driver/target impedance calculation and deterministic E24 series resistance | pass |
| Clock source damping | frequency-gated source termination with a clock-specific stable rejection | bounded pass |
| Passive ADC drive | acquisition/accuracy/source-capacitance settling proof | pass |
| Buffered ADC drive | proven op-amp evidence filter plus bandwidth and RC settling proof | pass |
| Generic physical lowering | semantic instances, connections, power/reference roles, and ordinary transaction lowering | pass |
| Class-AB single-supply reference | negative rail is bound to the external reference; midpoint remains a bias node | pass |
| Acute same-net branch repair | bounded perpendicular two-corner doglegs with full route/clearance revalidation | pass |

## Stable rejection families

- `POWER_RAIL_SOURCE_MISSING`
- `POWER_RAIL_SOURCE_AMBIGUOUS`
- `POWER_RAIL_CYCLE`
- `POWER_CAPACITOR_STABILITY_UNPROVEN`
- `POWER_TRANSIENT_CAPACITANCE_UNAVAILABLE`
- `POWER_SEQUENCE_UNPROVEN`
- `INTERFACE_PULLUP_WINDOW_EMPTY`
- `INTERFACE_TERMINATION_UNPROVEN`
- `INTERFACE_CLOCK_CONDITIONING_UNPROVEN`
- `INTERFACE_ADC_DRIVE_UNPROVEN`

Existing global checks continue to provide stable voltage-window,
current-budget, headroom, phase-margin, thermal-margin, noise, startup-state,
fault-response, and reference-separation failures.

## Verification evidence

- `go test ./... -count=1 -timeout 1200s`: pass after all routing, reference,
  and synthesis corrections (the composition-lowering suite completed in
  521.972 seconds).
- `go test ./internal/architecturesearch ./internal/designworkflow`: pass.
- Installed-KiCad design examples: `usb_c_led_indicator_protected`,
  `usb_c_i2c_sensor_3v3_protected`, and `esp32_wroom_32e_minimal_pass`: pass.
- Existing ten-case adversarial corpus: pass after the generic route and
  Class-AB reference corrections (216.04 seconds).
- Held-out `buffered_adc_acquisition`: offline and installed-KiCad pass with
  clean ERC/strict DRC, complete routing/connectivity, writer correctness,
  zero round-trip differences, and replay.
- `mcu_managed_class_ab_output` initially reproduced a committed-baseline acute
  route-junction failure. After the generic dogleg correction and single-supply
  reference correction it passes complete routing/connectivity, writer
  correctness, clean ERC, strict DRC, zero round-trip differences, and replay.

## Reproducibility constraints

- No production branch uses a fixture name, component identity, coordinate,
  project allowlist, or block-family dispatch for these changes.
- Existing formula-library hashes remain unchanged; new calculations use the
  established rating-margin evidence envelope so unrelated tied candidate
  ordering is stable.
- Startup calculations are emitted only for requirements that request power
  sequence evidence, preserving unrelated architecture fingerprints.
- All graph walks, catalog filtering, preferred-value selection, and fallback
  route candidates have stable ordering and bounded search.
- Autonomous spacing correction retains the established three-attempt policy:
  retries deterministically progress from +1 mm to +2 mm rather than changing
  the public retry limit or existing policy hashes.

## Remaining broader-envelope work

The next capability expansion is typed clock amplitude, common-mode, edge, and
jitter evidence. It is outside this bounded source-damping milestone and must
be added through reviewed catalog/model facts and a new held-out case.
