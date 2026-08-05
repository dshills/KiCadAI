# Capability Record

This page preserves the detailed implementation and evidence inventory that
previously occupied the opening of the project README. For the current
supported, experimental, and unsupported boundaries, start with
[Project Status](project-status.md) and [AI Readiness](ai-readiness.md).

## Current State

- Direct project, schematic, and PCB writers are functional and extensively
  tested.
- Structured intent can generate supported designs through planning, component
  selection, schematic/PCB realization, placement, routing, and validation.
- Every normalized creation request is deterministically classified as
  `supported`, `experimental`, or `unsupported` before project mutation.
  Experimental generation requires explicit `--experimental` opt-in and is
  barred from fabrication-ready and promotion-pass claims; unsupported
  requests fail closed with evidence-linked diagnostics. Assessments are
  re-evaluated through later workflow stages and embedded in workflow,
  promotion, and manifest artifacts.
- Unsupported assessments can now be converted into deterministic,
  content-addressed expansion plans. Source-backed component, model, physical,
  verification, and declarative architecture additions remain quarantined as
  `experimental`; automatic representative/adversarial cases and the complete
  workflow/KiCad gate contract produce a review-ready bundle only after every
  result passes. An exact-hash approval and explicit mutation command are
  required to update the supported-capability registry, which behavior-only
  requirement generation can load for fresh requests.
- Ordinary behavior-first requests can enter through the uncertainty-aware
  `intent compile` trust boundary. It accounts for every source statement,
  asks the minimum blocking clarification, records stable capability gaps, and
  releases a strict v3 requirement only after deterministic architecture
  search and trusted all-corner closed-loop evidence pass.
- Exact `ESP32-WROOM-32E-N4` minimal-system generation is supported through the
  built-in `esp32_wroom_32e_minimal` block, with power conditioning, EN/BOOT
  straps and buttons, UART/I2C/SPI/GPIO headers, and an antenna copper keepout.
- Behavior-driven MCU subsystem synthesis can select verified ATmega328P-A,
  ESP32-WROOM-32E, or STM32G031K8T6 targets from catalog evidence, assign real
  alternate-function pins deterministically, and add catalog-declared power,
  reset, programming, boot, and optional clock support. Three neutral requests
  reach the full installed-KiCad promotion lane without naming their target.
- Behavior-driven clock/programming synthesis now calculates verified external
  crystal load networks, bounds crystal drive/stability/startup, and records
  programming frequency, loading, voltage, reset/boot entry, and shared-pin
  isolation evidence. External-crystal ISP and integrated-clock UART cases pass
  the local installed-KiCad lane; unsupported unpowered SWD fails closed.
- Behavior-driven MCU power-integrity synthesis now replaces fixed decoupling
  recipes for verified ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6
  selections. It derives source drop, brownout headroom, ESR and capacitive
  droop from startup/transient behavior, qualifies concrete capacitors for
  tolerance, voltage derating, ripple, and temperature, emits one local
  network per supply domain plus shared bulk support per rail group, and
  records finalized calculation evidence. Three target-free held-out cases
  pass the complete local installed-KiCad lane; evidence, budget, and
  temperature gaps fail closed.
- The provider-backed natural-language lane retains two promoted bounded
  profiles and adds a catalog-resolved circuit graph contract with either an
  explicit graph or strict function-level intent. Function intent names
  functions, interfaces, operating domains, and bounded constraints; KiCadAI
  deterministically supplies verified parts, support networks, unused-pin
  policy, physical defaults, and routes.
- Generated `generic-circuit-v1` designs now have a deterministic correction
  loop with at most three placement/routing attempts, protected-invariant
  checks, stable retry evidence, and fail-closed unsupported actions.
- The ESP32-WROOM-32E minimal-system fixture and the bounded USB-C BMP280 and
  protected LED profiles are KiCad-backed `pass`. Six generic fixtures use the
  shared catalog-resolved graph contract:
  RC filter, protected USB-C LED, protected USB-C BMP280, single-stage LMV321,
  dual LMV321, and multi-unit LM358. Their recorded/live evidence varies by
  fixture, while each checked-in optional KiCad promotion lane is `pass`. None
  requires a topology-specific provider schema. The LM358 fixture additionally
  proves one-package, multi-unit schematic lowering with one footprint and BOM
  identity.
- A frozen held-out function-level corpus adds eight independently authored
  analog, power/protection, transistor, sensor/interface, ATmega328P, and
  ESP32/SHT31 circuits. All eight pass deterministic lowering, catalog and
  KiCad library resolution, support expansion, placement, complete routing,
  clean ERC and strict DRC, connectivity, writer correctness, zero round-trip
  differences, and byte-identical replay. The checked-in
  [capability report](../specs/function-level-circuit-synthesis/CAPABILITY_REPORT.json)
  records the authoritative hashes and per-circuit gates.
- A second frozen adversarial corpus composes 10 unfamiliar multi-function
  circuits from 35 objectives and 3 abstract participants. It exercises 18
  registered capabilities and 23 whole-circuit voltage, current, startup,
  loading, stability, tolerance, isolation, thermal, response-time, noise, and
  board constraints. All 10 pass deterministic replay, lowering, writer,
  routing/connectivity, zero-diff round trip, and installed-KiCad ERC and strict
  DRC. See the [composition audit](../specs/adversarial-multi-function-composition/AUDIT.md).
- A frozen 24-prompt behavioral-intent corpus adds 12 paraphrase groups across
  amplifiers, filters, power, protection, sensors, and MCU interfaces. Twelve
  prompts compile to six unique supported contracts, four require targeted
  clarification, and eight produce stable unsupported outcomes. Every supported
  contract passes deterministic replay and the installed-KiCad promotion lane.
  See the [behavioral compiler audit](../specs/uncertainty-aware-behavioral-intent-compilation/AUDIT.md).
- Constraint-driven power-tree and interface synthesis now has a four-design
  neutral promotion corpus: regulated MCU/sensor, protected power-MOSFET load,
  buffered ADC acquisition, and Class-AB power/interface cases. All four pass
  deterministic replay, complete routing/connectivity, writer correctness,
  clean installed-KiCad ERC and strict DRC, and zero-difference round trip.
  Ten reordered negative cases prove stable fail-closed power and interface
  diagnostics. See the [completion audit](../specs/constraint-driven-power-tree-interface-synthesis/AUDIT.md).
- A versioned twelve-case held-out capability benchmark spans analog, power,
  digital, MCU, sensor, and mixed-signal requirements without prescribing
  topology, components, nets, pins, or coordinates. The frozen installed-KiCad
  baseline passed 5/12 cases. Constant-current regulation and precision
  rectification advanced the original expansion report to 11/12, and generic
  fixed-frequency and resistor-programmed clock generation subsequently closed
  the benchmark at 12/12. Every row now passes simulation,
  routing/connectivity, writer, ERC, strict DRC, zero-diff round trip, and
  deterministic replay. See the
  [specification](../specs/held-out-capability-expansion/SPEC.md),
  [baseline](../specs/held-out-capability-expansion/BASELINE_REPORT.json), and
  [clock closeout audit](../specs/standalone-clock-generation/AUDIT.md).
- A frozen six-system V4 corpus now measures hierarchical synthesis across
  protected Class-AB, precision analog, regulated MCU/sensor, isolated
  mixed-voltage, high-current switched-load, and split-supply monitor systems.
  Production code derives subsystem/block hierarchy, typed boundary contracts,
  shared-resource plans, global backtracking evidence, physical partitions,
  and requirement-to-KiCad traceability. All six pass the complete offline and
  installed-KiCad two-run promotion gates. This is a bounded catalog-backed
  envelope, not unrestricted arbitrary-circuit generation.
- A frozen six-circuit V5 corpus now adds dynamic electrothermal and
  control-loop synthesis for reactive-load feedback amplification, analog
  servo regulation, switching conversion, protected inductive switching,
  Class-AB load/fault behavior, and sequenced dual rails. Reviewed catalog
  models drive deterministic return-ratio, corner, event, electrothermal,
  transient-SOA, protection, candidate-selection, and bounded-repair evidence.
  Every case passes the complete local KiCad promotion lane, and two
  independent clean roots produce the same 418-file content-addressed bundle.
  See the
  [completion audit](../specs/dynamic-electrothermal-control-loop-synthesis/AUDIT.md).
- A separately frozen nonlinear/switching corpus now turns five behavior-only
  requirements into a precision rectifier, diode limiter, relaxation
  oscillator, PWM MOSFET power stage, and low-power buck without accepting
  named parts, topologies, equations, models, nets, or geometry. Generic
  relationship operators, discontinuity-aware transient analysis, periodic
  waveform measurements, propagation-delay gating, electrothermal coupling,
  and transient-SOA checks drive selection. All five pass deterministic replay,
  exhaustive corners, readable lowering, complete routing/connectivity, writer
  checks, zero-difference round trip, and local installed-KiCad ERC/strict DRC.
  Two unsafe envelopes and one unsupported ultra-fast envelope fail closed.
  See the
  [completion audit](../specs/nonlinear-switching-architecture-synthesis/AUDIT.md).
- V6 adds explicit active-high/active-low control functions, asserted and safe
  startup state, directed transition timing, and stable-state dependencies to
  the V3 behavioral-composition envelope. Planning resolves every dependency
  to a trusted target, transient measurement rejects wrong-direction glitches,
  and generic polarity/bias/threshold/timing operators remain bounded. The two
  motivating protection cases now fail closed with a precise request for a
  separate startup-enable or sequencing dependency instead of weakening their
  low-output startup claim. See the
  [contract](../specs/control-state-sequencing/SPEC.md).
- A separate identity-neutral open-world evaluation freezes discovery and
  held-out behavior-only requirements across six domains, records stable
  failure clusters, and uses those clusters to drive reusable capability
  expansion. Held-out readiness improves from 1/12 to 6/12 without changing
  corpus membership: five newly ready cases cover clock fanout, translated MCU
  debug, push-pull sensor translation, functional isolation, and protected
  isolated power. All five pass the complete local installed-KiCad lane and
  deterministic replay. See the
  [open-world audit](../specs/open-world-capability-evaluation/AUDIT.md).
- A frozen four-case protocol-aware bus corpus now covers I2C, SMBus, SPI, and
  UART translation, including whole-bus loading, solved pull-ups, six
  independently isolated branches, mixed push-pull directions, reversed
  voltage domains, inactive startup, partial-power, hot-plug, and contention
  requirements. All four pass the complete local installed-KiCad two-run lane;
  eleven unsafe or incomplete variants fail closed. See the
  [specification](../specs/protocol-aware-bus-synthesis/SPEC.md) and
  [promotion matrix](../specs/protocol-aware-bus-synthesis/PROMOTION_MATRIX.json).
- Evidence-backed component onboarding can now turn behavior requirements and
  immutable manufacturer documents into quarantined component/model
  candidates without putting the unfamiliar part identity in production code.
  Exact excerpts, ratings, temperature, derating, package, symbol-pin,
  footprint-pad, model, license, and provenance claims are validated
  deterministically. Promotion requires exact-hash review plus two identical
  simulation, connectivity, routing, writer, round-trip, ERC, and strict-DRC
  runs. A frozen seven-family corpus covers an op-amp, transistor, regulator,
  converter, sensor, logic device, and interface part; all seven pass in two
  clean source roots. Run `make component-onboarding-promotion-bundle` with the
  installed KiCad environment to reproduce it. See the
  [specification](../specs/evidence-backed-component-onboarding/SPEC.md).
- A separate open-topology lane now constructs primitive-component graphs
  directly from strict behavior-only requirements instead of selecting a
  pre-authored functional block. All eight frozen analog, power, and
  mixed-signal requirements pass trusted simulation. Two independently frozen
  neutral multi-branch designs also pass deterministic replay and the complete
  installed-KiCad two-root promotion lane, including a selected graph-changing
  repair. The public
  `kicadai open-topology create` command writes full search/simulation and
  physical-promotion evidence while returning a compact JSON summary. See the
  [multi-branch completion audit](../specs/generic-multi-branch-analog-topology-synthesis/AUDIT.md).
- The simulation-grounded architecture milestone extends that lane from
  first-pass topology discovery to deterministic comparison of multiple
  materially different candidates. Frozen Class A, complementary Class AB,
  and 60 Hz notch requirements now receive equation-derived values, complete
  trusted simulation, ranked selection with alternatives, and two identical
  installed-KiCad promotions. A plausible but unsafe Class A value alternative
  is explicitly rejected by standing-current, thermal, or SOA evidence. See
  the [architecture synthesis audit](../specs/simulation-grounded-architecture-synthesis/AUDIT.md).
- A second, independently authored architecture-generalization corpus freezes
  six additional analog and power behaviors plus four adversarial safety
  envelopes before implementation changes. Five of six designs now pass
  multi-topology search, trusted simulation, readable lowering, two identical
  installed-KiCad promotions, and every physical gate; all four unsafe cases
  fail closed. At that frozen milestone the protected programmable current
  output remained a stable, evidence-backed non-pass. See the
  [completion audit](../specs/architecture-generalization-corpus/COMPLETION_AUDIT.md)
  and [promotion matrix](../specs/architecture-generalization-corpus/PROMOTION_MATRIX.md).
- The former protected-current non-pass is now closed by a separate
  checksum-frozen generic synthesis milestone. Behavior-only low-side sink,
  protected programmable output, and startup-safe high-side source cases
  derive current direction, sense relationships, independent startup/fault
  control, compliance, values, thermal/SOA evidence, and readable physical
  realization without a named driver block. All three pass deterministic
  replay and two isolated local KiCad 10.0.3 promotions with clean ERC, strict
  DRC, complete connectivity/routing, writer correctness, and zero round-trip
  differences. See the
  [completion audit](../specs/generic-protected-current-output-synthesis/AUDIT.md).
- Closed-loop candidate evaluation now uses a deterministic, budgeted
  cheap-to-expensive scheduler: structural checks precede DC, AC, transient,
  thermal/SOA, and exhaustive promotion verification. Exact trusted plans are
  reused through a bounded SHA-256 cache, conservative Pareto dominance retains
  auditable rejection evidence, and worker count cannot change persisted
  selections or hashes. See the
  [scheduler specification](../specs/deterministic-synthesis-evaluation-scheduler/SPEC.md)
  and [completion audit](../specs/deterministic-synthesis-evaluation-scheduler/AUDIT.md).
- Electrical and physical correction now share the versioned
  `kicadai.diagnosis-driven-repair.v1` trace. It binds normalized diagnoses to
  deterministic proposals, stage re-entry, budgets, effects, outcomes, and
  before/after hashes. A six-component, twelve-pad, two-route-tree benchmark
  recovers reproducibly from a real routing block and passes writer,
  connectivity, round-trip, ERC, and strict-DRC checks. Its historical
  protected-current trace remains useful reproducible failure evidence; the
  later generic protected-current milestone closes that synthesis gap. See the
  [repair results](../specs/diagnosis-driven-repair/RESULTS.md).
- End-to-end cross-stage repair now coordinates trusted simulation,
  schematic/ERC, placement, routing, connectivity, DRC, writer, and round-trip
  correction through `kicadai.cross-stage-autonomous-repair.v1`. A frozen
  nine-stage corpus proves earliest-stage re-entry, smallest-safe-candidate
  selection, exact checkpoint rollback, unrelated-scope preservation,
  electrical/thermal/SOA/physical guardrails, bounded execution, independent
  confirmation, and byte-identical replay. Real adapters exercise causal
  electrical repair, transaction-backed physical repair, and authoritative
  generated-file regeneration. See the
  [completion audit](../specs/cross-stage-autonomous-repair/AUDIT.md).
- Topology-aware schematic lowering now favors visible local conductors,
  conventional signal flow, compact route trees, feedback visibility,
  role-derived orientation, and the smallest fitting standard sheet. Five
  checked-in [educational examples](../examples/educational/README.md) cover a DC
  voltage source, BJT current source, differential amplifier, RC low-pass
  filter, and voltage divider.
- Unrestricted electronics generation is not guaranteed. Open-topology and
  generic graph paths fail closed on unknown behavior, parts, pins, models,
  ratings, placement, routing, or exhausted search capability.
- MCU synthesis is limited to verified catalog records and modeled electrical
  constraints. It does not infer arbitrary MCU pin data from KiCad symbols, and
  ESP32 variants, flash choices, RF optimization, and unverified external-bus
  loading remain fail-closed boundaries.
- Generated `pass` evidence is not automatically a fabrication-release claim.
- A clean checkout can discover the locked KiCad 10.0.3 installation or
  bootstrap its checksum-pinned distribution, run the release promotion matrix
  twice, compare normalized projects, and emit an independently verifiable,
  content-addressed evidence bundle with one command.

See [Project Status](project-status.md) for capability boundaries and
[Roadmap](../specs/ROADMAP.md) for remaining work.
