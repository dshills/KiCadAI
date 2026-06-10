# KiCad Round-Trip Validation Specification

## Purpose

Build a repeatable validation harness that generates KiCad project files, asks
KiCad itself to load and save or upgrade them, and compares the result against
the original generated output.

The project exists to close the gap between "KiCad can open the file" and "the
writer emits native-quality KiCad files that KiCad does not need to repair,
renumber, reorder, or normalize on first save."

## Background

The PCB writer now generates `.kicad_pcb` files that KiCad can parse, including
the canonical KiCad layer ids required by the installed KiCad toolchain. That
parse success is necessary but not sufficient.

KiCad may still rewrite generated files during save for reasons such as:

- Missing default sections.
- Non-native section ordering.
- Non-canonical layer tables.
- Missing generated metadata.
- Incomplete setup or plot parameters.
- Object attributes that are syntactically valid but not how KiCad persists
  them.
- Floating, unconnected, or normalized geometry.
- Implicit defaults that KiCad expands into explicit S-expressions.

Round-trip validation must turn those rewrites into visible, testable
differences so the writer can converge toward KiCad-native output.

## Goals

1. Generate deterministic KiCad project fixtures in a temporary workspace.
2. Run KiCad CLI load/save or upgrade operations against generated files.
3. Compare original generated files with KiCad-saved files.
4. Normalize known harmless differences before comparison.
5. Report structural differences in a developer-readable format.
6. Fail tests on unexpected KiCad rewrites.
7. Allow KiCad-dependent tests to be skipped on machines without KiCad.
8. Support PCB validation first, then schematic and project validation.
9. Preserve artifacts from failed round-trip runs for debugging when requested.
10. Make it easy to promote discovered KiCad rewrites into writer fixes and
    regression tests.

## Non-Goals

- Replacing KiCad's DRC or ERC.
- Building a full KiCad parser for every supported object type in this phase.
- Proving electrical correctness.
- Autorouting or geometric correction.
- Editing arbitrary user projects.
- Requiring KiCad to be installed for the normal `go test ./...` path.
- Treating every whitespace or formatting difference as a writer defect.

## Compatibility Target

The initial target is the locally installed KiCad command-line tool:

```text
/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```

The harness must also support discovery via `PATH` for non-macOS environments:

```text
kicad-cli
```

The active file-format target is KiCad 10:

- PCB format version: `20260206`
- PCB generator: `pcbnew`
- PCB generator version: `10.0`
- Schematic format version: `20260306`
- Schematic generator: `eeschema`
- Schematic generator version: `10.0`

The exact KiCad version used during validation must be captured in test logs or
diagnostic output. These format versions intentionally target the installed
KiCad 10 toolchain used by this project, not older KiCad releases.

## Test Gating

KiCad-dependent tests must be opt-in by default.

Required environment variable:

```sh
KICADAI_RUN_KICAD_CLI=1
```

Optional environment variables:

```sh
KICADAI_KICAD_CLI=/custom/path/to/kicad-cli
KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1
KICADAI_ROUNDTRIP_ARTIFACT_DIR=/tmp/kicadai-roundtrip
```

Behavior:

- If `KICADAI_RUN_KICAD_CLI` is not set to `1`, KiCad-dependent tests must
  skip.
- If the variable is set but `kicad-cli` cannot be found or executed, tests must
  fail with an actionable error.
- Normal unit tests must continue to run without KiCad installed.

## Scope

### Phase 1 Scope: PCB Round Trip

Validate generated `.kicad_pcb` files.

The first acceptance fixture is the generated LED PCB example:

```text
examples/07_generated_pcb/generated_pcb.kicad_pcb
```

The harness must also support generating the same fixture into a temp directory
from Go rather than relying only on the checked-in example.

### Phase 2 Scope: Schematic Round Trip

Validate generated `.kicad_sch` files once the PCB round-trip path is stable.

Expected focus areas:

- File header and generator metadata.
- Library symbol embedding.
- Symbol instance paths.
- Wire and label placement.
- Junction persistence.
- Title block and paper settings.
- Project-side schematic metadata interaction.

### Phase 3 Scope: Project Directory Round Trip

Validate complete generated project directories.

Expected files:

- `.kicad_pro`
- `.kicad_sch`
- `.kicad_pcb`
- `.kicad_prl`, if KiCad creates or updates it during save.
- `.kicad_pri`, if KiCad creates or updates it during save.
- `sym-lib-table`, when local symbol libraries are emitted.
- `fp-lib-table`, when local footprint libraries are emitted.

## KiCad CLI Operations

### PCB Upgrade

The first implementation must use a temp copy and run:

```sh
kicad-cli pcb upgrade --force <board.kicad_pcb>
```

This command is accepted as the initial load/save proxy because it forces KiCad
to parse and re-save the board using the installed format writer.

### Future PCB Checks

The harness should leave room for:

```sh
kicad-cli pcb run-drc
kicad-cli pcb export gerber
kicad-cli pcb export drill
```

These are not required for the initial round-trip project, but the API should
not make them difficult to add.

### Schematic Upgrade

If supported by the installed KiCad CLI, schematic validation should use the
corresponding schematic upgrade or export path. If no direct schematic save
operation is available, this phase must first document the available commands
and choose the closest reliable load/save proxy.

## File Comparison Model

Round-trip comparison must produce three layers of evidence:

1. Raw text diff.
2. Normalized text diff.
3. Structural summary.

### Raw Text Diff

The raw diff compares the generated file to the KiCad-saved file exactly as
written.

Raw diff is useful for diagnosis but must not be the only pass/fail signal
because KiCad may rewrite whitespace or formatting in harmless ways.

### Normalized Text Diff

The normalized diff must remove known harmless differences.

Initial normalizations:

- Line endings normalized to `\n`.
- Trailing whitespace removed.
- Final newline normalized.
- Known volatile KiCad backup metadata ignored if encountered.

Future semantic normalizations may include numeric precision canonicalization and
order-insensitive comparison for explicitly safe repeated nodes. These must be
implemented at the S-expression AST level, not with raw-text regular
expressions, and only after a concrete KiCad rewrite is observed because overly
broad normalization can hide writer defects.

The normalizer must not hide semantic changes such as:

- Layer number changes.
- Net code changes.
- Missing objects.
- Changed UUIDs.
- Changed coordinates.
- Changed pad stacks.
- Changed setup fields.
- Changed generator version.
- Added or removed design-rule settings.

Stable writer-generated UUIDs are required. Missing UUIDs or KiCad-generated
replacement UUIDs are writer defects unless explicitly documented as a temporary
known gap.

### Structural Summary

The structural summary must parse enough S-expression shape to classify changes
by top-level section and common object identity.

Required summary groups for PCB files:

- Header fields.
- `general`.
- `paper`.
- `title_block`.
- `layers`.
- `setup`.
- `net`.
- `footprint`.
- board graphics.
- tracks and vias.
- zones.
- groups.
- dimensions.
- preserved or unknown nodes.

The first implementation may use a lightweight S-expression parser or existing
sexpr package support, but it must avoid brittle regular expressions for nested
objects.

## Expected Difference Policy

The harness must support expected differences through explicit allowlists.

Allowlist entries must include:

- File type.
- Path or fixture name.
- Difference category.
- Reason.
- Expiration or cleanup note when possible.

Example categories:

- `formatting-only`
- `kiCad-adds-default`
- `known-writer-gap`
- `volatile-local-state`

Allowlists are temporary debt. They must not become the default way to pass
tests.

## Failure Output

On failure, the test output must identify:

- Fixture name.
- KiCad CLI path.
- KiCad CLI version, when available.
- Generated file path.
- KiCad-saved file path.
- First meaningful normalized diff hunk.
- Structural summary of changed sections.
- Artifact directory, if artifacts were preserved.

The output must be short enough to be readable in `go test`, with full files
available in the artifact directory.

## Artifact Management

By default, tests should use `t.TempDir()` and clean up after success.

When `KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1` is set:

- Preserve generated files.
- Preserve KiCad-saved files.
- Preserve raw diffs.
- Preserve normalized diffs.
- Preserve structural summaries.

If `KICADAI_ROUNDTRIP_ARTIFACT_DIR` is set, artifacts must be written under
that directory. Otherwise, create a timestamped directory under the system temp
directory.

## Go Package Design

Add a small internal validation package:

```text
internal/kicadfiles/roundtrip
```

Proposed API:

```go
type KiCadCLI struct {
    Path string
}

type Options struct {
    KeepArtifacts bool
    ArtifactDir   string
}

type FileType string

const (
    FileTypePCB       FileType = "pcb"
    FileTypeSchematic FileType = "schematic"
    FileTypeProject   FileType = "project"
)

type Result struct {
    FixtureName       string
    FileType          FileType
    KiCadCLIPath      string
    KiCadVersion      string
    OriginalPath      string
    RoundTrippedPath  string
    RawDiffPath       string
    NormalizedDiffPath string
    SummaryPath       string
    Equal             bool
    Differences       []Difference
}

type Difference struct {
    Category string
    Section  string
    ObjectID string
    Message  string
}

func DiscoverCLI() (KiCadCLI, error)
func EnabledFromEnv() bool
func OptionsFromEnv() Options
func RoundTripPCB(ctx context.Context, cli KiCadCLI, inputPath string, opts Options) (Result, error)
func CompareFiles(originalPath, roundTrippedPath string, opts Options) (Result, error)
```

The exact API may change during implementation, but it must preserve these
capabilities:

- CLI discovery.
- Environment gating.
- Running KiCad on temp copies.
- Capturing diagnostics.
- Comparing raw and normalized files.
- Returning structured differences.

## Test Requirements

### Unit Tests

Required unit tests that do not require KiCad:

- CLI discovery respects `KICADAI_KICAD_CLI`.
- `EnabledFromEnv` only enables on `KICADAI_RUN_KICAD_CLI=1`.
- Normalizer handles line endings and final newline.
- Normalizer does not remove semantic layer changes.
- Diff reporting includes file paths and fixture names.
- Artifact retention path selection is deterministic enough to test.

### KiCad Integration Tests

Required opt-in tests:

- Generated PCB round-trips with no unexpected normalized differences.
- Checked-in example `07_generated_pcb` round-trips with no unexpected
  normalized differences.
- Invalid PCB layer table fails with a useful KiCad CLI error.

### Regression Tests

Every bug discovered through round-trip validation must add one of:

- A writer unit test.
- A round-trip integration test.
- A corpus fixture check.

The layer-id bug fixed after KiCad reported `1 is not a valid layer count` must
remain covered by a test that asserts canonical layer ids in generated PCB
output.

## Acceptance Criteria

The project is complete when:

1. `go test ./...` passes without KiCad installed or without opt-in env vars.
2. `KICADAI_RUN_KICAD_CLI=1 go test ./...` runs KiCad-backed round-trip tests on
   machines with KiCad installed.
3. The generated PCB fixture round-trips through KiCad CLI without unexpected
   normalized differences.
4. Failure output points directly to changed sections and preserved artifacts.
5. The checked-in `examples/07_generated_pcb/generated_pcb.kicad_pcb` is covered
   by round-trip validation.
6. Round-trip tests never mutate checked-in fixtures.
7. The harness is documented well enough for future schematic and project
   round-trip phases.

## Risks

### KiCad CLI Surface Changes

KiCad CLI command names and behavior can change between versions. The harness
must capture the version and fail with clear diagnostics.

### False Positives

KiCad may rewrite harmless formatting details. The normalizer and structural
summary must separate formatting churn from semantic changes.

### False Negatives

Over-broad allowlists or normalization could hide writer bugs. Allowlist entries
must be explicit, reviewed, and tied to known reasons.

### Slow Tests

KiCad-backed tests may be slower than unit tests. They must remain opt-in and
target a small fixture set at first.

### Platform Differences

The first path target is macOS, but the implementation must not hard-code macOS
only behavior when `kicad-cli` is available on `PATH`.

## Open Questions

1. Which KiCad CLI command is the best schematic load/save proxy for KiCad 10?
2. Should round-trip comparison eventually use a full S-expression AST diff
   rather than normalized text plus structural summaries?
3. Should project sidecar files created by KiCad be treated as required writer
   output or expected KiCad-local state?
4. How much KiCad rewrite churn is acceptable before a writer fix becomes
   mandatory?
5. Should the harness support running against multiple KiCad versions in CI?
