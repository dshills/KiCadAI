# ERC/DRC Feedback Loop Implementation Plan

## Objective

Implement a KiCad CLI-backed feedback loop that can run schematic ERC and PCB
DRC, preserve raw KiCad evidence, parse results into stable structured findings,
and expose those findings through the CLI for AI-assisted review and repair.

This plan intentionally keeps normal test runs independent of an installed KiCad
binary. Real KiCad execution is opt-in and used for smoke/integration coverage.

## Implementation Rules

- Keep KiCad execution isolated behind a small runner interface so tests can use
  deterministic fake command output.
- Preserve raw reports, stdout, stderr, command arguments, working directory,
  KiCad version, duration, and exit code for every check run.
- Treat KiCad CLI process failures differently from design-rule violations.
- Do not claim fabrication readiness from parser success alone.
- Do not mutate source projects while checking; use copied workspaces or explicit
  artifact directories.
- Keep parsers conservative. Unknown report content must be preserved and surfaced
  as parser issues instead of being silently discarded.
- Commit after each implementation phase once tests and review pass.

## Phase 1: KiCad CLI Report Format Probe

### Goals

Determine the concrete command-line behavior and report formats available from
the installed KiCad CLI for schematic ERC and PCB DRC.

### Tasks

- Inspect `kicad-cli sch erc --help` and `kicad-cli pcb drc --help`.
- Record supported output/report flags, default report format, and exit-code
  behavior when violations are present.
- Run ERC/DRC against one known-good generated project and one intentionally bad
  project when suitable fixtures exist.
- Save representative raw reports under test data only if they are stable and do
  not contain machine-specific paths that would make tests brittle.
- Document any KiCad 10-specific behavior that affects parsing or command
  invocation.

### Acceptance Criteria

- The implementation has a clear command contract for ERC and DRC invocation.
- Any report samples used by parser tests are checked in as deterministic files.
- No normal tests require a local KiCad installation.
- The phase review confirms the selected report format is the most stable option
  available from KiCad CLI.

## Phase 2: Checks Package Skeleton

### Goals

Create the internal package boundary and stable data model for ERC/DRC checks.

### Tasks

- Add `internal/kicadfiles/checks`.
- Define `CheckKind`, `CheckStatus`, `CheckResult`, `CheckFinding`,
  `CheckOptions`, parser issue types, and command metadata types.
- Add a command runner interface that captures command path, args, working
  directory, stdout, stderr, exit code, duration, and execution error.
- Add artifact path planning without running KiCad.
- Add unit tests for model defaults, artifact naming, and status classification
  helpers.

### Acceptance Criteria

- Package compiles independently.
- Tests cover pass, fail, skipped, and error status classification.
- No CLI command integration is required yet.

## Phase 3: Report Parsers And Allowlist Filtering

### Goals

Parse ERC/DRC reports into structured findings and support narrow allowlisting
for expected known issues.

### Tasks

- Implement ERC report parser using Phase 1 samples.
- Implement DRC report parser using Phase 1 samples.
- Preserve unparsed report lines or records as parser issues.
- Map known KiCad severities into project severities.
- Map findings into repair categories such as connectivity, clearance, outline,
  footprint, net assignment, power, no-connect, metadata, and unknown.
- Implement allowlist matching by kind, target filename, severity, rule/code,
  message substring, reference, net, layer, and repair category.
- Ensure allowlists can suppress design findings but cannot convert tool
  execution errors into pass results.

### Acceptance Criteria

- Parser tests cover clean reports, violation reports, malformed reports, and
  unknown sections.
- Allowlist tests prove broad accidental suppressions are hard to create.
- Raw report content remains available even when parsing is incomplete.

## Phase 4: KiCad CLI Execution Harness

### Goals

Run KiCad ERC/DRC through the package API while keeping project files untouched.

### Tasks

- Add KiCad CLI discovery through explicit option, environment variable, and
  `PATH`.
- Add version probing and include the version in `CheckResult`.
- Implement `RunERC` for schematic files and project-scoped schematic checks.
- Implement `RunDRC` for PCB files and project-scoped PCB checks.
- Create artifact workspaces under a caller-provided directory or a temporary
  location.
- Copy the required project context into a unique artifact workspace when a
  project context is available, including `.kicad_pro`, all project `.kicad_sch`
  files needed for hierarchical designs, board files, `sym-lib-table`,
  `fp-lib-table`, project-local `.kicad_sym` files, local `.pretty` footprint
  libraries, and relative local library paths.
- Discover hierarchical schematic sheets by parsing the root schematic sheet
  references or by using KiCad project metadata when available.
- Exclude files that ERC/DRC do not need, such as `.git`, backups, generated
  check artifacts, fabrication exports, and large 3D model directories.
- For standalone schematic or PCB files without a nearby `.kicad_pro`, copy only
  the target file and record that the check ran without full project context.
- Emit a warning for standalone schematic ERC because results may differ from
  project-scoped ERC settings.
- Preserve external absolute library references as environment-dependent context
  rather than copying arbitrary host directories into the workspace.
- Detect and record relevant KiCad library environment variables, and make tests
  mock that environment instead of depending on a developer machine.
- Delete temporary workspaces by default unless artifact retention is explicitly
  requested.
- Classify results using both process outcome and parsed findings.
- Add fake runner tests for success, violations, missing binary, timeout, and
  malformed output.
- Add opt-in integration tests gated by `KICADAI_RUN_KICAD_CLI=1`.

### Acceptance Criteria

- Normal `go test ./...` passes without KiCad installed.
- Opt-in tests can run against a real KiCad CLI when configured.
- Source projects are not modified by a check run.
- Reports and command evidence are retained when `--keep-artifacts` behavior is
  requested by callers.
- Results indicate whether the check ran with full project context, partial
  context, or standalone file context.

## Phase 5: CLI Commands

### Goals

Expose ERC/DRC feedback through `kicadai` in a form suitable for humans and AI
agents.

### Tasks

- Add `kicadai check erc <project-or-schematic>`.
- Add `kicadai check drc <project-or-pcb>`.
- Add `kicadai check project <project-dir>` to run applicable checks.
- Add flags:
  - `--json`
  - `--kicad-cli`
  - `--artifact-dir`
  - `--keep-artifacts`
  - `--timeout`
  - `--allowlist`
- Return structured JSON using the existing report envelope conventions.
- Print concise text summaries when `--json` is not used.
- Set process exit codes so CI can distinguish pass, design violations, and tool
  errors.

### Acceptance Criteria

- CLI tests cover argument validation, JSON shape, text summaries, and exit-code
  mapping.
- JSON output includes raw artifact references and structured findings.
- `check project` reports both ERC and DRC results when both inputs are present.

## Phase 6: Known-Good And Known-Bad Fixtures

### Goals

Add small fixtures that prove generated designs can be electrically meaningful,
not only parseable.

### Tasks

- Add minimal ERC-pass schematic fixture.
- Add minimal ERC-fail schematic fixture with an intentional issue.
- Add minimal DRC-pass PCB fixture with valid outline, footprints, nets, and
  simple routing.
- Add minimal DRC-fail PCB fixture with at least one intentional clearance,
  disconnected pad, invalid net, missing outline, or zone issue.
- Add fixture README documenting the intended outcome for each design.
- Ensure fixtures are stable across KiCad save/upgrade behavior where practical.

### Acceptance Criteria

- Parser tests include fixture-derived report samples.
- Opt-in KiCad integration tests run against the fixtures.
- Fixture documentation states which failures are intentional.

## Phase 7: Evaluation And Readiness Integration

### Goals

Connect ERC/DRC feedback to the existing inspect/evaluate/readiness flow without
overstating guarantees.

### Tasks

- Add ERC/DRC check status to project evaluation output where applicable.
- Record the KiCad CLI version and check timestamp in readiness evidence.
- Require round-trip pass plus applicable ERC/DRC pass before marking generated
  outputs as electrically validated.
- Update docs to explain the difference between parseable, round-trip safe,
  ERC/DRC clean, and fabrication-ready.
- Add examples showing AI-consumable feedback output.

### Acceptance Criteria

- Evaluation reports can include ERC/DRC evidence when checks are requested.
- Documentation clearly states remaining limitations.
- Existing evaluate/inspect behavior remains backwards compatible.

## Phase 8: Feedback Loop Ergonomics

### Goals

Make check output directly useful for AI repair loops and future automated
editing.

### Tasks

- Group findings by repair category, file, sheet, reference, net, and layer.
- Add stable finding identifiers for deterministic comparison across runs.
- Add suggested repair hints for common categories without automatically editing
  files.
- Add a compact summary suitable for prompt context.
- Add regression tests for stable ordering and deterministic JSON output.

### Acceptance Criteria

- The same input produces stable finding order and stable IDs.
- AI-facing summaries are concise but retain enough evidence to guide repairs.
- Repair hints are advisory and do not hide underlying KiCad messages.

## Phase 9: Review, Documentation, And Commit Hygiene

### Goals

Finish the feature with reviewable commits and clear operating instructions.

### Tasks

- Run `gofmt` on touched Go files.
- Run `go test ./...`.
- Run opt-in KiCad tests if KiCad CLI is available and the environment is
  configured.
- Run `prism review staged` before each phase commit.
- Address actionable review findings before committing.
- Update README or relevant docs with command examples and test instructions.
- Commit each completed phase with focused commit messages.

### Acceptance Criteria

- All normal tests pass.
- Prism review has no unresolved blocking findings.
- Commits are phase-scoped.
- The final documentation explains how to run checks, how to preserve artifacts,
  and how to enable KiCad-backed tests.

## Suggested Commit Sequence

1. `Probe KiCad ERC DRC report formats`
2. `Add ERC DRC check models`
3. `Parse ERC DRC reports`
4. `Run KiCad ERC DRC checks`
5. `Expose ERC DRC check commands`
6. `Add ERC DRC check fixtures`
7. `Integrate ERC DRC readiness evidence`
8. `Improve ERC DRC feedback summaries`
