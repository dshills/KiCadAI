# Verification Evidence

## Result

The topology-aware generated path now emits visible local conductors, keeps
feedback paths traceable, places rail-entry power flags beside their connector,
orients passives by electrical role, and chooses the smallest fitting standard
sheet in either orientation. The motivating controlled-current fixture renders
on A4 landscape instead of a sparse A2 sheet.

KiCad PDF export was used for visual evidence. KiCad 10 SVG export currently
marks schematic wire paths with `stroke:none`, including for KiCad-authored
schematics, so SVG output is not used as a wire-visibility oracle.

## Installed-KiCad Generated Corpus

Command:

```sh
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 \
KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT=/tmp/kicadai-readable-review5 \
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
go test ./internal/opentopologysynthesis \
  -run '^TestFrozenHeldOutCorpusOptionalKiCadPromotion$' -count=1 -v
```

Result: pass. Six of the eight frozen cases produced physical KiCad projects
and passed the required promotion gates:

| Case | Coverage | Project hash |
| --- | --- | --- |
| `adjustable_current_output` | controlled current source and op-amp control | `bfc1557f22361a63578bdf4b87d82e6b0392b47ef0b323058e15b125612be1aa` |
| `audio_mute` | op-amp and controlled analog path | `836e3b74c5542a29221e5e01281903fbb5b995d7f53c75cb394d673c9318858c` |
| `ground_referenced_load_control` | protected controlled load | `80aebb35869926867a5e21e3b3047d12a35d7c07f44627526bb9bca063b887b7` |
| `hysteretic_detector` | op-amp feedback and hysteresis | `067762746f8242bc7e6cc22bb0f802247086d9e2336604f239e78bbc61de7067` |
| `powered_lowpass` | active/passive filter | `512eff164275a78fab68dafc2b151baf286ef3e1e32ab6ca91831972d31570b7` |
| `sensor_conditioner` | conditioned analog output | `887fdbbe1311e987e883d2851bc8aa255eb8567f3f6de7e29ec3a530c97767c3` |

`adjustable_voltage_regulation` and `voltage_window_monitor` remained stable
bounded synthesis non-passes and never entered physical promotion. They are not
counted as layout failures.

For each promoted project the harness required clean ERC, strict DRC,
connectivity, route completion, writer correctness, deterministic replay, and
zero normalized and raw writer round-trip diffs.

## Amplifier And Regulator Compatibility

The installed-KiCad design-example tier also passed for:

- `class_a_bjt_line_preamplifier`;
- `class_ab_headphone_protected`; and
- `usb_c_i2c_sensor_3v3_protected`, including its linear regulator and
  decoupling path; and
- `usb_c_led_indicator_protected`, with its regenerated deterministic
  transaction snapshot.

The optional harness accepts
`KICADAI_DESIGN_EXAMPLE_ARTIFACT_ROOT` so these projects can be retained and
rendered after a successful run.

The older amplifier composition fixtures still use long internal reference
identifiers and do not opt into automatic topology inference. Their electrical,
PCB, writer, and installed-KiCad gates remain green, but they are compatibility
fixtures rather than visual exemplars. Replacing their legacy reference,
off-page-origin, and stage-order conventions belongs to the next
composition-layout goal; this change does not add amplifier-specific placement
branches.

## Render Commands

Each generated schematic was rendered through KiCad PDF output and Poppler:

```sh
kicad-cli sch export pdf --output render.pdf generated.kicad_sch
pdftoppm -png -r 120 -singlefile render.pdf render
```

Retained visual artifacts include:

- `/tmp/kicadai-readable-review5/adjustable_current_output/render.png`;
- `/tmp/kicadai-readable-final-current/hysteretic_detector/render.png`;
- `/tmp/kicadai-readable-final-current/powered_lowpass/render.png`;
- `/tmp/kicadai-readable-design-current/class_a_bjt_line_preamplifier/render.png`;
- `/tmp/kicadai-readable-design-current/class_ab_headphone_protected/render.png`;
- `/tmp/kicadai-readable-design-current/usb_c_i2c_sensor_3v3_protected/render.png`.

## Determinism And Unit Coverage

Focused tests cover:

- same-name net normalization;
- continuous labeled local conductors;
- multi-endpoint route-tree junctions;
- bounded deterministic orthogonal grid search;
- preferred pin-direction endpoint access with orthogonal fallback;
- collision-safe route annotations;
- graph-derived feedback roles and stage ranks;
- role-derived passive orientation;
- power-flag attachment at rail entry;
- landscape/portrait page selection;
- readability metrics and report propagation.

All Go packages outside `internal/compositionlowering` pass locally. The
composition package's deterministic lowering, writer-request, hierarchy, and
shard-selection tests also pass. Running every composition promotion corpus in
one process exceeds even a 20-minute package timeout in the pre-existing
`mixed_function_control_power` nonlinear simulation path; the timeout stack is
in `internal/simmodel`, not in the schematic layout or writer changes.
