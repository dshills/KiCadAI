# USB-C I2C Route-Tree Promotion

## Goal

Promote `usb_c_i2c_sensor_3v3_protected` from `expected_fail` to reproducible
KiCad-backed `pass` without relaxing electrical, routing, writer, or DRC gates.

## Current Evidence

- `rail3v3.vout` now resolves after placement.
- VCC_3v3 now has exactly three logical netlist endpoints: regulator, sensor,
  and header. Duplicate KiCad pad-number aliases are one netlist target; pad
  hydration and writer validation still retain every physical pad shape for
  DRC and footprint-current checks.
- With all fixture-local copper protected, the regulator-to-sensor/header
  branch has no legal path.
- Making the sensor movable completes VCC_3v3 but causes a GND tree failure.
- Removing all local-power obstacles permits routes but produces real
  connectivity and DRC violations. It is explicitly not an acceptable fix.

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
