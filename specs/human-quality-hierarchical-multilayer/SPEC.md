# Human-Quality Hierarchical Schematics And Dense Multi-Layer PCB Specification

## Purpose

KiCadAI must turn behavior-only, multi-stage electrical requirements into a
project that is both electrically proven and conventionally reviewable by a
human engineer. The promoted project must use functional schematic hierarchy
and a fabrication-ready four-copper-layer PCB when the synthesized design has
multiple functional stages and benefits from plane-backed routing.

This milestone extends the existing open-topology, schematic-layout, and
physical-promotion pipelines. It does not introduce named topology templates,
fixture identities, coordinates, routes, or block-family shortcuts.

## Frozen Evaluation Corpus

The immutable manifest is
`internal/opentopologysynthesis/testdata/human_quality_physical_corpus/manifest.json`.
It selects four independently frozen behavior-only requirements at commit
`47f7a018adb292766f0b2dcf324dc5da40df051e`:

1. mixed-signal sensing and control;
2. feedback-regulated power;
3. bounded audio amplification;
4. protected inductive-load control.

The selected requirement files contain only electrical domains, ports,
operating cases, behavioral assertions, size/component budgets, and acceptance
gates. The physical quality contract lives in the corpus manifest and test
harness, never in the behavior input.

## Schematic Requirements

Every accepted design must:

- partition into at least two functional child sheets;
- derive sheet membership from graph structure, functional groups, electrical
  roles, and coupling rather than reference designators or corpus identity;
- preserve multi-unit component ownership on one sheet;
- expose every cross-sheet signal through explicit hierarchical interfaces;
- use global power symbols only for genuinely global power and reference nets;
- show conventional left-to-right signal flow within each sheet;
- keep feedback, bias, protection, decoupling, and driven devices visually
  local to the stage they serve;
- pass strict readability with zero body, field, label, wire, or sheet-object
  overlaps, zero diagonal wires, and zero accidental crossings;
- preserve exact flattened endpoint connectivity across the hierarchy; and
- write, parse, and round-trip every root and child schematic with zero diffs.

Hierarchy is required for this corpus even when a flat schematic could fit on
one large sheet. The purpose is to prove semantic functional partitioning, not
only overflow recovery.

## PCB Requirements

Every accepted design must:

- contain exactly four copper layers: `F.Cu`, `In1.Cu`, `In2.Cu`, and `B.Cu`;
- retain a continuous ground reference plane on `In1.Cu` except for bounded,
  reported antipads and unavoidable clearance;
- use `In2.Cu` for bounded power-plane regions or power distribution where the
  derived domains permit it;
- group input/sensing, control, power/output, protection, and connector
  functions into deterministic placement regions;
- place thermally significant power devices using catalog package, thermal
  path, airflow, and board-edge evidence rather than component identity;
- assign signal layers and via transitions from congestion, net role, return
  plane continuity, endpoint access, and clearance;
- prohibit signal-layer transitions that cross a reference-plane split without
  an explicit nearby return transition;
- retain a declared return net and maximum return-path distance for sensitive,
  feedback, switching, and high-current paths;
- route every required endpoint, fill required zones, and report all vias,
  layer transitions, plane assignments, and return-path evidence; and
- pass connectivity, writer correctness, ERC, strict DRC, and zero-diff
  round-trip checks in two deterministic runs.

## Deterministic Derivation

The implementation may use only normalized requirement data, synthesized graph
semantics, catalog evidence, package geometry, board constraints, and generic
fabrication/routing policy. Stable ordering and content hashes resolve equal
scores. Equivalent normalized inputs must produce byte-identical hierarchy,
stackup, placement, routing, and evidence.

## Fail-Closed Rules

Physical promotion must fail without a KiCad project when any required proof is
missing, including incomplete hierarchy interfaces, ambiguous multi-unit sheet
ownership, insufficient board area, unsupported four-layer stackup, broken
reference-plane continuity, unbounded thermal placement, unrouted endpoints,
unfilled required zones, ERC/DRC errors, writer defects, or round-trip changes.

Increasing a page or board size, using labels, changing signal layer, inserting
a legal via transition, moving an inferred placement, and selecting a compatible
thermal region are admissible physical repairs. Changing electrical topology,
component values, pin mappings, or behavior is not.

## Evidence Contract

Each promotion report must bind hashes for the normalized requirement,
inventory, selected graph and values, hierarchy plan, schematic files, board
stackup, plane/zone plan, placement, routing, KiCad projects, and validation
reports. It must also report:

- sheet count, functional partition rationale, interfaces, and cross-sheet nets;
- per-sheet readability metrics;
- copper layers, plane coverage, splits, and zone-fill status;
- functional placement regions and thermal-placement decisions;
- per-net layer usage, vias, return net, and worst return-path distance;
- routing completion and connectivity;
- ERC, strict DRC, writer, round-trip, and replay results; and
- precise blocking diagnostics for any failed case.

## Completion Criteria

The milestone is complete only when all four frozen designs pass twice through
the public behavior-to-KiCad workflow with deterministic evidence and satisfy
every schematic and PCB requirement above. All previously promoted corpora and
installed-KiCad fixtures must remain green. The full local test suite and Prism
review are required before the final commit.
