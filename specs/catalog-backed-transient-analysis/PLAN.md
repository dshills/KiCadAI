# Catalog-Backed Transient Analysis Plan

## Phase 1: contracts and registry

- Add a separate transient workflow, analysis kind, observation-grid fields,
  bounded pulse fields, time/edge assertions, and report evidence.
- Add a transient-only capacitor primitive with catalog voltage limit.
- Keep schema strict and reject provider equations, models, topology, and
  solver controls.

## Phase 2: solver

- Compile the reviewed nonlinear devices and transient capacitors.
- Solve the deterministic `t=0` DC state.
- Integrate capacitor state with fixed backward Euler and bounded Newton work.
- Check catalog operating limits at every accepted point and emit actionable
  point-specific convergence diagnostics.

## Phase 3: focused proof

- Test RC state evolution, diode and NPN/PNP behavior, time and 10–90% edge
  assertions, bounds, incompatible claims, operating-limit failures,
  convergence diagnostics, tamper rejection, and byte-identical replay.
- Test strict schema rejection of provider-controlled models, equations,
  topology, and solver configuration.

## Phase 4: held-out KiCad promotion

- Add a flat generic transistor switching fixture with a pulse input, catalog
  capacitor, reviewed BJT, and waveform/edge assertions.
- Run the optional KiCad-backed fixture after each correction until ERC, strict
  DRC, connectivity, route completion, simulation, writer correctness,
  round-trip, and replay all pass.

## Phase 5: preservation and release

- Run focused and full Go tests plus formatting/lint/coverage gates.
- Re-run linear MNA, nonlinear DC, USB-C I2C, and protected LED evidence.
- Update status/roadmap documentation, review staged changes with Prism,
  commit, push, and verify GitHub Actions.
