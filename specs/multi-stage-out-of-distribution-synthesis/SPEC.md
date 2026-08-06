# Multi-Stage Out-of-Distribution Circuit Synthesis

## Goal

Prove that the public open-topology workflow can generalize to unfamiliar
behavioral requirements that compose sensing, decision, regulation, nonlinear
transfer, power delivery, and protection. Success means composition of existing
knowledge plus reusable new capabilities, not recognition of a named circuit.

## Frozen Evaluation Envelope

The corpus is frozen against commit `f06a1a62` before production changes. It
contains nine valid requirements and four unsafe or unsupported requirements.
Every requirement is independently authored as electrical behavior, operating
conditions, events, safety bounds, and acceptance gates only.

Requirement inputs may not name or encode expected architectures, parts,
implementation primitives, device identities, simulation models, equations,
connections, physical coordinates, board layers, placement, or routing. Corpus
integrity tests enforce the prohibition and bind every file to a checksum.

A positive case qualifies as multi-stage only when its manifest declares at
least three distinct functional behaviors, its requirement exposes an external
stimulus and useful output, and its acceptance depends on at least three
analysis classes including dynamic and safety evidence. The frozen positive
matrix covers:

1. ambient tracking with controlled airflow power;
2. bipolar magnitude fault indication;
3. enabled current regulation;
4. inductive-load current control;
5. illumination-proportional power control;
6. low-voltage power delivery with soft start;
7. undervoltage load permission;
8. windowed heating power control; and
9. bounded audio power transfer.

The adversarial matrix covers excessive inductive energy, an impossible compact
thermal envelope, unsupported ultra-fast protected feedback, and an unsupported
high-energy sensing domain.

## Deterministic Public-CLI Baseline

Before production implementation, every frozen case must be executed twice
through the documented public CLI. A checked-in baseline records input hash,
command contract, exit status, terminal stage, diagnostic kind, artifact
emission, and replay equality. Failures are grouped by reusable capability gap,
never by fixture identity or expected implementation.

## Generic Implementation Boundary

Production work may add only capabilities justified by a baseline cluster:

- relationship-derived composition operators and bounded architecture search;
- reviewed device and analysis coverage with provenance;
- deterministic value selection and multi-stage constraint propagation;
- generic diagnosis and bounded cross-stage repair;
- semantic schematic-flow, power-return, placement, and routing rules; and
- precise safety or capability rejection.

Fixture identifiers, fixture coordinates, expected block families, allowlists,
special schemas, precomputed graphs, and case-selected writer output are
forbidden. A repair must re-enter the earliest invalidated stage and rerun every
affected electrical, safety, and physical gate.

## Positive Promotion Contract

All nine positive cases must pass twice from clean output directories through
the public CLI and produce identical normalized evidence. Each result must
prove:

- complete behavioral simulation across all declared cases and corners;
- reviewed provenance, convergence, thermal limits, and SOA margin;
- readable conventional schematic flow with visible control and return paths;
- complete placement and routing with connectivity preserved;
- writer correctness and zero normalized round-trip differences;
- clean installed-KiCad ERC and strict DRC; and
- deterministic reports, generated files, selected architecture, and repairs.

No partial, simulation-only, or mock-KiCad result counts as a design pass.

## Fail-Closed Contract

Each adversarial case must stop with its frozen stable failure kind, actionable
evidence, and no KiCad project. Unsafe electrical, thermal, or SOA evidence may
not be repaired around, downgraded, or replaced by a capability diagnosis.
Unsupported envelopes must be distinguished from unsafe feasible candidates.

## Preservation And Non-Goals

Every previously promoted corpus and installed-KiCad fixture must remain green.
This milestone does not claim arbitrary mains, isolation, RF, high-energy,
fabrication, dense multilayer, or hierarchical-design support. Dense multilayer
layout and human-quality schematic hierarchy are the next physical milestone
after this corpus is promoted.
