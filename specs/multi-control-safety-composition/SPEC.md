# Generic Multi-Control Safety Composition Specification

## Status

Implemented and locally promoted on 2026-08-02. See `AUDIT.md` for the
reproducible verification record.

## Objective

KiCadAI must synthesize a default-off startup permit and an independent
runtime protection input into one deterministic load-control command. The
implementation must be generic: circuit identity, fixture filenames, and
fixture coordinates may not affect architecture selection, realization,
simulation, repair, or promotion.

## Semantic Contract

A safety coordinator consumes two independently typed controls:

- a connecting control whose function is `enable` or `power_good`;
- a disconnecting control whose function is `fault`, `inhibit`, or `reset`;
- a typed permit output consumed by the protected load switch; and
- logic power and reference domains.

The coordinator normalizes physical polarity before applying this semantic
truth table:

| Connecting control | Protection control | Permit |
| --- | --- | --- |
| deasserted | deasserted | deasserted |
| deasserted | asserted | deasserted |
| asserted | deasserted | asserted |
| asserted | asserted | deasserted |

Protection therefore dominates enable in every state. The permit startup and
safe states are deasserted. Removing logic power must not create an asserted
permit or an apparent successful response.

## Sequencing

An energizing transition may depend on a rail, endpoint, or state becoming
valid and remaining stable for a declared interval. A disconnecting
protection transition is never delayed by an unmet enable dependency.
Dependency cycles, contradictory priority, missing bindings, and an energizing
transition without a deasserted startup state fail closed with stable paths.

## Generic Realization

Providers may use catalog-backed logic, transistor, or comparator structures.
Selection is driven by typed control polarity, voltage/current domains,
startup state, propagation delay, temperature, component budget, and model
availability. The initial discrete realization may use bounded BJT inverter
and clamp stages, but its topology must be selected by semantic roles and
constraints rather than project identity.

Every realization must provide:

- polarity normalization for each input;
- default-off bias when controls are absent or unpowered;
- a dominant protection clamp;
- a typed permit output compatible with the selected load switch;
- catalog and primitive-model provenance; and
- bounded repair variables for bias and timing values only.

Semantic function, polarity, dominance, and transition direction are immutable
contract facts and are not repair variables.

## Directed Verification

Simulation evidence must independently cover:

1. zero-energy startup with permit and load deasserted;
2. enable assertion after all declared dependencies remain stable;
3. protection assertion while enabled, with a falling load response;
4. recovery only after protection deasserts while enable remains asserted;
5. simultaneous enable and protection assertion, proving protection
   dominance; and
6. an unpowered or opposite-direction glitch, proving it is not accepted as a
   real response.

Response measurements carry required physical direction. A transition passes
only when the requested state is reached from the declared initial state
inside its timing envelope.

## Writer-Correctness Prerequisite

Before final promotion, the simulation-grounded active-filter fixture must
write a KiCad schematic with no dangling-wire ERC finding. The correction must
be a deterministic route-label or writer rule applicable to any label-free
direct net; no active-filter identity or coordinates may be encoded.

## Frozen Promotion Targets

`current_sense_protection` and `mixed_function_control_power` become V6
multi-control requirements. They must progress through strict decode,
architecture search, provider expansion, lowering, simulation, PCB generation,
writer correctness, ERC, strict DRC, connectivity, route completion, and
zero-difference round trip. Their prior precise-rejection overlay is removed
only when both become complete passes.

## Completion Criteria

- Truth-table and polarity cases are identity-neutral and frozen by hash.
- Both named targets produce the generic coordinator and pass all directed
  control-state assertions.
- Existing passing corpora and frozen report identities remain unchanged.
- The full simulation-grounded installed-KiCad lane passes twice from identical
  inputs with deterministic replay.
- No fixture-specific allowlist, coordinate, schema, block family, or provider
  branch is introduced.
