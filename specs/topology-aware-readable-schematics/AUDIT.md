# Baseline Audit

## Observed Current-Source Output

The reviewed KiCad image uses an A2 landscape sheet but occupies only a narrow
central column.  The functional current path, drive path, and feedback path are
not visibly traceable.  Most local nets appear as short stubs carrying labels
such as `DAC_SET_RAW`, `M1_HALF`, `M1_DC`, and `M2_SENSE`.

## Root Causes

1. `physicalSchematicIntent` puts every synthesized primitive in one
   non-inferred group at rank 1 and adds an arbitrary `near` chain in instance
   order.  This disables graph-derived left-to-right placement.
2. `shouldUseLabels` replaces a net with endpoint labels when it is explicitly
   preferred, has more than two endpoints, is power-like, crosses groups, or is
   longer than a threshold.
3. Schematic IR currently prefers labels for passive-only nets and power nets
   for ERC/naming reasons.  Routing interprets that naming requirement as a
   request to hide the conductor.
4. Multi-endpoint net fragments can arrive as separate two-endpoint layout nets,
   preventing a single visible route tree.
5. Page selection begins at the requested paper and only escalates, so it cannot
   recover from an oversized inferred paper.
6. Existing readability evidence checks collisions, diagonals, stage order, and
   power lanes but does not measure continuous local wiring, feedback
   visibility, whitespace, dispersion, or connector-edge placement.

## Conclusion

This is not a KiCad rendering flaw.  The generator emitted a legal but
label-heavy and weakly constrained drawing.  The remediation belongs in generic
topology normalization, placement, routing semantics, and readability evidence.

## Implementation Closeout

The remediation is implemented without circuit-name templates or
fixture-coordinate branches:

- synthesized instances receive graph-derived ranks and feedback roles instead
  of one forced rank and an arbitrary instance-order `near` chain;
- same-name net fragments are normalized into one hyperedge;
- visible local wiring is separate from optional conductor annotation;
- multi-endpoint nets produce compact route trees and real junctions;
- endpoint labels first exhaust the calibrated outward pin direction, then use
  collision-checked orthogonal access;
- explicit routes have a bounded deterministic grid fallback;
- rail-connected passives are vertical while signal and feedback passives are
  horizontal;
- ERC flags attach to the connector that introduces their rail;
- page selection evaluates both orientations from the smallest standard paper;
- the report records continuous nets, route trees, endpoint labels,
  annotations, junctions, feedback visibility, boundary placement, page area,
  whitespace, and dispersion.

Installed-KiCad and rendered evidence is recorded in
`specs/topology-aware-readable-schematics/EVIDENCE.md`.
