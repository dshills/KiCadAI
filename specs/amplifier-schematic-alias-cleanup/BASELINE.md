# Amplifier Schematic Alias Cleanup Baseline

## Fixture

- `examples/design/kicad-backed/class_ab_headphone_protected.json`
- `examples/design/kicad-backed/class_ab_headphone_protected.metadata.json`

## Current Workflow State

The protected Class AB headphone amplifier fixture reaches:

- block planning;
- component selection with verified LMV321, diode, and output transistor
  selections;
- schematic generation;
- schematic electrical validation.

It currently blocks at schematic electrical validation. The expected conflicts
are:

- `headphones_SIG` connected to `output_amp_out`;
- `output_lower_emitter` connected to `output_upper_emitter`.

Because schematic electrical validation blocks, these downstream stages are
expected to be skipped in the baseline:

- PCB realization;
- placement;
- routing;
- project write;
- writer correctness;
- validation;
- KiCad checks.

## Test Lock

`internal/designworkflow/design_examples_test.go` already locks this baseline in
`assertDesignExampleProtectedAmplifierEvidence`. Later phases should replace the
conflict assertions with alias-resolution evidence and downstream stage
progression assertions.
