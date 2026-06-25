# Fabrication Physical Rule Checks Implementation Plan

## Phase 1: Rule Model And Taxonomy

### Objective

Create the reusable physical-rule report model and stable check taxonomy without
changing readiness behavior yet.

### Implementation Steps

1. Add a focused package or module for physical fabrication checks:
   - preferred path: `internal/fabrication/physicalrules`;
   - alternative: keep files in `internal/fabrication` if local conventions make
     that simpler.
2. Define report types:
   - report schema constant;
   - report status enum;
   - category enum;
   - check result;
   - measurement/evidence structures;
   - summary counters.
3. Define stable check IDs for:
   - stackup;
   - net class;
   - solder mask;
   - solder paste;
   - edge cuts;
   - courtyard;
   - silkscreen;
   - mounting holes.
4. Add helpers that convert failed checks into `reports.Issue`.
5. Add deterministic JSON marshal tests.
6. Add aggregate status tests for pass, warning, blocked, and skipped checks.

### Review Checklist

- Check IDs are stable and human-readable.
- The model can represent both internal checks and optional KiCad evidence.
- The model does not require KiCad CLI.
- Issue paths are suitable for later repair mapping.

### Suggested Commit

`Add fabrication physical rule model`

## Phase 2: Stackup, Edge-Cut, And Net-Class Checks

### Objective

Implement the first board-level physical checks using existing PCB/project data.

### Implementation Steps

1. Add a board loader that accepts the same project target forms used by
   fabrication export.
2. Implement stackup checks:
   - valid copper layer count;
   - required top/bottom copper layers;
   - positive board thickness when present;
   - non-negative solder mask settings;
   - supported routing/via policy for enabled layers.
3. Implement edge-cut checks:
   - outline exists;
   - simple closed rectangular/polygonal outline detection;
   - positive bounding area;
   - generated objects inside board bounding box;
   - conservative warnings for unsupported arcs/complex geometry.
4. Implement net-class checks:
   - default net class exists where project settings are available;
   - routed/generated nets have effective width/clearance/via policy;
   - route segment widths agree with assigned net class where modeled;
   - high-current/power roles without explicit rules warn or block according to
     profile.
5. Add fixtures for valid board, missing outline, invalid stackup, missing
   net-class, and high-current default-rule cases.
6. Add unit tests for all checks.

### Review Checklist

- Missing outline remains blocking.
- Complex geometry produces conservative warnings, not false pass.
- No check requires real KiCad.
- Net-class rules reuse `internal/pcbrules` where possible.

### Suggested Commit

`Add stackup edge cut and net class fabrication checks`

## Phase 3: Solder Mask And Solder Paste Checks

### Objective

Validate pad layer policy and plotted mask/paste expectations for assembly.

### Implementation Steps

1. Inspect the current PCB pad model and parser coverage for:
   - pad type;
   - pad layers;
   - side;
   - mask/paste layer membership;
   - pad-level or footprint-level mask/paste overrides.
2. Add mask checks:
   - SMD pads expose mask on their assembly side;
   - mask Gerber evidence exists when package artifacts are present;
   - malformed mask margins or settings block.
3. Add paste checks:
   - SMD assembly pads include paste layer or explicit no-paste evidence;
   - through-hole and NPTH pads do not require paste;
   - paste Gerber evidence matches assembly sides when plotted;
   - malformed paste margin or ratio values block.
4. Add profile policy hooks for mask/paste strictness.
5. Add tests for top SMD, bottom SMD, THT, NPTH, missing paste, and missing
   plotted mask/paste cases.

### Review Checklist

- The checks distinguish SMD assembly pads from mechanical holes.
- Missing paste is not reported on THT-only boards.
- Artifact validation and physical-rule checks do not duplicate contradictory
  statuses.

### Suggested Commit

`Add solder mask and paste fabrication checks`

## Phase 4: Courtyard And Silkscreen Checks

### Objective

Catch common assembly and readability problems using conservative geometry.

### Implementation Steps

1. Reuse hydrated footprint graphics and parser data for:
   - courtyard bounds;
   - silkscreen graphics/text;
   - pad bounding boxes;
   - footprint placement transforms.
2. Implement courtyard checks:
   - missing courtyard on non-exempt assembly footprints;
   - courtyard overlap using transformed bounding boxes;
   - profile courtyard spacing threshold.
3. Implement silkscreen checks:
   - silkscreen outside board bounds;
   - silkscreen overlap with pad bounds;
   - silkscreen edge-clearance warning/blocking threshold;
   - missing reference text for assembly-critical components.
4. Mark complex unsupported geometry as warning and include the unsupported
   object type in evidence.
5. Add tests for missing courtyard, exempt courtyard, overlapping courtyards,
   clear courtyards, silkscreen-over-pad, off-board silkscreen, and pass cases.

### Review Checklist

- Conservative bounding-box checks cannot incorrectly mark unknown complex
  geometry as fully safe.
- Generated footprints with hydrated courtyard/fab/silk data are recognized.
- Silkscreen checks report references and layers.

### Suggested Commit

`Add courtyard and silkscreen fabrication checks`

## Phase 5: Mounting-Hole And Mechanical Checks

### Objective

Model mechanical hole requirements and edge/keepout constraints.

### Implementation Steps

1. Define mounting-hole policy inputs:
   - design request flag or generated manifest option;
   - manufacturer profile setting;
   - explicit footprint attribute or footprint ID pattern;
   - fallback heuristic for known mounting-hole footprints.
2. Implement mounting-hole checks:
   - required holes present;
   - plated/NPTH classification where available;
   - positive drill diameter;
   - edge clearance;
   - local keepout evidence when modeled;
   - exclusion from BOM/CPL assembly rows unless explicitly allowed.
3. Add profile thresholds:
   - minimum hole diameter;
   - minimum hole-to-edge clearance;
   - default required-hole policy.
4. Add tests for no requirement, missing required holes, invalid drill, edge
   violation, BOM/CPL inclusion, and valid mounting-hole boards.

### Review Checklist

- Mounting holes are not required by default unless profile or intent says so.
- Mechanical holes do not create false solder-paste issues.
- The report explains when classification is heuristic.

### Suggested Commit

`Add mounting hole fabrication checks`

## Phase 6: Fabrication Export And Manifest Integration

### Objective

Make physical-rule evidence part of the fabrication readiness gate.

### Implementation Steps

1. Extend `fabrication.Summary` with physical-rule evidence.
2. Add a physical-rule artifact kind or evidence entry:
   - suggested path: `fabrication/physical-rules.json`.
3. Run physical checks during:
   - `export preview`;
   - `export fabrication` dry-run;
   - `export fabrication --execute`.
4. Write `physical-rules.json` during executed package export.
5. Add the report to `package-manifest.json`.
6. Ensure readiness status rules:
   - `ready` requires no physical-rule blockers;
   - `candidate` may allow warnings;
   - blockers keep output `blocked`.
7. Add CLI JSON tests for preview, execute, manifest, and readiness downgrade.

### Review Checklist

- Existing fabrication export dry-run behavior remains non-mutating.
- Executed export writes the new report only under the package directory.
- Manifest paths are relative and deterministic.
- Existing BOM/CPL/Gerber/drill tests remain stable.

### Suggested Commit

`Integrate physical checks into fabrication export`

## Phase 7: Design Workflow And Repair Evidence Hooks

### Objective

Expose physical-rule evidence where AI design generation and later repair logic
can use it.

### Implementation Steps

1. Run physical checks in `design create` for `fabrication-candidate`
   acceptance.
2. Add physical-rule summary to workflow output:
   - status;
   - blocker count;
   - warning count;
   - profile;
   - report path when written.
3. Ensure physical-rule blockers downgrade achieved acceptance.
4. Add issue paths and suggestions that map cleanly to future repair actions:
   - outline repair;
   - net-class adjustment;
   - footprint replacement;
   - placement movement;
   - silkscreen cleanup;
   - mounting-hole add/move.
5. Add tests for design workflow pass, warning, and blocker outcomes.

### Review Checklist

- The workflow does not overclaim fabrication readiness.
- Physical-rule output is concise enough for AI feedback loops.
- Generated-project-only mutation rules remain unchanged.

### Suggested Commit

`Expose physical fabrication checks in design workflow`

## Phase 8: Fixtures, Optional KiCad Evidence, And Documentation

### Objective

Round out confidence with representative fixtures and update user-facing docs.

### Implementation Steps

1. Add fixture boards for each physical-rule category.
2. Add optional KiCad CLI smoke tests gated by environment variable if useful.
3. Compare optional KiCad DRC findings against physical-rule categories where
   possible, without making KiCad required in default tests.
4. Update README:
   - describe physical fabrication checks;
   - document report path;
   - explain `ready` versus manufacturer acceptance;
   - show `kicadai --json export preview` and `kicadai --json --execute export
     fabrication` examples.
5. Update `specs/ROADMAP.md`:
   - mark fabrication physical checks as implemented foundation once complete;
   - leave remaining DFM limits explicit.
6. Run:
   - `gofmt` on changed Go files;
   - `go test ./...`;
   - prism review on staged changes before each implementation commit.

### Review Checklist

- Default test suite is hermetic.
- Optional KiCad tests skip cleanly.
- Docs do not imply manufacturer certification.
- Roadmap status is accurate.

### Suggested Commit

`Document fabrication physical rule checks`

## Completion Criteria

The full project is complete when:

- every rule category has at least one passing and one failing test fixture;
- physical-rule reports are deterministic JSON;
- fabrication export writes and manifests `physical-rules.json`;
- physical-rule blockers prevent `ready` status;
- `design create` fabrication-candidate output includes physical-rule evidence;
- README and roadmap describe the capability and remaining limitations;
- `go test ./...` passes;
- prism review is clean or findings are addressed before commits.
