# Generic Multi-Control Safety Composition Plan

## Phase 0 — Writer prerequisite

- Reproduce the active-filter KiCad ERC failure in an isolated promotion shard.
- Map reported wire UUIDs to generated transaction connect operations.
- Correct the generic label-free direct-route fallback.
- Prove clean installed-KiCad ERC, strict DRC, and deterministic generation.

## Phase 1 — Freeze semantic evidence

- Replace the two one-control rejection overlays with explicit enable/fault or
  power-good/inhibit contracts and a typed permit signal.
- Freeze normalized truth-table cases across input polarities.
- Add stable diagnostics for missing roles, contradictory startup, priority,
  and dependency cycles.

## Phase 2 — Architecture projection and validation

- Project control constraints by semantic binding role so independent controls
  cannot overwrite one another.
- Derive safety dominance and startup invariants from control functions.
- Validate the coordinator-to-switch chain and transition dependencies.

## Phase 3 — Provider and lowering

- Add a generic safety-coordinator provider realization.
- Normalize active-high and active-low inputs with catalog-backed stages.
- Emit a default-off, protection-dominant permit and bind it to the load switch.
- Carry component/model provenance and bounded bias repair variables through
  realization and lowering.

## Phase 4 — Directed simulation

- Plan independent startup, enable, fault, recovery, simultaneous-control, and
  unpowered-glitch scenarios.
- Resolve semantic controls to concrete nodes and source events.
- Require transition direction, initial-state validity, timing, dependencies,
  and powered-state evidence.

## Phase 5 — Preservation and promotion

- Run focused architecture, provider, planning, simulation, lowering, writer,
  and round-trip tests.
- Run the complete bounded local suite and long preservation lanes.
- Run the full simulation-grounded installed-KiCad promotion twice.
- Record hashes, commands, tool versions, and results in `AUDIT.md`.
- Stage the scoped diff and remediate actionable Prism findings.

## Completion

Phases 0 through 5 are implemented. The active-filter writer prerequisite,
directed behavioral evaluation, both promoted overlays, the preserved USB-C
sensor fixture, and two complete installed-KiCad promotion lanes pass locally.
Exact commands and the bounded full-suite exception are recorded in
`AUDIT.md`.
