# Implementation Plan

## Phase 1: Conditional Model And Validation

Add destination endpoint-dogbone variants to block realization, cloning, and realized routes. Validate non-empty conditions and at least one route waypoint.

Tests: accepted conditional variant, missing condition, missing waypoint, and realization selection.

## Phase 2: Mixed-Layer Emission

Split an opted-in route at its final transformed waypoint, emit the default through via there, and add the destination-layer dogbone operation.

Tests: unchanged default route; B.Cu main route terminates at via; F.Cu dogbone terminates at destination pad; invalid same-layer/degenerate cases fail closed.

## Phase 3: BMP280 Geometry

Apply conditional endpoint dogbones and separate transition waypoints to BMP280 SDA/SCL only. Run focused packages and strict target ERC/DRC.

Acceptance: fewer than 32 DRC findings, no new finding types, zero unconnected items.

## Phase 4: Review And Commit

Stage only this capability, its tests/specification, and BMP280 route geometry. Run Prism, resolve findings, rerun focused tests and target evidence, then commit the completed blocker.
