# KiCad Demo Project Structure Notes

These notes summarize project-directory structures observed during a local review of a KiCad demo project collection outside this repository. The external demo files are discovery inputs only; normal repository tests must use synthetic in-repo fixtures and must not depend on that local directory.

## Core Project Files

Simple KiCad projects commonly use a root file trio:

```text
<project>.kicad_pro
<project>.kicad_sch
<project>.kicad_pcb
```

Schematic-only generated projects may omit `<project>.kicad_pcb`.

## Directory Name and Project Basename

The project directory basename is not required to match the KiCad project file basename.

Observed examples:

```text
cm5_minima/CM5_MINIMA_3.kicad_pro
openair-max/One-Air-Max.kicad_pro
royalblue54L_feather/RoyalBlue54L-Feather.kicad_pro
```

The generated project writer should treat the output directory and the KiCad project basename as separate concepts.

## Project-Local Libraries

Richer demo projects commonly include project-local library tables:

```text
sym-lib-table
fp-lib-table
```

Project-local symbol libraries are usually `.kicad_sym` files referenced by `sym-lib-table` through `${KIPRJMOD}`. Project-local footprint libraries are usually `.pretty/` directories referenced by `fp-lib-table` through `${KIPRJMOD}`.

## Hierarchical Schematics

Large projects commonly use a root schematic plus child sheets:

```text
video.kicad_sch
muxdata.kicad_sch
modul.kicad_sch
sch/Connectors.kicad_sch
```

Root schematic sheet nodes use `Sheetname` and `Sheetfile` properties. Every generated `Sheetfile` path must resolve to a child schematic file inside the project directory.

## Optional Artifacts

Some projects include optional project artifacts:

```text
<project>.kicad_dru
custom_page_layout.kicad_wks
*.3dshapes/
3d_shapes/
3dmodels/
lib/3dmodels/
```

These files and directories are not required for minimal generated projects, but the writer should support them when the design model references them.

## Modern Project JSON Sections

Representative modern `.kicad_pro` files include these top-level JSON sections:

```text
board
boards
component_class_settings
cvpcb
erc
libraries
meta
net_settings
pcbnew
schematic
sheets
text_variables
time_domain_parameters
```

KiCadAI project JSON should emit this KiCad-shaped section set with deterministic defaults.
