# Offline schematic readability: result and evidence

As of 2026-09-11. This phase improves native schematic rendering and establishes byte-exact repeated generation from the same in-memory request, but **neither development design passes the complete readability rubric**. The frozen AI evaluation is unchanged: this work adds zero frozen positive passes and does not complete the six-positive/two-paired-design milestone.

## Final-source results

| Development example | Physical components | Routed nets | Exact connected pins/pads | ERC / DRC findings | Repeated native runs | Readability |
|---|---:|---:|---:|---:|---:|---|
| Standalone 3.3 V regulator | 8 | 3/3 | 16/16 | 0 / 0 | 2 | Fail |
| Regulator, filter and controller ADC, 100 mA | 21 | 10/10 | 52/52 | 0 / 0 | 2 | Fail |

Both final-source examples pass all nine required workflow stages in both runs: schematic, schematic electrical validation, placement, routing, project writing, strict writer correctness, validation, simulation and native KiCad checks. Writer round-trip skips and failures are zero. Actual installed KiCad 10.0.3 libraries were used. Both requests achieve `erc-drc` and explicitly report `fabrication_ready: false`.

The requirements are byte-identical to the preceding explicit-reference phase. Components, nets, PCB regions, simulation, support circuitry, routing policy and catalog identities also match. Both outputs remain 3.3 V. Predicted junction temperatures remain 103.0495 °C and 114.0495 °C; the controller filter cutoff remains 923.361280 Hz within 900–1100 Hz. These are modeled results, not physical bench measurements. The original 150 mA controller rejection at 125.049500001 °C remains unchanged and is not replaced by the separately named 100 mA example.

These are two synthetic integration examples, not two new AI benchmark cases. Four successful final-source generation runs are not four distinct designs. The broader frozen practical campaign remains 0/8 complete primary positive boards.

## Reusable changes

`ResolveOptions.SchematicLayoutProfile = "topology-v1"` explicitly enables the experimental synthesis layout. Empty selection preserves the legacy synthesis policy and frozen capability receipts; unknown profile names fail closed. The profile is recorded in synthesis evidence. It removes the synthesized single fixed-rank group and fixed rotations, uses the shared topology/role inference with standard schematic spacing, reserves the title-block area and limits auxiliary rank density. Electrical nets and PCB placement/routing intent are not rewritten.

The oriented native path also reserves an envelope around symbol bodies and pin anchors, centers KiCad reference/value properties correctly, adds field separation, orients endpoint labels using KiCad's upright glyph convention, and rejects candidate label stubs that cross known symbol bodies. This is a heuristic improvement, not a complete collision guarantee: unknown geometry and bounded-placement fallback remain limitations.

Generated projects now include explicit schematic Default-netclass wire/bus widths and solid line style on this opt-in path. Before this correction, native KiCad SVG exports contained invisible wire strokes. KiCad schematic netclass widths use mils, unlike PCB dimensions; emitted widths are 6 and 12 mils, with round-trip regression coverage. The unit conversion is consistent with the [KiCad net-settings source](https://docs.kicad.org/doxygen/net__settings_8cpp_source.html). No user KiCad configuration or generated design was hand-edited to obtain these results.

The new synthesis profile is not a new global default or a general hierarchy implementation. Existing explicit `OrientEndpointLabels` users receive the corresponding native annotation/drawing behavior. The internal `SchematicNetClassDefaults` switch currently also selects native label geometry; this coupling is documented as a maintenance limitation in the review.

## Readability verdict

Every sheet was reviewed: one A4 portrait regulator sheet and one A2 landscape controller sheet. Review used full-page 2200-pixel-wide native exports plus five documented 254-DPI viewports (10 pixels/mm). The original sheet remains available for context around crop boundaries. The viewports are native SVG rasterizations, not retouched images.

Wires are now visible, the MCU and op-amp bodies are separated, and many fields are more legible. Remaining decisive failures include:

- The standalone power-flag wire crosses the connector's `composition_net_001` label. The drawing still disperses the regulator capacitors and does not make connector purpose or supply/reference identity readily apparent.
- In the controller drawing, `composition_net_007` overlays the R2 resistor body. The MCU reference U3 overlaps the reset-support label near C10. Several vertical wires cross unrelated labels or component value text around J4 and U3.
- Generic composition-net names and long support-net names obscure functional flow. Connector MPNs and pin numbers alone do not explain analog input, output, power or programming purpose.

See [REVIEW.md](REVIEW.md) for evidence locations and the acceptance decision. Neither native ERC nor an internal layout diagnostic overrides this visual failure. Copper plots were inspected, including both standalone layers and all four controller layers; no assembly, thermal-layout, signal-integrity, mechanical or fabrication approval is implied. Reference designations are not legible as assembly annotations in the retained board overview.

## Replay and connectivity authentication

`seal-and-render.mjs` inventories originals before exporting anything, then renders and exports native XML netlists from disposable copies. It rechecks original inventories afterward. The 14 regulator and 23 controller generated project files match byte-for-byte between first and second generation, including schematics, boards, projects, project-local symbols/footprints and both library tables. No normalization is used for this same-phase comparison. Diagnostic `.kicadai` subtrees are retained and hashed in the raw inventory but excluded from the definition of primary generated project content.

All 68 connected physical symbol endpoints match the native XML netlists. All 68 corresponding connected PCB pads match the request; the controller's 26 unconnected pads remain unconnected. KiCad's XML export omits virtual power symbols, so these are not falsely counted as missing physical parts; their ERC receipts remain mandatory.

The regulator PCB is byte-identical to the preceding phase. The controller PCB differs only in its drawing-paper orientation and numeric net-ID assignment. An independent S-expression audit resolves numeric net IDs to their unchanged names and excludes the drawing-paper setting, then requires all remaining PCB content to match exactly. This cross-phase semantic comparison is separate from the unnormalized same-phase replay test.

**Serialized-request replay is not certified.** Layout group `Inferred` state is runtime-only (`json:"-"`). Reading the saved workflow JSON therefore does not recreate the in-memory layout state. The development `layout-analysis.json` reports audit that serialized request, not the request used for native generation. The in-memory schematic transaction is retained separately, and neither those diagnostics nor repeated generation are presented as a full serialized replay pass.

## Verification and retained evidence

- Repository short suite: 151 packages passed, 14 had no tests, zero failures. The longest package completed in 679.708 seconds within the unchanged 12-minute package limit.
- Race checks passed for circuitgraph, designapi, project files, schematic IR, schematic layout and transactions.
- Repository-wide vet passed; lint reported zero issues across all seven affected packages. `git diff --check` passed.
- Final native test completed in 31.237 seconds; each example has a 20-minute context bound and is generated twice. These tests run separately from the ordinary short suite, where the native harness is deliberately opt-in.
- [verification.json](verification.json) binds 16 changed Go files and the final 380-file, 1,529,020,156-byte native evidence inventory: SHA-256 `4bf3e2c52f64e33eefcf27d115ced94bf78815a7ce6594439f93b79349be32ff`.
- [preservation.json](preservation.json) authenticates the original interface/practical evidence, immutable source checkout, frozen cases, supplementary/tamper receipts, sealed evaluator binaries and exact I04 counterexample. [preservation-final.json](preservation-final.json) repeats this audit after reporting and confirms the changed-file secret scan is clear.
- [previous-phase-verification.json](previous-phase-verification.json) authenticates the preceding phase's 17 source files at its own commit and its unchanged complete raw inventory. [previous-archive-verification.json](previous-archive-verification.json) independently verifies its archived copy. Old reports were not rewritten to match new source.
- [archive-verification.json](archive-verification.json) records the archive containing all nine development roots and the final root, including unsuccessful attempts, full library inventories and diagnostic outputs. Fresh extraction is checked against every original inventory. It also records an exact-existing-secret scan; the key is never printed or transmitted.
- [execution.json](execution.json) and the accompanying logs document verification commands and outcomes. [DEVELOPMENT.md](DEVELOPMENT.md) records negative iterations and audit limitations.

Final raw root: `/tmp/kicadai-schematic-readability-offline-v1-final`. The local archive is `.cache/schematic-readability-offline-v1-all-runs.tar.gz`. These local paths are evidence locations, not a public artifact upload. Verification hashes establish local integrity, not independent third-party attestation.

All generation/tests were key-free and used cached Go dependencies with module downloads disabled. No live OpenAI/Gemini/provider request, new campaign, external review, push, PR, merge, release or fabrication occurred. The existing OpenAI key was used only for a local exact-secret scan. Evidence-quality and validation skills guided separation of denominators, preservation of failed attempts, and the decision to retain the readability failure.

## Next boundary

Keep the experimental profile opt-in. The next substantive phase should make the native collision/readability gate agree with actual KiCad text and wire geometry, carry stable layout policy through serialization, and add functional annotations without changing electrical net identities. It should repeat these unchanged offline cases under the same strict electrical/native checks and sealed-copy review. That phase, any new live evaluation and any remote publication require separate authorization. This report does not claim the overall milestone is complete.
