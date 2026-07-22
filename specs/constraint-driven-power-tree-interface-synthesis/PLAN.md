# Constraint-Driven Power-Tree and Interface Synthesis Plan

## Working rules

- Keep the open-set requirement and typed contract model topology-neutral.
- Put device facts in validated catalog evidence, not provider branches.
- Keep selection, calculations, diagnostics, hashes, and output ordering
  deterministic.
- Fail closed whenever a safety-relevant electrical or thermal claim lacks
  evidence.
- Run the smallest relevant neutral KiCad-backed case after each physical
  correction and preserve established promotion evidence.
- Do not add fixture coordinates, project dispatch, target allowlists, or a
  block family for each corpus case.

## Phase 1: Baseline audit and governing contract

Objective: identify reusable capability and the exact missing guarantees.

- Inventory regulator, capacitor, translator, op-amp, MOSFET, protection,
  thermal, simulation, global-search, lowering, placement, routing, and writer
  support.
- Map every goal clause to source and test evidence or a named gap.
- Write `SPEC.md`, this plan, and an initial acceptance matrix.
- Freeze the current MCU and mixed-function promotion results as regression
  evidence.

Exit:

- no acceptance clause is represented only by prose or assumed legacy behavior;
- all planned schema additions remain generic and data-only.

## Phase 2: Typed power/interface evidence

Objective: make required facts machine-checkable.

- Add normalized power-conversion transient/startup evidence.
- Add generic interface electrical evidence for drivers, receivers,
  translators, clocks, and ADC acquisition.
- Extend capacitor/regulator evidence only where existing fields cannot express
  effective capacitance, ESR/ripple, startup, or transient conditions.
- Validate cross-field invariants and fabrication proof requirements.
- Add shuffled-order and malformed-evidence tests.
- Populate reviewed evidence for the smallest neutral component set needed by
  the corpus.

Exit:

- verified records cannot claim a modeled power/interface analysis without the
  necessary typed fields;
- catalog hashes and coverage goldens are reproducible.

## Phase 3: Deterministic rail planning

Objective: solve the whole directed rail graph rather than isolated regulator
requests.

- Build rail nodes from supply domains and power signals.
- Resolve exactly one producer for each generated rail and reject cycles.
- Aggregate participant, objective, downstream-converter, quiescent, startup,
  and transient demand.
- Apply current headroom and derating consistently.
- Select fixed, adjustable, negative, or isolated conversion using catalog
  evidence and stable ranking.
- Derive output stability capacitance and input/output bulk capacitance.
- Emit stable typed rejection codes and bounded candidate details.
- Prove input-order and catalog-order determinism.

Exit:

- multi-rail positive and negative tests cover source uniqueness, cycles,
  unknown/exceeded current, dropout, stability, and transient sizing;
- selected rail plans lower through ordinary fragment realizations.

## Phase 4: Sequencing, startup, transient, and thermal proof

Objective: connect power selection to operating behavior.

- Add generic sequence constraints and calculations.
- Require catalog startup/soft-start evidence or startup simulation.
- Validate monotonicity, inrush, downstream enable ordering, and fail-safe
  startup state where requested.
- Carry regulator and pass-device dissipation into thermal selection and
  simulation across operating cases.
- Reject incomplete ambient/case thermal paths.
- Add stable capability-gap mapping for all power failure families.

Exit:

- startup, transient, and thermal constraints are supported only when proven;
- missing evidence produces stable unsupported output, never a nominal guess.

## Phase 5: Interface discovery and direct-compatibility proof

Objective: reason across every endpoint sharing an interface.

- Group typed endpoints by signal/protocol and domain.
- Validate direction, signaling mode, voltage windows, reference domains,
  current, impedance, frequency, fanout, and default states.
- Permit direct connection only when the complete group is compatible.
- Derive deterministic child obligations when translation or conditioning is
  necessary and uniquely implied.
- Produce a stable clarification when multiple materially different
  conditioning choices remain.

Exit:

- direct, translated, unsupported, and ambiguous interface outcomes are covered
  under shuffled endpoint order.

## Phase 6: Pull-up, translation, and termination synthesis

Objective: replace fixed interface support values with bounded calculations.

- Solve whole-bus open-drain pull-up resistance windows.
- Select preferred catalog resistors and validate sink current and rise time.
- Select reviewed translators by voltage, direction, channel, mode, speed, and
  startup behavior.
- Derive source-series termination from impedance evidence.
- Support parallel/Thevenin termination only with explicit DC-load budgets.
- Emit calculations and margins into architecture evidence.

Exit:

- pull-up and termination values come from request/catalog facts;
- empty solution windows fail deterministically.

## Phase 7: Clock and ADC/op-amp conditioning

Objective: cover the mixed-signal boundaries that most often invalidate a
nominally connected design.

- Add catalog-driven clock source damping/translation/bias selection.
- Validate amplitude, common mode, fanout, edge rate, frequency, startup, and
  modeled jitter class.
- Add passive ADC drive/anti-alias calculation.
- Add op-amp buffer selection with common-mode, swing, bandwidth, slew, noise,
  output-current, capacitive-load stability, isolation, and thermal checks.
- Validate acquisition settling across operating cases.

Exit:

- passive and buffered ADC cases are proven;
- missing acquisition or stability evidence returns a stable gap.

## Phase 8: Generic physical constraints and lowering

Objective: preserve electrical intent through KiCad generation.

- Carry rail-current, decoupling, bulk, sequence/control, termination,
  return-path, thermal, and matched-group semantics into circuit graph and PCB
  intent.
- Prefer generic proximity/order/net-class rules.
- Ensure route completion does not erase impedance/current/length constraints.
- Verify writer round-trip preservation of all generated components, nets,
  layers, rules, and metadata.

Exit:

- no physical operation depends on corpus or selected device identity;
- generic placement/routing tests cover each new semantic rule.

## Phase 9: Neutral mixed-signal corpus

Objective: prove composition across MCU, sensor, power MOSFET, and amplifier
domains.

- Add the four ready cases required by `SPEC.md`.
- Add failure-driven cases for missing rail source, insufficient current,
  unstable capacitor window, unproven sequence, incompatible translation,
  empty pull-up window, unsupported termination, and ADC settling failure.
- Assert target-free requests and deterministic selected results.
- Require architecture coverage accounting and stable capability gaps.

Exit:

- all neutral cases replay identically and contain no catalog or fixture hints;
- negative cases fail for their intended generic codes.

## Phase 10: KiCad promotion and regression preservation

Objective: prove reproducible physical deliverables.

- Run offline workflow promotion for every ready case.
- Run installed-KiCad ERC and strict DRC.
- Require complete routing/connectivity, writer correctness, strict zero-diff
  schematic/PCB round trip, and normalized two-run equality.
- Re-run MCU corpus, ESP32 minimal, protected USB-C sensor/LED, and promoted
  amplifier evidence.
- Record tool versions and output hashes.

Exit:

- every required physical gate is clean;
- existing promoted evidence remains clean.

## Phase 11: Review, documentation, and closure

Objective: close with auditable evidence and an honest boundary.

- Update README, readiness, generation, component-intelligence, circuit-block,
  and roadmap documentation.
- Write `AUDIT.md` with requirement-to-test and output-hash evidence.
- Search production code for prohibited identities and coordinates.
- Review staged changes with Prism when policy permits and address actionable
  findings.
- Run the full repository suite, commit, push, and verify GitHub Actions.

Exit:

- the completion audit proves every `SPEC.md` acceptance item;
- unsupported guarantees are explicitly documented;
- the repository is clean and synchronized with its remote.
