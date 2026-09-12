# Retained development history

This phase starts at `446e19c9`, under the bounds in PLAN.md. No historical run is replaced. These are two unchanged development examples, not benchmark cases.

## Targeted regressions before native evaluation

The initial test construction exposed two invalid test fixtures: a synthetic multi-unit graph whose unit IDs do not satisfy the workflow's explicit-component contract, and a reserved `Value` placed in the custom-fields list. The fixtures were corrected to the checked-in RC filter and proper native properties. The subsequent focused test failed on the intended defects: no independently serialized placement seed, and rejection of cardinally rotated fields at 90/180/270 degrees. Both passed after the first implementation.

The first direct test commands were key-free, used cached Go dependencies, `-count=1` and a three-minute bound. Their outputs remain in the task history. The create-exclusive runner introduced next retains subsequent complete logs, exit codes and exact source snapshots as repository receipts.

## Native development 1

`native-dev1.execution.json` records four successful nine-stage workflows. Independent verification confirms preserved topology-v1 PCB geometry and byte-exact JSON/project replay for both examples. `native-dev1-pre-crops-verification.json` is the immutable verification taken after the initial native export and before additional dense crops/layer exports; its raw inventory intentionally predates those added exports.

Visual inspection of `controller_adc_100ma/render/review-analog.png` showed C1, R1 and R2 reference/value text still vertical. Setting the serialized property angle to zero does not cancel the parent symbol transform. The old numeric audit accepted the same mistaken horizontal bounds, so its success was insufficient. **Development 1 is a visual-check failure**, not a complete pass. Its exact source and all native outputs/crops are retained.

## Native development 2

The reusable correction cancels the parent symbol's cardinal angle in each visible property/field and audits the combined orientation. The second and final permitted development evaluation again passed all four nine-stage workflows. The controller analog crop now shows horizontal C1/22n, R1/10k and R2/10k text, with unchanged symbol and wire geometry.

The expanded all-cardinal unit matrix initially failed serialized collision checks for zero/180-degree parent symbols because the fixture supplied horizontal custom pin offsets for the native capacitor template. `focused-dev2.log` retains those failures. The fixture now requests the template's physical anchors and passes its actual parent rotation to AddSymbol, rather than changing rotation after pin construction. No production source changed after native development 2; only that test fixture changed before final freeze. `focused-final.log` passes all focused tests.

## Final boundary

Two development native evaluations were used. A single final native evaluation follows with exact source frozen in `.cache/pcb-seed-rotated-fields-v1-sources/native-final.json`. No further source repair or native rerun is permitted after that final evaluation. Lint/focused checks from the first implementation are explicitly supplementary, not final-source evidence; final checks have distinct names.
