# Deterministic Nonlinear And Switching Architecture Synthesis

## Goal

Extend the primitive-only open-topology path from predominantly continuous
analog behavior into low-energy nonlinear and switching behavior. The system
must derive architectures from electrical obligations, execute bounded trusted
device physics, and promote only designs with exhaustive electrical, thermal,
SOA, physical, and installed-KiCad evidence.

## Frozen Envelope

The independently authored corpus is frozen against commit `ebf2918a` before
production changes. Its five positive behavior families are:

1. low-level bipolar magnitude transfer;
2. bounded bipolar transfer;
3. autonomous square-wave generation;
4. controlled pulse power transfer; and
5. efficient low-power step-down conversion.

The adversarial set contains two unsafe electrical/thermal/SOA envelopes and
one deliberately unsupported ultra-fast conversion envelope. Requirement files
contain behavior, operating cases, safety bounds, and acceptance gates only.
They may not name parts, primitive families, equations, solver controls,
coordinates, routes, or expected architectures.

## Trusted Architecture And Model Boundary

- Architecture operators are selected only from typed relationships such as
  polarity folding, bounded transfer, autonomous periodic behavior,
  control-to-power transfer, energy storage, regulation, and feedback.
- Device identities, pin maps, parameters, thermal networks, SOA envelopes,
  switching frequency, and operating limits come from reviewed catalog and
  model-provenance records.
- Provider or fixture data cannot provide equations, matrices, device states,
  initial guesses, time steps, iteration limits, convergence tolerances,
  topology classifications, placement coordinates, route geometry, or
  promotion exceptions.
- Missing or incompatible model coverage is `unsupported`; nonconvergence,
  exceeded ratings, thermal limits, or SOA limits fail closed.

## Dynamic Analysis

The trusted evaluator must support deterministic discontinuity-aware transient
execution for source transitions, autonomous periodic behavior, and controlled
power transfer. Work is bounded by fixed grids, event-aligned substeps, Newton
iteration ceilings, continuation schedules, and total analysis budgets.
Evidence records accepted steps, substeps, iteration totals, continuation
stages, final update/residual bounds, periodic-window selection, and explicit
nonconvergence diagnostics.

Waveform evidence includes frequency, duty cycle, peak-to-peak ripple,
conversion efficiency, transition time, overshoot, device stress, junction
temperature, and transient SOA margin when required by the behavior.

## Physical Synthesis

Readable schematics must preserve left-to-right signal or energy flow, visible
feedback, local return paths, and clear power/reference rails. Switching power
layouts must identify and minimize the high-di/dt input loop, commutation loop,
switch-node copper, gate/control loop, feedback-sense path, and output-current
loop using generic graph-derived roles. Sensitive feedback must remain outside
the switch-node keepout and use a quiet reference return.

No fixture coordinates, fixture identities, block-family layouts, or routing
allowlists are permitted.

## Promotion Contract

Every positive case must prove:

- multiple deterministic architecture candidates where materially distinct
  relationships exist;
- reviewed component/model provenance and exhaustive operating corners;
- deterministic cold replay and concurrency-independent evidence;
- convergence and discontinuity evidence for every dynamic analysis;
- thermal and SOA checks wherever power devices are stressed;
- readable schematic lowering;
- complete placement, routing, and connectivity;
- writer correctness and zero normalized round-trip differences; and
- clean installed-KiCad ERC and strict DRC.

Unsafe cases must be rejected by the claimed safety evidence. The unsupported
case must stop with a stable actionable capability gap and must not emit a
fabrication-ready project.

## Preservation

The complete existing local suite and every promoted current-output,
voltage-output, architecture-generalization, amplifier, MCU, USB-C, and
installed-KiCad fixture must remain green.

## Non-Goals

This milestone does not claim mains safety, isolated conversion, RF power,
high-energy storage, arbitrary semiconductor models, arbitrary SPICE input, or
fabrication approval outside the reviewed low-energy envelope.
