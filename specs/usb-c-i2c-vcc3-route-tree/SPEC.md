# USB-C I2C Route-Tree Promotion

## Goal

Promote `usb_c_i2c_sensor_3v3_protected` from `expected_fail` to reproducible
KiCad-backed `pass` without relaxing electrical, routing, writer, or DRC gates.

## Completion Evidence

- `rail3v3.vout` resolves to a physical AMS1117 VOUT pad shape after placement.
- VCC_3v3 retains logical endpoint deduplication while route trees and contact
  proof retain every physical duplicate pad shape.
- The target's regulator, output capacitor, sensor, and header VCC_3v3
  endpoints are one proven route-contact graph component.
- Regulator local realization emits physical bypass routes only; virtual entry
  anchors are not emitted as copper.
- A declared wide board places fragments in deterministic left-to-right flow.
- The current outside-sandbox KiCad 10.0.3 run reports clean ERC and strict
  DRC, with writer correctness and normalized schematic/PCB round trips clean.

## Required Behavior

1. Retry placement and routing as one deterministic transaction when a
   route-tree branch produces an eligible geometry repair hint. Evaluate at
   a configured, bounded candidate budget per retry cycle; the default budget
   is four for this fixture and must be reported in evidence.
2. Preserve fixed local routes and their width, clearance, and protected USB-C
   power-path requirements. When an authorized placement group moves, transform
   only local routes whose endpoints and intermediate geometry are wholly
   relative to the group. A route touching a fixed component is a boundary
   route, remains fixed, and makes that group ineligible unless it can be
   regenerated safely.
3. Select complete candidate transactions, not greedy intermediate moves. Rank
   final states lexicographically by: complete required-net groups (higher),
   proven required endpoints (higher), route-contact graph components (lower),
   blocking findings (lower), then existing deterministic route score and
   stable component IDs. A retry that merely trades VCC_3v3 failure for GND
   failure is rejected.
4. Keep the correction generic: no fixture IDs, names, coordinates, or
   topology-specific branch rules.
5. Retain fail-closed diagnostics when no bounded candidate transaction reaches
   a safe, more complete final state.

## Non-Goals

- Disabling local copper obstacles globally.
- DRC/ERC allowlists or metadata-only promotion.
- New blocks, component families, or provider behavior.

## Acceptance

- VCC_3v3 and GND are complete in the same route-contact graph result.
- Internal connectivity, route completion, writer correctness, and normalized
  schematic/PCB round trips pass.
- KiCad ERC and strict DRC pass in the optional promotion lane.
- Metadata changes to `pass` only after current evidence proves every gate.
- An unsatisfiable candidate budget fails closed with stable diagnostics and
  leaves generated project output and prior accepted evidence unchanged.
