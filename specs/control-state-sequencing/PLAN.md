# Control-State and Sequencing Implementation Plan

## Phase 0 — Freeze the Gap

- Preserve the historical V3 simulation-grounded fixtures byte-for-byte.
- Add a separate identity-neutral V6 corpus with raw hashes and truth tables.
- Record the two strict transient non-responses as the initial failing cases.

## Phase 1 — Versioned Contract

- Add V6 schema, version, and policy identifiers.
- Add typed control semantics, directed transitions, timing bounds, and state
  dependencies.
- Normalize every added field and collection deterministically.

## Phase 2 — Fail-Closed Validation

- Validate the closed vocabularies and logical/physical direction agreement.
- Require V6 response behaviors to link to declared transitions.
- Require every transition to be measured.
- Reject unsupported asserted-startup proof and missing semantic bindings with
  stable paths and messages.

## Phase 3 — Architecture Projection

- Project polarity, startup, and safe state into generic provider constraints.
- Select threshold and switch orientation from semantic constraints.
- Keep V3 legacy selection behavior unchanged when no explicit V6 semantics
  exist.

## Phase 4 — Lowering and Planning

- Carry V6 through architecture search, closed-loop synthesis, and composition
  lowering without enabling V4 hierarchy or V5 dynamic features.
- Carry transition identity, direction, delay, and prerequisites into the
  analysis plan.
- Resolve prerequisite observations to concrete trusted targets or fail closed.

## Phase 5 — Directed Simulation

- Add a response-direction assertion field.
- Use the declared direction for windowed and non-windowed response timing.
- Reject opposite-direction glitches as non-responses.
- Retain legacy terminal-value direction inference outside V6.

## Phase 6 — Bounded Generic Repair

- Reuse preferred-value policy for threshold feedback and sequencing timing.
- Add a bounded high-side inverter bias repair variable.
- Keep polarity and state meanings immutable while allowing deterministic
  topology orientation.

## Phase 7 — Named Failure Closure

- Express both previously failing cases as V6 requirements.
- Attempt a real active-high fault-driven high-side disconnect path.
- Reject both requirements before synthesis when their low-output startup proof
  contradicts the sole fault control's declared connected startup state.
- Require a separate startup enable or sequencing dependency instead of
  deleting the safety claim or accepting unpowered motion.

## Phase 8 — Preservation and Promotion

- Run targeted schema, projection, provider, planner, solver, and corpus tests.
- Run the complete local test suite.
- Run the installed-KiCad promotion lane twice from the same committed inputs
  and compare deterministic evidence.

## Phase 9 — Documentation and Audit

- Update README, project status, AI readiness, and roadmap claims only to the
  demonstrated envelope.
- Record exact commands, results, hashes, and remaining limitations in
  `AUDIT.md`.
