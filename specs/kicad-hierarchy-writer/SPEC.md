# KiCad Hierarchy Writer Specification

Date: 2026-07-16
Status: Implemented

## Purpose

Make generated hierarchical KiCad schematics loadable by KiCad, ERC-capable,
and semantically round-trippable. This unblocks a catalog-resolved,
simulation-backed generic regulated-power fixture with strict evidence.

## Baseline

`generic-circuit-v1` can request `schematic.hierarchy.mode: "auto"` and a
component-per-sheet limit. KiCadAI emitted a root schematic and child sheets, but
KiCad 10 rejected the root during `kicad-cli sch upgrade`; child sheets also
produce normalized round-trip diffs. Flat generated fixtures are clean.

This is a writer defect, not a KiCad defect and not a fixture-specific routing
problem.

## Goals

1. Emit complete, KiCad-loadable root/child hierarchy instance data.
2. Preserve deterministic child-sheet identity, filenames, references, units,
   labels, buses, and cross-sheet connectivity.
3. Make all hierarchy output pass KiCad-native load/upgrade and normalized
   round-trip checks for root and every child sheet.
4. Add a generic strict regression graph with hierarchy plus the shared
   regulator simulation contract.
5. Prove clean ERC, DRC, routing, connectivity, writer correctness, and zero
   round-trip diffs without allowlists or fixture-specific exceptions.

## Non-goals

- No topology-specific hierarchy schema or coordinates.
- No suppression of KiCad load, ERC, DRC, or round-trip findings.
- No mutation of imported hierarchy.
- No fabrication-ready claim.

## Required writer contract

- Root and child schematics have valid, stable UUIDs.
- Every root sheet has an existing relative child filename and a parent instance
  path rooted at the root schematic UUID.
- The root `sheet_instances` block represents only the virtual root path `/`.
- Every parent sheet symbol owns its project/path/page instance; child files do
  not duplicate that ownership with a root `sheet_instances` block.
- Child symbol instance paths identify the containing sheet as
  `/root-uuid/sheet-uuid`; they do not append the symbol UUID.
- Cross-sheet labels and bus entries use valid parent/child ownership.
- KiCad library symbol records required by child sheets are embedded or
  resolvable exactly as they are for flat output.
- Reader, writer-correctness, and round-trip code discover and check every
  child sheet, not only the root.

## Acceptance evidence

For a recorded `generic-circuit-v1` protected regulated-power graph:

- trusted catalog and library resolution succeeds;
- the graph declares `linear_regulator_ideal_v1`, explicit DC constraints,
  and hierarchy `auto`;
- `StageSimulation` writes a passing simulation artifact;
- `kicad-cli sch upgrade` loads root and every child sheet;
- strict KiCad ERC/DRC reports zero errors and unconnected items;
- route completion, connectivity, project writer correctness, and every
  root/child normalized round-trip diff are clean;
- the promotion harness supplies the fixture's declared readiness explicitly,
  without changing achieved gates, and deterministic replay preserves it;
- the promotion report is `pass` and matches its metadata.
