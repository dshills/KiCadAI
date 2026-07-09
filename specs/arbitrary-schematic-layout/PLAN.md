# Arbitrary Schematic Human-Readable Layout Implementation Plan

## Phase 1: General Graph Placement Core

- Replace fixed five-column placement with graph islands, SCC condensation,
  deterministic ranks, semantic lanes, crossing-minimization sweeps, and
  body-aware spacing.
- Center occupied bounds in the requested sheet.
- Add linear, cyclic, branching, disconnected, and permutation-stability tests.
- Acceptance: every component is placed once; output is deterministic; bodies
  remain separated and inside the usable sheet for fitting inputs.

## Phase 2: Schematic IR Integration

- Adapt IR components, pins, nets, groups, `near` constraints, orientations, and
  policy into the general layout request.
- Replace the adapter's hard-coded coordinate grid with layout results.
- Add full 0/90/180/270 orientation semantics and rotated pin/body geometry.
- Acceptance: all IR examples use the shared engine and preserve electrical
  output without fixture-specific coordinates.

## Phase 3: Routing Contract And Obstacle Avoidance

- Extend optional transaction connection metadata with forced label or explicit
  orthogonal waypoint intent.
- Add deterministic local-route candidate scoring and obstacle/crossing checks.
- Emit bounded label stubs when direct routing is unsuitable.
- Acceptance: generated local wires avoid unrelated symbols/text and arbitrary
  graph fixtures have no accidental crossings.

## Phase 4: Text, Labels, And Readability Repair

- Place reference/value fields using rotated bounds and collision-aware
  candidates.
- Place and orient labels away from pins with collision checks.
- Add a bounded repair loop for spacing, order, route, and text defects.
- Acceptance: strict readability validation is clean for all fitting fixtures.

## Phase 5: Page Fit And Hierarchical Partition

- Add paper-dimension selection and deterministic page escalation.
- Add low-coupling graph partitioning and hierarchical sheet emission for
  designs that cannot fit the largest configured page.
- Preserve multi-unit symbols and cross-sheet net semantics.
- Acceptance: oversized fixtures generate readable multi-sheet KiCad projects.

## Phase 6: Arbitrary-Topology Verification

- Add mixed analog, digital bus, power, feedback, disconnected, and multi-unit
  end-to-end fixtures.
- Add seeded property tests over generated graph topologies and input
  permutations.
- Verify KiCad parsing, internal electrical checks, readability evidence, and
  round-trip stability.
- Acceptance: no topology-specific placement code is needed by the corpus.

## Phase 7: CLI Evidence And Documentation

- Expose layout evidence and actionable failures in the schematic IR CLI path.
- Document the AI contract, supported hard constraints, page/hierarchy behavior,
  and reproducible commands.
- Update README and roadmap only after implementation evidence exists.
- Acceptance: a compiled `kicadai` binary can generate and explain a readable
  schematic from any valid checked-in or generated IR fixture.

## Review And Commit Protocol

For each phase:

1. Run focused tests while implementing.
2. Run `go test ./...` before completion.
3. Stage only phase-related files.
4. Run `prism review staged` and resolve actionable findings.
5. Commit the completed phase.
6. Re-run the next phase from the committed state.
