# Verified Audio Amplifier Domain Implementation Plan

## Implementation Status

Phases 1 through 7 are implemented. Reproducible positive, unsafe, component,
layout, and seven-analysis evidence is frozen in `CAPABILITY_REPORT.json` and
its SHA-256 sidecar. The Class A and protected Class AB fixtures are declared
KiCad-backed passes and are exercised by the optional real-KiCad regression
tier; closeout commands and CI results remain the final release evidence.

## Phase 1: Freeze Baseline And Contracts

- Capture existing catalog, block, simulation, layout, readiness, and protected
  amplifier promotion status.
- Add the domain specification, corpus manifest schema, stable diagnostic
  taxonomy, and explicit promotion gate profile.
- Add failing contract tests for missing or inconsistent typed evidence.

Exit: the repository has an executable checklist for every objective
requirement, and current gaps are recorded without weakening acceptance.

## Phase 2: Component Evidence And Selection

- Extend catalog schemas for quantitative op-amp, capacitor, power-BJT, and
  power-MOSFET evidence.
- Validate values, units, provenance, complementary pairing, pinmaps, SOA
  ordering, thermal consistency, and acceptance-level completeness.
- Add reviewed concrete seed families and fail-closed selectors.
- Add adversarial catalog records/tests for every evidence class.

Exit: fabrication-oriented selection succeeds only for devices whose typed
evidence covers the requested operating envelope.

## Phase 3: Generic Amplifier Topologies

- Implement `class_a_voltage_stage` with BJT and MOSFET device contracts,
  calculated bias/degeneration/load/coupling networks, and explicit limits.
- Refactor Class AB output selection to use the catalog pair selector and typed
  bias/load evidence.
- Add VBE-multiplier support and calculated quiescent-current policy.
- Add deterministic composition helpers for supply decoupling, DC blocking,
  feedback, output damping, and protection.

Exit: positive milestone and unsafe requests lower from semantic parameters
without fixed component IDs, references, coordinates, or transaction scripts.

## Phase 4: Amplifier Layout Policy

- Add typed return-topology, current-loop, feedback-sense, Kelvin,
  decoupling, thermal-coupling, symmetry, keepout, and orientation constraints.
- Lower them into generic placement, routing, and physical-rule structures.
- Extend evidence evaluation and repair hints with stable categories.
- Add geometry-neutral unit and adversarial tests.

Exit: both milestone boards produce complete amplifier PCB evidence and unsafe
layout variants fail before promotion.

## Phase 5: Electrical, Stability, Thermal, And SOA Validation

- Register trusted device primitives and bounded models required by the two
  milestones.
- Extend reports for dissipation, loop stability, thermal paths, junction
  temperature, and interpolated SOA margin.
- Integrate registered tolerance and temperature corners.
- Add deterministic DC, AC, transient, tolerance, stability, thermal, and SOA
  assertions plus adversarial failures.

Exit: all required analyses are deterministic, catalog-backed, provider-safe,
and independently regression tested.

## Phase 6: Milestone Fixture Promotion

- Generate the low-power Class A BJT line preamplifier.
- Generate the protected BJT Class AB headphone amplifier.
- Run each through catalog resolution, schematic/PCB generation, amplifier
  constraints, simulation, routing/connectivity, clean ERC/strict DRC, writer
  correctness, fabrication checks, zero round-trip diffs, and replay.
- Fix only generic blockers and rerun the optional KiCad-backed fixtures after
  each correction.
- Freeze capability and adversarial reports with hash sidecars.

Exit: both positive fixtures pass all gates and every required unsafe variant
fails with the expected diagnostic.

## Phase 7: Closeout

- Update the amplifier AI readiness matrix and project status from evidence.
- Run focused tests, `go test ./...`, coverage, all protected regressions, and
  both optional KiCad-backed amplifier fixtures.
- Review staged changes with Prism and resolve all high/medium findings.
- Commit coherent phases, push, and verify GitHub Actions.

Exit: the specification's definition of complete is proven requirement by
requirement from current repository and CI evidence.
