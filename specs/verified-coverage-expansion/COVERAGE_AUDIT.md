# Verified Coverage Expansion Audit

Date: 2026-06-26

## Selected Slice

The first expansion slice is the 5 V to 3.3 V linear regulator path:

- `regulator.linear.ams1117_3v3.sot223`;
- 0805 ceramic input and output capacitors;
- the existing `voltage_regulator` block;
- structured-intent power rails that synthesize a regulator block.

This slice is intentionally narrow. It is already present in seed examples, but
its evidence is split across catalog records, block topology, and planner
calculations. Closing this path improves autonomous power-module and sensor
breakout generation without introducing a new analog design problem.

## Current Coverage

Catalog:

- `data/components/active_blocks.json` is the existing active component seed
  file for block-oriented active parts; it contains a verified AMS1117-3.3
  SOT-223 record with symbol pins, package pads, input-voltage rating,
  output-current rating, and companion capacitor requirements.
- `data/components/passives.json` contains rule-inferred 0603 and 0805 ceramic
  capacitors with capacitance ranges, voltage ratings, symbol pins, and package
  pad functions. Capacitance tolerance metadata is present on the ceramic
  capacitor records and remains part of selection evidence for future derating
  policy. These capacitors are acceptable for connectivity-level use because
  they are symmetric passives with explicit value/rating bounds and local
  symbol/footprint pin evidence; they are not promoted to exact-part
  fabrication identity.

Block library:

- `voltage_regulator` emits regulator, input-capacitor, and output-capacitor
  roles and has PCB realization for placement, local routes, and proximity
  constraints.
- The block topology validates basic input/output voltage, current, dropout,
  capacitance, and fixed three-pin AMS1117-family symbol support.

Intent planner:

- power input and power rail intents can synthesize a `voltage_regulator` block
  and connect a declared input source to regulator `VIN/GND`;
- synthesis reports include regulator headroom requirements.

## Gaps To Close

- The block component roles do not yet consistently carry component IDs or
  selection queries for the regulator and its capacitors.
- The selection layer does not have a focused positive case proving the
  regulator companion capacitors satisfy capacitance and voltage constraints.
- Workflow evidence does not assert that generated power-module requests expose
  component selection outputs for the regulator path.
- Documentation describes the block and catalog separately, but does not make
  the end-to-end verified power slice explicit for AI-agent use.

## Priority Matrix

| Candidate | Design Value | Evidence Availability | Implementation Risk | Decision |
| --- | --- | --- | --- | --- |
| AMS1117 3.3 V regulator slice | High: common sensor, MCU, and breakout boards | High: symbol, footprint, ratings, companions, block and PCB realization already exist | Low: mostly integration and tests | Selected |
| I2C sensor expansion | High: sensor breakout workflows | Medium: one verified sensor exists, but address and variant mapping need broader policy | Medium | Next candidate |
| Connector/programming headers | Medium-high: practical generated boards | Medium-high: catalog and blocks exist, but pin-role variants broaden quickly | Medium | Later |
| USB-C sink variants | High: modern power entry | Medium: block exists, but connector/no-connect/protection policy remains broader | Medium-high | Later |

## Acceptance For This Slice

- Component tests prove verified regulator selection and capacitor rating
  selection/failure.
- Block tests prove `voltage_regulator` role selection returns the verified
  AMS1117 and the expected 0805 capacitors.
- Intent planner/workflow tests prove a power-rail request emits the regulator
  block and preserves evidence-bearing component requirements.
- Docs and agent guidance point users at the compiled `kicadai` binary and the
  verified power path.
