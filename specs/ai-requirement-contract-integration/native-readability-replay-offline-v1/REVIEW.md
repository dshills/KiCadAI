# Local review and evidence validation

## Assessment

**Share the negative result with caveats; do not promote the experimental profile.** This is a local review by the implementing agent, not an independent or external-model review. No external provider review was requested or run in this phase. Data-quality and analysis-validation workflows were used to distinguish evidence authenticity, native tool passes, replay, preservation, and overall milestone acceptance.

The unit of analysis is an unchanged development example, not an individual successful subprocess. Planned denominator: two examples. Final workflow rows: three (two standalone generations, one blocked controller generation). Complete phase passes: zero. Missing controller outputs are explicitly unavailable, not treated as zero findings.

## Blocking findings

1. **PCB preservation failure, high confidence.** Eight of eight standalone component poses changed across phases, with routes changing from 37 segments and three vias to 38 segments and five vias. The exact physical net/pad mapping still agrees, but that does not meet the board-geometry invariant. The placement seed comes from the generation hash (`internal/designworkflow/explicit_pcb.go:36`); that hash changes with the new drawing profile. Both previous/current poses, route digests and seed hashes are retained in `verification.json`. Further work must isolate electrical/physical identity from drawing-only metadata without rewriting old receipts or adopting a fixture-specific seed.

2. **Rotated native fields unsupported, high confidence.** Final controller project writing rejects `C1.property.7`. The retained add-symbol transaction shows a 90-degree capacitor with its reference and value at 270 degrees. Horizontal centered-field estimates cannot certify those glyphs. Earlier dev5/dev11 controller passes predate this final guard and are not valid evidence of supported rotated-field readability. The final run was not repaired or repeated.

3. **Functional readability remains partial.** The source-derived guide makes connector mappings and net roles explicit, but requires cross-referencing long source IDs. Capacitor/support groups remain physically dispersed and many connections use remote labels. This is useful traceability, not a demonstration of compact, naturally flowing functional-block schematics. No full practical-readability pass is claimed.

## Visual review

Inspected the final standalone sheet at 2200-pixel width and its upper/lower native-SVG viewports at 254 DPI (10 pixels/mm). There is one unique final sheet; both serialized runs' complete 14-file primary trees are byte-identical, so the inspected first-run sheet also represents the second run. The controller has no final emitted sheet; earlier images are not substituted.

Viewed artifacts under `/tmp/kicadai-native-readability-replay-offline-v1-final/standalone_regulator/render/`:

- `offline_regulated_output.png`: full sheet, including all 25 guide lines. Text stays inside the page. Guide indentation/alignment and long identifiers are awkward; the large unused region and dispersed capacitors remain visible.
- `review-upper.png`: all four capacitor labels, values and references. No observed label/field/wire glyph crossing in the inspected area. The viewport intentionally cuts part of the distant guide; the full-sheet export was checked for actual clipping.
- `review-lower.png`: U1, J1/J2/J3 and both power flags. The earlier J2/ground-label collision is absent; pin numbers and adjacent labels are legible.
- `review-F.Cu.png` and `review-B.Cu.png`: both generated copper layers. Native DRC and the independent pin/pad audit supply connectivity evidence; visual inspection does not establish fabrication readiness or thermal performance. These layers differ from the preceding version, as recorded above.

The emitted annotation receipt covers 18 labels, ten symbols (eight physical plus two power flags), and 25 guide lines. It reports no supported-geometry collisions for the standalone. Bounds are conservative estimates, not a general KiCad typography proof. Unknown geometries, non-local label kinds, hierarchy and rotated fields are unsupported by the experimental certification path. `Design()` remains a snapshot API; write-path checks apply through `WriteProject` / `WriteSchematicProject`.

## Evidence quality and calculations

`verify.mjs` independently checks unique stage identities, unchanged requirements and physical inputs, closed-loop assertion coverage and numeric limits, all required native flags (`--severity-all`, `--exit-code-violations`), raw ERC/DRC findings, zero writer skips, original seals, actual JSON replay and exact physical symbol/pad endpoints. It records the PCB comparison as a failed invariant. It checks the exact expected controller failure, skipped downstream stages, absence of a simulation stage and absence of emitted or replayed controller projects.

Standalone promotion remains 3.3 V and 103.0495 degrees C in the modeled case. Controller promotion remains 3.3 V, cutoff 923.3612800239786 Hz, and 114.049500001452 degrees C at the separately recorded 100 mA operating point. These are model results; there is no bench measurement, hardware validation, manufacturing approval or new AI benchmark success.

The predecessor's 16 changed source files were authenticated at its own commit, not compared with the edited worktree. Its 380-file raw inventory and 758,924,689-byte archive still match recorded digests. The older frozen interface/practical campaigns, counterexample fixture and sealed evaluator binaries were also reauthenticated. Local exact-secret scans found no existing credential in checked evidence or changed publication files.

## Code review notes

The new profile is explicit; version/rank-policy validation fails closed. Inferred rank semantics survive real JSON decoding, while explicit groups remain fixed. Label search is deterministic and capped; disconnected same-named wire islands are not interchangeable. Final drawing projection does not feed changed annotation positions into subsequent builder calls. Supported KiCad 10 notes receive native save order/flags, and only the fully modeled default-font form is decoded structurally; other effects remain raw preservation content.

The reading guide uses existing IR references, IDs, pin functions and net roles without adding electrical meaning. It assumes the trusted lowering path has resolved references; it is not a new natural-language interpretation layer or a newly exposed provider schema. Full generic-layout, hierarchy, typography and presentation coverage is not claimed.

Focused regressions, six-package race checks, seven-package lint and repository vet are recorded in the associated logs. The exact full-suite outcome and its fixed 12-minute per-package limit are recorded in `execution.json` and `full-tests.log`; it must not be inferred from the shorter checks. No review finding authorizes new repairs after the frozen final evaluation.
