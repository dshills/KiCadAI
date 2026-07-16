# Graph-Derived Catalog-Backed MNA Simulation

## Status

Implemented and verified on 2026-07-16.

## Problem

The trusted simulation registry proves provenance and deterministic replay, but
its first three evaluators encode complete regulator, divider, and RC-filter
topologies. Selecting one of those evaluators is not evidence that KiCadAI can
derive and analyze an unfamiliar circuit from its resolved connectivity.

## Requirements

1. `generic-circuit-v1` may request the trusted `linear_circuit_mna_v1`
   analysis model. The provider may declare bounded DC operating-point and AC
   sweep requests, source operating conditions, observation nodes, and
   assertion bounds. It may not provide equations, matrices, stamps,
   expressions, executable code, model/include files, paths, or topology or
   block classifications.
2. Simulation topology is compiled only after circuit-graph resolution, from
   resolved nets, resolved component functions, immutable catalog identity,
   catalog-validated component values, and catalog simulation evidence.
3. The trusted primitive registry supports deterministic stamps for resistors,
   capacitors, independent voltage sources, independent current sources, and a
   finite-gain, single-pole catalog-parameterized op-amp. Source amplitudes and
   phases are bounded analysis conditions; primitive kind, terminals, device
   values, and op-amp limits come from trusted registry and catalog evidence.
4. Plans snapshot registry/catalog identity plus canonically ordered nodes,
   primitive devices, terminal-to-net bindings, analyses, and a topology hash.
   Execution rejects stale, incomplete, incompatible, ambiguous, or tampered
   plans.
5. The solver uses deterministic unknown and pivot ordering, bounded matrix and
   solution sizes, finite checks, scaled partial pivoting, and residual checks.
   Singular/floating circuits identify the affected unknown and suggest a
   connectivity or reference correction.
6. DC execution treats capacitors as open circuits and validates catalog-backed
   op-amp supply and output operating limits. AC execution uses complex MNA and
   a deterministic logarithmic sweep. All nodes and sweep points are emitted in
   canonical order.
7. Nonlinear devices, unsupported primitive families, multi-valued terminals,
   missing ground/source evidence, incompatible source requests, out-of-range
   op-amp operation, non-finite or numerically unbounded systems, and singular
   systems fail closed with actionable diagnostics.
8. `.kicadai/simulation.json` records registry and catalog provenance,
   topology hash, compiled devices, analyses, every solved node value, assertion
   results, and status. Recorded replay must reproduce it byte-for-byte.

## Held-Out Acceptance Fixture

Add a new generic recorded fixture for a catalog LMV321 buffered two-pole
conditioner assembled only from explicit components and nets. It is not a
block-family request and no divider, filter, amplifier, or other topology is
named in simulation intent. The graph uses independent catalog source ports,
two resistor/capacitor sections, a catalog op-amp follower, automatic hierarchy
partitioning, and both DC and AC analyses.

One optional real-KiCad promotion run must prove catalog resolution, electrical
validation, simulation assertions, route completion, connectivity, clean ERC,
strict clean DRC, writer correctness, promotion readiness, multiple generated
child sheets, zero root/child/PCB round-trip diffs, byte-identical sanitized
recorded replay, and a byte-identical simulation report.

## Regression Requirements

The linear regulator, RC, divider, protected LED, protected I2C, hierarchical
BMP280, and existing generic op-amp evidence remain passing. Existing analytic
models remain accepted for recorded compatibility, but the held-out proof must
use only graph-derived MNA and primitive catalog claims.

## Prohibitions

No fixture-specific coordinates, solver branches, models, metric allowlists,
schemas, topology detectors, block families, readiness overrides, or exception
paths. No provider-supplied executable/model content or direct matrix/stamp
input.
