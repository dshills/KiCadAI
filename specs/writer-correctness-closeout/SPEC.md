# Writer Correctness Closeout Specification

## Purpose

Close the remaining correctness gaps in the KiCad project, schematic, and PCB
writers so generated projects are not merely parseable, but electrically
consistent, KiCad-native, and stable enough for higher-level AI workflows.

This project is the bridge between the current generation pipeline and the next
round of AI autonomy. Before the system chooses more parts, performs more
repairs, or exports fabrication packages, the writer layer must prove that the
artifacts it emits preserve the intended schematic and PCB connectivity.

## Background

KiCadAI already has a substantial writer foundation:

- project, schematic, and PCB file writers;
- schematic round-trip preservation work;
- PCB object correctness tests;
- footprint library expansion;
- schematic-to-PCB transfer;
- circuit block PCB realization;
- connectivity-first board validation;
- placement and routing foundations;
- AI design workflow CLI;
- optional KiCad CLI ERC/DRC checks.

The remaining problem is not broad file-format coverage in the abstract. The
remaining problem is writer trust. Generated designs must carry correct net
identity through schematic symbols, PCB footprints, pads, tracks, vias, zones,
and validation reports. KiCad should be able to open and save those files
without material connectivity rewrites.

## Goals

1. Establish one writer-correctness gate for generated KiCad projects.
2. Ensure schematic nets transfer to PCB nets deterministically.
3. Ensure PCB pads, tracks, vias, and zones refer to valid net identities.
4. Ensure generated footprints preserve pad net assignments and geometry needed
   for validation and routing.
5. Ensure generated route operations write copper on legal layers with correct
   net codes.
6. Ensure generated zone operations write valid net/layer assignments.
7. Detect writer defects before KiCad GUI or DRC is required.
8. Add optional KiCad round-trip evidence when `kicad-cli` is available.
9. Add golden generated projects that lock down the expected writer output.
10. Produce AI-actionable diagnostics for any writer failure.

## Non-Goals

- Implementing arbitrary natural-language design generation.
- Implementing full autorouting or production-quality routing.
- Implementing manufacturing export.
- Replacing KiCad DRC or ERC.
- Fully preserving every unsupported object in imported KiCad projects.
- Completing all symbol-library semantics.
- Adding a sourcing or MPN database.

## Definitions

- **Writer correctness**: The generated files faithfully represent the internal
  design model and preserve intended project, schematic, PCB, net, footprint,
  pad, copper, and zone relationships.
- **Connectivity identity**: The stable relationship between schematic nets and
  PCB net codes/names.
- **Material KiCad rewrite**: A KiCad parse/save change that alters connectivity,
  object identity, footprint pad nets, route nets, zones, library links, or
  other modeled semantics.
- **Generated project**: A project produced by KiCadAI from transactions, circuit
  blocks, design workflow requests, or examples.
- **Writer gate**: A deterministic validation workflow that checks writer output
  before higher-level AI steps continue.

## Scope

The closeout applies to generated projects and examples first. Imported project
mutation remains important, but existing-project preservation is not the primary
acceptance target for this project.

Included file types:

- `.kicad_pro`
- `.kicad_sch`
- `.kicad_pcb`
- `fp-lib-table`
- `sym-lib-table`
- generated local library files when present

Included object classes:

- schematic symbols;
- labels;
- wires;
- junctions;
- no-connect markers where modeled;
- schematic symbol footprint assignments;
- PCB nets;
- PCB footprints;
- PCB pads;
- PCB tracks;
- PCB vias;
- PCB zones;
- board outlines;
- project library tables.

## Current Known Risk Areas

The roadmap gap analysis identifies writer correctness as the first priority.
Known risk areas include:

- generated PCB net-code and net-name correctness;
- footprint pad net assignment;
- route net assignment;
- zone net assignment;
- schematic-to-PCB netlist consistency;
- KiCad parse/save stability;
- avoidable KiCad round-trip diffs;
- generated workflow projects exposing writer gaps;
- validation output that reports symptoms instead of writer-stage causes.

## Required Capabilities

### 1. Project Structure Correctness

The writer must emit a KiCad project directory with the files KiCad expects for
generated designs.

Requirements:

- project file name and schematic/PCB file names must be consistent;
- project-local library tables must be emitted when local libraries are used;
- relative library paths must be stable;
- missing optional files must not cause KiCad load errors;
- generated examples must follow the same directory conventions as real KiCad
  projects.

Validation:

- generated project files exist;
- file basenames are consistent;
- required library table entries resolve;
- paths are relative where practical;
- no generated path points at an ephemeral temporary directory unless explicitly
  requested.

### 2. Schematic Connectivity Correctness

The schematic writer must preserve intended electrical connectivity.

Requirements:

- every intended net has a deterministic name;
- wires connect to symbol pins at exact pin endpoints;
- junctions are emitted where required by KiCad semantics;
- labels attach to wires or pins intentionally;
- no-connect markers are explicit;
- generated references are unique;
- generated UUIDs are stable within one write;
- symbol footprint assignments are present for PCB-bearing components.

Validation:

- parse the generated schematic back into KiCadAI's model;
- reconstruct schematic connectivity;
- compare expected nets against parsed nets;
- verify each PCB-bearing symbol has an assigned footprint;
- emit writer-stage issues for missing, dangling, or ambiguous connectivity.

### 3. Schematic-To-PCB Net Transfer Correctness

The transfer layer must convert schematic nets into PCB nets without losing
identity.

Requirements:

- every PCB net must map back to a schematic net or a documented generated net;
- net code assignment must be deterministic;
- net names must be preserved;
- hidden or generated power nets must be explicit in the transfer result;
- schematic symbol pin to footprint pad mappings must be validated before PCB
  write.

Validation:

- compare expected schematic net names to PCB net names;
- verify all footprint pad net names exist in the PCB net table;
- verify every routed net references the correct PCB net code;
- report unmapped, duplicate, or missing nets as blocking writer failures.

### 4. PCB Footprint And Pad Correctness

Generated PCB footprints must preserve enough real geometry and net metadata for
KiCad, routing, validation, and future DRC.

Requirements:

- footprint references are unique and match schematic references;
- footprint library IDs are preserved;
- footprint positions and rotations are deterministic;
- pads have legal numbers;
- pads have legal types, shapes, sizes, layers, and drill data;
- pads assigned to nets reference valid net codes and names;
- local footprint graphics and models must not invalidate parse/save behavior;
- unresolved footprint geometry must block or degrade explicitly.

Validation:

- parse written PCB and inspect footprint/pad records;
- verify every expected footprint exists;
- verify every expected pad exists;
- verify pad nets match the transfer result;
- verify required pad geometry is present for validation/routing.

### 5. PCB Copper Correctness

Tracks, vias, and zones must carry correct net identity and legal geometry.

Requirements:

- tracks reference valid net codes;
- vias reference valid net codes;
- zones reference valid net codes and legal layers;
- copper endpoints connect to pads or other copper objects within tolerance;
- generated copper is on valid board copper layers;
- route width and via sizes are legal positive values;
- zone outlines are closed and non-degenerate;
- edge cuts remain separate from electrical copper.

Validation:

- structural PCB validation passes;
- generated connectivity validation passes;
- route completion validation passes for routed nets;
- zone validation reports valid net/layer references;
- wrong-net copper is blocking;
- unrouted required nets are blocking unless explicitly allowed.

### 6. KiCad Round-Trip Evidence

When `kicad-cli` is available, generated files should be opened/saved or
otherwise normalized by KiCad and compared against writer output.

Requirements:

- round-trip runs in a stable working directory;
- no test should depend on a deleted temporary working directory;
- artifacts may use example or fixture directories when needed;
- KiCad absence must skip optional integration tests cleanly;
- material diffs must be classified separately from harmless formatting churn.

Validation:

- run KiCad parse/check/save flow when available;
- compare semantic snapshots before and after;
- report material changes to nets, footprints, pads, copper, zones, and library
  links as blocking;
- store retained artifacts only when requested.

### 7. Golden Writer Corpus

The project needs a small generated corpus that represents increasing writer
complexity.

Required fixture classes:

- schematic-only single symbol and label;
- schematic with multi-component net;
- schematic-to-PCB LED or connector board;
- board with footprints and no routes;
- board with routed tracks and vias;
- board with a zone;
- circuit-block generated PCB fragment;
- design workflow generated project.

Each fixture should include:

- generation input;
- expected schematic nets;
- expected PCB nets;
- expected footprint references;
- expected pad net assignments;
- expected routed nets when applicable;
- expected validation status.

## Writer Correctness Gate

Add or consolidate a gate that higher-level workflows can call before continuing.

Suggested package:

```text
internal/writercorrectness
```

Suggested API:

```go
type Options struct {
    RequireKiCadRoundTrip bool
    KiCadCLI              string
    KeepArtifacts         bool
    ArtifactDir           string
    StrictDiffs           bool
    AllowUnrouted         bool
}

type Result struct {
    OK             bool
    Target         string
    Project        string
    Schematic      string
    PCB            string
    Checks         []CheckResult
    Issues         []reports.Issue
    Artifacts      []reports.Artifact
    OverallSummary Summary
}

type CheckStatus string

const (
    CheckPass    CheckStatus = "pass"
    CheckFail    CheckStatus = "fail"
    CheckWarning CheckStatus = "warning"
    CheckSkipped CheckStatus = "skipped"
)

type CheckResult struct {
    Name     string
    Status   CheckStatus
    Issues   []reports.Issue
    Artifacts []reports.Artifact
    Summary  string
}

type Summary struct {
    CheckCount    int
    IssueCount    int
    FailCount     int
    WarningCount  int
    SkippedCount  int
}
```

Suggested checks:

- `project_structure`
- `schematic_parse`
- `schematic_connectivity`
- `schematic_pcb_transfer`
- `pcb_parse`
- `pcb_net_table`
- `footprint_pad_nets`
- `copper_net_references`
- `generated_connectivity`
- `zone_net_references`
- `kicad_round_trip`

The result model must be deterministic and usable by AI workflows. Every issue
should include:

- stable issue code;
- severity;
- file path;
- object reference when available;
- net name/code when available;
- writer stage;
- repair category;
- suggested next action.

## CLI Surface

Expose the writer gate through the CLI.

Preferred command:

```sh
kicadai --json writer check <project-or-directory>
```

Useful flags:

```text
--require-kicad-roundtrip
--kicad-cli string
--keep-artifacts
--artifact-dir string
--strict-diffs
--allow-unrouted
```

The command should support generated project directories and direct file paths
where practical.

If `--require-kicad-roundtrip` is set, the command must resolve a usable KiCad
CLI executable before running any round-trip work. Resolution order is:

1. explicit `--kicad-cli`;
2. trusted user configuration outside the project under test, if already
   supported by the CLI;
3. `kicad-cli` found on `PATH`.

The writer check must not honor executable paths stored in the KiCad project or
inside any file being inspected. Project-controlled executable paths are
untrusted input.

If no executable is found, or the selected path is not executable, the command
must fail with `writer.roundtrip.kicad_unavailable`. If round-trip evidence is
optional, the same condition should produce a skipped check rather than a
blocking issue.

KiCad CLI-backed round-trip evidence targets KiCad versions that provide
`kicad-cli`. Older KiCad versions without `kicad-cli` are unsupported for this
specific optional evidence path and must be reported as skipped unless
round-trip evidence is required. The implementation should record the detected
KiCad CLI version when available.

## Material Round-Trip Changes

Round-trip comparison must be semantic. Raw text diffs are not material by
themselves.

The following changes are material and blocking:

- project, schematic, or PCB parse failure after round trip;
- schematic net name changes;
- schematic symbol reference, library ID, footprint field, or UUID changes;
- schematic wire, label, junction, or no-connect changes that alter
  connectivity;
- PCB net name changes, missing names, or name-to-object mapping changes;
- PCB net code changes only when the code change causes pads, tracks, vias, or
  zones to resolve to a different net name after parsing;
- footprint reference, library ID, position, rotation, or side changes beyond
  the configured geometry tolerance;
- pad number, type, shape, size, drill, layer, or net changes;
- track, via, or zone net changes;
- track, via, or zone layer changes;
- copper endpoint movement beyond tolerance;
- board outline changes beyond tolerance;
- local library table entries becoming unresolved.

The following changes are benign unless `--strict-diffs` is set:

- whitespace and S-expression formatting;
- KiCad field ordering where semantic values are unchanged;
- KiCad-generated default metadata that does not affect connectivity or
  geometry;
- UUID replacement is material by default. It is benign only when the semantic
  snapshot can prove that the object is not referenced by any schematic, PCB,
  sheet, or cross-file synchronization path, or when every affected schematic
  and PCB UUID mapping is updated consistently and the cross-file mapping
  remains intact.

The geometry tolerance must be configurable. The default tolerance is 1,000
KiCad internal units when comparing parsed semantic snapshots. For modern KiCad
files this means 1,000 nanometers, or 0.001 mm. That remains small relative to
schematic and PCB grid sizes while avoiding false positives in converted or
unit-normalized projects. The implementation must inspect the parsed file format
version before applying unit conversions, and must not assume nanometer units
for any legacy format that uses a different scale.

## Integration Points

### AI Design Workflow

`design create` should run the writer correctness gate after project write and
before final acceptance.

At first, writer failures may downgrade the workflow result or block depending
on the requested acceptance level. Eventually, writer failures should feed into
the repair loop.

### Connectivity-First Board Validation

Writer correctness should reuse existing board validation instead of duplicating
geometry and connectivity checks.

### Round-Trip Validation

Writer correctness should reuse existing KiCad round-trip helpers and fix any
working-directory assumptions that cause errors such as:

```text
Failed to get the working directory (error 2: No such file or directory)
```

Implementations must not call `os.Chdir` to prepare KiCad commands. KiCad
subprocesses should receive absolute paths and, when a working directory is
needed, set `exec.Cmd.Dir` to a directory that exists for the full command
lifetime. Tests should include a guard that fails if writer correctness code
uses process-wide directory changes.

### Project Evaluation

Project evaluation should include writer correctness status as a first-class
readiness signal.

## Diagnostics

Issue codes should distinguish writer-stage failures from design-stage failures.

Examples:

- `writer.project.missing_file`
- `writer.schematic.parse_failed`
- `writer.schematic.unmapped_net`
- `writer.schematic.dangling_wire`
- `writer.transfer.missing_footprint`
- `writer.transfer.pinmap_missing`
- `writer.pcb.net_missing`
- `writer.pcb.pad_wrong_net`
- `writer.pcb.track_wrong_net`
- `writer.pcb.via_wrong_net`
- `writer.pcb.zone_wrong_net`
- `writer.roundtrip.material_diff`
- `writer.roundtrip.kicad_unavailable`

## Testing Strategy

The project should include unit, fixture, integration, and optional KiCad tests.

Unit tests:

- net table construction;
- schematic-to-PCB net mapping;
- pad net assignment;
- copper net reference validation;
- issue classification.

Fixture tests:

- generated schematic fixtures;
- generated PCB fixtures;
- known-bad wrong-net fixtures;
- known-bad missing-net fixtures;
- known-bad dangling copper fixtures;
- known-bad missing-footprint fixtures.

Integration tests:

- `design create` generated examples;
- circuit-block PCB fragments;
- schematic-to-PCB transfer examples;
- project writer output.

Optional KiCad tests:

- parse/check generated projects;
- save or normalize generated projects;
- compare semantic snapshots;
- skip when KiCad is unavailable.

## Acceptance Criteria

This project is complete when:

1. A writer correctness gate exists and is callable from Go.
2. The gate is exposed through the CLI.
3. The AI design workflow uses the gate after writing output.
4. Generated examples pass project structure checks.
5. Generated schematics pass parse and connectivity checks.
6. Generated PCBs pass net table, footprint pad net, copper net, and zone net
   checks.
7. Generated routed projects pass connectivity-first board validation.
8. Wrong-net, missing-net, missing-footprint, and dangling-route fixtures fail
   with stable issue codes.
9. Optional KiCad round-trip tests skip cleanly when KiCad is unavailable.
10. Optional KiCad round-trip tests do not fail because the working directory was
    deleted or unavailable.
11. Material KiCad round-trip diffs are classified and reported.
12. README documents the command, current guarantees, and remaining limits.

## Risks

- KiCad save output may include benign formatting or ordering differences that
  require semantic diffing rather than raw text diffing.
- Some generated files may intentionally omit advanced KiCad sections that KiCad
  later fills in.
- Symbol semantics may be insufficient for complex multi-unit parts.
- Footprint library data may be incomplete for some generated examples.
- Real KiCad CLI behavior may vary by installed KiCad version.

## Open Questions

1. Should writer correctness failures always block `design create`, or should
   acceptance level control blocking behavior?
2. Should semantic snapshots become a shared package for schematic, PCB, and
   round-trip validation?
3. Should generated examples be rewritten as part of this project if the gate
   exposes existing defects?
