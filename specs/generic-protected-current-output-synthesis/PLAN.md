# Generic Protected Current-Output Synthesis Plan

Status: completed locally on 2026-08-03. See `AUDIT.md` for the
requirement-by-requirement evidence and retained promotion hashes.

## Phase 0: Freeze and baseline

- Freeze the existing failure and two neutral independently authored variants.
- Bind the corpus, requirements, and untouched synthesis result by SHA-256.
- Prove strict schema validity, source/sink coverage, deterministic replay, and
  absence of implementation detail.
- Record the deepest stage and diagnostic categories reached by each case.

Exit: the frozen corpus and baseline tests pass without production changes.

## Phase 1: Electrical diagnosis and contracts

- Trace relationship seeding, topology scoring, value-domain derivation,
  simulation harness resolution, and diagnosis/repair behavior.
- Add focused characterization tests for source/sink polarity, compliance
  headroom, independent controls, transfer scaling, thermal, and SOA evidence.
- Define structured diagnoses for every missing contract before implementing
  repair.

Exit: each baseline failure is attributable to an explicit stage and generic
missing contract.

## Phase 2: Bidirectional regulated-current architectures

- Generalize transconductance relationship synthesis into high-side source and
  low-side sink orientations derived from port/domain semantics.
- Generate reviewed controller/pass-device alternatives with explicit sense,
  drive, compensation, and output paths.
- Compose independent startup permission and fault-dominant shutdown without
  embedding a named block.

Exit: all frozen cases produce multiple graph-valid candidate topologies with
the required direction and controls.

## Phase 3: Value, compliance, thermal, and SOA closure

- Derive sense values from transconductance bounds and catalog tolerances.
- Derive controller/pass-device headroom across supply and load corners.
- Calculate worst-case dissipation and thermal/SOA margins for steady state,
  startup, overload, and shutdown intervals.
- Fail closed when no reviewed part/value combination covers the envelope.

Exit: valid candidates pass every trusted electrical and safety assertion;
unsafe perturbations fail with stable diagnoses.

## Phase 4: Deterministic repair and stage re-entry

- Add generic polarity, sense-ratio, compliance, control-composition,
  compensation, and rating repairs.
- Bind causal perturbations and affected-scope stage re-entry to repair traces.
- Verify rollback, unrelated-branch preservation, budget accounting, and
  byte-identical replay.

Exit: at least one frozen case selects a real generic repair and all repair
evidence validates.

## Phase 5: Readable lowering and physical realization

- Lower the command/control, feedback, pass, sense, protection, and output
  relationships into conventional schematic groups and ordering.
- Add deterministic placement constraints for the high-current loop, sense
  path, controller feedback, protection, thermal spacing, and decoupling.
- Route with affected-scope retries while preserving unrelated content.

Exit: schematic readability, electrical validation, placement, routing,
connectivity, writer, and round-trip gates pass offline.

## Phase 6: Promotion and preservation

- Run each valid frozen case twice in isolated local installed-KiCad roots.
- Require clean ERC, strict DRC, complete connectivity/routes, writer
  correctness, zero round-trip differences, identical normalized evidence,
  and identical project hashes.
- Run the 8/8 multi-branch benchmark, both neutral physical promotions,
  amplifier regressions, and protected LED/I2C preservation lanes locally.

Exit: all goal and preservation evidence is current and reproducible.

## Phase 7: Closeout

- Produce a requirement-by-requirement completion audit and promotion matrix.
- Update README, project status, AI readiness, and roadmap without broadening
  claims beyond the measured envelope.
- Run Prism on the staged diff, remediate actionable findings, and commit.

Exit: documentation matches measured capability and no required work remains.
