# Protected Amplifier KiCad Evidence Baseline

Baseline command:

```text
kicadai --request examples/design/amplifier/class_ab_headphone_driver.json --output examples/.generated/class_ab_headphone_driver_baseline --overwrite design create
```

This assumes the compiled `kicadai` binary is installed on `PATH`; during local
development the same workflow can be run with the repository's normal build or
test commands.

Observed result:

- `block_planning` succeeds.
- The Class AB output stage summary is present with `readiness:
  headphone_connectivity`.
- The protected output summary is present:
  - instance: `output_protection`;
  - block: `headphone_output_protection`;
  - load kind: `headphone`;
  - nominal load: `32Ω`;
  - AC output coupling: present;
  - DC-blocking capacitance: `220uF`;
  - bleed resistor policy: `present`;
  - series resistor: `omitted`;
  - connector return: `load_return_and_reference_connected` in the current
    workflow summary, meaning the `HP_RET` connector port is attached to the
    `LOAD_REF` return node;
  - fault protection: `placeholder_blocked`;
  - readiness: `connectivity`.
- `component_selection` blocks before schematic, PCB realization, placement,
  routing, project write, writer correctness, validation, or KiCad checks.
- The first blocking issue is:
  - code: `COMPONENT_NOT_FOUND`;
  - path: `component_selection.gain.opamp`;
  - message: `no component selected for gain.opamp`.
- Amplifier output transistor evidence remains warning-level only for
  fabrication promotion:
  - power dissipation is review-required;
  - thermal evidence is review-required;
  - safe operating area evidence is review-required;
  - fabrication-candidate use is blocked until review is complete.

Implication:

The next optional KiCad-backed fixture should start as `expected_fail`. Its
metadata should expect `block_planning` and `component_selection`, should
require a promotion report artifact, and should name the missing op-amp
component selection plus unpromoted amplifier output evidence as the current
blockers. Later phases can promote it only after component selection reaches
schematic and PCB evidence.
