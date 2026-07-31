# Simulation-Guided Open-Topology Synthesis Specification

## 1. Purpose

KiCadAI shall synthesize a bounded circuit topology directly from verified
primitive components and behavior-only requirements, refine component choices
and values using trusted simulation evidence, and promote the result through
the existing physical and KiCad-backed acceptance path.

This milestone closes the gap between selecting a registered functional
fragment and constructing a new circuit graph. Completion proves a measured
open-topology envelope. It does not claim unrestricted analog synthesis,
arbitrary SPICE compatibility, RF, mains, high-voltage, safety certification,
or fabrication signoff.

## 2. Existing Trusted Base

The implementation must reuse and preserve:

- strict behavioral requirement decoding and canonical hashing;
- the reviewed component catalog and model-provenance registry;
- graph-derived linear, nonlinear, transient, noise, stability, thermal, and
  electrothermal analyses;
- deterministic preferred-value and catalog-variant selection;
- deterministic circuit-graph lowering and physical generation;
- routing, connectivity, writer correctness, normalized round-trip, ERC, DRC,
  replay, and promotion evidence;
- all existing open-set, closed-loop, MCU, amplifier, component-onboarding,
  writer, routing, and KiCad-backed regressions.

Existing named capability providers remain supported for their measured
envelope, but they are not valid evidence for this milestone's held-out cases.

## 3. Public Requirement Boundary

The public contract is `kicadai.open-topology-requirement.v1`.

It contains only:

- project identity and descriptive intent;
- external supply, reference, input, output, and control ports;
- electrical limits on those ports;
- named operating cases and bounded events;
- measurable behavioral assertions with registered analyses, metrics, units,
  bounds, observation targets, and optional excitation targets;
- component-count and board-envelope limits;
- the complete acceptance profile.

The contract may not contain:

- component, manufacturer, MPN, catalog, symbol, footprint, pin, or pad IDs;
- topology, block-family, provider, expansion, formula, model, or solver IDs;
- internal net names or an expected component count;
- values, coordinates, layers, tracks, vias, routing hints, or repair actions;
- a preferred or expected circuit class.

An excitation and observation identify semantic external ports or domains.
They describe what must be measured, not how the circuit must be built.

## 4. Primitive Contract

Production search may instantiate only catalog records that provide:

1. an accepted symbol and package variant;
2. complete function-to-pin and function-to-pad evidence;
3. a registered primitive electrical model for every required analysis;
4. reviewed operating/rating evidence covering the requested cases;
5. deterministic value or parameter candidates where applicable.

The initial synthesis envelope is limited to generic primitive families:

- resistor, capacitor, and inductor;
- signal, clamp, and reference diode;
- NPN and PNP BJT;
- N-channel and P-channel MOSFET;
- operational amplifier;
- comparator;
- adjustable and floating linear voltage regulator;
- external connector, voltage source, and load harness elements used only by
  trusted evaluation.

Current-source, load-switch, translator, isolator, clock-source, MCU, sensor,
and other functional compact models may not satisfy an open-topology held-out
case. A regulator is admitted only as one catalog-backed atomic component with
its terminal-level model and complete fabrication-candidate evidence; a
registered regulator block expansion is not. Fixed regulator records whose
output-network evidence remains incomplete therefore continue to fail closed.
Other compact-model claims remain available to existing workflows.

Every primitive candidate records its catalog identity, variant, terminal
contract, model revision/hash, evidence rank, rating envelope, and allowed
value domain. These details appear only in generated evidence, never in the
input requirement.

## 5. Candidate Graph

A candidate graph contains:

- external semantic nodes derived from requirement ports and domains;
- canonical anonymous internal nodes;
- primitive instances with typed terminals;
- catalog-backed values and bounded repair domains;
- node roles inferred from connectivity and observation use;
- provenance hashes and a canonical topology fingerprint.

Graph identity is invariant to instance ordering, internal-node names, input
JSON ordering, map iteration, process identity, filesystem location, and
parallel execution.

The candidate must pass structural checks before simulation:

- every required primitive terminal is connected or explicitly permitted open;
- no power-source short, reference conflict, duplicate terminal attachment, or
  impossible direction exists;
- every external output is reachable from a relevant excitation or supply;
- every active device has a complete supply/reference path;
- every internal node participates in a source-to-observation cone;
- component, net, degree, and graph-depth budgets are respected;
- ratings that can be disproved statically reject the graph before simulation.

## 6. Deterministic Bounded Search

Search starts from semantic external nodes and expands only generic,
terminal-level graph operations:

- add one primitive;
- connect one primitive terminal to an existing compatible node;
- introduce one anonymous internal node;
- join two compatible anonymous nodes;
- add a feedback or feed-forward edge between existing nodes;
- replace one primitive with a compatible catalog alternative;
- remove a primitive or internal node proven irrelevant.

Operations are generated from primitive terminal contracts and electrical
compatibility rules. They may not be selected by a fixture ID, named circuit
family, expected topology, or corpus-specific metric combination.

Search is best-first and deterministic. Ranking may use:

1. structural invalidity and unproven safety obligations;
2. failed critical assertions;
3. worst normalized simulation margin;
4. model and catalog evidence rank;
5. graph size and estimated physical area;
6. repair count;
7. canonical graph fingerprint.

Equivalent graphs are removed by canonical topology hashing. Dominated partial
states are removed only when the domination proof is recorded.

The policy has explicit count budgets for expanded graph states, generated
graphs, primitive instances, internal nodes, candidate simulations, operating
corners, value trials, topology repairs, and retained complete candidates.
Wall-clock time is not a ranking or termination input.

## 7. Value Search

Each value-bearing primitive receives a deterministic bounded domain derived
from:

- the accepted catalog record;
- its reviewed rating and tolerance evidence;
- requested operating bounds;
- registered preferred-number series;
- analytically derived scale estimates used only to seed or order candidates.

Search may change only to a value present in that domain. It must record the
derivation, preferred-series identity, tolerance, trial order, rejected
values, and selected value. Literal values from held-out fixtures may not
appear in production branches.

## 8. Trusted Simulation And Diagnosis

Every retained complete graph is resolved through the existing reviewed
primitive registry. All applicable assertions run over every declared supply,
load, input, temperature, tolerance, model, startup, and event case.

Failed assertions produce stable topology-neutral diagnoses, including:

- observation unreachable;
- DC target low or high;
- gain low or high;
- cutoff or bandwidth low or high;
- threshold or hysteresis low or high;
- output impedance or drive insufficient;
- startup state wrong;
- rise, fall, or settling too slow;
- stability margin insufficient;
- distortion, noise, dissipation, junction temperature, or SOA excessive;
- operating point invalid, singular, nonconvergent, or model unsupported.

A diagnosis must identify the assertion, case, analysis, observed value,
required bound, affected graph cone, and evidence hash.

## 9. Generic Graph Repair

Repair is a bounded continuation of graph search. Registered repair operators
may:

- adjust a value within its frozen domain;
- substitute a compatible primitive variant;
- add, remove, or redirect one passive edge;
- add or remove one generic feedback/feed-forward connection;
- exchange one polarity-compatible primitive family;
- add one generic protection, bias, pull, or compensation primitive when its
  terminal contract and diagnosis make the change admissible.

Repair operators are keyed to diagnosis classes and terminal roles, not
fixture IDs, circuit names, topology labels, or expected components. Each
repair records preconditions, the exact graph delta, affected assertions,
before/after hashes, work consumed, and why the operation is expected to move
the diagnosed metric.

Every repaired graph is fully re-resolved and all required analyses and
physical gates are rerun. Cached partial success is not promotion evidence.

## 10. Stable Failure Contract

Non-pass outcomes use stable codes:

- `OPEN_TOPOLOGY_REQUIREMENT_INVALID`;
- `OPEN_TOPOLOGY_PRIMITIVE_UNAVAILABLE`;
- `OPEN_TOPOLOGY_MODEL_UNAVAILABLE`;
- `OPEN_TOPOLOGY_SEARCH_EXHAUSTED`;
- `OPEN_TOPOLOGY_NO_COMPLETE_GRAPH`;
- `OPEN_TOPOLOGY_NO_PASSING_GRAPH`;
- `OPEN_TOPOLOGY_VALUE_EXHAUSTED`;
- `OPEN_TOPOLOGY_REPAIR_UNSUPPORTED`;
- `OPEN_TOPOLOGY_REPAIR_EXHAUSTED`;
- `OPEN_TOPOLOGY_CANCELED`;
- `OPEN_TOPOLOGY_PHYSICAL_PROMOTION_FAILED`.

Every failure includes budget consumption, the strongest rejected candidate,
stable diagnosis summaries, and an actionable suggestion. Missing model,
rating, package, or physical evidence fails closed.

## 11. Frozen Held-Out Corpus

Before production implementation, freeze and SHA-256 pin eight
identity-neutral, topology-neutral cases:

1. adjustable current source;
2. hysteretic detector;
3. active low-pass filter;
4. discrete linear regulator;
5. low-side load driver;
6. sensor conditioner;
7. audio muting circuit;
8. voltage-window monitor.

Fixtures may describe only external behavior, interfaces, operating cases,
events, limits, and acceptance gates. They must not name primitive families.

At least six of eight cases must pass the complete promotion lane. Every
remaining case must fail with a stable, correct unsupported or exhausted code;
no case may be silently mapped to a registered functional block.

The corpus must include:

- at least three distinct active-device families among passing results;
- at least two cases with more than one materially distinct retained topology;
- at least two cases selected only after a simulation failure;
- at least one successful graph-changing repair;
- one startup/event-driven case;
- one thermal or SOA-constrained case;
- one dual-threshold or multi-observation case.

## 12. Promotion Gates

Every passing held-out case must prove:

- strict requirement decode and implementation-detail rejection;
- primitive-only component and model provenance;
- deterministic bounded topology and value search;
- complete search, rejection, diagnosis, and repair evidence;
- all required nominal, corner, event, and safety assertions;
- deterministic catalog resolution and circuit-graph lowering;
- complete connectivity and routing;
- writer correctness;
- normalized zero-difference schematic, PCB, and hierarchy round trips;
- clean installed-KiCad ERC and strict DRC;
- byte-identical replay in two runs from clean local roots.

Promotion evidence must state that no selected component uses a prohibited
functional compact model and no selected graph came from a named provider
expansion.

## 13. Leakage And Regression Controls

Tests must scan production code and generated evidence for:

- held-out IDs, hashes, filenames, titles, and prompts;
- expected catalog identities or values;
- fixture-specific metric combinations;
- topology labels used as search branches;
- corpus paths or acceptance exceptions;
- weakened simulation, physical, writer, ERC, DRC, or round-trip gates.

The existing provider-backed path remains byte-stable for existing fixtures.
The protected USB-C LED/I2C, ESP32, Class-A, Class-AB, component-onboarding,
writer, routing, simulation, and promotion suites remain green.

## 14. Completion Claim

Completion requires a clause-by-clause audit linked to production files and
fresh local command evidence, Prism review of the complete staged diff, a
commit, and a push. Per standing project policy, GitHub Actions are not run as
the primary test loop; local results are authoritative unless a later GitHub
failure is reported.
