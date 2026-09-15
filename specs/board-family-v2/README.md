# Two deterministic board families — live acceptance failed

**September 15, 2026:** the latest separately approved, frozen indexed-request-04 evaluation completed all 14 requests in 59.48 seconds: **9/14 complete passes, 11/14 correct application outcomes and 3/5 useful native board bundles**, at a ledger-estimated USD 0.020037. All responses and accounting authenticated, but semantic reliability still failed acceptance. See the [latest complete results, source-bound review and evidence](indexed-evaluation-04/README.md). PR #14 remains draft; no further live requests are authorized. Do not use the natural-language workflow unattended or treat native validation as proof that every requested behavior is supported.

The earlier September 14 baseline (8/14 application, 4/14 raw, including an unsupported heater request that generated a board) remains unchanged in [historical results](RESULTS.md) and [review](REVIEW.md). The later failed typed-v2 and stopped indexed-request-03 records also remain historical evidence; none was repaired or replaced. The usage instructions below describe the unchanged deterministic generator and historical default language path, not acceptance of the new experimental path.

The shared `kicadai-board-family` command supports the existing BMP280 pressure board and the SHT31 temperature/humidity board. Both use the fixed ESP32 controller/support design. This is bounded family selection and deterministic construction, not arbitrary schematic or board synthesis.

Offline inspection:

```sh
go run ./cmd/kicadai-board-family --list-families
```

Offline generation plus native/export validation:

```sh
go run ./cmd/kicadai-board-family \
  --config specs/board-family-v2/configurations/sht31-standard.json \
  --output /tmp/kicadai-sht31-new-project \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```

The output directory must be new. Use `sht31-fast.json` for the second SHT31 profile; the three existing `examples/board-family-v1/*/configuration.json` configurations remain supported. KiCad 10.0.3 is required for this qualification version. Native subprocesses do not inherit provider keys.

Each successful command produces native project and library files, configuration/electrical reports, BOM JSON/CSV, schematic/PCB previews and `validation.json`. The `manufacturing` directory adds nine Gerber layers with job metadata, PTH/NPTH drills and SVG maps, a drill report, all-component placement CSV and a source-bound file-hash manifest. Any failed native gate withholds exports; any failed export gate makes the command fail. Existing manufacturing directories are never silently overwritten.

Placement coordinates use the absolute board origin, millimeters and negative native-board Y. The CSV includes through-hole headers and requires assembler confirmation of orientations. KiCad's Gerber-job dimensions include outline stroke width; the nominal centerline is 120x80 mm. Native timestamps are retained in raw exports. These files do not constitute an order, machine setup, fabrication signoff or proof of working hardware.

The two-family natural-language contract is implemented and can be inspected without a call:

```sh
go run ./cmd/kicadai-board-family \
  --export-live-contract /tmp/kicadai-two-family-contract.json
```

The separately approved 14-request/USD 1.00 final batch is complete and its request slots are exhausted. The old 41-request budget and ledger also remain exhausted and unchanged. Neither ledger may be reset or reused as new authority. See the [actual approval](evaluation/APPROVAL-01.json) and [recorded results](RESULTS.md); the low estimated cost does not authorize additional attempts.

See [work log](WORK.md), [SHT31 restrictions](SHT31.md), [consolidated electrical/assembly-data review](ELECTRICAL_ASSEMBLY_REVIEW.md), [published examples](../../examples/board-family-v2/README.md) and [current deterministic replay](evidence/deterministic-replay-02.json). All five explicit configurations passed two fresh rounds of native/export validation after the SHT31 mask/paste correction. The five offline example bundles contain 256 byte-identical copied files; 201 deliverables pass repeat-generation comparison. Earlier manufacturing-review records remain historical checkpoints, not qualification of the revised footprint. Final live evidence is complete but acceptance failed; physical assembly/bench qualification is a separate milestone.

The schema uses closed objects and family-specific nested alternatives following [OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs). The existing pinned GPT-4.1-mini model, transport and retry/accounting policy were not migrated. Export options were checked against the installed executable and [KiCad 10 CLI documentation](https://docs.kicad.org/10.0/en/cli/cli.html).

The separate v2 budget controls and fixed-batch runner are now offline-qualified. [Runtime 01](evaluation/runtime-01.json) binds the budget-enabled binary, selected Go/assembly/embedded dependencies, tests and generated artifacts. Five fresh explicit-configuration builds still match all 201 previously reviewed deliverables; the frozen model payload is unchanged. This is not live language acceptance. Read [execution safeguards and review](evaluation/RUNNER-REVIEW-01.md) before running the offline preflight:

```sh
node specs/board-family-v2/evaluation/run-language.mjs --check
```

The [budget policy](evaluation/budget-01.json) is **not spending authorization**. The separate approval and final ledger now exist and are bound to the retained evaluation. Frozen preparation documents correctly retain their pre-approval status as history. Do not invoke `--live` or `--resume` again: every planned case has been attempted. Use the offline post-run audit in [RESULTS.md](RESULTS.md) to verify the preserved publication.
