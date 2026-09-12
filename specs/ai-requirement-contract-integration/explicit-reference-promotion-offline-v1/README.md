# Explicit reference routing: offline phase result

As of 2026-09-11. The approved implementation and native-validation phase produced **two synthetic designs that pass electrical/native stages, but zero complete practical-board passes**. Schematic readability fails visual review. The frozen AI evaluations have not been rerun or reclassified.

## What changed

Supply domains can declare `reference_domain` in v3+ requirements. Selection uses that reference for participant return pins; lowering uses it for physical return connections; load, stimulus, control-output and behavioral measurements carry the same identity. Invalid declarations cannot fall back to a name or first-reference guess. Explicit objective references must agree with their bound endpoint domains. Empty fields preserve legacy serialized requirements and existing legacy routing behavior.

This is a bounded contract, not general multi-ground support. New pin observations and generated output ports require an explicit return or one unambiguous reference. Separate physical grounds still require reviewed isolation primitives. Common-reference and explicit `_a`/`_b` objective roles are recognized; other reference conventions fail closed rather than being guessed. Bundled participant ports still cannot be observed by silently choosing their first lane.

## Native results

Both examples are unchanged electrical variants from the preceding phase, now declaring their shared return explicitly. The controller example is the separately named 100 mA design, not a weakened version of the retained 150 mA rejection.

| Synthetic example | Components | Routed nets | ERC findings | DRC findings | Native runs | Complete pass |
|---|---:|---:|---:|---:|---:|---|
| Standalone 3.3 V regulator | 8 | 3/3 | 0 | 0 | 2 | No: readability |
| Regulator/filter/controller ADC, 100 mA | 21 | 10/10 | 0 | 0 | 2 | No: readability |

Installed KiCad 10.0.3 and actual installed symbol/footprint libraries were used. All nine required workflow stages passed in each final-source run, including strict writer round-trip without skipped checks. Both requests achieved `erc-drc`; both explicitly report `fabrication_ready: false`. No generated parts, nets, placements or routes were manually edited.

Selected electrical evidence includes 3.3 V output for both designs; predicted regulator junction temperature 103.0495 °C for the standalone design and 114.0495 °C for the controller; controller ADC cutoff 923.36128 Hz within 900–1100 Hz. The original 150 mA controller rejection remains: predicted 125.049500001 °C exceeds the 125 °C limit. Native promotion retained alternate candidate failures and used zero repair trials; it did not cherry-pick away negative engineering evidence.

## Readability and replay review

Native schematic SVGs and their PNG renders were inspected alongside board copper/silkscreen renders. The regulator sheet stacks isolated symbols vertically, uses generic composition-net labels, and overlaps labels around the regulator and power flags. The controller sheet is more severe: the MCU, op-amp, pin labels and support-net labels collide, and the functional signal/power flow is difficult to follow. These are high-confidence readability failures despite zero ERC findings. Board plots were inspected, but no assembly, thermal-layout, mechanical or fabrication-readiness approval is implied.

The 14 standalone and 23 controller common project files match byte-for-byte across their two final runs, including all schematic/PCB/project files, local libraries and tables. No content normalization was needed. However, later native SVG export created one `.kicad_prl` local-state file in each first-run tree. These additions remain in the raw inventory. **Strict whole-tree equality is false and is not claimed.** Diagnostic `.kicadai` trees are retained and hashed, not treated as identical generated design content. A future evaluation should render from disposable copies after sealing the original output inventory.

Final evidence root: `/tmp/kicadai-explicit-reference-promotion-offline-v1-final`.

- Standalone: `standalone_regulator/first` and `second`; visual evidence in `standalone_regulator/render`.
- Controller: `controller_adc_100ma/first` and `second`; visual evidence in `controller_adc_100ma/render`.
- The local compressed archive is `.cache/explicit-reference-promotion-offline-v1-final.tar.gz`.
- `verification.json` binds 17 changed Go files and a 316-file, 1,527,502,860-byte native evidence inventory. `verify.mjs` rechecks the source hashes, required stage receipts, raw ERC/DRC reports and file comparisons.
- `preservation.json` authenticates the original interface/practical evidence, archive/publication checks, sealed binaries and exact I04 fixture. All prior phase reports remain unchanged.

## Verification and audit trail

`focused-tests.log`, `race-tests.log`, `composition-race-tests.log`, `lint.log` and `vet.log` retain offline verification. The 16 provider request forms remain below the unchanged 131,072-byte cap; the largest is 126,585 bytes. No live provider requests occurred.

The final repository short suite passed: 165 packages completed, comprising 151 passes and 14 packages without tests. The longest package completed in 689.838 seconds within its unchanged 12-minute limit. Race checks passed for architecture search, behavioral intent, closed-loop synthesis and the evaluation harness, plus focused composition/reference regressions. Vet passed repository-wide; lint reported zero issues across the four affected packages. Exact commands and exit codes are in `execution.json`. Fresh archive extraction reproduced the complete inventory (`archive-verification.json`).

The first native preflight had no configured library roots. The second found the installed inventory but the new harness incorrectly treated unrelated library-entry diagnostics as a global blocker. The harness now follows the existing native runner: retain the whole inventory diagnostics and enforce all checks on selected library entries. The second preflight's console transcript was truncated by the tool; the complete inventory diagnostics from the same installed libraries are retained in the final raw evidence. Neither preflight generated a board.

The initial successful native run is retained separately under `/tmp/kicadai-explicit-reference-promotion-offline-v1-native`. It predates the explicit-only legacy-compatibility guard. Final-source verification reran both examples and their replay once in a fresh directory; it was not another live evaluation or an acceptance change. An early repository test run was terminated after correcting a synthetic adversarial fixture that lacked a reviewed isolation primitive; its partial log is explicitly superseded, not a pass.

The evidence-quality and analysis-validation skills informed separation of synthetic cases from frozen evaluation denominators, preservation of rejected runs, and the decision not to equate native-check success with readability or milestone completion. The strict provider field shape follows the required-field/closed-object rules in the [OpenAI Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs); no provider was contacted for testing.

## Remaining boundary

This phase has no PR, remote push, merge, release, fabrication or new live AI evaluation. It does not establish six complete positives or two new paired frozen baseline-to-pass cases. The next substantive work is a separately approved schematic readability/layout phase with an explicit visual acceptance rubric and sealed-before-render replay handling. The negative readability result is retained without starting that repair phase.
