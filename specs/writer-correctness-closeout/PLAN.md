# Writer Correctness Closeout Implementation Plan

## 1. Objective

Implement the writer correctness closeout described in `SPEC.md`.

The end state is a deterministic validation gate that proves generated KiCad
projects preserve intended schematic and PCB connectivity through project write,
parse-back, schematic-to-PCB transfer, footprint pad assignment, copper net
assignment, zone assignment, and optional KiCad round-trip evidence.

This plan focuses on generated projects first. Imported project mutation and
full preservation of unsupported KiCad features remain separate work.

## 2. Implementation Rules

- Reuse existing validators before adding new checks.
- Keep checks deterministic and testable without KiCad installed.
- Treat KiCad CLI integration as optional unless explicitly required.
- Avoid raw-text diff as the primary correctness signal; prefer semantic
  snapshots.
- Emit stable `reports.Issue` codes for all writer failures.
- Preserve stage attribution so AI workflows can eventually repair failures.
- Keep each phase independently reviewable.
- Run `gofmt` on edited Go files.
- Run focused package tests after each phase.
- Run `go test ./...` before phase commits where practical.
- Run `prism review staged` before each commit and address material findings.
- Commit between phases.
- Do not stage unrelated local changes such as an existing modified
  `specs/ROADMAP.md` unless explicitly requested.

## 3. Phase 1: Audit Existing Writer And Validation Coverage

### Goal

Create a concrete inventory of existing writer checks, generated examples,
known gaps, and reusable APIs before adding new code.

### Work

- Review current packages for:
  - project writer;
  - schematic writer;
  - PCB writer;
  - schematic-to-PCB transfer;
  - footprint expansion;
  - routing operation emission;
  - zone writing;
  - connectivity-first validation;
  - round-trip validation;
  - AI design workflow validation.
- Identify existing issue codes that can be reused.
- Identify where generated workflow projects still expose writer defects.
- Add a short status document:

```text
specs/writer-correctness-closeout/STATUS.md
```

- Record:
  - checks already implemented;
  - checks partially implemented;
  - checks missing;
  - generated examples to include in the golden corpus;
  - known KiCad CLI working-directory constraints.

### Tests

- No product tests required unless the audit reveals a small broken test fixture.

### Acceptance Criteria

- `STATUS.md` identifies reusable packages and missing checks.
- The next phases have file/package targets grounded in current code.

### Commit Message

```text
Audit writer correctness gaps
```

## 4. Phase 2: Add Writer Correctness Result Model

### Goal

Add a stable internal result contract for writer correctness checks.

### Work

- Add package:

```text
internal/writercorrectness
```

- Define:
  - `Options`;
  - `Result`;
  - `CheckResult`;
  - `CheckStatus`;
  - `Summary`;
  - `Target`;
  - issue helper functions.
- Add stable check names:
  - `project_structure`;
  - `schematic_parse`;
  - `schematic_connectivity`;
  - `schematic_pcb_transfer`;
  - `pcb_parse`;
  - `pcb_net_table`;
  - `footprint_pad_nets`;
  - `copper_net_references`;
  - `generated_connectivity`;
  - `zone_net_references`;
  - `kicad_round_trip`.
- Add helpers for:
  - appending blocking issues;
  - appending warnings;
  - computing `OK`;
  - sorting checks and issues deterministically;
  - serializing JSON-friendly output.

### Tests

- Empty result succeeds.
- Blocking issue makes result fail.
- Warning-only result succeeds.
- Check ordering is deterministic.
- Issue ordering is deterministic.

### Acceptance Criteria

- The package can represent all planned checks without performing them yet.
- Result JSON is stable and suitable for CLI output.

### Commit Message

```text
Add writer correctness result model
```

## 5. Phase 3: Target Discovery And Project Structure Checks

### Goal

Resolve user-provided project directories or files and validate generated KiCad
project structure.

### Work

- Implement target discovery:
  - project directory;
  - `.kicad_pro`;
  - `.kicad_sch`;
  - `.kicad_pcb`.
- Locate sibling project, schematic, and PCB files where possible.
- For project and root schematic targets, recursively discover hierarchical
  sheet files referenced by the root schematic and include the full sheet tree
  in schematic validation. Use canonical file paths, a visited set, and a
  default recursion-depth limit of 32 to prevent cycles or symlink loops from
  causing unbounded traversal.
- Expand KiCad-style path variables before checking local library table paths,
  including at minimum `${KIPRJMOD}`, `${PRJMOD}`, `${KICAD_PROJECT_DIR}`, and
  configured KiCad library roots when available. Support common KiCad variables
  such as versioned `${KICAD*_FOOTPRINT_DIR}`, `${KICAD*_SYMBOL_DIR}`,
  `${KICAD*_3DMODEL_DIR}`, and user-local library variables when they are
  present in the process environment or configured roots.
- Restrict existence checks to the project directory, generated local library
  directories, and explicitly configured or discovered KiCad global library
  roots. Absolute paths or `..` escapes outside those roots must be reported as
  untrusted unresolved paths rather than probed freely.
- Validate:
  - project file exists;
  - schematic file exists when expected;
  - PCB file exists when expected;
  - basenames for the core `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb` trio
    are identical when those files are present;
  - local `fp-lib-table` entries point to existing paths;
  - local `sym-lib-table` entries point to existing paths;
  - global library table references are treated as resolvable through configured
    KiCad roots or user/system library tables rather than falsely failing
    project-local validation;
  - generated project paths are not tied to missing temporary directories.
- Emit structured issues for missing or inconsistent files.

### Tests

- Project directory target resolves.
- `.kicad_pro` target resolves.
- Schematic-only target resolves.
- Hierarchical project target resolves root and child sheet files.
- Missing project file fails.
- Missing schematic file fails when required.
- Missing child sheet file fails.
- Mismatched project/schematic/PCB basenames fail.
- Broken local library table path fails.
- Valid fixture project passes.

### Acceptance Criteria

- Writer gate can identify the files it needs to check.
- Project structure defects produce stable writer issue codes.

### Commit Message

```text
Validate writer project structure
```

## 6. Phase 4: Schematic Parse And Connectivity Checks

### Goal

Validate that generated schematics parse back and preserve intended connectivity
well enough for transfer to PCB.

### Work

- Reuse existing schematic parser/reader.
- Add a schematic snapshot model if no suitable one exists.
- Capture:
  - symbols;
  - references;
  - library IDs;
  - footprint fields;
  - wires;
  - labels;
  - global labels;
  - hierarchical labels;
  - sheet symbols;
  - sheet pins;
  - junctions;
  - no-connect markers where parsed;
  - reconstructed net names where available.
- Validate:
  - schematic parses;
  - references are unique;
  - PCB-bearing symbols have footprint assignments;
  - wires and labels are not obviously dangling;
  - sheet pins match child-sheet hierarchical labels where supported;
  - generated labels connect to expected wires or pins;
  - schematic connectivity can be reconstructed without ambiguity.
- Emit issues for:
  - parse failure;
  - duplicate reference;
  - missing footprint assignment;
  - dangling wire;
  - unattached label;
  - ambiguous net.

### Tests

- Valid generated schematic passes.
- Duplicate reference fixture fails.
- Missing footprint assignment fixture fails.
- Dangling label fixture fails.
- Parse failure fixture fails.

### Acceptance Criteria

- Generated schematic fixtures have deterministic snapshots.
- Schematic defects are attributed to writer-stage issue codes.

### Commit Message

```text
Check generated schematic connectivity
```

## 7. Phase 5: Schematic-To-PCB Net Transfer Checks

### Goal

Prove that schematic nets become PCB nets without losing names or identity.

### Work

- Reuse existing schematic-to-PCB transfer output where available.
- Add a net-transfer snapshot that records:
  - schematic net name;
  - PCB net name;
  - PCB net code;
  - symbol reference;
  - symbol pin;
  - footprint reference;
  - footprint pad;
  - pinmap source;
  - confidence.
- Validate:
  - every PCB net maps to a schematic/generated net;
  - every expected schematic net has a PCB net when it reaches a footprint;
  - PCB net codes are unique and deterministic;
  - no pad references a missing PCB net;
  - missing pinmaps block where required.
- Compute transfer confidence deterministically:
  - `verified`: resolver-backed symbol and footprint records with a verified
    pinmap;
  - `inferred`: resolver-backed records where symbol pins and footprint pads
    match by number/name but no verified pinmap exists;
  - `synthetic`: generated block-local pin hints or synthetic footprints;
  - `unknown`: missing symbol, footprint, or pinmap evidence.
- Treat `unknown` as blocking for PCB-bearing symbols. Treat `inferred` and
  `synthetic` as warnings unless the consuming workflow requests strict
  verified pinmaps.
- Emit issues for:
  - missing PCB net;
  - duplicate net code;
  - unmapped PCB net;
  - missing pinmap;
  - footprint reference mismatch.

### Tests

- LED schematic-to-PCB transfer passes.
- Connector board transfer passes.
- Missing pinmap fixture fails.
- Duplicate net code fixture fails.
- Missing PCB net fixture fails.

### Acceptance Criteria

- The gate can explain schematic-to-PCB net mismatches before KiCad DRC runs.

### Commit Message

```text
Validate schematic to PCB net transfer
```

## 8. Phase 6: PCB Net Table And Footprint Pad Checks

### Goal

Validate PCB net tables, footprints, and pad net assignments after writing and
parsing the PCB file.

### Work

- Reuse existing PCB parser and validator.
- Build a PCB semantic snapshot with:
  - net table;
  - footprints;
  - pads;
  - pad geometry summaries;
  - pad net names and codes;
  - footprint library IDs;
  - footprint positions.
- Validate:
  - PCB parses;
  - net table codes are valid;
  - net names are non-empty where required;
  - footprint references are unique;
  - expected footprints exist;
  - expected pads exist;
  - pad net codes exist in the net table;
  - pad net names match expected transfer data;
  - pad geometry needed by routing/validation exists.
- Emit issues for:
  - missing net;
  - invalid net code;
  - duplicate footprint reference;
  - missing footprint;
  - missing pad;
  - pad wrong net;
  - incomplete pad geometry.

### Tests

- Generated LED board passes.
- Generated connector board passes.
- Missing pad net fixture fails.
- Wrong pad net fixture fails.
- Duplicate footprint reference fixture fails.
- Incomplete pad geometry fixture fails or warns according to severity.

### Acceptance Criteria

- Generated PCB footprints and pads can be trusted by validation and routing.

### Commit Message

```text
Validate PCB footprint pad nets
```

## 9. Phase 7: PCB Copper And Zone Net Checks

### Goal

Validate that tracks, vias, and zones reference legal nets and carry the intended
connectivity.

### Work

- Extend PCB snapshot to include:
  - tracks;
  - vias;
  - zones;
  - copper layers;
  - board outline bounds.
- Reuse existing structural PCB validation.
- Reuse generated connectivity validation.
- Reuse route completion validation where route metadata is available.
- Validate:
  - track net codes exist;
  - via net codes exist;
  - zone net codes exist;
  - copper layers are legal;
  - route widths are positive;
  - via sizes and drills are positive and legal;
  - via diameter satisfies `via_diameter >= via_drill + (2 *
    min_annular_ring_width)`;
  - zone outlines are closed and non-degenerate;
  - same-net copper connects expected pads;
  - wrong-net copper is blocking;
  - unrouted required nets are blocking unless allowed.
- Emit issues for:
  - track wrong net;
  - via wrong net;
  - zone wrong net;
  - invalid copper layer;
  - dangling copper endpoint;
  - unrouted required net;
  - invalid zone outline.

### Tests

- Routed generated board passes.
- Via-routed fixture passes.
- Zone fixture passes.
- Wrong track net fixture fails.
- Wrong via net fixture fails.
- Wrong zone net fixture fails.
- Dangling copper fixture fails.
- Unrouted required net fixture fails unless `AllowUnrouted` is set.

### Acceptance Criteria

- Generated routed boards fail fast when they only look connected visually.

### Commit Message

```text
Validate PCB copper net references
```

## 10. Phase 8: CLI Command

### Goal

Expose writer correctness through a user-facing command.

### Work

- Add command:

```sh
kicadai --json writer check <project-or-directory>
```

- Add flags:
  - `--require-kicad-roundtrip`;
  - `--kicad-cli`;
  - `--keep-artifacts`;
  - `--artifact-dir`;
  - `--strict-diffs`;
  - `--allow-unrouted`.
- Return JSON by default when global `--json` is set.
- Return concise text output otherwise.
- Exit non-zero on blocking failures.

### Tests

- Valid fixture exits zero.
- Invalid fixture exits non-zero.
- JSON output includes checks, issues, summary, and artifacts.
- Text output is concise and deterministic.
- Unknown target fails with a clear message.

### Acceptance Criteria

- Users and agents can run the writer gate independently of `design create`.

### Commit Message

```text
Expose writer correctness CLI
```

## 11. Phase 9: Integrate With AI Design Workflow

### Goal

Run the writer correctness gate automatically after `design create` writes a
project.

### Work

- Add workflow stage:

```text
writer_correctness
```

- Run writer gate after project write and before final acceptance.
- Include gate issues in workflow feedback.
- Map writer issue codes to repair suggestions.
- Decide blocking behavior based on request validation/acceptance settings.
- Include writer artifacts in workflow result.
- Ensure generated examples exercise the stage.

### Tests

- Successful design workflow includes passing writer stage.
- Writer failure blocks or downgrades according to acceptance settings.
- Writer issues are grouped in feedback by file/ref/net/stage.
- Existing design workflow tests remain deterministic.

### Acceptance Criteria

- `design create` no longer reports success when writer output is internally
  inconsistent.

### Commit Message

```text
Run writer checks in design workflow
```

## 12. Phase 10: Golden Writer Corpus

### Goal

Add generated fixtures that lock down writer behavior across increasing
complexity.

### Work

- Add fixture inputs and expected snapshots for:
  - schematic-only single symbol and label;
  - multi-component schematic net;
  - LED schematic-to-PCB board;
  - connector board;
  - footprints without routes;
  - routed tracks;
  - routed vias;
  - zone-bearing board;
  - circuit-block PCB fragment;
  - design workflow generated project.
- Add known-bad fixtures for:
  - missing net;
  - wrong pad net;
  - wrong track net;
  - wrong via net;
  - wrong zone net;
  - missing footprint;
  - dangling copper.
- Store expected output as semantic snapshots rather than brittle whole-file
  text where practical.

### Tests

- Every golden good fixture passes the writer gate.
- Every golden bad fixture fails with the expected issue code.
- Snapshot comparisons are deterministic.

### Acceptance Criteria

- Future writer changes cannot silently regress generated connectivity.

### Commit Message

```text
Add writer correctness golden corpus
```

## 13. Phase 11: Optional KiCad Round-Trip Evidence

### Goal

Add optional KiCad parse/save or validation evidence without making normal unit
tests depend on KiCad installation.

### Work

- Reuse existing KiCad CLI and round-trip helpers.
- Ensure the working directory exists for the entire command lifetime.
- Do not use `os.Chdir`; it mutates process-wide state and is unsafe in
  concurrent Go tests. Pass absolute paths and set `exec.Cmd.Dir` for KiCad
  subprocesses when a working directory is required.
- Avoid deleted temp directories in tests.
- Prefer retained fixture/example workspaces when requested.
- Add semantic before/after snapshots for:
  - nets;
  - footprints;
  - pads;
  - tracks;
  - vias;
  - zones;
  - library links.
- Classify diffs:
  - harmless normalization;
  - warning-level metadata change;
  - material connectivity change;
  - parse/save failure.
- Skip cleanly when KiCad is unavailable unless required.

### Tests

- Missing KiCad skips when optional.
- Missing KiCad fails when required.
- Working-directory setup does not trigger `Failed to get the working directory`.
- Fake runner material diff fails.
- Fake runner harmless normalization passes.
- Real KiCad smoke test may run behind environment flag.

### Acceptance Criteria

- Optional KiCad evidence is available and stable.
- Working-directory errors are fixed.
- Material KiCad rewrites are reported clearly.

### Commit Message

```text
Add writer KiCad roundtrip evidence
```

## 14. Phase 12: Documentation And README Update

### Goal

Document what writer correctness guarantees today and what remains outside its
scope.

### Work

- Update README with:
  - `writer check` command;
  - examples;
  - JSON result shape;
  - optional KiCad round-trip usage;
  - current guarantees;
  - current limits.
- Update relevant spec/status files.
- Add troubleshooting note for KiCad working-directory failures.
- Add roadmap note that Writer Correctness Closeout is implemented or in
  progress, depending on completion state.

### Tests

- Run documented example commands where practical.
- Run `go test ./...`.

### Acceptance Criteria

- Users can run and interpret writer correctness checks from README alone.

### Commit Message

```text
Document writer correctness checks
```

## 15. Final Verification

Before marking the project complete:

```sh
go test ./...
```

Run focused CLI examples:

```sh
out_dir="$(mktemp -d)"
trap 'rm -rf "$out_dir"' EXIT
kicadai --json writer check examples/07_generated_pcb
kicadai --json design create --request examples/design/led_indicator.json --output "$out_dir" --overwrite
kicadai --json writer check "$out_dir"
```

If KiCad CLI is available:

```sh
kicadai --json writer check --require-kicad-roundtrip --keep-artifacts examples/07_generated_pcb
```

Run Prism before each phase commit:

```sh
prism review staged
```

## 16. Completion Criteria

The project is complete when:

- writer correctness has a Go API;
- writer correctness has a CLI command;
- project structure checks run;
- schematic parse/connectivity checks run;
- schematic-to-PCB net transfer checks run;
- PCB net table and footprint pad checks run;
- PCB copper and zone checks run;
- `design create` runs writer correctness;
- golden good fixtures pass;
- golden bad fixtures fail with stable issue codes;
- optional KiCad round-trip evidence works or skips cleanly;
- README documents the feature;
- all tests pass.
