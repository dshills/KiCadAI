# Implementation Plan

## Phase 1: Retry Diagnostics (superseded)

- Capture route-tree incomplete groups, endpoint counts, graph components, and
  blocking findings in retry attempt selection.
- Add regression coverage proving final-state selection rejects a retry that
  replaces one incomplete required net with another.
- Add an unsatisfiable fixture case proving bounded retries fail closed without
  emitting a replacement project or accepting degraded evidence.

## Phase 2: Coordinated Mobility (superseded)

- Let existing authorized route-tree repair hints nominate the minimal movable
  placement groups rather than only individual free components, using a
  configured candidate budget with a reported default of four.
- Transform only fully group-relative local routes with an authorized placement
  group. Keep boundary routes to fixed components unchanged unless a supported
  deterministic rebuild is available; preserve all unrelated fixed copper and
  protected power constraints.
- Test deterministic candidate generation and repeat-run identity.

## Phase 3: Route-Graph Selection (superseded)

- Rank complete candidate transactions lexicographically by required-net group
  completion, proven endpoints, graph components, blocking findings, current
  route score represented as scaled integers, then stable IDs.
- Accept only a final state that improves the ranking without regressing a
  previously complete required net.
- Add focused USB-C/I2C fixture regression coverage.

## Phase 4: Promotion

- Run the target fixture, writer checks, round trip, and optional KiCad ERC/DRC.
- Restore `pass` metadata and evidence docs only after all gates pass.
- Run full tests, lint, coverage, Prism, and CI.

## Outcome

Physical duplicate-pad endpoint preservation and deterministic wide-board flow
resolved the VCC_3v3 route-tree defect without retrying placement. Promotion
evidence is recorded in the checked-in fixture metadata and example guide.
