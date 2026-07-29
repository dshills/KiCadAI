# Behavior-Driven MCU Power-Integrity Synthesis Plan

Status: complete on 2026-07-28. See [AUDIT.md](AUDIT.md).

## Phase 1: Freeze Evidence And Diagnostics

- Add normalized per-rail MCU power-integrity evidence.
- Validate one complete record for every normalized rail group.
- Add stable missing-evidence, domain, budget, and capacitor rejection codes.
- Add validation tests for missing, duplicate, unknown, malformed, and
  incomplete records.

## Phase 2: Qualify Concrete Capacitors

- Add or promote manufacturer-backed low-ESR capacitor evidence needed by the
  corpus.
- Implement deterministic capacitor candidate evaluation for nominal/effective
  capacitance, tolerance, ESR, ripple, voltage derating, temperature, package,
  pin map, and fabrication evidence.
- Bound rejection diagnostics and preserve deterministic ordering.
- Add positive and adversarial selector tests.

## Phase 3: Synthesize Rail Networks

- Resolve behavior overrides against reviewed MCU defaults.
- Calculate source, brownout, ripple, noise, ESR, and capacitive droop budgets.
- Emit one local capacitor per supply domain and one bulk capacitor per rail
  group.
- Connect each instance through reviewed power/ground functions.
- Attach domain-aware parameters and reviewed placement-distance constraints.
- Emit finalized calculation evidence for local and bulk networks.

## Phase 4: Held-Out Corpus

- Add neutral ESP32, STM32, and ATmega behavior-only cases.
- Add missing-evidence, missing-ESR, budget-exceeded, domain, derating, and
  temperature adversarial cases.
- Prove deterministic results under catalog and request reordering.
- Prove that fixed MCU decoupling recipes are replaced, not duplicated.

## Phase 5: Physical Promotion And Preservation

- Run the corpus through offline lowering and complete workflow validation.
- Run installed-KiCad ERC, strict DRC, routing/connectivity, writer, and
  round-trip gates.
- Re-run clock/programming, protected USB-C, ESP32, and amplifier preservation
  fixtures.
- Update roadmap, readiness, project status, and a requirement-by-requirement
  completion audit.

## Phase 6: Review

- Stage only milestone files.
- Review the exact staged diff.
- Resolve actionable findings without weakening fail-closed evidence policy.
- Commit only after the full local gate set passes.
