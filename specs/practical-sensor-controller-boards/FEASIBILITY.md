# Pre-evaluation feasibility review

Date: 2026-09-10. Reviewer: the implementing Codex agent (self-review, not an
independent engineer). Status at sealing: electrical consistency reviewed;
evaluator and environment bound by the companion freeze. No corpus case had
been executed when these bytes were sealed.

## Meaning of this review

The positive briefs have mutually achievable voltage, load, bandwidth,
temperature and enclosure requirements. That is not a prediction that merged
main can synthesize them. Available component envelopes and conservative
arithmetic establish plausible implementations; the evaluated generator must
still independently select and qualify its actual parts, analyze all requested
conditions and complete the physical project. Nothing below is passed to the
live provider as a circuit answer. It contains no expected netlist or layout.

The corpus uses modest peripheral rates, low load energy and a generous but
finite 8,000 mm² enclosure. A 32-pin controller, a few small ICs and peripheral
connectors do not create an intrinsic packing contradiction at that area.
This is a geometric feasibility judgment, not proof of generated placement,
connector access, routing or manufacturability. All remain acceptance gates.

## Per-case consistency findings

| Case | Independent consistency check | Qualification pressure, not a waived gate |
|---|---|---|
| P01 | Even allowing 0.3 V input-protection loss, minimum input remains 4.45 V, above a 3.45 V output. At 50 mA internal demand, a linear path dissipates approximately `(5.25-3.3)*0.05 = 0.0975 W`; a 100 mA source leaves a plausible controller/sensor/interface budget. | Actual reverse-input survival, regulator dropout/thermal derating, simultaneous temperature AND humidity support and all bus loads must be proven. |
| P02 | Gain 2 with ±2% maps 0.05–1 V to 0.098–2.04 V, inside the 0.08–2.1 V window and below the 3.15 V minimum supply. The passband and stopband do not conflict: an illustrative 1 kHz single-pole response loses about 0.5% at 100 Hz and 20 dB at 10 kHz. This is an existence check, not a mandated topology. | Input bias/impedance, near-ground input/output operation, gain tolerance, filter corners and actual ADC settling cannot be inferred from nominal gain alone. |
| P03 | Two I2C instances exist in the reviewed STM32 catalog mapping on disjoint candidate pin bundles, leaving other pins for ready and SWD. The 5 V and 3.3 V domains are compatible with reviewed level-translation voltage ranges. Bus RC bounds have nonempty sink-current/rise-time intervals at 100 pF/100 kHz and 50 pF/400 kHz. | The solver must actually allocate two independent buses, not alias them; actual pull-up/translator loading and shared-current/thermal budgets must pass. The ESP32 catalog's 0.5 A minimum supply envelope is NOT compatible with this 100 mA source and cannot be silently used. |
| P04 | The reviewed 5 V controller family exposes UART and SPI; the 20 mA auxiliary load is materially below the 100 mA total limit. At worst input, 20 mA through a 5.25-to-3.3 V path dissipates about 39 mW before other loads. At 1 MHz, the SPI period is 1 us, not a high-speed multi-GHz routing problem. | UART clock accuracy, SPI direction roles, 3.3 V protection, reset-time chip-select bias and programmer loading need explicit evidence. No powered-off assumption may be added. |
| P05 | Output dissipation at the limiting on-state point is only `0.02*0.25 = 0.005 W`. The non-inductive 20 mA load leaves 80 mA of input budget for the controller, UART and power path. Feedback maximum 2.8 V remains below minimum rail 3.15 V. | Reset/off leakage, control threshold, edge behavior, thermal/current margins and ADC settling remain real requirements, not firmware promises. |
| P06 | A 16 MHz controller at 4.75–5.25 V is within the reviewed 5 V controller envelope. The clock requirement is achievable with available tight-tolerance crystal/oscillator options; the source allows tighter than standard frequency grades. Event, programming and indicator roles can occupy distinct resources. | A generic crystal label is insufficient. Initial tolerance plus stability plus loading must total <=100 ppm. A standard ±50 ppm initial / ±50 ppm stability grade leaves no loading margin and must not be accepted on those two numbers alone. |
| P07 | Independent-supply translation with power-off isolation is physically available: the TI TXS0102 specifies isolation when either supply is grounded and ±2 uA maximum partial-power-down port current under its stated conditions. Two lines therefore need not intrinsically exceed the 10 uA backfeed limit. Local controller demand is separate from host-interface power. | Whole-board supply backfeed includes every path, not just one IC's port leakage. Bias/OE connections and both power-loss directions must be analyzed. Catalog partial-power evidence must cover the exact selected topology. |
| P08 | The burden limit allows up to `0.25/0.02 = 12.5 ohms` effective series sense resistance. Required transimpedance for a 1.5 V span over 16 mA is 93.75 ohms; these are compatible with low-side gain rather than a 93.75-ohm direct burden. A 100-ohm effective transfer gives 0.4–2 V and 2.2 V at 22 mA, inside a 3.15 V rail. A 30 mA electronics budget dissipates roughly `(12-3.3)*0.03 = 0.261 W` in a linear supply, feasible with a suitably qualified thermal path; a tiny unqualified regulator is not implied. | Common-mode range is essential: INA168's 2.7 V minimum common mode cannot justify a near-ground input. A low-side-capable precision family such as INA240 demonstrates feasibility, but its actual shunt, gain/offset/linearity corners, ADC interface and regulator thermal path must all be qualified by the generator. |

## Source checks and boundaries

Reviewed repository sources include `data/components/verified_active.json`,
`active_blocks.json`, `open_set_power.json`, `multifunction_power.json` and the
MCU, power-integrity, clock/programming and protocol-aware synthesis audits.
These show available envelope evidence, not a substitute for a selected-part
review. In particular, a broad MCU absolute supply-current rating must not be
confused with a proven selected operating-current budget.

Manufacturer cross-checks on 2026-09-10:

- [TI INA240 datasheet](https://www.ti.com/lit/ds/symlink/ina240.pdf): low-side
  common-mode capability, 2.7–5.5 V supply, available 100 V/V gain and precision
  error terms establish a plausible current-loop measurement envelope. Its
  total error depends on the actual operating conditions; no universal accuracy
  is inferred from a typical example.
- [TI TXS0102 datasheet](https://www.ti.com/lit/ds/symlink/txs0102.pdf), sections
  5.5 and 7: powered-off port-current and isolation evidence is conditional.
  Whole-board <=10 uA backfeed still needs an explicit accounting of all paths.
- [Abracon ABM3 specification](https://abracon.com/Resonators/ABM3.pdf): standard
  temperature coverage includes -10 to +60°C; tighter frequency options exist.
  The standard initial and temperature terms alone are not enough for the
  complete P06 clock budget. Exact ordering-code qualification is required.
- [Microchip ATmega family datasheet](https://ww1.microchip.com/downloads/en/DeviceDoc/ATmega48A-PA-88A-PA-168A-PA-328-P-DS-DS40002061A.pdf),
  section 24.6.1: low source impedance is important for ADC acquisition. The
  source describes approximately 10 kohms or less; the briefs' 100-ohm external
  sources are not an inherent acquisition obstacle, but intervening circuitry
  and the selected MCU's actual acquisition schedule must still be checked.

No feasibility check weakens a requirement or qualifies an undocumented model.
The following are deliberate synthesis/evidence pressures: combined sensing
rather than either/or sensing, independent bus allocation, whole-board power
budgets, reset-safe outputs, current-loop overrange, ADC acquisition evidence,
clock uncertainty and partial-power behavior. Missing proof is a failure.

## Negative and ambiguous cases

N01 violates the safe exposed-interface boundary for non-isolated mains. N02 is
algebraically inconsistent (`4.75 > 3.45` on the same rail at the same instant).
N03 combines high energy with explicitly omitted protections. N04 requests
unsupported RF/model/regulatory assurance. Their classifications are fixed
before testing and are not reinterpreted as inconvenient positive cases.

C01 lacks the supply envelope; C02 lacks load voltage/current/energy/rate.
Both have fixed answer text prepared before evaluation. Supplying these answers
is recorded assistance. Additional unstated electrical facts may not be
invented to force a ready result. Paraphrases W01/W02 preserve their parent
electrical requirements and do not increase the positive denominator.
