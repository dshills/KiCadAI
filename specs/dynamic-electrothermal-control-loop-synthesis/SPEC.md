# Dynamic Electrothermal And Control-Loop Synthesis Specification

## 1. Purpose

This milestone closes the next trust gap between static/corner-bounded circuit
validation and dynamically safe circuit synthesis. Requirements continue to
describe behavior, interfaces, environmental conditions, faults, safety
margins, and physical acceptance. Production code must derive implementation
topology, component identities, model choices, loop probes, solver controls,
thermal networks, repair variables, and physical realization.

Completion is measured against a frozen behavior-only corpus created before
production support. Passing the corpus establishes a bounded dynamic
electrical and electrothermal synthesis envelope. It does not claim arbitrary
SPICE, RF, mains, or unrestricted high-energy design.

## 2. Required Outcomes

The implementation must:

- resolve every required electrical and thermal primitive from reviewed,
  hash-bound catalog evidence;
- identify candidate feedback loops from resolved circuit connectivity rather
  than provider-authored loop labels;
- calculate deterministic return-ratio evidence, crossover frequency, phase
  margin, gain margin, and closed-loop peaking;
- prove stability across every declared supply, load, temperature, tolerance,
  parasitic, fault, and operating-mode corner;
- calculate time-varying device dissipation, thermal-network state, junction
  temperature, and transient safe-operating-area margin;
- execute explicit startup, shutdown, short-circuit, inductive-kick, overload,
  blocked-airflow, and protection-response events where applicable;
- make dynamic evidence participate in candidate rejection, ranking,
  selection, component sizing, and bounded repair;
- preserve immutable safety requirements throughout repair;
- emit complete traceability from behavior through candidates, models,
  analyses, events, diagnoses, repairs, transactions, KiCad objects, and final
  promotion evidence;
- fail closed when any required loop, model, event, corner, assertion, thermal
  path, SOA boundary, convergence result, or protection proof is absent.

## 3. Requirement Contract

The public requirement schema is `kicadai.open-set-requirement.v5`. V5 is an
additive evolution of V4. It retains all V4 hierarchy, interface, resource,
physical-partition, traceability, and promotion requirements.

V5 adds behavior-only operating events and dynamic acceptance gates. An event
may declare:

- a stable semantic identity;
- a registered event kind;
- a semantic target such as a domain, port, signal, participant, or complete
  circuit;
- an optional trigger time and bounded duration;
- initial, applied, and recovered values with canonical units.

Events cannot identify components, models, equations, matrix entries, solver
steps, loop-break locations, nets, pins, coordinates, layers, routes, or
expected implementation data.

V5 acceptance adds mandatory gates for:

- reviewed dynamic-model provenance;
- derived return-ratio and control-loop evidence;
- coupled dynamic electrothermal evidence;
- explicit event and protection-response coverage;
- dynamic architecture selection;
- bounded dynamic repair.

V1-V4 canonical behavior and replay must remain unchanged.

## 4. Reviewed Dynamic Models

Every selected dynamic primitive must be supplied by one unique reviewed
catalog claim. The minimum reusable model set must cover the corpus and include
only bounded, documented behavior. Expected families include:

- inductance with winding resistance, current limit, and tolerance;
- voltage-controlled or state-controlled switching;
- NMOS and PMOS switching behavior with conduction, gate charge/capacitance,
  body-diode, voltage/current, and switching limits;
- finite-bandwidth controlled stages with saturation and slew bounds;
- thermal impedance expressed as a finite Foster or Cauer RC network;
- time-dependent semiconductor SOA boundaries;
- protection elements needed to represent clamp and disconnect response.

Every claim records source, revision, immutable SHA-256, review status,
temperature applicability, allowed analyses, parameter units, and bounded
uncertainties. Provider output cannot supply or alter models, equations,
thermal networks, SOA envelopes, or solver policy.

Unsupported or multiply applicable model claims are blocking.

## 5. Loop Identification And Return Ratio

Loop analysis operates on the resolved graph. It must:

- derive directed control influence from primitive terminal semantics;
- find canonical strongly connected feedback paths;
- identify a valid injection/observation cut without changing the DC operating
  point;
- distinguish nested or multiple loops and preserve deterministic loop order;
- reject unobservable, uncontrollable, positive-feedback, ambiguous, or
  unsupported loops with stable diagnostics;
- evaluate return ratio over a bounded deterministic frequency grid;
- interpolate crossover and margin values deterministically;
- record loop members, injection boundary, DC preservation evidence, sampled
  return ratio, crossover, phase margin, gain margin, and peaking.

Static catalog stability statements are admissible only as model applicability
evidence. They cannot substitute for required candidate-level return-ratio
analysis.

## 6. Dynamic Electrothermal Evaluation

Electrothermal evaluation couples electrical transient results to reviewed
thermal networks using deterministic bounded work:

1. solve or reuse the required electrical operating point;
2. execute the declared electrical event sequence;
3. calculate instantaneous power for each modeled device;
4. integrate every finite thermal RC state over the same observation grid;
5. update temperature-dependent bounded electrical parameters where supported;
6. check junction temperature, power, current, voltage, energy, and
   time-dependent SOA at every observation point;
7. iterate only when the registered model declares coupled feedback and stop
   under explicit convergence and work limits.

Ambient temperature, case or heatsink boundary, airflow degradation, thermal
coupling, and recovery conditions must be explicit. A scalar steady-state
thermal-resistance estimate cannot satisfy a required dynamic thermal gate.

## 7. Events And Protection

Registered event kinds include startup, shutdown, input step, load step,
short-circuit, overload, inductive turn-off, blocked airflow, rail loss, and
protection recovery.

Event execution must prove applicable:

- output overshoot and undershoot;
- settling and recovery time;
- peak voltage, current, and dissipation;
- clamp or disconnect response;
- current-limit behavior;
- thermal peak and recovery;
- transient SOA margin;
- safe startup and shutdown state.

The analysis plan must account for every declared event and every critical
behavioral requirement. Missing or silently skipped event coverage is
blocking.

## 8. Dynamic Search And Repair

Search evaluates complete candidates using static, dynamic, hierarchical, and
physical evidence. It must retain canonical alternatives and:

- reject candidates that pass static checks but fail stability,
  electrothermal, SOA, or protection evidence;
- rank passing candidates by worst critical dynamic margin after model trust
  and assertion coverage;
- prove at least two frozen cases select a dynamically safe alternative after
  rejecting a statically valid preferred candidate;
- expose only registered, bounded variables such as compensation value,
  damping, gate resistance, current limit, thermal path, or protection timing;
- evaluate repair trials deterministically and retain full before/after
  evidence;
- preserve topology identity unless candidate backtracking, rather than local
  repair, is explicitly recorded;
- stop on unsupported diagnosis, non-improvement, repeated state, exhausted
  work, or any immutable safety conflict.

No repair may weaken a required behavioral bound, remove an event, relax an
SOA/temperature limit, or replace reviewed evidence with an estimate.

## 9. Frozen Held-Out Corpus

The corpus lives at
`internal/architecturesearch/testdata/dynamic_electrothermal_control_loop_corpus`
and contains six SHA-256-pinned V5 requirements:

1. feedback amplification into a reactive load;
2. a compensated analog servo regulator;
3. a switching power converter;
4. a protected inductive-load switch;
5. a Class-AB stage with dynamic thermal and load-fault requirements;
6. a sequenced multi-rail controller with transient protection.

The corpus must collectively cover stability, startup, shutdown, input/load
steps, overload, short circuit, inductive turn-off, blocked airflow,
protection response, thermal recovery, and transient SOA. It must not contain
topology, part, model, loop, equation, net, pin, coordinate, route, provider,
or expected-result hints.

The independent freeze test pins manifest membership and every fixture byte,
strict-decodes through a mirror schema, checks semantic neutrality and coverage,
and rejects unmanifested files. The pre-implementation baseline must prove
that production V4 rejects V5 and cannot emit the new dynamic evidence.

## 10. Negative Corpus

A reordered negative corpus must prove stable fail-closed diagnostics for at
least:

- unsupported primitive or missing reviewed provenance;
- ambiguous or unobservable feedback loop;
- insufficient phase or gain margin;
- electrothermal nonconvergence;
- thermal runaway or junction-temperature violation;
- transient SOA violation;
- unproven clamp, disconnect, or current-limit response;
- missing event coverage;
- unsupported dynamic repair;
- dynamic work-budget exhaustion.

Input and candidate reordering must not change the canonical terminal code,
diagnostic path, selected safe result, or evidence hash.

## 11. Physical And KiCad Promotion

Every positive fixture must pass:

- strict decode, normalization, dynamic planning, and deterministic replay;
- reviewed model selection and applicability validation;
- loop, corner, event, electrothermal, SOA, protection, search, and repair
  evidence;
- hierarchy, interface-contract, shared-resource, physical-partition, and
  traceability validation;
- complete lowering without lost dynamic evidence;
- internal validation, connectivity, route completion, and writer correctness;
- clean installed-KiCad ERC and strict DRC;
- zero normalized schematic and PCB round-trip differences;
- two local clean-checkout promotion runs with identical normalized bundles.

The 12/12 held-out benchmark, six hierarchical systems, amplifier lanes,
MCU/sensor/ESP32 fixtures, protected USB-C fixtures, writer/routing suites, and
the existing installed-KiCad promotion matrix must remain green locally.

Production code may not add fixture-specific coordinates, allowlists, schemas,
topology families, block identities, benchmark-aware selection, or conditional
success paths.

## 12. Closeout

Closeout requires:

- a requirement-by-requirement evidence audit;
- Prism review with every high and medium finding resolved;
- rerun of all affected and full local gates after review changes;
- a clean committed and pushed tree.

GitHub Actions are not used as the development or promotion test runner. The
push may start existing automatic workflows, but local evidence is the
authoritative closeout loop; any later GitHub failure reported by the user
reopens the milestone.
