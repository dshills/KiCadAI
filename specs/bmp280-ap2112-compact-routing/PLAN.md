# Implementation Plan

## Phase 1: Conditional Realization Model

Add entry-anchor placement variants and atomic route geometry variants. Validate conditions, placements, layers, mutually exclusive waypoint controls, and non-empty changes. Deep-clone conditions and waypoint slices.

## Phase 2: Realization Selection

Select the first matching anchor and route variant before applying origins and emitting realized local routes. Add focused selection and default-preservation tests.

## Phase 3: AP2112 Compact Geometry

Define AP2112-only VIN anchor and local route variants. Keep route widths and electrical intent unchanged. Add operation-level assertions that AP local geometry remains inside the board-side boundary.

## Phase 4: KiCad Evidence

Run focused block/design-workflow tests and the target workflow with strict KiCad ERC/DRC. Tune only AP local coordinates. Revert any geometry that worsens the 28-finding baseline or creates unconnected items.

## Phase 5: Prism And Commit

Stage only the model, tests, spec, and AP2112 geometry. Run Prism, resolve findings, rerun focused and target evidence, then commit the completed blocker.
