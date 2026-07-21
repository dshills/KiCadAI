# Generic MCU Subsystem Synthesis Implementation Plan

Date: 2026-07-21

## Completion Status

All seven phases are implemented for the initial verified three-target
envelope. The work was delivered as one coherent change because catalog schema,
selection, lowering, physical realization, and regenerated provenance hashes
must remain synchronized. The requirement-to-test mapping, installed-KiCad
evidence, hashes, and remaining boundaries are recorded in [AUDIT.md](AUDIT.md).

## Implementation Rules

- Keep target facts in verified catalog data and generic schema. Never switch
  on project, fixture, component, manufacturer, or MCU-family identity.
- Preserve current ATmega328P and ESP32 generated-board evidence throughout.
- Use the architecture-search catalog provider as the behavior-level entry
  point and emit resolved physical functions for all downstream stages.
- Keep the solver deterministic under shuffled candidates, pins, alternates,
  roles, and constraints.
- Add stable issue codes before adding prose diagnostics.
- Keep default tests hermetic. Installed-KiCad checks are optional locally but
  mandatory acceptance evidence for promoted fixtures.
- Use `atlas` for structural code questions and summarize large files before
  reading them.
- Review each staged phase with Prism and commit coherent phases independently.
- Run focused tests after each change and `go test ./...` before every promoted
  phase and final closeout.

## Phase 1: Contract And Third-Target Evidence

Objective: define the generic evidence model and add the third verified family.

Tasks:

- Add MCU capability, physical-pin, alternate-function, programming, clock,
  boot, and current-budget schema.
- Add normalization and catalog validation, including symbol-function and
  package-pad consistency.
- Upgrade ATmega328P-A and ESP32-WROOM-32E records to the new schema without
  changing their existing generated projects.
- Add the STM32G031K8T6 LQFP-32 verified record and source evidence.
- Add shuffled-input normalization and invalid-record tests.
- Document any conservative unmodeled electrical limits as explicit evidence
  gaps rather than inferred values.

Gates:

- `go test ./internal/components ./internal/pinmap`
- `go test ./...`
- `prism review staged`
- Commit: `Add generic MCU catalog evidence`

## Phase 2: Deterministic Selection And Pin Solver

Objective: select feasible MCUs and assign complete interface bundles.

Tasks:

- Normalize behavior ports and interface constraints into MCU role demands.
- Implement a deterministic, bounded backtracking solver over physical pins and
  peripheral-instance bundles.
- Support GPIO, UART, I2C, SPI, PWM, ADC, interrupt, and programming roles.
- Reserve supply, reset, boot, programming, and selected clock pins.
- Rank candidates and assignments using the specification's generic ordering.
- Persist chosen instance, signal, physical function, pad, evidence, and stable
  rejected-alternative codes.
- Replace the programmable-controller single-I2C/GPIO shortcut in the catalog
  provider with the solver.
- Add shuffled-input, conflict, exhaustion, and bundle-consistency tests.

Gates:

- `go test ./internal/architecturesearch ./internal/circuitgraph`
- `go test ./...`
- `prism review staged`
- Commit: `Resolve MCU peripherals deterministically`

## Phase 3: Electrical, Boot, Clock, And Loading Validation

Objective: reject electrically impossible assignments before realization.

Tasks:

- Validate supply and I/O voltage domains and logic thresholds.
- Validate boot-strap loads and reset-time states.
- Select and validate internal or external clocks from request policy.
- Track per-pin source/sink current and aggregate MCU I/O/supply demand.
- Validate modeled I2C pull-up/capacitance, UART/SPI fanout and speed, ADC source
  impedance/settling, and programming-interface loading.
- Add stable issue codes, requirement paths, required/offered values, and
  margins.
- Ensure validation results participate in candidate feasibility and do not
  disappear after selection.

Gates:

- `go test ./internal/architecturesearch ./internal/electricalrules`
- `go test ./...`
- `prism review staged`
- Commit: `Validate MCU assignments electrically`

## Phase 4: Catalog-Derived MCU Support Networks

Objective: construct complete MCU subsystems from selected catalog policy.

Tasks:

- Generalize companion expansion for per-domain decoupling and bulk
  capacitance, reset, boot bias, programming/debug, and optional clock parts.
- Derive I2C pull-ups and supported level transitions from validated bus and
  voltage evidence.
- Add request-driven external connectors and protection networks.
- Make expansion conditional and idempotent.
- Carry placement/routing constraints by semantic role without coordinates.
- Retire device-specific support-network choices from the active generic path;
  retain legacy blocks only as compatibility callers until fixtures migrate.
- Add target-independent subsystem graph tests and target-specific catalog
  expectation tests.

Gates:

- `go test ./internal/components ./internal/circuitgraph ./internal/architecturesearch ./internal/blocks`
- `go test ./...`
- `prism review staged`
- Commit: `Synthesize catalog-driven MCU support networks`

## Phase 5: Behavioral Clarifications And Neutral Corpus

Objective: prove that behavior, not a named fixture, drives MCU synthesis.

Tasks:

- Map unresolved MCU choices to stable behavioral clarifications.
- Map infeasible selection and assignment to stable capability gaps with
  deterministic rejected-candidate evidence.
- Replace the old programmable-controller simulation gap for now-supported
  behaviors while retaining unsupported simulation claims where appropriate.
- Freeze neutral cases that indirectly exercise all three targets through
  capability and electrical requirements.
- Include unfamiliar multi-peripheral cases, alternative prompt phrasings,
  impossible conflicts, insufficient current, boot conflicts, and unsupported
  programming modes.
- Prove manifest integrity and byte-identical replay under shuffled catalog and
  role ordering.

Gates:

- `go test ./internal/behavioralintent ./internal/architecturesearch ./internal/intentplanner`
- corpus integrity and replay commands documented by the repository
- `go test ./...`
- `prism review staged`
- Commit: `Add neutral MCU synthesis corpus`

## Phase 6: Three-Target KiCad Promotion

Objective: produce reproducible KiCad-backed boards for every initial target.

Tasks:

- Add or migrate one neutral generated fixture per target.
- Ensure downstream graph lowering consumes resolved physical functions without
  reassigning pins.
- Tune only generic placement, endpoint-access, branch-order, routing, or
  layer-transition policy when failures occur.
- Run each optional installed-KiCad fixture after every correction.
- Require clean ERC, strict DRC, complete connectivity and routing, writer
  correctness, zero round-trip differences, and deterministic replay.
- Re-run existing ATmega, ESP32, USB-C I2C sensor, LED indicator, and amplifier
  regression evidence.
- Record tool versions, commands, and artifact hashes in the closeout report.

Gates:

- all focused and full Go tests
- all repository deterministic replay and round-trip gates
- installed KiCad ERC and strict DRC for all three targets
- `prism review staged`
- Commit: `Promote generic MCU KiCad fixtures`

## Phase 7: Documentation, Audit, Push, And CI

Objective: close the work with reproducible evidence and honest capability
boundaries.

Tasks:

- Update architecture, catalog authoring, supported-components, testing,
  generation, and roadmap documentation.
- Add a requirement-by-requirement audit mapping acceptance clauses to tests,
  generated artifacts, and remaining limitations.
- Confirm there are no target-name dispatches, fixture-specific coordinates,
  allowlists, schemas, or hard-coded circuit families in the implementation.
- Confirm the worktree contains no generated temporary files.
- Run the complete local gate and review all staged changes with Prism.
- Commit, push, and verify GitHub Actions to completion.

Gates:

- `go test ./...`
- repository documentation and deterministic-artifact checks
- `prism review staged`
- clean worktree after commit
- successful push and GitHub Actions
- Commit: `Document generic MCU subsystem synthesis`
