# KiCad Round-Trip Validation Phased Implementation Plan

## Goal

Implement the round-trip validation project described in
`specs/pcb-writer/ROUND_TRIP_VALIDATION_SPEC.md`.

The harness must let us generate KiCad files, run them through KiCad CLI, compare
the KiCad-saved result against our generated output, and turn unexpected rewrites
into actionable test failures.

## Working Rules

- Keep each phase independently reviewable and commit-ready.
- Do not mutate checked-in examples during validation.
- Keep KiCad-backed tests opt-in with `KICADAI_RUN_KICAD_CLI=1`.
- Keep normal `go test ./...` green without KiCad installed.
- Use temporary directories for generated and round-tripped files.
- Preserve artifacts only when explicitly requested.
- Run `gofmt -w` on changed Go files.
- Run `go test ./...` after each phase.
- When KiCad is available, also run:

```sh
KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip
```

## Phase 1: Package Skeleton And Environment Gating

### Objective

Create the `roundtrip` package with opt-in KiCad CLI gating, CLI discovery, and
configuration helpers.

### Implementation Tasks

1. Add package:

```text
internal/kicadfiles/roundtrip
```

2. Define core types:

```go
type KiCadCLI struct {
    Path string
}

type Options struct {
    KeepArtifacts bool
    ArtifactDir   string
    Timeout       time.Duration
}

type FileType string

const (
    FileTypePCB       FileType = "pcb"
    FileTypeSchematic FileType = "schematic"
    FileTypeProject   FileType = "project"
)
```

3. Implement environment helpers:

```go
func EnabledFromEnv() bool
func OptionsFromEnv() Options
func DiscoverCLI() (KiCadCLI, error)
```

`OptionsFromEnv` must read `KICADAI_KEEP_ROUNDTRIP_ARTIFACTS` and
`KICADAI_ROUNDTRIP_ARTIFACT_DIR`.

4. CLI discovery order:

- `KICADAI_KICAD_CLI`, if set.
- `kicad-cli` from `PATH`.
- `/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli`.

5. Add helper for KiCad version capture:

```go
func (cli KiCadCLI) Version(ctx context.Context) (string, error)
```

### Tests

- `EnabledFromEnv` returns true only for `KICADAI_RUN_KICAD_CLI=1`.
- `OptionsFromEnv` reads artifact settings.
- `DiscoverCLI` honors `KICADAI_KICAD_CLI`.
- `DiscoverCLI` returns a useful error when no executable is found.
- Version command errors include the CLI path.

### Acceptance Criteria

- Package compiles.
- Unit tests pass without KiCad installed.
- No KiCad process is spawned unless an explicit test asks for it.

## Phase 2: Normalization And Diff Reporting

### Objective

Build the file comparison layer used by round-trip results.

### Implementation Tasks

1. Define result and difference types:

```go
type Result struct {
    FixtureName        string
    FileType           FileType
    KiCadCLIPath       string
    KiCadVersion       string
    OriginalPath       string
    RoundTrippedPath   string
    RawDiffPath        string
    NormalizedDiffPath string
    SummaryPath        string
    Stdout             string
    Stderr             string
    ExitCode           int
    Equal              bool
    Differences        []Difference
}

type Difference struct {
    Category string
    Section  string
    ObjectID string
    Message  string
}
```

2. Add text normalization:

- Convert CRLF and CR to LF.
- Trim trailing whitespace per line.
- Ensure exactly one final newline.

3. Add raw and normalized diff generation.

4. Use a Go-native unified diff implementation so round-trip tests do not
   depend on platform-specific external `diff` behavior.

5. Add `CompareFiles`:

```go
func CompareFiles(originalPath, roundTrippedPath string, opts Options) (Result, error)
```

6. Include short, readable first-difference text for test failures.

### Tests

- Normalization handles CRLF input.
- Normalization trims trailing whitespace.
- Normalization preserves semantic layer-number changes.
- Equal normalized content reports `Equal=true`.
- Different normalized content reports `Equal=false`.
- Diff files are written when artifacts are preserved.

### Acceptance Criteria

- Comparison works independently of KiCad.
- Failure messages identify both compared files.
- Semantic changes remain visible after normalization.

## Phase 3: Artifact Directory Management

### Objective

Make failed round-trip runs debuggable without cluttering the repository or
mutating fixtures.

### Implementation Tasks

1. Add artifact workspace helper:

```go
type ArtifactWorkspace struct {
    Root string
}

func NewArtifactWorkspace(fixtureName string, opts Options) (ArtifactWorkspace, func(), error)
```

2. Behavior:

- Use caller-provided temp directories during normal tests.
- Preserve artifacts only when `Options.KeepArtifacts` is true.
- Use `Options.ArtifactDir` when provided.
- Always create a unique subdirectory for each run. If `Options.ArtifactDir` is
  provided, create the unique run directory inside it; otherwise use
  `os.MkdirTemp` under the system temp directory.

3. Add helper methods:

```go
func (w ArtifactWorkspace) Path(parts ...string) string
func (w ArtifactWorkspace) CopyInput(src string) (string, error)
func (w ArtifactWorkspace) WriteText(name string, contents string) (string, error)
```

4. Sanitize fixture names for filesystem paths.

5. Validate destination paths created by `Path` and `WriteText` with
   `filepath.Clean`, reject absolute destination inputs, then use `filepath.Rel`
   from the workspace root to the resolved destination and reject any result
   equal to `..` or beginning with `../`.

6. Allow `CopyInput` source paths to point at project fixtures outside the
   artifact workspace, but always sanitize and contain the copied destination
   path inside the workspace.

7. Resolve the artifact workspace root with `filepath.EvalSymlinks` after
   creation. Destination containment checks must compare against that resolved
   root with an exact root match or a prefix match that includes the path
   separator, so `/tmp/root2` cannot match `/tmp/root`. Helper methods must not
   follow caller-supplied symlink destinations outside the workspace.

### Tests

- Artifact path creation uses sanitized fixture names.
- Artifact path creation includes a unique suffix to avoid parallel-test
  collisions.
- Cleanup removes non-preserved temp artifacts.
- Preserved artifacts remain in the requested directory.
- Helper refuses path traversal.

### Acceptance Criteria

- Round-trip tests can report artifact locations.
- No generated validation files appear under project source directories unless
  explicitly requested.

## Phase 4: PCB Round-Trip Command

### Objective

Run KiCad CLI against a temp copy of a generated PCB and compare the result.

### Implementation Tasks

1. Implement:

```go
func RoundTripPCB(ctx context.Context, cli KiCadCLI, inputPath string, opts Options) (Result, error)
```

2. Command behavior:

- Copy `inputPath` into the artifact workspace.
- Use `KiCadCLI.Version` to select the local upgrade command syntax. The initial
  implementation may support only the installed KiCad 10 command form, but the
  decision point must be isolated so older-version support can be added without
  rewriting the harness.
- Support the installed KiCad 10 in-place upgrade form used by this project.
- Capture unsupported-flag failures clearly so compatibility fallbacks can be
  added for older KiCad CLI versions.
- Run external KiCad commands with a configurable context timeout that defaults
  to 60 seconds so malformed files cannot hang the test suite indefinitely.
- Compare the original file with the upgraded temp copy.
- Capture stdout, stderr, exit code, CLI path, and KiCad version.

3. Error behavior:

- If KiCad exits non-zero, return an error containing the command, exit code,
  stderr, and artifact path.
- If comparison fails, return a `Result` with `Equal=false` rather than losing
  diagnostic paths.

4. Ensure checked-in fixtures are never passed directly to KiCad for mutation.

### Tests

- Unit test command construction using a fake CLI script.
- Unit test non-zero CLI errors include stderr.
- Unit test input file is copied before CLI invocation.
- Opt-in integration test round-trips a minimal generated PCB with the real
  KiCad CLI when `KICADAI_RUN_KICAD_CLI=1`.

### Acceptance Criteria

- `RoundTripPCB` validates parse/save without mutating source files.
- Real KiCad CLI successfully upgrades a generated PCB temp copy.
- Failed KiCad runs produce actionable errors.

## Phase 5: Generated Fixture Integration Tests

### Objective

Cover the generated LED PCB example and a generated-in-test PCB with opt-in
round-trip tests.

### Implementation Tasks

1. Add `roundtrip` integration tests guarded by:

```go
if !EnabledFromEnv() {
    t.Skip(...)
}
```

2. Add checked-in fixture test for:

```text
examples/07_generated_pcb/generated_pcb.kicad_pcb
```

3. Add generated-in-temp fixture test using existing LED PCB builder and writer.

4. Make tests fail on unexpected normalized differences.

5. When a difference is detected, print:

- Fixture name.
- CLI path.
- KiCad version.
- Original path.
- Round-tripped path.
- Artifact path.
- First normalized diff hunk.

### Tests

- `go test ./...` skips KiCad-backed tests unless enabled.
- `KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip` runs the
  real integration tests.

### Acceptance Criteria

- Checked-in example round-trips without unexpected normalized differences, or
  differences are captured as explicit known gaps.
- Generated-in-test fixture round-trips without unexpected normalized
  differences, or differences are captured as explicit known gaps.
- The layer-id regression remains covered.

## Phase 6: Structural Summary For PCB Files

### Objective

Add a lightweight structural summary so failures identify which KiCad section
changed, not only raw diff lines.

### Implementation Tasks

1. Reuse the existing S-expression tooling where practical.

2. Parse top-level PCB sections into summary counts and identities:

- Header fields.
- `general`.
- `paper`.
- `title_block`.
- `layers`.
- `setup`.
- `net`.
- `footprint`.
- `gr_*` board graphics.
- `segment`.
- route `arc`.
- `via`.
- `zone`.
- `group`.
- `dimension`.
- unknown/preserved nodes.

3. Add:

```go
func SummarizePCB(path string) (Summary, error)
```

4. Include summary changes in `Result.Differences`.

5. Write summary artifacts when artifacts are preserved.

### Tests

- Summary counts layers, nets, footprints, tracks, vias, zones, and graphics.
- Summary detects added or removed top-level sections.
- Summary errors are non-fatal when raw comparison already succeeded.
- Summary output is deterministic.

### Acceptance Criteria

- Round-trip failures point to changed sections.
- Structural summary does not require a complete KiCad object model.

## Phase 7: Expected Difference Allowlist

### Objective

Support temporary known gaps without hiding unexpected KiCad rewrites.

### Implementation Tasks

1. Add allowlist model:

```go
type AllowlistEntry struct {
    FileType    FileType
    FixtureName string
    Category    string
    Section     string
    Message     string
    DiffContains string
    Reason      string
    CleanupNote string
}
```

2. Add explicit matching for known differences. Matches must be narrow enough to
   identify the specific message or diff hunk, not only a broad section such as
   `setup` or `layers`.

3. Require every allowlist entry to include a reason.

4. Report allowed differences separately from unexpected differences.

5. Store initial allowlist in Go code or a small testdata file. Prefer Go code
   until the format proves stable.

### Tests

- Matching allowlist entries suppress only the intended difference.
- Missing reason fails validation.
- Unexpected differences still fail.
- Allowed differences are printed in verbose output.

### Acceptance Criteria

- Integration tests can move forward with documented known gaps.
- Allowlists are narrow and auditable.

## Phase 8: Documentation And Developer Workflow

### Objective

Document how to run, interpret, and debug round-trip validation.

### Implementation Tasks

1. Add documentation to the round-trip spec or a new README under:

```text
internal/kicadfiles/roundtrip/README.md
```

2. Include commands:

```sh
go test ./...
KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip
KICADAI_RUN_KICAD_CLI=1 KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1 go test ./internal/kicadfiles/roundtrip
```

3. Document artifact contents.

4. Document how to promote a round-trip failure into a writer regression test.

5. Add a short reference from:

```text
specs/pcb-writer/ROADMAP.md
```

### Tests

- Documentation examples use current env var names and package paths.
- No code tests required beyond previous phases.

### Acceptance Criteria

- A developer can run round-trip validation locally without reading the source.
- A failed run gives enough detail to decide whether the writer or allowlist must
  change.

## Completion Checklist

- `go test ./...` passes.
- `KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip` passes on
  a machine with KiCad installed.
- Round-trip tests skip cleanly without opt-in.
- Checked-in example files are not mutated by tests.
- Artifacts are preserved only when requested.
- Failures include CLI path, KiCad version, compared paths, and first useful
  diff.
- The implementation is ready to extend to schematic and full-project
  round-trip validation.
