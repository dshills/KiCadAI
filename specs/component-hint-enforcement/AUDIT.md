# Component Hint Enforcement Audit

Date: 2026-07-02

## Checked-In Hint Inventory

The initial checked-in catalog has a small but useful hint surface:

- `regulator.linear.ams1117_3v3.sot223`
  - placement: `near input_capacitor 3mm`, `near output_capacitor 3mm`
  - routing: `net_class power 0.5mm`
- `regulator.linear.ap2112k_3v3.sot23_5`
  - placement: `near input_capacitor 2mm`, `near output_capacitor 2mm`
  - routing: `net_class power 0.3mm`, `tie enable`, `no_connect nc`
- op-amp seed records
  - placement: `near decoupling_capacitor 3mm`
- crystal/canned oscillator records
  - placement: `near mcu_oscillator_pins 10mm`
  - routing: `short_loop clock`

## Supported Phase-1 Classification

Supported placement hint kinds:

- `near`
- `edge`
- `keepout`

Supported routing hint kinds:

- `net_class`
- `tie`
- `no_connect`

Known unsupported checked-in routing hint:

- `short_loop`

`short_loop` should remain visible as evidence but must not mutate routing until
the routing model can represent timing-loop constraints directly.

## Initial Enforcement Strategy

The first implementation step only exposes selected hints in workflow evidence.
Later phases normalize, consume, validate, and report hint status.

Phase 1 is complete when selected component summaries include placement/routing
hint counts for regulator selections and the raw hints are preserved in
`ComponentSelectionEntry`.
