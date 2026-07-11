# BMP280 Local-Route Endpoint Dogbones

## Goal

Support a deterministic layer transition outside a fine-pitch SMD pad so the BMP280 SDA and SCL local routes can use legal board-default vias without via-in-pad clearance violations.

## Scope

- Add optional source- and destination-endpoint dogbone intent to `PCBLocalRoute`.
- Use the route's first/final authored waypoints as layer-transition via locations.
- Emit the main route on its authored layer up to that waypoint.
- Emit a short same-net dogbone on the destination pad layer from the transition via to the pad.
- Use existing default via diameter/drill and active board rules.
- Opt in only the existing BMP280 SDA and SCL pull-up routes through a conditional variant.

Arbitrary multi-layer stacks, blind/buried vias, inter-block routing, placement changes, and design-rule changes are out of scope.

## Model

`PCBLocalRoute` gains conditional endpoint-access variants. A v1 variant contains:

- `from_endpoint_dogbone` and/or `to_endpoint_dogbone`: boolean;
- `when`: existing `RealizationWhen` condition.

When active, the route must have at least one authored waypoint. The final waypoint is the transition point. The realized route records the selected dogbone intent so placement can transform all route points using existing geometry code.

## Emission Semantics

For endpoint dogbones:

1. Resolve and transform the normal route points.
2. Treat the second transformed point as the source transition when enabled.
3. Treat the penultimate transformed point as the destination transition when enabled.
4. Emit the main route between the selected transition points.
5. Materialize normal through vias at enabled transition points.
6. Emit pad-layer dogbones between each enabled endpoint pad and transition.
7. Count all operations and preserve endpoint-connectivity evidence.

The destination endpoint must be an SMD pad on a copper layer different from the main route. Unsupported or degenerate geometry fails closed with a route-binding issue.

## BMP280 Geometry

The SDA and SCL B.Cu routes receive distinct final transition waypoints outside the LGA-8 body. Their top-layer dogbones must not cross each other, VCC/GND ties, or package pads. Vias remain at the existing 0.6 mm diameter and 0.3 mm drill.

## Acceptance

- Existing local routes emit unchanged transactions when no variant matches.
- Invalid dogbone declarations fail validation.
- The main B.Cu route ends at the transition via, not the SMD pad center.
- A connected F.Cu dogbone reaches the exact destination pad.
- The target retains zero KiCad unconnected items, clean ERC, writer correctness, and route-completion evidence.
- Strict KiCad DRC improves from the committed 32-finding baseline without new violation types.
