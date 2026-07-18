# Verified Low-Power Speaker Amplifier Fabrication Candidate

## Objective

Establish a reproducible, fabrication-oriented speaker power-amplifier
capability by generating and validating a protected Class AB amplifier that
delivers at least 10 W RMS into 8 ohms. The implementation must add reusable
component, electrothermal, load, distortion, protection, layout, mechanical,
and fabrication evidence. It must not special-case the checked-in fixture.

## Scope

The first promoted design is a dual-rail, complementary-BJT Class AB speaker
amplifier. It uses a verified audio op-amp voltage stage, a verified
complementary driver pair, a verified complementary power pair, emitter
resistors, a thermally coupled bias spreader, local current limiting, a Zobel
network, and relay-based speaker isolation.

The implementation must remain parameter driven. Component IDs, topology
roles, operating limits, PCB constraints, and validation requests may be
declared by reusable blocks or catalog records. Fixture names and absolute
fixture coordinates must not affect selection, routing, validation, writing,
or fabrication decisions.

## Required Component Evidence

The promoted design must select concrete records for:

- an audio op-amp whose supply, input range, output swing/current,
  gain-bandwidth, slew-rate, noise, distortion, pin map, and package are
  reviewed;
- complementary medium-power driver BJTs with voltage, current, gain, thermal,
  and pin-map evidence;
- complementary audio power BJTs with a reviewed DC safe-operating-area
  boundary and thermal path;
- power emitter resistors and the speaker-isolation relay with concrete MPN,
  package, pin-map, and lifecycle evidence.

Fabrication-candidate selection must fail closed if a required device lacks
quantitative evidence or if the complementary devices do not share a reviewed
pairing group. Substitutes must satisfy the same application envelope rather
than merely sharing a footprint.

## Electrical Contract

The positive fixture must prove:

- at least 10 W RMS into an 8 ohm resistive load over the declared audio band;
- bounded operation into a 4 ohm resistive load without exceeding the declared
  current limit, device SOA, or thermal envelope;
- bounded operation into at least one representative reactive speaker load;
- adequate rail headroom, closed-loop gain, gain-bandwidth, slew rate, and
  output-driver current;
- deterministic clipping and output-power calculations;
- a declared maximum THD budget with independently quantified op-amp,
  crossover, feedback, and load contributions;
- a minimum phase-margin contract for every declared load case;
- deterministic component-tolerance corners including cold/minimum-bias and
  hot/maximum-bias conditions.

All calculations must reject non-finite, missing, dimensionally invalid, or
degenerate inputs. Results and reports must be stable across platforms.

## Electrothermal And SOA Contract

Validation must calculate and gate:

- quiescent and worst-case Class B/AB output-device dissipation;
- driver dissipation and power-resistor dissipation;
- shared-heatsink temperature rise;
- junction temperature through junction-to-case, case-to-sink, and
  sink-to-ambient paths;
- a hot-ambient condition;
- a blocked-airflow condition with an explicit thermal degradation factor;
- transient junction rise using a declared transient thermal impedance or
  conservative bounded approximation;
- DC and fault operating points against the reviewed semiconductor SOA;
- current-limit operation during a shorted output.

The applied maximum junction temperature must include a design margin below
the component absolute maximum. Missing heatsink or mounting evidence blocks
fabrication-candidate status.

## Protection Contract

The promoted topology must include and validate:

- emitter-resistor current sensing and bounded output current limiting;
- a normally open speaker-isolation relay;
- positive and negative DC-output detection;
- delayed relay engagement for turn-on muting;
- bounded relay release for turn-off and supply-fault muting;
- a relay-coil flyback or clamp path;
- an output Zobel/stability network;
- explicit speaker and load-return connectors.

Unsafe variants must fail closed for at least: missing current limiting,
excessive DC trip threshold, excessive turn-on or release time, missing relay
clamp, insufficient contact rating, and a short-circuit point outside SOA.

## PCB And Mechanical Contract

The speaker-power layout profile must require quantified evidence for:

- star return topology separating signal, power, and speaker returns;
- Kelvin feedback and emitter-resistor sensing;
- local rail decoupling;
- high-current positive-rail, negative-rail, output, and speaker-return paths;
- calculated minimum copper width and clearance;
- short feedback and input paths separated from output and relay-current loops;
- driver, bias-spreader, and output-device thermal coupling;
- complementary-device symmetry;
- a heatsink edge, mounting access, component height, and keepout envelope;
- polarized capacitor and connector orientation.

The block realization must use ordinary PCB constraints, routes, zones,
keepouts, and placement groups. No new fixture-specific routing family,
allowlist, coordinate schema, or writer exception is permitted.

## KiCad And Fabrication Proof

The checked-in `class_ab_speaker_10w_protected` fixture must pass:

1. request decoding and deterministic block planning;
2. fabrication-candidate component selection;
3. schematic generation and electrical validation;
4. PCB realization, placement, and route completion;
5. writer correctness;
6. internal connectivity and board validation;
7. real KiCad ERC and strict DRC;
8. zero-difference write/read/write replay;
9. BOM and CPL generation;
10. Gerber and drill generation;
11. fabrication manifest and physical-rule report validation.

The fabrication package must identify the manufacturer profile and record the
source of every threshold used for its readiness decision.

## Preservation

The following existing evidence must remain green:

- `class_a_bjt_line_preamplifier`;
- `class_ab_headphone_protected`;
- `usb_c_i2c_sensor_3v3_protected`;
- the full offline Go test suite and coverage floor.

## Capability Report

The milestone must produce a deterministic capability report and SHA-256
sidecar. The report must contain component-evidence hashes, calculated positive
case measurements, unsafe-case issue codes and paths, KiCad fixture hashes,
fabrication artifact hashes, and the policy version.

## Completion

Completion requires zero high- or medium-severity Prism findings, a clean
worktree after commit, a pushed branch, and green GitHub Actions. Passing unit
tests alone is insufficient.
