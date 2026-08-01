# Simulation-Grounded Circuit Architecture Synthesis Plan

## Phase 0 — Audit And Freeze

- Map the current open-topology search, value, simulation, repair, ranking, and
  physical paths against the specification.
- Freeze behavior-only Class A, Class AB, and non-amplifier requirements plus a
  SHA-256 manifest.
- Record the untouched outcome and prohibit identity/topology/value leakage.

Exit: the new corpus and exact capability gaps are immutable.

## Phase 1 — Ranked Multi-Architecture Selection

- Evaluate diverse retained topologies before graph repair.
- Retain the best physical pass per topology.
- Rank passing architectures by normalized requirement margin, repairs,
  complexity, and stable hashes.
- Emit structured alternatives and a human-readable selection explanation.

Exit: winner selection is evidence-based rather than first-pass order.

## Phase 2 — Analog Architecture Grammar

- Add requirement-derived relationships for single-ended active stages,
  complementary follower/output stages, bias networks, local feedback,
  coupling/decoupling, and protection.
- Generate op-amp, BJT, MOSFET, passive, and protection combinations without
  topology names or fixture selectors.
- Add permutation and identity-neutral tests.

Exit: each held-out case produces multiple materially distinct complete graphs.

## Phase 3 — Equation-Proven Value Seeding

- Add documented analytic scales for bias, gain, standing/idle current,
  headroom, load drive, compensation, coupling, protection, and thermal power.
- Bind every value to requirements and reviewed catalog evidence.
- Prove deterministic preferred-series and device-variant ordering.

Exit: candidate values are reproducible and explainable before simulation.

## Phase 4 — Amplifier Simulation And Rejection

- Bind all required operating-point, AC, transient, distortion, noise,
  stability, thermal/electrothermal, and SOA analyses.
- Add generic diagnoses for bias, headroom, crossover, drive, dissipation, and
  stability failures.
- Add bounded topology/value/device repair where a trusted diagnosis supports
  an expected direction.

Exit: unsafe or underperforming candidates fail closed and safer alternatives
can win by measured evidence.

## Phase 5 — Physical Integration And Readability

- Lower new primitive structures through the existing graph and design paths.
- Exercise topology-aware schematic placement for active stages, feedback,
  symmetric output pairs, rails, and returns.
- Require connectivity, routing, writer, ERC/DRC, and round-trip evidence.

Exit: simulation-selected candidates become readable reproducible KiCad
projects without fixture coordinates.

## Phase 6 — Held-Out Promotion

- Run all three requirements twice from clean roots.
- Require multiple topologies, ranked selection, complete trusted evidence,
  and identical project/evidence hashes.
- Record a machine-readable promotion matrix and clause-by-clause audit.

Exit: Class A, Class AB, and non-amplifier demonstrations satisfy the complete
specification.

## Phase 7 — Preservation, Documentation, And Review

- Run the authoritative bounded suite and promoted fixture lanes locally.
- Update roadmap, project status, AI readiness/generation, and CLI docs.
- Scan production code for held-out leakage.
- Review the staged diff with Prism, remediate actionable findings, and commit.

Exit: the full goal is locally proven, documented, reviewed, and committed.
