# Constraint-Driven Power-Tree and Interface Synthesis Specification

## 1. Purpose

KiCadAI shall turn topology-neutral circuit intent and bounded load contracts
into a deterministic, catalog-backed power tree and the interface networks
needed between selected functional fragments. The result must be an electrically
justified KiCad project, not merely a connected schematic.

The synthesis path must reuse the open-set requirement, typed port contracts,
catalog selection, circuit graph, simulation, placement, routing, writer, and
KiCad validation layers. It must not dispatch on project names, corpus names,
fixture identities, or selected manufacturer families.

## 2. Required outcomes

For supported evidence, synthesis shall:

- select regulators for every generated supply domain;
- aggregate steady-state and bounded transient rail demand;
- enforce requested current headroom and regulator derating;
- derive input bulk, output stability, and local decoupling networks;
- prove or reject rail startup ordering, monotonicity, and fail-safe states;
- insert level translation when two interface sides have incompatible voltage
  windows and a reviewed translator supports the protocol;
- derive open-drain pull-ups from voltage, speed, capacitance, and sink-current
  limits;
- derive source termination from driver impedance, interconnect target
  impedance, receiver capacitance, and maximum edge-time constraints;
- derive clock conditioning from source/receiver electrical evidence;
- derive ADC and op-amp drive isolation/settling networks from source
  impedance, acquisition time, load capacitance, bandwidth, and stability
  evidence;
- validate startup, transient, thermal, voltage-domain, current-budget, and
  modeled signal-integrity constraints before physical promotion;
- return stable clarification or capability-gap evidence when a required fact
  is absent or no catalog-backed implementation is feasible.

## 3. Reused foundations

This milestone extends, rather than replaces:

- `architecturesearch.Requirement` v3 domains, signals, participants,
  objectives, operating cases, and behavioral requirements;
- typed `PortContract` voltage, current, impedance, frequency, protocol,
  default-state, and evidence fields;
- catalog-backed `voltage_regulation`, `logic_level_translation`,
  `transient_protection`, `signal_amplification`, `class_a_amplification`,
  `class_ab_bias_control`, and `class_ab_output_stage` providers;
- regulator stability, capacitor effective-capacitance/ESR/ripple, op-amp,
  power-semiconductor SOA, and generic thermal evidence;
- global voltage-window, aggregate current, current-headroom, startup-state,
  phase-margin, thermal-margin, and noise checks;
- DC, AC, transient, startup, periodic steady-state, and thermal simulation;
- generic fragment lowering and the existing KiCad-backed promotion harness.

## 4. Power-tree contract

### 4.1 Rail identity

A supply domain is external only when `source` is `external`. Every other
supply domain must name a power signal produced by exactly one selected
objective. The producer must expose a source power contract in that domain.
Consumers may be participants or objectives and must expose sink power
contracts with bounded current demand.

The planner shall build a directed acyclic graph whose nodes are supply domains
and whose edges are selected power-conversion fragments. Reference domains are
tracked separately and may only be joined by a selected isolation or explicit
reference-joining fragment.

### 4.2 Load aggregation

For each rail the planner shall deterministically aggregate:

- maximum steady-state sink current;
- impedance-derived demand where an explicit current is unavailable;
- selected-component quiescent current;
- declared startup or transient load current and duration;
- margin required by `rail_current_headroom` or
  `supply_current_headroom`;
- downstream converter input demand, including catalog-backed efficiency or a
  conservative declared bound.

Unknown safety-relevant demand fails closed. A nominal domain current ceiling
is not treated as the regulator's capacity; it is a requirement that selected
sources must meet.

### 4.3 Regulator selection

Candidate regulators are filtered using the worst requested input/output
voltage window, load current, dropout/headroom, package thermal path, output
accuracy, enable limits, and required analyses. Feasible candidates are ranked
by generic evidence, worst margin, quiescent power, area, and stable catalog
identity.

The planner may use an isolated converter, fixed regulator, adjustable
regulator, positive/negative regulator, or an explicitly supported composed
chain. It may not infer an unreviewed converter topology from a KiCad symbol.

### 4.4 Decoupling and bulk capacitance

Regulator input/output capacitors must be selected from catalog evidence, not
hard-coded package identities. Output capacitance and ESR must satisfy the
selected regulator's stability window after voltage, tolerance, temperature,
and DC-bias derating.

Bulk capacitance shall be derived from the bounded transient relation
`C >= I * dt / dV`, then rounded upward through the deterministic preferred
value policy. Ripple-current and voltage-rating evidence must be checked when
the request makes them relevant.

Local decoupling remains component-companion driven, but rail synthesis shall
account for its effective capacitance and avoid duplicate equivalent networks.

### 4.5 Sequencing and startup

Supported sequencing constraints are topology-neutral system constraints:

- `rail_sequence_before`: a producer rail must become valid before a consumer
  rail;
- `rail_sequence_delay`: minimum or maximum delay between named rail signals;
- `startup_monotonic`: a named rail must rise monotonically;
- `startup_inrush_current`: a named rail's bounded source current;
- existing startup load/output/bus state constraints.

The selected fragments must expose catalog-backed startup calculations or
simulation evidence. Missing timing evidence yields a stable unsupported result
instead of a guessed RC delay.

## 5. Interface synthesis contract

### 5.1 Interface discovery

Interface synthesis operates on typed signal endpoints and participant ports.
It groups endpoints by signal, protocol, direction, voltage domain, frequency,
impedance, and required traits. It never sees project or fixture identity.

Direct connection is allowed only when all endpoint voltage windows,
directions, modes, current limits, bandwidth, and reference-domain constraints
overlap. Otherwise an explicit conditioning objective or a deterministically
derived child obligation must be satisfied.

### 5.2 Level translation and pull-ups

Open-drain and push-pull translation are separate policies. Translator
selection must prove side-A/side-B voltage ranges, directionality, channel
count, protocol frequency, reference compatibility, startup behavior, and
required bias/decoupling companions.

Open-drain pull-ups are solved across the whole bus. The allowed resistance
interval is bounded by rise time, bus capacitance, leakage/noise margin, and the
weakest sink-current limit. Preferred values are chosen deterministically.

### 5.3 Termination and clock conditioning

Source termination is permitted when driver output impedance and target
interconnect impedance are known. The series value is the non-negative
difference, rounded through the preferred-value policy and checked against
edge-rate and receiver-threshold constraints.

Parallel or Thevenin termination requires explicit DC loading and source-power
evidence. It is never inferred solely from frequency.

Clock conditioning may select source series damping, AC coupling, bias,
translation, or fanout only when the source and receiver contracts plus catalog
evidence prove amplitude, common mode, edge rate, frequency, jitter class, and
startup behavior. Unsupported jitter or oscillator-startup claims fail closed.

### 5.4 ADC and op-amp drive networks

An ADC drive request must identify source signal range, required settling
accuracy, acquisition time or sample rate, ADC input capacitance, and maximum
source impedance. The solver may choose:

- a passive RC network when settling and source loading pass;
- an op-amp buffer plus isolation resistor/charge reservoir when catalog
  bandwidth, slew rate, output current, input common mode, output swing,
  capacitive-load stability, noise, and thermal evidence pass;
- an unsupported result when the necessary ADC or amplifier evidence is
  absent.

Op-amp output isolation is derived from the requested capacitive load and
reviewed stability policy. A generic fixed resistor is not proof of stability.

## 6. Evidence model

Catalog records may carry normalized power-tree and interface evidence. The
schema must remain generic and data-only. At minimum it must support:

- regulator startup time, soft-start/inrush policy, quiescent current,
  efficiency bound, dropout, and load-transient evidence;
- capacitor effective capacitance, ESR, ripple current, voltage rating, and
  tolerance at stated conditions;
- interface driver impedance/current, input capacitance/leakage, voltage
  thresholds, edge time, maximum frequency, and supported signaling modes;
- translator channel/mode/frequency/startup information;
- ADC acquisition capacitance/time and source-impedance limits;
- op-amp capacitive-load/isolation stability evidence;
- clock amplitude/common-mode/edge/jitter/startup evidence.

Evidence normalization and validation must be deterministic under reordered
catalog input. Verified records used for fabrication must fail catalog
validation if a claimed supported analysis lacks its required fields.

## 7. Determinism and failure behavior

All candidate sets, rails, endpoints, calculations, diagnostics, and selected
parts are sorted by normalized semantic identity before ranking or hashing.
Equivalent input ordering must produce byte-identical architecture results and
normalized KiCad outputs.

Required stable rejection families include:

- `POWER_RAIL_SOURCE_MISSING`;
- `POWER_RAIL_CYCLE`;
- `POWER_CURRENT_BUDGET_UNKNOWN` and `POWER_CURRENT_BUDGET_EXCEEDED`;
- `POWER_DROPOUT_MARGIN_UNAVAILABLE`;
- `POWER_CAPACITOR_STABILITY_UNPROVEN`;
- `POWER_TRANSIENT_CAPACITANCE_UNAVAILABLE`;
- `POWER_SEQUENCE_UNPROVEN`;
- `INTERFACE_VOLTAGE_DOMAIN_MISMATCH`;
- `INTERFACE_TRANSLATION_UNAVAILABLE`;
- `INTERFACE_PULLUP_WINDOW_EMPTY`;
- `INTERFACE_TERMINATION_UNPROVEN`;
- `INTERFACE_ADC_DRIVE_UNPROVEN`;
- `INTERFACE_CLOCK_CONDITIONING_UNPROVEN`.

Provider errors must survive architecture search and map to stable behavioral
capability gaps. A user-owned choice that changes isolation, sequencing,
external clocking, or acceptable signal-conditioning tradeoffs produces a
stable clarification.

## 8. Physical realization

Selected support networks lower through semantic functions and graph
connections. Placement hints may express proximity, ordering, edge-facing,
current class, return-path, thermal, and matched-group constraints. They may
not contain fixture coordinates.

Routing shall use generic net class, current, impedance, length, and topology
constraints. No selected regulator, MCU, MOSFET, amplifier, corpus case, or
project name may dispatch to routing coordinates or a hard-coded topology.

## 9. Neutral promotion corpus

The frozen corpus shall include at least:

1. a generated 3.3 V MCU/sensor subsystem supplied from a wider external input,
   including regulator sizing, I2C pull-ups or translation, startup, and local
   decoupling;
2. a protected power-MOSFET load path with logic-domain drive conditioning,
   current and thermal bounds, transient clamp, and fail-safe startup;
3. an ADC/op-amp acquisition path with source impedance, settling, bandwidth,
   noise, and capacitive-load conditioning;
4. a Class A or Class AB amplifier-related power case with bulk capacitance,
   rail current, startup/mute, power-device SOA, and thermal constraints.

Corpus requirements describe behavior, domains, loads, signals, operating
cases, and acceptance only. They must not name catalog IDs, manufacturers,
packages, block families, routing coordinates, or expected selected parts.

At least one negative request for each major solver shall prove deterministic
unsupported output.

## 10. Acceptance

The milestone is complete only when:

- power-tree and interface evidence schemas validate and normalize
  deterministically;
- rail planning proves source uniqueness, acyclicity, current aggregation,
  headroom, dropout, capacitor stability, transient capacitance, startup, and
  requested sequencing;
- interface planning proves direct compatibility or selects reviewed
  translation, pull-up, termination, clock, or ADC-drive conditioning;
- stable failures are proven under shuffled inputs and catalog order;
- the neutral MCU, sensor, MOSFET, and amplifier corpus passes architecture
  search and deterministic replay;
- every ready corpus case passes schematic/electrical checks, placement,
  complete routing/connectivity, project writing, writer correctness,
  installed-KiCad ERC, strict DRC, strict zero-diff round trip, and two-run
  normalized equality;
- existing MCU, protected USB-C sensor/LED, ESP32, and amplifier promotion
  evidence remains green;
- production synthesis contains no fixture-name or project-name dispatch,
  device-specific coordinates, allowlists, or one circuit family per case;
- documentation and the final audit distinguish modeled guarantees from
  unsupported SI, thermal, startup, and fabrication claims.
