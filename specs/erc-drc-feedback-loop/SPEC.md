# ERC/DRC Feedback Loop Specification

## Purpose

Add a KiCad CLI-backed feedback loop that runs schematic ERC and PCB DRC,
captures the resulting reports, parses them into stable structured findings,
and returns actionable JSON that an AI agent can use to repair generated
schematics and boards.

The goal is to move KiCadAI from "files parse and round-trip" toward "designs
receive electrical and physical validation feedback." This project does not
make generated designs fabrication-ready by itself; it creates the feedback
surface needed to iterate toward that state.

## Background

KiCadAI now has:

- direct KiCad project, schematic, and PCB writers;
- schematic and PCB round-trip validation through `kicad-cli`;
- `inspect`, `evaluate`, `pinmap`, `roundtrip`, block generation, and
  structured JSON report conventions;
- circuit block examples that are intentionally schematic-first and not
  fabrication-ready.

The next gap is ERC/DRC:

- schematic ERC is not exposed through a first-class CLI command;
- PCB DRC is not exposed as a structured feedback report;
- generated block readiness does not yet include electrical or design-rule
  validation evidence;
- AI cannot consume KiCad validation output in a stable, repair-oriented shape.

Installed KiCad CLI evidence:

```sh
kicad-cli sch --help
# subcommands include: erc, export, upgrade

kicad-cli pcb --help
# subcommands include: drc, export, import, render, upgrade
```

## Goals

1. Run KiCad schematic ERC through `kicad-cli sch erc`.
2. Run KiCad PCB DRC through `kicad-cli pcb drc`.
3. Preserve raw KiCad reports and command stderr/stdout when requested.
4. Parse ERC/DRC output into a stable Go model.
5. Expose structured CLI commands for AI and human workflows.
6. Distinguish tool failures, skipped checks, and validation violations.
7. Add known-good and intentionally-bad fixtures for parser and CLI behavior.
8. Integrate results into project readiness reporting without overstating
   fabrication readiness.

## Non-Goals

- Replacing KiCad ERC or DRC algorithms.
- Guaranteeing fabrication readiness from a passing ERC/DRC result alone.
- Building an automatic repair engine in this project.
- Implementing full schematic routing, PCB routing, or zone refill.
- Implementing every possible KiCad report output format if one stable format
  is available.
- Depending on live KiCad IPC write calls.

## Compatibility Target

The primary execution target is the locally installed KiCad CLI used by the
round-trip harness. The current local version reports `10.0.3`; implementation
must discover and record the actual CLI path and version at runtime.

Normal Go tests must remain KiCad-free. KiCad-backed tests must be opt-in using
environment variables or explicit CLI flags, consistent with the existing
round-trip harness.

## User-Facing Commands

The preferred command namespace is `check`, because ERC/DRC are validation
checks rather than file normalization.

Initial commands:

```sh
kicadai --json check erc <project-or-schematic>
kicadai --json check drc <project-or-pcb>
kicadai --json check project <project-dir>
```

Required flags:

```text
--kicad-cli string       KiCad CLI executable path
--keep-artifacts         Keep generated check workspaces and reports
--artifact-dir string    Directory for retained check artifacts
--timeout duration       KiCad CLI timeout, for example 10s or 2m
--allowlist string       Check allowlist JSON path
```

The `check project` command should:

- discover the `.kicad_pro`;
- discover the root `.kicad_sch`;
- discover a `.kicad_pcb` if present, preferring project metadata or an explicit
  target file over name-matching heuristics;
- run ERC when a schematic is present;
- run DRC when a PCB is present;
- return skipped findings for missing schematic or PCB files only when the
  project is otherwise parseable.

## Package Boundaries

Add a new package rather than overloading `roundtrip`:

```text
internal/kicadfiles/checks/
```

The package owns:

- KiCad CLI check invocation;
- artifact workspace management or reuse of round-trip artifact utilities;
- raw report capture;
- report parsing;
- allowlist filtering;
- check result models.

The existing CLI layer in `cmd/kicadai` should only translate command-line
options to package options and serialize `reports.Result`.

## Project Context And Library Resolution

ERC and DRC must run with enough project context for KiCad to resolve settings,
library tables, local libraries, and design rules. A file-only copy is not
sufficient for many projects.

When the target is a project directory, the runner should copy the required
project context into the artifact workspace, not blindly clone every byte in the
tree. The copy filter should include KiCad project files, schematic files, board
files, drawing sheets, project-local library tables, project-local `.kicad_sym`
files, project-local `.pretty` footprint libraries, and other relative library
targets needed by ERC/DRC. It should exclude non-essential or high-churn content
such as `.git`, backup directories, generated check artifacts, fabrication
exports, large 3D model directories such as `shapes3d`, and other files that are
not needed for electrical or design-rule validation.

When the target is a schematic or PCB file, the runner should:

- locate a nearby `.kicad_pro` when present, bounded to the file directory and a
  small parent-depth limit or a repository/project boundary;
- copy the containing project directory when the file appears to belong to a
  project;
- fall back to copying only the target file when no project context exists;
- record in the result whether a full project context was available.

Standalone schematic ERC should produce a warning because rules, severities,
variables, and library mappings may differ from the full project context.

The first implementation may rely on host-global KiCad libraries for built-in
symbols and footprints. Project-local libraries must be preserved when they are
inside the copied project directory, including `sym-lib-table`,
`fp-lib-table`, `.kicad_sym`, `.pretty`, and other local relative library
targets. External absolute library paths should not be copied automatically;
they should be reported as environment-dependent check context so users and AI
agents know the result depends on the local KiCad installation. Generated
projects should prefer project-relative library paths or standard KiCad
environment variables over host-specific absolute paths so the feedback loop can
run consistently on developer machines and CI hosts.

The runner should pass through and record relevant KiCad library environment
variables when they are present, such as symbol, footprint, template, and 3D
model roots. If generated projects use standardized KiCad variables, the runner
may remap them for artifact workspaces. Tests that exercise command construction
must mock environment lookup instead of depending on a developer machine. Opt-in
integration tests should skip with a structured reason when required KiCad
library paths are unavailable.

Artifact retention must be bounded. Retained workspaces should live under the
user-provided `--artifact-dir` when supplied, and future cleanup should be able
to remove old retained workspaces by age or count. The initial implementation
must at least document that `--keep-artifacts` is intended for debugging and CI
evidence, not unlimited high-frequency loops. Temporary workspaces must be
deleted by default when `--keep-artifacts` is not set.

Artifact workspace names must be unique per invocation, using `os.MkdirTemp` or
an equivalent collision-resistant mechanism, so parallel checks cannot write to
the same workspace.

Cleanup should be registered with `defer` immediately after workspace creation.
Signal interruption may still leave orphaned directories, so retained artifact
documentation should explain where temporary check directories are created and
how to remove stale workspaces. Workspaces should use a predictable prefix so a
future cleanup command can delete stale check directories older than a configured
threshold.

For large project-local libraries, the implementation may use hard links or
copy-on-write behavior when the host filesystem supports it, but it must never
mutate linked source files. Files that KiCad may rewrite during a check must be
copied as independent files.

## Check Execution Model

### Inputs

Each check accepts:

- file path or project directory;
- file type: schematic, PCB, or project;
- KiCad CLI path;
- timeout;
- artifact retention settings;
- allowlist entries;
- optional working directory preference.

### Execution

Checks must never run KiCad against tracked files directly when KiCad may write
side effects. Use an artifact workspace and copy the required input files.

The command runner must capture:

- command path;
- command args;
- working directory;
- KiCad CLI version;
- stdout;
- stderr;
- exit code;
- elapsed time;
- raw report path.

The command runner must execute KiCad with `os/exec` argument slicing, for
example `exec.CommandContext(ctx, path, args...)`. It must not construct shell
command strings from user-controlled file paths. Target paths must be resolved
to local filesystem paths before invocation.

### Exit-Code Semantics

KiCad CLI may use non-zero exit codes for either command failure or validation
violations, depending on flags and version. The wrapper must classify:

| Condition | Result |
|---|---|
| CLI not found | skipped or error, depending on command mode |
| CLI execution failure before report creation | tool failure |
| Report exists and contains violations | check completed with findings |
| Report exists and contains no violations | pass |
| Report cannot be parsed | parser error with raw artifact path |

Violation findings should not be modeled as command execution failures. They
are successful check results with non-empty findings.

When `--exit-code-violations` is used, the runner must learn and document the
KiCad version's violation exit code, expected to be `1` for the probed KiCad 10
CLI. A nonzero violation exit code should not be treated as a tool failure by
itself when the expected report exists, was written during the current run, and
parses successfully. Other nonzero exit codes should be treated as tool failures
unless the implementation has explicit evidence that the code means "violations
found" for that KiCad version. Treat the run as a tool failure when the expected
report is missing, stale, empty, incomplete, or unparseable.

## Report Format Strategy

Implementation must first probe KiCad CLI support for report format options.
Prefer a machine-readable format if available. If KiCad only emits text or
RPT-like output, implement a conservative parser and preserve the raw report.

The parser must be defensive:

- retain unparsed lines in a diagnostic field or parser warning;
- never silently drop a violation block;
- include raw report path when artifacts are retained;
- tolerate KiCad version-specific wording changes where possible.

## Structured Result Model

Each check returns a `CheckResult`.

```go
type CheckKind string

const (
    CheckKindERC CheckKind = "erc"
    CheckKindDRC CheckKind = "drc"
)

type CheckStatus string

const (
    CheckStatusPass    CheckStatus = "pass"
    CheckStatusFail    CheckStatus = "fail"
    CheckStatusSkipped CheckStatus = "skipped"
    CheckStatusError   CheckStatus = "error"
)

type CheckResult struct {
    Kind          CheckKind       `json:"kind"`
    Status        CheckStatus     `json:"status"`
    TargetPath    string          `json:"target_path"`
    FileType      string          `json:"file_type"`
    ProjectContext string          `json:"project_context,omitempty"`
    Units         string          `json:"units,omitempty"`
    KiCadCLIPath  string          `json:"kicad_cli_path,omitempty"`
    KiCadVersion  string          `json:"kicad_version,omitempty"`
    Command       []string        `json:"command,omitempty"`
    WorkingDir    string          `json:"working_dir,omitempty"`
    ExitCode      int             `json:"exit_code,omitempty"`
    DurationMS    int64           `json:"duration_ms,omitempty"`
    ReportPath    string          `json:"report_path,omitempty"`
    Stdout        string          `json:"stdout,omitempty"`
    Stderr        string          `json:"stderr,omitempty"`
    Findings      []CheckFinding  `json:"findings,omitempty"`
    Allowed       []CheckFinding  `json:"allowed,omitempty"`
    ParserIssues  []ParserIssue   `json:"parser_issues,omitempty"`
}
```

Each finding must be stable enough for AI repair loops:

```go
type CheckFinding struct {
    ID             string         `json:"id,omitempty"`
    Kind           CheckKind      `json:"kind"`
    Severity       string         `json:"severity"`
    Rule           string         `json:"rule,omitempty"`
    Code           string         `json:"code,omitempty"`
    Message        string         `json:"message"`
    File           string         `json:"file,omitempty"`
    Sheet          string         `json:"sheet,omitempty"`
    References     []string       `json:"references,omitempty"`
    Net            string         `json:"net,omitempty"`
    Nets           []string       `json:"nets,omitempty"`
    Layer          string         `json:"layer,omitempty"`
    Location       *CheckLocation `json:"location,omitempty"`
    Objects        []CheckObject  `json:"objects,omitempty"`
    Raw            string         `json:"raw,omitempty"`
    RepairCategory string         `json:"repair_category,omitempty"`
}

type CheckLocation struct {
    X     float64 `json:"x"`
    Y     float64 `json:"y"`
    Units string  `json:"units,omitempty"`
}
```

`CheckFinding.ID` must be deterministic for the same KiCad report content while
remaining useful across small AI edits. Use a tiered canonical key before
hashing:

- ERC should prefer logical identity such as rule/code, reference designator,
  pin number when available, sheet, net, and normalized message.
- DRC should prefer rule/code, net, layer, object type, and rounded coordinates
  when available.
- Object UUIDs may be included only as a last-resort discriminator because AI
  repair operations can replace objects and change UUIDs.
- Coordinates should be normalized to a high enough precision to distinguish
  dense-board findings, such as nanometer-scale internal units or at least
  4-6 decimal places in millimeters. UUIDs or object identifiers may be appended
  as collision discriminators when the logical key is otherwise identical.
  Nets, object types, and rule/code remain primary discriminators because
  distinct violations can share the same coordinate.

The hash must not include volatile data such as absolute artifact paths,
temporary working directories, command duration, report path, stdout ordering
outside the finding, or timestamps.
Findings should be sorted by severity, rule/code, relative file, sheet,
reference, net, layer, location, message, and ID before serialization.

Structured JSON intended for AI prompts should prefer project-relative paths
where possible. Absolute paths may remain in retained artifact references for
local debugging, but user-facing summaries should not require absolute host
paths to be useful.

Repair categories should be conservative and heuristic:

| Category | Meaning |
|---|---|
| `connectivity` | Missing, dangling, or shorted connection |
| `clearance` | Copper, courtyard, silk, edge, or mask clearance |
| `outline` | Missing or invalid board outline |
| `footprint` | Missing, duplicate, mismatched, or invalid footprint |
| `net_assignment` | Net conflict or schematic/PCB parity mismatch |
| `power` | ERC power-input/output or unpowered-net issue |
| `no_connect` | Missing or conflicting no-connect marker |
| `metadata` | Missing fields, unresolved variables, or library metadata |
| `unknown` | Parser cannot classify safely |

Repair category mapping should prefer stable KiCad violation codes from JSON
reports when available. Message substring heuristics may be used only as a
fallback and must map unknown or changed wording to `unknown` instead of
guessing aggressively.

## Allowlist/Baseline Model

Add a check allowlist, similar in spirit to round-trip allowlists but separate
because validation findings are not diffs.

Each entry must include:

- reason;
- check kind;
- at least one narrow matcher.

Supported matchers:

- target filename;
- severity;
- rule/code;
- message substring;
- reference;
- net;
- layer;
- repair category.

Allowlisting must return both:

- remaining findings;
- allowed findings.

It must never make a tool execution error appear as a validation pass.

## CLI JSON Contract

For `check erc` and `check drc`, output should use the existing
`reports.Result` envelope:

```json
{
  "ok": false,
  "command": "check",
  "version": "0.1.0",
  "data": {
    "checks": [
      {
        "kind": "drc",
        "status": "fail",
        "findings": [
          {
            "severity": "error",
            "rule": "clearance",
            "message": "Clearance violation...",
            "repair_category": "clearance"
          }
        ]
      }
    ],
    "fabrication_ready": false,
    "fabrication_ready_reason": "DRC findings remain"
  },
  "issues": [
    {
      "code": "VALIDATION_FAILED",
      "severity": "error",
      "path": "check.drc",
      "message": "DRC reported 1 error"
    }
  ],
  "artifacts": [
    {
      "kind": "validation_report",
      "path": "..."
    }
  ]
}
```

`ok` should be false when unallowed error-level findings remain. Warnings may
also make `ok` false when the command is run in strict mode later, but strict
mode is not required in the first implementation.

## Fixture Strategy

Add fixtures in a project-owned location:

```text
examples/checks/
```

Initial fixtures:

- ERC pass or minimal parseable schematic, if KiCad permits a no-error minimal
  design.
- ERC fail schematic with an intentional, stable violation.
- DRC pass or minimal board, if current writer can generate one.
- DRC fail board with a stable violation such as missing outline, short, or
  clearance issue.

Fixtures should be tiny and purpose-built. They do not need to be realistic
products.

## Testing Requirements

Normal tests:

- parser tests using checked-in sample reports;
- allowlist filtering tests;
- CLI argument and JSON envelope tests with fake check runners;
- artifact path behavior tests;
- status classification tests.

Opt-in tests:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test ./internal/kicadfiles/checks
```

Optional CLI smoke:

```sh
go run ./cmd/kicadai --json --kicad-cli "$KICADAI_KICAD_CLI" check erc examples/checks/erc_fail/erc_fail.kicad_sch
go run ./cmd/kicadai --json --kicad-cli "$KICADAI_KICAD_CLI" check drc examples/checks/drc_fail/drc_fail.kicad_pcb
```

## Readiness Integration

The circuit block readiness docs must report:

- whether ERC/DRC checks were run;
- KiCad CLI version;
- fixture list;
- pass/fail/skipped status;
- remaining findings or allowlist entries.

No block may be promoted to `erc_drc_verified` unless:

- its checked-in example passes round-trip;
- relevant ERC and/or DRC checks pass with no unallowed error findings;
- pinmap/library resolver status is compatible with the claim;
- the readiness document records the evidence.

## Open Questions

1. Which KiCad report format should be treated as canonical for KiCad 10:
   text, RPT, JSON, or another export option?
2. Does `kicad-cli sch erc` require a full project context for useful results,
   or can it run reliably against standalone `.kicad_sch` files?
3. Which first DRC fixture gives the most stable cross-version result:
   missing outline, shorted pads, disconnected item, or clearance violation?
4. Should check allowlists live beside fixtures or in a top-level validation
   directory?
5. Should `evaluate project` eventually call `check project`, or remain a
   KiCad-free structural evaluator?
