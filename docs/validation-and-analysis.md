# Validation And Analysis

Project inspection, evaluation, writer correctness, transactions, round-trip validation, and KiCad ERC/DRC checks.

### Promotion Reports

Generated design workflows emit `<output>/.kicadai/design-promotion.json` when
`design create` can write project metadata in the requested output directory.
The report is a normalized gate summary for KiCad-backed promotion, separate
from the command's requested acceptance result.

Promotion gates currently cover:

- Fixture metadata;
- Workflow stages;
- Writer correctness;
- Connectivity;
- KiCad ERC/DRC evidence;
- Route completion;
- Physical-rule evidence;
- Expected artifacts.

Each report includes declared and achieved readiness, gate statuses, referenced
artifacts, machine-readable issue codes, and repair guidance. Required KiCad
ERC/DRC checks are optional to run in environments without `kicad-cli`, but
they must produce evidence before a fixture can reach `candidate` or `pass`.
Missing `kicad-cli` is visible as skipped external evidence and blocks
promotion when those checks are required.

Readiness values are `expected_fail`, `candidate`, `pass`, and `blocked`.
Promotion statuses include `pass`, `warn`, `failed`, `expected_fail`,
`unexpected_pass`, `blocked`, `skipped`, `error`, and `not_run`.
Issue severities are `info`, `warning`, `error`, and `blocked`. In issue
objects, `stage` is the workflow stage that produced or owns the issue; it is
not a promotion gate ID, although a gate such as `kicad_checks` can intentionally
wrap evidence from the workflow stage with the same name. Status uses `warn`
and severity uses `warning` because those are the current JSON enum values.

Minimal report shape:

```json
{
  "id": "led_indicator_kicad_smoke",
  "declared_readiness": "expected_fail",
  "achieved_readiness": "expected_fail",
  "status": "expected_fail",
  "gates": [
    {
      "id": "kicad_checks",
      "status": "skipped",
      "required_for": ["candidate", "pass"],
      "issue_codes": ["kicad_checks_missing"]
    }
  ],
  "artifacts": [
    {
      "path": ".kicadai/design-promotion.json",
      "kind": "promotion_report",
      "required": true
    }
  ],
  "issues": [
    {
      "code": "kicad_checks_missing",
      "severity": "blocked",
      "stage": "kicad_checks",
      "repair": "configure kicad-cli and preserve the required ERC/DRC report evidence"
    }
  ]
}
```

Use promotion reports for fixture readiness and AI workflow gating. Use
`validate board`, `writer check`, and KiCad ERC/DRC reports for the underlying
technical evidence.

### Generated Schematic Semantic Checks

Generated schematics have an in-process semantic connectivity gate before any
KiCad ERC command is required. The Go API is
`schematic.InspectGeneratedConnectivity(schematic.SchematicFile)`, with
`schematic.ValidateGeneratedConnectivity` retained as the blocking error path.
The report is intended for generator tests, writer checks, and AI-facing
feedback when a schematic is structurally wrong even if it still parses.

The report includes:

- `status`: `clean` or `blocked`;
- symbol, wire, label, no-connect, and symbol-pin-anchor counts;
- `power_symbol_count` and `power_flag_count`;
- `power_policy`: `not_required`, `requires_driver`, or `driven`;
- report-local model paths such as `wires[0].points[1]` or
  `symbols[2].pin_anchors[0]`;
- repair-oriented issue messages.

`power_policy=requires_driver` means the schematic uses KiCad `power:*`
symbols but does not express explicit driver intent. Add a real driving source
or an intentional `power:PWR_FLAG` symbol before treating the schematic as
ERC-ready. For non-standalone subcircuits, `requires_driver` can be acceptable
when the external driver requirement is explicit in the surrounding design
contract. `power_policy=driven` means a case-insensitive `power:PWR_FLAG`
library reference or a schematic symbol value equal to `PWR_FLAG` after
trimming whitespace was present; it is design intent evidence, not proof of
KiCad electrical pin typing or proof that KiCad ERC has passed.

The design API now embeds seed symbol bodies and derives default pin anchors
for the supported seed set: `Device:R`, `Device:C`, `Device:D`, `Device:LED`,
`power:GND`, `power:VCC`, `power:+3.3V`, `power:+3V3`, `power:+5V`,
`power:+12V`, `power:-12V`, `power:PWR_FLAG`, `power:VDD`, `power:VEE`, and
`power:VSS`. Common KiCad variants such as `Device:R_Small`, inductors,
transistors, and IC symbols still require explicit pin anchors or
resolver-backed metadata if callers expect generated-connectivity checks to
prove connections. This conservative default avoids pretending an unsupported
library symbol has known electrical anchors.

Current KiCad-backed caveat: the optional block-corpus KiCad smoke tests still
record expected failures for richer ERC/DRC proof. The generated LED schematic
now has better local pin-anchor alignment, but current optional KiCad CLI smoke
fixtures may still report symbol-body mismatch warnings plus remaining pin/wire
ERC findings, and the test runner can observe a nonzero external-check failure
code while preserving its report path. Treat the semantic report as a fast
pre-ERC gate, not a replacement for promoted KiCad ERC/DRC evidence.

### Connectivity-First Board Validation

`validate board` is the current board-readiness gate for generated PCBs. It
combines file parsing, structural PCB validation, net-to-pad checks, generated
connectivity checks, unrouted-net summaries, route endpoint checks, zone
evidence, and optional KiCad DRC evidence into one JSON result.

```sh
kicadai \
  validate board ./examples/07_generated_pcb
```

`validate board` accepts either a `.kicad_pcb` file, a `.kicad_pro` file, or a
project directory. When given a project directory, it chooses the board matching
the project name and reports an error for ambiguous board files.

Useful flags:

- `--strict-zones`: make zones without fill evidence blocking.
- `--require-drc`: require KiCad DRC evidence.
- `--allow-missing-drc`: reserved for future workflows; it is currently mutually
  exclusive with `--require-drc`, and missing DRC evidence is already
  non-blocking by default.
- `--kicad-cli /path/to/kicad-cli`: run KiCad DRC and include parsed findings.
- `--allowlist ./allowlist.json`: suppress known KiCad DRC findings through the
  existing check allowlist format.
- `--keep-artifacts --artifact-dir ./reports`: retain KiCad DRC reports.

The result includes stable check names:

- `pcb_structural_validation`
- `net_to_pad_validation`
- `generated_connectivity`
- `unrouted_net_validation`
- `route_completion`
- `zone_validation`
- `kicad_drc`

Missing DRC evidence is explicit but non-blocking by default. DRC findings are
blocking when DRC runs and returns unsuppressed findings. Zone fills are not
refilled in-process; a zone without filled polygons is a warning by default and
becomes blocking with `--strict-zones`.

Known limitations:

- The validator does not replace KiCad DRC or zone refill.
- File-backed validation internally normalizes a small set of fields the current
  PCB reader does not fully hydrate yet, such as footprint paths and property
  layers. This is read-only and does not modify project files on disk.
- In-process route completion uses deterministic geometric evidence and is meant
  to catch generated-board mistakes early, not to certify fabrication outputs.

Current routing support includes deterministic net ordering, single-layer
Manhattan routing, two-layer routing with vias, keepout and existing-copper
obstacles, route simplification, connectivity validation, clearance validation,
operation emission, KiCad check feedback mapping, golden routed examples,
bounded stress tests, shared `internal/pcbrules` resolution, per-net route
quality reports, net class and role-aware trace/via/layer rules, length policy
warnings/failures, explicit zone policies, coupled-net intent reporting,
repairable route diagnostics, workflow route-quality summaries, and a routing
hardening golden corpus.
Detailed design rationale and remaining hardening requirements are documented in
`specs/routing-engine-hardening/`.

Current routing limitations:

- The router is intended for simple boards and early AI workflow validation, not
  dense BGA or production autorouting.
- Routes are orthogonal grid paths. Length policies can warn or fail routes,
  but automatic length tuning/meanders, diagonal routing, curved routing,
  differential-pair routing, and impedance-aware routing are not supported yet.
- The shared rule model includes clearance matrices and neckdown constraints,
  but full DRC-grade neckdown geometry and clearance-matrix enforcement remain
  limited.
- Placement quality strongly affects routing success.
- Copper zones are treated as obstacles or unsupported policy inputs; zone-fill
  aware routing and conservative zone-sufficient proof remain intentionally
  blocked until proof evidence exists.
- Placement can hydrate bounds and compact pad summaries from resolver records.
  New-project and imported-project transaction apply can hydrate pads, pad
  shapes, through-hole metadata, text, graphics, and models. Routing still
  consumes compact pad summaries rather than full pad-stack data.
- KiCad DRC execution is integrated through the checks package, but tests still
  rely primarily on deterministic parser/fake-runner paths unless a local
  stable KiCad fixture is available.


### Writer Correctness Checks

`writer check` is the generated-file correctness gate. It is stricter than a
plain parser and more writer-focused than board readiness. It answers whether
files emitted by KiCadAI preserve project structure, schematic connectivity,
schematic-to-PCB transfer, footprint pad net assignments, copper net
references, zone references, and optional KiCad round-trip stability.

```sh
kicadai writer check ./examples/07_generated_pcb/generated_pcb.kicad_pcb
kicadai writer check --strict-diffs --allow-unrouted ./examples/07_generated_pcb
```

The command accepts a project directory, `.kicad_pro`, `.kicad_sch`, or
`.kicad_pcb` target. It returns nonzero when blocking writer issues are present,
which makes it suitable for CI and AI workflow gating. Project and `.kicad_pro`
targets can run cross-file checks. Single-file targets run the checks supported
by that file and skip checks that require a missing sibling schematic or PCB.

Stable checks:

- `project_structure`
- `schematic_parse`
- `schematic_connectivity`
- `schematic_pcb_transfer`
- `pcb_parse`
- `pcb_net_table`
- `footprint_pad_nets`
- `copper_net_references`
- `zone_net_references`
- `kicad_round_trip`

Useful flags:

- `--require-kicad-roundtrip`
- `--kicad-cli /path/to/kicad-cli`
- `--strict-diffs`
- `--allow-unrouted`
- `--keep-artifacts --artifact-dir ./reports`

Current limits:

- Some older generated examples intentionally fail writer checks because they
  lack footprint assignments or resolver-backed pinmaps.
- KiCad round-trip evidence is skipped unless a KiCad CLI path is available, or
  blocking when `--require-kicad-roundtrip` is set.
- Check names and flags use their stable CLI/API identifiers, so separators
  differ between JSON check IDs such as `kicad_round_trip` and flags such as
  `--require-kicad-roundtrip`.
- The writer gate is not a fabrication package validator.


### Inspection

Inspect KiCad projects and files:

```sh
kicadai inspect project ./examples/07_generated_pcb
kicadai inspect schematic ./examples/01_led_indicator/led_indicator.kicad_sch
kicadai inspect pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
```

Inspection reports summarize discovered files, symbol counts, footprint counts,
and reader issues.


### Evaluation

Evaluate projects and files for generated-output readiness:

```sh
kicadai evaluate project ./examples/07_generated_pcb
kicadai evaluate schematic ./examples/01_led_indicator/led_indicator.kicad_sch
kicadai evaluate pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
```

Current evaluation includes checks for missing files, duplicate references,
missing footprints, missing board outlines, disconnected pads, invalid net
assignments, and preservation conflicts. Reports include a
`fabrication_ready` field when applicable.


### Transactions

Transactions are structured edit plans. They can be validated, planned against a
target, or applied. Validation and planning include generated operation
summaries with stable IDs, and apply annotates operation-scoped failures with
the same IDs where attribution is safe.

```sh
kicadai transaction validate ./tx.json
kicadai transaction plan ./out/project ./tx.json
kicadai --overwrite transaction apply ./out/project ./tx.json
```

For AI repair loops, add `--feedback` to validation or planning. The command
keeps the raw issue list and also returns a grouped `feedback` object keyed by
stable operation IDs.

```sh
kicadai --feedback transaction validate ./examples/transactions/invalid_feedback.json
kicadai --feedback transaction plan ./out/invalid_feedback ./examples/transactions/invalid_feedback.json
```

Each transaction operation summary includes an `id`, and operation-scoped
issues include `operation_id` when KiCadAI can safely link the issue back to a
source operation. A repair agent should use that ID to find the matching
operation, edit or replace it, and rerun validation. Some issues intentionally
remain unlinked when attribution would be ambiguous, such as shared refs,
shared nets, or generic KiCad CLI findings without trace data.

Feedback summaries include:

- `operations[]`: grouped issues by operation ID, including refs, nets,
  artifacts, severity, and suggestions;
- `issues[]`: the original flat issue list;
- `summary`: operation count, issue count, blocking/error/warning counts, and
  unlinked issue count.

Transaction inputs do not need to contain operation IDs. KiCadAI derives IDs
from operation content and disambiguates identical operations. If an operation
changes, its ID is expected to change.

Supported operation kinds:

- `create_project`
- `set_board_outline`
- `add_symbol`
- `connect`
- `assign_footprint`
- `place_footprint`
- `route`
- `add_zone`
- `write_project`

`remove_symbol` is modeled but intentionally unsafe for imported apply and is
blocked by planning.

Minimal generated-project transaction:

```json
{
  "name": "demo",
  "operations": [
    { "op": "create_project", "name": "demo" },
    { "op": "set_board_outline", "board": { "width_mm": 30, "height_mm": 20 } },
    {
      "op": "add_symbol",
      "ref": "R1",
      "value": "10k",
      "library_id": "Device:R",
      "at": { "x_mm": 25, "y_mm": 25 },
      "pins": [
        { "number": "1", "x_mm": -2.54, "y_mm": 0 },
        { "number": "2", "x_mm": 2.54, "y_mm": 0 }
      ]
    },
    { "op": "assign_footprint", "ref": "R1", "footprint_id": "Resistor_SMD:R_0805_2012Metric" },
    {
      "op": "place_footprint",
      "ref": "R1",
      "at": { "x_mm": 15, "y_mm": 10 },
      "pads": [
        { "name": "1", "type": "smd", "width_mm": 1.2, "height_mm": 1.4, "net": "NET_A" },
        { "name": "2", "type": "smd", "width_mm": 1.2, "height_mm": 1.4, "net": "NET_B" }
      ]
    },
    { "op": "write_project" }
  ]
}
```

Imported-project apply is deliberately conservative. It supports adding symbols,
assigning footprints, placing or moving footprints, adding simple routes and
zones, and writing the project. It blocks unsupported raw content, unsafe
removals, arbitrary hierarchy refactors, and operations that could damage
unknown KiCad constructs. Imported writes use a project lock, atomic file
replacement, permission preservation, and fsync before rename.


### Round-Trip Validation

Round-trip commands use `kicad-cli` to save or normalize files and compare the
result:

```sh
kicadai \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  roundtrip schematic ./examples/01_led_indicator/led_indicator.kicad_sch

kicadai roundtrip pcb ./examples/07_generated_pcb/generated_pcb.kicad_pcb
kicadai roundtrip project ./examples/07_generated_pcb
```

Useful flags:

- `--keep-artifacts`
- `--artifact-dir ./examples/roundtrip_artifacts`
- `--timeout 30s`
- `--allowlist ./allowlist.json`

If `kicad-cli` is not found, round-trip checks return a structured skipped
result rather than failing the deterministic unit suite.


### ERC/DRC Checks

KiCad-backed ERC/DRC checks run through `kicad-cli`, preserve the raw JSON
report, and return structured findings for AI repair loops:

```sh
kicadai \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  check erc ./examples/checks/erc_fail/erc_fail.kicad_sch

kicadai check drc ./examples/checks/drc_pass/drc_pass.kicad_pcb
kicadai check project ./examples/checks/drc_pass
```

Useful flags:

- `--keep-artifacts`
- `--artifact-dir ./examples/check_artifacts`
- `--timeout 30s`
- `--allowlist ./check_allowlist.json`

`evaluate` reports now include skipped ERC/DRC evidence placeholders when
schematic or PCB files are present. Run `check project` when you need actual
KiCad ERC/DRC evidence. A design can be parseable and structurally evaluated
without being ERC/DRC clean or fabrication-ready.

Check output includes:

- stable finding IDs;
- KiCad rule/code, severity, references, nets, layers, locations, and raw report
  snippets when available;
- repair categories such as `connectivity`, `power`, `clearance`, `outline`,
  `footprint`, and `net_assignment`;
- an AI-friendly summary prompt grouped by category, reference, net, and layer.

Current caveat: real ERC smoke testing works with local KiCad 10.0.3. The DRC
runner and parser are implemented and covered with deterministic tests, but the
current local KiCad CLI exits before writing DRC JSON for the generated PCB
fixtures, so a stable real DRC fixture remains a follow-up.
