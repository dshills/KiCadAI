# BMP280 AP2112 Compact Local Routing

## Goal

Replace inherited AMS1117-oriented local geometry with a coherent AP2112-specific regulator realization so the BMP280 fixture no longer emits copper on or beyond the left board edge.

## Scope

- Add conditional placement variants to PCB entry anchors.
- Add conditional atomic local-route geometry variants that may replace or clear waypoints, override layer, and disable an inherited entry-anchor dogbone.
- Apply variants only when `regulator_symbol` is `Regulator_Linear:AP2112K-3.3`.
- Preserve component selection, AP2112 footprint, net names, power widths, board outline, inter-block routing policy, and non-AP regulator behavior.

## Entry Anchor Variants

An anchor placement variant contains a complete `RelativePlacement` and a non-empty existing `RealizationWhen`. The first matching variant replaces the default placement before fragment origin and placement transforms are applied.

The AP2112 VIN anchor must move inside the board while retaining adequate clearance from the input capacitor ground pad and board edge. VOUT and GND anchors remain unchanged unless DRC evidence requires otherwise.

## Route Geometry Variants

A local-route geometry variant is atomic and contains one or more of:

- replacement `waypoints`;
- `clear_waypoints`;
- replacement `layer`;
- `disable_entry_anchor_dogbone`;
- `disable_entry_anchor_via` when the anchor is already joined to same-layer pad copper;
- `disable_route` when a generic route is replaced by an active profile-specific route;
- non-empty `when`.

`waypoints` and `clear_waypoints` are mutually exclusive. A variant with no geometry change is invalid. The first matching variant is applied before route points are realized. Existing `waypoint_variants` remain supported and unchanged.

## AP2112 Geometry

The AP2112 variant must keep all regulator-local copper inside the board and connect:

- VIN anchor to input capacitor;
- input capacitor to AP2112 VIN/EN net;
- VOUT anchor/output capacitor to AP2112 output;
- GND anchor/input capacitor to output-capacitor return;
- AP2112 VIN to EN using the existing tie.

Routes should use short orthogonal paths around the SOT-23-5 body. Entry dogbones and standalone anchor vias inherited for the larger AMS1117 realization are disabled where the AP anchor or capacitor pad already provides physical same-layer access.

## Acceptance

- Existing non-AP regulator realization tests remain unchanged.
- Conditional anchor and route variants validate, clone deeply, and select deterministically.
- AP2112 target operations contain no regulator-local coordinate at or left of the board edge.
- The target retains zero unconnected items, ERC, writer correctness, and route-completion evidence.
- Strict KiCad DRC improves from 28 findings and removes all ten `copper_edge_clearance` findings without introducing a new violation type.
