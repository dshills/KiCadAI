# Simulation-Grounded Closed-Loop Circuit Synthesis Specification

## 1. Purpose

This milestone extends deterministic open-set composition into a bounded
generate-simulate-diagnose-repair loop. A promoted circuit must be shown to
meet explicit behavioral requirements over declared operating corners, not
merely be structurally valid, connected, routed, and accepted by KiCad.

Completion establishes a measured behavioral synthesis envelope. It does not
claim arbitrary circuit generation, unrestricted SPICE compatibility, RF or
mains design, or physical hardware equivalence.

## 2. Existing Trusted Base

The implementation must build on, and preserve, the current trusted boundary:

- strict provider output and deterministic open-set architecture search;
- typed component, signal, voltage-domain, and realization contracts;
- graph-derived linear MNA DC/AC, nonlinear DC, and transient solvers;
- catalog-owned primitive models and immutable catalog/topology hashes;
- bounded nominal and tolerance-corner evaluation;
- deterministic lowering, replay, project writing, routing, connectivity,
  writer correctness, normalized round trips, KiCad ERC, and strict DRC.

Provider output may request behavior and bounded operating conditions. It may
not provide equations, matrices, solver settings, executable content, SPICE
directives, model files, include paths, arbitrary expressions, or repair code.

## 3. Public Behavioral Requirement Contract

The public schema is `kicadai.open-set-requirement.v3`. It preserves v1/v2
compatibility and adds capability-neutral behavioral requirements and
operating cases. This is the strict target produced from natural-language
requests by the AI intent boundary.

A behavioral requirement contains:

- a stable ID;
- a semantic observation target referring to a declared external port,
  internal signal, participant port, domain, or whole circuit;
- a registered metric such as DC voltage/current, gain, bandwidth, cutoff,
  noise, phase margin, rise/fall/settling time, startup state, distortion,
  output power, dissipation, or junction temperature;
- a registered analysis class;
- lower and/or upper bounds with explicit units;
- the operating cases in which it must hold;
- whether failure is safety critical.

An operating case contains bounded supply, input, load, temperature, and model
corner selections. It identifies semantic ports, signals, or domains rather
than component names or circuit nets. Unknown metrics, analyses, targets,
units, or corner axes fail strict validation.

The AI translation is accepted only when every material natural-language
performance statement has exactly one normalized requirement or an explicit
fail-closed ambiguity diagnostic. Unrequested invented guarantees are not
accepted as evidence.

## 4. Trusted Analysis Planning

After a candidate is lowered to resolved graph connectivity, a trusted planner
must bind semantic observation and excitation targets to canonical nodes and
catalog-backed devices. The planner selects only registered analyses and emits
a provenance-complete simulation plan.

The bounded analysis registry must support, when applicable:

1. DC operating point and device operating limits;
2. small-signal AC response, gain, cutoff, bandwidth, and phase;
3. input/output-referred noise over a declared band;
4. loop or return-ratio stability evidence and phase/gain margin;
5. transient response, rise/fall/settling time, clipping, and distortion;
6. startup behavior from a registered initial-condition policy;
7. worst-case supply, load, temperature, tolerance, and model corners;
8. thermal dissipation and junction-temperature bounds tied to catalog
   package evidence.

An analysis may be reported `not_applicable` only when no requirement depends
on it. If a required analysis lacks a compatible trusted model, excitation,
observation binding, numerical method, or bounded work plan, the candidate
fails closed.

## 5. Model Provenance And Trust

Every primitive or compact model used for promotion must identify:

- catalog component and variant identity;
- registered model and revision;
- authoritative source identity and source revision/date;
- immutable content or parameter-set SHA-256;
- review status and allowed analysis classes;
- parameter bounds, temperature range, and known applicability limits.

Provider-supplied model provenance is never trusted. Missing, malformed,
stale, incompatible, unreviewed, or out-of-domain evidence blocks every
dependent analysis. Reports must list both used and rejected model claims and
the reason for each rejection.

## 6. Candidate Generation And Selection

The loop receives all retained materially distinct architectures from bounded
search. It evaluates them in canonical fingerprint order and records:

- static architecture score and rationale;
- resolved components, values, topology, and model claims;
- planned analyses and operating corners;
- assertion results and normalized margins;
- rejection, repair, or selection decisions.

Safety and trust failures reject a candidate before scoring. Among passing
candidates, selection remains deterministic and prefers, in order: no safety
failures, greater worst normalized behavioral margin, stronger model evidence,
fewer repairs, the existing architecture score, and the canonical fingerprint.

## 7. Diagnosis And Generic Repair

Failed assertions are converted to typed diagnoses. Registered repair actions
may adjust only public architecture or catalog-backed design variables:

- choose a retained architecture alternative;
- choose a compatible catalog variant;
- move a calculated passive value to a bounded preferred-series neighbor;
- adjust a registered bias, gain, filter, compensation, or protection
  parameter within its declared formula and component ratings;
- add or replace a provider-declared optional compensation/support obligation.

Every action must name its failed assertion, direction of improvement,
authorized variables, preconditions, bounds, and deterministic ordering. After
each action the circuit is re-resolved and all analyses and downstream gates
are rerun; cached partial success is not promotion evidence.

The loop stops on pass, cancellation, exhausted candidate/repair/corner/work
budget, repeated state, non-improvement, unsupported diagnosis, ambiguity,
model-trust failure, or a safety violation. Every non-pass stop is fail-closed.

## 8. Determinism And Replay Evidence

The closed-loop report must snapshot:

- requirement, registry, catalog, formula-library, model-registry, topology,
  and policy hashes;
- every architecture candidate considered;
- every model-claim decision, plan, corner, assertion, diagnosis, and repair;
- state fingerprints before and after repair;
- budget consumption and stop reason;
- final selected architecture and its selection rationale.

Canonical recorded replay must reproduce the lowered circuit, simulation
plans, reports, repair history, selected result, and sanitized artifacts
byte-for-byte. Candidate or repair ordering may not depend on map iteration,
wall-clock time, process identity, filesystem paths, or solver concurrency.

## 9. Frozen Held-Out Behavioral Corpus

Before production implementation, freeze and SHA-256 pin at least ten
behavior-only requirements spanning:

1. low-noise sensor amplifier with bandwidth and threshold behavior;
2. active low-pass or band-pass filter followed by an amplifier;
3. comparator-controlled MOSFET load driver with hysteresis and startup-safe
   fault indication;
4. adjustable or split-supply analog front end;
5. regulated sensor/interface circuit over supply and load corners;
6. protected current-sense/load-control chain;
7. transiently protected digital or mixed-signal interface;
8. Class-A amplifier with gain, bias, swing, distortion, thermal, and
   protection requirements;
9. Class-AB amplifier with gain, bandwidth, output power, crossover/distortion,
   stability, quiescent current, thermal, mute/startup, and protection
   requirements;
10. a mixed-function circuit combining analog conditioning, control, power,
    and protection.

Fixtures contain only behavior, interfaces, domains, operating cases, board
limits, and acceptance gates. They may not name expected topologies, providers,
parts, values, nets, coordinates, layers, routes, or repair actions. The freeze
test rejects implementation detail, unmanifested files, and byte changes.

## 10. Promotion Gates

Every promoted held-out circuit must prove:

- complete natural-language-to-behavior coverage or explicit fail-closed
  ambiguity handling;
- multiple materially distinct generated alternatives when the registry can
  supply them, with deterministic selection rationale;
- trusted model provenance for every required analysis;
- passing nominal and all declared supply, load, temperature, tolerance, and
  model corners;
- passing required DC, AC, noise, stability, transient, startup, distortion,
  and thermal assertions when applicable;
- a bounded, replayable diagnosis/repair history, including at least one
  successful generic repair in the corpus and fail-closed mutation tests for
  every repair category;
- complete catalog resolution, lowering, route completion, connectivity,
  writer correctness, zero normalized round-trip differences, clean KiCad
  ERC, and strict DRC;
- byte-identical end-to-end replay.

Existing open-set, adversarial multi-function, amplifier, ESP32/MCU, analog,
sensor, USB-C, fabrication, simulation, writer, repair, routing, and
round-trip suites must remain green.

## 11. Prohibitions

Production code may not contain fixture IDs, fixture hashes, corpus paths,
expected components or values, topology-specific request mappers, metric
allowlists scoped to corpus entries, coordinate/layer/route exceptions, hidden
repair overrides, weakened gates, or simulation-success substitutions.

Analytic estimates may guide candidate ranking or repair direction, but a
required simulation assertion is satisfied only by its registered trusted
analysis over every required corner.

## 12. Completion Claim

Completion requires a requirement-by-requirement audit linking every clause
above to current files and fresh command evidence, Prism review of the complete
staged diff, committed and pushed changes, and green GitHub Actions for the
exact commit. Documentation must describe the measured envelope and must not
claim arbitrary generation.
