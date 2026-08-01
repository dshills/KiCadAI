# Simulation-Grounded Circuit Architecture Synthesis Specification

## Objective

Convert behavior-only electrical requirements into multiple materially
different primitive circuit architectures, derive evidence-backed component
values, evaluate every required operating case, rank all passing candidates,
and promote the selected architecture through the normal KiCad pipeline.

The implementation extends `internal/opentopologysynthesis`; it must not add a
fixture selector, named circuit template, hidden coordinates, or a new bypass
around trusted simulation and physical validation.

## Behavioral Contract

Requirements must express behavior and bounds, including as applicable:

- supply domains and tolerances;
- input and output ports;
- voltage or transimpedance gain;
- bandwidth or gain at frequency;
- source and load impedance;
- output voltage/current swing;
- quiescent current;
- output noise;
- harmonic distortion;
- phase/gain margin and transient response;
- ambient/load/supply corners and fault events;
- junction temperature, device dissipation, and SOA margin.

Component identities, internal nets, topology names, values, providers,
coordinates, routes, and repair instructions remain forbidden input.

## Candidate Generation

Production search must:

1. derive obligations only from normalized behavior, port, domain, and
   acceptance facts;
2. construct candidates using reviewed primitive terminal contracts;
3. retain distinct canonical topology hashes;
4. include passive, op-amp, BJT, MOSFET, diode, and protection primitives when
   their evidence and the requirements justify them;
5. generate at least two materially distinct architectures when the bounded
   inventory and work policy permit;
6. remain stable under reordered requirements, catalog entries, primitives,
   ports, cases, and assertions.

## Value Derivation

Every analytic seed must record its equation or derivation, inputs, units, and
source requirement. Preferred catalog values may be ranked around the ideal
result, but only catalog-valid values with tolerance/rating/model provenance may
enter simulation.

Required amplifier derivations include, where applicable:

- closed-loop gain ratio;
- bias-divider current and impedance;
- Class A standing current and load/emitter/source resistance;
- Class AB idle bias and emitter/source ballast;
- load-current and output-swing headroom;
- coupling/bypass/compensation time constants;
- device dissipation and thermal margin;
- protection threshold and series impedance.

## Simulation And Rejection

A candidate passes only when trusted evidence covers every critical assertion
and required corner. The evaluation lane must use operating-point, AC,
transient, distortion, noise, stability, thermal/electrothermal, and SOA
analyses whenever required by the contract.

Candidates must fail closed for:

- missing or untrusted models/provenance;
- supply/output/common-mode/headroom violations;
- insufficient load drive or output swing;
- unsafe quiescent bias or device dissipation;
- rating, thermal, or SOA violations;
- instability, excessive distortion/noise, or unmet bandwidth;
- incomplete case, event, or assertion coverage;
- nonconvergence or exhausted explicit work budgets.

## Ranking And Explanation

Synthesis must not select the first passing candidate. It must evaluate the
available diverse candidates within the explicit count budgets, retain the best
passing value realization for each topology, and rank distinct passing
topologies deterministically.

The selection report must include:

- ranking policy;
- every physically ready passing topology considered;
- requirement-margin score;
- component/internal-node counts;
- topology-repair count;
- evaluation/value/topology evidence hashes;
- selected flag and a concise human-readable reason.

## Physical Promotion

The winner must use the generic topology-aware schematic layout and standard
physical production path. Required gates are:

- schematic readability and electrical diagnostics;
- connectivity and route completion;
- writer correctness;
- native KiCad ERC and strict DRC when required;
- zero normalized round-trip differences;
- deterministic clean-root replay.

No held-out identity or coordinate may occur in production inference, routing,
lowering, or writer code.

## Held-Out Acceptance Corpus

Freeze three independently authored behavior-only requirements before adding
their production search support:

1. a single-ended Class A analog stage;
2. a complementary Class AB load driver;
3. a non-amplifier analog circuit with materially different behavior.

Each passing case must show multiple generated topology hashes, trusted
simulation evidence, an explainable ranked winner, readable KiCad output, and
all required physical gates. At least one case must reject a nominally plausible
candidate for rating/thermal/bias evidence and select a safer alternative.

## Preservation

All existing open-topology, USB-C, MCU/ESP32, amplifier, bus, component
onboarding, routing, writer, and round-trip promoted evidence must remain green
under their authoritative local commands.
