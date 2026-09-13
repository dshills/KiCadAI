# Two deterministic board families — integration in progress

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

No live requests are authorized for this milestone yet. The old 41-request budget is exhausted and its ledger must not be reset or reused as new authority. A new, small final evaluation requires a separately approved request/dollar budget after offline review.

See [work log](WORK.md), [SHT31 restrictions](SHT31.md), [manufacturing visual review](MANUFACTURING_REVIEW.md) and [deterministic replay](evidence/deterministic-replay-01.json). All five configurations pass two fresh rounds of native/export validation. Manufacturing-data review and timestamp-normalized replay are recorded, with explicit limitations. Consolidated electrical/assembly review, published examples, live acceptance and updated PR CI remain incomplete.

The schema uses closed objects and family-specific nested alternatives following [OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs). The existing pinned GPT-4.1-mini model, transport and retry/accounting policy were not migrated. Export options were checked against the installed executable and [KiCad 10 CLI documentation](https://docs.kicad.org/10.0/en/cli/cli.html).
