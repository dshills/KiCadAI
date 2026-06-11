# KiCadAI Core CLI and AI Integration Implementation Plan

Source specification: `specs/ai/CORE_CLI_SPEC.md`

## 1. Implementation Strategy

This plan builds the AI-facing capability as a CLI-first product layer over the
existing KiCad file writers, validators, round-trip tools, corpus scanner, and
`designapi.Builder`.

The guiding rule is simple: every phase must leave the CLI more useful to an
AI agent without making KiCad file generation less safe. Each phase should be
small enough to review, test, and commit independently.

MCP is intentionally out of scope. No MCP package, server, schema, or protocol
adapter should be added under this plan.

## 2. Current Baseline

The existing codebase already has:

- `cmd/kicadai` with configuration, IPC probes, document/capability commands,
  and generated-project demo commands.
- Direct KiCad project, schematic, and PCB writers.
- Project directory writing with safe path inventory and overwrite controls.
- Schematic validation and generated connectivity validation.
- PCB validation and generated connectivity validation.
- Project-level design validation.
- PCB corpus scanning.
- KiCad CLI round-trip helpers.
- A high-level `internal/kicadfiles/designapi.Builder` with `AddSymbol`,
  `Connect`, `AssignFootprint`, `PlaceFootprint`, `Route`, `AddZone`,
  `Design`, and `WriteProject`.

This plan must reuse those packages rather than create competing writer paths.

## 3. Delivery Rules

Each phase should:

- Keep normal tests independent of a running KiCad instance.
- Gate KiCad CLI behavior behind explicit configuration or skip states.
- Add unit tests for package logic.
- Add CLI tests for argument parsing and JSON output.
- Prefer structured Go APIs under `internal/...` with thin CLI wrappers.
- Avoid broad refactors of existing writer code unless a phase explicitly
  requires it.
- Preserve current CLI commands unless a compatibility break is explicitly
  documented.

Recommended verification after each phase:

```sh
gofmt -w <changed go files>
GOCACHE=/tmp/kicadai-gocache go test ./...
GOCACHE=/tmp/kicadai-gocache go vet ./...
```

Optional verification where relevant:

```sh
KICAD_CLI=/path/to/kicad-cli go test -tags=integration ./internal/kicadfiles/roundtrip
KICAD_VALIDATE_GENERATED_FILES=1 KICAD_CLI=/path/to/kicad-cli go test -tags=integration ./internal/kicadfiles/design
```

## 4. Phase 1: Structured CLI Result Foundation

### 4.1 Objective

Create the shared JSON contract that future AI-facing commands will use:
result envelopes, structured issues, artifact descriptions, and stable issue
codes.

This phase should not attempt to implement inspection, transactions, or new
generators. It creates the common language for those features.

### 4.2 Packages

Add:

```text
internal/reports
```

Possible files:

```text
internal/reports/result.go
internal/reports/issue.go
internal/reports/artifact.go
internal/reports/codes.go
internal/reports/json.go
internal/reports/result_test.go
internal/reports/issue_test.go
```

### 4.3 Types

Add `reports.Result`:

```go
type Result struct {
    OK        bool       `json:"ok"`
    Command   string     `json:"command"`
    Version   string     `json:"version"`
    Data      any        `json:"data,omitempty"`
    Issues    []Issue    `json:"issues"`
    Artifacts []Artifact `json:"artifacts"`
}
```

Add `reports.Issue`:

```go
type Issue struct {
    Code       Code     `json:"code"`
    Severity   Severity `json:"severity"`
    Path       string   `json:"path,omitempty"`
    Message    string   `json:"message"`
    UUIDs      []string `json:"uuids,omitempty"`
    Refs       []string `json:"refs,omitempty"`
    Nets       []string `json:"nets,omitempty"`
    Suggestion string   `json:"suggestion,omitempty"`
}
```

Add `reports.Artifact`:

```go
type Artifact struct {
    Kind        ArtifactKind `json:"kind"`
    Path        string       `json:"path"`
    Description string       `json:"description,omitempty"`
}
```

Add severity constants:

- `info`
- `warning`
- `error`
- `blocked`

`blocked` is reserved for toolchain or support limitations that prevent a check
from completing, such as a missing reader, missing KiCad CLI, or unsupported
preservation boundary. Design rule violations should use `error`.

Add initial issue codes:

- `UNKNOWN`
- `INVALID_ARGUMENT`
- `MISSING_FILE`
- `UNSUPPORTED_OPERATION`
- `SKIPPED_EXTERNAL_TOOL`
- `VALIDATION_FAILED`
- `MISSING_FOOTPRINT`
- `DUPLICATE_REFERENCE`
- `DUPLICATE_UUID`
- `UNKNOWN_SYMBOL_LIBRARY`
- `UNKNOWN_FOOTPRINT_LIBRARY`
- `MISSING_BOARD_OUTLINE`
- `DISCONNECTED_PAD`
- `INVALID_NET_ASSIGNMENT`
- `KICAD_CLI_FAILED`
- `ROUNDTRIP_DIFF`
- `PRESERVATION_CONFLICT`

Add artifact kinds:

- `kicad_project`
- `schematic`
- `pcb`
- `symbol_library_table`
- `footprint_library_table`
- `validation_report`
- `roundtrip_report`
- `drc_report`
- `erc_report`
- `preview`
- `bom`
- `gerber`
- `drill`
- `fabrication_package`

### 4.4 CLI Integration

Add a small helper in `cmd/kicadai` to write `reports.Result` values when
`--json` is enabled.

Keep existing JSON output shape for legacy commands if changing it would break
tests too broadly. New AI-facing commands should use the envelope from day one.

### 4.5 Error Mapping

Add helpers:

- `reports.ErrorResult(command string, issue Issue) Result`
- `reports.OKResult(command string, data any, artifacts []Artifact) Result`
- `reports.IssueFromError(err error) Issue`

Do not try to perfectly map all existing validation strings in this phase.
Start with generic `VALIDATION_FAILED` and let later phases add richer mapping.

### 4.6 Tests

Add tests for:

- JSON shape for success.
- JSON shape for failure.
- `OK` false when any issue has severity `error` or `blocked`.
- Artifact JSON omits optional empty fields.
- Issue code and severity string values are stable.
- CLI helper writes valid JSON.

### 4.7 Acceptance Criteria

- `internal/reports` exists and is covered by tests.
- New result envelope can represent success, failure, skipped external tools,
  and artifact paths.
- Existing tests pass.

## 5. Phase 2: CLI Command Restructure for AI-Facing Commands

### 5.1 Objective

Add a clean command-family structure without breaking existing commands.

This phase creates command routing for future subcommands:

```text
kicadai inspect ...
kicadai evaluate ...
kicadai transaction ...
kicadai roundtrip ...
kicadai generate ...
kicadai export ...
```

Only the command skeleton and shared flag handling are required here. Behavior
can be stubbed with structured `UNSUPPORTED_OPERATION` results for commands not
implemented yet.

### 5.2 Implementation

Update `cmd/kicadai/main.go` with:

- shared `--json` behavior for new command families;
- command parsing helpers;
- consistent usage text;
- structured unsupported-operation responses for known but unimplemented
  subcommands.

Prefer adding small command handler structs/functions instead of growing one
large switch.

Suggested internal command helpers:

```go
type commandContext struct {
    json bool
    stdout io.Writer
    stderr io.Writer
}
```

### 5.3 Commands Introduced as Skeletons

```text
kicadai inspect project <path> --json
kicadai inspect schematic <path> --json
kicadai inspect pcb <path> --json
kicadai evaluate project <path> --json
kicadai evaluate schematic <path> --json
kicadai evaluate pcb <path> --json
kicadai transaction validate <tx.json> --json
kicadai transaction plan <project> <tx.json> --json
kicadai transaction apply <project> <tx.json> --json
kicadai roundtrip schematic <path> --json
kicadai roundtrip pcb <path> --json
kicadai roundtrip project <path> --json
kicadai generate example --name <name> --output <dir> --json
kicadai generate project --request <request.json> --output <dir> --json
kicadai export preview <project> --output <dir> --json
kicadai export bom <project> --output <path> --json
kicadai export fabrication <project> --output <path> --json
```

### 5.4 Backward Compatibility

Existing commands should continue to work:

- `config`
- `ping`
- `version`
- `documents`
- `capabilities`
- `generate-led-demo`
- `generate-project`
- `plan-led-demo`
- `draw-led-demo`

If `generate example --name led_indicator` is implemented in this phase, it can
wrap existing `generate-led-demo`. Otherwise it can return
`UNSUPPORTED_OPERATION` until Phase 8.

### 5.5 Tests

Add CLI tests for:

- each skeleton command returns valid JSON;
- missing required arguments produce `INVALID_ARGUMENT`;
- unimplemented commands produce `UNSUPPORTED_OPERATION`;
- legacy commands still pass existing tests.

### 5.6 Acceptance Criteria

- The new CLI command families are discoverable.
- Stubbed commands fail in a structured way.
- Existing command tests remain green.

## 6. Phase 3: Inspection Data Models

### 6.1 Objective

Define bounded, AI-friendly inspection summaries independent of terminal
formatting.

Inspection should summarize what exists in a KiCad project without overwhelming
the AI with raw S-expressions.

### 6.2 Packages

Add:

```text
internal/inspect
```

Possible files:

```text
internal/inspect/model.go
internal/inspect/project.go
internal/inspect/pcb_scan.go
internal/inspect/generated.go
internal/inspect/issues.go
internal/inspect/inspect_test.go
```

### 6.3 Models

Add `inspect.ProjectSummary`:

```go
type ProjectSummary struct {
    Root              string             `json:"root"`
    Name              string             `json:"name,omitempty"`
    Files             []FileSummary      `json:"files"`
    Schematic         *SchematicSummary  `json:"schematic,omitempty"`
    PCB               *PCBSummary        `json:"pcb,omitempty"`
    Unsupported       []UnsupportedNode  `json:"unsupported,omitempty"`
    PreservationOnly  []UnsupportedNode  `json:"preservation_only,omitempty"`
    Issues            []reports.Issue    `json:"issues"`
}
```

Add `inspect.SchematicSummary` with:

- filename;
- format version if detectable;
- generator if detectable;
- symbol count;
- symbols by reference;
- library IDs;
- footprint assignments;
- sheet count;
- wire/label/junction/no-connect counts;
- known unsupported node count;
- issue count.

Add `inspect.PCBSummary` with:

- filename;
- format version if detectable;
- generator if detectable;
- layer count;
- net count;
- footprint count;
- pad count;
- track/via/zone/drawing/dimension counts;
- board outline presence;
- corpus scan object counts;
- unsupported object counts;
- preservation-only object counts.

### 6.4 Initial Implementation Scope

Because full readers are not implemented yet:

- Project inspection should detect expected files by path.
- PCB inspection should use existing corpus scanner and lightweight file
  metadata extraction.
- Schematic inspection should initially provide file metadata and shallow counts
  where practical, or clearly report `UNSUPPORTED_OPERATION` for deep
  structured inspection.
- Generated project inspection may use known generated design paths only when a
  model is already available in memory; do not invent fragile parsing.

### 6.5 CLI Wiring

Implement:

```text
kicadai inspect project <path> --json
kicadai inspect pcb <path> --json
kicadai inspect schematic <path> --json
```

Return `reports.Result` with inspection summary in `data`.

### 6.6 Tests

Add tests for:

- missing project path;
- project with `.kicad_pro`, `.kicad_sch`, `.kicad_pcb`;
- project missing schematic;
- project missing PCB;
- PCB scan summary from a small fixture;
- unsupported objects surfaced from PCB corpus scanner;
- schematic deep inspection skipped or shallow summary produced honestly.

### 6.7 Acceptance Criteria

- AI can ask what files and major objects exist in a project.
- PCB inspection gives useful information before full readers exist.
- Unsupported inspection depth is reported honestly as an issue, not hidden.

## 7. Phase 4: Evaluation Data Models and Internal Checks

### 7.1 Objective

Expose existing validation and connectivity checks as structured evaluation
commands.

### 7.2 Packages

Add:

```text
internal/evaluate
```

Possible files:

```text
internal/evaluate/model.go
internal/evaluate/project.go
internal/evaluate/generated.go
internal/evaluate/kicad_cli.go
internal/evaluate/issues.go
internal/evaluate/evaluate_test.go
```

### 7.3 Models

Add `evaluate.Report`:

```go
type Report struct {
    Target       string          `json:"target"`
    Checks       []CheckResult   `json:"checks"`
    Issues       []reports.Issue `json:"issues"`
    FabricationReady       bool   `json:"fabrication_ready"`
    FabricationReadyReason string `json:"fabrication_ready_reason,omitempty"`
}
```

Add `evaluate.CheckResult`:

```go
type CheckResult struct {
    Name     string          `json:"name"`
    Status   CheckStatus     `json:"status"`
    Issues   []reports.Issue `json:"issues,omitempty"`
    Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}
```

Statuses:

- `passed`
- `failed`
- `skipped`
- `blocked`

### 7.4 Internal Checks

Wrap existing checks:

- project file existence checks;
- generated `design.Validate`;
- schematic `Validate`;
- schematic `ValidateGeneratedConnectivity`;
- PCB `Validate`;
- PCB `ValidateGeneratedConnectivity`;
- PCB corpus scan checks;
- round-trip availability checks.

For checks that require full readers, return `skipped` with a structured issue
explaining the reader gap.

### 7.5 CLI Wiring

Implement:

```text
kicadai evaluate project <path> --json
kicadai evaluate schematic <path> --json
kicadai evaluate pcb <path> --json
```

Initial implementation can evaluate:

- project structure;
- PCB scan health;
- KiCad CLI parse/round-trip when explicitly requested;
- generated design models only where available from generator commands.

### 7.6 Issue Mapping

Add mapping from common validation failures to stable issue codes where
possible:

- duplicate UUID -> `DUPLICATE_UUID`;
- duplicate reference -> `DUPLICATE_REFERENCE`;
- missing footprint -> `MISSING_FOOTPRINT`;
- unresolved symbol library -> `UNKNOWN_SYMBOL_LIBRARY`;
- unresolved footprint library -> `UNKNOWN_FOOTPRINT_LIBRARY`;
- missing board outline -> `MISSING_BOARD_OUTLINE`;
- disconnected pad -> `DISCONNECTED_PAD`.

Do not overfit brittle string parsing. If a mapping is uncertain, use
`VALIDATION_FAILED` with the original message.

### 7.7 Tests

Add tests for:

- missing target path;
- project structure pass/fail;
- PCB scan evaluation pass;
- unsupported reader check skipped;
- mapped validation error code;
- JSON result ready false when errors exist.

### 7.8 Acceptance Criteria

- AI can ask "what is wrong with this project?" and receive structured issues.
- Unsupported checks are explicit.
- No normal test requires KiCad.

## 8. Phase 5: Round-Trip CLI Commands

### 8.1 Objective

Wrap existing round-trip validation package in first-class CLI commands.

### 8.2 CLI Commands

Implement:

```text
kicadai roundtrip schematic <path> --json
kicadai roundtrip pcb <path> --json
kicadai roundtrip project <path> --json
```

Options:

- `--kicad-cli <path>`;
- `--keep-artifacts`;
- `--artifact-dir <path>`;
- `--timeout <duration>` using Go duration strings such as `500ms`, `10s`, or
  `2m`;
- `--allowlist <path>`.

The allowlist file must use the existing structured round-trip allowlist format
from `internal/kicadfiles/roundtrip`; free-form regex or line-delimited formats
are not part of this plan.

### 8.3 Behavior

- If KiCad CLI path is missing and cannot be discovered, return `ok: true`
  with a skipped check unless user explicitly required KiCad CLI.
- If user explicitly provides an invalid KiCad CLI path, return `ok: false`.
- User-provided `--kicad-cli` values must resolve to a regular executable file
  before execution. The CLI must not invoke shells to run KiCad CLI paths or
  arguments.
- KiCad CLI invocations must use context-aware process execution so timeout or
  cancellation terminates the child process instead of leaving it running.
  Implementations should also terminate the process group on platforms where
  KiCad CLI can spawn child processes.
- Preserve current round-trip artifact behavior: artifact paths only when
  artifacts are kept.
- For `roundtrip project`, run PCB checks for the project PCB and discover all
  schematic sheets linked from the root schematic hierarchy. Until sheet links
  can be read reliably, restrict checks to the root schematic and mark
  hierarchy completeness as `skipped`; do not guess by validating every
  `.kicad_sch` file in the directory.
- Round-trip commands must copy files to temporary artifact workspaces and must
  never overwrite original project files. Cleanup must run on normal failures
  and context cancellation; retained artifacts are kept only when explicitly
  requested.

### 8.4 Tests

Use existing fake KiCad CLI helper patterns to test:

- schematic round-trip command success;
- PCB round-trip command success;
- KiCad CLI failure normalized to `KICAD_CLI_FAILED`;
- missing KiCad CLI skipped behavior;
- artifact paths present only with `--keep-artifacts`;
- project command with schematic only, PCB only, and both.

### 8.5 Acceptance Criteria

- Users can run round-trip checks without writing custom Go tests.
- AI receives structured round-trip differences and artifact paths.

## 9. Phase 6: Transaction Schema

### 9.1 Objective

Define and validate the JSON transaction format without applying it yet.

### 9.2 Packages

Add:

```text
internal/transactions
```

Possible files:

```text
internal/transactions/model.go
internal/transactions/schema.go
internal/transactions/validate.go
internal/transactions/plan.go
internal/transactions/model_test.go
internal/transactions/validate_test.go
```

### 9.3 Transaction Types

Add:

```go
type Transaction struct {
    Name       string      `json:"name,omitempty"`
    Project    string      `json:"project,omitempty"`
    Operations []Operation `json:"operations"`
}
```

Use a required delayed-decoding strategy for raw operation payloads:

```go
type Operation struct {
    Op    OperationKind   `json:"op"`
    Index int             `json:"-"`
    Raw   json.RawMessage `json:"-"`
}
```

`Operation` must implement `UnmarshalJSON`. The custom method captures the full
operation object in `Raw`, decodes `op`, and leaves concrete operation decoding
to the validator or executor. Implement this with an alias type or small
discriminator struct to avoid recursive unmarshaling. `Transaction` parsing must
use that custom method; default unmarshaling with an unpopulated `Raw` field is
a bug. Planning and logging must preserve `Raw` so operation fields can be
re-serialized without data loss.
`Operation` must also implement `MarshalJSON` and return the preserved `Raw`
payload after validation or planning, because the `json:"-"` tag intentionally
keeps `Raw` out of default marshaling.

Safe decode pattern:

```go
func (op *Operation) UnmarshalJSON(data []byte) error {
    var head struct {
        Op OperationKind `json:"op"`
    }
    if err := json.Unmarshal(data, &head); err != nil {
        return err
    }
    op.Op = head.Op
    op.Raw = bytes.Clone(data)
    return nil
}
```

Initial operation structs:

- `CreateProjectOperation`;
- `SetBoardOutlineOperation`;
- `AddSymbolOperation`;
- `ConnectOperation`;
- `AssignFootprintOperation`;
- `PlaceFootprintOperation`;
- `RouteOperation`;
- `AddZoneOperation`;
- `WriteProjectOperation`.

### 9.4 Validation Rules

Validate:

- transaction has at least one operation;
- operation kind is known;
- required fields are present;
- references are non-empty;
- library IDs are syntactically valid where applicable;
- coordinates are finite and in supported units;
- route has at least two distinct points and no zero-length segment;
- zone polygon has at least three points;
- pad specs have names;
- net names are non-empty for route/zone unless explicitly allowed.

Transactions must represent KiCad's `No Net` state explicitly with JSON `null`
for optional net fields. Empty strings are invalid unless a specific operation
documents otherwise.

### 9.5 CLI Wiring

Implement:

```text
kicadai transaction validate <tx.json> --json
```

Return parsed operation count and structured issues.

### 9.6 Tests

Add tests for:

- valid transaction;
- empty operation list;
- unknown operation;
- invalid JSON;
- missing required field;
- invalid route;
- invalid zone;
- operation index in issue path.

### 9.7 Acceptance Criteria

- AI can check a transaction file before attempting mutation.
- Invalid transactions fail with stable issue codes and operation indexes.

## 10. Phase 7: Transaction Planner

### 10.1 Objective

Add a dry-run planner that reads a transaction and reports what it would do,
without writing files.

### 10.2 Planner Model

Add:

```go
type Plan struct {
    Operations []PlannedOperation `json:"operations"`
    Issues     []reports.Issue    `json:"issues"`
}
```

Each `PlannedOperation` should include:

- operation index;
- operation kind;
- refs touched;
- nets touched;
- artifacts that would be written;
- whether the operation is currently supported.

### 10.3 Initial Scope

Support planning for generated-project workflows only:

- `create_project`;
- `add_symbol`;
- `connect`;
- `assign_footprint`;
- `place_footprint`;
- `route`;
- `add_zone`;
- `write_project`.

Return `UNSUPPORTED_OPERATION` for removal, imported-project mutation, and
advanced sheet operations.

### 10.4 CLI Wiring

Implement:

```text
kicadai transaction plan <project-or-output-dir> <tx.json> --json
```

For now, the target may be an output directory for a new generated project.
Existing project planning should report the reader gap clearly until readers
exist.

### 10.5 Tests

Add tests for:

- supported generated-project plan;
- unsupported removal plan;
- existing project blocked by missing reader;
- operation indexes preserved;
- planned artifacts reported for write operations.

### 10.6 Acceptance Criteria

- AI can ask "would this edit be supported?" before applying it.
- Planner does not mutate files.

## 11. Phase 8: Generated-Project Transaction Apply

### 11.1 Objective

Apply supported transactions to new generated projects through
`designapi.Builder`.

This is the first phase where AI can create non-demo KiCad output through a
general command contract.

### 11.2 Executor

Add:

```go
type ApplyOptions struct {
    OutputDir string
    Overwrite bool
    Seed string
}

type ApplyResult struct {
    Plan      Plan               `json:"plan"`
    Artifacts []reports.Artifact `json:"artifacts"`
    Issues    []reports.Issue    `json:"issues"`
}
```

Executor behavior:

1. Validate transaction.
2. Create builder from `create_project`.
3. Apply operations in order.
4. Stop on first fatal issue.
5. Validate resulting design with `design.Validate`.
6. Write project with safe writer.
7. Return artifacts.

### 11.3 Supported Operations

Implement:

- `create_project`;
- `add_symbol`;
- `connect`;
- `assign_footprint`;
- `place_footprint`;
- `route`;
- `add_zone`;
- `write_project`.

Defer:

- `remove_symbol`;
- `disconnect`;
- `remove_footprint`;
- `remove_route`;
- imported project mutation;
- hierarchical sheet mutation.

### 11.4 CLI Wiring

Implement:

```text
kicadai transaction apply <output-dir> <tx.json> --json
```

Options:

- `--overwrite`;
- `--seed`;
- `--keep-artifacts`;
- `--validate-roundtrip`;
- `--kicad-cli`.

### 11.5 Tests

Add tests for:

- transaction builds a simple LED project;
- transaction builds a two-connector breakout;
- missing `create_project` fails;
- operation failure includes operation index;
- output directory safety rules are preserved;
- design validation failure returns structured issue;
- generated project files exist.

### 11.6 Acceptance Criteria

- AI can produce a complete project from a transaction file.
- The generated project passes internal validation.
- Files are written only through the safe project writer.

## 12. Phase 9: Generated Project Inspection and Evaluation

### 12.1 Objective

Make generated-project output immediately inspectable and evaluable by the CLI
without requiring full arbitrary KiCad readers.

### 12.2 Approach

When `transaction apply` or `generate` writes a project, also write a KiCadAI
sidecar manifest:

```text
.kicadai/
  manifest.json
```

The manifest should contain:

- project name;
- generator version;
- output file paths;
- operation summary;
- artifact list;
- validation issue summary;
- optional compact design summary.
- SHA-256 hashes for generated KiCad files used by the manifest.

Do not store secrets or absolute machine-specific paths unless necessary.
Inspection and evaluation commands must verify hashes before trusting manifest
summaries. If hashes do not match before full readers exist, report the
manifest as stale and mark manifest-backed inspection fields as blocked. After
readers exist, stale manifests may fall back to live file inspection.

### 12.3 Commands Updated

Update:

```text
kicadai inspect project <generated-project> --json
kicadai evaluate project <generated-project> --json
```

to use `.kicadai/manifest.json` when present.

### 12.4 Tests

Add tests for:

- manifest written during transaction apply;
- inspect reads manifest;
- evaluate reads manifest and reruns cheap checks;
- missing manifest falls back to path inspection.

### 12.5 Acceptance Criteria

- AI can generate a project, then inspect/evaluate it in a second CLI call.
- This works before full KiCad readers are complete.

## 13. Phase 10: First High-Level Generator - Breakout Board

### 13.1 Objective

Implement the first structured one-shot generator beyond demos: a simple
breakout board.

### 13.2 Package

Add:

```text
internal/generate
```

Possible files:

```text
internal/generate/model.go
internal/generate/breakout.go
internal/generate/validate.go
internal/generate/breakout_test.go
```

### 13.3 Request Shape

Support:

```json
{
  "kind": "breakout_board",
  "name": "sensor_breakout",
  "board": {"width_mm": 50, "height_mm": 30},
  "connectors": [
    {"ref": "J1", "pins": ["VCC", "GND", "SCL", "SDA"]},
    {"ref": "J2", "pins": ["VCC", "GND", "SCL", "SDA"]}
  ]
}
```

Initial generator behavior:

- create project;
- create rectangular board outline;
- add two connectors;
- connect matching pins by net name;
- assign connector footprints;
- place connectors at opposite board edges;
- route simple direct traces where possible;
- add GND zone if requested;
- write project and manifest.

### 13.4 CLI Wiring

Implement:

```text
kicadai generate breakout --request request.json --output <dir> --json
```

### 13.5 Tests

Add tests for:

- valid two-connector breakout;
- invalid board dimensions;
- invalid connector pin list;
- generated project validates;
- generated PCB has board outline;
- generated PCB has expected footprints and nets;
- CLI returns artifacts.

### 13.6 Acceptance Criteria

- User can request a simple breakout board and open the generated project in
  KiCad.
- AI receives structured artifacts and validation issues.

## 14. Phase 11: Reader Foundation

### 14.1 Objective

Start full project readers needed for arbitrary existing design inspection and
future safe mutation.

This phase should be conservative. It does not need to parse every KiCad node
on day one, but it must preserve unsupported nodes.

### 14.2 Reader Scope

Implement readers for:

- project file metadata sufficient for name, page settings, library tables, and
  preserved JSON sections;
- schematic root file metadata, symbols, wires, labels, junctions, sheets, and
  raw unsupported items;
- PCB metadata, nets, footprints, pads, drawings, tracks, vias, zones, and raw
  unsupported items.

### 14.3 Package Placement

Prefer adding read functions next to writers:

```text
internal/kicadfiles/project/read.go
internal/kicadfiles/schematic/read.go
internal/kicadfiles/pcb/read.go
internal/kicadfiles/design/read.go
```

Use existing `sexpr` parser capabilities where possible. If parser support is
missing, add it to `internal/kicadfiles/sexpr` with tests first.

### 14.4 Preservation Rules

Readers must:

- retain unsupported top-level nodes as raw items;
- record unknown child nodes when possible;
- keep enough ordering metadata to preserve imported unsupported content
  relative to modeled content during read-modify-write;
- continue using deterministic writer ordering for newly generated content so
  Git diffs remain stable;
- avoid dropping content silently.

Read-modify-write updates must write replacement files to a temporary path in
the same directory and then rename them into place. Direct in-place writes to
KiCad project, schematic, or PCB files are not allowed for mutation commands.

### 14.5 Tests

Add tests for:

- read minimal project/schematic/PCB written by current writer;
- read KiCad demo fixtures where available;
- read unsupported raw nodes and write them back;
- read-write-read stability for modeled fields;
- malformed file errors include path and useful message.

### 14.6 Acceptance Criteria

- Inspection commands can read current generated files as structured models.
- Unsupported content is visible in summaries.
- Writer output can be read back by KiCadAI.

## 15. Phase 12: Existing Project Inspection

### 15.1 Objective

Upgrade inspection commands to use full readers for arbitrary KiCad projects.

### 15.2 Implementation

Update `internal/inspect` to:

- read project, schematic, and PCB files;
- summarize symbols, footprints, nets, routes, zones, board outline, and
  unsupported nodes;
- include child schematic summaries;
- report library table state;
- bound output for AI context.

Add output limits:

- maximum symbols listed before summarization;
- maximum footprints listed before summarization;
- maximum issues;
- include `truncated: true` flags where needed.

### 15.3 Tests

Add tests for:

- generated LED project inspection;
- hierarchical schematic inspection;
- PCB object correctness example inspection;
- truncation behavior;
- unsupported nodes reported.

### 15.4 Acceptance Criteria

- AI can inspect existing KiCad projects with useful structured summaries.
- Output remains bounded.

## 16. Phase 13: Existing Project Evaluation

### 16.1 Objective

Upgrade evaluation commands to use readers and run internal validation against
imported projects.

### 16.2 Checks

Run:

- `project.Validate`;
- `schematic.Validate`;
- `schematic.ValidateGeneratedConnectivity` where pin anchors are available;
- `pcb.Validate`;
- `pcb.ValidateGeneratedConnectivity` where geometry is sufficiently modeled;
- `design.Validate` when project-level model can be assembled;
- corpus/preservation checks for unsupported content.

### 16.3 Tests

Add tests for:

- valid generated project passes;
- duplicate reference imported schematic fails with `DUPLICATE_REFERENCE`;
- disconnected PCB pad fails with `DISCONNECTED_PAD`;
- unsupported nodes become warnings or preservation issues;
- validation skipped where geometry is insufficient.

### 16.4 Acceptance Criteria

- AI can evaluate existing projects with structured issues.
- Validation uses real parsed models where possible.

## 17. Phase 14: Safe Existing-Project Mutation Planning

### 17.1 Objective

Plan edits against imported projects without applying them.

### 17.2 Supported Planning

Plan:

- add symbol;
- assign footprint;
- connect pins by adding labels/wires where supported;
- place new footprint;
- route simple segment;
- add zone.

Return blocked issues for:

- removing objects with unsupported dependencies;
- modifying unsupported raw regions;
- edits requiring unimplemented reader fields;
- ambiguous net or pin mappings.

### 17.3 Conflict Model

Add conflict types:

- `PRESERVATION_CONFLICT`;
- `AMBIGUOUS_REFERENCE`;
- `UNSUPPORTED_IMPORTED_OBJECT`;
- `UNSAFE_REMOVE`;
- `PINMAP_UNVERIFIED`.

### 17.4 Tests

Add tests for:

- planning safe add on imported generated project;
- blocking removal of object referenced by route;
- blocking edit near unsupported preserved raw item;
- ambiguous reference failure;
- operation indexes and paths.

### 17.5 Acceptance Criteria

- AI can know whether an edit to an existing project is safe before applying.

## 18. Phase 15: Safe Existing-Project Mutation Apply

### 18.1 Objective

Apply a conservative subset of transactions to imported projects.

### 18.2 Initial Apply Scope

Support:

- add new schematic symbol;
- assign footprint to new or existing symbol;
- place new footprint;
- add simple route;
- add zone;
- update symbol value.

Defer:

- arbitrary remove;
- complex reroute;
- refactor hierarchy;
- edit unsupported raw regions.

### 18.3 Tests

Add tests for:

- apply safe symbol addition to imported project;
- apply footprint assignment;
- apply placement;
- write project preserves existing unsupported raw item;
- round-trip check after mutation with fake or optional KiCad CLI;
- blocked unsafe mutation does not write files.

### 18.4 Acceptance Criteria

- AI can safely modify a subset of existing projects.
- Unsafe edits are refused before write.

## 19. Phase 16: Pinmap and Library Intelligence

### 19.1 Objective

Build reliable symbol-footprint-pinmap verification needed before AI can claim
designs are fabrication-ready.

### 19.2 Data Model

Add a pinmap database format:

```json
{
  "symbol": "Device:Q_NPN_BEC",
  "footprint": "Package_TO_SOT_THT:TO-92_Inline",
  "pins": [
    {"symbol_pin": "1", "function": "E", "footprint_pad": "1"},
    {"symbol_pin": "2", "function": "B", "footprint_pad": "2"},
    {"symbol_pin": "3", "function": "C", "footprint_pad": "3"}
  ],
  "source": "human_verified",
  "notes": "Verify against selected transistor datasheet."
}
```

### 19.3 Commands

Add:

```text
kicadai pinmap validate <project> --json
kicadai pinmap list --json
```

### 19.4 Tests

Add tests for:

- verified mapping passes;
- missing critical mapping blocks fabrication readiness;
- mismatched symbol/footprint pin count warns or blocks;
- human notes surfaced in report.

### 19.5 Acceptance Criteria

- Critical components can be marked verified or blocked.
- AI receives clear feedback before fabrication export.

## 20. Phase 17: Review Summary and Fabrication Readiness

### 20.1 Objective

Produce a human-readable and JSON review summary that combines inspection,
evaluation, pinmap status, and artifact checks.

### 20.2 Command

Add:

```text
kicadai review project <project> --json
```

### 20.3 Readiness Rules

Fabrication readiness is false unless:

- internal validation passes;
- KiCad CLI checks pass when required;
- no blocking preservation issues exist;
- board outline exists;
- required footprints exist;
- critical pinmaps are verified;
- review checklist is generated.

### 20.4 Tests

Add tests for:

- generated project not fabrication-ready because pinmaps missing;
- ready false when DRC skipped and user requires DRC;
- ready false when warnings lack waivers;
- ready true for a controlled fixture with all required checks satisfied.

### 20.5 Acceptance Criteria

- CLI can answer "is this ready to fabricate?" conservatively.

## 21. Phase 18: Artifact Export Commands

### 21.1 Objective

Expose KiCad CLI artifact generation through structured commands.

### 21.2 Commands

Implement:

```text
kicadai export preview <project> --output <dir> --json
kicadai export bom <project> --output <path> --json
kicadai export fabrication <project> --output <dir-or-zip> --json
```

### 21.3 Behavior

- Discover KiCad CLI or accept `--kicad-cli`.
- Return skipped if unavailable unless required.
- Capture stdout/stderr.
- Return artifact paths.
- Normalize failures into `KICAD_CLI_FAILED`.
- Sanitize every generated filename inside export directories or zip archives.
  Component names, net names, project names, and user-provided labels must not
  be used as paths without removing separators, `..`, drive prefixes, and other
  traversal-sensitive forms. Sanitization must handle both `/` and `\`
  separators, Windows drive prefixes, absolute paths, and reserved Windows
  device names such as `CON`, `PRN`, `AUX`, `NUL`, `COM1`, and `LPT1`.
  Reserved Windows names must be rejected case-insensitively even when an
  extension is present, such as `AUX.gbr`. Sanitization must also remove ASCII
  control characters and truncate long generated names to a conservative limit
  while preserving uniqueness.
- Capture KiCad CLI stdout/stderr with bounded buffers or temporary log files
  so unexpectedly large external output cannot exhaust memory.

### 21.4 Tests

Use fake KiCad CLI scripts for:

- successful export;
- command failure;
- missing output artifact;
- skipped unavailable KiCad CLI.

### 21.5 Acceptance Criteria

- Generated designs can produce review and fabrication artifacts through CLI.

## 22. Phase 19: Domain Generators

### 22.1 Objective

Add practical one-shot generators that compile structured intent into
transactions and then into KiCad projects.

### 22.2 Generators

Implement in order:

1. Two-connector breakout board.
2. Sensor breakout board.
3. Power indicator or regulator board.
4. Op-amp buffer.
5. Simple headphone amplifier only after pinmap and review readiness are
   mature enough.

### 22.3 Design Principle

Generators should emit transactions internally. This keeps their output
inspectable and debuggable.

### 22.4 Tests

Each generator needs:

- request validation tests;
- generated design validation tests;
- generated file existence tests;
- inspection summary tests;
- evaluation report tests.

### 22.5 Acceptance Criteria

- At least one non-trivial design can be generated from structured intent in
  one command.

## 23. Phase 20: End-to-End AI Loop Fixtures

### 23.1 Objective

Create realistic fixtures that model how an AI would use the CLI:

1. Generate a design.
2. Inspect it.
3. Evaluate it.
4. Apply a correction transaction.
5. Re-evaluate.
6. Produce a review summary.

### 23.2 Fixtures

Add fixtures for:

- LED board;
- two-connector breakout;
- sensor breakout;
- intentionally broken disconnected pad;
- missing footprint;
- unverified pinmap.

### 23.3 Tests

Add CLI-level tests that execute the loop with temporary directories.

Normal tests should use fake KiCad CLI behavior. Real KiCad tests remain
integration-gated.

### 23.4 Acceptance Criteria

- The CLI demonstrates a complete AI correction loop without requiring a live
  AI model.

## 24. Dependency and Ordering Notes

Phases 1 through 10 are the highest-value path for near-term AI use:

```text
structured results
  -> command skeleton
  -> inspection
  -> evaluation
  -> round-trip CLI
  -> transaction validation
  -> transaction planning
  -> transaction apply for generated projects
  -> generated project manifest
  -> first breakout generator
```

Phases 11 through 15 are the path to safe existing-project modification:

```text
readers
  -> imported inspection
  -> imported evaluation
  -> mutation planning
  -> conservative mutation apply
```

Phases 16 through 20 are the path to fabrication-oriented confidence:

```text
pinmap verification
  -> review readiness
  -> exports
  -> domain generators
  -> end-to-end AI loop fixtures
```

## 25. Global Acceptance Criteria

The plan is complete when:

- all new CLI JSON commands return `reports.Result`;
- AI can inspect generated and imported projects;
- AI can evaluate generated and imported projects;
- AI can apply transactions to generated projects;
- AI can safely plan and apply a conservative subset of edits to imported
  projects;
- at least one domain generator creates a complete schematic and PCB from a
  structured request;
- generated output opens in KiCad;
- validation, inspection, and review outputs are structured and stable;
- normal tests do not require KiCad;
- KiCad-backed tests remain opt-in;
- no MCP code has been introduced under this plan.
