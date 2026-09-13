# PCB seed and rotated-field follow-up

The two targeted defects are fixed on **2/2 unchanged development examples**: schematic annotation changes no longer reroll their PCB placement/routing, and the controller's rotated reference/value fields emit and render correctly. Both complete native projects replay byte-for-byte from an actual JSON-decoded workflow request.

This is **not** completion of the practical AI-board milestone. It adds no frozen positive passes, makes no new baseline-to-pass claim, and does not establish full functional readability or fabrication readiness. The original practical campaign remains 0/8 complete positive boards. No API/provider calls, push, PR, merge or release occurred.

## Final native results

| Unchanged example | Strict native stages, each run | Exact project replay | Historical PCB geometry | Physical connectivity |
| --- | --- | --- | --- | --- |
| Standalone regulator | 9/9, twice | 14/14 primary files | All 8 placements; 37 segments and 3 vias unchanged | 3 nets, 16 connected physical pins/pads |
| Controller ADC, recorded 100 mA case | 9/9, twice | 23/23 primary files | All 21 placements; 203 segments and 29 vias unchanged | 10 nets, 52 connected physical pins/pads; 26 intentionally unconnected pads |

All four final workflows passed schematic, schematic electrical, placement, routing, project write, writer correctness, validation, simulation and KiCad checks. Required native ERC/DRC used full project context, all severities and violation-sensitive exit codes, with zero findings. Writer checks had zero failures and zero skips. Achieved acceptance is `erc-drc`, **not fabrication-ready**.

Both requirements and electrical/PCB input contracts remain unchanged. The regulator's modeled output is 3.3 V with 103.0495 °C junction temperature. The controller's recorded 100 mA case is 3.3 V, approximately 923.36 Hz cutoff (900–1100 Hz requirement), and 114.0495 °C. These are model results, not bench measurements. The separate original 150 mA thermal-rejection regression remains in the full suite; the load or 125 °C limit was not relaxed.

## What changed

An optional, versioned placement seed is derived using the existing deterministic circuit hash with only the native annotation-profile tag omitted. It reproduces the corresponding topology-v1 seed without hard-coded example seeds. Full generation/resolution hashes retain annotation provenance. Legacy requests omit the new field and retain their existing fallback. Other physical/electrical inputs remain part of the hash; this does not promise independence from all possible drawing metadata.

The field correction cancels cardinal parent-symbol orientation when writing visible fields. The emitted-file audit checks the combined orientation. Unsupported non-cardinal inputs still fail closed. The opt-in profile, provider-facing boundaries and legacy default behavior are unchanged.

## Readability review

The corrected controller text is visible in the [final analog viewport](/tmp/kicadai-pcb-seed-rotated-fields-offline-v1-final/controller_adc_100ma/render/review-analog.png). Both whole sheets, five dense viewports, both regulator copper layers and all four controller copper layers were inspected. Individual fields are now horizontal, and no local annotation/wire crossings were observed in the reviewed supported geometry.

Full functional readability remains incomplete: remote capacitor groups, long generated names, substantial whitespace and some weak reference-to-component associations still require work. See the [local review](REVIEW.md). These findings prevent presenting these examples as complete practical-board successes.

The first development evaluation exposed a misleading numeric pass: serialized zero-degree fields still rendered vertically under rotated parent symbols. That failure and a test-fixture correction are retained in [development history](DEVELOPMENT.md). Two development native evaluations and one frozen final evaluation were used; none was silently repeated.

## Verification and provenance

- [Independent final verification](verification.json) and [read-only verifier](verify.mjs): exact request/project replay, raw native reports, physical pin/pad connectivity, board preservation, source and raw inventory.
- [Final execution](native-final.execution.json) and [native log](native-final.log): native run exited 0 in 37.143 seconds of Go test time, with exact source retained before execution.
- Final focused regressions, three-package race checks, three-package lint (zero issues), and repository-wide vet passed. The [full repository short suite](full.execution.json) passed: 151 test packages and 14 packages without tests. Its longest package, `internal/opentopologysynthesis`, took 670.049 seconds under the unchanged 720-second bound. See [full log](full.log) and [handoff QA](qa.json).
- [Historical authentication](history-authentication.json): previous 23 source files at `446e19c9`, 170 raw files, archive, earlier offline evidence and original campaign preservation. The previous 0/2 outcome remains unchanged.
- [Archive authentication](archive.json) and [member manifest](archive-manifest.json): all 1,169 data/source files verified, with macOS AppleDouble metadata separately checked. The archive was created once; its verification was corrected without rebuilding it. Exact existing-key scans found no credential in raw/source or archive contents.

Final source base: `446e19c9b934ad8026b4e9ec2996b47d9ef413f9`. Ten changed Go files are bound to `.cache/pcb-seed-rotated-fields-v1-sources/native-final.json`, SHA-256 `224a974c81b23ce07e2ff18cf9fbccc1b1b2b5876862d3cfdc2e7164b2a97e8a`. No source changed after the final freeze.

Final raw evidence: `/tmp/kicadai-pcb-seed-rotated-fields-offline-v1-final`, 386 files, 1,532,405,841 bytes; inventory SHA-256 `8404851b3f7857a869245e2380fc81cdf4d80032009be14e6cf9e6ba96a0fd5a`.

Retained archive: `.cache/pcb-seed-rotated-fields-offline-v1-all-runs.tar.gz`, 232,891,237 bytes; SHA-256 `63c409b9ebf9f2a32c1cac90abfe4140b093e8bde1369f35dfb95de5ff3d5cdf`. It contains both development raw trees, the final raw tree and all eleven execution source snapshots. Logs and reports are retained in this repository phase directory. Native tooling: KiCad 10.0.3, Go 1.26.8 darwin/arm64, golangci-lint 2.13.1.

To recheck existing evidence without generation, run `node specs/ai-requirement-contract-integration/pcb-seed-rotated-fields-offline-v1/verify.mjs` and `node specs/ai-requirement-contract-integration/pcb-seed-rotated-fields-offline-v1/verify-archive.mjs`. The archive verifier uses the existing environment key solely for an in-memory local exact-secret scan; it makes no network calls. The historical authenticator and run/archive creators are create-exclusive and should not be rerun into existing receipt paths.

## Remaining milestone work

The six-positive/two-new-paired-design target, practical functional-readability evidence, new authorized live evaluation and reviewed PR remain unachieved. The next technical need is reusable functional grouping and clearer local role/reference presentation while retaining the newly verified PCB and replay invariants. That is a separate bounded phase, not an implied continuation or authorization for another evaluation.
