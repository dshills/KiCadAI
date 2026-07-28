# Behavior-Driven Clock And Programming-Interface Synthesis

## Objective

Synthesize controller clock and programming support from behavior-level
requirements using only reviewed catalog evidence, generic calculations, and
the shared circuit graph. The generator must choose and realize supported
clock/programming architectures without requiring the request to name parts,
pins, nets, coordinates, layers, routes, providers, or repair actions.

## Supported Envelope

The milestone covers:

- internal or integrated controller clocks already qualified by the selected
  MCU record;
- calculated two-pin external crystal networks;
- existing catalog-backed canned oscillators and buffered clock fanout;
- ISP, UART bootloader, and SWD selection from controller evidence;
- reset/boot entry networks declared by the controller record;
- shared programming-pin series isolation where required; and
- existing generic mixed-voltage programming translation.

It does not claim PLL, RF, spread-spectrum, high-speed differential clock,
arbitrary resonator, USB bootloader, JTAG, or unmodeled programmer support.

## Clock Contract

External-crystal synthesis must:

- require a positive frequency target, tolerance, bounded board-stray
  capacitance, and startup limit;
- choose a concrete verified crystal whose frequency, stability, drive,
  package functions, and lifecycle satisfy the request;
- compute each load capacitor from `C_each = 2 * (C_load - C_stray)`;
- choose concrete capacitors from the catalog;
- connect the crystal and capacitors through semantic MCU clock and reference
  functions;
- retain frequency, stability, drive, load-error, and startup bounds in
  finalized calculation evidence; and
- fail closed when any required bound or catalog fact is absent.

## Programming Contract

Programming-interface synthesis must:

- select only a programming mode declared by the chosen controller;
- map every required signal, reset/boot entry function, supply reference, and
  physical header endpoint from catalog evidence;
- prove the requested programming frequency does not exceed the controller
  limit;
- bound connected input capacitance and programmer/target voltage mismatch;
- calculate selected series isolation and its `2.2RC` edge bound when shared
  application pins require physical isolation;
- retain all inputs, bounds, and a stable hash in finalized calculation
  evidence; and
- reject unpowered-target, missing-entry, incompatible-voltage, excessive-load,
  or excessive-frequency requests with stable capability codes.

## Shared-Graph And Physical Contract

- Passive crystals and load capacitors are first-class published function
  capabilities, not legacy allowlist entries.
- The standard KiCad 5032 two-pin crystal footprint has verified shared pad
  geometry derived from the installed KiCad library.
- Crystal placement remains near the selected MCU clock pins.
- Both crystal branches and their reference returns must be fully connected and
  routed.
- No fixture identity, fixture coordinate, topology-specific schema, or
  request-specific block family is permitted in production code.

## Frozen Evidence

The frozen corpus contains:

- calculated 16 MHz external-crystal ISP;
- integrated-clock UART bootloader; and
- unsupported SWD while the target is unpowered.

The manifest also records inherited evidence for canned oscillators, buffered
fanout, and translated mixed-voltage programming.

## Acceptance

Completion requires:

- byte-stable architecture replay and selected-component/calculation hashes;
- stable fail-closed behavior for the unsupported case;
- both supported cases pass shared-graph resolution, lowering, placement,
  routing, connectivity, writer correctness, clean ERC, strict DRC, zero
  normalized round-trip differences, and deterministic replay;
- preserved protected USB-C LED, protected USB-C I2C sensor, ESP32, MCU,
  standalone-clock, and amplifier evidence; and
- documentation and checked-in derived provenance agree with the current
  catalog.
