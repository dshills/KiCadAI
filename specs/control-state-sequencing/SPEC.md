# Control-State and Sequencing Semantics

## Status

Implemented; final promotion evidence is recorded in `AUDIT.md`.

## Problem

Behavior-level synthesis previously identified a signal as `control`, `enable`,
or `fault`, but did not state whether electrical high or low meant asserted,
which state was required during startup, which direction constituted a valid
response, or which prerequisite state had to remain stable first. A transient
that moved in the wrong direction—or occurred before the relevant circuit was
powered—could therefore be mistaken for a response.

The strict transient evaluator exposed this gap in the identity-neutral
`current_sense_protection` and `mixed_function_control_power` cases. The fix
must describe reusable behavior. It must not recognize fixture names,
coordinates, selected parts, net names, or topology identities.

## Scope

The public `kicadai.open-set-requirement.v6` contract extends the V3
behavioral-composition envelope with explicit control-state semantics. V6 is
not a replacement for the V4 hierarchical contract or the V5 dynamic thermal
and control-loop contract.

V6 adds:

- control function: `enable`, `inhibit`, `reset`, `fault`, `power_good`, or
  generic `state`;
- electrical polarity: `active_high` or `active_low`;
- startup and safe state: `asserted`, `deasserted`, or `unknown` where allowed;
- a named transition with semantic target and trigger observations;
- logical from/to state and required physical `rising` or `falling` direction;
- optional minimum and maximum transition delay;
- zero or more prerequisite states with an optional stable-duration bound;
- an explicit link from each response-time behavior to the transition it
  measures.

## Invariants

1. A control declaration is topology- and identity-neutral.
2. Function, polarity, startup state, safe state, direction, and state tokens
   use closed vocabularies and fail validation when contradictory.
3. Every V6 `response_time`, `protection_response_time`, or `sequence_delay`
   behavior names exactly one declared transition.
4. Every declared transition is measured by at least one behavior.
5. Transition targets, triggers, and prerequisites are semantic observations;
   lowering must resolve them to trusted concrete targets or stop with a stable
   diagnostic.
6. A response-time assertion measures only its declared physical direction.
   An earlier opposite-direction excursion is not a successful response.
7. Startup assertions cannot claim proof for a requested asserted state unless
   a reviewed provider can establish it.
8. Normalization and planning are deterministic under irrelevant input order.
9. V3, V4, and V5 requirements preserve their existing behavior and hashes.

## Generic Synthesis and Repair

Control polarity is projected into generic producer output polarity and
consumer control action. Active-high and active-low enable/inhibit/reset/fault
contracts select the corresponding connect or disconnect orientation without
request identity checks.

The repair surface remains bounded and catalog-backed:

- polarity and threshold orientation select a deterministic topology before
  simulation;
- threshold feedback resistance is a bounded threshold-orientation variable;
- high-side control-inverter bias resistance is a bounded bias variable;
- rail-sequencing timing capacitance is a bounded timing variable;
- all numeric candidates come from the existing preferred-value policy and
  retain declared effects and limits.

The contract's polarity and required direction are immutable requirements, not
repair variables. A repair may alter implementation orientation or bounded
bias/timing values, but may not redefine what asserted means to make a failing
candidate pass.

## Required Evidence

- a frozen identity-neutral corpus covering active-high and active-low enable,
  inhibit, reset, fault, and power-good sequencing;
- raw fixture hashes plus canonical decode/replay checks;
- precise rejection tests for contradictory and underconstrained contracts;
- directed transient measurement that rejects an opposite-direction glitch;
- resolved prerequisite targets and precise rejection of missing bindings;
- bounded polarity, bias, threshold, and timing synthesis/repair coverage;
- precise topology-neutral rejection of the contradictory startup claims in
  `current_sense_protection` and `mixed_function_control_power`;
- preservation of every existing passing corpus;
- complete local tests and two identical installed-KiCad promotion runs.

## Non-Goals

This milestone does not establish unrestricted state-machine synthesis,
arbitrary mixed-signal models, arbitrary power sequencing, or arbitrary circuit
generation. It does not infer polarity or safe state from a fixture name. V4
hierarchy and V5 dynamic electrothermal behavior remain separate versioned
contracts until an independently specified successor safely combines them.
