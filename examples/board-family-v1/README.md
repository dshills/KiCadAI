# Generated board-family examples

These three complete projects were copied byte-for-byte from the successful offline acceptance run `acceptance-configurations-04`; no generated board was repaired manually. Open each `board.kicad_pro` in KiCad 10.0.3:

- [standard](standard/board.kicad_pro): 4.7 kΩ pull-ups, 100 kHz, ≤200 pF total bus.
- [fast](fast/board.kicad_pro): 2.2 kΩ pull-ups, 400 kHz, ≤100 pF total bus.
- [low_current](low_current/board.kicad_pro): 10 kΩ pull-ups, 100 kHz, ≤100 pF total bus.

Each directory includes local symbol/footprint libraries, schematic and PCB SVG previews, exact BOM, numerical configuration, electrical calculations, native validation and original KiCad reports. The reports preserve their actual execution paths; relocation of these copies is not a newly executed validation.

The [integrity manifest](../../specs/board-family-v1/evidence/offline/manifest.json) binds copied bytes to the recorded run. It is local provenance, not an independent authentication signature. See the [family definition](../../specs/board-family-v1/FAMILY.md) for operating requirements, programming connections and review limitations. No firmware, assembled hardware, manufacturing qualification or wireless performance has been demonstrated.
