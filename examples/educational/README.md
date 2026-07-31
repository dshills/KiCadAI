# Educational Schematic Examples

These five examples are generated from Schematic Design IR and checked in as
ordinary KiCad projects. They favor direct wires, conventional signal flow,
matched-branch alignment, short labels, and enough whitespace to explain the
circuit from the page alone.

| Example | Teaching focus | Useful relationship |
|---|---|---|
| `01_dc_voltage_source` | A 9 V battery, load, return path, and output connector. | The source fixes the voltage; load current is approximately `I = V / R`. |
| `02_bjt_current_source` | A two-transistor NPN current mirror with a reference resistor and collector load. | Ignoring base currents, `IOUT` follows `IREF`. |
| `03_differential_amplifier` | A symmetric NPN long-tailed pair with equal collector resistors. | The collector outputs respond to the difference `V1 - V2`. |
| `04_rc_low_pass_filter` | A first-order passive low-pass filter. | `fc = 1 / (2*pi*R*C)`, approximately 1.59 kHz here. |
| `05_voltage_divider` | A two-resistor voltage divider. | `VOUT = VIN * R2 / (R1 + R2)`, or 4.5 V unloaded here. |

Each directory contains:

- `source.json`: semantic circuit connectivity and functional groups, with no
  component placement recipes or absolute coordinates;
- `generated/*.kicad_sch`: the generated KiCad schematic;
- `generated/*.kicad_pro`: the matching KiCad project;
- `generated/lib/` and `generated/sym-lib-table`, when present: local symbol
  dependencies needed to open the project reproducibly.

Open the `.kicad_pro` file in each `generated/` directory. To regenerate one
example from the repository root, use absolute input and output paths because
the writer changes into its output directory:

```sh
root="$(pwd)"
go run ./cmd/kicadai \
  --request "$root/examples/educational/04_rc_low_pass_filter/source.json" \
  --symbols-root /path/to/KiCad/share/kicad/symbols \
  --footprints-root /path/to/KiCad/share/kicad/footprints \
  --output "$root/examples/educational/04_rc_low_pass_filter/generated" \
  --overwrite schematic-ir write
```

KiCadAI derives row, column, branch, symmetry, rail, return, and endpoint
alignment from each circuit graph. Signal entry and exit points use standard
test-point terminals with visible conductors and local net names; external
power domains use KiCad power flags so native ERC remains clean. The examples
therefore exercise generic topology-aware layout instead of fixture-specific
placement hints or page coordinates.
