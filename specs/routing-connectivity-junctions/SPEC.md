# Routing Connectivity Junctions

## Status

Proposed for the `generic_usb_c_bmp280_breakout` promotion milestone.

## Problem

The router can produce a valid same-net route tree whose later branches contact the
middle of an earlier segment. It can also place a via on the interior of a segment.
The current internal connectivity validator creates graph edges only between each
segment's two endpoints and between a via's layer points. It does not join:

- two same-layer segments whose centerlines intersect at a T-junction or crossing;
- a via layer point to a same-layer segment that passes through that point.

Route search also chooses among every physical access point associated with a
logical endpoint independently for each branch. When a footprint has duplicate pad
numbers, separate branches can therefore start from different physical pads even
though the route tree treats them as one endpoint. The resulting copper is genuinely
split.

As a result, physically continuous copper can be reported as `DISCONNECTED_PAD`.
The captured live generic BMP280 design demonstrates this after all nine nets route.

## Goals

- Model exact same-layer segment intersections as electrical graph connections.
- Model a via as connected to every same-layer segment passing through its center.
- Pin a repeated logical endpoint to the physical access point selected by its first
  successful branch, and reuse that exact point and layer for later branches.
- Preserve fail-closed behavior for separated, different-layer, and near-but-not-
  touching copper.
- Keep connectivity results deterministic and independent of segment ordering.
- Preserve existing routing, clearance, writer, and KiCad-backed fixtures.

## Non-Goals

- Changing route search, net ordering, placement, trace widths, or clearances.
- Treating nearby copper as connected merely because trace widths overlap.
- Connecting different nets or layers without a via.
- Adding fixture-specific references, coordinates, or topology dispatch.
- Replacing KiCad DRC or net connectivity evidence.

## Required Semantics

For one routed net, the connectivity graph must include:

1. An edge between each segment's start and end on its normalized layer.
2. An edge between intersecting segment components when two segment centerlines
   intersect on the same normalized layer.
3. An edge from each via layer point to a segment component when the via center lies
   on that segment's centerline on the same normalized layer.
4. Existing via edges joining every layer spanned by the via.
5. Route search must retain the first successful access-point selection for each
   repeated endpoint within a net. Later branches may add a via from that access but
   may not silently switch to another duplicate pad location.

Coordinates continue to use the routing package's canonical rounding and epsilon
rules. A same-coordinate crossing on different layers is not connected unless a via
spans those layers.

## Validation

Focused regression cases must cover:

- a T-junction into the middle of a segment;
- crossing same-layer segments of one net;
- a via on a segment interior joining top and bottom routes;
- crossing segments on different layers without a via remaining disconnected;
- near, non-touching same-layer segments remaining disconnected;
- deterministic results when segment order is reversed.
- a duplicate-pad endpoint used by two branches producing one connected route tree.

The captured live generic BMP280 request must advance from a false
`DISCONNECTED_PAD` result to a clean internal route-connectivity result. Existing
optional KiCad-backed pass fixtures must remain clean.

## Risks

- Overly loose geometric tolerance could join separate copper.
- Quadratic segment comparisons could regress large-route performance.
- Via contacts could be joined to the wrong layer if layer normalization differs.
- Pinning a poor first access point can lengthen later branches or expose a real route
  search failure that independent, disconnected branches previously hid.

The implementation should reuse exact centerline intersection predicates and use a
spatial candidate index if focused performance evidence shows pairwise comparison is
material.
