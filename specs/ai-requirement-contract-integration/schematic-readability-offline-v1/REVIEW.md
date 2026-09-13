# Local source and native-output review

Decision: retain as an opt-in experimental improvement and evidence checkpoint; **do not promote either design to readable or fabrication-ready**. This is a local agent review, not an independent reviewer or Gemini review. Base source is `b9d48213aa0431d5cf19824287c6403a4aea5529`; final changed-source hashes are in `verification.json`.

## Material findings

### R1 — Native collision handling still permits unreadable output

Priority: high for readability acceptance. The controller's R2 body is overlaid by `composition_net_007`; the U3 reference overlaps the C10 reset label. A vertical J4 wire crosses multiple labels and the MCU value. The standalone power-flag wire crosses the J2 ground-net label. These remain visible at 254 DPI, not merely at thumbnail scale.

The new body-envelope and field-anchor corrections reduce earlier collisions but do not reconcile every planner, adapter and writer text convention. Candidate-stub body rejection is not sufficient for existing route labels, foreign wires and annotation glyphs. Bounded fallback can still return a non-clear choice, and unknown symbol geometry is skipped. `erc-drc` requests enforce electrical/native correctness but do not require the separate readable acceptance tier. Thus these successful runs cannot serve as readability passes.

Evidence relative to `/tmp/kicadai-schematic-readability-offline-v1-final/`:

- `standalone_regulator/render/review-lower.png`: power-flag wire at approximately sheet x=50.8 mm crosses the J2 label around y=189 mm.
- `controller_adc_100ma/render/review-analog.png`: R2 and `composition_net_007`, near sheet x=282 mm, y=247 mm; vertical wires also cross inline horizontal route labels.
- `controller_adc_100ma/render/review-controller.png`: U3/reset annotation collision near sheet x=383 mm, y=230 mm; unrelated wire/label/value crossings near J4.

Required follow-up: one coherent native glyph/anchor model across placement, routing, output and post-write validation; collision failure must be explicit rather than silently accepted. Preserve all current negative artifacts and electrical requirements.

### R2 — Repeated in-memory generation is not serialized workflow replay

Priority: high for reproducibility claims. Schematic layout groups contain runtime-only `Inferred` state excluded from JSON. The new profile relies on inferred rather than fixed groups. Re-reading `workflow_request.json` can therefore choose a different layout, even though first/second generation from the same in-memory request is byte-exact.

The `TestSerializedReadabilityRequestDiagnostics` helper is deliberately labeled diagnostic-only. The older development diagnostic output does not represent the in-memory native run and is not used as a pass gate. Final artifacts include the in-memory `schematic_transaction.json` as well as the serialized request, but execution of that retained transaction as a complete replay was not established here.

Required follow-up: a versioned persisted layout-policy contract or an explicitly sealed canonical replay representation, with tests comparing in-memory and serialize/deserialize native output. Do not normalize away geometry differences or reinterpret the present evidence as full replay.

### R3 — Functional readability is still insufficient

Priority: medium for engineering usability, blocking this phase's rubric. Both sheets rely on `composition_net_###` labels instead of visually apparent supply/reference and signal roles. The controller is a single A2 landscape sheet with long support-net names. Connector MPNs and pin numbers do not identify purpose; supply decoupling is visually detached from the parts it supports.

Required follow-up: derive block headings and connector/endpoint purpose annotations from existing explicit contracts, without renaming electrical nets or guessing unprovided intent. Reassess sheet organization at intended print scale. No new hierarchy capability is claimed by this change.

### R4 — Internal option naming couples drawing defaults and geometry

Priority: low, maintenance limitation. `SchematicNetClassDefaults` selects both the project-file schematic netclass defaults and the builder's native label geometry. The adapter currently derives it from `OrientEndpointLabels`. Callers must not assume it is only a cosmetic project setting. A future versioned native layout policy should make that relationship explicit; the synthesis opt-in and default-path preservation tests bound the current change.

## Visual review coverage

| Example / view | Sheet or viewport | Rendering scale | Verdict |
|---|---|---|---|
| Regulator overview | One A4 portrait sheet, 210 × 297 mm nominal | 2200 px wide | Improved but fails functional/collision rubric |
| Regulator upper | x=60, y=25, w=105, h=115 mm | 254 DPI, 10 px/mm | Capacitors and fields legible; context remains dispersed |
| Regulator lower | x=10, y=130, w=180, h=105 mm | 254 DPI | Power-flag/connector annotation crossing |
| Controller overview | One A2 landscape sheet, 594 × 420 mm nominal | 2200 px wide | Large sparse sheet with dense core; fail |
| Controller capacitors | x=250, y=45, w=130, h=105 mm | 254 DPI | Fields improved; functional association not apparent |
| Controller analog | x=155, y=135, w=200, h=145 mm | 254 DPI | R2 collision and wire/label crossings |
| Controller core/support | x=345, y=135, w=140, h=170 mm | 254 DPI | U3/reset and J4-region collisions |

Viewport cropping is documented in `visual-review-commands.json`; clipping at a viewport boundary is not confused with clipping in the source sheet. Both complete native SVGs were reviewed before the detail viewports. The underlying native exports omit the drawing-sheet border; title-block reservation is a layout setting, not a visual certification of drawing-sheet/title-block completeness.

Both copper-layer PNGs for the regulator and all four copper-layer PNGs for the controller were inspected, together with outer-copper/silkscreen/outline overviews. Board geometry/connectivity matches the preceding phase. These renderings do not validate assembly labels, grounding/return quality, mechanical fit, manufacturability, or hardware operation. In particular, the outer-layer overview alone cannot show the controller's internal copper; separate In1.Cu/In2.Cu views are retained.

## Source and evidence checks

The changed code and new tests were reviewed locally after Atlas-assisted navigation. Unknown synthesis profiles fail closed; opt-in tests demonstrate electrical/PCB-intent invariance. Default synthesis still passes the frozen capability suite. Native label tests exercise all four endpoint directions; project serialization tests check exact mil units and round-trip; field/envelope regressions cover the observed anchor mismatch. The complete short suite, six-package race run, seven-package lint run and repository vet passed.

The full source tree was not exhaustively reviewed line by line. This is a bounded review of the diff, tests and resulting outputs. Existing explicit oriented-label paths receive corrected native behavior; no claim is made that every preexisting caller's rendered bytes are unchanged. The raw evidence verifier independently checks physical pins, pads, all mandatory native reports, identical repeated project inventories, and cross-phase PCB invariants. SHA-256 receipts bind final source and files, while development source revisions were not separately committed/sealed and must not be described as independently reproducible historical source snapshots.

No blocking readability finding was suppressed or converted into a pass. R1–R3 remain open; R4 is documented. No remote review or publication occurred.
