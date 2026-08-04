# Generic Protected Voltage-Output Synthesis Plan

Status: Phase 0 in progress.

## Phase 0: Freeze and baseline

- Freeze three independently authored behavior-only requirements: adjustable
  low-noise, protected high-power, and bidirectional midrail regulation.
- Bind requirements and manifest to the pre-implementation commit with
  SHA-256 checksums.
- Prove strict schema validity, behavioral independence, deterministic replay,
  and absence of implementation detail.
- Capture and checksum the untouched synthesis result, deepest stage,
  resource consumption, and structured diagnoses for every case.

Exit: frozen corpus and baseline tests pass without production changes.

## Phase 1: Relationship and stage diagnosis

- Trace fixed and adjustable voltage relationships, bidirectional output
  semantics, topology scoring, value derivation, simulation resolution,
  repair, and physical lowering.
- Classify each baseline failure by earliest responsible stage.
- Add focused characterization tests for command transfer, midpoint behavior,
  dropout, current limiting, fault events, compensation, noise, thermal, and
  SOA evidence.

Exit: every missing capability has a structured generic contract and an
authoritative failing test.

## Phase 2: Generic voltage-control architectures

- Generalize voltage relationships across fixed, command-dependent, and
  supply-fraction outputs.
- Generate materially different reviewed control/pass-path alternatives when
  the inventory and requirement permit them.
- Derive unidirectional and bidirectional power paths from port/domain/load
  semantics.
- Compose startup permission and fault-dominant current limiting without a
  named regulator block.

Exit: every frozen case produces graph-valid candidates with correct external
direction, regulation, and protection relationships.

## Phase 3: Value, dropout, stability, and noise closure

- Derive references, feedback ratios, compensation, output support, current
  limits, and sharing values from behavioral bounds and catalog tolerances.
- Evaluate dropout across supply, output, load, tolerance, and temperature
  corners.
- Require trusted phase-margin, settling, and integrated-noise evidence.
- Fail closed when no reviewed value/model combination covers the complete
  envelope.

Exit: electrically valid candidates pass every declared steady-state and
dynamic assertion; targeted perturbations fail with stable diagnoses.

## Phase 4: Thermal, SOA, and fault closure

- Calculate steady-state and transient dissipation for normal, startup,
  overload, and short-circuit operation.
- Derive safe pass-path multiplicity/current sharing only from measured demand
  and reviewed evidence.
- Prove current limiting, shutdown/recovery behavior, junction temperature,
  and SOA at every critical corner.

Exit: safe candidates pass measured protection evidence; unsafe or incomplete
evidence remains blocking and produces no physical artifact.

## Phase 5: Deterministic repair and stage re-entry

- Add generic transfer-ratio, reference, dropout, compensation, current-limit,
  bidirectional-drive, rating, and model repairs.
- Bind causal perturbations and affected-scope stage re-entry to repair traces.
- Verify rollback, unrelated-branch preservation, work budgets, and
  byte-identical replay.

Exit: at least one frozen case selects a real generic repair, with preserved
unrelated graph and physical state.

## Phase 6: Readable lowering and physical realization

- Lower power, feedback, reference, compensation, protection, and midpoint
  relationships into conventional schematic groups and ordering.
- Derive placement constraints for high-current loops, sense paths, feedback,
  decoupling, thermal spacing, and bidirectional output flow.
- Route through affected-scope retries while preserving unrelated placement
  and copper.

Exit: readability, electrical validation, placement, routing, connectivity,
writer, ERC, DRC, and round-trip gates pass offline.

## Phase 7: Promotion and preservation

- Run every valid frozen case twice in isolated local installed-KiCad roots.
- Require identical synthesis, topology, physical, project, and normalized
  evidence hashes.
- Run protected-current, 8/8 multi-branch, both neutral physical, architecture,
  dynamic, amplifier, protected LED, and protected I2C preservation lanes.

Exit: all goal and preservation evidence is current and reproducible locally.

## Phase 8: Closeout

- Produce a requirement-by-requirement completion audit and promotion matrix.
- Update README, project status, AI readiness, and roadmap without broadening
  claims beyond the measured linear-regulation envelope.
- Run Prism on the staged diff, remediate actionable findings, commit, and
  push.

Exit: documentation matches measured capability and no required work remains.
