# Generated Inter-Block Routing Audit

Date: 2026-06-28

## Current Behavior

- `PlaceFragments` builds placement nets from realized block-local PCB routes.
- `RoutePlacement` routes the placement nets produced by placement and emits local-route connectivity evidence.
- Connector/LED generated examples bind the LED block-local resistor-to-LED route to placed pad endpoints.
- Request-level block connections such as `header.SIG -> status.IN` are present in schematic composition operations, but are not promoted into `placement.Request.Nets`.

## Gap

The PCB writer can prove that generated block-local copper touches the intended pads, but it cannot yet prove that separate generated blocks are electrically connected on the PCB. A connector-to-LED design can therefore be structurally valid while relying on schematic-only inter-block connectivity.

## Required Closeout

- Resolve each exported block port to the concrete footprint pad used inside that block.
- Promote request-level block connections into placement/routing nets using those physical endpoints.
- Distinguish local, inter-block, anchor, global, and external route evidence.
- Fail routing evidence when any endpoint of an inter-block net cannot resolve to a physical pad or cannot produce route copper.
- Promote connector/LED and I2C breakout examples from local-route evidence to full inter-block route-completion evidence.
