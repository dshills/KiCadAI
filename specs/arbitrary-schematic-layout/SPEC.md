# Arbitrary Schematic Human-Readable Layout Specification

## Purpose

KiCadAI must convert any structurally valid schematic design IR into a
deterministic KiCad schematic organized for human review. Layout must be based
on circuit topology and declared intent, not fixture names or hard-coded block
families.

"Arbitrary" means the engine accepts general circuit graphs, including cycles,
feedback, disconnected functional islands, high-fanout nets, mixed analog and
digital content, power domains, and multi-unit symbols. It does not mean every
possible circuit fits legibly on one A4 sheet. The engine must enlarge the page
or partition the design into hierarchical sheets rather than silently crowding
or clipping content.

## Required Outcomes

1. Signal paths progress left to right unless explicit layout intent fixes a
   different relation.
2. Positive supply content is above the signal path; ground, return, and
   negative supply content is below it.
3. Closely related parts remain visually local: decoupling near its powered
   device, pull-ups near their bus, feedback near its stage, and protection near
   the protected boundary.
4. Components, fields, labels, and wires do not overlap symbol bodies or each
   other beyond explicitly permitted pin contact.
5. Wires are orthogonal. Local connections use wires; long, high-fanout,
   cross-group, power, and bus connections use bounded label stubs.
6. The occupied drawing is centered in the usable page area.
7. The result is deterministic for equivalent normalized input.
8. Layout evidence explains page choice, graph ranks, label fallback, repairs,
   and any unresolved readability defect.
9. Generated output parses as current KiCad schematic syntax and passes the
   existing internal structural, electrical, and readability checks.

## Inputs

The primary input is `schematicir.Document`. Circuit intent remains authoritative
for electrical connectivity. Layout intent supplies optional constraints:

- ordered groups and side preferences;
- relative `near` relationships;
- component orientation;
- signal-flow and power-lane conventions;
- spacing, crossing avoidance, centering, and label policy;
- repair permissions.

Missing layout hints are inferred. Invalid or contradictory hard constraints
fail closed during IR validation.

## Topology Model

The engine builds a component/net hypergraph. Power, ground, return, and shield
nets are classified separately from signal-flow edges. Direction is inferred in
this order:

1. explicit group rank and port side;
2. output-to-input pin electrical direction;
3. input/output connector boundary role;
4. known source/sink component roles;
5. deterministic undirected graph fallback.

Feedback edges participate in proximity scoring but do not force forward rank.
Strongly connected components are condensed before rank assignment. Disconnected
islands are laid out independently and packed as readable regions.

## Placement

Placement has four deterministic passes:

1. Assign graph ranks from left to right.
2. Assign semantic lanes for positive power, signal, reference/bias, ground,
   and negative power.
3. Minimize edge crossings with stable barycentric sweeps while preserving hard
   rank, side, group, fixed-position, and `near` constraints.
4. Pack groups and islands using symbol-body and text extents, then center the
   occupied bounds in the usable page.

Spacing is body-aware. Rotated body and pin geometry must be used when computing
clearance and anchors. Explicit positions, when supported by the IR, are hard
constraints; inferred positions are repairable.

## Page Fit And Hierarchy

The engine first tries the requested paper. When content cannot fit at minimum
readable spacing and spacing repair is allowed, it escalates through compatible
KiCad paper sizes. If the largest configured page cannot contain the design, it
partitions at low-coupling group or graph boundaries into hierarchical sheets.
Cross-sheet nets use hierarchical or global labels according to electrical
scope. Partitioning must never split a multi-unit component or silently change
connectivity.

## Routing

The layout result controls writer routing through explicit per-connection
intent:

- direct orthogonal waypoints for short local nets;
- label stubs for long, high-fanout, power, bus, and cross-sheet nets;
- junctions only for real branch points;
- no wire through unrelated symbol or text bounds;
- no accidental connection at unrelated wire crossings.

Candidate local routes are scored for length, bends, body intersections, text
intersections, wire crossings, and group-gutter crossings. If no clean local
route exists and label insertion is allowed, the engine uses labels. Otherwise
it reports a blocking readability issue.

## Property And Label Placement

Reference and value fields are placed after symbol placement. Candidate field
locations are searched above, below, left, and right of the rotated symbol body.
Labels are oriented away from their pin and placed on a short clear stub. Text
extent estimates must account for text length, orientation, and justification.

## Repair Policy

Allowed automatic repairs include inferred ranks, crossing-minimization order,
spacing expansion, page-size escalation, label insertion, and field relocation
when the corresponding policy permits them. Electrical changes, pin guessing,
symbol substitution, and net merging are never layout repairs.

## Diagnostics And Evidence

Evidence must include:

- selected paper and usable bounds;
- occupied bounds and centering offset;
- graph island, SCC, rank, and lane counts;
- direct-wire and label-fallback counts;
- crossing and overlap counts;
- page escalation or sheet partition decisions;
- deterministic repair diagnostics;
- unresolved errors and warnings with component/net references.

Readability pass means zero errors, zero symbol/body overlaps, zero diagonal
wires, zero out-of-page objects, zero accidental wire crossings, and no
unresolved hard layout constraints.

## Compatibility

Existing transaction and design workflows remain valid. New routing fields are
optional and old transactions retain their current behavior. Existing generated
fixtures must remain electrically equivalent and pass their current checks.

## Test Requirements

Coverage must include:

- linear passive chain;
- branching/high-fanout digital bus;
- analog feedback cycle;
- mixed positive/negative supply analog stage;
- disconnected functional islands;
- multi-unit component;
- dense circuit requiring label fallback;
- circuit requiring larger paper;
- circuit requiring hierarchical partition;
- stable output under input permutation;
- generated KiCad parse, electrical, readability, and round-trip checks.

Property tests must generate valid random graph topologies and assert
determinism, complete component placement, page containment, orthogonal routing,
and preserved endpoint connectivity.

## Completion Criteria

The project is complete when the IR-to-KiCad path uses this layout engine by
default, all topology and property tests pass, all checked-in schematic IR
examples generate clean readable schematics, oversized designs use page
escalation or hierarchy, and no fixture-specific placement rule is required for
correctness.
