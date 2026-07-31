# Topology-Aware Readable Schematic Specification

## Purpose

KiCadAI must generate schematics that communicate circuit operation to a human,
not merely satisfy KiCad connectivity.  The generator must derive the drawing
from the circuit graph and pin roles.  It must not recognize fixture names,
component coordinates, or named circuit templates.

The motivating failure is a valid current-source schematic in which local nets
were replaced by many label stubs, every synthesized primitive was forced into
one rank, and an A2 sheet contained a small dispersed circuit.  That output is a
generator defect.  KiCad rendered the objects it was given correctly.

## Topology Model

Before placement, the generator builds a component/net hypergraph and classifies
the following generic regions:

- boundary input and output interfaces;
- forward signal or controlled-current paths;
- controllers and drivers;
- feedback and sense return paths;
- bias and reference branches;
- protection branches;
- positive, negative, ground, and return supply trees.

Classification uses explicit semantic roles first, then pin direction, boundary
reachability, graph rank, strongly connected components, and deterministic
degree/identifier tie breaking.  Names may refine an existing classification
but may not select a layout template.

## Drawing Conventions

1. Forward signal flow is left to right.
2. Positive supply paths enter from above; ground, return, and negative supply
   paths leave below.
3. A feedback or sense path returns below or around the forward path and remains
   visibly traceable to the controller input.
4. Boundary connectors occupy the left or right edge of the occupied drawing.
5. Protection remains near the protected boundary, bias/reference parts remain
   near their controlled stage, and ERC power flags remain near their rail
   entry.
6. Main-path passive components are horizontal. Supply, shunt, bias, and sense
   branches are vertical when that makes the current or voltage path clearer.
7. The smallest standard KiCad sheet that contains the drawing with normal
   margins is selected for inferred layouts.

For a current sink, the resulting drawing must visibly expose the chain
`positive rail -> load -> pass device -> sense element -> return`, the control
input, the drive connection, and the sense feedback connection.  This is an
acceptance example, not a current-source-specific algorithm.

## Wire And Label Policy

- A local two-endpoint net is a continuous orthogonal wire by default.
- A required local net name annotates the continuous conductor once; it does not
  replace the conductor with two disconnected-looking stubs.
- A local multi-endpoint net is a deterministic route tree with visible
  junctions at real branch points.
- Endpoint-only labels are reserved for global rails, cross-sheet connections,
  buses, or a route for which bounded obstacle search proves that no clean
  conductor exists.
- Crossing group boundaries or exceeding an arbitrary Manhattan-distance
  threshold is not, by itself, sufficient reason to hide a local net.
- No wire may cross an unrelated symbol, pin, label, or different-net wire.
- All wire segments are orthogonal and on the KiCad connection grid.

## Readability Evidence

The layout report must include:

- local two-point nets and the number drawn continuously;
- local multi-point nets and the number drawn as route trees;
- endpoint-only label count and net-annotation count;
- branch-junction count;
- wire crossing and diagonal counts;
- occupied drawing area and usable page area;
- occupied-page ratio and whitespace ratio;
- component dispersion relative to occupied bounds;
- boundary-connector placement violations;
- feedback paths inferred and visibly wired;
- selected paper and page escalation or compaction.

Strict generated-layout acceptance requires:

- 100% of routable local two-point nets are continuously wired unless a
  `route_label_fallback` diagnostic records a proven obstacle failure;
- 100% of routable local multi-point nets are connected by one geometric tree
  unless they are global, bus, or cross-sheet nets;
- zero diagonal wires, different-net contacts, unrelated-pin contacts, and
  symbol/wire overlaps;
- zero hidden inferred feedback paths;
- zero boundary-connector placement violations;
- an occupied-page ratio of at least 0.08 for drawings with six or more
  components, unless fixed coordinates or hierarchy constraints prevent sheet
  compaction;
- deterministic normalized output for permuted equivalent input.

Metrics are evidence, not permission to alter electrical connectivity.

## Fixture Matrix

The locally rendered corpus covers:

1. controlled current source;
2. op-amp gain/buffer stage;
3. passive or active filter;
4. linear regulator with decoupling;
5. Class-A amplifier stage;
6. Class-AB amplifier stage.

Each fixture must be written as a KiCad schematic and rendered by installed
KiCad.  Automated checks inspect the generated file and the readability report;
the rendered artifact is retained for visual review.

## Compatibility Gates

Every changed fixture must preserve:

- schematic IR and transaction validation;
- generated connectivity;
- route completion;
- ERC;
- writer correctness;
- deterministic replay;
- zero-diff read/write round trip;
- existing PCB, strict DRC, and fabrication-candidate evidence where applicable.

No fixture-specific coordinates, allowlists, schema variants, or block-family
layout branches are permitted.
